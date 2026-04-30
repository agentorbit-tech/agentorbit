// Package errtrack wraps Sentry error tracking with an explicit-context-only API.
// When SENTRY_DSN is empty, all operations are no-ops and the package has no runtime cost.
package errtrack

import (
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/getsentry/sentry-go"
)

// sensitiveHeaders is the set of HTTP header names whose values must be
// stripped before an event is sent to Sentry. Names compared case-insensitively.
var sensitiveHeaders = map[string]struct{}{
	"authorization":    {},
	"x-api-key":        {},
	"x-internal-token": {},
	"cookie":           {},
}

var keyTokenRegex = regexp.MustCompile(`(sk-|ao-)[a-zA-Z0-9]{20,}`)
var bearerRegex = regexp.MustCompile(`Bearer\s+\S+`)

// redactSecrets returns s with API keys and Bearer tokens replaced.
func redactSecrets(s string) string {
	if s == "" {
		return s
	}
	s = keyTokenRegex.ReplaceAllString(s, "[redacted-key]")
	s = bearerRegex.ReplaceAllString(s, "Bearer [redacted]")
	return s
}

// scrubEvent strips sensitive headers, body, and stack-frame locals.
func scrubEvent(event *sentry.Event) *sentry.Event {
	if event == nil {
		return nil
	}
	if event.Request != nil {
		if event.Request.Headers != nil {
			for name := range event.Request.Headers {
				lower := name
				for i := 0; i < len(lower); i++ {
					if lower[i] >= 'A' && lower[i] <= 'Z' {
						b := []byte(lower)
						b[i] = lower[i] + 32
						lower = string(b)
					}
				}
				if _, ok := sensitiveHeaders[lower]; ok {
					event.Request.Headers[name] = "[redacted]"
				}
			}
		}
		if event.Request.Data != "" {
			event.Request.Data = "[redacted]"
		}
	}
	for i := range event.Exception {
		if event.Exception[i].Stacktrace != nil {
			for j := range event.Exception[i].Stacktrace.Frames {
				event.Exception[i].Stacktrace.Frames[j].Vars = nil
			}
		}
		event.Exception[i].Value = redactSecrets(event.Exception[i].Value)
	}
	for i := range event.Threads {
		if event.Threads[i].Stacktrace != nil {
			for j := range event.Threads[i].Stacktrace.Frames {
				event.Threads[i].Stacktrace.Frames[j].Vars = nil
			}
		}
	}
	for ctxName, ctxMap := range event.Contexts {
		for k, v := range ctxMap {
			if s, ok := v.(string); ok {
				ctxMap[k] = redactSecrets(s)
			}
		}
		event.Contexts[ctxName] = ctxMap
	}
	if event.Tags != nil {
		for k, v := range event.Tags {
			event.Tags[k] = redactSecrets(v)
		}
	}
	if event.Message != "" {
		event.Message = redactSecrets(event.Message)
	}
	return event
}

type Config struct {
	DSN         string
	Environment string
	Release     string
	Service     string
	SampleRate  float64
}

type Fields map[string]string

var enabled bool

func resetForTest() {
	enabled = false
}

// Init configures Sentry. If cfg.DSN is empty, tracking is disabled and this returns nil.
// A non-nil return value means the Sentry SDK rejected the config; callers should log
// and continue — error tracking must never block startup.
func Init(cfg Config) error {
	if cfg.DSN == "" {
		enabled = false
		slog.Info("error tracking disabled")
		return nil
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Environment,
		Release:          cfg.Release,
		ServerName:       "",
		SendDefaultPII:   false,
		AttachStacktrace: true,
		SampleRate:       cfg.SampleRate,
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			return scrubEvent(event)
		},
	})
	if err != nil {
		return fmt.Errorf("sentry init: %w", err)
	}
	sentry.CurrentHub().Scope().SetTags(map[string]string{
		"service": cfg.Service,
		"tier":    cfg.Environment,
	})
	enabled = true
	slog.Info("sentry initialized", "environment", cfg.Environment, "release", cfg.Release)
	return nil
}

// Capture sends an error event to Sentry with the given fields as tags.
// No-op when disabled or err is nil. Never attaches request bodies, headers, or locals.
func Capture(err error, fields Fields) {
	if !enabled || err == nil {
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		for k, v := range fields {
			scope.SetTag(k, v)
		}
		sentry.CaptureException(err)
	})
}

// CapturePanic is for use inside defer recover() blocks. Converts r to an error.
func CapturePanic(r any, fields Fields) {
	if !enabled || r == nil {
		return
	}
	var err error
	if e, ok := r.(error); ok {
		err = e
	} else {
		err = fmt.Errorf("panic: %v", r)
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		for k, v := range fields {
			scope.SetTag(k, v)
		}
		sentry.CurrentHub().Recover(err)
	})
}

// Flush drains pending events. Call during graceful shutdown after workers have stopped.
func Flush(timeout time.Duration) {
	if !enabled {
		return
	}
	sentry.Flush(timeout)
	slog.Info("sentry flush complete", "timeout", timeout)
}
