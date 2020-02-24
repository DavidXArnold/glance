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

// NodeStats is an object to hold relevent node stats
type NodeStats struct {
	status                  string
	providerID              string
	allocatableCPU          int64
	allocatableMemory       int64
	capacityCPU             int64
	capacityMemory          int64
	allocatedCPUrequests    int64
	allocatedCPULimits      int64
	allocatedMemoryRequests int64
	allocatedMemoryLimits   int64
	usageCPU                string
	usageMemory             string
}

type nodeMap map[string]*NodeStats

type counter struct {
	totalAllocatableCPU    int64
	totalAllocatableMemory int64
	totalCapacityCPU       int64
	totalCapacityMemory    int64
}
