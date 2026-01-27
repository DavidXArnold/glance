/*
Package cmd contains the kubectl-glance CLI commands and shared helpers.

This file defines reusable aggregation helpers for pods and deployments that
are used by both the live TUI (live.go) and static CLI views.
*/

package cmd

import (
	"context"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	Status    string
}

// CollectPodStats aggregates pod-level resource stats for a given namespace and optional selectors.
// It is a shared helper used by both static pod views and the live TUI.
func CollectPodStats(
	ctx context.Context,
	k8sClient *kubernetes.Clientset,
	metricsClient metricsclientset.Interface,
	namespace string,
	selector labels.Selector,
) ([]PodSummaryRow, error) {
	// List pods in the namespace using the provided selector (if any).
	// Use ResourceVersion="0" to leverage the API server watch cache for
	// consistent behavior with live views and better performance.
	listOptions := metav1.ListOptions{ResourceVersion: "0"}
	if selector != nil && !selector.Empty() {
		listOptions.LabelSelector = selector.String()
	}

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

	metricsMap := make(map[string]*metricsv1beta1.PodMetrics)
	if podMetricsList != nil {
		for i := range podMetricsList.Items {
			pm := &podMetricsList.Items[i]
			metricsMap[pm.Name] = pm
		}
	}

	rows := make([]PodSummaryRow, 0, len(pods.Items))

	for i := range pods.Items {
		pod := &pods.Items[i]

		cpuReq := resource.NewMilliQuantity(0, resource.DecimalSI)
		cpuLimit := resource.NewMilliQuantity(0, resource.DecimalSI)
		memReq := resource.NewQuantity(0, resource.BinarySI)
		memLimit := resource.NewQuantity(0, resource.BinarySI)
		cpuUsage := resource.NewMilliQuantity(0, resource.DecimalSI)
		memUsage := resource.NewQuantity(0, resource.BinarySI)

		for _, container := range pod.Spec.Containers {
			if req := container.Resources.Requests.Cpu(); req != nil {
				cpuReq.Add(*req)
			}
			if lim := container.Resources.Limits.Cpu(); lim != nil {
				cpuLimit.Add(*lim)
			}
			if req := container.Resources.Requests.Memory(); req != nil {
				memReq.Add(*req)
			}
			if lim := container.Resources.Limits.Memory(); lim != nil {
				memLimit.Add(*lim)
			}
		}

		if pm, ok := metricsMap[pod.Name]; ok {
			for _, container := range pm.Containers {
				cpuUsage.Add(container.Usage[v1.ResourceCPU])
				memUsage.Add(container.Usage[v1.ResourceMemory])
			}
		}

		status := string(pod.Status.Phase)

		row := PodSummaryRow{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			CPUReq:    cpuReq,
			CPULimit:  cpuLimit,
			CPUUsage:  cpuUsage,
			MemReq:    memReq,
			MemLimit:  memLimit,
			MemUsage:  memUsage,
			Status:    status,
		}

		rows = append(rows, row)
	}

	return rows, nil
}

// CollectDeploymentStats aggregates deployment-level resource stats for a given namespace and optional selectors.
// It mirrors the logic in fetchDeploymentData but is usable from non-TUI contexts.
func CollectDeploymentStats(
	ctx context.Context,
	k8sClient *kubernetes.Clientset,
	namespace string,
	selector labels.Selector,
) ([]DeploymentSummaryRow, error) {
	// Use ResourceVersion="0" to leverage the API server watch cache, matching
	// the behavior used by live views for consistency and performance.
	listOptions := metav1.ListOptions{ResourceVersion: "0"}
	if selector != nil && !selector.Empty() {
		listOptions.LabelSelector = selector.String()
	}

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

		for _, container := range deploy.Spec.Template.Spec.Containers {
			if req := container.Resources.Requests.Cpu(); req != nil {
				cpuReq.Add(*req)
			}
			if lim := container.Resources.Limits.Cpu(); lim != nil {
				cpuLimit.Add(*lim)
			}
			if req := container.Resources.Requests.Memory(); req != nil {
				memReq.Add(*req)
			}
			if lim := container.Resources.Limits.Memory(); lim != nil {
				memLimit.Add(*lim)
			}
		}

		replicas := int32(1)
		if deploy.Spec.Replicas != nil {
			replicas = *deploy.Spec.Replicas
		}

		cpuReq = multiplyQuantity(cpuReq, int(replicas))
		cpuLimit = multiplyQuantity(cpuLimit, int(replicas))
		memReq = multiplyQuantity(memReq, int(replicas))
		memLimit = multiplyQuantity(memLimit, int(replicas))

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
			Status:    status,
		}

		rows = append(rows, row)
	}

	return rows, nil
}
