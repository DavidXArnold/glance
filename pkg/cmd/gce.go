/*
Copyright 2020 David Arnold
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
	"context"

	log "github.com/sirupsen/logrus"

	container "cloud.google.com/go/container/apiv1"
	containerpb "google.golang.org/genproto/googleapis/container/v1"
)

func getGKENodePool(nodepool string) (np *containerpb.NodePool) {
	ctx := context.Background()
	c, err := container.NewClusterManagerClient(ctx)
	if err != nil {
		log.Warn(err)
		return nil
	}
	req := &containerpb.GetNodePoolRequest{
		Name: nodepool,
	}
	resp, err := c.GetNodePool(ctx, req)
	if err != nil {
		log.Warnf("unable to retrieve nodepool: %v %v", nodepool, err)
		return nil
	}
	return resp
}
