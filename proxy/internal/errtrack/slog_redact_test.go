package errtrack

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestSlog_LogsAttribute_RedactsApiKey(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{ReplaceAttr: SlogReplaceAttr})
	logger := slog.New(h)
	logger.Info("test", "provider_key", "sk-realkey99999999999999")
	out := buf.String()
	if strings.Contains(out, "sk-realkey99999999999999") {
		t.Errorf("provider_key value leaked: %s", out)
	}
	if !strings.Contains(out, `"provider_key":"[redacted]"`) {
		t.Errorf("expected provider_key redacted, got: %s", out)
	}
}

func TestSlog_LogsAttribute_RedactsBearerInValue(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{ReplaceAttr: SlogReplaceAttr})
	logger := slog.New(h)
	logger.Info("upstream", "auth_header", "Bearer sk-realkey99999999999999")
	out := buf.String()
	if strings.Contains(out, "sk-realkey99999999999999") {
		t.Errorf("Bearer token leaked: %s", out)
	}
}

func TestSlog_LogsAttribute_PreservesNonSensitive(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{ReplaceAttr: SlogReplaceAttr})
	logger := slog.New(h)
	logger.Info("ok", "user_id", "abc-123", "model", "gpt-4")
	out := buf.String()
	if !strings.Contains(out, `"user_id":"abc-123"`) {
		t.Errorf("user_id was scrubbed: %s", out)
	}
}
