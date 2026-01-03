package cloud

import "context"

// Provider is implemented by cloud providers capable of returning metadata for
// a given node/instance identifier.
type Provider interface {
	NodeMetadata(ctx context.Context, id string) (*Metadata, error)
}

// ProviderFactory creates a new Provider instance.
type ProviderFactory func() Provider

// Provider names used by glanceutil.ParseProviderID and callers.
const (
	ProviderAWS = "aws"
	ProviderGCE = "gce"
)

var providerRegistry = map[string]ProviderFactory{}

// RegisterProvider registers a provider factory under the given name.
// It is typically called from init() functions in provider-specific files.
func RegisterProvider(name string, factory ProviderFactory) {
	providerRegistry[name] = factory
}

// LookupProvider returns a Provider implementation for the given provider name.
// Unknown providers return nil and should be treated as "no cloud metadata".
func LookupProvider(name string) Provider {
	if factory, ok := providerRegistry[name]; ok {
		return factory()
	}
	return nil
}
