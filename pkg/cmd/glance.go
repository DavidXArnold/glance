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
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	glanceutil "gitlab.com/davidxarnold/glance/pkg/util"
	v "gitlab.com/davidxarnold/glance/version"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/cmd/top"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
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

	// If a config file is found, read it in (ignore errors - config is optional)
	_ = viper.ReadInConfig()

	// Set default values for configuration options
	viper.SetDefault("cloud-cache-ttl", 5*time.Minute)
	viper.SetDefault("cloud-cache-disk", false)
	viper.SetDefault("show-node-version", false)
	viper.SetDefault("show-node-age", false)

	// Configure logging after config is loaded
	configureLogging()
}

// configureLogging sets up logging based on GLANCE_LOG_LEVEL env var or config file
func configureLogging() {
	// Get log level from environment variable or config file
	// Environment variable takes precedence
	logLevel := os.Getenv("GLANCE_LOG_LEVEL")
	if logLevel == "" {
		logLevel = viper.GetString("log-level")
	}
	if logLevel == "" {
		logLevel = "warn" // default to warn - minimal output
	}
	logLevel = strings.ToLower(logLevel)

	// Parse and set log level
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		level = log.WarnLevel
		logLevel = "warn"
	}
	log.SetLevel(level)

	// Only create log file if level is debug, info, or trace (not for warn/error/fatal)
	// Logrus levels: Panic=0, Fatal=1, Error=2, Warn=3, Info=4, Debug=5, Trace=6
	if level >= log.InfoLevel {
		// Determine log directory - prefer ~/.glance/, fall back to /tmp
		logDir := ""
		home, err := homedir.Dir()
		if err == nil {
			glanceDir := filepath.Join(home, ".glance")
			if err := os.MkdirAll(glanceDir, 0750); err == nil {
				logDir = glanceDir
			}
		}
		if logDir == "" {
			logDir = os.TempDir()
		}

		// Create log file: <log-level>-glance.log
		logFileName := fmt.Sprintf("%s-glance.log", logLevel)
		logFilePath := filepath.Clean(filepath.Join(logDir, logFileName))

		// #nosec G304 -- path is constructed from controlled inputs (home dir + fixed filename)
		logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err == nil {
			log.SetOutput(logFile)
			log.SetFormatter(&log.TextFormatter{
				FullTimestamp: true,
			})
		}
	} else {
		// For warn/error/fatal, output to stderr
		log.SetOutput(os.Stderr)
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp: true,
		})
	}
}

// NewGlanceConfig provides an instance of GlanceConfig with default values.
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

