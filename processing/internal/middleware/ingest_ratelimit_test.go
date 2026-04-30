package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newIngestHandler(t *testing.T, max int) (http.Handler, *atomic.Int32, *string) {
	t.Helper()
	var bodySeen string
	var calls atomic.Int32
	rl := PerKeyIngestRateLimit(context.Background(), max, 1*time.Minute)
	h := rl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		b, _ := io.ReadAll(r.Body)
		bodySeen = string(b)
		w.WriteHeader(http.StatusAccepted)
	}))
	return h, &calls, &bodySeen
}

func postJSON(h http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/internal/spans/ingest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestPerKeyIngestRateLimit_AllowsUnderLimit(t *testing.T) {
	h, calls, _ := newIngestHandler(t, 5)
	body := `{"api_key_id":"k1","model":"x"}`
	for i := 0; i < 5; i++ {
		w := postJSON(h, body)
		if w.Code != http.StatusAccepted {
			t.Fatalf("req %d: status = %d, want 202", i, w.Code)
		}
	}
	if got := calls.Load(); got != 5 {
		t.Errorf("expected 5 handler calls, got %d", got)
	}
}

func TestPerKeyIngestRateLimit_BlocksAtLimit(t *testing.T) {
	h, _, _ := newIngestHandler(t, 3)
	body := `{"api_key_id":"k1"}`
	for i := 0; i < 3; i++ {
		w := postJSON(h, body)
		if w.Code != http.StatusAccepted {
			t.Fatalf("req %d: %d", i, w.Code)
		}
	}
	w := postJSON(h, body)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("4th: status = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Errorf("expected Retry-After")
	}
}

func TestPerKeyIngestRateLimit_DifferentKeysIsolated(t *testing.T) {
	h, _, _ := newIngestHandler(t, 1)
	w := postJSON(h, `{"api_key_id":"k1"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("k1 first: %d", w.Code)
	}
	w = postJSON(h, `{"api_key_id":"k2"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("k2 first should pass (different bucket): %d", w.Code)
	}
	w = postJSON(h, `{"api_key_id":"k1"}`)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("k1 second: want 429, got %d", w.Code)
	}
}

func TestPerKeyIngestRateLimit_BodyPreserved(t *testing.T) {
	h, _, bodySeen := newIngestHandler(t, 5)
	body := `{"api_key_id":"k1","model":"gpt-4","input":"hello"}`
	w := postJSON(h, body)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d", w.Code)
	}
	if *bodySeen != body {
		t.Errorf("downstream saw %q, want %q", *bodySeen, body)
	}
}

func TestPerKeyIngestRateLimit_MalformedJSONFastPath(t *testing.T) {
	h, calls, _ := newIngestHandler(t, 1)
	// Body without api_key_id (or non-JSON) should pass through; downstream will 400.
	for i := 0; i < 5; i++ {
		w := postJSON(h, `{"foo":"bar"}`)
		if w.Code != http.StatusAccepted {
			t.Fatalf("malformed req %d: %d", i, w.Code)
		}
	}
	if calls.Load() != 5 {
		t.Errorf("expected all 5 to pass through, got %d", calls.Load())
	}
}

func TestPerKeyIngestRateLimit_BucketTimeWindow(t *testing.T) {
	// Use a tiny window to test that old timestamps drop out.
	rl := PerKeyIngestRateLimit(context.Background(), 2, 50*time.Millisecond)
	h := rl(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	for i := 0; i < 2; i++ {
		if w := postJSON(h, `{"api_key_id":"k1"}`); w.Code != http.StatusAccepted {
			t.Fatalf("req %d: %d", i, w.Code)
		}
	}
	if w := postJSON(h, `{"api_key_id":"k1"}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd: want 429, got %d", w.Code)
	}
	time.Sleep(80 * time.Millisecond)
	if w := postJSON(h, `{"api_key_id":"k1"}`); w.Code != http.StatusAccepted {
		t.Fatalf("after window: want 202, got %d", w.Code)
	}
}
