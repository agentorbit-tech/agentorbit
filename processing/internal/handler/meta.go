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
func NewMetaHandler(appVersion, billingURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := map[string]string{"version": appVersion}
		if billingURL != "" {
			body["billing_url"] = billingURL
		}
		WriteJSON(w, http.StatusOK, body)
	}
}
