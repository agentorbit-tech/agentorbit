package config

import (
	"os"
	"testing"
	"time"
)

// setRequiredEnv sets the minimum required env vars for LoadProxy to succeed.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PROCESSING_URL", "https://processing.example.com")
	t.Setenv("INTERNAL_TOKEN", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	t.Setenv("HMAC_SECRET", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
}

func TestLoadProxy_Success(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if cfg.ProcessingURL != "https://processing.example.com" {
		t.Errorf("ProcessingURL = %q", cfg.ProcessingURL)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.SpanBufferSize != 1000 {
		t.Errorf("SpanBufferSize = %d, want 1000", cfg.SpanBufferSize)
	}
	if cfg.SpanWorkers != 3 {
		t.Errorf("SpanWorkers = %d, want 3", cfg.SpanWorkers)
	}
	if cfg.PerKeyRateLimit != 120 {
		t.Errorf("PerKeyRateLimit = %d, want 120", cfg.PerKeyRateLimit)
	}
}

func TestLoadProxy_MissingRequiredEnvs(t *testing.T) {
	// Clear all required env vars
	os.Clearenv()

	_, err := LoadProxy()
	if err == nil {
		t.Fatal("expected error for missing env vars")
	}
}

func TestLoadProxy_MissingProcessingURL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PROCESSING_URL", "")

	_, err := LoadProxy()
	if err == nil {
		t.Fatal("expected error for missing PROCESSING_URL")
	}
}

func TestLoadProxy_ShortHMACSecret(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HMAC_SECRET", "short")

	_, err := LoadProxy()
	if err == nil {
		t.Fatal("expected error for short HMAC_SECRET")
	}
}

func TestLoadProxy_ShortInternalToken(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("INTERNAL_TOKEN", "short")

	_, err := LoadProxy()
	if err == nil {
		t.Fatal("expected error for short INTERNAL_TOKEN")
	}
}

func TestLoadProxy_InvalidPort(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PROXY_PORT", "notanumber")

	_, err := LoadProxy()
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

func TestLoadProxy_PortOutOfRange(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PROXY_PORT", "99999")

	_, err := LoadProxy()
	if err == nil {
		t.Fatal("expected error for port out of range")
	}
}

func TestLoadProxy_SpanBufferSizeCapped(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SPAN_BUFFER_SIZE", "999999")

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SpanBufferSize != 100000 {
		t.Errorf("SpanBufferSize = %d, want 100000 (capped)", cfg.SpanBufferSize)
	}
}

func TestLoadProxy_PlaintextProcessingURL_Rejected(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PROCESSING_URL", "http://processing.example.com")

	_, err := LoadProxy()
	if err == nil {
		t.Fatal("expected error for HTTP PROCESSING_URL without AllowPlaintextInternal")
	}
}

func TestLoadProxy_PlaintextProcessingURL_Allowed(t *testing.T) {
	setRequiredEnv(t)
	// ALLOW_PLAINTEXT_INTERNAL now also requires the host to resolve to a
	// loopback / RFC1918 IP — use a literal RFC1918 address so the test
	// does not depend on DNS.
	t.Setenv("PROCESSING_URL", "http://10.0.0.5:8081")
	t.Setenv("ALLOW_PLAINTEXT_INTERNAL", "true")

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.AllowPlaintextInternal {
		t.Error("AllowPlaintextInternal should be true")
	}
}

func TestLoadProxy_CustomPort(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PROXY_PORT", "9090")

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
}

func TestLoadProxy_PortFallback(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PORT", "3000")

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "3000" {
		t.Errorf("Port = %q, want 3000 (PORT fallback)", cfg.Port)
	}
}

func TestLoadProxy_AllowPrivateProviderIPs(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ALLOW_PRIVATE_PROVIDER_IPS", "true")

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.AllowPrivateProviderIPs {
		t.Error("AllowPrivateProviderIPs should be true")
	}
}

func TestLoadProxy_ShutdownHTTPTimeout_Default(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ShutdownHTTPTimeout != 30*time.Second {
		t.Errorf("ShutdownHTTPTimeout = %v, want 30s", cfg.ShutdownHTTPTimeout)
	}
}

func TestLoadProxy_ShutdownHTTPTimeout_Custom(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SHUTDOWN_HTTP_TIMEOUT_SECONDS", "45")

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ShutdownHTTPTimeout != 45*time.Second {
		t.Errorf("ShutdownHTTPTimeout = %v, want 45s", cfg.ShutdownHTTPTimeout)
	}
}

func TestLoadProxy_ShutdownDrainTimeout_Default(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ShutdownDrainTimeout != 15*time.Second {
		t.Errorf("ShutdownDrainTimeout = %v, want 15s", cfg.ShutdownDrainTimeout)
	}
}

func TestLoadProxy_ShutdownDrainTimeout_PreferSpecific(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SHUTDOWN_DRAIN_TIMEOUT_SECONDS", "20")
	t.Setenv("DRAIN_TIMEOUT_SECONDS", "7")

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ShutdownDrainTimeout != 20*time.Second {
		t.Errorf("ShutdownDrainTimeout = %v, want 20s (specific env wins)", cfg.ShutdownDrainTimeout)
	}
	if cfg.DrainTimeout != 7*time.Second {
		t.Errorf("DrainTimeout = %v, want 7s (still parsed)", cfg.DrainTimeout)
	}
}

func TestLoadProxy_ShutdownDrainTimeout_FallbackToDrainTimeout(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DRAIN_TIMEOUT_SECONDS", "7")

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ShutdownDrainTimeout != 7*time.Second {
		t.Errorf("ShutdownDrainTimeout = %v, want 7s (fallback to DRAIN_TIMEOUT_SECONDS)", cfg.ShutdownDrainTimeout)
	}
}

func TestLoadProxy_DrainTimeout_DefaultUnchanged(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DrainTimeout != 5*time.Second {
		t.Errorf("DrainTimeout = %v, want 5s (default unchanged)", cfg.DrainTimeout)
	}
}

func TestLoadProxy_CustomPerKeyRateLimit(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PER_KEY_RATE_LIMIT", "0")

	cfg, err := LoadProxy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PerKeyRateLimit != 0 {
		t.Errorf("PerKeyRateLimit = %d, want 0", cfg.PerKeyRateLimit)
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	t.Setenv("TEST_KEY", "value")
	if v := getEnvOrDefault("TEST_KEY", "default"); v != "value" {
		t.Errorf("got %q, want value", v)
	}
	if v := getEnvOrDefault("NONEXISTENT_KEY_12345", "default"); v != "default" {
		t.Errorf("got %q, want default", v)
	}
}

func TestGetIntOrDefault(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	if v := getIntOrDefault("TEST_INT", 10); v != 42 {
		t.Errorf("got %d, want 42", v)
	}

	// Zero falls back to default (must be >= 1)
	t.Setenv("TEST_INT", "0")
	if v := getIntOrDefault("TEST_INT", 10); v != 10 {
		t.Errorf("got %d, want 10 (zero fallback)", v)
	}

	// Negative value falls back to default
	t.Setenv("TEST_INT", "-5")
	if v := getIntOrDefault("TEST_INT", 10); v != 10 {
		t.Errorf("got %d, want 10 (negative fallback)", v)
	}

	// Invalid value falls back to default
	t.Setenv("TEST_INT", "notanumber")
	if v := getIntOrDefault("TEST_INT", 10); v != 10 {
		t.Errorf("got %d, want 10 (invalid fallback)", v)
	}

	// Missing key falls back to default
	if v := getIntOrDefault("NONEXISTENT_KEY_12345", 10); v != 10 {
		t.Errorf("got %d, want 10 (missing key)", v)
	}
}

func TestGetNonNegativeIntOrDefault(t *testing.T) {
	t.Setenv("TEST_NONNEG", "0")
	if v := getNonNegativeIntOrDefault("TEST_NONNEG", 5); v != 0 {
		t.Errorf("got %d, want 0", v)
	}

	t.Setenv("TEST_NONNEG", "-1")
	if v := getNonNegativeIntOrDefault("TEST_NONNEG", 5); v != 5 {
		t.Errorf("got %d, want 5 (negative fallback)", v)
	}

	t.Setenv("TEST_NONNEG", "abc")
	if v := getNonNegativeIntOrDefault("TEST_NONNEG", 5); v != 5 {
		t.Errorf("got %d, want 5 (invalid fallback)", v)
	}
}
