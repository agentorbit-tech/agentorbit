package middleware

import (
	"net/http"
)

// DBPoolStat is the minimal interface exposed by *pgxpool.Pool that we
// need for the breaker. Defining an interface keeps the middleware
// trivially testable without spinning up a real PostgreSQL connection
// pool.
type DBPoolStat interface {
	AcquiredConns() int32
	MaxConns() int32
}

// DBPoolStatProvider returns the current snapshot of the pool. We keep
// the indirection simple so test doubles can implement a one-line stub.
type DBPoolStatProvider interface {
	Stat() DBPoolStat
}

// PgxPoolStat adapts *pgxpool.Stat to DBPoolStat. *pgxpool.Stat already has
// AcquiredConns() and MaxConns() but is a struct, so we wrap it in a tiny
// adapter to satisfy our interface.
type PgxPoolStat struct {
	Acquired int32
	Max      int32
}

func (p PgxPoolStat) AcquiredConns() int32 { return p.Acquired }
func (p PgxPoolStat) MaxConns() int32      { return p.Max }

// PgxPoolAdapter wraps any value with a Stat() method returning a struct
// that has AcquiredConns() and MaxConns() methods (matching pgxpool.Pool).
// We use a function-typed adapter so the production wiring can simply
// pass `middleware.NewPgxAdapter(pool)`.
type PgxPoolAdapter struct {
	StatFn func() (acquired, max int32)
}

func (a PgxPoolAdapter) Stat() DBPoolStat {
	acq, mx := a.StatFn()
	return PgxPoolStat{Acquired: acq, Max: mx}
}

// NewPgxAdapter returns an adapter that calls the pgxpool.Pool.Stat()
// method via a closure, avoiding a direct import of pgxpool in this
// file (keeps middleware free of DB driver dependencies).
func NewPgxAdapter(statFn func() (acquired, max int32)) DBPoolStatProvider {
	return PgxPoolAdapter{StatFn: statFn}
}

// DBPoolBreaker returns a middleware that rejects requests with 503
// db_overloaded when AcquiredConns / MaxConns exceeds threshold. Used on
// /internal/spans/ingest so a span ingestion burst does not starve the
// dashboard API of pool capacity.
//
// threshold is a fraction in (0, 1]. Typical values: 0.8 (reject above
// 80% utilisation). The Retry-After header advises a short backoff.
func DBPoolBreaker(provider DBPoolStatProvider, threshold float64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			stat := provider.Stat()
			max := stat.MaxConns()
			if max <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			usage := float64(stat.AcquiredConns()) / float64(max)
			if usage > threshold {
				w.Header().Set("Retry-After", "5")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"db_overloaded","message":"DB pool saturated; retry shortly"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
