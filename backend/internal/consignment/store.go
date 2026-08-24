package consignment

import (
	"context"
	"database/sql/driver"
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

// JSONB is a custom type for storing JSON data in SQLite / PostgreSQL.
type JSONB map[string]any

// Value implements the driver.Valuer interface.
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface.
func (j *JSONB) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to unmarshal JSONB value: %v", value)
	}

	return json.Unmarshal(bytes, j)
}

// ConsignmentRecord represents a consignment (workflow) in the Agency database.
// Each consignment groups one or more application records.
type ConsignmentRecord struct {
	ID        string    `gorm:"type:text;primaryKey"`
	Status    string    `gorm:"type:varchar(50);not null;default:'PENDING'"`
	Data      JSONB     `gorm:"type:text;serializer:json"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// TableName returns the table name for ConsignmentRecord
func (ConsignmentRecord) TableName() string {
	return "consignments"
}

// Summary represents a unique consignment with its most recent activity.
type Summary struct {
	ConsignmentID          string    `json:"consignmentId"`
	TraderCompanyName      string    `json:"traderCompanyName,omitempty"`
	ExporterRegistrationNo string    `json:"exporterRegistrationNo,omitempty"`
	CusdecNumber           string    `json:"cusdecNumber,omitempty"`
	UpdatedAt              time.Time `json:"updatedAt"`
	Status                 string    `json:"status"`    // Status of the most recent application
	TaskCount              int       `json:"taskCount"` // Total number of applications in this consignment
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
	if err := s.db.WithContext(ctx).Select("data").First(&rec, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return rec.Data, nil
}

// FillData writes extras onto a consignment that does not yet have them.
// Rows that already have data are left unchanged so a later inject cannot
// overwrite a successful first fetch.
func (s *Store) FillData(ctx context.Context, id string, data JSONB) error {
	if len(data) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(&ConsignmentRecord{}).
		Where("id = ? AND data IS NULL", id).
		Update("data", data).Error
}

// MergeData copies extra keys into consignments.data. Existing keys are
// overwritten so a later inject can fill exporter/CUSDEC fields that the
// first NSW extras fetch did not have.
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
	return s.db.WithContext(ctx).Model(&ConsignmentRecord{}).
		Where("id = ?", id).
		Update("data", current).Error
}

// Upsert upserts a consignment record using the given connection or
// transaction. Callers that also write a child application record (e.g.
// internal/application) should pass a transaction so the FK reference exists
// atomically.
//
// Only status and updated_at are touched on conflict — created_at and data
// must survive every later upsert of an already-existing consignment (e.g. when a
// second application is injected into the same consignment).
func (s *Store) Upsert(tx *gorm.DB, id, status string) error {
	return s.UpsertWithData(tx, id, status, nil)
}

// UpsertWithData upserts a consignment record along with its metadata JSONB.
func (s *Store) UpsertWithData(tx *gorm.DB, id, status string, data JSONB) error {
	record := &ConsignmentRecord{
		ID:     id,
		Status: status,
		Data:   data,
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
	ConsignmentID string    `gorm:"column:consignment_id"`
	Status        string    `gorm:"column:status"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
	Data          JSONB     `gorm:"column:data;type:text;serializer:json"`
	TaskCount     int       `gorm:"column:task_count"`
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
		Select("consignments.id AS consignment_id, consignments.status, consignments.updated_at, consignments.data, COUNT(applications.task_id) AS task_count").
		Joins("LEFT JOIN applications ON applications.consignment_id = consignments.id").
		Group("consignments.id, consignments.status, consignments.updated_at, consignments.data").
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
		summaries[i] = summaryFrom(r.ConsignmentID, r.Status, r.UpdatedAt, r.Data, r.TaskCount)
	}
	if err := s.overlayFormFields(ctx, summaries); err != nil {
		return nil, 0, err
	}

	return summaries, total, nil
}

// Get returns a single consignment summary by exact ID, including task count.
func (s *Store) Get(ctx context.Context, id string) (*Summary, error) {
	var rec ConsignmentRecord
	if err := s.db.WithContext(ctx).First(&rec, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var taskCount int64
	if err := s.db.WithContext(ctx).Table("applications").Where("consignment_id = ?", id).Count(&taskCount).Error; err != nil {
		return nil, err
	}

	summaries := []Summary{summaryFrom(rec.ID, rec.Status, rec.UpdatedAt, rec.Data, int(taskCount))}
	if err := s.overlayFormFields(ctx, summaries); err != nil {
		return nil, err
	}
	return &summaries[0], nil
}

func summaryFrom(id, status string, updatedAt time.Time, data JSONB, taskCount int) Summary {
	return Summary{
		ConsignmentID:          id,
		TraderCompanyName:      stringField(data, "traderCompanyName"),
		ExporterRegistrationNo: stringField(data, "exporterRegistrationNo", "exporter_registration_no"),
		CusdecNumber:           stringField(data, "cusdecNumber", "cusdec_number"),
		Status:                 status,
		UpdatedAt:              updatedAt,
		TaskCount:              taskCount,
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

type appDataRow struct {
	ConsignmentID string `gorm:"column:consignment_id"`
	Data          JSONB  `gorm:"column:data;type:text;serializer:json"`
}

// overlayFormFields fills exporter registration / CUSDEC from injected
// application rows when consignments.data does not yet have them (already-
// injected consignments).
func (s *Store) overlayFormFields(ctx context.Context, summaries []Summary) error {
	var ids []string
	for _, sm := range summaries {
		if sm.ExporterRegistrationNo == "" || sm.CusdecNumber == "" {
			ids = append(ids, sm.ConsignmentID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	var rows []appDataRow
	if err := s.db.WithContext(ctx).Table("applications").
		Select("consignment_id, data").
		Where("consignment_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return err
	}

	byID := map[string]JSONB{}
	for _, r := range rows {
		cur := byID[r.ConsignmentID]
		if cur == nil {
			cur = JSONB{}
		}
		if v := stringField(r.Data, "exporter_registration_no", "exporterRegistrationNo"); v != "" {
			cur["exporterRegistrationNo"] = v
		}
		if v := stringField(r.Data, "cusdec_number", "cusdecNumber"); v != "" {
			cur["cusdecNumber"] = v
		}
		byID[r.ConsignmentID] = cur
	}

	for i := range summaries {
		extra := byID[summaries[i].ConsignmentID]
		if summaries[i].ExporterRegistrationNo == "" {
			summaries[i].ExporterRegistrationNo = stringField(extra, "exporterRegistrationNo")
		}
		if summaries[i].CusdecNumber == "" {
			summaries[i].CusdecNumber = stringField(extra, "cusdecNumber")
		}
	}
	return nil
}

