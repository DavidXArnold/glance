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

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gitlab.com/davidxarnold/glance/pkg/cmd"
)

func main() {
	// Ctrl-C or SIGTERM cancels in-flight API requests instead of leaving
	// the process waiting on a slow or unresponsive API server.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	root := cmd.NewGlanceCmd()
	err := root.ExecuteContext(ctx)
	stop()
	if err != nil {
		// Print a user-facing error to stderr and exit with non-zero status.
		// Logging is handled inside the library code via configured logrus.
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
