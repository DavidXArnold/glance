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
	"errors"
	"os"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)


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

	// restConfig is now populated lazily in RunE, so it should be nil initially
	if gc.restConfig != nil {
		t.Errorf("NewGlanceConfig() restConfig should be nil (lazy loaded)")
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
}

// TestGlanceK8sErrorPropagation documents the intent that GlanceK8s should
// return errors instead of exiting the process. It is currently skipped
// because reliably provoking an error without a real cluster or heavy
// mocking is non-trivial.
func TestGlanceK8sErrorPropagation(t *testing.T) {
	t.Skip("TODO(issue 20): full GlanceK8s integration error test still requires a richer fake clientset")
}

func TestIsMetricsServerNotAvailable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "NotFound error from API",
			err:      k8serrors.NewNotFound(schema.GroupResource{Group: "metrics.k8s.io", Resource: "nodes"}, "foo"),
			expected: true,
		},
		{
			name:     "generic metrics.k8s.io missing message",
			err:      errors.New("the server could not find the requested resource (get pods.metrics.k8s.io)"),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("some other failure"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMetricsServerNotAvailable(tt.err)
			if result != tt.expected {
				t.Errorf("isMetricsServerNotAvailable(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}
