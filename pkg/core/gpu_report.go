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
	"sort"
	"time"

	v1 "k8s.io/api/core/v1"
)

// GPU node states reported by BuildGPUReport.
const (
	GPUNodeReady    = "Ready"
	GPUNodeCordoned = "Cordoned"
	GPUNodeNotReady = "NotReady"
)

// GPUNodeReport describes accelerator allocation on one node.
type GPUNodeReport struct {
	Name         string         `json:"name"`
	Model        string         `json:"model"`
	State        string         `json:"state"`
	Accelerators AcceleratorMap `json:"accelerators"`
	// GPUPods counts pods on the node that request accelerators.
	GPUPods int `json:"gpuPods"`
	// OtherPods counts pods that request no accelerators, excluding
	// per-node system pods (DaemonSets such as device plugins and exporters,
	// and static pods such as kube-proxy on GKE).
	OtherPods int `json:"otherPods"`
}

// Schedulable reports whether new pods can be placed on the node.
func (n *GPUNodeReport) Schedulable() bool { return n.State == GPUNodeReady }

// GPUPool summarizes one accelerator type for one GPU model.
type GPUPool struct {
	Model    string          `json:"model"`
	Resource v1.ResourceName `json:"resource"`
	Nodes    int             `json:"nodes"`
	// Allocatable and Requests cover all nodes in the pool.
	Allocatable int64 `json:"allocatable"`
	Requests    int64 `json:"requests"`
	// Free counts unrequested devices on schedulable nodes only.
	Free int64 `json:"free"`
	// LargestFreeBlock is the most free devices on any single schedulable
	// node: the largest request that can currently be placed.
	LargestFreeBlock int64 `json:"largestFreeBlock"`
	// NodesWithFree counts schedulable nodes with at least one free device.
	NodesWithFree int `json:"nodesWithFree"`
	// UnschedulableFree counts free devices on cordoned or NotReady nodes.
	UnschedulableFree int64 `json:"unschedulableFree"`
}

// PendingGPUPod is an unscheduled pod that requests accelerators.
type PendingGPUPod struct {
	Namespace string          `json:"namespace"`
	Name      string          `json:"name"`
	Resource  v1.ResourceName `json:"resource"`
	Requested int64           `json:"requested"`
	Age       time.Duration   `json:"age"`
	// Reason and Message come from the PodScheduled condition, when set.
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	// Fits reports whether some schedulable node has enough free devices of
	// the requested type. It ignores taints, affinity, and other resources.
	Fits bool `json:"fits"`
	// LargestFreeBlock is the most free devices of the type on one node.
	LargestFreeBlock int64 `json:"largestFreeBlock"`
	// TotalFree is the number of free devices of the type across schedulable nodes.
	TotalFree int64 `json:"totalFree"`
}

// GPU finding kinds reported in GPUReport.Findings.
const (
	FindingIdleNode       = "idle-gpu-node"
	FindingIdleBusyNode   = "idle-gpus-busy-node"
	FindingFragmentation  = "fragmentation"
	FindingCordonedFree   = "cordoned-free-gpus"
	FindingPendingNoSpace = "pending-no-capacity"
)

// GPUFinding is a notable condition worth acting on.
type GPUFinding struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

// GPUReport is a cluster-wide view of accelerator allocation and waste.
type GPUReport struct {
	Nodes    []GPUNodeReport `json:"nodes"`
	Pools    []GPUPool       `json:"pools"`
	Pending  []PendingGPUPod `json:"pending"`
	Findings []GPUFinding    `json:"findings"`
}

