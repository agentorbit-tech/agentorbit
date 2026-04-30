package config

import (
	"strings"
	"testing"
)

func TestLoadProxy_RejectsChangemePrefix(t *testing.T) {
	cases := []string{"INTERNAL_TOKEN", "HMAC_SECRET"}
	for _, key := range cases {
		t.Run(key, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(key, "changeme_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
			_, err := LoadProxy()
			if err == nil {
				t.Fatalf("expected error for placeholder %s", key)
			}
			if !strings.Contains(err.Error(), "placeholder") {
				t.Errorf("error should mention 'placeholder', got: %v", err)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error should mention %s, got: %v", key, err)
			}
		})
	}
}

func TestLoadProxy_RejectsChangemePrefix_CaseInsensitive(t *testing.T) {
	for _, val := range []string{
		"CHANGEME_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"Changeme_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	} {
		t.Run(val, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("INTERNAL_TOKEN", val)
			_, err := LoadProxy()
			if err == nil || !strings.Contains(err.Error(), "placeholder") {
				t.Errorf("expected placeholder error for %q, got %v", val, err)
			}
		})
	}
}
