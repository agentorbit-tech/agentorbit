package errtrack

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
)

func TestMiddleware_RecoversPanic_Returns500(t *testing.T) {
	initWithMock(t, "proxy")
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/boom", func(_ http.ResponseWriter, _ *http.Request) {
		panic("kaboom")
	})

	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/boom")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: got %d want 500", resp.StatusCode)
	}
}

func TestMiddleware_CapturesEventWithRoutePattern(t *testing.T) {
	mt := initWithMock(t, "proxy")
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})

	srv := httptest.NewServer(r)
	defer srv.Close()

	_, _ = http.Get(srv.URL + "/users/secret-value-123")
	sentry.Flush(time.Second)

	events := mt.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Tags["path"] != "/users/{id}" {
		t.Errorf("path tag: got %q want pattern /users/{id}", ev.Tags["path"])
	}
	if ev.Tags["method"] != "GET" {
		t.Errorf("method tag: got %q", ev.Tags["method"])
	}
	if ev.Tags["component"] != "http" {
		t.Errorf("component tag: got %q", ev.Tags["component"])
	}
}

func TestMiddleware_DoesNotLeakRequestBody(t *testing.T) {
	mt := initWithMock(t, "proxy")
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Post("/echo", func(_ http.ResponseWriter, _ *http.Request) {
		panic("with body in flight")
	})

	srv := httptest.NewServer(r)
	defer srv.Close()

	const secret = "sk-live-SECRET-DO-NOT-LEAK-abc123"
	_, _ = http.Post(srv.URL+"/echo", "application/json",
		strings.NewReader(`{"api_key":"`+secret+`"}`))
	sentry.Flush(time.Second)

	for _, ev := range mt.Events() {
		if ev.Request != nil {
			if strings.Contains(ev.Request.Data, secret) {
				t.Errorf("event.Request.Data leaked secret: %q", ev.Request.Data)
			}
			for k, v := range ev.Request.Headers {
				if strings.Contains(v, secret) {
					t.Errorf("event.Request.Headers[%q] leaked secret", k)
				}
			}
		}
		for k, ctx := range ev.Contexts {
			for field, v := range ctx {
				if s, ok := v.(string); ok && strings.Contains(s, secret) {
					t.Errorf("event.Contexts[%q][%q] leaked secret", k, field)
				}
			}
		}
		if ev.Message != "" && strings.Contains(ev.Message, secret) {
			t.Errorf("event.Message leaked secret")
		}
	}
}
