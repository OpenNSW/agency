package consignment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound is returned when a consignment does not exist in the agency store.
var ErrNotFound = errors.New("consignment not found")

// JSONB is an in-memory map used when reading or merging NSW extras.
type JSONB map[string]any

// ConsignmentRecord represents a consignment (workflow) in the Agency database.
// Each consignment groups one or more application records.
type ConsignmentRecord struct {
	ID        string          `gorm:"type:text;primaryKey"`
	Status    string          `gorm:"type:varchar(50);not null;default:'PENDING'"`
	NSWData   json.RawMessage `gorm:"type:jsonb"`
	CreatedAt time.Time       `gorm:"autoCreateTime"`
	UpdatedAt time.Time       `gorm:"autoUpdateTime"`
}

// TableName returns the table name for ConsignmentRecord
func (ConsignmentRecord) TableName() string {
	return "consignments"
}

// Summary represents a unique consignment with its most recent activity.
type Summary struct {
	ConsignmentID     string    `json:"consignmentId"`
	TraderCompanyName string    `json:"traderCompanyName,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt"`
	Status            string    `json:"status"`    // Status of the most recent application
	TaskCount         int       `json:"taskCount"` // Total number of applications in this consignment
}

// Store handles database operations for consignments.
type Store struct {
	db *gorm.DB
}

// NewConsignmentStore creates a new ConsignmentStore backed by db.
func NewConsignmentStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// GetData returns the cached NSW extras for a consignment.
// ErrNotFound means no row exists yet; a nil JSONB means the row exists but
// extras have not been filled in (the NSW fetch failed or has not completed).
func (s *Store) GetData(ctx context.Context, id string) (JSONB, error) {
	var rec ConsignmentRecord
	if err := s.db.WithContext(ctx).Select("nsw_data").First(&rec, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return decodeNSWData(rec.NSWData)
}

// MergeData copies extra keys into consignments.nsw_data. Existing keys are
// overwritten.
func (s *Store) MergeData(ctx context.Context, id string, extra JSONB) error {
	if len(extra) == 0 {
		return nil
	}
	current, err := s.GetData(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		current = JSONB{}
	}
	changed := false
	for k, v := range extra {
		if current[k] == v {
			continue
		}
		current[k] = v
		changed = true
	}
	if !changed {
		return nil
	}
	raw, err := encodeNSWData(current)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&ConsignmentRecord{}).
		Where("id = ?", id).
		Update("nsw_data", raw).Error
}

// Upsert upserts a consignment record using the given connection or
// transaction. Callers that also write a child application record (e.g.
// internal/application) should pass a transaction so the FK reference exists
// atomically.
//
// Only status and updated_at are touched on conflict — created_at and nsw_data
// must survive every later upsert of an already-existing consignment (e.g. when a
// second application is injected into the same consignment).
func (s *Store) Upsert(tx *gorm.DB, id, status string) error {
	return s.UpsertWithData(tx, id, status, nil)
}

// UpsertWithData upserts a consignment record along with its NSW metadata.
func (s *Store) UpsertWithData(tx *gorm.DB, id, status string, data JSONB) error {
	raw, err := encodeNSWData(data)
	if err != nil {
		return err
	}
	record := &ConsignmentRecord{
		ID:      id,
		Status:  status,
		NSWData: raw,
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "updated_at"}),
	}).Create(record).Error
}

// UpdateStatus updates a consignment's status using the given connection or transaction.
func (s *Store) UpdateStatus(tx *gorm.DB, id, status string, updatedAt time.Time) error {
	return tx.Model(&ConsignmentRecord{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     status,
			"updated_at": updatedAt,
		}).Error
}

type summaryRow struct {
	ConsignmentID string          `gorm:"column:consignment_id"`
	Status        string          `gorm:"column:status"`
	UpdatedAt     time.Time       `gorm:"column:updated_at"`
	NSWData       json.RawMessage `gorm:"column:nsw_data;type:jsonb"`
	TaskCount     int             `gorm:"column:task_count"`
}

// List returns a paginated list of consignments with task count and optional search.
func (s *Store) List(ctx context.Context, search string, offset, limit int) ([]Summary, int64, error) {
	var total int64

	countQ := s.db.WithContext(ctx).Model(&ConsignmentRecord{})
	if search != "" {
		countQ = countQ.Where("id LIKE ?", "%"+search+"%")
	}
	if err := countQ.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	dataQ := s.db.WithContext(ctx).Model(&ConsignmentRecord{}).
		Select("consignments.id AS consignment_id, consignments.status, consignments.updated_at, consignments.nsw_data, COUNT(applications.task_id) AS task_count").
		Joins("LEFT JOIN applications ON applications.consignment_id = consignments.id").
		Group("consignments.id, consignments.status, consignments.updated_at, consignments.nsw_data").
		Order("consignments.updated_at DESC").
		Offset(offset).
		Limit(limit)

	if search != "" {
		dataQ = dataQ.Where("consignments.id LIKE ?", "%"+search+"%")
	}

	var rows []summaryRow
	if err := dataQ.Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	summaries := make([]Summary, len(rows))
	for i, r := range rows {
		data, err := decodeNSWData(r.NSWData)
		if err != nil {
			return nil, 0, fmt.Errorf("decode nsw_data for %s: %w", r.ConsignmentID, err)
		}
		summaries[i] = summaryFrom(r.ConsignmentID, r.Status, r.UpdatedAt, data, r.TaskCount)
	}

	return summaries, total, nil
}

func summaryFrom(id, status string, updatedAt time.Time, data JSONB, taskCount int) Summary {
	return Summary{
		ConsignmentID:     id,
		TraderCompanyName: stringField(data, "traderCompanyName"),
		Status:            status,
		UpdatedAt:         updatedAt,
		TaskCount:         taskCount,
	}
}

func stringField(data JSONB, keys ...string) string {
	if data == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := data[k].(string); ok {
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func decodeNSWData(raw json.RawMessage) (JSONB, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out JSONB
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func encodeNSWData(data JSONB) (json.RawMessage, error) {
	if data == nil {
		return nil, nil
	}
	return json.Marshal(data)
}
