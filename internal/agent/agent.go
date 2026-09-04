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

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/http"
	"github.com/cloudnative-pg/cnpg-i/pkg/postgres"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	"github.com/leonardoce/cnpg-i-spiffe/internal/instance"
)

// Options configures the agent
type Options struct {
	// SpireAgentSocketPath is the path of the SPIRE Agent's Workload API socket
	SpireAgentSocketPath string

	// CertsDir is where the SVID, private key and trust bundle are written
	CertsDir string

	// SVIDFileName, SVIDKeyFileName and SVIDBundleFileName are the file
	// names used to write the SVID, its private key and the trust bundle
	// into CertsDir
	SVIDFileName       string
	SVIDKeyFileName    string
	SVIDBundleFileName string

	// PostgresSocketDir is the directory holding PostgreSQL's Unix socket,
	// used to trigger a configuration reload after every SVID rotation
	PostgresSocketDir string

	// PluginPath is the directory holding the Unix socket this agent serves
	// the CNPG-i Postgres service on, shared with the instance manager
	PluginPath string

	// HealthCheckAddr is the address the health check HTTP server listens on
	HealthCheckAddr string

	// HealthCheckLivenessPath and HealthCheckReadinessPath are the paths
	// served by the health check HTTP server
	HealthCheckLivenessPath  string
	HealthCheckReadinessPath string
}

// Run fetches and rotates X.509 SVIDs from the SPIRE Workload API, writing
// them into opts.CertsDir and reloading PostgreSQL after every rotation. It
// blocks until ctx is canceled.
func Run(ctx context.Context, opts Options) error {
	logger := log.FromContext(ctx)

	watcher := &x509Watcher{
		opts:   opts,
		logger: logger,
	}

	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		return serveHealthChecks(ctx, opts, &watcher.ready)
	})

	group.Go(func() error {
		client, err := workloadapi.New(ctx, workloadapi.WithAddr("unix://"+opts.SpireAgentSocketPath))
		if err != nil {
			return fmt.Errorf("while creating the Workload API client: %w", err)
		}
		defer func() { _ = client.Close() }()

		return client.WatchX509Context(ctx, watcher)
	})

	group.Go(func() error {
		server := &http.Server{
			IdentityImpl: instance.IdentityImplementation{},
			Enrichers: []http.ServerEnricher{
				func(s *grpc.Server) error {
					postgres.RegisterPostgresServer(s, instance.PostgresImplementation{
						CertsDir:           opts.CertsDir,
						SVIDBundleFileName: opts.SVIDBundleFileName,
					})

					return nil
				},
			},
			PluginPath: opts.PluginPath,
		}

		return server.Start(ctx)
	})

	return group.Wait()
}

// x509Watcher implements workloadapi.X509ContextWatcher, writing every
// updated SVID/bundle to disk and triggering a PostgreSQL reload
type x509Watcher struct {
	opts   Options
	logger log.Logger
	ready  atomic.Bool
}

// OnX509ContextUpdate is called by the Workload API client with the latest
// X.509 context
func (w *x509Watcher) OnX509ContextUpdate(x509Context *workloadapi.X509Context) {
	svid := x509Context.DefaultSVID()

	certPEM, keyPEM, err := svid.Marshal()
	if err != nil {
		w.logger.Error(err, "while marshaling the SVID")
		return
	}

	bundle, err := x509Context.Bundles.GetX509BundleForTrustDomain(svid.ID.TrustDomain())
	if err != nil {
		w.logger.Error(err, "while getting the trust bundle")
		return
	}

	bundlePEM, err := bundle.Marshal()
	if err != nil {
		w.logger.Error(err, "while marshaling the trust bundle")
		return
	}

	if err := writeFile(filepath.Join(w.opts.CertsDir, w.opts.SVIDFileName), certPEM); err != nil {
		w.logger.Error(err, "while writing the SVID")
		return
	}
	if err := writeFile(filepath.Join(w.opts.CertsDir, w.opts.SVIDKeyFileName), keyPEM); err != nil {
		w.logger.Error(err, "while writing the SVID private key")
		return
	}
	if err := writeFile(filepath.Join(w.opts.CertsDir, w.opts.SVIDBundleFileName), bundlePEM); err != nil {
		w.logger.Error(err, "while writing the trust bundle")
		return
	}

	w.logger.Info("wrote a rotated SVID", "spiffeID", svid.ID.String())
	w.ready.Store(true)

	if err := ReloadPostgres(context.Background(), w.opts.PostgresSocketDir); err != nil {
		// PostgreSQL may not be up yet (e.g. on the very first SVID, before
		// the postgres container has started): log and retry on the next
		// rotation rather than failing the agent.
		w.logger.Info("could not reload PostgreSQL, will retry on the next rotation", "reason", err.Error())
	}
}

// OnX509ContextWatchError is called by the Workload API client when there is
// a problem establishing or maintaining connectivity with the Workload API
func (w *x509Watcher) OnX509ContextWatchError(err error) {
	if err != nil {
		w.logger.Error(err, "while watching the Workload API")
	}
}

// writeFile writes contents to path atomically, via a temporary file in the
// same directory followed by a rename, so readers never observe a partial
// write
func writeFile(path string, contents []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(contents); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), path)
}
