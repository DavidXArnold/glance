/*
Package core provides domain types and aggregation logic for glance
that are independent of any particular CLI or UI.
*/

package core

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metricsV1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// NodeSnapshotOptions controls behavior of ComputeNodeSnapshot.
type NodeSnapshotOptions struct {
	// RequireMetrics indicates whether missing metrics for a Ready node
	// should be treated as an error. Static glance currently expects
	// metrics to be available, whereas live mode can tolerate gaps.
	RequireMetrics bool
}

// ComputeNodeSnapshot builds a NodeMap and Totals from the provided
// Kubernetes objects. It does not perform any API calls; callers are
// responsible for fetching Nodes, Pods, and NodeMetrics.
func ComputeNodeSnapshot(
	nodes []v1.Node,
	podsByNode map[string][]v1.Pod,
	nodeMetrics map[string]*metricsV1beta1api.NodeMetrics,
	opts NodeSnapshotOptions,
) (NodeMap, Totals, error) {
	// Initialize totals with zero-valued quantities so we can safely Add().
	totals := Totals{
		TotalAllocatableCPU:          resource.NewMilliQuantity(0, resource.DecimalSI),
		TotalAllocatableMemory:       resource.NewQuantity(0, resource.BinarySI),
		TotalCapacityCPU:             resource.NewMilliQuantity(0, resource.DecimalSI),
		TotalCapacityMemory:          resource.NewQuantity(0, resource.BinarySI),
		TotalAllocatedCPUrequests:    resource.NewMilliQuantity(0, resource.DecimalSI),
		TotalAllocatedCPULimits:      resource.NewMilliQuantity(0, resource.DecimalSI),
		TotalAllocatedMemoryRequests: resource.NewQuantity(0, resource.BinarySI),
		TotalAllocatedMemoryLimits:   resource.NewQuantity(0, resource.BinarySI),
		TotalAllocatableGPU:          resource.NewQuantity(0, resource.DecimalSI),
		TotalCapacityGPU:             resource.NewQuantity(0, resource.DecimalSI),
		TotalAllocatedGPURequests:    resource.NewQuantity(0, resource.DecimalSI),
		TotalAllocatedGPULimits:      resource.NewQuantity(0, resource.DecimalSI),
		TotalUsageCPU:                resource.NewMilliQuantity(0, resource.DecimalSI),
		TotalUsageMemory:             resource.NewQuantity(0, resource.BinarySI),
		Accelerators:                 AcceleratorMap{},
	}

	nm := make(NodeMap, len(nodes))

	for i := range nodes {
		node := nodes[i]
		name := node.Name

		// Determine Ready / NotReady status from conditions.
		var readyCondition *v1.NodeCondition
		for j := range node.Status.Conditions {
			if node.Status.Conditions[j].Type == v1.NodeReady {
				readyCondition = &node.Status.Conditions[j]
				break
			}
		}

		if readyCondition == nil || readyCondition.Status != v1.ConditionTrue {
			// Preserve existing static behavior: NotReady nodes are included
			// with status only and excluded from totals, but we still record
			// basic metadata such as creation time for AGE column rendering.
			nm[name] = &NodeStats{
				Status:       "Not Ready",
				CreationTime: node.CreationTimestamp.Time,
			}
			continue
		}

		stats := &NodeStats{
			Status:       "Ready",
			CreationTime: node.CreationTimestamp.Time,
		}

		// Copy node info.
		stats.NodeInfo = node.Status.NodeInfo

		populateNodeResources(stats, &node)
		aggregatePodResources(stats, podsByNode[name])
		applyNodeMetrics(stats, nodeMetrics[name])
		accumulateTotals(&totals, stats)

		nm[name] = stats
	}

	if len(totals.Accelerators) == 0 {
		totals.Accelerators = nil
	}
	totals.GPUGroups = buildGPUGroups(nm)

	return nm, totals, nil
}

// populateNodeResources fills allocatable, capacity, and GPU fields on stats from node status.
func populateNodeResources(stats *NodeStats, node *v1.Node) {
	if cpu := node.Status.Allocatable.Cpu(); cpu != nil {
		q := cpu.DeepCopy()
		stats.AllocatableCPU = &q
	}
	if mem := node.Status.Allocatable.Memory(); mem != nil {
		q := mem.DeepCopy()
		stats.AllocatableMemory = &q
	}
	if cpu := node.Status.Capacity.Cpu(); cpu != nil {
		q := cpu.DeepCopy()
		stats.CapacityCPU = &q
	}
	if mem := node.Status.Capacity.Memory(); mem != nil {
		q := mem.DeepCopy()
		stats.CapacityMemory = &q
	}

	// Accelerator allocatable and capacity, per resource type and summed.
	acc := AcceleratorMap{}
	for rName, qty := range node.Status.Allocatable {
		if IsGPUResource(rName) {
			acc.get(rName).Allocatable = qty.Value()
		}
	}
	for rName, qty := range node.Status.Capacity {
		if IsGPUResource(rName) {
			acc.get(rName).Capacity = qty.Value()
		}
	}
	if len(acc) == 0 {
		return
	}
	stats.Accelerators = acc

	if gpuAllocatable := SumAccelerators(node.Status.Allocatable); !gpuAllocatable.IsZero() {
		stats.AllocatableGPU = gpuAllocatable
	}
	if gpuCapacity := SumAccelerators(node.Status.Capacity); !gpuCapacity.IsZero() {
		stats.CapacityGPU = gpuCapacity
	}
	stats.GPUModel, stats.GPUMemoryMiB = GPUModelFromLabels(node.Labels)
}

