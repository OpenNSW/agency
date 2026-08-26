package certificate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/OpenNSW/agency/backend/internal/application"
	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/httputil"
)

// GenerateRequest is the payload sent by the frontend to populate a certificate template.
// The template and the consignment used to auto-fill fields are resolved
// server-side from the task identified in the URL, not from client input.
type GenerateRequest struct {
	Data map[string]any `json:"data"`
}

// Handler handles HTTP requests for certificate generation.
type Handler struct {
	service         Service
	applications    ApplicationLookup
	MaxRequestBytes int64
}

// NewHandler creates a new certificate handler instance. applications is used
// to resolve the caller-supplied taskId to its configured certificate
// template, data schema, and consignment before rendering.
func NewHandler(service Service, applications ApplicationLookup, maxRequestBytes int64) (*Handler, error) {
	if maxRequestBytes <= 0 {
		return nil, fmt.Errorf("invalid MaxRequestBytes: %d (must be greater than 0)", maxRequestBytes)
	}
	return &Handler{
		service:         service,
		applications:    applications,
		MaxRequestBytes: maxRequestBytes,
	}, nil
}

// HandleGenerate populates the certificate template configured for the task
// identified by {taskId} with the given data and returns the resulting HTML
// for the frontend to render/print/convert to PDF. The route this is mounted
// on must enforce the REVIEW action for the task, so a caller reaching this
// handler is already authorized to review it; the template, its data schema,
// and the consignment used to auto-fill fields all come from that task's own
// configuration, never from the request body.
func (h *Handler) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskId")
	if taskID == "" {
		httputil.Error(w, r, http.StatusBadRequest, "taskId is required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.MaxRequestBytes)
	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	app, err := h.applications.GetApplication(r.Context(), taskID)
	if err != nil {
		if errors.Is(err, application.ErrApplicationNotFound) {
			httputil.Error(w, r, http.StatusNotFound, "Application not found")
			return
		}
		httputil.InternalServerError(w, r, "failed to load application for certificate generation", err, "taskId", taskID)
		return
	}
	if app.CertificateTemplateID == "" {
		httputil.Error(w, r, http.StatusBadRequest, "no certificate is configured for this task")
		return
	}

	if err := validateAgainstSchema(app.CertificateDataSchema, req.Data); err != nil {
		httputil.Error(w, r, http.StatusBadRequest, "certificate data does not match the configured schema: "+err.Error())
		return
	}

	html, err := h.service.Generate(r.Context(), app.CertificateTemplateID, app.ConsignmentID, req.Data)
	if err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			httputil.Error(w, r, http.StatusNotFound, "Certificate template not found")
			return
		}
		httputil.InternalServerError(w, r, "failed to generate certificate", err, "templateId", app.CertificateTemplateID)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}
