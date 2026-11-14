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
	"fmt"

	v1 "k8s.io/api/core/v1"
)

// Pod status values that are not pod phases.
const (
	podStatusTerminating = "Terminating"
	podStatusCompleted   = "Completed"
	podStatusNotReady    = "NotReady"
	podReasonNodeLost    = "NodeLost"
	podReasonInitPrefix  = "Init:"
)

// podStatusReason returns the pod's effective status the way the STATUS
// column of `kubectl get pods` computes it: Terminating while a deletion is
// pending, a failing init container (Init:CrashLoopBackOff, Init:1/2), the
// waiting or termination reason of a container (CrashLoopBackOff,
// ImagePullBackOff, OOMKilled, Error), and otherwise the pod phase or reason
// (Running, Pending, Evicted).
func podStatusReason(pod *v1.Pod) string {
	reason := string(pod.Status.Phase)
	if pod.Status.Reason != "" {
		reason = pod.Status.Reason
	}

	initializing, initReason := initContainerStatus(pod)
	if initializing {
		reason = initReason
	} else {
		reason = containerStatusReason(pod, reason)
	}

	if pod.DeletionTimestamp != nil {
		if pod.Status.Reason == podReasonNodeLost {
			return string(v1.PodUnknown)
		}
		return podStatusTerminating
	}
	return reason
}

// initContainerStatus reports whether the pod is still initializing and, if
// so, the init container status to show.
func initContainerStatus(pod *v1.Pod) (initializing bool, reason string) {
	sidecars := make(map[string]bool)
	for i := range pod.Spec.InitContainers {
		c := &pod.Spec.InitContainers[i]
		if c.RestartPolicy != nil && *c.RestartPolicy == v1.ContainerRestartPolicyAlways {
			sidecars[c.Name] = true
		}
	}

	total := len(pod.Spec.InitContainers)
	for i := range pod.Status.InitContainerStatuses {
		cs := &pod.Status.InitContainerStatuses[i]
		switch {
		case cs.State.Terminated != nil && cs.State.Terminated.ExitCode == 0:
			continue
		case sidecars[cs.Name] && cs.Started != nil && *cs.Started:
			// A started sidecar does not block initialization.
			continue
		case cs.State.Terminated != nil:
			t := cs.State.Terminated
			switch {
			case t.Reason != "":
				return true, podReasonInitPrefix + t.Reason
			case t.Signal != 0:
				return true, fmt.Sprintf("Init:Signal:%d", t.Signal)
			default:
				return true, fmt.Sprintf("Init:ExitCode:%d", t.ExitCode)
			}
		case cs.State.Waiting != nil && cs.State.Waiting.Reason != "" && cs.State.Waiting.Reason != "PodInitializing":
			return true, podReasonInitPrefix + cs.State.Waiting.Reason
		default:
			return true, fmt.Sprintf("Init:%d/%d", i, total)
		}
	}
	return false, ""
}

// containerStatusReason applies the app containers' waiting and termination
// reasons on top of the phase-derived reason.
func containerStatusReason(pod *v1.Pod, reason string) string {
	hasRunning := false
	// Walk in reverse so the first container's reason wins, as kubectl does.
	for i := len(pod.Status.ContainerStatuses) - 1; i >= 0; i-- {
		cs := &pod.Status.ContainerStatuses[i]
		switch {
		case cs.State.Waiting != nil && cs.State.Waiting.Reason != "":
			reason = cs.State.Waiting.Reason
		case cs.State.Terminated != nil && cs.State.Terminated.Reason != "":
			reason = cs.State.Terminated.Reason
		case cs.State.Terminated != nil && cs.State.Terminated.Signal != 0:
			reason = fmt.Sprintf("Signal:%d", cs.State.Terminated.Signal)
		case cs.State.Terminated != nil:
			reason = fmt.Sprintf("ExitCode:%d", cs.State.Terminated.ExitCode)
		case cs.Ready && cs.State.Running != nil:
			hasRunning = true
		}
	}

	// A completed container next to a running one means the pod is still up.
	if reason == podStatusCompleted && hasRunning {
		if podReady(pod) {
			return string(v1.PodRunning)
		}
		return podStatusNotReady
	}
	return reason
}

// podReady reports whether the pod's Ready condition is true.
func podReady(pod *v1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == v1.PodReady {
			return c.Status == v1.ConditionTrue
		}
	}
	return false
}
