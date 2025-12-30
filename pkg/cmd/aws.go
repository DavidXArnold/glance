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
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"
	log "github.com/sirupsen/logrus"
)

func getAWSNodeInfo(id string) *ec2.DescribeInstancesOutput {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load aws config: %v", err)
	}
	svc := ec2.NewFromConfig(cfg)
	input := &ec2.DescribeInstancesInput{
		InstanceIds: []string{id},
	}
	result, err := svc.DescribeInstances(context.Background(), input)
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			log.Warnf("error occurred describing instance %s: %v", id, ae.ErrorCode())
			return &ec2.DescribeInstancesOutput{}
		}
		log.Warnf("error occurred describing instance %s: %v", id, err)
		return &ec2.DescribeInstancesOutput{}
	}
	return result
}
