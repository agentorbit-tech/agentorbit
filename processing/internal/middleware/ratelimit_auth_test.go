package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRateLimit_Auth_5PerMinPerIP exercises the 5 req/min/IP limiter wired
// onto the /auth router in main.go (SP-2 #4). The limiter itself is the
// shared NewRateLimiter — we instantiate it with the same parameters the
// production wiring uses and confirm 6th request is denied with Retry-After.
func TestRateLimit_Auth_5PerMinPerIP(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 5, 1*time.Minute, nil)
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(""))
		req.RemoteAddr = "10.0.0.1:1234"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("req %d: got status %d, want 200", i+1, w.Code)
		}
	}

	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(""))
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("6th request: got status %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Errorf("expected Retry-After header on 429 response")
	}
}
