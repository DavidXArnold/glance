package cloud

// Metadata holds cloud provider metadata for a node in a provider-agnostic form.
// Fields are a superset of what AWS and GCE currently expose so callers can map
// them onto NodeStats/row structs without depending on provider-specific types.
type Metadata struct {
	InstanceType   string
	NodeGroup      string // AWS nodegroup or generic node group
	NodePool       string // GKE node pool or similar construct
	FargateProfile string // AWS Fargate profile name, if applicable
	CapacityType   string // ON_DEMAND, SPOT, FARGATE, STANDARD, etc.
}
