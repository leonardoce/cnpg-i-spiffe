// Package metadata contains the metadata of this plugin
package metadata

import "github.com/cloudnative-pg/cnpg-i/pkg/identity"

// PluginName is the name of the plugin
const PluginName = "cnpg-i-spiffe.cloudnative-pg.io"

// Data is the metadata of this plugin
var Data = identity.GetPluginMetadataResponse{
	Name:          PluginName,
	Version:       "0.0.1",
	DisplayName:   "SPIFFE/SPIRE sidecar injector",
	ProjectUrl:    "https://github.com/leonardoce/cnpg-i-spiffe",
	RepositoryUrl: "https://github.com/leonardoce/cnpg-i-spiffe",
	License:       "Apache 2.0",
	LicenseUrl:    "https://github.com/leonardoce/cnpg-i-spiffe/LICENSE",
	Maturity:      "alpha",
}
