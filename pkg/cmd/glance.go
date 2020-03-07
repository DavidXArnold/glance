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
	"fmt"
	"os"
	"strings"

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
	nodeutil "k8s.io/kubernetes/pkg/controller/util/node"
	"k8s.io/metrics/pkg/apis/metrics"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
	metricsV1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	// needed to authenticate with GCP
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var (
	cfgFile               string
	KubernetesConfigFlags *genericclioptions.ConfigFlags
)

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			log.Fatalln(err)
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
func NewGlanceConfig() (gc *GlanceConfig, err error) {
	cf := genericclioptions.NewConfigFlags(true)
	rc, err := cf.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	return &GlanceConfig{
		configFlags: cf,
		restConfig:  rc,
	}, err
}

// NewGlanceCmd provides a cobra command
func NewGlanceCmd() *cobra.Command {
	var labelSelector string
	var fieldSelector string

	KubernetesConfigFlags = genericclioptions.NewConfigFlags(false)

	gc, err := NewGlanceConfig()
	if err != nil {
		log.Fatalf("Unable to create glance configuration: %v", err)
	}

	cmd := &cobra.Command{
		Use:           "glance",
		Short:         "Take a quick glance at your Kubernetes resources.",
		Long:          "Glance allows you to quickly look at your kubernetes resource usage.",
		SilenceErrors: true,
		SilenceUsage:  true,
		PreRun: func(cmd *cobra.Command, args []string) {
			viper.BindPFlags(cmd.Flags())
		},
		RunE: func(cmd *cobra.Command, args []string) error {
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

	cmd.PersistentFlags().StringVar(
		&fieldSelector, "field-selector", "", "Selector (field query) to filter on, supports '=', '==', and '!='.(e.g. --field-selector key1=value1,key2=value2). The server only supports a limited number of field queries per type.")
	viper.BindPFlag("field-selector", cmd.PersistentFlags().Lookup("field-selector"))
	cmd.PersistentFlags().StringVar(
		&labelSelector, "selector", "", "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)")

	KubernetesConfigFlags.AddFlags(cmd.Flags())
	cobra.OnInitialize(initConfig)

	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	viper.BindPFlag("selector", cmd.PersistentFlags().Lookup("selector"))
	viper.BindPFlags(cmd.Flags())

	return cmd
}

// GlanceK8s displays cluster information for a given clientset
func GlanceK8s(k8sClient *kubernetes.Clientset, gc *GlanceConfig) (err error) {

	c := &counter{
		totalAllocatableCPU:          resource.NewMilliQuantity(0, resource.DecimalSI),
		totalAllocatableMemory:       resource.NewQuantity(0, resource.BinarySI),
		totalCapacityCPU:             resource.NewMilliQuantity(0, resource.DecimalSI),
		totalCapacityMemory:          resource.NewQuantity(0, resource.BinarySI),
		totalAllocatedCPUrequests:    resource.NewMilliQuantity(0, resource.DecimalSI),
		totalAllocatedCPULimits:      resource.NewMilliQuantity(0, resource.DecimalSI),
		totalAllocatedMemoryRequests: resource.NewQuantity(0, resource.BinarySI),
		totalAllocatedMemoryLimits:   resource.NewQuantity(0, resource.BinarySI),
		totalUsageCPU:                resource.NewMilliQuantity(0, resource.DecimalSI),
		totalUsageMemory:             resource.NewQuantity(0, resource.BinarySI),
	}

	nm := make(nodeMap)
	nodes, err := getNodes(k8sClient)
	if err != nil {
		log.Fatalf("Error getting Node list from host: %+v ", err.Error())
	}

	if len(nodes.Items) == 0 {
		return fmt.Errorf("no Nodes found")
	}

	log.WithFields(log.Fields{
		"Host": gc.restConfig.Host,
	}).Infof("There are %d node(s) in the cluster\n", len(nodes.Items))

	for _, n := range nodes.Items {
		_, nc := nodeutil.GetNodeCondition(
			&n.Status,
			v1.NodeReady)

		if nc.Type != v1.NodeReady && nc.Status != "True" {
			nm[n.Name] = &NodeStats{
				status: "Not Ready",
			}
			continue
		}

		podList, err := getPods(k8sClient, n.Name)
		if err != nil {
			log.Fatalf("Error getting Pod list from host: %+v ", err.Error())
		}

		nm[n.Name] = describeNodeResource(podList, &n)

		if n.Spec.ProviderID != "" {
			nm[n.Name].providerID = n.Spec.ProviderID
		}

		nm[n.Name].status = "Ready"
		nm[n.Name].allocatableCPU = n.Status.Allocatable.Cpu()
		nm[n.Name].allocatableMemory = n.Status.Allocatable.Memory()

		n.Status.Allocatable.Cpu().Add(*c.totalAllocatableCPU)
		n.Status.Allocatable.Memory().Add(*c.totalAllocatableMemory)
		c.totalAllocatedCPUrequests.Add(nm[n.Name].allocatedCPUrequests)
		c.totalAllocatedCPULimits.Add(nm[n.Name].allocatedCPULimits)
		c.totalAllocatedMemoryRequests.Add(nm[n.Name].allocatedMemoryRequests)
		c.totalAllocatedMemoryLimits.Add(nm[n.Name].allocatedMemoryLimits)

		nodeMetrics, _, err := getNodeUtilization(k8sClient, n.Name, gc)
		if err != nil {
			log.Fatalf("Unable to retrieve Node metrics: %v", err)
		}

		nm[n.Name].usageCPU = nodeMetrics[0].Usage.Cpu()
		nm[n.Name].usageMemory = nodeMetrics[0].Usage.Memory()

		nodeMetrics[0].Usage.Cpu().Add(*c.totalUsageCPU)
		nodeMetrics[0].Usage.Memory().Add(*c.totalUsageMemory)
	}

	render(&nm, c)

	return nil
}

func render(nm *nodeMap, c *counter) {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{
		"Node Name", "Status", "ProviderID", "Allocatable\nCPU", "Allocatable\nMEM (Mi)",
		"Allocated\nCPU Req", "Allocated\nCPU Lim", "Allocated\nMEM Req", "Allocated\nMEM Lim", "Usage\nCPU", "Usage\nMem",
	})

	for k, v := range *nm {
		t.AppendRow([]interface{}{k, v.status, v.providerID,
			v.allocatableCPU.AsDec().String(), v.allocatableMemory.String(),
			v.allocatedCPUrequests.AsDec().String(), v.allocatedCPULimits.AsDec().String(),
			v.allocatedMemoryRequests.String(), v.allocatedMemoryLimits.String(),
			v.usageCPU.AsDec().String(),
			v.usageMemory.String()})
	}

	t.AppendFooter(table.Row{
		"Totals", "", "", c.totalAllocatableCPU, c.totalAllocatableMemory,
		c.totalAllocatedCPUrequests, c.totalAllocatedCPULimits, c.totalAllocatedMemoryRequests,
		c.totalAllocatedMemoryLimits, c.totalUsageCPU.String(), c.totalUsageMemory.String(),
	})
	t.SetStyle(table.StyleColoredDark)
	t.Render()
}

func getNodes(clientset *kubernetes.Clientset) (nodes *v1.NodeList, err error) {
	nodes, err = clientset.CoreV1().Nodes().List(
		metav1.ListOptions{LabelSelector: viper.GetString("selector"), FieldSelector: viper.GetString("field-selector")},
	)
	if err != nil {
		return nil, err
	}
	return nodes, err
}

func getPods(clientset *kubernetes.Clientset, nodeName string) (pods *v1.PodList, err error) {
	fieldSelector, err := fields.ParseSelector(
		"spec.nodeName=" + nodeName + ",status.phase!=" + string(v1.PodSucceeded) + ",status.phase!=" + string(v1.PodFailed))
	if err != nil {
		return nil, err
	}

	nodeNonTerminatedPodsList, err := clientset.CoreV1().Pods("").List(metav1.ListOptions{FieldSelector: fieldSelector.String()})
	if err != nil {
		return nil, err
	}
	return nodeNonTerminatedPodsList, nil
}

func describeNodeResource(nodeNonTerminatedPodsList *v1.PodList, node *v1.Node) *NodeStats {
	reqs, limits := getPodsTotalRequestsAndLimits(nodeNonTerminatedPodsList)

	cpuReqs, cpuLimits, memoryReqs, memoryLimits :=
		reqs[v1.ResourceCPU], limits[v1.ResourceCPU], reqs[v1.ResourceMemory], limits[v1.ResourceMemory]

	ns := &NodeStats{
		allocatedCPUrequests:    cpuReqs,
		allocatedCPULimits:      cpuLimits,
		allocatedMemoryRequests: memoryReqs,
		allocatedMemoryLimits:   memoryLimits,
	}
	return ns
}

// Based on: https://github.com/kubernetes/kubernetes/pkg/kubectl/describe/versioned/describe.go#L3223
func getPodsTotalRequestsAndLimits(podList *v1.PodList) (reqs, limits map[v1.ResourceName]resource.Quantity) {
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
func getNodeUtilization(clientset *kubernetes.Clientset, nodeName string, gc *GlanceConfig) (
	[]metrics.NodeMetrics, map[string]v1.ResourceList, error) {
	metricsClientset, err := metricsclientset.NewForConfig(gc.restConfig)
	if err != nil {
		return nil, nil, err
	}

	labelSelector := labels.Everything()
	ls := viper.GetString("selector")
	fs := viper.GetString("field-selector")

	log.Printf(" %+v ", ls+" "+fs)

	if fs != "" || ls != "" {
		labelSelector, err = labels.Parse(ls + " " + fs)
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
		metricsutil.DefaultHeapsterNamespace, metricsutil.DefaultHeapsterScheme,
		metricsutil.DefaultHeapsterService, metricsutil.DefaultHeapsterPort)

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

//nolint interfacer
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
