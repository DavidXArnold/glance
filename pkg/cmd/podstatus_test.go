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
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func running() v1.ContainerStatus {
	return v1.ContainerStatus{Name: "app", Ready: true, State: v1.ContainerState{Running: &v1.ContainerStateRunning{}}}
}

func waiting(reason string) v1.ContainerStatus {
	return v1.ContainerStatus{Name: "app", State: v1.ContainerState{Waiting: &v1.ContainerStateWaiting{Reason: reason}}}
}

func terminated(reason string, exitCode int32) v1.ContainerStatus {
	return v1.ContainerStatus{
		Name:  "app",
		State: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{Reason: reason, ExitCode: exitCode}},
	}
}

func TestPodStatusReason(t *testing.T) {
	now := metav1.Now()
	always := v1.ContainerRestartPolicyAlways
	started := true
	readyCond := []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}}

	tests := []struct {
		name string
		pod  v1.Pod
		want string
	}{
		{
			name: "running",
			pod:  v1.Pod{Status: v1.PodStatus{Phase: v1.PodRunning, ContainerStatuses: []v1.ContainerStatus{running()}}},
			want: "Running",
		},
		{
			name: "crash loop shows waiting reason, not phase",
			pod: v1.Pod{Status: v1.PodStatus{
				Phase: v1.PodRunning, ContainerStatuses: []v1.ContainerStatus{waiting("CrashLoopBackOff")},
			}},
			want: "CrashLoopBackOff",
		},
		{
			name: "image pull failure",
			pod: v1.Pod{Status: v1.PodStatus{
				Phase: v1.PodPending, ContainerStatuses: []v1.ContainerStatus{waiting("ImagePullBackOff")},
			}},
			want: "ImagePullBackOff",
		},
		{
			name: "oom killed",
			pod: v1.Pod{Status: v1.PodStatus{
				Phase: v1.PodRunning, ContainerStatuses: []v1.ContainerStatus{terminated("OOMKilled", 137)},
			}},
			want: "OOMKilled",
		},
		{
			name: "exit code without reason",
			pod: v1.Pod{Status: v1.PodStatus{
				Phase: v1.PodFailed, ContainerStatuses: []v1.ContainerStatus{terminated("", 2)},
			}},
			want: "ExitCode:2",
		},
		{
			name: "terminating",
			pod: v1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now},
				Status:     v1.PodStatus{Phase: v1.PodRunning, ContainerStatuses: []v1.ContainerStatus{running()}},
			},
			want: "Terminating",
		},
		{
			name: "node lost",
			pod: v1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now},
				Status:     v1.PodStatus{Phase: v1.PodRunning, Reason: "NodeLost"},
			},
			want: "Unknown",
		},
		{
			name: "evicted",
			pod:  v1.Pod{Status: v1.PodStatus{Phase: v1.PodFailed, Reason: "Evicted"}},
			want: "Evicted",
		},
		{
			name: "init container crash looping",
			pod: v1.Pod{
				Spec: v1.PodSpec{InitContainers: []v1.Container{{Name: "init"}}},
				Status: v1.PodStatus{
					Phase:                 v1.PodPending,
					InitContainerStatuses: []v1.ContainerStatus{waiting("CrashLoopBackOff")},
				},
			},
			want: "Init:CrashLoopBackOff",
		},
		{
			name: "init container progress",
			pod: v1.Pod{
				Spec: v1.PodSpec{InitContainers: []v1.Container{{Name: "a"}, {Name: "b"}}},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
					InitContainerStatuses: []v1.ContainerStatus{
						terminated("Completed", 0),
						{Name: "b", State: v1.ContainerState{Running: &v1.ContainerStateRunning{}}},
					},
				},
			},
			want: "Init:1/2",
		},
		{
			name: "started sidecar does not block",
			pod: v1.Pod{
				Spec: v1.PodSpec{InitContainers: []v1.Container{{Name: "proxy", RestartPolicy: &always}}},
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
					InitContainerStatuses: []v1.ContainerStatus{{
						Name: "proxy", Started: &started, State: v1.ContainerState{Running: &v1.ContainerStateRunning{}},
					}},
					ContainerStatuses: []v1.ContainerStatus{running()},
				},
			},
			want: "Running",
		},
		{
			name: "completed container beside a running one",
			pod: v1.Pod{Status: v1.PodStatus{
				Phase:             v1.PodRunning,
				Conditions:        readyCond,
				ContainerStatuses: []v1.ContainerStatus{terminated("Completed", 0), running()},
			}},
			want: "Running",
		},
		{
			name: "completed job pod",
			pod: v1.Pod{Status: v1.PodStatus{
				Phase: v1.PodSucceeded, ContainerStatuses: []v1.ContainerStatus{terminated("Completed", 0)},
			}},
			want: "Completed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := podStatusReason(&tt.pod); got != tt.want {
				t.Errorf("podStatusReason() = %q, want %q", got, tt.want)
			}
		})
	}
}
