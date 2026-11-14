/*
Copyright 2026 David Arnold
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

package core

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	resourcehelper "k8s.io/component-helpers/resource"
)

// Well-known accelerator extended resource names advertised by device plugins.
const (
	ResourceNvidiaGPU       v1.ResourceName = "nvidia.com/gpu"
	ResourceNvidiaGPUShared v1.ResourceName = "nvidia.com/gpu.shared" // time-slicing / MPS with renameByDefault
	ResourceAMDGPU          v1.ResourceName = "amd.com/gpu"
	ResourceIntelGPUi915    v1.ResourceName = "gpu.intel.com/i915"
	ResourceIntelGPUXe      v1.ResourceName = "gpu.intel.com/xe"
	ResourceAWSNeuron       v1.ResourceName = "aws.amazon.com/neuron"
	ResourceAWSNeuronDevice v1.ResourceName = "aws.amazon.com/neurondevice"
	ResourceAWSNeuronCore   v1.ResourceName = "aws.amazon.com/neuroncore"
	ResourceHabanaGaudi     v1.ResourceName = "habana.ai/gaudi"
	ResourceGoogleTPU       v1.ResourceName = "google.com/tpu"

	// nvidiaMIGPrefix matches MIG slices exposed with the "mixed" strategy,
	// e.g. nvidia.com/mig-1g.5gb or nvidia.com/mig-3g.20gb.
	nvidiaMIGPrefix = "nvidia.com/mig-"
)

var knownAccelerators = map[v1.ResourceName]bool{
	ResourceNvidiaGPU:       true,
	ResourceNvidiaGPUShared: true,
	ResourceAMDGPU:          true,
	ResourceIntelGPUi915:    true,
	ResourceIntelGPUXe:      true,
	ResourceAWSNeuron:       true,
	ResourceAWSNeuronDevice: true,
	ResourceAWSNeuronCore:   true,
	ResourceHabanaGaudi:     true,
	ResourceGoogleTPU:       true,
}

var (
	extraAcceleratorMu       sync.RWMutex
	extraAcceleratorPatterns []string
)

// SetExtraGPUResources registers additional resource names or path.Match
// glob patterns (e.g. "example.com/*") to treat as accelerators, for device
// plugins glance does not know about. It replaces any previously set patterns.
func SetExtraGPUResources(patterns []string) error {
	cleaned := make([]string, 0, len(patterns))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("invalid gpu resource pattern %q: %w", p, err)
		}
		cleaned = append(cleaned, p)
	}

	extraAcceleratorMu.Lock()
	extraAcceleratorPatterns = cleaned
	extraAcceleratorMu.Unlock()
	return nil
}

// IsGPUResource reports whether the resource name is a GPU or other
// accelerator (MIG slices, shared GPUs, Neuron, Gaudi, TPU, ...). Besides the
// well-known names it matches any resource ending in "/gpu" and any patterns
// registered with SetExtraGPUResources.
func IsGPUResource(name v1.ResourceName) bool {
	if knownAccelerators[name] {
		return true
	}
	s := string(name)
	if strings.HasPrefix(s, nvidiaMIGPrefix) || strings.HasSuffix(s, "/gpu") {
		return true
	}

	extraAcceleratorMu.RLock()
	defer extraAcceleratorMu.RUnlock()
	for _, p := range extraAcceleratorPatterns {
		if ok, _ := path.Match(p, s); ok {
			return true
		}
	}
	return false
}

// AcceleratorStats counts one accelerator resource type on a node or across
// the cluster. Accelerators are whole devices (or slices), so plain integers
// are used rather than resource.Quantity.
type AcceleratorStats struct {
	Allocatable int64 `json:"allocatable"`
	Capacity    int64 `json:"capacity"`
	Requests    int64 `json:"requests"`
	Limits      int64 `json:"limits"`
}

// Free returns the number of allocatable devices not requested by pods.
func (s *AcceleratorStats) Free() int64 {
	if free := s.Allocatable - s.Requests; free > 0 {
		return free
	}
	return 0
}

func (s *AcceleratorStats) add(o *AcceleratorStats) {
	s.Allocatable += o.Allocatable
	s.Capacity += o.Capacity
	s.Requests += o.Requests
	s.Limits += o.Limits
}

// AcceleratorMap holds per-resource accelerator counts, keyed by resource name.
type AcceleratorMap map[v1.ResourceName]*AcceleratorStats

func (m AcceleratorMap) get(name v1.ResourceName) *AcceleratorStats {
	s, ok := m[name]
	if !ok {
		s = &AcceleratorStats{}
		m[name] = s
	}
	return s
}

// Names returns the resource names in the map, sorted.
func (m AcceleratorMap) Names() []v1.ResourceName {
	names := make([]v1.ResourceName, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

// ShortAcceleratorName trims the vendor domain from a resource name for
// compact display, e.g. "nvidia.com/mig-1g.5gb" -> "mig-1g.5gb".
func ShortAcceleratorName(name v1.ResourceName) string {
	s := string(name)
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// PodRequests returns the effective resource requests of a pod as the
// scheduler computes them: the larger of the sum of app containers (plus
// restartable sidecar init containers) and the largest init container, plus
// pod overhead, honoring pod-level resources when set.
func PodRequests(pod *v1.Pod) v1.ResourceList {
	return resourcehelper.PodRequests(pod, resourcehelper.PodResourcesOptions{})
}

// PodLimits returns the effective resource limits of a pod, computed the
// same way as PodRequests.
func PodLimits(pod *v1.Pod) v1.ResourceList {
	return resourcehelper.PodLimits(pod, resourcehelper.PodResourcesOptions{})
}

// SumAccelerators returns the total accelerator count across all accelerator
// resource types in list.
func SumAccelerators(list v1.ResourceList) *resource.Quantity {
	total := resource.NewQuantity(0, resource.DecimalSI)
	for name, qty := range list {
		if IsGPUResource(name) {
			total.Add(qty)
		}
	}
	return total
}

// Node labels that describe the GPU model, in order of preference.
const (
	LabelNvidiaGPUProduct  = "nvidia.com/gpu.product"              // NVIDIA GPU Feature Discovery
	LabelNvidiaGPUMemory   = "nvidia.com/gpu.memory"               // MiB, NVIDIA GPU Feature Discovery
	LabelGKEAccelerator    = "cloud.google.com/gke-accelerator"    // GKE
	LabelKarpenterGPUName  = "karpenter.k8s.aws/instance-gpu-name" // Karpenter (AWS)
	LabelKarpenterGPUMaker = "karpenter.k8s.aws/instance-gpu-manufacturer"
	LabelKarpenterGPUMem   = "karpenter.k8s.aws/instance-gpu-memory" // MiB
)

// GPUModelFromLabels returns a human-readable GPU model and per-device memory
// in MiB (0 when unknown) from well-known node labels. It returns an empty
// model when no label identifies the GPU.
func GPUModelFromLabels(labels map[string]string) (model string, memoryMiB int64) {
	switch {
	case labels[LabelNvidiaGPUProduct] != "":
		model = labels[LabelNvidiaGPUProduct]
		memoryMiB = parseMiB(labels[LabelNvidiaGPUMemory])
	case labels[LabelGKEAccelerator] != "":
		model = labels[LabelGKEAccelerator]
	case labels[LabelKarpenterGPUName] != "":
		model = labels[LabelKarpenterGPUName]
		if maker := labels[LabelKarpenterGPUMaker]; maker != "" {
			model = maker + "-" + model
		}
		memoryMiB = parseMiB(labels[LabelKarpenterGPUMem])
	}
	return model, memoryMiB
}

func parseMiB(s string) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// UnknownGPUModel is used to group accelerators on nodes without model labels.
const UnknownGPUModel = "unknown"

// GPUGroup aggregates one accelerator resource type for one GPU model across
// Ready nodes, e.g. all nvidia.com/gpu devices on NVIDIA-A100-SXM4-80GB nodes.
type GPUGroup struct {
	Model    string          `json:"model"`
	Resource v1.ResourceName `json:"resource"`
	Nodes    int             `json:"nodes"`
	AcceleratorStats
}

// buildGPUGroups groups per-node accelerator stats by GPU model and resource,
// sorted by model then resource.
func buildGPUGroups(nm NodeMap) []GPUGroup {
	type key struct {
		model    string
		resource v1.ResourceName
	}
	groups := make(map[key]*GPUGroup)
	for _, stats := range nm {
		if stats == nil || len(stats.Accelerators) == 0 {
			continue
		}
		model := stats.GPUModel
		if model == "" {
			model = UnknownGPUModel
		}
		for name, acc := range stats.Accelerators {
			if acc.Allocatable == 0 && acc.Capacity == 0 {
				continue
			}
			k := key{model: model, resource: name}
			g, ok := groups[k]
			if !ok {
				g = &GPUGroup{Model: model, Resource: name}
				groups[k] = g
			}
			g.Nodes++
			g.add(acc)
		}
	}

	out := make([]GPUGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].Resource < out[j].Resource
	})
	return out
}
