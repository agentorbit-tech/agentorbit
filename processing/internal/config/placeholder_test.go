package config

import (
	"strings"
	"testing"
)

func TestLoadProcessing_RejectsChangemePrefix(t *testing.T) {
	cases := []struct {
		envKey string
	}{
		{"JWT_SECRET"},
		{"HMAC_SECRET"},
		{"INTERNAL_TOKEN"},
		// ENCRYPTION_KEY tested separately; placeholder check fires before length/hex checks.
		{"ENCRYPTION_KEY"},
	}
	for _, c := range cases {
		t.Run(c.envKey, func(t *testing.T) {
			setRequiredEnv(t)
			// 64-char placeholder so length-only checks would otherwise pass.
			t.Setenv(c.envKey, "changeme_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")

			_, err := LoadProcessing()
			if err == nil {
				t.Fatalf("expected error for placeholder %s", c.envKey)
			}
			if !strings.Contains(err.Error(), "placeholder") {
				t.Errorf("error should mention 'placeholder', got: %v", err)
			}
			if !strings.Contains(err.Error(), c.envKey) {
				t.Errorf("error should mention %s, got: %v", c.envKey, err)
			}
		})
	}
}

func TestLoadProcessing_RejectsChangemePrefix_CaseInsensitive(t *testing.T) {
	cases := []string{
		"CHANGEME_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"Changeme_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"chAnGeMe_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	}
	for _, val := range cases {
		t.Run(val, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("JWT_SECRET", val)
			_, err := LoadProcessing()
			if err == nil || !strings.Contains(err.Error(), "placeholder") {
				t.Errorf("expected placeholder error for %q, got %v", val, err)
			}
		})
	}
}

func TestLoadProcessing_AcceptsRealSecret(t *testing.T) {
	setRequiredEnv(t)
	// 64 hex chars is a real secret per `openssl rand -hex 32`.
	t.Setenv("JWT_SECRET", "f0e1d2c3b4a5968778695a4b3c2d1e0ff0e1d2c3b4a5968778695a4b3c2d1e0f")

	cfg, err := LoadProcessing()
	if err != nil {
		t.Fatalf("expected success with real secret, got %v", err)
	}
	if cfg == nil {
		t.Fatal("cfg is nil")
	}
}
