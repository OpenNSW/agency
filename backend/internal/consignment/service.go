package consignment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/OpenNSW/agency/backend/internal/nswclient"
	"github.com/OpenNSW/agency/backend/pkg/httputil"
)

// NSWClient fetches consignment display metadata from NSW Core.
type NSWClient interface {
	// GetConsignmentAgency fetches consignment display metadata from NSW Core.
	GetConsignmentAgency(ctx context.Context, consignmentID string) (*nswclient.ConsignmentAgency, error)
}

// Service handles consignment operations against the local store.
// NSW extras (e.g. trader company name) are cached on create, not fetched on list.
type Service interface {
	// GetConsignments returns a paginated list of unique consignments with their latest status (optionally filtered by search)
	GetConsignments(ctx context.Context, search string, page, pageSize int) (*httputil.PagedResponse[Summary], error)
	CreateConsignment(ctx context.Context, id string) error
	UpdateConsignment(ctx context.Context, id string, status string) error
	GetConsignment(ctx context.Context, id string) (*ConsignmentResponse, error)
}

// ConsignmentResponse is a single consignment as returned by GetConsignment.
type ConsignmentResponse struct {
	ID            string `json:"consignmentId"`
	Status        string `json:"status"`
	TraderCompany string `json:"traderCompanyName"`
}

type service struct {
	store     *Store
	nswClient NSWClient
}

// NewService creates a new consignment Service backed by store and nswClient.
func NewService(store *Store, nswClient NSWClient) Service {
	if store == nil || nswClient == nil {
		panic("NewService: all dependencies must be non-nil")
	}
	return &service{store: store, nswClient: nswClient}
}

// GetConsignments returns a paginated list of unique consignments.
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

// CreateConsignment inserts the consignment if it does not already exist.
// When the row is new, NSW extras are fetched and stored in the same insert.
func (s *service) CreateConsignment(ctx context.Context, id string) error {
	_, err := s.store.Get(ctx, id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("failed to load consignment: %w", err)
	}

	info, err := s.fetchConsignmentExtras(ctx, id)
	if err != nil {
		return err
	}
	var extras JSONB
	if info != nil && info.TraderCompanyName != "" {
		extras = JSONB{"traderCompanyName": info.TraderCompanyName}
	}
	if err := s.store.Create(ctx, id, extras); err != nil {
		return fmt.Errorf("failed to create consignment: %w", err)
	}
	return nil
}

func (s *service) fetchConsignmentExtras(ctx context.Context, consignmentID string) (*nswclient.ConsignmentAgency, error) {
	return s.nswClient.GetConsignmentAgency(ctx, consignmentID)
}

// UpdateConsignment updates a consignment's status.
func (s *service) UpdateConsignment(ctx context.Context, id string, status string) error {
	return s.store.UpdateStatus(s.store.db.WithContext(ctx), id, status, time.Now())
}

// GetConsignment returns a single consignment by id.
func (s *service) GetConsignment(ctx context.Context, id string) (*ConsignmentResponse, error) {
	rec, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &ConsignmentResponse{
		ID:            rec.ID,
		Status:        rec.Status,
		TraderCompany: stringField(rec.NSWData, "traderCompanyName"),
	}, nil
}
