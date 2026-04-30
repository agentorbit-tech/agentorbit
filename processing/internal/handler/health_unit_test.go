package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// fakePinger satisfies the minimal interface used by ReadinessHandler.
type fakePinger struct{ err error }

func (f *fakePinger) Ping(ctx context.Context) error { return f.err }

func TestLivenessAlwaysOK(t *testing.T) {
	h := NewLivenessHandler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("liveness = %d, want 200", rr.Code)
	}
}

func TestReadinessFailsWhenDBDown(t *testing.T) {
	h := NewReadinessHandler(&fakePinger{err: context.DeadlineExceeded})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness = %d, want 503", rr.Code)
	}
	if got := rr.Body.String(); got != `{"status":"db_unreachable"}` {
		t.Errorf("body = %q, want db_unreachable JSON", got)
	}
}

func TestReadinessOKWhenDBUp(t *testing.T) {
	h := NewReadinessHandler(&fakePinger{err: nil})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("readiness = %d, want 200", rr.Code)
	}
	if got := rr.Body.String(); got != `{"status":"ok"}` {
		t.Errorf("body = %q, want ok JSON", got)
	}
}

// Compile-time assertion: *pgxpool.Pool satisfies the Pinger interface so we
// can pass it from main.go without a wrapper.
var _ Pinger = (*pgxpool.Pool)(nil)
