package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/OpenNSW/agency/backend/internal/authn"
	"github.com/OpenNSW/agency/backend/internal/feedback"
	"github.com/OpenNSW/agency/backend/internal/rbac"
	"github.com/OpenNSW/agency/backend/internal/taskconfig"
	"github.com/OpenNSW/agency/backend/internal/taskconfig/taskconfigart"
	"github.com/OpenNSW/agency/backend/pkg/httputil"
	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/generictemplate"
	"gorm.io/gorm"
)

// ErrApplicationNotFound is returned when an application is not found
var ErrApplicationNotFound = errors.New("application not found")

// ErrApplicationAlreadyClaimed is returned when a claim attempt conflicts
// with an existing claim held by a different officer.
var ErrApplicationAlreadyClaimed = errors.New("application already claimed by another officer")

// ErrApplicationNotClaimedByYou is returned when an action that requires a
// claim (reviewing, releasing) is attempted by someone other than the
// current claimant.
var ErrApplicationNotClaimedByYou = errors.New("application must be claimed by you first")

// ErrApplicationNotPending is returned when claiming or releasing an
// application that has already been reviewed (i.e. is no longer PENDING).
var ErrApplicationNotPending = errors.New("application has already been reviewed and is no longer pending")

// ErrApplicationReviewConflict is returned when a review outcome can no
// longer be persisted because the caller's claim or the application's
// PENDING status changed since the review was validated (e.g. a concurrent
// review already completed, or the claim was released and re-claimed).
var ErrApplicationReviewConflict = errors.New("application was already reviewed or your claim has changed")

// ErrInvalidInjectRequest is returned when an inject request is malformed:
// missing required fields, references a task code with no task
// configuration, or submits data that fails the task's view form schema.
var ErrInvalidInjectRequest = errors.New("invalid inject request")

// Service handles Agency portal operations
type Service interface {
	// CreateApplication creates a new application from injected data
	CreateApplication(ctx context.Context, req *InjectRequest) error

	// GetApplications returns a paginated list of applications (optionally filtered by status, consignment, or search)
	GetApplications(ctx context.Context, status string, consignmentID string, search string, page, pageSize int) (*httputil.PagedResponse[Application], error)

	// GetApplication returns a specific application by task ID
	GetApplication(ctx context.Context, taskID string) (*Application, error)

	// GetApplicationByTaskCode returns the application within a consignment
	// whose TaskCode matches, for internal lookups (e.g. certificate template
	// field resolution) that key on TaskCode rather than TaskID.
	GetApplicationByTaskCode(ctx context.Context, consignmentID string, taskCode string) (*Application, error)

	// ReviewApplication approves or rejects an application and sends response back to service.
	// Requires that the caller currently holds the claim on the application.
	ReviewApplication(ctx context.Context, taskID string, reviewerData map[string]any) error

	// FeedbackApplication sends a change-request feedback to the trader via the NSW task API
	// and updates the application status to FEEDBACK_REQUESTED.
	FeedbackApplication(ctx context.Context, taskID string, content map[string]any) error

	// ClaimApplication marks the application as claimed by the calling
	// officer, required before ReviewApplication can be called. Idempotent
	// if the caller already holds the claim.
	ClaimApplication(ctx context.Context, taskID string) error

	// ReleaseApplication releases the calling officer's claim on the
	// application.
	ReleaseApplication(ctx context.Context, taskID string) error

	// Close closes the service and releases resources
	Close() error
}

// InjectRequest represents the incoming data from services
type InjectRequest struct {
	TaskID                string           `json:"taskId"`
	TaskCode              string           `json:"taskCode"`
	ConsignmentID         string           `json:"consignmentId"`
	Data                  map[string]any   `json:"data"`
	AgencyFeedbackHistory []map[string]any `json:"agencyFeedbackHistory,omitempty"`
}

