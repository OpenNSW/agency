// Package engine owns an application's state from the moment it is
// submitted (after its parent consignment is ensured to exist) until it
// reaches a terminal review outcome or is resubmitted by the trader.
// internal/application depends on this package only through the Engine
// interface, so a different implementation can be substituted in
// cmd/server/main.go without changing Service or any HTTP handler.
package engine

import (
	"context"
	"errors"

	"github.com/OpenNSW/agency/backend/internal/feedback"
)

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

// Engine owns an application's state from the moment it is submitted (after
// its parent consignment is ensured to exist) until it reaches a terminal
// review outcome or is resubmitted by the trader. ApplicationStore is the
// default implementation, persisting this state directly in the
// applications/consignments tables. A different Engine implementation can be
// substituted in cmd/server/main.go without changing Service or any HTTP
// handler.
type Engine interface {
	// CreateOrUpdate creates or updates an application record, merging
	// pushedFields onto the parent consignment's custom_data in the same
	// transaction. See ApplicationStore.CreateOrUpdate.
	CreateOrUpdate(ctx context.Context, app *ApplicationRecord, pushedFields map[string]any) error

	// UpdateDataAndResetStatus records a trader resubmission: updates the
	// submitted data and resets status to PENDING. See
	// ApplicationStore.UpdateDataAndResetStatus.
	UpdateDataAndResetStatus(ctx context.Context, taskID string, data map[string]any, pushedFields map[string]any) error

	// GetByTaskID retrieves an application by task ID, with its parent
	// Consignment preloaded.
	GetByTaskID(ctx context.Context, taskID string) (*ApplicationRecord, error)

	// GetByConsignmentAndTaskCode retrieves the application within a
	// consignment whose TaskCode matches taskCode.
	GetByConsignmentAndTaskCode(ctx context.Context, consignmentID, taskCode string) (*ApplicationRecord, error)

	// List retrieves applications with optional filters and pagination. See
	// ApplicationStore.List.
	List(ctx context.Context, status, consignmentID, search string, scope map[string]any, offset, limit int) ([]ApplicationRecord, int64, error)

	// ClaimApplication atomically claims an application for the given
	// officer. See ApplicationStore.ClaimApplication.
	ClaimApplication(ctx context.Context, taskID, userID string) error

	// ReleaseApplication releases the given officer's claim on an
	// application. See ApplicationStore.ReleaseApplication.
	ReleaseApplication(ctx context.Context, taskID, userID string) error

	// FinalizeReview atomically records a review outcome, guarded on the
	// caller still holding the claim and the application still being
	// PENDING. See ApplicationStore.FinalizeReview.
	FinalizeReview(ctx context.Context, taskID, userID, status string, reviewerResponse map[string]any) error

	// AppendFeedback appends a feedback entry and sets status to
	// FEEDBACK_REQUESTED. See ApplicationStore.AppendFeedback.
	AppendFeedback(ctx context.Context, taskID string, entry feedback.Entry) error

	// GetTaskCode resolves a task's task_code from its task_id; also
	// satisfies rbac.TaskCodeResolver.
	GetTaskCode(ctx context.Context, taskID string) (string, error)

	// Close releases any resources held by the engine.
	Close() error
}

var _ Engine = (*ApplicationStore)(nil)
