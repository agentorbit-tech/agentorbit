package auth

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestProxyAuthCache_Stringify_Redacts(t *testing.T) {
	const raw = "sk-real-12345-abcdefg"
	r := AuthVerifyResult{
		Valid:        true,
		ProviderKey:  NewSecret(raw),
		ProviderType: "openai",
	}

	cases := []struct {
		name string
		out  string
	}{
		{"%v", fmt.Sprintf("%v", r)},
		{"%+v", fmt.Sprintf("%+v", r)},
		{"%#v", fmt.Sprintf("%#v", r)},
	}
	for _, c := range cases {
		if strings.Contains(c.out, raw) {
			t.Errorf("%s leaked raw secret: %s", c.name, c.out)
		}
		if !strings.Contains(c.out, "[redacted]") {
			t.Errorf("%s missing redacted marker: %s", c.name, c.out)
		}
	}

	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), raw) {
		t.Errorf("default JSON marshal leaked raw secret: %s", b)
	}
	if !strings.Contains(string(b), "[redacted]") {
		t.Errorf("default JSON marshal missing redacted marker: %s", b)
	}

	if got := r.ProviderKey.Reveal(); got != raw {
		t.Errorf("Reveal() = %q, want %q", got, raw)
	}
}

func TestProxyAuthSecret_UnmarshalJSON_AcceptsPlainString(t *testing.T) {
	wireBody := []byte(`{"valid":true,"provider_key":"sk-from-wire-1234567"}`)
	var r AuthVerifyResult
	if err := json.Unmarshal(wireBody, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.ProviderKey.Reveal() != "sk-from-wire-1234567" {
		t.Errorf("Reveal() = %q, want sk-from-wire-1234567", r.ProviderKey.Reveal())
	}
}
