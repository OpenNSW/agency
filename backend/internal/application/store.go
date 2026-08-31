package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/OpenNSW/agency/backend/internal/consignment"
	"github.com/OpenNSW/agency/backend/internal/database"
	"github.com/OpenNSW/agency/backend/internal/feedback"
	"github.com/OpenNSW/agency/backend/internal/user"
	"github.com/OpenNSW/agency/backend/pkg/dbtype"
	"github.com/OpenNSW/agency/backend/pkg/jsonquery"
	"gorm.io/gorm"
)

// JSONB is a custom type for storing JSON data. See pkg/dbtype for the
// Value/Scan implementation, shared with other domain packages.
type JSONB = dbtype.JSONB

// ApplicationRecord represents an application (task) in the Agency database
type ApplicationRecord struct {
	TaskID                string                        `gorm:"type:text;primaryKey"`
	TaskCode              string                        `gorm:"type:varchar(100);not null"`
	ConsignmentID         string                        `gorm:"type:text;index;not null;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Consignment           consignment.ConsignmentRecord `gorm:"foreignKey:ConsignmentID;references:ID"`
	Data                  JSONB                         `gorm:"type:text"`                                   // Injected data from service
	ReviewerResponse      JSONB                         `gorm:"type:text"`                                   // Response from reviewer
	Status                string                        `gorm:"type:varchar(50);not null;default:'PENDING'"` // PENDING, FEEDBACK_REQUESTED, DONE
	AgencyFeedbackHistory []feedback.Entry              `gorm:"type:text;serializer:json"`
	ClaimedBy             *string                       `gorm:"column:claimed_by;type:text"` // user_id of the officer currently working on this application
	ClaimedByName         *string                       `gorm:"-"`                           // looked up from users via ClaimedBy; not persisted
	ClaimedByEmail        *string                       `gorm:"-"`                           // looked up from users via ClaimedBy; not persisted
	ClaimedAt             *time.Time                    `gorm:"column:claimed_at"`
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
	// consignmentCustomDataSchema is the optional deployment-level JSON
	// Schema validating consignments' merged custom_data (see
	// consignment.Store.MergeCustomData). nil skips validation.
	consignmentCustomDataSchema json.RawMessage
	// driver is db's dialect name ("postgres" or "sqlite"), captured once at
	// construction so List can build a dialect-appropriate SQL predicate for
	// a datascope-resolved custom_data filter (see pkg/jsonquery).
	driver string
}

// NewApplicationStore creates a new ApplicationStore with configured
// database. Schema must be applied before starting the server via the
// migrate command. consignmentCustomDataSchema is passed through to every
// consignment.Store.MergeCustomData call this store makes; pass nil if none
// is configured for this deployment.
func NewApplicationStore(cfg database.Config, consignmentCustomDataSchema json.RawMessage) (*ApplicationStore, error) {
	connector, err := database.NewConnector(cfg)
	if err != nil {
		return nil, err
	}

	db, err := connector.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &ApplicationStore{
		db:                          db,
		consignmentStore:            consignment.NewConsignmentStore(db),
		consignmentCustomDataSchema: consignmentCustomDataSchema,
		driver:                      db.Name(),
	}, nil
}

// CreateOrUpdate creates or updates an application record and its parent
// consignment. pushedFields (from resolvePushedFields) is merged onto the
// consignment's own custom_data in the same transaction; pass nil/empty
// when the task has nothing to push.
func (s *ApplicationStore) CreateOrUpdate(app *ApplicationRecord, pushedFields map[string]any) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Upsert the consignment first so the FK reference exists.
		if err := s.consignmentStore.Upsert(tx, app.ConsignmentID, app.Status); err != nil {
			return fmt.Errorf("failed to upsert consignment: %w", err)
		}
		if err := s.consignmentStore.MergeCustomData(tx, app.ConsignmentID, pushedFields, s.consignmentCustomDataSchema); err != nil {
			return fmt.Errorf("failed to merge consignment custom data: %w", err)
		}
		return tx.Save(app).Error
	})
}

// GetByTaskID retrieves an application by task ID. The parent Consignment is
// preloaded so callers (e.g. application.buildApplication) can check its
// custom_data against a datascope resolution without a second query.
func (s *ApplicationStore) GetByTaskID(taskID string) (*ApplicationRecord, error) {
	var app ApplicationRecord
	if err := s.db.Preload("Consignment").First(&app, "task_id = ?", taskID).Error; err != nil {
		return nil, err
	}
	if err := s.hydrateClaimant(&app); err != nil {
		return nil, err
	}
	return &app, nil
}

// GetByConsignmentAndTaskCode retrieves the application within a consignment
// whose TaskCode matches taskCode, assuming at most one such application per
// consignment. The parent Consignment is preloaded — see GetByTaskID.
func (s *ApplicationStore) GetByConsignmentAndTaskCode(consignmentID, taskCode string) (*ApplicationRecord, error) {
	var app ApplicationRecord
	if err := s.db.Preload("Consignment").First(&app, "consignment_id = ? AND task_code = ?", consignmentID, taskCode).Error; err != nil {
		return nil, err
	}
	if err := s.hydrateClaimant(&app); err != nil {
		return nil, err
	}
	return &app, nil
}

// hydrateClaimant looks up the claimant's current name and email from the
// users table via ClaimedBy and populates them onto app. This is a live
// lookup rather than a denormalized copy, so a released, re-claimed, or
// deleted claimant can never leave stale identity information behind.
//
// It also enforces that ClaimedAt is only ever meaningful alongside an
// active claim: the users FK's ON DELETE SET NULL only clears ClaimedBy,
// so without this, ClaimedAt would survive as a claim timestamp with no
// claimant behind it.
func (s *ApplicationStore) hydrateClaimant(app *ApplicationRecord) error {
	if app.ClaimedBy == nil {
		app.ClaimedAt = nil
		return nil
	}
	var u user.UserRecord
	if err := s.db.Select("name", "email").First(&u, "user_id = ?", *app.ClaimedBy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	app.ClaimedByName = &u.Name
	app.ClaimedByEmail = &u.Email
	return nil
}

// List retrieves applications with optional status, consignment, and search
// filters and pagination, restricted to those whose parent consignment
// matches scope (a datascope-resolved map of consignments.custom_data JSON
// Pointer -> required value, applied as an AND of equality predicates;
// nil/empty means unrestricted).
// It projects out the JSONB columns (Data, ReviewerResponse, AgencyFeedbackHistory) since callers of
// List only ever use the lightweight fields; GetByTaskID reads full rows for that.
//
// Every one of this method's own columns is qualified with "applications."
// throughout — harmless whether or not the scope join below is present, and
// required once it is: consignments also has status/created_at/updated_at
// columns, so an unqualified reference would become ambiguous SQL the
// moment that join exists.
func (s *ApplicationStore) List(ctx context.Context, status string, consignmentID string, search string, scope map[string]any, offset, limit int) ([]ApplicationRecord, int64, error) {
	var apps []ApplicationRecord
	var total int64

	query := s.db.WithContext(ctx).Model(&ApplicationRecord{}).
		Select("applications.task_id", "applications.task_code", "applications.consignment_id",
			"applications.status", "applications.claimed_by", "applications.claimed_at",
			"applications.reviewed_at", "applications.created_at", "applications.updated_at")
	if status != "" {
		query = query.Where("applications.status = ?", status)
	}
	if consignmentID != "" {
		query = query.Where("applications.consignment_id = ?", consignmentID)
	}
	if search != "" {
		query = query.Where("applications.task_id LIKE ? OR applications.consignment_id LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if len(scope) > 0 {
		query = query.Joins("JOIN consignments ON consignments.id = applications.consignment_id")
		pointers := make([]string, 0, len(scope))
		for pointer := range scope {
			pointers = append(pointers, pointer)
		}
		sort.Strings(pointers)
		for _, pointer := range pointers {
			sql, arg, err := jsonquery.Predicate(s.driver, "consignments.custom_data", pointer, scope[pointer])
			if err != nil {
				return nil, 0, err
			}
			query = query.Where(sql, arg)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("CASE WHEN applications.status = 'PENDING' THEN 0 WHEN applications.status = 'FEEDBACK_REQUESTED' THEN 1 ELSE 2 END ASC, applications.created_at DESC").Offset(offset).Limit(limit).Find(&apps).Error; err != nil {
		return nil, 0, err
	}

	for i := range apps {
		if err := s.hydrateClaimant(&apps[i]); err != nil {
			return nil, 0, err
		}
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

// FinalizeReview atomically records a review outcome, but only if userID
// still holds the claim and the application is still PENDING. It returns
// ErrApplicationReviewConflict if that condition no longer holds (e.g. a
// concurrent review already completed, or the claim changed hands), so the
// caller never overwrites another officer's outcome or double-records its
// own.
func (s *ApplicationStore) FinalizeReview(taskID, userID string, status string, reviewerResponse map[string]any) error {
	now := time.Now()

	jsonResponse, err := json.Marshal(reviewerResponse)
	if err != nil {
		return fmt.Errorf("failed to marshal reviewer response: %w", err)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&ApplicationRecord{}).
			Where("task_id = ? AND claimed_by = ? AND status = ?", taskID, userID, "PENDING").
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
			return ErrApplicationReviewConflict
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

// UpdateDataAndResetStatus updates the submitted data and resets status to
// PENDING, called when a trader resubmits after receiving feedback.
// pushedFields is merged onto the consignment's custom_data in the same
// transaction, same as CreateOrUpdate — the resubmitted data may carry new
// values for previously-pushed fields.
func (s *ApplicationStore) UpdateDataAndResetStatus(taskID string, data map[string]any, pushedFields map[string]any) error {
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

		if err := s.consignmentStore.MergeCustomData(tx, app.ConsignmentID, pushedFields, s.consignmentCustomDataSchema); err != nil {
			return fmt.Errorf("failed to merge consignment custom data: %w", err)
		}

		return s.consignmentStore.UpdateStatus(tx, app.ConsignmentID, "PENDING", now)
	})
}

// ClaimApplication atomically claims an application for the given officer,
// unless it is already claimed by a different officer or is no longer
// PENDING (i.e. it has already been reviewed). If the application is already
// claimed by userID, this is a no-op: it succeeds without touching the row
// or refreshing claimed_at.
func (s *ApplicationStore) ClaimApplication(taskID, userID string) error {
	now := time.Now()

	result := s.db.Model(&ApplicationRecord{}).
		Where("task_id = ? AND claimed_by IS NULL AND status = ?", taskID, "PENDING").
		Updates(map[string]any{
			"claimed_by": userID,
			"claimed_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}

	// No rows updated: disambiguate why - not found, no longer PENDING,
	// already claimed by this same officer (no-op success), or claimed by
	// someone else.
	record, err := s.GetByTaskID(taskID)
	if err != nil {
		return err
	}
	if record.Status != "PENDING" {
		return ErrApplicationNotPending
	}
	if record.ClaimedBy != nil && *record.ClaimedBy == userID {
		return nil
	}
	return ErrApplicationAlreadyClaimed
}

// ReleaseApplication releases the given officer's claim on an application.
// Fails if the application is not currently claimed by userID, or if it is
// no longer PENDING (i.e. it has already been reviewed).
func (s *ApplicationStore) ReleaseApplication(taskID, userID string) error {
	result := s.db.Model(&ApplicationRecord{}).
		Where("task_id = ? AND claimed_by = ? AND status = ?", taskID, userID, "PENDING").
		Updates(map[string]any{
			"claimed_by": nil,
			"claimed_at": nil,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}

	// No rows updated: disambiguate why.
	record, err := s.GetByTaskID(taskID)
	if err != nil {
		return err
	}
	if record.ClaimedBy == nil || *record.ClaimedBy != userID {
		return ErrApplicationNotClaimedByYou
	}
	return ErrApplicationNotPending
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
