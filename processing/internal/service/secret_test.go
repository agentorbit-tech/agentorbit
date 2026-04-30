package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAuthVerifyResult_Stringify_Redacts(t *testing.T) {
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
