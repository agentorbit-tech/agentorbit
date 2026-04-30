package errtrack

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Middleware recovers panics, logs them, emits a Sentry event with route pattern context,
// and returns 500. Replaces chi's built-in Recoverer so panics reach errtrack.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rv := recover()
			if rv == nil {
				return
			}
			if rv == http.ErrAbortHandler {
				panic(rv)
			}
			pattern := ""
			if rc := chi.RouteContext(r.Context()); rc != nil {
				pattern = rc.RoutePattern()
			}
			slog.Error("http handler panic", "error", rv, "method", r.Method, "path", pattern)
			CapturePanic(rv, Fields{
				"component": "http",
				"method":    r.Method,
				"path":      pattern,
			})
			w.WriteHeader(http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}
