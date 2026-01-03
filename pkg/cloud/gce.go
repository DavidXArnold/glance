package cloud

import (
	"context"
	"fmt"
	"strings"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

// gceProvider implements Provider for GCE-backed nodes.
type gceProvider struct{}

// NodeMetadata fetches GCE-specific node information and maps it to Metadata.
// The id format is expected to be "project/zone/instance-name".
func (p *gceProvider) NodeMetadata(ctx context.Context, id string) (*Metadata, error) {
	parts := strings.Split(id, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid GCE provider ID format: %q", id)
	}

	projectID := parts[0]
	zone := parts[1]
	instanceName := parts[2]

	c, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCE client: %w", err)
	}
	defer func() {
		// Best-effort close; ignore error to satisfy staticcheck/errcheck.
		_ = c.Close()
	}()

	req := &computepb.GetInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	instance, err := c.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get GCE instance: %w", err)
	}

	metadata := &Metadata{
		CapacityType: "STANDARD",
	}

	if instance.MachineType != nil {
		// Extract just the machine type name from the full URL.
		parts := strings.Split(*instance.MachineType, "/")
		metadata.InstanceType = parts[len(parts)-1]
	}

	// Extract node pool from metadata/labels.
	if instance.Metadata != nil && instance.Metadata.Items != nil {
		for _, item := range instance.Metadata.Items {
			if item.Key != nil && *item.Key == "gke-nodepool" && item.Value != nil {
				metadata.NodePool = *item.Value
			}
		}
	}

	// Alternative location for node pool.
	if metadata.NodePool == "" && instance.Labels != nil {
		if poolName, ok := instance.Labels["gke-nodepool"]; ok {
			metadata.NodePool = poolName
		}
	}

	// Check for spot instances.
	if instance.Scheduling != nil && instance.Scheduling.ProvisioningModel != nil {
		if *instance.Scheduling.ProvisioningModel == "SPOT" {
			metadata.CapacityType = capacityTypeSpot
		}
	}

	return metadata, nil
}

// nolint:gochecknoinits // registration-style init keeps provider wiring local to this file.
func init() {
	RegisterProvider(ProviderGCE, func() Provider { return &gceProvider{} })
}
