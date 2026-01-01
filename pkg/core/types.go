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

// Package core contains domain types and (eventually) aggregation logic
// for glance that are independent of any particular CLI or UI.
package core

import (
	"time"

	containerpb "cloud.google.com/go/container/apiv1/containerpb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// NodeStats holds relevant node statistics including resource allocation and usage.
type NodeStats struct {
	Status                  string              `json:",omitempty"`
	ProviderID              string              `json:",omitempty"`
	Region                  string              `json:",omitempty"`
	InstanceType            string              `json:",omitempty"`
	NodeGroup               string              `json:",omitempty"` // AWS EKS node group
	NodePool                string              `json:",omitempty"` // GCP GKE node pool
	FargateProfile          string              `json:",omitempty"` // AWS Fargate profile
	CapacityType            string              `json:",omitempty"` // ON_DEMAND, SPOT, etc.
	NodeInfo                v1.NodeSystemInfo   `json:",omitempty"`
	CloudInfo               cloudInfo           `json:",omitempty"`
	AllocatableCPU          *resource.Quantity  `json:",omitempty"`
	AllocatableMemory       *resource.Quantity  `json:",omitempty"`
	CapacityCPU             *resource.Quantity  `json:",omitempty"`
	CapacityMemory          *resource.Quantity  `json:",omitempty"`
	AllocatedCPUrequests    resource.Quantity   `json:",omitempty"`
	AllocatedCPULimits      resource.Quantity   `json:",omitempty"`
	AllocatedMemoryRequests resource.Quantity   `json:",omitempty"`
	AllocatedMemoryLimits   resource.Quantity   `json:",omitempty"`
	UsageCPU                *resource.Quantity  `json:",omitempty"`
	UsageMemory             *resource.Quantity  `json:",omitempty"`
	PodInfo                 map[string]*PodInfo `json:",omitempty"`
	CreationTime            time.Time           `json:",omitempty"`
	PodCount                int                 `json:",omitempty"`
}

// NodeMap is a map of node names to their statistics.
type NodeMap map[string]*NodeStats

// PodInfo holds pod-level resource information including QoS and usage.
type PodInfo struct {
	Qos         *v1.PodQOSClass    `json:",omitempty"`
	PodReqs     *v1.ResourceList   `json:",omitempty"`
	PodLimits   *v1.ResourceList   `json:",omitempty"`
	UsageCPU    *resource.Quantity `json:",omitempty"`
	UsageMemory *resource.Quantity `json:",omitempty"`
}

// cloudInfo holds cloud provider specific information for nodes.
type cloudInfo struct {
	Aws   *ec2.DescribeInstancesOutput `json:",omitempty"`
	Gce   *containerpb.NodePool        `json:",omitempty"`
	Azure map[string]string            `json:",omitempty"`
}

// ClusterInfo holds metadata about the cluster.
type ClusterInfo struct {
	Host          string `json:",omitempty"`
	MasterVersion string `json:",omitempty"`
}

// Totals holds aggregate resource statistics across the entire cluster.
type Totals struct {
	ClusterInfo                  ClusterInfo        `json:",omitempty"`
	TotalAllocatableCPU          *resource.Quantity `json:",omitempty"`
	TotalAllocatableMemory       *resource.Quantity `json:",omitempty"`
	TotalCapacityCPU             *resource.Quantity `json:",omitempty"`
	TotalCapacityMemory          *resource.Quantity `json:",omitempty"`
	TotalAllocatedCPUrequests    *resource.Quantity `json:",omitempty"`
	TotalAllocatedCPULimits      *resource.Quantity `json:",omitempty"`
	TotalAllocatedMemoryRequests *resource.Quantity `json:",omitempty"`
	TotalAllocatedMemoryLimits   *resource.Quantity `json:",omitempty"`
	TotalUsageCPU                *resource.Quantity `json:",omitempty"`
	TotalUsageMemory             *resource.Quantity `json:",omitempty"`
}

// Glance holds the complete cluster state including per-node statistics and totals.
type Glance struct {
	Nodes  NodeMap
	Totals Totals
}

// Snapshot represents a complete, immutable view of cluster state at a point in time.
// It is currently an alias of Glance but may diverge as the core API evolves.
type Snapshot = Glance

// NewSnapshot constructs a Snapshot from the provided node map and totals.
// It performs no aggregation; callers are responsible for populating the inputs.
func NewSnapshot(nodes NodeMap, totals Totals) Snapshot {
	return Snapshot{
		Nodes:  nodes,
		Totals: totals,
	}
}