// setupGlanceFlags configures persistent flags for the glance command.
func setupGlanceFlags(cmd *cobra.Command, labelSelector, fieldSelector, output *string, cloudInfo, pods *bool) {
	cmd.PersistentFlags().StringVar(
		fieldSelector, "field-selector", "",
		//nolint lll
		"Selector (field query) to filter on, supports '=', '==', and '!='.(e.g. --field-selector key1=value1,key2=value2). The server only supports a limited number of field queries per type.")
	_ = viper.BindPFlag("field-selector", cmd.PersistentFlags().Lookup("field-selector"))
	cmd.PersistentFlags().StringVar(
		labelSelector, "selector", "",
		"Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)")
	cmd.PersistentFlags().StringVarP(
		output, "output", "o", "pretty",
		"Output format. One of: txt|pretty|json|dash|pie|chart")
	cmd.PersistentFlags().BoolVarP(
		cloudInfo, "show-cloud-provider", "c", true,
		"-c, --show-cloud-provider  Display cloud provider metadata (AWS/GCP instance types, regions). Enabled by default.")
	cmd.PersistentFlags().BoolVarP(
		pods, "pods", "p", false,
		"-p, --pods  Display pod resources. true|false")
	cmd.PersistentFlags().Bool(
		"exact", false,
		"Display exact values instead of human-readable format (e.g., 1000m instead of 1)")

	KubernetesConfigFlags.AddFlags(cmd.Flags())
	cobra.OnInitialize(initConfig)

	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	_ = viper.BindPFlag("selector", cmd.PersistentFlags().Lookup("selector"))
	_ = viper.BindPFlag("output", cmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("show-cloud-provider", cmd.PersistentFlags().Lookup("show-cloud-provider"))
	_ = viper.BindPFlag("cloud-info", cmd.PersistentFlags().Lookup("show-cloud-provider")) // Backwards compatibility alias
	_ = viper.BindPFlag("exact", cmd.PersistentFlags().Lookup("exact"))
	_ = viper.BindPFlags(cmd.Flags())
}

// NewGlanceCmd creates and configures the main glance cobra command.
func NewGlanceCmd() *cobra.Command {
	var (
		labelSelector string
		fieldSelector string
		output        string
		cloudInfo     bool
		pods          bool
	)

	KubernetesConfigFlags = genericclioptions.NewConfigFlags(false)

	gc, err := NewGlanceConfig()
	if err != nil {
		log.Fatalf("Unable to create glance configuration: %v", err)
	}

	cmd := &cobra.Command{
		Use:           "glance",
		Short:         "Take a glance at your Kubernetes resources.",
		Long:          "Glance allows you to quickly look at your kubernetes resources.",
		SilenceErrors: true,
		SilenceUsage:  true,
		PreRun: func(cmd *cobra.Command, args []string) {
			err = viper.BindPFlags(cmd.Flags())
			if err != nil {
				log.Fatalf("unable to initialize glance: %v ", err)
			}
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return glanceutil.SetupLogger()
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

	cmd.Version = v.Version

	setupGlanceFlags(cmd, &labelSelector, &fieldSelector, &output, &cloudInfo, &pods)

	// Add live subcommand
	cmd.AddCommand(NewLiveCmd(gc))

	return cmd
}

// GlanceK8s displays cluster information for a given clientset
// GlanceK8s performs the core glance operation on a Kubernetes cluster.
// nolint gocyclo
func GlanceK8s(k8sClient *kubernetes.Clientset, gc *GlanceConfig) (err error) {
	ctx := context.Background()
	c := &Totals{
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

	nm := make(NodeMap)
	k8sver, err := k8sClient.Discovery().ServerVersion()
	if err != nil {
		log.Fatalf(" %+v ", err.Error())
	}

	// Set cluster info for display in summary
	c.ClusterInfo = ClusterInfo{
		Host:          gc.restConfig.Host,
		MasterVersion: k8sver.GitVersion,
	}

	nodes, err := getNodes(ctx, k8sClient)
	if err != nil {
		log.Fatalf("Error getting Node list from host: %+v ", err.Error())
	}

	if len(nodes.Items) == 0 {
		return fmt.Errorf("no Nodes found")
	}

	metricsClientset, err := metricsclientset.NewForConfig(gc.restConfig)
	if err != nil {
		return err
	}

	labelSelector := labels.Everything()
	ls := viper.GetString("selector")
	fs := viper.GetString("field-selector")

	if fs != "" || ls != "" {
		labelSelector, err = labels.Parse(ls + " " + fs)
		if err != nil {
			return err
		}
	}

	for i := range nodes.Items {
		nn := nodes.Items[i].Name
		// Get node condition for ready status
		var readyCondition *v1.NodeCondition
		for j := range nodes.Items[i].Status.Conditions {
			if nodes.Items[i].Status.Conditions[j].Type == v1.NodeReady {
				readyCondition = &nodes.Items[i].Status.Conditions[j]
				break
			}
		}

		if readyCondition == nil || readyCondition.Status != v1.ConditionTrue {
			nm[nn] = &NodeStats{
				Status: "Not Ready",
			}
			continue
		}

		podList, err := getPods(ctx, k8sClient, nn)
		if err != nil {
			log.Fatalf("Error getting Pod list from host: %+v ", err.Error())
		}

		nm[nn] = describeNodeResource(podList)
		nm[nn].Status = "Ready"

		if nodes.Items[i].Spec.ProviderID != "" {
			nm[nn].ProviderID = nodes.Items[i].Spec.ProviderID
		}

		nm[nn].NodeInfo = nodes.Items[i].Status.NodeInfo

		nm[nn].AllocatableCPU = nodes.Items[i].Status.Allocatable.Cpu()
		nm[nn].AllocatableMemory = nodes.Items[i].Status.Allocatable.Memory()

		c.TotalAllocatableCPU.Add(*nm[nn].AllocatableCPU)
		c.TotalAllocatableMemory.Add(*nm[nn].AllocatableMemory)
		c.TotalAllocatedCPUrequests.Add(nm[nn].AllocatedCPUrequests)
		c.TotalAllocatedCPULimits.Add(nm[nn].AllocatedCPULimits)
		c.TotalAllocatedMemoryRequests.Add(nm[nn].AllocatedMemoryRequests)
		c.TotalAllocatedMemoryLimits.Add(nm[nn].AllocatedMemoryLimits)

		nodeMetrics, _, err := getNodeUtilization(ctx, k8sClient, nn, gc)
		if err != nil {
			log.Fatalf("Unable to retrieve Node metrics (metrics-server running?): %v", err)
		}

		nm[nn].UsageCPU = nodeMetrics[0].Usage.Cpu()
		nm[nn].UsageMemory = nodeMetrics[0].Usage.Memory()

		c.TotalUsageCPU.Add(*nm[nn].UsageCPU)
		c.TotalUsageMemory.Add(*nm[nn].UsageMemory)

		if viper.GetBool("pods") {
			nm[nn].PodInfo = getPodsInfo(ctx, podList, metricsClientset, labelSelector)
		}
	}

	// Fetch cloud info asynchronously for all nodes if enabled
	if viper.GetBool("show-cloud-provider") {
		var cloudWg sync.WaitGroup
		for i := range nodes.Items {
			nn := nodes.Items[i].GetName()
			getCloudInfo(ctx, &nodes.Items[i], nm[nn], &cloudWg)
		}
		cloudWg.Wait()
	}

	render(&nm, c)

	return nil
}

func getNodes(ctx context.Context, clientset *kubernetes.Clientset) (nodes *v1.NodeList, err error) {
	nodes, err = clientset.CoreV1().Nodes().List(ctx,
		metav1.ListOptions{LabelSelector: viper.GetString("selector"), FieldSelector: viper.GetString("field-selector")},
	)
	if err != nil {
		return nil, err
	}
	return nodes, err
}

func getPods(ctx context.Context, clientset *kubernetes.Clientset, nodeName string) (pods *v1.PodList, err error) {
	fieldSelector, err := fields.ParseSelector(
		"spec.nodeName=" + nodeName + ",status.phase!=" + string(v1.PodSucceeded) + ",status.phase!=" + string(v1.PodFailed))
	if err != nil {
		return nil, err
	}

	nodeNonTerminatedPodsList, err := clientset.CoreV1().Pods("").List(
		ctx,
		metav1.ListOptions{FieldSelector: fieldSelector.String()},
	)
	if err != nil {
		return nil, err
	}
	return nodeNonTerminatedPodsList, nil
}

func describeNodeResource(nodeNonTerminatedPodsList *v1.PodList) *NodeStats {
	reqs, limits := getPodsTotalRequestsAndLimits(nodeNonTerminatedPodsList)

	cpuReqs, cpuLimits, memoryReqs, memoryLimits :=
		reqs[v1.ResourceCPU], limits[v1.ResourceCPU], reqs[v1.ResourceMemory], limits[v1.ResourceMemory]

	ns := &NodeStats{
		AllocatedCPUrequests:    cpuReqs,
		AllocatedCPULimits:      cpuLimits,
		AllocatedMemoryRequests: memoryReqs,
		AllocatedMemoryLimits:   memoryLimits,
	}
	return ns
}

// Based on: https://github.com/kubernetes/kubernetes/pkg/kubectl/describe/versioned/describe.go#L3223
func getPodsTotalRequestsAndLimits(podList *v1.PodList) (reqs, limits map[v1.ResourceName]resource.Quantity) {
	reqs, limits = map[v1.ResourceName]resource.Quantity{}, map[v1.ResourceName]resource.Quantity{}
	for i := range podList.Items {
		podReqs, podLimits := resourcehelper.PodRequestsAndLimits(&podList.Items[i])
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

func getPodsInfo(
	ctx context.Context,
	podList *v1.PodList,
	metricsClient metricsclientset.Interface,
	selector labels.Selector,
) map[string]*PodInfo {
	podMap := make(map[string]*PodInfo)
	for i := range podList.Items {
		n := podList.Items[i].Name
		_, _ = getPodMetricsFromMetricsAPI(ctx, metricsClient, n, getNamespace(), selector)
	}
	return podMap
}

func getLabelSelector() (labels.Selector, error) {
	labelSelector := labels.Everything()
	ls := viper.GetString("selector")
	fs := viper.GetString("field-selector")

	if fs != "" || ls != "" {
		var err error
		labelSelector, err = labels.Parse(ls + " " + fs)
		if err != nil {
			return nil, err
		}
	}
	return labelSelector, nil
}

// reimplementation of https://github.com/kubernetes/kubernetes/blob/master/pkg/kubectl/cmd/top/top_node.go#L159
func getNodeUtilization(ctx context.Context, clientset *kubernetes.Clientset, nodeName string, gc *GlanceConfig) (
	[]metricsapi.NodeMetrics, map[string]v1.ResourceList, error) {
	metricsClientset, err := metricsclientset.NewForConfig(gc.restConfig)
	if err != nil {
		return nil, nil, err
	}

	labelSelector, err := getLabelSelector()
	if err != nil {
		return nil, nil, err
	}

	apiGroups, err := clientset.Discovery().ServerGroups()
	if err != nil {
		return nil, nil, err
	}
	metricsAPIAvailable := top.SupportedMetricsAPIVersionAvailable(apiGroups)

	// Note: Heapster is deprecated and removed from Kubernetes
	// The metrics-server is now the standard metrics provider
	//nolint staticcheck
	metrics := &metricsapi.NodeMetricsList{}
	if metricsAPIAvailable {
		metrics, err = getNodeMetricsFromMetricsAPI(ctx, metricsClientset, nodeName, labelSelector)
		if err != nil {
			return nil, nil, err
		}
	} else {
		// If metrics-server is not available, return error
		return nil, nil, errors.New("metrics API not available - ensure metrics-server is installed")
	}
	if len(metrics.Items) == 0 {
		return nil, nil, errors.New("metrics not available yet")
	}
	var nodes []v1.Node
	if len(nodeName) > 0 {
		node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			return nil, nil, err
		}
		nodes = append(nodes, *node)
	} else {
		nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector.String(),
		})
		if err != nil {
			return nil, nil, err
		}
		nodes = append(nodes, nodeList.Items...)
	}

	allocatable := make(map[string]v1.ResourceList)

	for i := range nodes {
		allocatable[nodes[i].Name] = nodes[i].Status.Allocatable
	}

	return metrics.Items, allocatable, nil
}

// nolint interfacer
func getNodeMetricsFromMetricsAPI(ctx context.Context, metricsClient metricsclientset.Interface, resourceName string, selector labels.Selector) (*metricsapi.NodeMetricsList, error) {
	var err error
	versionedMetrics := &metricsV1beta1api.NodeMetricsList{}
	mc := metricsClient.MetricsV1beta1()
	nm := mc.NodeMetricses()
	if resourceName != "" {
		m, err := nm.Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		versionedMetrics.Items = []metricsV1beta1api.NodeMetrics{*m}
	} else {
		versionedMetrics, err = nm.List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
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

// nolint interfacer
func getPodMetricsFromMetricsAPI(ctx context.Context, metricsClient metricsclientset.Interface, resourceName string, namespace string, selector labels.Selector) (*metricsapi.PodMetricsList, error) {
	var err error
	versionedMetrics := &metricsV1beta1api.PodMetricsList{}
	mc := metricsClient.MetricsV1beta1()
	pm := mc.PodMetricses(namespace)
	if resourceName != "" {
		m, err := pm.Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		versionedMetrics.Items = []metricsV1beta1api.PodMetrics{*m}
	} else {
		versionedMetrics, err = pm.List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
		if err != nil {
			return nil, err
		}
	}
	metrics := &metricsapi.PodMetricsList{}
	err = metricsV1beta1api.Convert_v1beta1_PodMetricsList_To_metrics_PodMetricsList(versionedMetrics, metrics, nil)
	if err != nil {
		return nil, err
	}
	return metrics, nil
}

// nolint:unparam // ctx parameter reserved for future use in cloud provider API calls
func getCloudInfo(ctx context.Context, n *v1.Node, ns *NodeStats, wg *sync.WaitGroup) {
	if n.Spec.ProviderID == "" {
		log.Debugf("unable to get cloud-info for node: %v providerID not set", n.GetName())
		return
	}

	ns.ProviderID = n.Spec.ProviderID

	// Get region from node labels (available immediately)
	if region, ok := n.Labels["topology.kubernetes.io/region"]; ok {
		ns.Region = region
	}

	// Parse provider type
	cp, id := glanceutil.ParseProviderID(ns.ProviderID)

	// Fetch detailed cloud info asynchronously
	if wg != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch cp {
			case "aws":
				instanceType, err := getAWSNodeInfo(id[1])
				if err == nil {
					ns.InstanceType = instanceType
				}
			case "gce":
				instanceType, err := getGCENodeInfo(id[1])
				if err == nil {
					ns.InstanceType = instanceType
				}
			case "azure":
				// Azure support not yet implemented
				log.Debugf("Azure cloud info not yet implemented")
			default:
				log.Debugf("Unknown cloud provider: %v", cp)
			}
		}()
	}
}

func getNamespace() (ns string) {
	ns = viper.GetString("namespace")
	if ns == "" {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules,
			configOverrides)

		ns, _, err := kubeConfig.Namespace()
		if err != nil {
			log.Fatalf("Unable to determine namespace: %v", err)
		}
		return ns
	}
	return
}
