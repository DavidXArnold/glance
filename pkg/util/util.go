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
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// SetupLogger sets configuration for the default logger.
func SetupLogger() (err error) {
	var (
		lf = strings.ToLower(viper.GetString("output"))
	)

	// Set log format
	switch lf {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
	default:
		log.SetFormatter(&log.TextFormatter{
			DisableLevelTruncation: true,
		})
	}
	return nil
}

// ParseProviderID returns the cloud provider and associated info.
func ParseProviderID(pi string) (cp string, id []string) {
	s := strings.Split(pi, ":")
	return s[0], strings.Split(strings.TrimPrefix(s[1], "//"), "/")
}

// FormatAge returns a human-readable duration similar to kubectl output.
func FormatAge(t time.Time) string {
	if t.IsZero() {
		return "0s"
	}

	duration := time.Since(t)
	if duration < 0 {
		duration = 0
	}

	seconds := int(duration.Seconds())
	if seconds < 120 {
		return fmt.Sprintf("%ds", seconds)
	}

	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}

	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}

	days := hours / 24
	hours %= 24
	if days < 7 {
		return fmt.Sprintf("%dd%dhr", days, hours)
	}
	if days < 30 {
		return fmt.Sprintf("%dd", days)
	}

	months := days / 30
	if months < 12 {
		return fmt.Sprintf("%dmo", months)
	}

	years := months / 12
	months %= 12
	if months == 0 {
		return fmt.Sprintf("%dyr", years)
	}
	return fmt.Sprintf("%dyr%dmo", years, months)
}
