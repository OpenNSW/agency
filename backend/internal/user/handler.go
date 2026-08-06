package user

import (
	"errors"
	"net/http"

	"github.com/OpenNSW/core/httputil"
)

// ProfileHandler handles HTTP requests for user profile operations.
type ProfileHandler struct {
	service *ProfileService
}

// NewProfileHandler creates a ProfileHandler.
func NewProfileHandler(service *ProfileService) *ProfileHandler {
	return &ProfileHandler{service: service}
}

// HandleMe handles GET /api/v1/users/me
func (h *ProfileHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputil.Error(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()
	result, err := h.service.GetMe(ctx)
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) {
			httputil.Error(w, r, http.StatusUnauthorized, "Unauthenticated")
		} else {
			httputil.InternalServerError(w, r, "failed to get user profile", err)
		}
		return
	}

	httputil.JSON(w, http.StatusOK, result)
}
