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
	core "gitlab.com/davidxarnold/glance/pkg/core"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

// GlanceConfig contains options and configurations needed by glance.
//
// NOTE: Core domain types such as NodeStats, Totals, and Glance have been
// moved to pkg/core. To preserve compatibility inside this package and for
// external callers importing pkg/cmd, we re-export them as type aliases
// below.
type GlanceConfig struct {
	configFlags *genericclioptions.ConfigFlags
	restConfig  *rest.Config
	genericclioptions.IOStreams
}

// Re-export core domain types so existing code referring to cmd.NodeStats,
// cmd.Totals, etc. continues to compile while the canonical definitions live
// in pkg/core.

type (
	NodeStats   = core.NodeStats
	NodeMap     = core.NodeMap
	PodInfo     = core.PodInfo
	ClusterInfo = core.ClusterInfo
	Totals      = core.Totals
	Glance      = core.Glance
)
