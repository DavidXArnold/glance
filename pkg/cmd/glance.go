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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	core "gitlab.com/davidxarnold/glance/pkg/core"
	glanceutil "gitlab.com/davidxarnold/glance/pkg/util"
	v "gitlab.com/davidxarnold/glance/version"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
	metricsV1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	// needed to authenticate with GCP
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	providerAWS = "aws"
	providerGCE = "gce"
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
	viper.SetDefault("show-node-group", false)
	viper.SetDefault("filter-node-group", "")
	viper.SetDefault("filter-capacity-type", "")

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
func setupGlanceFlags(cmd *cobra.Command, labelSelector, fieldSelector, output *string, cloudInfo *bool) {
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
		cloudInfo, "show-cloud-provider", "c", false,
		"-c, --show-cloud-provider  Display cloud provider metadata (AWS/GCP instance types, regions).\n"+
			"Enabled by default if cloud detected.")

	// Add --raw and --exact flags (aliases)
	var showRaw bool
	var exactValues bool
	cmd.PersistentFlags().BoolVar(&showRaw, "raw", false, "Show raw Kubernetes resource values (e.g., 1500m, 2048Mi)")
	cmd.PersistentFlags().BoolVar(&exactValues, "exact", false, "Alias for --raw")

	cobra.OnInitialize(initConfig)

	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	_ = viper.BindPFlag("selector", cmd.PersistentFlags().Lookup("selector"))
	_ = viper.BindPFlag("output", cmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("show-cloud-provider", cmd.PersistentFlags().Lookup("show-cloud-provider"))
	_ = viper.BindPFlag("cloud-info", cmd.PersistentFlags().Lookup("show-cloud-provider")) // Backwards compatibility alias
	_ = viper.BindPFlag("show-raw", cmd.PersistentFlags().Lookup("raw"))
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
		// pods          bool
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
			// Make --exact and --raw aliases
			showRaw := viper.GetBool("show-raw") || viper.GetBool("exact")
			viper.Set("show-raw", showRaw)
			viper.Set("exact", showRaw)

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

	setupGlanceFlags(cmd, &labelSelector, &fieldSelector, &output, &cloudInfo)

	// Add live subcommand
	cmd.AddCommand(NewLiveCmd(gc))

	return cmd
}

// GlanceK8s displays cluster information for a given clientset
// GlanceK8s performs the core glance operation on a Kubernetes cluster.
// nolint gocyclo
func GlanceK8s(k8sClient *kubernetes.Clientset, gc *GlanceConfig) (err error) {
	ctx := context.Background()

	nodes, err := getNodes(ctx, k8sClient)
	if err != nil {
		log.Fatalf("Error getting Node list from host: %+v ", err.Error())
	}

	if len(nodes.Items) == 0 {
		return fmt.Errorf("no Nodes found")
	}

	k8sver, err := k8sClient.Discovery().ServerVersion()
	if err != nil {
		log.Fatalf(" %+v ", err.Error())
	}

	// Detect if user explicitly set the flag
	userSetCloud := false
	for _, arg := range os.Args {
		if arg == "--show-cloud-provider" || arg == "-c" || arg == "--no-show-cloud-provider" {
			userSetCloud = true
			break
		}
	}

	// Detect provider from nodes and set flag if not user-set
	cloudDetected := false
	for _, node := range nodes.Items {
		if node.Spec.ProviderID != "" {
			cp, _ := glanceutil.ParseProviderID(node.Spec.ProviderID)
			if cp == providerAWS || cp == providerGCE {
				cloudDetected = true
				break
			}
		}
	}
	if !userSetCloud {
		viper.Set("show-cloud-provider", cloudDetected)
	}

	metricsClientset, err := metricsclientset.NewForConfig(gc.restConfig)
	if err != nil {
		return err
	}

	// Build pod and metrics maps using list+group patterns similar to live mode.
	podsByNode, err := buildNonTerminatedPodsByNode(ctx, k8sClient)
	if err != nil {
		return err
	}

	nodeMetricsByName, err := buildNodeMetricsByName(ctx, metricsClientset)
	if err != nil {
		return err
	}

	// Compute core snapshot (NodeMap + Totals) using shared aggregation logic.
	snapshotOpts := core.NodeSnapshotOptions{RequireMetrics: true}
	nm, totals, err := core.ComputeNodeSnapshot(nodes.Items, podsByNode, nodeMetricsByName, snapshotOpts)
	if err != nil {
		return err
	}

	// Set cluster info for display in summary
	totals.ClusterInfo = core.ClusterInfo{
		Host:          gc.restConfig.Host,
		MasterVersion: k8sver.GitVersion,
	}

	// If requested, enrich with pod-level details (reusing existing helper).
	labelSelector := labels.Everything()
	ls := viper.GetString("selector")
	fs := viper.GetString("field-selector")

	if fs != "" || ls != "" {
		labelSelector, err = labels.Parse(ls + " " + fs)
		if err != nil {
			return err
		}
	}

	if viper.GetBool("pods") {
		for _, node := range nodes.Items {
			podList := &v1.PodList{Items: podsByNode[node.Name]}
			if existing, ok := nm[node.Name]; ok {
				existing.PodInfo = getPodsInfo(ctx, podList, metricsClientset, labelSelector)
			}
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

	render(&nm, &totals)

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

// buildNonTerminatedPodsByNode fetches all non-terminated pods once and groups
// them by node name. This mirrors the live path's list+group pattern and
// avoids per-node pod list calls.
func buildNonTerminatedPodsByNode(ctx context.Context, clientset *kubernetes.Clientset) (map[string][]v1.Pod, error) {
	podsByNode := make(map[string][]v1.Pod)

	podList, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "status.phase!=Succeeded,status.phase!=Failed",
	})
	if err != nil {
		return nil, err
	}

	for _, pod := range podList.Items {
		if pod.Spec.NodeName == "" {
			continue
		}
		podsByNode[pod.Spec.NodeName] = append(podsByNode[pod.Spec.NodeName], pod)
	}

	return podsByNode, nil
}

// buildNodeMetricsByName lists node metrics once and returns a map keyed by
// node name. Callers can decide how strictly to enforce metrics presence.
func buildNodeMetricsByName(
	ctx context.Context,
	metricsClient *metricsclientset.Clientset,
) (map[string]*metricsV1beta1api.NodeMetrics, error) {
	metricsList, err := metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{
		ResourceVersion: "0",
	})
	if err != nil {
		return nil, err
	}

	result := make(map[string]*metricsV1beta1api.NodeMetrics, len(metricsList.Items))
	for i := range metricsList.Items {
		m := &metricsList.Items[i]
		result[m.Name] = m
	}

	return result, nil
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
			case providerAWS:
				awsInfo, err := getAWSNodeInfo(id[1])
				if err == nil && awsInfo != nil {
					ns.InstanceType = awsInfo.InstanceType
					ns.NodeGroup = awsInfo.NodeGroup
					ns.FargateProfile = awsInfo.FargateProfile
					ns.CapacityType = awsInfo.CapacityType
				}
			case providerGCE:
				gceInfo, err := getGCENodeInfo(id[1])
				if err == nil && gceInfo != nil {
					ns.InstanceType = gceInfo.InstanceType
					ns.NodePool = gceInfo.NodePool
					ns.CapacityType = gceInfo.CapacityType
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
