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

package config

import (
	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/common"
)

const (
	sidecarImageParameter         = "sidecarImage"
	spireAgentSocketPathParameter = "spireAgentSocketPath"
	certsMountPathParameter       = "certsMountPath"
	certsVolumeMediumParameter    = "certsVolumeMedium"
	svidFileNameParameter         = "svidFileName"
	svidKeyFileNameParameter      = "svidKeyFileName"
	svidBundleFileNameParameter   = "svidBundleFileName"
	postgresSocketDirParameter    = "postgresSocketDir"
)

// Configuration represents the plugin configuration parameters
type Configuration struct {
	// SidecarImage is the image used for the injected agent sidecar. It has
	// no default: unlike the previous spiffe-helper-based sidecar, there is
	// no stable public tag for this plugin's own image, so it must be set
	// explicitly to whatever image this plugin was deployed with.
	SidecarImage string

	// SpireAgentSocketPath is the path, on the node, of the SPIRE Agent's
	// Workload API socket. Its parent directory is hostPath-mounted into the
	// sidecar container.
	SpireAgentSocketPath string

	// CertsMountPath is where the SVID/bundle files are mounted in both the
	// sidecar and the postgres container.
	CertsMountPath string

	// CertsVolumeMedium is the storage medium of the certs volume: "Memory"
	// for a tmpfs-backed emptyDir (the default, keeping key material off
	// disk), or "Disk" for a regular disk-backed emptyDir.
	CertsVolumeMedium string

	// SVIDFileName, SVIDKeyFileName and SVIDBundleFileName are the file names
	// the sidecar writes into the certs volume.
	SVIDFileName       string
	SVIDKeyFileName    string
	SVIDBundleFileName string

	// PostgresSocketDir is the directory holding PostgreSQL's Unix socket,
	// mounted into the sidecar so it can reload PostgreSQL's configuration
	// after every SVID rotation.
	PostgresSocketDir string
}

// FromParameters builds a plugin configuration from the configuration parameters
func FromParameters(helper *common.Plugin) *Configuration {
	configuration := &Configuration{
		SidecarImage:         helper.Parameters[sidecarImageParameter],
		SpireAgentSocketPath: helper.Parameters[spireAgentSocketPathParameter],
		CertsMountPath:       helper.Parameters[certsMountPathParameter],
		CertsVolumeMedium:    helper.Parameters[certsVolumeMediumParameter],
		SVIDFileName:         helper.Parameters[svidFileNameParameter],
		SVIDKeyFileName:      helper.Parameters[svidKeyFileNameParameter],
		SVIDBundleFileName:   helper.Parameters[svidBundleFileNameParameter],
		PostgresSocketDir:    helper.Parameters[postgresSocketDirParameter],
	}

	configuration.applyDefaults()

	return configuration
}

// applyDefaults fills the configuration with the defaults
func (config *Configuration) applyDefaults() {
	if config.PostgresSocketDir == "" {
		config.PostgresSocketDir = "/controller/run"
	}
	if config.SpireAgentSocketPath == "" {
		config.SpireAgentSocketPath = "/run/spire/agent-sockets/spire-agent.sock"
	}
	if config.CertsMountPath == "" {
		config.CertsMountPath = "/spiffe-certs"
	}
	if config.CertsVolumeMedium == "" {
		config.CertsVolumeMedium = "Memory"
	}
	if config.SVIDFileName == "" {
		config.SVIDFileName = "svid.pem"
	}
	if config.SVIDKeyFileName == "" {
		config.SVIDKeyFileName = "svid_key.pem"
	}
	if config.SVIDBundleFileName == "" {
		config.SVIDBundleFileName = "svid_bundle.pem"
	}
}
