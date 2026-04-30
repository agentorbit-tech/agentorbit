package handler

import (
	"context"
	"net/http"
	"time"
)

// Pinger lets the readiness handler accept either *pgxpool.Pool or a fake.
type Pinger interface {
	Ping(ctx context.Context) error
}

// NewLivenessHandler returns an unconditional 200 — the process is up.
// Used by container HEALTHCHECK; must NOT depend on external resources.
func NewLivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}

// NewReadinessHandler verifies the process can serve traffic. Pings the
// pool with a 1s deadline; returns 503 on failure. Used by load balancers
// to drain instances during DB blips and graceful shutdown.
func NewReadinessHandler(p Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()
		if err := p.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"db_unreachable"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}

// NewHealthHandler is retained for backwards compatibility — equivalent to readiness.
// Deprecated: prefer NewReadinessHandler.
func NewHealthHandler(p Pinger) http.HandlerFunc {
	return NewReadinessHandler(p)
}
