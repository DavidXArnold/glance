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
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// SetupLogger sets configuration for the default logger
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

// ParseProviderID returns the cloud provider and associated info
func ParseProviderID(pi string) (cp string, id []string) {
	s := strings.Split(pi, ":")
	return s[0], strings.Split(strings.TrimPrefix(s[1], "//"), "/")
}
