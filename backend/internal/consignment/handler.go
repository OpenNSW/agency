package consignment

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/OpenNSW/nsw-agency/backend/pkg/httputil"
)

// Handler handles HTTP requests for consignment operations
type Handler struct {
	service Service
}

// NewHandler creates a new consignment handler instance
func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// HandleGetConsignments handles GET /api/v1/consignments
// Returns a paginated list of unique consignments with their latest status, optionally filtered by q
func (h *Handler) HandleGetConsignments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputil.WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()
	search := r.URL.Query().Get("q")

	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil && r.URL.Query().Get("page") != "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "Invalid page number")
		return
	}
	pageSize, err := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if err != nil && r.URL.Query().Get("pageSize") != "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "Invalid page size")
		return
	}

	result, err := h.service.GetConsignments(ctx, search, page, pageSize)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get consignments", "error", err)
		httputil.WriteJSONError(w, http.StatusInternalServerError, "Failed to get consignments")
		return
	}

	httputil.WriteJSONResponse(w, http.StatusOK, result)
}
