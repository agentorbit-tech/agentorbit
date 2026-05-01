package handler

import (
	"net/http"
	"testing"
)

func TestAuthCookieSameSite(t *testing.T) {
	tests := []struct {
		name         string
		cookieDomain string
		cookieSecure bool
		want         http.SameSite
	}{
		{"self_host_http", "", false, http.SameSiteLaxMode},
		{"self_host_https", "", true, http.SameSiteLaxMode},
		{"cloud_https_with_domain", ".agentorbit.tech", true, http.SameSiteNoneMode},
		{"misconfigured_domain_without_https", ".agentorbit.tech", false, http.SameSiteLaxMode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &AuthHandler{cookieDomain: tt.cookieDomain, cookieSecure: tt.cookieSecure}
			if got := h.authCookieSameSite(); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
