package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWithDeadline_AddsDeadline(t *testing.T) {
	var seen time.Time
	h := WithDeadline(50 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dl, ok := r.Context().Deadline()
		if !ok {
			t.Fatal("no deadline on context")
		}
		seen = dl
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if seen.Before(time.Now()) || seen.After(time.Now().Add(60*time.Millisecond)) {
		t.Fatalf("deadline %v out of expected window", seen)
	}
}

func TestWithDeadline_PreservesExistingDeadline(t *testing.T) {
	earlier := time.Now().Add(10 * time.Millisecond)
	parent, cancel := context.WithDeadline(context.Background(), earlier)
	defer cancel()
	var seen time.Time
	h := WithDeadline(1 * time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = r.Context().Deadline()
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !seen.Equal(earlier) {
		t.Fatalf("middleware overrode earlier deadline: got %v want %v", seen, earlier)
	}
}
