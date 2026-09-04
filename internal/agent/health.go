package agent

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"
)

// serveHealthChecks runs the health check HTTP server used by the injected
// container's liveness and readiness probes. Liveness is always healthy
// once the server is up; readiness only becomes healthy once ready reports
// true, i.e. once the first SVID has been written to disk. It blocks until
// ctx is canceled.
func serveHealthChecks(ctx context.Context, opts Options, ready *atomic.Bool) error {
	mux := http.NewServeMux()

	mux.HandleFunc(opts.HealthCheckLivenessPath, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc(opts.HealthCheckReadinessPath, func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:              opts.HealthCheckAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		_ = server.Close()
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return err
	}
}