// Application represents an application for display in the UI
type Application struct {
	TaskID           string         `json:"taskId"`
	TaskCode         string         `json:"taskCode"`
	ConsignmentID    string         `json:"consignmentId"`
	Data             map[string]any `json:"data,omitempty"`             // Data from NSW service to be rendered in the UI
	AgencyActionData map[string]any `json:"agencyActionData,omitempty"` // Copy of the payload sent back to the NSW after review, for display in the UI
	AllowedActions   []string       `json:"allowedActions,omitempty"`

	// Task metadata from config
	Title                 string          `json:"title,omitempty"`
	Description           string          `json:"description,omitempty"`
	Icon                  string          `json:"icon,omitempty"`
	Category              string          `json:"category,omitempty"`
	CertificateTemplateID string          `json:"certificateTemplateId,omitempty"` // Set when this task's officer can generate a certificate
	CertificateDataSchema json.RawMessage `json:"certificateDataSchema,omitempty"` // JSON Schema for the certificate generate request's data, validated client-side

	DataForm        json.RawMessage  `json:"dataForm,omitempty"`   // Schema for rendering the data in Read Only mode in the UI
	AgencyForm      json.RawMessage  `json:"agencyForm,omitempty"` // Schema for rendering the Agency Action form in the UI
	Status          string           `json:"status"`
	FeedbackHistory []feedback.Entry `json:"feedbackHistory,omitempty"`
	ReviewedAt      *time.Time       `json:"reviewedAt,omitempty"`

	// Set when an officer has claimed the application to work on it; required
	// before ReviewApplication will accept a decision.
	ClaimedByName  *string    `json:"claimedByName,omitempty"`
	ClaimedByEmail *string    `json:"claimedByEmail,omitempty"`
	ClaimedAt      *time.Time `json:"claimedAt,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NSWClient sends task outcomes and amendment requests back to the originating
// NSW service.
type NSWClient interface {
	// SendOutcome sends a review outcome (command + payload) for a task.
	SendOutcome(ctx context.Context, taskID, command string, payload any) error
	// RequestAmendment asks the trader to amend a submission.
	RequestAmendment(ctx context.Context, taskID string, payload any) error
}

// ConsignmentService is the subset of the consignment domain used when injecting
// applications. Keeping this interface in the application domain avoids coupling
// application code to consignment store or NSW fetch details.
type ConsignmentService interface {
	// CreateConsignment creates the consignment if it does not exist and caches NSW extras.
	CreateConsignment(ctx context.Context, consignmentID string) error
	// UpdateConsignment updates the status column of the consignment.
	UpdateConsignment(ctx context.Context, consignmentID, status string) error
}

type service struct {
	store              *ApplicationStore
	artifactRegistry   *artifact.Registry
	nsw                NSWClient
	roleService        *rbac.RoleService
	consignmentService ConsignmentService
}

// NewService creates a new Agency service instance with database storage
func NewService(store *ApplicationStore, artifactRegistry *artifact.Registry, nsw NSWClient, roleService *rbac.RoleService, consignmentService ConsignmentService) Service {
	if store == nil || artifactRegistry == nil || nsw == nil || roleService == nil || consignmentService == nil {
		panic("NewService: all dependencies must be non-nil")
	}
	return &service{
		store:              store,
		artifactRegistry:   artifactRegistry,
		nsw:                nsw,
		roleService:        roleService,
		consignmentService: consignmentService,
	}
}

// CreateApplication creates a new application from injected data.
func (s *service) CreateApplication(ctx context.Context, req *InjectRequest) error {
	if req.TaskID == "" || req.TaskCode == "" || req.ConsignmentID == "" {
		return fmt.Errorf("%w: missing required fields in InjectRequest", ErrInvalidInjectRequest)
	}

	config, err := taskconfigart.Load(ctx, s.artifactRegistry, req.TaskCode)
	if err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			return fmt.Errorf("%w: unknown task code %q", ErrInvalidInjectRequest, req.TaskCode)
		}
		return fmt.Errorf("failed to load task config for task code %s: %w", req.TaskCode, err)
	}

	if config.Forms.View != "" {
		if err := validateAgainstViewForm(ctx, s.artifactRegistry, config.Forms.View, req.Data); err != nil {
			return err
		}
	}

	// Computed once so both branches below (resubmission and normal
	// create/update) push the same way; most tasks have no ConsignmentFields
	// declared, so this is nil/free in the common case.
	pushedFields := resolvePushedFields(config.ConsignmentFields, req.Data)

	existing, err := s.store.GetByTaskID(req.TaskID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to query existing application: %w", err)
		}
		// Record doesn't exist — fall through to create.
	} else if existing.Status == "FEEDBACK_REQUESTED" {
		slog.InfoContext(ctx, "trader resubmitted after feedback, resetting to PENDING", "taskID", req.TaskID)
		return s.store.UpdateDataAndResetStatus(req.TaskID, req.Data, pushedFields)
	}

	appRecord := &ApplicationRecord{
		TaskID:        req.TaskID,
		TaskCode:      req.TaskCode,
		ConsignmentID: req.ConsignmentID,
		Data:          req.Data,
		Status:        "PENDING",
	}
	if existing != nil {
		// CreateOrUpdate does a full-row Save, so any field left unset here
		// would be overwritten to NULL. Carry the claim forward so
		// re-injecting an already-claimed application doesn't erase it.
		appRecord.ClaimedBy = existing.ClaimedBy
		appRecord.ClaimedAt = existing.ClaimedAt
	}

	if existing == nil {
		if err := s.consignmentService.CreateConsignment(ctx, req.ConsignmentID); err != nil {
			// TODO: revert application creation when inject and consignment writes share a transaction.
			slog.WarnContext(ctx, "failed to create consignment after application inject",
				"consignmentID", req.ConsignmentID, "error", err)
		}
	}

	if err := s.store.CreateOrUpdate(appRecord, pushedFields); err != nil {
		return err
	}
	return nil
}

// GetApplications returns a paginated list of applications. List items are
// intentionally lean: unlike GetApplication, they carry no Data,
// AgencyActionData, or form schemas — callers needing an application's
// submitted data or review outcome must fetch it individually via
// GetApplication.
func (s *service) GetApplications(ctx context.Context, status string, consignmentID string, search string, page, pageSize int) (*httputil.PagedResponse[Application], error) {
	page, pageSize, offset := httputil.NormalizePage(page, pageSize)
	records, total, err := s.store.List(ctx, status, consignmentID, search, offset, pageSize)
	if err != nil {
		return nil, err
	}

	principal, authenticated := authn.FromContext(ctx)
	var roles []rbac.RoleRecord
	if authenticated && principal.Kind == authn.KindUser {
		var err error
		roles, err = s.roleService.GetRolesForUser(principal.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to get roles for user: %w", err)
		}
	}

	applications := make([]Application, 0, len(records))
	for _, record := range records {
		var permissions []taskconfig.Permission
		app := Application{
			TaskID:         record.TaskID,
			TaskCode:       record.TaskCode,
			ConsignmentID:  record.ConsignmentID,
			Status:         record.Status,
			ReviewedAt:     record.ReviewedAt,
			ClaimedByName:  record.ClaimedByName,
			ClaimedByEmail: record.ClaimedByEmail,
			ClaimedAt:      record.ClaimedAt,
			CreatedAt:      record.CreatedAt,
			UpdatedAt:      record.UpdatedAt,
		}

		config, err := taskconfigart.Load(ctx, s.artifactRegistry, record.TaskCode)
		if err != nil {
			if !errors.Is(err, artifact.ErrNotFound) {
				return nil, fmt.Errorf("failed to load task config for task %s: %w", record.TaskCode, err)
			}
			slog.WarnContext(ctx, "task config not found for application", "taskID", record.TaskID, "taskCode", record.TaskCode)
		} else {
			app.Title = config.Meta.Title
			app.Category = config.Meta.Category
			app.Icon = config.Meta.Icon
			permissions = config.Permissions
		}

		accessible, _ := rbac.ResolveAccess(roles, permissions)
		if !accessible {
			continue
		}

		applications = append(applications, app)
	}

	return &httputil.PagedResponse[Application]{
		Items:    applications,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetApplication returns a specific application by task ID
func (s *service) GetApplication(ctx context.Context, taskID string) (*Application, error) {
	record, err := s.store.GetByTaskID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApplicationNotFound
		}
		return nil, fmt.Errorf("failed to get application: %w", err)
	}
	return s.buildApplication(ctx, record)
}

// GetApplicationByTaskCode returns the application within a consignment whose
// TaskCode matches, resolved directly against the store rather than through a
// paginated list lookup.
func (s *service) GetApplicationByTaskCode(ctx context.Context, consignmentID string, taskCode string) (*Application, error) {
	record, err := s.store.GetByConsignmentAndTaskCode(consignmentID, taskCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApplicationNotFound
		}
		return nil, fmt.Errorf("failed to get application: %w", err)
	}
	return s.buildApplication(ctx, record)
}

// buildApplication assembles the API-facing Application DTO from a stored
// record: resolving the caller's roles and attaching task config metadata,
// permissions, and forms.
func (s *service) buildApplication(ctx context.Context, record *ApplicationRecord) (*Application, error) {
	principal, authenticated := authn.FromContext(ctx)
	var roles []rbac.RoleRecord
	if authenticated && principal.Kind == authn.KindUser {
		var err error
		roles, err = s.roleService.GetRolesForUser(principal.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to get roles for user: %w", err)
		}
	}

	app := &Application{
		TaskID:           record.TaskID,
		TaskCode:         record.TaskCode,
		ConsignmentID:    record.ConsignmentID,
		Data:             record.Data,
		AgencyActionData: record.ReviewerResponse,
		Status:           record.Status,
		FeedbackHistory:  record.AgencyFeedbackHistory,
		ReviewedAt:       record.ReviewedAt,
		ClaimedByName:    record.ClaimedByName,
		ClaimedByEmail:   record.ClaimedByEmail,
		ClaimedAt:        record.ClaimedAt,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}

	// Attach task configuration
	config, err := taskconfigart.Load(ctx, s.artifactRegistry, record.TaskCode)
	if err != nil {
		if !errors.Is(err, artifact.ErrNotFound) {
			// A genuine load failure (network, credentials, malformed config)
			// must not fall back to nil permissions, which would grant full
			// access to any authenticated user. Fail closed.
			return nil, fmt.Errorf("failed to load task config for task %s: %w", record.TaskCode, err)
		}
		// Config genuinely absent — omit metadata/forms. With no permissions
		// declared for this task code, nobody is granted access by default.
		slog.WarnContext(ctx, "task config not found for application", "taskID", record.TaskID, "taskCode", record.TaskCode)
		_, app.AllowedActions = rbac.ResolveAccess(roles, nil)
	} else {
		app.Title = config.Meta.Title
		app.Description = config.Meta.Description
		app.Icon = config.Meta.Icon
		app.Category = config.Meta.Category
		if config.Certificate != nil {
			app.CertificateTemplateID = config.Certificate.TemplateID
			app.CertificateDataSchema = config.Certificate.DataSchema
		}

		_, app.AllowedActions = rbac.ResolveAccess(roles, config.Permissions)

		if config.Forms.View != "" {
			if form, err := generictemplate.Load(ctx, s.artifactRegistry, config.Forms.View); err == nil {
				app.DataForm = form
			} else {
				slog.WarnContext(ctx, "view form not found", "taskCode", record.TaskCode, "formID", config.Forms.View)
			}
		}
		if config.Forms.Review != "" {
			if form, err := generictemplate.Load(ctx, s.artifactRegistry, config.Forms.Review); err == nil {
				app.AgencyForm = form
			} else {
				slog.WarnContext(ctx, "review form not found", "taskCode", record.TaskCode, "formID", config.Forms.Review)
			}
		}
	}

	return app, nil
}

// ReviewApplication approves or rejects an application. The caller must
// currently hold the claim on the application.
func (s *service) ReviewApplication(ctx context.Context, taskID string, reviewerResponse map[string]any) error {
	record, err := s.store.GetByTaskID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrApplicationNotFound
		}
		return fmt.Errorf("failed to get application: %w", err)
	}

	principal, authenticated := authn.FromContext(ctx)
	if !authenticated || principal.Kind != authn.KindUser || record.ClaimedBy == nil || *record.ClaimedBy != principal.UserID {
		return ErrApplicationNotClaimedByYou
	}
	userID := principal.UserID

	app, err := s.buildApplication(ctx, record)
	if err != nil {
		return err
	}

	config, configErr := taskconfigart.Load(ctx, s.artifactRegistry, app.TaskCode)
	if configErr != nil {
		// Without a config we don't know how this task's review outcome
		// should be interpreted. Fail closed rather than trusting an
		// arbitrary reviewer-supplied field as the command and status.
		return fmt.Errorf("failed to load task config for task %s: %w", app.TaskCode, configErr)
	}
	behavior := config.Behavior

	command := "approve"
	status := "DONE"

	if behavior.Type == taskconfig.BehaviorTypeAutoApprove {
		// No decision field to read: a successful submission always means
		// "record the data and move the application forward."
		status = "APPROVED"
	} else {
		outcomeField := taskconfig.DefaultOutcomeField
		if behavior.OutcomeField != "" {
			outcomeField = behavior.OutcomeField
		}

		if outcome, ok := reviewerResponse[outcomeField].(string); ok && outcome != "" {
			command = outcome
		}

		if behavior.StatusMap != nil {
			if outcome, ok := reviewerResponse[outcomeField].(string); ok {
				if mappedStatus, ok := behavior.StatusMap[outcome]; ok {
					status = mappedStatus
				}
			}
		}
	}

	if err := s.nsw.SendOutcome(ctx, app.TaskID, command, reviewerResponse); err != nil {
		return fmt.Errorf("failed to send response to service: %w", err)
	}

	// Persist the outcome only if userID still holds the claim and the
	// application is still PENDING. This closes the race window between the
	// ownership check above and this write: a concurrent or stale review
	// request (duplicate submission, or a claim released and re-claimed by
	// another officer while this call was in flight) fails here instead of
	// silently recording a second, conflicting outcome.
	if err := s.store.FinalizeReview(taskID, userID, status, reviewerResponse); err != nil {
		if errors.Is(err, ErrApplicationReviewConflict) {
			return err
		}
		return fmt.Errorf("failed to finalize review: %w", err)
	}
	return nil
}

// FeedbackApplication sends Agency feedback to the trader
func (s *service) FeedbackApplication(ctx context.Context, taskID string, content map[string]any) error {
	app, err := s.GetApplication(ctx, taskID)
	if err != nil {
		return err
	}

	entry := feedback.Entry{
		Content:   content,
		Timestamp: time.Now().UTC(),
		Round:     len(app.FeedbackHistory) + 1,
	}

	if err := s.nsw.RequestAmendment(ctx, app.TaskID, content); err != nil {
		return fmt.Errorf("failed to send feedback to service: %w", err)
	}

	return s.store.AppendFeedback(taskID, entry)
}

// ClaimApplication marks the application as claimed by the calling officer.
func (s *service) ClaimApplication(ctx context.Context, taskID string) error {
	principal, authenticated := authn.FromContext(ctx)
	if !authenticated || principal.Kind != authn.KindUser {
		return fmt.Errorf("claiming an application requires an authenticated user")
	}

	if err := s.store.ClaimApplication(taskID, principal.UserID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrApplicationNotFound
		}
		return err
	}
	return nil
}

// ReleaseApplication releases the calling officer's claim on the application.
func (s *service) ReleaseApplication(ctx context.Context, taskID string) error {
	principal, authenticated := authn.FromContext(ctx)
	if !authenticated || principal.Kind != authn.KindUser {
		return fmt.Errorf("releasing an application requires an authenticated user")
	}

	if err := s.store.ReleaseApplication(taskID, principal.UserID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrApplicationNotFound
		}
		return err
	}
	return nil
}

func (s *service) Close() error {
	if s.store != nil {
		return s.store.Close()
	}
	return nil
}
