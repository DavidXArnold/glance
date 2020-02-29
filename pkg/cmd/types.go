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

//nolint unused
// NodeStats is an object to hold relevent node stats
type NodeStats struct {
	status                  string
	providerID              string
	allocatableCPU          float64
	allocatableMemory       int64
	capacityCPU             float64
	capacityMemory          int64
	allocatedCPUrequests    float64
	allocatedCPULimits      float64
	allocatedMemoryRequests int64
	allocatedMemoryLimits   int64
	usageCPU                string
	usageMemory             string
}

type nodeMap map[string]*NodeStats

//nolint unused
type counter struct {
	totalAllocatableCPU          float64
	totalAllocatableMemory       int64
	totalCapacityCPU             float64
	totalCapacityMemory          int64
	totalAllocatedCPUrequests    float64
	totalAllocatedCPULimits      float64
	totalAllocatedMemoryRequests int64
	totalAllocatedMemoryLimits   int64
	totalUsageCPU                float64
	totalUsageMemory             int64
}
