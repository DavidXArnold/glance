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
	"os"
	"testing"
)

const testDefaultNamespace = "default"

func TestNewGlanceConfig(t *testing.T) {
	// Skip this test in CI environments where kubeconfig is not available
	if os.Getenv("CI") != "" || os.Getenv("GITLAB_CI") != "" {
		t.Skip("Skipping test in CI environment - no kubeconfig available")
	}

	gc, err := NewGlanceConfig()

	if err != nil {
		t.Errorf("NewGlanceConfig() returned error: %v", err)
		return
	}

	if gc.configFlags == nil {
		t.Errorf("NewGlanceConfig() configFlags is nil")
	}

	if gc.restConfig == nil {
		t.Errorf("NewGlanceConfig() restConfig is nil")
	}
}

func TestNewGlanceCmdNotNil(t *testing.T) {
	cmd := NewGlanceCmd()

	if cmd.Use != "glance" {
		t.Errorf("NewGlanceCmd() Use = %q, want %q", cmd.Use, "glance")
	}

	if cmd.Short == "" {
		t.Errorf("NewGlanceCmd() Short is empty")
	}

	if cmd.Long == "" {
		t.Errorf("NewGlanceCmd() Long is empty")
	}
}

func TestGetNamespace(t *testing.T) {
	// Skip this test in CI environments where kubeconfig is not available
	if os.Getenv("CI") != "" || os.Getenv("GITLAB_CI") != "" {
		t.Skip("Skipping test in CI environment - no kubeconfig available")
	}

	ns := getNamespace()

	if ns == "" {
		t.Errorf("getNamespace() returned empty string")
	}

	// Most likely returns "default" if no namespace is set
	if ns != testDefaultNamespace && ns != "" {
		t.Logf("getNamespace() returned %q", ns)
	}
}

func TestGetLabelSelector(t *testing.T) {
	selector, err := getLabelSelector()

	if err != nil {
		t.Errorf("getLabelSelector() returned error: %v", err)
	}

	if selector == nil {
		t.Errorf("getLabelSelector() returned nil selector")
	}
}

func TestNewGlanceCmdFlags(t *testing.T) {
	cmd := NewGlanceCmd()

	if cmd.PersistentFlags() == nil {
		t.Errorf("Glance command persistent flags is nil")
	}

	// Check that important flags were added
	if cmd.PersistentFlags().Lookup("selector") == nil {
		t.Errorf("selector flag not found")
	}

	if cmd.PersistentFlags().Lookup("output") == nil {
		t.Errorf("output flag not found")
	}

	if cmd.PersistentFlags().Lookup("show-cloud-provider") == nil {
		t.Errorf("show-cloud-provider flag not found")
	}

	if cmd.PersistentFlags().Lookup("pods") == nil {
		t.Errorf("pods flag not found")
	}
}
