package storage

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/OpenNSW/core/httputil"
)

// Handler handles HTTP requests for storage operations
type Handler struct {
	service         Service
	MaxRequestBytes int64
}

// NewHandler creates a new storage handler instance
func NewHandler(service Service, maxRequestBytes int64) (*Handler, error) {
	if maxRequestBytes <= 0 {
		return nil, fmt.Errorf("invalid MaxRequestBytes: %d (must be greater than 0)", maxRequestBytes)
	}
	return &Handler{
		service:         service,
		MaxRequestBytes: maxRequestBytes,
	}, nil
}

// HandleGetUploadURL returns a download URL for a file stored in the main backend.
func (h *Handler) HandleGetUploadURL(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		httputil.Error(w, r, http.StatusBadRequest, "key is required")
		return
	}
	metadata, err := h.service.GetDownloadURL(r.Context(), key)
	if err != nil {
		httputil.InternalServerError(w, r, "failed to get download URL from backend", err, "key", key)
		return
	}

	httputil.JSON(w, http.StatusOK, metadata)
}

// HandleCreateUpload prepares an upload by requesting an upload URL from the main backend.
func (h *Handler) HandleCreateUpload(w http.ResponseWriter, r *http.Request) {
	// TODO: Add Authentication & Authorization middleware
	// Access must be restricted to authorized Agency officers to prevent unauthorized users
	// from generating proxy pre-signed upload URLs. Introduce a configuration flag (e.g. AGENCY_DISABLE_AUTH)
	// to make bypassing explicit for specific environments

	r.Body = http.MaxBytesReader(w, r.Body, h.MaxRequestBytes)
	var req UploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	result, err := h.service.CreateUploadURL(r.Context(), req)
	if err != nil {
		httputil.InternalServerError(w, r, "failed to create upload URL", err)
		return
	}

	httputil.JSON(w, http.StatusOK, result)
}
