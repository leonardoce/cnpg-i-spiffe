/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package instance

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/cloudnative-pg/cnpg-i/pkg/postgres"
)

// PostgresImplementation is the Postgres service exposed on the
// instance-side plugin socket. It overrides the ssl_ca_file GUC to point at
// the trust bundle written by the agent (see internal/agent).
type PostgresImplementation struct {
	postgres.UnimplementedPostgresServer

	// CertsDir is where the agent writes the SVID, private key and trust
	// bundle. Must match internal/agent.Options.CertsDir.
	CertsDir string

	// SVIDBundleFileName is the file name the agent writes the trust bundle
	// into, inside CertsDir. Must match internal/agent.Options.SVIDBundleFileName.
	SVIDBundleFileName string
}

// GetCapabilities implements the PostgresServer interface
func (PostgresImplementation) GetCapabilities(
	context.Context,
	*postgres.PostgresCapabilitiesRequest,
) (*postgres.PostgresCapabilitiesResult, error) {
	return &postgres.PostgresCapabilitiesResult{
		Capabilities: []*postgres.PostgresCapability{
			{
				Type: &postgres.PostgresCapability_Rpc{
					Rpc: &postgres.PostgresCapability_RPC{
						Type: postgres.PostgresCapability_RPC_TYPE_ENRICH_CONFIGURATION,
					},
				},
			},
		},
	}, nil
}

// EnrichConfiguration implements the PostgresServer interface, overriding
// ssl_ca_file to point at the trust bundle written by the agent.
func (impl PostgresImplementation) EnrichConfiguration(
	_ context.Context,
	req *postgres.EnrichConfigurationRequest,
) (*postgres.EnrichConfigurationResult, error) {
	bundlePath := filepath.Join(impl.CertsDir, impl.SVIDBundleFileName)

	if _, err := os.Stat(bundlePath); err != nil {
		return nil, fmt.Errorf("while looking up the trust bundle: %w", err)
	}

	// The instance manager replaces its whole generated configuration with
	// whatever this RPC returns, rather than merging it in: start from the
	// configuration it handed us and only override ssl_ca_file.
	configs := make(map[string]string, len(req.GetConfigs())+1)
	maps.Copy(configs, req.GetConfigs())
	configs["ssl_ca_file"] = bundlePath

	return &postgres.EnrichConfigurationResult{
		Configs: configs,
	}, nil
}
