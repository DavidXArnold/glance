/*
Package cmd contains the kubectl-glance CLI commands and shared helpers.

This file defines reusable aggregation helpers for pods and deployments that
are used by both the live TUI (live.go) and static CLI views.
*/

package cmd

import (
	"context"

	log "github.com/sirupsen/logrus"
	core "gitlab.com/davidxarnold/glance/pkg/core"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

// PodSummaryRow holds the textual columns and metrics for a single pod in a static view.
type PodSummaryRow struct {
	Namespace string
	Name      string
	CPUReq    *resource.Quantity
	CPULimit  *resource.Quantity
	CPUUsage  *resource.Quantity
	MemReq    *resource.Quantity
	MemLimit  *resource.Quantity
	MemUsage  *resource.Quantity
	GPUReq    *resource.Quantity
	GPULimit  *resource.Quantity
	Status    string
}

// DeploymentSummaryRow holds the textual columns and metrics for a single deployment in a static view.
type DeploymentSummaryRow struct {
	Namespace string
	Name      string
	Replicas  int32
	Ready     int32
	Available int32
	CPUReq    *resource.Quantity
	CPULimit  *resource.Quantity
	MemReq    *resource.Quantity
	MemLimit  *resource.Quantity
	GPUReq    *resource.Quantity
	GPULimit  *resource.Quantity
	Status    string
}

// CollectPodStats aggregates pod-level resource stats for a given namespace and optional selectors.
// It is a shared helper used by both static pod views and the live TUI.
func CollectPodStats(
	ctx context.Context,
	k8sClient kubernetes.Interface,
	metricsClient metricsclientset.Interface,
	namespace string,
	selector labels.Selector,
	fieldSelector fields.Selector,
) ([]PodSummaryRow, error) {
	// List pods in the namespace using the provided selectors (if any).
	// Use ResourceVersion="0" to leverage the API server watch cache for
	// consistent behavior with live views and better performance.
	listOptions := buildListOptions(selector, fieldSelector)

	pods, err := k8sClient.CoreV1().Pods(namespace).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	// Try to fetch metrics for the same namespace; failure is logged but not fatal.
	var podMetricsList *metricsv1beta1.PodMetricsList
	if metricsClient != nil {
		listOpts := metav1.ListOptions{ResourceVersion: "0"}
		podMetricsList, err = metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, listOpts)
		if err != nil {
			log.Debugf("Failed to fetch pod metrics for namespace %s: %v", namespace, err)
		}
	}

	return podSummaryRows(pods.Items, podMetricsList), nil
}

// podSummaryRows builds pod summary rows from pods and their metrics
// (podMetrics may be nil). The live view calls it with cached pods.
func podSummaryRows(pods []v1.Pod, podMetrics *metricsv1beta1.PodMetricsList) []PodSummaryRow {
	// Key by namespace and name: when listing all namespaces, pod names are
	// not unique (e.g. StatefulSet pods such as redis-0).
	metricsMap := make(map[types.NamespacedName]*metricsv1beta1.PodMetrics)
	if podMetrics != nil {
		for i := range podMetrics.Items {
			pm := &podMetrics.Items[i]
			metricsMap[types.NamespacedName{Namespace: pm.Namespace, Name: pm.Name}] = pm
		}
	}

	rows := make([]PodSummaryRow, 0, len(pods))

	for i := range pods {
		pod := &pods[i]

		cpuReq := resource.NewMilliQuantity(0, resource.DecimalSI)
		cpuLimit := resource.NewMilliQuantity(0, resource.DecimalSI)
		memReq := resource.NewQuantity(0, resource.BinarySI)
		memLimit := resource.NewQuantity(0, resource.BinarySI)
		cpuUsage := resource.NewMilliQuantity(0, resource.DecimalSI)
		memUsage := resource.NewQuantity(0, resource.BinarySI)

		gpuReq := resource.NewQuantity(0, resource.DecimalSI)
		gpuLimit := resource.NewQuantity(0, resource.DecimalSI)

		// Effective requests/limits as the scheduler computes them (init
		// containers, sidecars, overhead and pod-level resources included).
		r := effectivePodResources(pod)
		cpuReq.Add(r.cpuReq)
		cpuLimit.Add(r.cpuLimit)
		memReq.Add(r.memReq)
		memLimit.Add(r.memLimit)
		gpuReq.Add(r.gpuReq)
		gpuLimit.Add(r.gpuLimit)

		if pm, ok := metricsMap[types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}]; ok {
			for _, container := range pm.Containers {
				cpuUsage.Add(container.Usage[v1.ResourceCPU])
				memUsage.Add(container.Usage[v1.ResourceMemory])
			}
		}

		status := podStatusReason(pod)

		row := PodSummaryRow{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			CPUReq:    cpuReq,
			CPULimit:  cpuLimit,
			CPUUsage:  cpuUsage,
			MemReq:    memReq,
			MemLimit:  memLimit,
			MemUsage:  memUsage,
			GPUReq:    gpuReq,
			GPULimit:  gpuLimit,
			Status:    status,
		}

		rows = append(rows, row)
	}

	return rows
}

