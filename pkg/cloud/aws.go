package cloud

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"
)

const (
	capacityOnDemand = "ON_DEMAND"
	capacityTypeSpot = "SPOT"
)

// awsProvider implements Provider for AWS EC2-backed nodes.
type awsProvider struct{}

// NodeMetadata fetches AWS-specific node information and maps it to Metadata.
func (p *awsProvider) NodeMetadata(ctx context.Context, id string) (*Metadata, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	svc := ec2.NewFromConfig(cfg)
	input := &ec2.DescribeInstancesInput{
		InstanceIds: []string{id},
	}

	result, err := svc.DescribeInstances(ctx, input)
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			return nil, fmt.Errorf("describe instance %s: %s", id, ae.ErrorCode())
		}
		return nil, fmt.Errorf("describe instance %s: %w", id, err)
	}

	if len(result.Reservations) == 0 || len(result.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf("no instance information found for %s", id)
	}

	instance := result.Reservations[0].Instances[0]
	metadata := &Metadata{
		InstanceType: string(instance.InstanceType),
		CapacityType: capacityOnDemand,
	}

	// Extract node group and potential Fargate profile from tags.
	for _, tag := range instance.Tags {
		if tag.Key == nil || tag.Value == nil {
			continue
		}
		switch *tag.Key {
		case "eks:nodegroup-name":
			metadata.NodeGroup = *tag.Value
		case "eks:compute-type":
			if *tag.Value == "fargate" {
				metadata.CapacityType = "FARGATE"
			}
		case "eks:fargate-profile":
			metadata.FargateProfile = *tag.Value
		}
	}

	return metadata, nil
}

// nolint:gochecknoinits // registration-style init keeps provider wiring local to this file.
func init() {
	RegisterProvider(ProviderAWS, func() Provider { return &awsProvider{} })
}
