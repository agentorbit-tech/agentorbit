package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubProvider struct {
	acquired int32
	max      int32
}

func (s stubProvider) Stat() DBPoolStat {
	return PgxPoolStat{Acquired: s.acquired, Max: s.max}
}

func TestDBPoolBreaker_HighUsage_Rejects(t *testing.T) {
	provider := stubProvider{acquired: 21, max: 25} // 84%
	mw := DBPoolBreaker(provider, 0.8)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	req := httptest.NewRequest("POST", "/internal/spans/ingest", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestDBPoolBreaker_LowUsage_PassesThrough(t *testing.T) {
	provider := stubProvider{acquired: 5, max: 25} // 20%
	mw := DBPoolBreaker(provider, 0.8)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	req := httptest.NewRequest("POST", "/internal/spans/ingest", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", w.Code)
	}
}

func TestDBPoolBreaker_AtThreshold_PassesThrough(t *testing.T) {
	// usage equals threshold exactly — must pass (we use strict > comparison).
	provider := stubProvider{acquired: 20, max: 25} // 0.8
	mw := DBPoolBreaker(provider, 0.8)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	req := httptest.NewRequest("POST", "/internal/spans/ingest", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202 at exact threshold", w.Code)
	}
}
