// Package instance implements the CNPG-i services exposed by the plugin from
// inside the PostgreSQL Pod, over the Unix socket shared with the instance
// manager under the "plugins" volume.
package instance

import (
	"context"

	"github.com/cloudnative-pg/cnpg-i/pkg/identity"

	"github.com/leonardoce/cnpg-i-spiffe/pkg/metadata"
)

// IdentityImplementation is the identity service exposed on the
// instance-side plugin socket. It is distinct from internal/identity's
// Implementation, which is exposed on the operator-side plugin socket and
// advertises a different set of capabilities.
type IdentityImplementation struct {
	identity.IdentityServer
}

// GetPluginMetadata implements the IdentityServer interface
func (IdentityImplementation) GetPluginMetadata(
	context.Context,
	*identity.GetPluginMetadataRequest,
) (*identity.GetPluginMetadataResponse, error) {
	return &metadata.Data, nil
}

// GetPluginCapabilities implements the IdentityServer interface
func (IdentityImplementation) GetPluginCapabilities(
	context.Context,
	*identity.GetPluginCapabilitiesRequest,
) (*identity.GetPluginCapabilitiesResponse, error) {
	return &identity.GetPluginCapabilitiesResponse{
		Capabilities: []*identity.PluginCapability{
			{
				Type: &identity.PluginCapability_Service_{
					Service: &identity.PluginCapability_Service{
						Type: identity.PluginCapability_Service_TYPE_POSTGRES,
					},
				},
			},
		},
	}, nil
}

// Probe implements the IdentityServer interface
func (IdentityImplementation) Probe(context.Context, *identity.ProbeRequest) (*identity.ProbeResponse, error) {
	return &identity.ProbeResponse{
		Ready: true,
	}, nil
}
