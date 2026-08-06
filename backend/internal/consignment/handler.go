package consignment

import (
	"net/http"
	"strconv"

	"github.com/OpenNSW/core/httputil"
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
		httputil.Error(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()
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

	result, err := h.service.GetConsignments(ctx, search, page, pageSize)
	if err != nil {
		httputil.InternalServerError(w, r, "failed to get consignments", err)
		return
	}

	httputil.JSON(w, http.StatusOK, result)
}
