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

package identity_test

import (
	"context"
	"testing"

	"github.com/cloudnative-pg/cnpg-i/pkg/identity"

	implPkg "github.com/leonardoce/cnpg-i-spiffe/internal/identity"
)

// TestGetPluginCapabilitiesAdvertisesInstanceSidecarInjection guards against a
// regression that broke EnrichConfiguration end to end: the instance manager
// only loads a plugin's instance-side (Postgres, WAL, ...) services when the
// operator-side Identity service advertises
// PluginCapability_Service_TYPE_INSTANCE_SIDECAR_INJECTION in the Cluster's
// status (see cloudnative-pg's Cluster.GetInstanceEnabledPluginNames).
// Without it, EnrichConfiguration is silently never called.
func TestGetPluginCapabilitiesAdvertisesInstanceSidecarInjection(t *testing.T) {
	t.Parallel()

	resp, err := implPkg.Implementation{}.GetPluginCapabilities(
		context.Background(), &identity.GetPluginCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var foundLifecycle, foundInstanceSidecarInjection bool
	for _, capability := range resp.GetCapabilities() {
		switch capability.GetService().GetType() { //nolint:exhaustive
		case identity.PluginCapability_Service_TYPE_LIFECYCLE_SERVICE:
			foundLifecycle = true
		case identity.PluginCapability_Service_TYPE_INSTANCE_SIDECAR_INJECTION:
			foundInstanceSidecarInjection = true
		}
	}

	if !foundLifecycle {
		t.Error("expected the TYPE_LIFECYCLE_SERVICE capability to be advertised")
	}
	if !foundInstanceSidecarInjection {
		t.Error("expected the TYPE_INSTANCE_SIDECAR_INJECTION capability to be advertised")
	}
}
