/*
Copyright 2020 David Arnold
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"errors"
	"os"

	"github.com/jedib0t/go-pretty/table"
	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/cmd/top"
	"k8s.io/kubectl/pkg/metricsutil"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
	"k8s.io/metrics/pkg/apis/metrics"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
	metricsV1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var cfgFile string

func init() {
	cobra.OnInitialize(initConfig)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".glance" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".glance")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log.Println("Using config file:", viper.ConfigFileUsed())
	}
}

// GlanceOptions contains options and configurations needed by glance
type GlanceConfig struct {
	configFlags   *genericclioptions.ConfigFlags
	resourceFlags *genericclioptions.ResourceBuilderFlags
	restConfig    *rest.Config

	genericclioptions.IOStreams
}

// NewGlanceConfig provides an instance of GlanceConfig with default values
func NewGlanceConfig(streams genericclioptions.IOStreams) (*GlanceConfig, error) {
	cf := genericclioptions.NewConfigFlags(true)
	rc, err := cf.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	return &GlanceConfig{
		configFlags:   cf,
		IOStreams:     streams,
		resourceFlags: genericclioptions.NewResourceBuilderFlags(),
		restConfig:    rc,
	}, nil
}

// NewGlanceCmd provides a cobra command
func NewGlanceCmd(streams genericclioptions.IOStreams) *cobra.Command {

	// configFlags := genericclioptions.NewConfigFlags(true)
	// resourceFlags := genericclioptions.NewResourceBuilderFlags()
	gc, err := NewGlanceConfig(streams)
	if err != nil {
		log.Fatalf("Unable to create glance configuration: %v", err)
	}

	cmd := &cobra.Command{
		Use:   "glance",
		Short: "Take a quick glance at your Kubernetes resources.",
		Long:  "Glance allows you to quickly look at your kubernetes resource usage.",
		RunE: func(cmd *cobra.Command, args []string) error {
			gc.configFlags.AddFlags(cmd.Flags())
			gc.resourceFlags.AddFlags(cmd.Flags())

			// create the clientset
			k8sClient, err := kubernetes.NewForConfig(gc.restConfig)
			if err != nil {
				return err
			}

			err = GlanceK8s(k8sClient, gc)
			if err != nil {
				return err
			}
			return nil
		},
	}

	return cmd
}

//GlanceK8s displays cluster information for a given clientset
func GlanceK8s(k8sClient *kubernetes.Clientset, gc *GlanceConfig) (err error) {
	var (
		c counter
	)

	nm := make(nodeMap)
	nodes := getNodes(k8sClient)

	log.Infof("There are %d node(s) in the cluster\n", len(nodes.Items))
	for _, n := range nodes.Items {
		podList, err := getPods(k8sClient, n.Name)
		if err != nil {
			log.Errorf("Error getting Pod List: %+v ", err.Error())
		}

		nm[n.Name] = describeNodeResource(*podList, &n)
		nm[n.Name].allocatableCPU = n.Status.Allocatable.Cpu().Value()
		nm[n.Name].allocatableMemory = n.Status.Allocatable.Memory().Value()
		nm[n.Name].capacityCPU = n.Status.Capacity.Cpu().Value()
		nm[n.Name].capacityMemory = n.Status.Capacity.Memory().Value()
		c.totalAllocatableCPU = c.totalAllocatableCPU + n.Status.Allocatable.Cpu().Value()
		c.totalAllocatableMemory = c.totalAllocatableMemory + n.Status.Allocatable.Memory().Value()
		c.totalCapacityCPU = c.totalCapacityCPU + n.Status.Capacity.Cpu().Value()
		c.totalCapacityMemory = c.totalCapacityMemory + n.Status.Capacity.Memory().Value()
		nodeMetrics, _, err := getNodeUtilization(k8sClient, n.Name, gc)
		if err != nil {
			log.Fatalf("Unable to retrieve Node metrics: %v", err)
		}

		nm[n.Name].usageCPU = nodeMetrics[0].Usage.Cpu().AsDec().String()
		nm[n.Name].usageMemory = nodeMetrics[0].Usage.Memory().String()

	}

	render(&nm, &c)

	return nil
}

func render(nm *nodeMap, c *counter) {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Node Name", "ProviderID", "Allocatable CPU", "Allocatable MEM (Mi)", "Capacity CPU", "Capacity MEM (Mi)", "Allocated CPU Requests", "Allocated CPU Limits", "Allocated MEM Requests", "Allocated MEM Limits", "Usage CPU", "Usage Mem"})

	for k, v := range *nm {
		t.AppendRow([]interface{}{k, v.providerID,
			v.allocatableCPU, int64(v.allocatableMemory / 1024 / 1024),
			v.capacityCPU, int64(v.capacityMemory / 1024 / 1024), v.allocatedCPUrequests,
			v.allocatedCPULimits, v.allocatedMemoryRequests, v.allocatedMemoryLimits, v.usageCPU, v.usageMemory})

	}

	t.AppendFooter(table.Row{"", "", "Total", c.totalAllocatableCPU, int64(c.totalAllocatableMemory / 1024 / 1024), c.totalCapacityCPU, int(c.totalCapacityMemory / 1024 / 1024)})
	t.SetStyle(table.StyleBold)
	// t.SetStyle()
	t.Render()

}

func getNodes(clientset *kubernetes.Clientset) (nodes *v1.NodeList) {
	nodes, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	return nodes
}

func getPods(clientset *kubernetes.Clientset, nodeName string) (pods *v1.PodList, err error) {
	fieldSelector, err := fields.ParseSelector("spec.nodeName=" + nodeName + ",status.phase!=" + string(v1.PodSucceeded) + ",status.phase!=" + string(v1.PodFailed))
	if err != nil {
		return nil, err
	}

	nodeNonTerminatedPodsList, err := clientset.CoreV1().Pods("").List(metav1.ListOptions{FieldSelector: fieldSelector.String()})
	if err != nil {
		return nil, err
	}
	return nodeNonTerminatedPodsList, nil
}

// Based on: https://github.com/kubernetes/kubernetes/pkg/kubectl/describe/versioned/describe.go#L3223
func describeNodeResource(nodeNonTerminatedPodsList v1.PodList, node *v1.Node) *NodeStats {

	allocatable := node.Status.Capacity
	if len(node.Status.Allocatable) > 0 {
		allocatable = node.Status.Allocatable
	}

	reqs, limits := getPodsTotalRequestsAndLimits(&nodeNonTerminatedPodsList)

	// @TODO storage
	// cpuReqs, cpuLimits, memoryReqs, memoryLimits, ephemeralstorageReqs, ephemeralstorageLimits :=
	// reqs[corev1.ResourceCPU], limits[corev1.ResourceCPU], reqs[corev1.ResourceMemory], limits[corev1.ResourceMemory], reqs[corev1.ResourceEphemeralStorage], limits[corev1.ResourceEphemeralStorage]
	cpuReqs, cpuLimits, memoryReqs, memoryLimits :=
		reqs[v1.ResourceCPU], limits[v1.ResourceCPU], reqs[v1.ResourceMemory], limits[v1.ResourceMemory]
	fractionCpuReqs := float64(0)
	fractionCpuLimits := float64(0)
	if allocatable.Cpu().MilliValue() != 0 {
		fractionCpuReqs = float64(cpuReqs.MilliValue()) / float64(allocatable.Cpu().MilliValue()) * 100
		fractionCpuLimits = float64(cpuLimits.MilliValue()) / float64(allocatable.Cpu().MilliValue()) * 100
	}
	fractionMemoryReqs := float64(0)
	fractionMemoryLimits := float64(0)
	if allocatable.Memory().Value() != 0 {
		fractionMemoryReqs = float64(memoryReqs.Value()) / float64(allocatable.Memory().Value()) * 100
		fractionMemoryLimits = float64(memoryLimits.Value()) / float64(allocatable.Memory().Value()) * 100
	}
	//@TODO add Storage
	// fractionEphemeralStorageReqs := float64(0)
	// fractionEphemeralStorageLimits := float64(0)
	// if allocatable.StorageEphemeral().Value() != 0 {
	// 	fractionEphemeralStorageReqs = float64(ephemeralstorageReqs.Value()) / float64(allocatable.StorageEphemeral().Value()) * 100
	// 	fractionEphemeralStorageLimits = float64(ephemeralstorageLimits.Value()) / float64(allocatable.StorageEphemeral().Value()) * 100
	// }
	// return corev1.ResourceCPU, cpuReqs.String(), int64(fractionCpuReqs), cpuLimits.String(), int64(fractionCpuLimits),
	// 	corev1.ResourceMemory, memoryReqs.String(), int64(fractionMemoryReqs), memoryLimits.String(), int64(fractionMemoryLimits)

	ns := &NodeStats{
		allocatedCPUrequests:    int64(fractionCpuReqs),
		allocatedCPULimits:      int64(fractionCpuLimits),
		allocatedMemoryRequests: int64(fractionMemoryReqs),
		allocatedMemoryLimits:   int64(fractionMemoryLimits),
	}
	return ns

}

// Based on: https://github.com/kubernetes/kubernetes/pkg/kubectl/describe/versioned/describe.go#L3223
func getPodsTotalRequestsAndLimits(podList *v1.PodList) (reqs map[v1.ResourceName]resource.Quantity, limits map[v1.ResourceName]resource.Quantity) {
	reqs, limits = map[v1.ResourceName]resource.Quantity{}, map[v1.ResourceName]resource.Quantity{}
	for _, pod := range podList.Items {
		podReqs, podLimits := resourcehelper.PodRequestsAndLimits(&pod)
		for podReqName, podReqValue := range podReqs {
			if value, ok := reqs[podReqName]; !ok {
				reqs[podReqName] = podReqValue.DeepCopy()
			} else {
				value.Add(podReqValue)
				reqs[podReqName] = value
			}
		}
		for podLimitName, podLimitValue := range podLimits {
			if value, ok := limits[podLimitName]; !ok {
				limits[podLimitName] = podLimitValue.DeepCopy()
			} else {
				value.Add(podLimitValue)
				limits[podLimitName] = value
			}
		}
	}
	return
}

// reimplementation of https://github.com/kubernetes/kubernetes/blob/master/pkg/kubectl/cmd/top/top_node.go#L159
func getNodeUtilization(clientset *kubernetes.Clientset, nodeName string, gc *GlanceConfig) ([]metrics.NodeMetrics, map[string]v1.ResourceList, error) {

	metricsClientset, err := metricsclientset.NewForConfig(gc.restConfig)
	if err != nil {
		return nil, nil, err
	}

	labelSelector := labels.Everything()

	if gc.resourceFlags.LabelSelector != nil && len(*gc.resourceFlags.LabelSelector) > 0 {
		labelSelector, err = labels.Parse(*gc.resourceFlags.LabelSelector)
		if err != nil {
			return nil, nil, err
		}
	}

	apiGroups, err := clientset.DiscoveryClient.ServerGroups()
	if err != nil {
		return nil, nil, err
	}
	metricsAPIAvailable := top.SupportedMetricsAPIVersionAvailable(apiGroups)

	heapsterClient := metricsutil.NewHeapsterMetricsClient(clientset.CoreV1(),
		metricsutil.DefaultHeapsterNamespace, metricsutil.DefaultHeapsterScheme, metricsutil.DefaultHeapsterService, metricsutil.DefaultHeapsterPort)

	//nolint staticcheck
	metrics := &metricsapi.NodeMetricsList{}
	if metricsAPIAvailable {
		metrics, err = getNodeMetricsFromMetricsAPI(metricsClientset, nodeName, labelSelector)
		if err != nil {
			return nil, nil, err
		}
	} else {
		metrics, err = heapsterClient.GetNodeMetrics(nodeName, labelSelector.String())
		if err != nil {
			return nil, nil, err
		}
	}
	if len(metrics.Items) == 0 {
		return nil, nil, errors.New("metrics not available yet")
	}
	var nodes []v1.Node
	if len(nodeName) > 0 {
		node, err := clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return nil, nil, err
		}
		nodes = append(nodes, *node)
	} else {
		nodeList, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{
			LabelSelector: labelSelector.String(),
		})
		if err != nil {
			return nil, nil, err
		}
		nodes = append(nodes, nodeList.Items...)
	}

	allocatable := make(map[string]v1.ResourceList)

	for _, n := range nodes {
		allocatable[n.Name] = n.Status.Allocatable
	}

	return metrics.Items, allocatable, nil
}

func getNodeMetricsFromMetricsAPI(metricsClient metricsclientset.Interface, resourceName string, selector labels.Selector) (*metricsapi.NodeMetricsList, error) {
	var err error
	versionedMetrics := &metricsV1beta1api.NodeMetricsList{}
	mc := metricsClient.MetricsV1beta1()
	nm := mc.NodeMetricses()
	if resourceName != "" {
		m, err := nm.Get(resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		versionedMetrics.Items = []metricsV1beta1api.NodeMetrics{*m}
	} else {
		versionedMetrics, err = nm.List(metav1.ListOptions{LabelSelector: selector.String()})
		if err != nil {
			return nil, err
		}
	}
	metrics := &metricsapi.NodeMetricsList{}
	err = metricsV1beta1api.Convert_v1beta1_NodeMetricsList_To_metrics_NodeMetricsList(versionedMetrics, metrics, nil)
	if err != nil {
		return nil, err
	}
	return metrics, nil
}
