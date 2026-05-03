package handler

import (
	"net/http"
)

// NewMetaHandler returns an http.HandlerFunc serving deployment metadata.
// Publicly accessible — exposes deployment-level public info for the UI.
//
// `billingURL` is non-empty in cloud (where the billing service is reachable)
// and empty in self-host. When empty, the field is omitted from the JSON
// response so the client can use a simple truthy check.
//
// `proxyURL` is the public base URL of the proxy service (e.g.
// "https://api.agentorbit.tech"). When empty (typical for self-host where the
// UI and proxy share an origin) the client falls back to window.location.origin.
func NewMetaHandler(appVersion, billingURL, proxyURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := map[string]string{"version": appVersion}
		if billingURL != "" {
			body["billing_url"] = billingURL
		}
		if proxyURL != "" {
			body["proxy_url"] = proxyURL
		}
		WriteJSON(w, http.StatusOK, body)
	}
}
