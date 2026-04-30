package middleware

import (
	"context"
	"net/http"
	"time"
)

// WithDeadline attaches a per-request deadline. If the parent context already
// has an earlier deadline, that one wins (context.WithDeadline behavior).
func WithDeadline(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
