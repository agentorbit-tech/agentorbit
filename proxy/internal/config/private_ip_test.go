package config

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func stubLookup(m map[string][]net.IPAddr) LookupFunc {
	return func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if v, ok := m[host]; ok {
			return v, nil
		}
		return nil, errors.New("stub: no record for " + host)
	}
}

func ipAddr(s string) net.IPAddr {
	return net.IPAddr{IP: net.ParseIP(s)}
}

func TestValidateInternalURL_HTTPSAccepted(t *testing.T) {
	if err := ValidateInternalURL("https://anything.example.com", false, nil); err != nil {
		t.Errorf("https should pass without lookup: %v", err)
	}
	if err := ValidateInternalURL("https://1.1.1.1", false, nil); err != nil {
		t.Errorf("https with public IP should still pass: %v", err)
	}
}

func TestValidateInternalURL_HTTPRejectedWithoutFlag(t *testing.T) {
	err := ValidateInternalURL("http://processing:8081", false, nil)
	if err == nil {
		t.Fatal("expected error for http without ALLOW_PLAINTEXT_INTERNAL")
	}
	if !strings.Contains(err.Error(), "ALLOW_PLAINTEXT_INTERNAL") {
		t.Errorf("error should hint at the flag, got %v", err)
	}
}

func TestValidateInternalURL_LoopbackAccepted(t *testing.T) {
	cases := []struct {
		name string
		url  string
		stub LookupFunc
	}{
		{"127.0.0.1", "http://127.0.0.1:8081", nil},
		{"::1", "http://[::1]:8081", nil},
		{"localhost", "http://localhost:8081", stubLookup(map[string][]net.IPAddr{
			"localhost": {ipAddr("127.0.0.1")},
		})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidateInternalURL(c.url, true, c.stub); err != nil {
				t.Errorf("expected accept, got %v", err)
			}
		})
	}
}

func TestValidateInternalURL_RFC1918Accepted(t *testing.T) {
	for _, ip := range []string{"10.0.0.5", "172.20.0.3", "192.168.1.10"} {
		if err := ValidateInternalURL("http://"+ip+":8081", true, nil); err != nil {
			t.Errorf("expected accept for %s, got %v", ip, err)
		}
	}
}

func TestValidateInternalURL_PublicIPRejected(t *testing.T) {
	for _, ip := range []string{"8.8.8.8", "1.1.1.1"} {
		err := ValidateInternalURL("http://"+ip+":8081", true, nil)
		if err == nil {
			t.Errorf("expected reject for public %s", ip)
			continue
		}
		if !strings.Contains(err.Error(), ip) {
			t.Errorf("error for %s should mention the IP, got: %v", ip, err)
		}
	}
}

func TestValidateInternalURL_DockerHostnameAccepted(t *testing.T) {
	stub := stubLookup(map[string][]net.IPAddr{
		"processing": {ipAddr("172.18.0.5")},
	})
	if err := ValidateInternalURL("http://processing:8081", true, stub); err != nil {
		t.Errorf("expected accept, got %v", err)
	}
}

func TestValidateInternalURL_MixedResolutionRejected(t *testing.T) {
	stub := stubLookup(map[string][]net.IPAddr{
		"sneaky": {ipAddr("10.0.0.1"), ipAddr("8.8.8.8")},
	})
	err := ValidateInternalURL("http://sneaky:8081", true, stub)
	if err == nil {
		t.Fatal("expected reject when any resolved IP is public")
	}
	if !strings.Contains(err.Error(), "8.8.8.8") {
		t.Errorf("error should mention the public IP, got: %v", err)
	}
}

func TestValidateInternalURL_InvalidScheme(t *testing.T) {
	err := ValidateInternalURL("ftp://x:8081", true, nil)
	if err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Errorf("expected scheme error, got %v", err)
	}
}

func TestLoadProxy_HTTPPublicIPWithFlagFails(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PROCESSING_URL", "http://1.1.1.1:8081")
	t.Setenv("ALLOW_PLAINTEXT_INTERNAL", "true")
	_, err := LoadProxy()
	if err == nil {
		t.Fatal("expected reject for http to public IP even with flag")
	}
}

func TestIsPrivate_Coverage(t *testing.T) {
	cases := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"10.0.0.5", true},
		{"172.20.0.3", true},
		{"192.168.1.10", true},
		{"169.254.0.1", true},
		{"100.64.0.1", true},
		{"fc00::1", true},
		{"fe80::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, c := range cases {
		got := IsPrivate(net.ParseIP(c.ip))
		if got != c.private {
			t.Errorf("IsPrivate(%s) = %v, want %v", c.ip, got, c.private)
		}
	}
}