// BuildGPUReport analyzes nodes and pods for accelerator allocation. pods
// should include pending (unscheduled) pods and exclude terminated ones.
// Nodes without accelerators are left out. It makes no API calls.
func BuildGPUReport(nodes []v1.Node, pods []*v1.Pod, now time.Time) GPUReport {
	report := GPUReport{
		Nodes:    []GPUNodeReport{},
		Pools:    []GPUPool{},
		Pending:  []PendingGPUPod{},
		Findings: []GPUFinding{},
	}

	podsByNode := make(map[string][]*v1.Pod)
	var unscheduled []*v1.Pod
	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}
		if pod.Spec.NodeName == "" {
			unscheduled = append(unscheduled, pod)
			continue
		}
		podsByNode[pod.Spec.NodeName] = append(podsByNode[pod.Spec.NodeName], pod)
	}

	for i := range nodes {
		if n, ok := buildGPUNode(&nodes[i], podsByNode[nodes[i].Name]); ok {
			report.Nodes = append(report.Nodes, n)
		}
	}
	sort.Slice(report.Nodes, func(i, j int) bool { return report.Nodes[i].Name < report.Nodes[j].Name })

	report.Pools = buildGPUPools(report.Nodes)
	report.Pending = buildPendingGPUPods(unscheduled, report.Nodes, now)
	report.Findings = buildGPUFindings(&report)
	return report
}

func buildGPUNode(node *v1.Node, pods []*v1.Pod) (GPUNodeReport, bool) {
	acc := AcceleratorMap{}
	for name, q := range node.Status.Allocatable {
		if IsGPUResource(name) && !q.IsZero() {
			acc.get(name).Allocatable = q.Value()
		}
	}
	for name, q := range node.Status.Capacity {
		if IsGPUResource(name) && !q.IsZero() {
			acc.get(name).Capacity = q.Value()
		}
	}
	if len(acc) == 0 {
		return GPUNodeReport{}, false
	}

	model, _ := GPUModelFromLabels(node.Labels)
	if model == "" {
		model = UnknownGPUModel
	}
	n := GPUNodeReport{Name: node.Name, Model: model, State: gpuNodeState(node), Accelerators: acc}

	for _, pod := range pods {
		requestsGPU := false
		for name, q := range PodRequests(pod) {
			if IsGPUResource(name) && !q.IsZero() {
				acc.get(name).Requests += q.Value()
				requestsGPU = true
			}
		}
		for name, q := range PodLimits(pod) {
			if IsGPUResource(name) && !q.IsZero() {
				acc.get(name).Limits += q.Value()
			}
		}
		switch {
		case requestsGPU:
			n.GPUPods++
		case !isPerNodeSystemPod(pod):
			n.OtherPods++
		}
	}
	return n, true
}

func gpuNodeState(node *v1.Node) string {
	ready := false
	for _, c := range node.Status.Conditions {
		if c.Type == v1.NodeReady {
			ready = c.Status == v1.ConditionTrue
			break
		}
	}
	switch {
	case !ready:
		return GPUNodeNotReady
	case node.Spec.Unschedulable:
		return GPUNodeCordoned
	default:
		return GPUNodeReady
	}
}

// mirrorPodAnnotation marks pods the kubelet created from static manifests.
const mirrorPodAnnotation = "kubernetes.io/config.mirror"

// isPerNodeSystemPod reports whether the pod runs on the node regardless of
// workload placement: DaemonSet pods and static (mirror) pods.
func isPerNodeSystemPod(pod *v1.Pod) bool {
	if _, ok := pod.Annotations[mirrorPodAnnotation]; ok {
		return true
	}
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "DaemonSet" || ref.Kind == "Node" {
			return true
		}
	}
	return false
}

func buildGPUPools(nodes []GPUNodeReport) []GPUPool {
	type key struct {
		model    string
		resource v1.ResourceName
	}
	pools := make(map[key]*GPUPool)
	for i := range nodes {
		n := &nodes[i]
		for name, acc := range n.Accelerators {
			k := key{n.Model, name}
			p, ok := pools[k]
			if !ok {
				p = &GPUPool{Model: n.Model, Resource: name}
				pools[k] = p
			}
			p.Nodes++
			p.Allocatable += acc.Allocatable
			p.Requests += acc.Requests
			free := acc.Free()
			if !n.Schedulable() {
				p.UnschedulableFree += free
				continue
			}
			p.Free += free
			if free > 0 {
				p.NodesWithFree++
			}
			if free > p.LargestFreeBlock {
				p.LargestFreeBlock = free
			}
		}
	}

	out := make([]GPUPool, 0, len(pools))
	for _, p := range pools {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].Resource < out[j].Resource
	})
	return out
}

