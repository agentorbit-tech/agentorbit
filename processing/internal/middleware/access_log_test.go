// These tests mutate slog.Default() and must NOT run with t.Parallel().
package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

func TestSlogAccess_EmitsJSON(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := SlogAccess(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d: %q", len(lines), buf.String())
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, lines[0])
	}
	if rec["msg"] != "request" {
		t.Errorf("msg = %v, want 'request'", rec["msg"])
	}
	if rec["method"] != "GET" || rec["path"] != "/foo" {
		t.Errorf("method/path mismatch: %v", rec)
	}
	if rec["status"].(float64) != 418 {
		t.Errorf("status = %v, want 418", rec["status"])
	}
}

func TestSlogAccess_SkipsHealthAndReadyz(t *testing.T) {
	for _, p := range []string{"/health", "/readyz"} {
		var buf bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
		t.Cleanup(func() { slog.SetDefault(prev) })

		h := SlogAccess(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if buf.Len() != 0 {
			t.Errorf("expected no log for %s, got %q", p, buf.String())
		}
	}
}

// Verifies request_id is captured when chi's RequestID middleware runs upstream.
func TestSlogAccess_CapturesRequestID(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := chimw.RequestID(SlogAccess(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

	var rec map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &rec); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	if rid, _ := rec["request_id"].(string); rid == "" {
		t.Errorf("request_id missing or empty in: %v", rec)
	}
}

// Verifies duration_ms reflects handler execution time, not zero.
func TestSlogAccess_RecordsDuration(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := SlogAccess(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/y", nil))

	var rec map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &rec); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	d, ok := rec["duration_ms"].(float64)
	if !ok {
		t.Fatalf("duration_ms missing or wrong type: %v", rec)
	}
	if d < 1 {
		t.Errorf("duration_ms = %v, want >= 1 after a 2ms sleep", d)
	}
}
