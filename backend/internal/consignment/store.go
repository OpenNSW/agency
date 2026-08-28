package consignment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"time"

	"github.com/OpenNSW/agency/backend/pkg/dbtype"
	"github.com/OpenNSW/agency/backend/pkg/jsonschemautil"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound is returned when a consignment does not exist in the agency store.
var ErrNotFound = errors.New("consignment not found")

// JSONB is an in-memory map used when reading or storing a consignment's
// JSON columns (NSWData, CustomData). See pkg/dbtype for the Value/Scan
// implementation, shared with other domain packages.
type JSONB = dbtype.JSONB

// ConsignmentRecord represents a consignment (workflow) in the Agency database.
// Each consignment groups one or more application records.
type ConsignmentRecord struct {
	ID     string `gorm:"type:text;primaryKey"`
	Status string `gorm:"type:varchar(50);not null;default:'PENDING'"`
	// NSWData is display metadata (e.g. trader company name) fetched once
	// from NSW Core and cached — see consignment.Service.CreateConsignment.
	NSWData json.RawMessage `gorm:"type:jsonb"`
	// CustomData is agency-derived: fields pushed from injected application
	// data via each task config's ConsignmentFields (see
	// docs/consignment-custom-data.md), accumulated across every task
	// that touches this consignment. Different provenance and lifecycle
	// from NSWData, so it's a separate column, not folded into it.
	CustomData json.RawMessage `gorm:"column:custom_data;type:jsonb"`
	CreatedAt  time.Time       `gorm:"autoCreateTime"`
	UpdatedAt  time.Time       `gorm:"autoUpdateTime"`
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

// Create inserts a consignment row with optional NSW extras if one does not
// already exist. On conflict the existing row (including nsw_data) is left unchanged.
func (s *Store) Create(ctx context.Context, id string, data JSONB) error {
	raw, err := encodeJSONB(data)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&ConsignmentRecord{ID: id, Status: "PENDING", NSWData: raw}).Error
}

// Get returns a consignment by id.
func (s *Store) Get(ctx context.Context, id string) (*ConsignmentRecord, error) {
	var rec ConsignmentRecord
	if err := s.db.WithContext(ctx).First(&rec, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &rec, nil
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
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "updated_at"}),
	}).Create(&ConsignmentRecord{ID: id, Status: status}).Error
}

// MergeCustomData merges fields into the consignment's own custom_data
// document (creating it if this is the first merge). Existing keys not
// present in fields are preserved — this merges in, it doesn't replace the
// whole document; on a repeated key, fields wins. No-op when fields is
// empty, so a task with nothing to push costs no query.
//
// tx must be a transaction: this locks the consignment row for the rest of
// it (SELECT ... FOR UPDATE) so two concurrent injects into the same
// consignment can't lose an update to each other.
//
// If the merged document fails schema validation, the merge is skipped
// (left as it was) and this still returns nil rather than an error — a
// downstream enrichment feature must never be able to fail the caller's
// larger transaction (e.g. the application save it's part of). See
// docs/consignment-custom-data.md for why. A genuine query error
// still propagates normally.
func (s *Store) MergeCustomData(tx *gorm.DB, id string, fields map[string]any, schema json.RawMessage) error {
	if len(fields) == 0 {
		return nil
	}

	var rec ConsignmentRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&rec, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to lock consignment %s for custom data merge: %w", id, err)
	}

	merged, err := decodeJSONB(rec.CustomData)
	if err != nil {
		return fmt.Errorf("decode custom_data for %s: %w", id, err)
	}
	if merged == nil {
		merged = JSONB{}
	}
	maps.Copy(merged, fields)

	if err := jsonschemautil.ValidateInstance(schema, merged); err != nil {
		slog.Warn("consignment custom_data failed schema validation, skipping merge",
			"consignmentID", id, "error", err)
		return nil
	}

	encoded, err := encodeJSONB(merged)
	if err != nil {
		return fmt.Errorf("encode custom_data for %s: %w", id, err)
	}
	return tx.Model(&ConsignmentRecord{}).Where("id = ?", id).Update("custom_data", encoded).Error
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
		data, err := decodeJSONB(r.NSWData)
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

// decodeJSONB and encodeJSONB convert between a JSONB column's raw bytes and
// its decoded map form. Generic — used for both NSWData and CustomData.
func decodeJSONB(raw json.RawMessage) (JSONB, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out JSONB
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func encodeJSONB(data JSONB) (json.RawMessage, error) {
	if data == nil {
		return nil, nil
	}
	return json.Marshal(data)
}