// CollectDeploymentStats aggregates deployment-level resource stats for a given namespace and optional selectors.
// It mirrors the logic in fetchDeploymentData but is usable from non-TUI contexts.
func CollectDeploymentStats(
	ctx context.Context,
	k8sClient kubernetes.Interface,
	namespace string,
	selector labels.Selector,
	fieldSelector fields.Selector,
) ([]DeploymentSummaryRow, error) {
	// Use ResourceVersion="0" to leverage the API server watch cache, matching
	// the behavior used by live views for consistency and performance.
	listOptions := buildListOptions(selector, fieldSelector)

	deployments, err := k8sClient.AppsV1().Deployments(namespace).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	rows := make([]DeploymentSummaryRow, 0, len(deployments.Items))

	for i := range deployments.Items {
		deploy := &deployments.Items[i]

		cpuReq := resource.NewMilliQuantity(0, resource.DecimalSI)
		cpuLimit := resource.NewMilliQuantity(0, resource.DecimalSI)
		memReq := resource.NewQuantity(0, resource.BinarySI)
		memLimit := resource.NewQuantity(0, resource.BinarySI)
		gpuReq := resource.NewQuantity(0, resource.DecimalSI)
		gpuLimit := resource.NewQuantity(0, resource.DecimalSI)

		// Effective requests/limits as the scheduler computes them (init
		// containers, sidecars, overhead and pod-level resources included).
		r := effectivePodResources(&v1.Pod{Spec: deploy.Spec.Template.Spec})
		cpuReq.Add(r.cpuReq)
		cpuLimit.Add(r.cpuLimit)
		memReq.Add(r.memReq)
		memLimit.Add(r.memLimit)
		gpuReq.Add(r.gpuReq)
		gpuLimit.Add(r.gpuLimit)

		replicas := int32(1)
		if deploy.Spec.Replicas != nil {
			replicas = *deploy.Spec.Replicas
		}

		cpuReq = multiplyQuantity(cpuReq, int(replicas))
		cpuLimit = multiplyQuantity(cpuLimit, int(replicas))
		memReq = multiplyQuantity(memReq, int(replicas))
		memLimit = multiplyQuantity(memLimit, int(replicas))
		gpuReq = multiplyQuantity(gpuReq, int(replicas))
		gpuLimit = multiplyQuantity(gpuLimit, int(replicas))

		status := statusReady + " " + nodeStatusReady
		if deploy.Status.ReadyReplicas < replicas {
			if deploy.Status.ReadyReplicas == 0 {
				status = statusFailed + " " + nodeStatusNotReady
			} else {
				status = statusPending + " Partial"
			}
		}

		row := DeploymentSummaryRow{
			Namespace: deploy.Namespace,
			Name:      deploy.Name,
			Replicas:  replicas,
			Ready:     deploy.Status.ReadyReplicas,
			Available: deploy.Status.AvailableReplicas,
			CPUReq:    cpuReq,
			CPULimit:  cpuLimit,
			MemReq:    memReq,
			MemLimit:  memLimit,
			GPUReq:    gpuReq,
			GPULimit:  gpuLimit,
			Status:    status,
		}

		rows = append(rows, row)
	}

	return rows, nil
}

// buildListOptions returns list options that read from the API server watch
// cache and apply the label and field selectors when they are non-empty.
func buildListOptions(selector labels.Selector, fieldSelector fields.Selector) metav1.ListOptions {
	listOptions := metav1.ListOptions{ResourceVersion: "0"}
	if selector != nil && !selector.Empty() {
		listOptions.LabelSelector = selector.String()
	}
	if fieldSelector != nil && !fieldSelector.Empty() {
		listOptions.FieldSelector = fieldSelector.String()
	}
	return listOptions
}

// podResources holds a pod's effective CPU, memory, and accelerator requests
// and limits.
type podResources struct {
	cpuReq, cpuLimit resource.Quantity
	memReq, memLimit resource.Quantity
	gpuReq, gpuLimit resource.Quantity
}

// effectivePodResources computes a pod's requests and limits the way the
// scheduler does, so init containers, sidecars, pod overhead, and pod-level
// resources are counted. Accelerators are summed across resource types.
func effectivePodResources(pod *v1.Pod) podResources {
	reqs := core.PodRequests(pod)
	lims := core.PodLimits(pod)
	return podResources{
		cpuReq:   *reqs.Cpu(),
		cpuLimit: *lims.Cpu(),
		memReq:   *reqs.Memory(),
		memLimit: *lims.Memory(),
		gpuReq:   *core.SumAccelerators(reqs),
		gpuLimit: *core.SumAccelerators(lims),
	}
}
