package cmd

import (
	"context"
	"fmt"
	"sort"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

// NewPodsCmd creates the static "glance pods" subcommand.
func NewPodsCmd(gc *GlanceConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pods",
		Short: "Show pod-level resource usage (static view)",
		Long: `Display a static snapshot of pod-level resource requests, limits, and usage.

This mirrors the live Pods view but runs once and exits, suitable for scripting.
Respects --namespace/-n, --selector, --field-selector, and --output.`,
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

			selector, err := getLabelSelector()
			if err != nil {
				return fmt.Errorf("invalid label/field selector: %w", err)
			}

			// Determine namespace from kubeconfig/flags; empty means all namespaces.
			namespace, _, err := gc.configFlags.ToRawKubeConfigLoader().Namespace()
			if err != nil {
				log.Debugf("Failed to determine namespace for pods: %v", err)
				namespace = ""
			}

			ctx := context.Background()
			rows, err := CollectPodStats(ctx, k8sClient, metricsClient, namespace, selector)
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

	return cmd
}

// NewDeploymentsCmd creates the static "glance deployments" subcommand.
func NewDeploymentsCmd(gc *GlanceConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployments",
		Short: "Show deployment-level resource usage (static view)",
		Long: `Display a static snapshot of deployment-level resource requests, limits, and replica status.

This mirrors the live Deployments view but runs once and exits.
Respects --namespace/-n, --selector, --field-selector, and --output.`,
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

			selector, err := getLabelSelector()
			if err != nil {
				return fmt.Errorf("invalid label/field selector: %w", err)
			}

			// Determine namespace from kubeconfig/flags; empty means all namespaces.
			namespace, _, err := gc.configFlags.ToRawKubeConfigLoader().Namespace()
			if err != nil {
				log.Debugf("Failed to determine namespace for deployments: %v", err)
				namespace = ""
			}

			ctx := context.Background()
			rows, err := CollectDeploymentStats(ctx, k8sClient, namespace, selector)
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

	return cmd
}
