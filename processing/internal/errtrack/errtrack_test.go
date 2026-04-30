package errtrack

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

func TestInit_EmptyDSN_DisablesTracking(t *testing.T) {
	resetForTest()
	if err := Init(Config{DSN: "", Service: "proxy"}); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if enabled {
		t.Fatal("expected enabled=false when DSN is empty")
	}
}

func TestCapture_WhenDisabled_IsNoOp(t *testing.T) {
	resetForTest()
	_ = Init(Config{DSN: "", Service: "proxy"})
	Capture(errors.New("boom"), Fields{"component": "test"})
}

func TestCapture_NilError_IsNoOp(t *testing.T) {
	resetForTest()
	enabled = true
	Capture(nil, Fields{"component": "test"})
	enabled = false
}

func TestFlush_WhenDisabled_IsNoOp(t *testing.T) {
	resetForTest()
	_ = Init(Config{DSN: "", Service: "proxy"})
	Flush(100 * time.Millisecond)
}

// mockTransport captures events instead of sending them to Sentry.
type mockTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (m *mockTransport) Configure(sentry.ClientOptions) {}
func (m *mockTransport) SendEvent(event *sentry.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}
func (m *mockTransport) Flush(time.Duration) bool                    { return true }
func (m *mockTransport) FlushWithContext(_ context.Context) bool     { return true }
func (m *mockTransport) Close()                                      {}
func (m *mockTransport) Events() []*sentry.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*sentry.Event, len(m.events))
	copy(out, m.events)
	return out
}

func initWithMock(t *testing.T, svc string) *mockTransport {
	t.Helper()
	mt := &mockTransport{}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              "https://public@sentry.example.com/1",
		Environment:      "test",
		Transport:        mt,
		SendDefaultPII:   false,
		AttachStacktrace: true,
		SampleRate:       1.0,
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			return scrubEvent(event)
		},
	})
	if err != nil {
		t.Fatalf("sentry init: %v", err)
	}
	sentry.CurrentHub().Scope().SetTags(map[string]string{"service": svc, "tier": "test"})
	enabled = true
	t.Cleanup(func() {
		enabled = false
		sentry.Flush(time.Second)
	})
	return mt
}

func TestCapture_Enabled_SendsEventWithTags(t *testing.T) {
	mt := initWithMock(t, "proxy")
	Capture(errors.New("dispatch failed"), Fields{"component": "span_dispatch", "stage": "send"})
	sentry.Flush(time.Second)

	events := mt.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Tags["component"] != "span_dispatch" {
		t.Errorf("component tag: got %q", ev.Tags["component"])
	}
	if ev.Tags["stage"] != "send" {
		t.Errorf("stage tag: got %q", ev.Tags["stage"])
	}
	if ev.Tags["service"] != "proxy" {
		t.Errorf("service tag: got %q", ev.Tags["service"])
	}
}

func TestCapturePanic_WithStringValue_ConvertsToError(t *testing.T) {
	mt := initWithMock(t, "proxy")
	CapturePanic("something bad happened", Fields{"component": "masking"})
	sentry.Flush(time.Second)

	events := mt.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if len(ev.Exception) == 0 {
		t.Fatal("expected exception on event")
	}
	msg := ev.Exception[0].Value
	if msg != "panic: something bad happened" {
		t.Errorf("expected converted message, got %q", msg)
	}
}

func TestCapturePanic_WithErrorValue_UsesErrorDirectly(t *testing.T) {
	mt := initWithMock(t, "proxy")
	origErr := errors.New("worker crashed")
	CapturePanic(origErr, Fields{"component": "worker"})
	sentry.Flush(time.Second)

	events := mt.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Exception[0].Value != "worker crashed" {
		t.Errorf("expected original error message, got %q", events[0].Exception[0].Value)
	}
}
