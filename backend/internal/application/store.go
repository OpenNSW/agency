package application

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/OpenNSW/nsw-agency/backend/internal/consignment"
	"github.com/OpenNSW/nsw-agency/backend/internal/database"
	"github.com/OpenNSW/nsw-agency/backend/internal/feedback"
	"gorm.io/gorm"
)

// JSONB is a custom type for storing JSON data in SQLite
type JSONB map[string]any

// Value implements the driver.Valuer interface
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface
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

// ApplicationRecord represents an application (task) in the Agency database
type ApplicationRecord struct {
	TaskID                string                        `gorm:"type:text;primaryKey"`
	TaskCode              string                        `gorm:"type:varchar(100);not null"`
	ConsignmentID         string                        `gorm:"type:text;index;not null;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Consignment           consignment.ConsignmentRecord `gorm:"foreignKey:ConsignmentID;references:ID"`
	ServiceURL            string                        `gorm:"type:varchar(512);not null"`                  // URL to send response back to
	Data                  JSONB                         `gorm:"type:text"`                                   // Injected data from service
	ReviewerResponse      JSONB                         `gorm:"type:text"`                                   // Response from reviewer
	Status                string                        `gorm:"type:varchar(50);not null;default:'PENDING'"` // PENDING, FEEDBACK_REQUESTED, DONE
	AgencyFeedbackHistory []feedback.Entry              `gorm:"type:text;serializer:json"`
	ReviewedAt            *time.Time                    // When it was reviewed
	CreatedAt             time.Time                     `gorm:"autoCreateTime"`
	UpdatedAt             time.Time                     `gorm:"autoUpdateTime"`
}

// TableName returns the table name for ApplicationRecord
func (ApplicationRecord) TableName() string {
	return "applications"
}

// ApplicationStore handles database operations for Agency applications
type ApplicationStore struct {
	db               *gorm.DB
	consignmentStore *consignment.Store
}

// NewApplicationStore creates a new ApplicationStore with configured database.
// Schema must be applied before starting the server via the migrate command.
func NewApplicationStore(cfg database.Config) (*ApplicationStore, error) {
	connector, err := database.NewConnector(cfg)
	if err != nil {
		return nil, err
	}

	db, err := connector.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &ApplicationStore{db: db, consignmentStore: consignment.NewConsignmentStore(db)}, nil
}

// CreateOrUpdate creates or updates an application record and its parent consignment.
func (s *ApplicationStore) CreateOrUpdate(app *ApplicationRecord) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Upsert the consignment first so the FK reference exists.
		if err := s.consignmentStore.Upsert(tx, app.ConsignmentID, app.Status); err != nil {
			return fmt.Errorf("failed to upsert consignment: %w", err)
		}
		return tx.Save(app).Error
	})
}

// GetByTaskID retrieves an application by task ID
func (s *ApplicationStore) GetByTaskID(taskID string) (*ApplicationRecord, error) {
	var app ApplicationRecord
	if err := s.db.First(&app, "task_id = ?", taskID).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

// List retrieves applications with optional status, consignment, task code,
// and search filters and pagination.
func (s *ApplicationStore) List(ctx context.Context, status string, consignmentID string, taskCode string, search string, offset, limit int) ([]ApplicationRecord, int64, error) {
	var apps []ApplicationRecord
	var total int64

	query := s.db.WithContext(ctx).Model(&ApplicationRecord{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if consignmentID != "" {
		query = query.Where("consignment_id = ?", consignmentID)
	}
	if taskCode != "" {
		query = query.Where("task_code = ?", taskCode)
	}
	if search != "" {
		query = query.Where("task_id LIKE ? OR consignment_id LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("CASE WHEN status = 'PENDING' THEN 0 WHEN status = 'FEEDBACK_REQUESTED' THEN 1 ELSE 2 END ASC, created_at DESC").Offset(offset).Limit(limit).Find(&apps).Error; err != nil {
		return nil, 0, err
	}

	return apps, total, nil
}

// UpdateStatus updates the status of an application and propagates it to the parent consignment.
func (s *ApplicationStore) UpdateStatus(taskID string, status string, reviewerResponse map[string]any) error {
	now := time.Now()

	jsonResponse, err := json.Marshal(reviewerResponse)
	if err != nil {
		return fmt.Errorf("failed to marshal reviewer response: %w", err)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&ApplicationRecord{}).
			Where("task_id = ?", taskID).
			Updates(map[string]any{
				"status":            status,
				"reviewed_at":       now,
				"updated_at":        now,
				"reviewer_response": jsonResponse,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("application with task_id %s not found", taskID)
		}

		var app ApplicationRecord
		if err := tx.Select("consignment_id").Where("task_id = ?", taskID).First(&app).Error; err != nil {
			return fmt.Errorf("failed to fetch consignment_id: %w", err)
		}

		return s.consignmentStore.UpdateStatus(tx, app.ConsignmentID, status, now)
	})
}

// AppendFeedback appends a feedback entry to the application's history and sets
// the status to FEEDBACK_REQUESTED.
func (s *ApplicationStore) AppendFeedback(taskID string, entry feedback.Entry) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var app ApplicationRecord
		if err := tx.First(&app, "task_id = ?", taskID).Error; err != nil {
			return err
		}
		updated := append(app.AgencyFeedbackHistory, entry)
		updatedJSON, err := json.Marshal(updated)
		if err != nil {
			return fmt.Errorf("failed to marshal feedback history: %w", err)
		}

		now := time.Now()

		if err := tx.Model(&ApplicationRecord{}).
			Where("task_id = ?", taskID).
			Updates(map[string]any{
				"agency_feedback_history": string(updatedJSON),
				"status":                  "FEEDBACK_REQUESTED",
				"updated_at":              now,
			}).Error; err != nil {
			return err
		}

		return s.consignmentStore.UpdateStatus(tx, app.ConsignmentID, "FEEDBACK_REQUESTED", now)
	})
}

// UpdateDataAndResetStatus updates the submitted data and resets status to PENDING.
// Called when a trader resubmits after receiving feedback.
func (s *ApplicationStore) UpdateDataAndResetStatus(taskID string, data map[string]any) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		var app ApplicationRecord
		if err := tx.Select("consignment_id").Where("task_id = ?", taskID).First(&app).Error; err != nil {
			return err
		}

		now := time.Now()

		if err := tx.Model(&ApplicationRecord{}).
			Where("task_id = ?", taskID).
			Updates(map[string]any{
				"data":       string(dataJSON),
				"status":     "PENDING",
				"updated_at": now,
			}).Error; err != nil {
			return err
		}

		return s.consignmentStore.UpdateStatus(tx, app.ConsignmentID, "PENDING", now)
	})
}

// Delete removes an application by task ID
func (s *ApplicationStore) Delete(taskID string) error {
	return s.db.Delete(&ApplicationRecord{}, "task_id = ?", taskID).Error
}

// Close closes the database connection
func (s *ApplicationStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// DB returns the underlying gorm.DB connection.
func (s *ApplicationStore) DB() *gorm.DB {
	return s.db
}

// GetTaskCode implements rbac.TaskCodeResolver.
func (s *ApplicationStore) GetTaskCode(ctx context.Context, taskID string) (string, error) {
	var app ApplicationRecord
	if err := s.db.WithContext(ctx).Select("task_code").First(&app, "task_id = ?", taskID).Error; err != nil {
		return "", err
	}
	return app.TaskCode, nil
}