func buildPendingGPUPods(pods []*v1.Pod, nodes []GPUNodeReport, now time.Time) []PendingGPUPod {
	// Free capacity per resource type across schedulable nodes, ignoring
	// model: a pod requesting nvidia.com/gpu can land on any such node
	// unless constrained by selectors this report does not evaluate.
	largest := map[v1.ResourceName]int64{}
	total := map[v1.ResourceName]int64{}
	for i := range nodes {
		if !nodes[i].Schedulable() {
			continue
		}
		for name, acc := range nodes[i].Accelerators {
			free := acc.Free()
			total[name] += free
			if free > largest[name] {
				largest[name] = free
			}
		}
	}

	out := []PendingGPUPod{}
	for _, pod := range pods {
		reqs := PodRequests(pod)
		for _, name := range sortedGPUResources(reqs) {
			q := reqs[name]
			p := PendingGPUPod{
				Namespace:        pod.Namespace,
				Name:             pod.Name,
				Resource:         name,
				Requested:        q.Value(),
				LargestFreeBlock: largest[name],
				TotalFree:        total[name],
				Fits:             largest[name] >= q.Value(),
			}
			if !pod.CreationTimestamp.IsZero() {
				p.Age = now.Sub(pod.CreationTimestamp.Time)
			}
			for _, c := range pod.Status.Conditions {
				if c.Type == v1.PodScheduled && c.Status == v1.ConditionFalse {
					p.Reason, p.Message = c.Reason, c.Message
				}
			}
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Age != out[j].Age {
			return out[i].Age > out[j].Age // longest waiting first
		}
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func sortedGPUResources(list v1.ResourceList) []v1.ResourceName {
	var names []v1.ResourceName
	for name, q := range list {
		if IsGPUResource(name) && !q.IsZero() {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

func buildGPUFindings(r *GPUReport) []GPUFinding {
	findings := []GPUFinding{}

	for i := range r.Nodes {
		n := &r.Nodes[i]
		var alloc, req, free int64
		for _, acc := range n.Accelerators {
			alloc += acc.Allocatable
			req += acc.Requests
			free += acc.Free()
		}
		switch {
		case n.State == GPUNodeReady && alloc > 0 && req == 0 && n.OtherPods > 0:
			findings = append(findings, GPUFinding{
				Kind:    FindingIdleBusyNode,
				Subject: n.Name,
				Message: fmt.Sprintf("%d GPU(s) unallocated while running %d non-GPU pod(s); "+
					"consider tainting GPU nodes so only GPU workloads land there", alloc, n.OtherPods),
			})
		case n.State == GPUNodeReady && alloc > 0 && req == 0:
			findings = append(findings, GPUFinding{
				Kind:    FindingIdleNode,
				Subject: n.Name,
				Message: fmt.Sprintf("%d GPU(s) allocatable and none requested", alloc),
			})
		case n.State != GPUNodeReady && free > 0:
			findings = append(findings, GPUFinding{
				Kind:    FindingCordonedFree,
				Subject: n.Name,
				Message: fmt.Sprintf("%d free GPU(s) unavailable because the node is %s", free, n.State),
			})
		}
	}

	// Pending pods: distinguish "not enough GPUs anywhere" from
	// "enough in total, but split across nodes" (fragmentation).
	for i := range r.Pending {
		p := &r.Pending[i]
		if p.Fits {
			continue
		}
		subject := p.Namespace + "/" + p.Name
		if p.TotalFree >= p.Requested {
			findings = append(findings, GPUFinding{
				Kind:    FindingFragmentation,
				Subject: subject,
				Message: fmt.Sprintf("needs %d %s on one node; %d free in total but at most %d on any node",
					p.Requested, ShortAcceleratorName(p.Resource), p.TotalFree, p.LargestFreeBlock),
			})
			continue
		}
		findings = append(findings, GPUFinding{
			Kind:    FindingPendingNoSpace,
			Subject: subject,
			Message: fmt.Sprintf("needs %d %s; only %d free on schedulable nodes",
				p.Requested, ShortAcceleratorName(p.Resource), p.TotalFree),
		})
	}
	return findings
}