// aggregatePodResources sums CPU, memory, and GPU requests/limits from all pods into stats.
func aggregatePodResources(stats *NodeStats, pods []v1.Pod) {
	stats.PodCount = len(pods)

	cpuReq := resource.NewMilliQuantity(0, resource.DecimalSI)
	cpuLim := resource.NewMilliQuantity(0, resource.DecimalSI)
	memReq := resource.NewQuantity(0, resource.BinarySI)
	memLim := resource.NewQuantity(0, resource.BinarySI)
	gpuReq := resource.NewQuantity(0, resource.DecimalSI)
	gpuLim := resource.NewQuantity(0, resource.DecimalSI)

	for i := range pods {
		// Use the scheduler's view of the pod so init containers, sidecars,
		// overhead and pod-level resources are accounted for.
		reqs := PodRequests(&pods[i])
		lims := PodLimits(&pods[i])

		cpuReq.Add(*reqs.Cpu())
		cpuLim.Add(*lims.Cpu())
		memReq.Add(*reqs.Memory())
		memLim.Add(*lims.Memory())

		for rName, qty := range reqs {
			if IsGPUResource(rName) {
				gpuReq.Add(qty)
				if stats.Accelerators == nil {
					stats.Accelerators = AcceleratorMap{}
				}
				stats.Accelerators.get(rName).Requests += qty.Value()
			}
		}
		for rName, qty := range lims {
			if IsGPUResource(rName) {
				gpuLim.Add(qty)
				if stats.Accelerators == nil {
					stats.Accelerators = AcceleratorMap{}
				}
				stats.Accelerators.get(rName).Limits += qty.Value()
			}
		}
	}

	stats.AllocatedCPUrequests = *cpuReq
	stats.AllocatedCPULimits = *cpuLim
	stats.AllocatedMemoryRequests = *memReq
	stats.AllocatedMemoryLimits = *memLim
	stats.AllocatedGPURequests = *gpuReq
	stats.AllocatedGPULimits = *gpuLim
}

// applyNodeMetrics records usage metrics from the metrics-server onto stats.
func applyNodeMetrics(stats *NodeStats, m *metricsV1beta1api.NodeMetrics) {
	if m == nil {
		return
	}
	if cpuQty, ok := m.Usage[v1.ResourceCPU]; ok {
		q := cpuQty.DeepCopy()
		stats.UsageCPU = &q
	}
	if memQty, ok := m.Usage[v1.ResourceMemory]; ok {
		q := memQty.DeepCopy()
		stats.UsageMemory = &q
	}
}

// accumulateTotals adds a single node's stats into the running totals.
func accumulateTotals(totals *Totals, stats *NodeStats) {
	if stats.AllocatableCPU != nil {
		totals.TotalAllocatableCPU.Add(*stats.AllocatableCPU)
	}
	if stats.AllocatableMemory != nil {
		totals.TotalAllocatableMemory.Add(*stats.AllocatableMemory)
	}
	if stats.CapacityCPU != nil {
		totals.TotalCapacityCPU.Add(*stats.CapacityCPU)
	}
	if stats.CapacityMemory != nil {
		totals.TotalCapacityMemory.Add(*stats.CapacityMemory)
	}

	totals.TotalAllocatedCPUrequests.Add(stats.AllocatedCPUrequests)
	totals.TotalAllocatedCPULimits.Add(stats.AllocatedCPULimits)
	totals.TotalAllocatedMemoryRequests.Add(stats.AllocatedMemoryRequests)
	totals.TotalAllocatedMemoryLimits.Add(stats.AllocatedMemoryLimits)

	if stats.AllocatableGPU != nil {
		totals.TotalAllocatableGPU.Add(*stats.AllocatableGPU)
	}
	if stats.CapacityGPU != nil {
		totals.TotalCapacityGPU.Add(*stats.CapacityGPU)
	}
	totals.TotalAllocatedGPURequests.Add(stats.AllocatedGPURequests)
	totals.TotalAllocatedGPULimits.Add(stats.AllocatedGPULimits)
	for name, acc := range stats.Accelerators {
		totals.Accelerators.get(name).add(acc)
	}

	if stats.UsageCPU != nil {
		totals.TotalUsageCPU.Add(*stats.UsageCPU)
	}
	if stats.UsageMemory != nil {
		totals.TotalUsageMemory.Add(*stats.UsageMemory)
	}
}
