package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/OpenNSW/core/httputil"
)

// Service is a narrow interface for feedback operations, avoiding a circular
// import with the parent internal package.
type Service interface {
	FeedbackApplication(ctx context.Context, taskID string, content map[string]any) error
}

type Handler struct {
	service         Service
	MaxRequestBytes int64
}

func NewHandler(service Service, maxRequestBytes int64) (*Handler, error) {
	if maxRequestBytes <= 0 {
		return nil, fmt.Errorf("invalid MaxRequestBytes: %d (must be greater than 0)", maxRequestBytes)
	}
	return &Handler{
		service:         service,
		MaxRequestBytes: maxRequestBytes,
	}, nil
}

func (h *Handler) HandleFeedback(w http.ResponseWriter, r *http.Request) {
	taskIDStr := r.PathValue("taskId")
	if strings.TrimSpace(taskIDStr) == "" {
		httputil.Error(w, r, http.StatusBadRequest, "taskId is required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.MaxRequestBytes)

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			httputil.Error(w, r, http.StatusRequestEntityTooLarge, "Request body too large")
			return
		}
		httputil.Error(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	feedback, ok := body["feedback"].(string)

	if !ok || strings.TrimSpace(feedback) == "" {
		httputil.Error(w, r, http.StatusBadRequest, "feedback field is required and must be a non-empty string")
		return
	}

	if err := h.service.FeedbackApplication(r.Context(), taskIDStr, body); err != nil {
		httputil.InternalServerError(w, r, "failed to send feedback", err, "taskID", taskIDStr)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Feedback sent successfully",
	})
}
