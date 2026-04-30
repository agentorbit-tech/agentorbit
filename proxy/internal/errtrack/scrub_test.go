package errtrack

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

func TestErrtrack_Capture_StripsAuthorizationHeader(t *testing.T) {
	mt := initWithMock(t, "proxy")
	const token = "Bearer ao-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	req := httptest.NewRequest("POST", "/test", strings.NewReader("body with sk-realkey99999999999999"))
	req.Header.Set("Authorization", token)
	req.Header.Set("X-Api-Key", "sk-realkey99999999999999")
	req.Header.Set("X-Internal-Token", "internal-secret-blah")
	req.Header.Set("Cookie", "session=abcdef")
	req.Header.Set("Content-Type", "application/json")

	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetRequest(req)
		sentry.CaptureException(errors.New("boom"))
	})
	sentry.Flush(time.Second)

	events := mt.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Request == nil {
		t.Fatalf("event.Request is nil; SDK did not capture request")
	}
	hdr := events[0].Request.Headers
	for _, name := range []string{"Authorization", "X-Api-Key", "X-Internal-Token", "Cookie"} {
		if got, ok := lookupHeader(hdr, name); ok && got != "[redacted]" {
			t.Errorf("header %s = %q, want [redacted] or absent", name, got)
		}
	}
	for _, v := range hdr {
		if strings.Contains(v, token) || strings.Contains(v, "ao-aaaa") {
			t.Errorf("header value leaked token: %q", v)
		}
	}
}

func lookupHeader(m map[string]string, name string) (string, bool) {
	if v, ok := m[name]; ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, name) {
			return v, true
		}
	}
	return "", false
}

func TestErrtrack_Capture_StripsBearerInMessage(t *testing.T) {
	mt := initWithMock(t, "proxy")
	Capture(errors.New("failed: Bearer sk-realkey99999999999999"), Fields{"hint": "Bearer sk-anotherrealkey0000000000000"})
	sentry.Flush(time.Second)

	events := mt.Events()
	if len(events) == 0 {
		t.Fatalf("no events captured")
	}
	for _, ev := range events {
		for _, ex := range ev.Exception {
			if strings.Contains(ex.Value, "sk-realkey") || strings.Contains(ex.Value, "Bearer sk-") {
				t.Errorf("exception value leaked: %q", ex.Value)
			}
		}
		for k, v := range ev.Tags {
			if strings.Contains(v, "sk-realkey") || strings.Contains(v, "sk-anotherrealkey") {
				t.Errorf("tag %s leaked key: %q", k, v)
			}
		}
	}
}

func TestErrtrack_CapturePanic_StripsLocals(t *testing.T) {
	mt := initWithMock(t, "proxy")
	defer func() {
		if r := recover(); r != nil {
			CapturePanic(r, Fields{"component": "test"})
		}
	}()
	func() {
		secret := "sk-shouldnotleak999999999999"
		_ = secret
		panic(errors.New("boom"))
	}()
	sentry.Flush(time.Second)

	events := mt.Events()
	if len(events) == 0 {
		t.Fatalf("no events captured")
	}
	for _, ev := range events {
		for _, ex := range ev.Exception {
			if ex.Stacktrace == nil {
				continue
			}
			for _, fr := range ex.Stacktrace.Frames {
				if fr.Vars != nil && len(fr.Vars) > 0 {
					t.Errorf("frame still has Vars: %v", fr.Vars)
				}
			}
		}
	}
}

func TestErrtrack_ScrubEvent_StripsRequestData(t *testing.T) {
	ev := &sentry.Event{
		Request: &sentry.Request{
			Data: "request body with Bearer sk-realkey99999999999999",
		},
	}
	scrubEvent(ev)
	if ev.Request.Data != "[redacted]" {
		t.Errorf("Request.Data = %q, want [redacted]", ev.Request.Data)
	}
}
