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

package util

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func TestParseProviderID(t *testing.T) {
	tests := []struct {
		name             string
		providerID       string
		expectedProvider string
		expectedParts    []string
	}{
		{
			name:             "AWS provider",
			providerID:       "aws:///us-west-2a/i-1234567890abcdef0",
			expectedProvider: "aws",
			expectedParts:    []string{"", "us-west-2a", "i-1234567890abcdef0"},
		},
		{
			name:             "GCE provider",
			providerID:       "gce://my-project/us-central1-a/my-instance",
			expectedProvider: "gce",
			expectedParts:    []string{"my-project", "us-central1-a", "my-instance"},
		},
		{
			name:             "Azure provider",
			providerID:       "azure:///subscriptions/sub-id/resourceGroups",
			expectedProvider: "azure",
			expectedParts:    []string{"", "subscriptions", "sub-id", "resourceGroups"},
		},
		{
			name:             "Simple provider",
			providerID:       "test://simple",
			expectedProvider: "test",
			expectedParts:    []string{"simple"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, parts := ParseProviderID(tt.providerID)

			if provider != tt.expectedProvider {
				t.Errorf("ParseProviderID(%q) got provider %q, want %q", tt.providerID, provider, tt.expectedProvider)
			}

			if len(parts) != len(tt.expectedParts) {
				t.Errorf("ParseProviderID(%q) got %d parts, want %d", tt.providerID, len(parts), len(tt.expectedParts))
			}

			for i, part := range parts {
				if part != tt.expectedParts[i] {
					t.Errorf("ParseProviderID(%q) part[%d] = %q, want %q", tt.providerID, i, part, tt.expectedParts[i])
				}
			}
		})
	}
}

func TestSetupLogger(t *testing.T) {
	tests := []struct {
		name       string
		outputType string
		checkFunc  func(*testing.T, log.Formatter)
	}{
		{
			name:       "JSON formatter",
			outputType: "json",
			checkFunc: func(t *testing.T, formatter log.Formatter) {
				_, ok := formatter.(*log.JSONFormatter)
				if !ok {
					t.Errorf("Expected JSONFormatter, got %T", formatter)
				}
			},
		},
		{
			name:       "Text formatter default",
			outputType: "txt",
			checkFunc: func(t *testing.T, formatter log.Formatter) {
				_, ok := formatter.(*log.TextFormatter)
				if !ok {
					t.Errorf("Expected TextFormatter, got %T", formatter)
				}
			},
		},
		{
			name:       "Text formatter for unknown type",
			outputType: "unknown",
			checkFunc: func(t *testing.T, formatter log.Formatter) {
				_, ok := formatter.(*log.TextFormatter)
				if !ok {
					t.Errorf("Expected TextFormatter, got %T", formatter)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Set("output", tt.outputType)
			err := SetupLogger()
			if err != nil {
				t.Errorf("SetupLogger() returned error: %v", err)
			}

			tt.checkFunc(t, log.StandardLogger().Formatter)

			// Reset logger state
			viper.Set("output", "txt")
		})
	}
}

func TestSetupLoggerReturnsNil(t *testing.T) {
	viper.Set("output", "json")
	err := SetupLogger()
	if err != nil {
		t.Errorf("SetupLogger() expected nil error, got %v", err)
	}

	viper.Set("output", "txt")
	err = SetupLogger()
	if err != nil {
		t.Errorf("SetupLogger() expected nil error, got %v", err)
	}
}
