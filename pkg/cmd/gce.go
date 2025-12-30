/*
Copyright 2025 David Arnold
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
	"strings"

	log "github.com/sirupsen/logrus"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

// GCENodeMetadata holds GCP-specific node information.
type GCENodeMetadata struct {
	InstanceType string
	NodePool     string // GKE node pool name
	CapacityType string // STANDARD, SPOT
}

func getGCENodeInfo(id string) (*GCENodeMetadata, error) {
	// Parse the GCE provider ID to get project, zone, instance name
	// Format: project-id/zone/instance-name
	parts := strings.Split(id, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid GCE provider ID format")
	}

	projectID := parts[0]
	zone := parts[1]
	instanceName := parts[2]

	ctx := context.Background()
	c, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		log.Debugf("failed to create GCE client: %v", err)
		return nil, err
	}
	defer func() {
		if err := c.Close(); err != nil {
			log.Debugf("failed to close GCE client: %v", err)
		}
	}()

	req := &computepb.GetInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	instance, err := c.Get(ctx, req)
	if err != nil {
		log.Debugf("failed to get GCE instance: %v", err)
		return nil, err
	}

	metadata := &GCENodeMetadata{
		CapacityType: "STANDARD", // Default
	}

	if instance.MachineType != nil {
		// Extract just the machine type name from the full URL
		parts := strings.Split(*instance.MachineType, "/")
		metadata.InstanceType = parts[len(parts)-1]
	}

	// Extract node pool from metadata/labels
	if instance.Metadata != nil && instance.Metadata.Items != nil {
		for _, item := range instance.Metadata.Items {
			if item.Key != nil && *item.Key == "gke-nodepool" && item.Value != nil {
				metadata.NodePool = *item.Value
			}
		}
	}

	// Check labels for node pool (alternative location)
	if metadata.NodePool == "" && instance.Labels != nil {
		if poolName, ok := instance.Labels["gke-nodepool"]; ok {
			metadata.NodePool = poolName
		}
	}

	// Check for spot instances
	if instance.Scheduling != nil && instance.Scheduling.ProvisioningModel != nil {
		if *instance.Scheduling.ProvisioningModel == "SPOT" {
			metadata.CapacityType = "SPOT"
		}
	}

	return metadata, nil
}
