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

import "k8s.io/apimachinery/pkg/api/resource"

//nolint unused
// NodeStats is an object to hold relevent node stats
type NodeStats struct {
	status                  string
	providerID              string
	allocatableCPU          *resource.Quantity
	allocatableMemory       *resource.Quantity
	capacityCPU             *resource.Quantity
	capacityMemory          *resource.Quantity
	allocatedCPUrequests    resource.Quantity
	allocatedCPULimits      resource.Quantity
	allocatedMemoryRequests resource.Quantity
	allocatedMemoryLimits   resource.Quantity
	usageCPU                *resource.Quantity
	usageMemory             *resource.Quantity
}

type nodeMap map[string]*NodeStats

//nolint unused
type counter struct {
	totalAllocatableCPU          *resource.Quantity
	totalAllocatableMemory       *resource.Quantity
	totalCapacityCPU             *resource.Quantity
	totalCapacityMemory          *resource.Quantity
	totalAllocatedCPUrequests    *resource.Quantity
	totalAllocatedCPULimits      *resource.Quantity
	totalAllocatedMemoryRequests *resource.Quantity
	totalAllocatedMemoryLimits   *resource.Quantity
	totalUsageCPU                *resource.Quantity
	totalUsageMemory             *resource.Quantity
}
