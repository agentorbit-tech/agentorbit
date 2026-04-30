package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type ProxyConfig struct {
	ProcessingURL   string        // URL of the processing service
	InternalToken   string        // X-Internal-Token shared secret
	Port            string        // port to listen on (default "8080")
	ProviderTimeout time.Duration // timeout for provider requests (default 120s)
	HMACSecret      string        // HMAC-SHA256 secret for API key digest computation
	AuthCacheTTL    time.Duration // TTL for auth cache entries (default 30s)
	SpanBufferSize         int           // buffered channel size for span dispatch (default 1000)
	SpanSendTimeout        time.Duration // timeout for individual span send to Processing (default 10s)
	DefaultAnthropicVersion string       // default anthropic-version header (default "2024-10-22")
	AllowPrivateProviderIPs bool         // allow provider URLs pointing to private IPs (default false)
	AllowPlaintextInternal  bool         // allow HTTP (non-TLS) PROCESSING_URL (default false)
	PerKeyRateLimit         int          // max requests per API key per minute (0 = disabled, default 120)
	CacheEvictInterval      time.Duration // interval for cache eviction sweep (default 60s)
	DrainTimeout            time.Duration // dispatcher internal drain fallback when caller passes 0 (default 5s); main.go uses ShutdownDrainTimeout explicitly
	SpanWorkers             int           // number of concurrent span dispatch workers (default 3)
	ShutdownHTTPTimeout     time.Duration // timeout for HTTP server graceful shutdown (default 30s)
	ShutdownDrainTimeout    time.Duration // timeout for span dispatcher drain after HTTP shutdown (default 15s)
	SentryDSN         string  // DSN for error tracking (empty disables)
	SentryEnvironment string  // event tag (default "self-host")
	SentryRelease     string  // release/version tag
	SentrySampleRate  float64 // 0.0..1.0 fraction of events to send
}

