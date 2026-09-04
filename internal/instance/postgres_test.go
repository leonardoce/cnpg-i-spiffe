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

package instance_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudnative-pg/cnpg-i/pkg/postgres"

	"github.com/leonardoce/cnpg-i-spiffe/internal/instance"
)

func TestEnrichConfiguration(t *testing.T) {
	t.Parallel()

	certsDir := t.TempDir()
	bundlePath := filepath.Join(certsDir, "svid_bundle.pem")
	if err := os.WriteFile(bundlePath, []byte("bundle"), 0o600); err != nil {
		t.Fatalf("while writing the trust bundle fixture: %v", err)
	}

	impl := instance.PostgresImplementation{
		CertsDir:           certsDir,
		SVIDBundleFileName: "svid_bundle.pem",
	}

	result, err := impl.EnrichConfiguration(context.Background(), &postgres.EnrichConfigurationRequest{
		Configs: map[string]string{"shared_buffers": "256MB"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := result.GetConfigs()["ssl_ca_file"]; got != bundlePath {
		t.Errorf("expected ssl_ca_file %q, got %q", bundlePath, got)
	}
	if got := result.GetConfigs()["shared_buffers"]; got != "256MB" {
		t.Errorf("expected the incoming configuration to be preserved, got %q for shared_buffers", got)
	}
}

func TestEnrichConfigurationMissingBundle(t *testing.T) {
	t.Parallel()

	impl := instance.PostgresImplementation{
		CertsDir:           t.TempDir(),
		SVIDBundleFileName: "svid_bundle.pem",
	}

	if _, err := impl.EnrichConfiguration(context.Background(), &postgres.EnrichConfigurationRequest{}); err == nil {
		t.Error("expected an error when the trust bundle is missing, got nil")
	}
}
