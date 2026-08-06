package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/nsw-agency/backend/internal/version"
)

// Handler handles HTTP requests for agency portal operations
type Handler struct {
	service         Service
	MaxRequestBytes int64
}

// NewHandler creates a new agency handler instance
func NewHandler(service Service, maxRequestBytes int64) (*Handler, error) {
	if maxRequestBytes <= 0 {
		return nil, fmt.Errorf("invalid MaxRequestBytes: %d (must be greater than 0)", maxRequestBytes)
	}
	return &Handler{
		service:         service,
		MaxRequestBytes: maxRequestBytes,
	}, nil
}

// parseTaskID extracts the taskId from the request path.
func (h *Handler) parseTaskID(w http.ResponseWriter, r *http.Request) (string, error) {
	taskIDStr := r.PathValue("taskId")
	if taskIDStr == "" {
		httputil.Error(w, r, http.StatusBadRequest, "taskId is required")
		return "", errors.New("taskId is required")
	}
	return taskIDStr, nil
}

// HandleInjectData handles POST /api/v1/inject
// This is the endpoint that external services use to inject data into agency portal
func (h *Handler) HandleInjectData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.Error(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	r.Body = http.MaxBytesReader(w, r.Body, h.MaxRequestBytes)

	var req InjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			httputil.Error(w, r, http.StatusRequestEntityTooLarge, "Request body too large")
			return
		}
		httputil.Error(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Create application in database
	if err := h.service.CreateApplication(ctx, &req); err != nil {
		httputil.InternalServerError(w, r, "failed to create application", err)
		return
	}

	slog.InfoContext(ctx, "data injected successfully",
		"taskID", req.TaskID,
		"consignmentID", req.ConsignmentID)

	httputil.JSON(w, http.StatusCreated, map[string]any{
		"success": true,
		"message": "Data injected successfully",
		"taskId":  req.TaskID,
	})
}

// HandleGetApplications handles GET /api/v1/applications
// Returns all applications, optionally filtered by status, consignmentId, or q query parameter
func (h *Handler) HandleGetApplications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputil.Error(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()
	status := r.URL.Query().Get("status")
	consignmentID := r.URL.Query().Get("consignmentId")
	search := r.URL.Query().Get("q")

	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil && r.URL.Query().Get("page") != "" {
		httputil.Error(w, r, http.StatusBadRequest, "Invalid page number")
		return
	}
	pageSize, err := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if err != nil && r.URL.Query().Get("pageSize") != "" {
		httputil.Error(w, r, http.StatusBadRequest, "Invalid page size")
		return
	}

	result, err := h.service.GetApplications(ctx, status, consignmentID, search, page, pageSize)
	if err != nil {
		httputil.InternalServerError(w, r, "failed to get applications", err)
		return
	}

	httputil.JSON(w, http.StatusOK, result)
}

// HandleGetApplication handles GET /api/v1/applications/{taskId}
// Returns a specific application by task ID
func (h *Handler) HandleGetApplication(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputil.Error(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	taskID, err := h.parseTaskID(w, r)
	if err != nil {
		return
	}

	ctx := r.Context()
	application, err := h.service.GetApplication(ctx, taskID)
	if err != nil {
		if errors.Is(err, ErrApplicationNotFound) {
			httputil.Error(w, r, http.StatusNotFound, "Application not found")
		} else {
			httputil.InternalServerError(w, r, "failed to get application", err, "taskID", taskID)
		}
		return
	}

	httputil.JSON(w, http.StatusOK, application)
}

// HandleHealth handles GET /health
// Simple health check endpoint
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.JSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "nsw-agency-portal",
		"version": version.Get(),
	})
}

// HandleReviewApplication handles POST /api/v1/applications/{taskId}/review
// Called when Agency officer approves/rejects an application
// Sends the response back to the originating service
func (h *Handler) HandleReviewApplication(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.Error(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	taskID, err := h.parseTaskID(w, r)
	if err != nil {
		return
	}

	ctx := r.Context()

	r.Body = http.MaxBytesReader(w, r.Body, h.MaxRequestBytes)

	// Parse request body
	var requestBody map[string]any

	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			httputil.Error(w, r, http.StatusRequestEntityTooLarge, "Request body too large")
			return
		}
		httputil.Error(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Process review and send response to service
	if err := h.service.ReviewApplication(ctx, taskID, requestBody); err != nil {
		if errors.Is(err, ErrApplicationNotFound) {
			httputil.Error(w, r, http.StatusNotFound, "Application not found")
		} else {
			httputil.InternalServerError(w, r, "failed to review application", err, "taskID", taskID)
		}
		return
	}

	slog.InfoContext(ctx, "application reviewed",
		"taskID", taskID,
	)

	httputil.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Application reviewed successfully",
	})
}
