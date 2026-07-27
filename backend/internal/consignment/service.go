package consignment

import (
	"context"

	"github.com/OpenNSW/nsw-agency/backend/pkg/httputil"
)

// Service handles consignment read operations.
type Service interface {
	// GetConsignments returns a paginated list of unique consignments with their latest status (optionally filtered by search)
	GetConsignments(ctx context.Context, search string, page, pageSize int) (*httputil.PagedResponse[Summary], error)
}

type service struct {
	store *Store
}

// NewService creates a new consignment Service backed by store.
func NewService(store *Store) Service {
	if store == nil {
		panic("NewService: store must be non-nil")
	}
	return &service{store: store}
}

// GetConsignments returns a paginated list of unique consignments
func (s *service) GetConsignments(ctx context.Context, search string, page, pageSize int) (*httputil.PagedResponse[Summary], error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize
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
