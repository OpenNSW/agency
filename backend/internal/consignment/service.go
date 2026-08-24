package consignment

import (
	"context"
	"log/slog"
	"strings"

	"github.com/OpenNSW/agency/backend/internal/nswclient"
	"github.com/OpenNSW/agency/backend/pkg/httputil"
)

// NSWClient fetches allowlisted consignment display names from NSW Core.
// It is satisfied by *nswclient.Client.
type NSWClient interface {
	GetConsignmentAgency(ctx context.Context, consignmentID string) (*nswclient.ConsignmentAgency, error)
}

// Service handles consignment read operations.
type Service interface {
	// GetConsignments returns a paginated list of unique consignments with their latest status (optionally filtered by search)
	GetConsignments(ctx context.Context, search string, page, pageSize int) (*httputil.PagedResponse[Summary], error)
	// GetConsignment returns a single consignment summary, with Core display names when available.
	GetConsignment(ctx context.Context, consignmentID string) (*Summary, error)
}

type service struct {
	store *Store
	nsw   NSWClient
}

// NewService creates a new consignment Service backed by store.
// nsw may be nil; GetConsignment then omits display names.
func NewService(store *Store, nsw NSWClient) Service {
	if store == nil {
		panic("NewService: store must be non-nil")
	}
	return &service{store: store, nsw: nsw}
}

// GetConsignments returns a paginated list of unique consignments
func (s *service) GetConsignments(ctx context.Context, search string, page, pageSize int) (*httputil.PagedResponse[Summary], error) {
	page, pageSize, offset := httputil.NormalizePage(page, pageSize)
	summaries, total, err := s.store.List(ctx, search, offset, pageSize)
	if err != nil {
		return nil, err
	}

	return &httputil.PagedResponse[Summary]{
		Items:    summaries,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetConsignment returns one consignment by ID. A Core lookup failure leaves names empty.
func (s *service) GetConsignment(ctx context.Context, consignmentID string) (*Summary, error) {
	summary, err := s.store.Get(ctx, consignmentID)
	if err != nil {
		return nil, err
	}
	if s.nsw == nil {
		return summary, nil
	}
	info, err := s.nsw.GetConsignmentAgency(ctx, summary.ConsignmentID)
	if err != nil {
		slog.WarnContext(ctx, "failed to fetch consignment display names from NSW",
			"consignmentId", summary.ConsignmentID, "error", err)
		return summary, nil
	}
	if info != nil {
		summary.ConsignmentName = displayName(info.ConsignmentName)
		summary.ConsigneeName = displayName(info.ConsigneeName)
	}
	return summary, nil
}

// displayName copies a Core name onto the officer DTO only when it is a real
// value. Empty strings and placeholders such as "N/A" are omitted (omitempty).
func displayName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	switch strings.ToLower(s) {
	case "n/a", "n.a.", "n.a", "na", "nil", "null", "undefined", "unknown", "-", "--", "none", "not applicable":
		return ""
	}
	return s
}
