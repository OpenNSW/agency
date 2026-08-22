package certificate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/httputil"
)

// GenerateRequest is the payload sent by the frontend to populate a certificate template.
type GenerateRequest struct {
	TemplateID string `json:"templateId"`
	// ConsignmentID is optional. When set, fields derivable from the
	// consignment's application history are auto-filled; Data still wins on
	// any key both provide.
	ConsignmentID string         `json:"consignmentId,omitempty"`
	Data          map[string]any `json:"data"`
}

// Handler handles HTTP requests for certificate generation.
type Handler struct {
	service         Service
	MaxRequestBytes int64
}

// NewHandler creates a new certificate handler instance.
func NewHandler(service Service, maxRequestBytes int64) (*Handler, error) {
	if maxRequestBytes <= 0 {
		return nil, fmt.Errorf("invalid MaxRequestBytes: %d (must be greater than 0)", maxRequestBytes)
	}
	return &Handler{
		service:         service,
		MaxRequestBytes: maxRequestBytes,
	}, nil
}

// HandleGenerate populates the requested certificate template with the given data
// and returns the resulting HTML for the frontend to render/print/convert to PDF.
func (h *Handler) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.MaxRequestBytes)
	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.TemplateID == "" {
		httputil.Error(w, r, http.StatusBadRequest, "templateId is required")
		return
	}

	html, err := h.service.Generate(r.Context(), req.TemplateID, req.ConsignmentID, req.Data)
	if err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			httputil.Error(w, r, http.StatusNotFound, "Certificate template not found")
			return
		}
		httputil.InternalServerError(w, r, "failed to generate certificate", err, "templateId", req.TemplateID)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}
