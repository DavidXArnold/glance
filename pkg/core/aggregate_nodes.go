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
		TotalUsageCPU:                resource.NewMilliQuantity(0, resource.DecimalSI),
		TotalUsageMemory:             resource.NewQuantity(0, resource.BinarySI),
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
			// with status only and excluded from totals.
			nm[name] = &NodeStats{Status: "Not Ready"}
			continue
		}

		stats := &NodeStats{Status: "Ready"}

		// Copy node info.
		stats.NodeInfo = node.Status.NodeInfo

		// Allocatable and capacity resources.
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

		// Aggregate pod-level requests/limits from the pods scheduled on this node.
		pods := podsByNode[name]
		stats.PodCount = len(pods)

		cpuReq := resource.NewMilliQuantity(0, resource.DecimalSI)
		cpuLim := resource.NewMilliQuantity(0, resource.DecimalSI)
		memReq := resource.NewQuantity(0, resource.BinarySI)
		memLim := resource.NewQuantity(0, resource.BinarySI)

		for _, pod := range pods {
			for _, container := range pod.Spec.Containers {
				if req := container.Resources.Requests.Cpu(); req != nil {
					cpuReq.Add(*req)
				}
				if lim := container.Resources.Limits.Cpu(); lim != nil {
					cpuLim.Add(*lim)
				}
				if req := container.Resources.Requests.Memory(); req != nil {
					memReq.Add(*req)
				}
				if lim := container.Resources.Limits.Memory(); lim != nil {
					memLim.Add(*lim)
				}
			}
		}

		stats.AllocatedCPUrequests = *cpuReq
		stats.AllocatedCPULimits = *cpuLim
		stats.AllocatedMemoryRequests = *memReq
		stats.AllocatedMemoryLimits = *memLim

		// Update totals with allocatable, capacity, and allocated values.
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

		// Usage from metrics-server. If metrics are missing for a Ready node,
		// treat usage as unknown (zero) but do not fail the snapshot. This
		// avoids hard failures during scale up/down when metrics-server has not
		// yet scraped new nodes.
		if m := nodeMetrics[name]; m != nil {
			if cpuQty, ok := m.Usage[v1.ResourceCPU]; ok {
				q := cpuQty.DeepCopy()
				stats.UsageCPU = &q
				totals.TotalUsageCPU.Add(*stats.UsageCPU)
			}
			if memQty, ok := m.Usage[v1.ResourceMemory]; ok {
				q := memQty.DeepCopy()
				stats.UsageMemory = &q
				totals.TotalUsageMemory.Add(*stats.UsageMemory)
			}
		}

		nm[name] = stats
	}

	return nm, totals, nil
}