func LoadProxy() (*ProxyConfig, error) {
	var missing []string
	requireEnv := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	port := getEnvOrDefault("PROXY_PORT", getEnvOrDefault("PORT", "8080"))

	cfg := &ProxyConfig{
		ProcessingURL:   requireEnv("PROCESSING_URL"),
		InternalToken:   requireEnv("INTERNAL_TOKEN"),
		HMACSecret:      requireEnv("HMAC_SECRET"),
		Port:            port,
		ProviderTimeout: time.Duration(getIntOrDefault("PROVIDER_TIMEOUT_SECONDS", 120)) * time.Second,
		AuthCacheTTL:    time.Duration(getIntOrDefault("AUTH_CACHE_TTL_SECONDS", 30)) * time.Second,
		SpanBufferSize:         getIntOrDefault("SPAN_BUFFER_SIZE", 1000),
		SpanSendTimeout:        time.Duration(getIntOrDefault("SPAN_SEND_TIMEOUT_SECONDS", 10)) * time.Second,
		DefaultAnthropicVersion: getEnvOrDefault("DEFAULT_ANTHROPIC_VERSION", "2024-10-22"),
		AllowPrivateProviderIPs: getEnvOrDefault("ALLOW_PRIVATE_PROVIDER_IPS", "") == "true",
		AllowPlaintextInternal:  getEnvOrDefault("ALLOW_PLAINTEXT_INTERNAL", "") == "true",
		PerKeyRateLimit:         getNonNegativeIntOrDefault("PER_KEY_RATE_LIMIT", 120),
		CacheEvictInterval:      time.Duration(getIntOrDefault("CACHE_EVICT_INTERVAL_SECONDS", 60)) * time.Second,
		DrainTimeout:            time.Duration(getIntOrDefault("DRAIN_TIMEOUT_SECONDS", 5)) * time.Second,
		SpanWorkers:             getIntOrDefault("SPAN_WORKERS", 3),
		ShutdownHTTPTimeout:     time.Duration(getIntOrDefault("SHUTDOWN_HTTP_TIMEOUT_SECONDS", 30)) * time.Second,
		SentryDSN:               os.Getenv("SENTRY_DSN"),
		SentryEnvironment:       getEnvOrDefault("SENTRY_ENVIRONMENT", "self-host"),
		SentryRelease:           os.Getenv("SENTRY_RELEASE"),
		SentrySampleRate:        getFloatOrDefault("SENTRY_SAMPLE_RATE", 1.0),
	}

	// ShutdownDrainTimeout: prefer SHUTDOWN_DRAIN_TIMEOUT_SECONDS; fall back to
	// DRAIN_TIMEOUT_SECONDS if set (backwards compat); otherwise default to 15s.
	if v := os.Getenv("SHUTDOWN_DRAIN_TIMEOUT_SECONDS"); v != "" {
		cfg.ShutdownDrainTimeout = time.Duration(getIntOrDefault("SHUTDOWN_DRAIN_TIMEOUT_SECONDS", 15)) * time.Second
	} else if os.Getenv("DRAIN_TIMEOUT_SECONDS") != "" {
		cfg.ShutdownDrainTimeout = cfg.DrainTimeout
	} else {
		cfg.ShutdownDrainTimeout = 15 * time.Second
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("required env vars not set: %s", strings.Join(missing, ", "))
	}

	// Reject placeholder secrets (anything starting with "changeme") BEFORE
	// length checks — operator should see the actionable error first.
	for _, check := range []struct{ name, value string }{
		{"HMAC_SECRET", cfg.HMACSecret},
		{"INTERNAL_TOKEN", cfg.InternalToken},
	} {
		if err := rejectPlaceholder(check.name, check.value); err != nil {
			return nil, err
		}
	}

	// Validate secret strength — weak secrets enable brute-force attacks
	if len(cfg.HMACSecret) < 32 {
		return nil, fmt.Errorf("HMAC_SECRET must be at least 32 characters (got %d)", len(cfg.HMACSecret))
	}
	if len(cfg.InternalToken) < 32 {
		return nil, fmt.Errorf("INTERNAL_TOKEN must be at least 32 characters (got %d)", len(cfg.InternalToken))
	}

	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return nil, fmt.Errorf("invalid port %q: must be a number between 1 and 65535", port)
	}

	const maxBufferSize = 100000
	if cfg.SpanBufferSize > maxBufferSize {
		slog.Warn("SPAN_BUFFER_SIZE exceeds max, capping", "value", cfg.SpanBufferSize, "max", maxBufferSize)
		cfg.SpanBufferSize = maxBufferSize
	}

	if err := ValidateInternalURL(cfg.ProcessingURL, cfg.AllowPlaintextInternal, nil); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(cfg.ProcessingURL, "https://") {
		slog.Warn("PROCESSING_URL uses plain HTTP (ALLOW_PLAINTEXT_INTERNAL=true) — provider API keys transit in cleartext")
	}

	return cfg, nil
}

// rejectPlaceholder returns a user-friendly error if the secret value still
// looks like the placeholder shipped in .env.example (any "changeme" prefix,
// case-insensitive). The check runs before length validation so the operator
// sees the most actionable diagnostic first.
func rejectPlaceholder(name, value string) error {
	if value == "" {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(value), "changeme") {
		return fmt.Errorf("%s is set to a placeholder value (starts with \"changeme\"). Generate a real secret: openssl rand -hex 32. Refusing to start.", name)
	}
	return nil
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getNonNegativeIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Warn("invalid env var value, using default", "key", key, "value", v, "default", def)
			return def
		}
		if n < 0 {
			slog.Warn("negative env var value, using default", "key", key, "value", n, "default", def)
			return def
		}
		return n
	}
	return def
}

func getFloatOrDefault(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		slog.Warn("invalid env var value, using default", "key", key, "value", v, "default", def)
		return def
	}
	if f < 0 || f > 1 {
		slog.Warn("sentry sample rate out of [0,1], using default", "key", key, "value", f, "default", def)
		return def
	}
	return f
}

func getIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Warn("invalid env var value, using default", "key", key, "value", v, "default", def)
			return def
		}
		if n < 1 {
			slog.Warn("value must be >= 1, using default", "key", key, "value", n, "default", def)
			return def
		}
		return n
	}
	return def
}
