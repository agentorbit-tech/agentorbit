package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/go-chi/chi/v5"
)

// InternalHandler handles requests from the Proxy on the Internal API.
type InternalHandler struct {
	internalService *service.InternalService
}

// NewInternalHandler creates a new InternalHandler.
func NewInternalHandler(internalService *service.InternalService) *InternalHandler {
	return &InternalHandler{internalService: internalService}
}

// Routes returns a chi.Router with the internal API endpoints.
// X-Internal-Token auth is applied at the mount point in main.go.
//
// Deprecated: kept for backwards compatibility with tests that mounted
// the whole handler. main.go now wires Verify and Ingest individually
// so per-route middleware (rate limit, DB pool breaker) only applies
// where it belongs.
func (h *InternalHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/auth/verify", h.Verify)
	r.Post("/spans/ingest", h.Ingest)

	// Debug endpoints — only available when built with -tags pprof (D-09).
	registerPprof(r)

	return r
}

// PprofRoutes returns just the pprof routes (a no-op router unless built
// with -tags pprof). main.go uses this instead of Routes() so that
// per-route middleware can wrap Verify and Ingest individually.
func (h *InternalHandler) PprofRoutes() chi.Router {
	r := chi.NewRouter()
	registerPprof(r)
	return r
}

// verifyRequest is the body for POST /internal/auth/verify.
type verifyRequest struct {
	KeyDigest string `json:"key_digest"`
}

// authVerifyWireResult is the on-the-wire envelope for /internal/auth/verify.
// It mirrors service.AuthVerifyResult but renders ProviderKey as a plain
// string so the proxy receives the real value. This is the ONE place
// allowed to bypass the Secret redaction — see service.Secret docs.
type authVerifyWireResult struct {
	Valid              bool   `json:"valid"`
	Reason             string `json:"reason,omitempty"`
	APIKeyID           string `json:"api_key_id,omitempty"`
	OrganizationID     string `json:"organization_id,omitempty"`
	ProviderType       string `json:"provider_type,omitempty"`
	ProviderKey        string `json:"provider_key,omitempty"`
	BaseURL            string `json:"base_url,omitempty"`
	OrganizationStatus string `json:"organization_status,omitempty"`
	StoreSpanContent   bool   `json:"store_span_content"`
	MaskingConfig      json.RawMessage `json:"masking_config,omitempty"`
}

func toWire(r *service.AuthVerifyResult) authVerifyWireResult {
	if r == nil {
		return authVerifyWireResult{}
	}
	return authVerifyWireResult{
		Valid:              r.Valid,
		Reason:             r.Reason,
		APIKeyID:           r.APIKeyID,
		OrganizationID:     r.OrganizationID,
		ProviderType:       r.ProviderType,
		ProviderKey:        r.ProviderKey.Reveal(),
		BaseURL:            r.BaseURL,
		OrganizationStatus: r.OrganizationStatus,
		StoreSpanContent:   r.StoreSpanContent,
		MaskingConfig:      r.MaskingConfig,
	}
}

// Verify handles POST /internal/auth/verify.
// Returns 200 with AuthVerifyResult regardless of key validity — the Proxy
// interprets the valid field. Errors in DB/decryption still return 5xx.
func (h *InternalHandler) Verify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.KeyDigest == "" {
		WriteJSON(w, http.StatusOK, toWire(&service.AuthVerifyResult{Valid: false, Reason: "invalid_key"}))
		return
	}

	result, err := h.internalService.VerifyAPIKey(r.Context(), req.KeyDigest)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify API key")
		return
	}

	WriteJSON(w, http.StatusOK, toWire(result))
}

// Ingest handles POST /internal/spans/ingest.
// Returns 202 Accepted when the span is accepted for storage (stub in Phase 2).
// Returns 429 when the free-plan 3000 spans/month limit is exceeded (ORG-12).
func (h *InternalHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	var req service.SpanIngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.internalService.IngestSpan(r.Context(), &req); err != nil {
		var quotaErr *service.SpanQuotaExceededError
		if errors.As(err, &quotaErr) {
			WriteError(w, http.StatusTooManyRequests, "span_quota_exceeded", "Free plan limit of 3000 spans/month reached")
			return
		}
		var svcErr *service.ServiceError
		if errors.As(err, &svcErr) {
			WriteError(w, svcErr.Status, svcErr.Code, svcErr.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to ingest span")
		return
	}

	WriteJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}
