package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetaHandler_ReturnsBillingURL_WhenSet(t *testing.T) {
	h := NewMetaHandler("0.1.2", "https://billing.agentorbit.tech")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/meta", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["version"] != "0.1.2" {
		t.Fatalf("version: %v", got["version"])
	}
	if got["billing_url"] != "https://billing.agentorbit.tech" {
		t.Fatalf("billing_url: %v", got["billing_url"])
	}
}

func TestMetaHandler_OmitsBillingURL_WhenEmpty(t *testing.T) {
	h := NewMetaHandler("0.1.2", "")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/meta", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := got["billing_url"]; ok {
		// We require the field to be omitted (not null) so self-host clients
		// can use a simple `if (meta.billing_url)` check.
		t.Fatalf("billing_url should be omitted when empty")
	}
}
