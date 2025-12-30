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
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"
	log "github.com/sirupsen/logrus"
)

const capacityOnDemand = "ON_DEMAND"
const capacityTypeSpot = "SPOT"

// AWSNodeMetadata holds AWS-specific node information.
type AWSNodeMetadata struct {
	InstanceType   string
	NodeGroup      string // EKS nodegroup name
	CapacityType   string // ON_DEMAND, SPOT, FARGATE
	FargateProfile string // Fargate profile name
}

func getAWSNodeInfo(id string) (*AWSNodeMetadata, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}
	svc := ec2.NewFromConfig(cfg)
	input := &ec2.DescribeInstancesInput{
		InstanceIds: []string{id},
	}
	result, err := svc.DescribeInstances(context.Background(), input)
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			log.Debugf("error occurred describing instance %s: %v", id, ae.ErrorCode())
			return nil, err
		}
		log.Debugf("error occurred describing instance %s: %v", id, err)
		return nil, err
	}

	if len(result.Reservations) > 0 && len(result.Reservations[0].Instances) > 0 {
		instance := result.Reservations[0].Instances[0]
		metadata := &AWSNodeMetadata{
			InstanceType: string(instance.InstanceType),
		}

		// Extract node group from tags
		for _, tag := range instance.Tags {
			if tag.Key != nil && tag.Value != nil {
				switch *tag.Key {
				case "eks:nodegroup-name":
					metadata.NodeGroup = *tag.Value
				case "eks:cluster-name":
					// Store cluster name if needed for context
				}
			}
		}

		metadata.CapacityType = capacityOnDemand

		// Check for Fargate (Fargate nodes have specific tags)
		for _, tag := range instance.Tags {
			if tag.Key != nil && *tag.Key == "eks:compute-type" {
				if tag.Value != nil && *tag.Value == "fargate" {
					metadata.CapacityType = "FARGATE"
					// Extract Fargate profile
					for _, fgTag := range instance.Tags {
						if fgTag.Key != nil && *fgTag.Key == "eks:fargate-profile" && fgTag.Value != nil {
							metadata.FargateProfile = *fgTag.Value
						}
					}
				}
			}
		}

		return metadata, nil
	}

	return nil, fmt.Errorf("no instance information found")
}
