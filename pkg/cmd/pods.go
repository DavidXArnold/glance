package cmd

import (
	"context"
	"fmt"
	"sort"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

// NewPodsCmd creates the static "glance pods" subcommand.
func NewPodsCmd(gc *GlanceConfig) *cobra.Command {
	var allNamespaces bool
	cmd := &cobra.Command{
		Use:   "pods",
		Short: "Show pod-level resource usage (static view)",
		Long: `Display a static snapshot of pod-level resource requests, limits, and usage.

This mirrors the live Pods view but runs once and exits, suitable for scripting.
Uses the kubeconfig context's namespace; pass -n to pick one or -A for all namespaces.
Respects --selector, --field-selector, and --output.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve REST config to respect kube flags (context, namespace, etc.).
			rc, err := gc.configFlags.ToRESTConfig()
			if err != nil {
				return fmt.Errorf("failed to get kubernetes config: %w", err)
			}
			gc.restConfig = rc

			k8sClient, err := kubernetes.NewForConfig(gc.restConfig)
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			metricsClient, err := metricsclientset.NewForConfig(gc.restConfig)
			if err != nil {
				log.Debugf("Failed to create metrics client for pods: %v", err)
				metricsClient = nil
			}

			selector, fieldSelector, err := getSelectors()
			if err != nil {
				return err
			}

			namespace := resolveNamespace(gc, allNamespaces)

			ctx := context.Background()
			rows, err := CollectPodStats(ctx, k8sClient, metricsClient, namespace, selector, fieldSelector)
			if err != nil {
				return fmt.Errorf("failed to collect pod stats: %w", err)
			}

			// Sort by namespace, then name for stable output.
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].Namespace == rows[j].Namespace {
					return rows[i].Name < rows[j].Name
				}
				return rows[i].Namespace < rows[j].Namespace
			})

			return renderPodsStatic(rows)
		},
	}

	addAllNamespacesFlag(cmd, &allNamespaces)
	return cmd
}

// NewDeploymentsCmd creates the static "glance deployments" subcommand.
func NewDeploymentsCmd(gc *GlanceConfig) *cobra.Command {
	var allNamespaces bool
	cmd := &cobra.Command{
		Use:   "deployments",
		Short: "Show deployment-level resource usage (static view)",
		Long: `Display a static snapshot of deployment-level resource requests, limits, and replica status.

This mirrors the live Deployments view but runs once and exits.
Uses the kubeconfig context's namespace; pass -n to pick one or -A for all namespaces.
Respects --selector, --field-selector, and --output.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve REST config to respect kube flags (context, namespace, etc.).
			rc, err := gc.configFlags.ToRESTConfig()
			if err != nil {
				return fmt.Errorf("failed to get kubernetes config: %w", err)
			}
			gc.restConfig = rc

			k8sClient, err := kubernetes.NewForConfig(gc.restConfig)
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			selector, fieldSelector, err := getSelectors()
			if err != nil {
				return err
			}

			namespace := resolveNamespace(gc, allNamespaces)

			ctx := context.Background()
			rows, err := CollectDeploymentStats(ctx, k8sClient, namespace, selector, fieldSelector)
			if err != nil {
				return fmt.Errorf("failed to collect deployment stats: %w", err)
			}

			// Sort by namespace, then name for stable output.
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].Namespace == rows[j].Namespace {
					return rows[i].Name < rows[j].Name
				}
				return rows[i].Namespace < rows[j].Namespace
			})

			return renderDeploymentsStatic(rows)
		},
	}

	addAllNamespacesFlag(cmd, &allNamespaces)
	return cmd
}

// addAllNamespacesFlag adds kubectl's -A/--all-namespaces flag.
func addAllNamespacesFlag(cmd *cobra.Command, allNamespaces *bool) {
	cmd.Flags().BoolVarP(allNamespaces, "all-namespaces", "A", false,
		"List across all namespaces, overriding the kubeconfig context's namespace and -n")
}

// resolveNamespace returns the namespace a static view should query: all
// namespaces ("") with -A, otherwise -n or the kubeconfig context's
// namespace, which falls back to "default".
func resolveNamespace(gc *GlanceConfig, allNamespaces bool) string {
	if allNamespaces {
		return metav1.NamespaceAll
	}
	namespace, _, err := gc.configFlags.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		log.Debugf("Failed to determine namespace, using %q: %v", metav1.NamespaceDefault, err)
		return metav1.NamespaceDefault
	}
	return namespace
}
