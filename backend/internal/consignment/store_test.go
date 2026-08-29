package consignment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OpenNSW/agency/backend/internal/database"
)

// testApplicationRow is a minimal stand-in for internal/application's
// ApplicationRecord, just enough to exercise the LEFT JOIN in List. Kept
// local so this package doesn't need to depend on internal/application.
type testApplicationRow struct {
	TaskID        string `gorm:"column:task_id;primaryKey"`
	ConsignmentID string `gorm:"column:consignment_id"`
	Status        string `gorm:"column:status"`
}

func (testApplicationRow) TableName() string { return "applications" }

// newTestStore creates a ConsignmentStore backed by an in-memory SQLite database,
// with both the consignments table and a minimal applications table migrated.
func newTestStore(t *testing.T) *Store {
	t.Helper()

	connector, err := database.NewConnector(database.Config{Driver: "sqlite", SQLite: database.SQLiteConfig{Path: ":memory:"}})
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}
	db, err := connector.Open()
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := db.AutoMigrate(&ConsignmentRecord{}, &testApplicationRow{}); err != nil {
		t.Fatalf("failed to migrate schema: %v", err)
	}

	return NewConsignmentStore(db)
}

func seedConsignmentWithData(t *testing.T, store *Store, id, status string, data JSONB, taskIDs ...string) {
	t.Helper()
	tx := store.db.Begin()
	if err := tx.Create(&ConsignmentRecord{ID: id, Status: status, NSWData: data}).Error; err != nil {
		t.Fatalf("seed consignment %s: %v", id, err)
	}
	for _, taskID := range taskIDs {
		if err := tx.Create(&testApplicationRow{TaskID: taskID, ConsignmentID: id, Status: status}).Error; err != nil {
			t.Fatalf("failed to seed application row %s: %v", taskID, err)
		}
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit failed: %v", err)
	}
}

func seedConsignment(t *testing.T, store *Store, id, status string, taskIDs ...string) {
	t.Helper()
	seedConsignmentWithData(t, store, id, status, nil, taskIDs...)
}

func TestConsignmentStore_List(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	seedConsignmentWithData(t, store, "wf1", "PENDING", JSONB{"traderCompanyName": "ACME CORP"}, "wf1-t1", "wf1-t2")
	seedConsignment(t, store, "wf2", "PENDING", "wf2-t1")
	seedConsignment(t, store, "wf3", "REJECTED", "wf3-t1")

	summaries, total, err := store.List(ctx, "", 0, 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if total != 3 {
		t.Errorf("expected 3 unique consignments, got %d", total)
	}
	if len(summaries) != 3 {
		t.Errorf("expected 3 summaries returned, got %d", len(summaries))
	}

	foundWF1 := false
	foundWF2 := false
	for _, s := range summaries {
		if s.ConsignmentID == "wf1" {
			foundWF1 = true
			if s.TaskCount != 2 {
				t.Errorf("expected 2 tasks for wf1, got %d", s.TaskCount)
			}
			if s.TraderCompanyName != "ACME CORP" {
				t.Errorf("expected trader company name ACME CORP for wf1, got %q", s.TraderCompanyName)
			}
		}
		if s.ConsignmentID == "wf2" {
			foundWF2 = true
			if s.TaskCount != 1 {
				t.Errorf("expected 1 task for wf2, got %d", s.TaskCount)
			}
			if s.TraderCompanyName != "" {
				t.Errorf("consignment without extras must omit trader company name, got %q", s.TraderCompanyName)
			}
			if s.Status != "PENDING" {
				t.Errorf("expected PENDING for wf2, got %q", s.Status)
			}
		}
	}
	if !foundWF1 {
		t.Error("wf1 not found in summaries")
	}
	if !foundWF2 {
		t.Error("wf2 not found in summaries")
	}
}

func TestConsignmentStore_Upsert_PreservesCreatedAtAndNSWData(t *testing.T) {
	store := newTestStore(t)

	// Seed directly with a fixed, distinctive CreatedAt and NSWData
	fixedCreatedAt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.db.Create(&ConsignmentRecord{
		ID:        "wf-created",
		Status:    "PENDING",
		NSWData:   JSONB{"traderCompanyName": "INITIAL CORP"},
		CreatedAt: fixedCreatedAt,
	}).Error; err != nil {
		t.Fatalf("failed to seed consignment: %v", err)
	}

	// Re-upsert as if a second application landed in the same consignment.
	tx := store.db.Begin()
	if err := store.Upsert(tx, "wf-created", "APPROVED"); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	var got ConsignmentRecord
	if err := store.db.First(&got, "id = ?", "wf-created").Error; err != nil {
		t.Fatalf("failed to fetch consignment: %v", err)
	}

	if !got.CreatedAt.Equal(fixedCreatedAt) {
		t.Errorf("expected CreatedAt to be preserved as %v, got %v", fixedCreatedAt, got.CreatedAt)
	}
	if got.Status != "APPROVED" {
		t.Errorf("expected Status to be updated to APPROVED, got %q", got.Status)
	}
	if got.NSWData["traderCompanyName"] != "INITIAL CORP" {
		t.Errorf("expected NSWData to be preserved as INITIAL CORP, got %v", got.NSWData["traderCompanyName"])
	}
}

func TestConsignmentStore_List_Search(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	seedConsignment(t, store, "alpha-wf", "PENDING", "t1")
	seedConsignment(t, store, "beta-wf", "PENDING", "t2")

	summaries, total, err := store.List(ctx, "alpha", 0, 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if summaries[0].ConsignmentID != "alpha-wf" {
		t.Errorf("expected alpha-wf, got %s", summaries[0].ConsignmentID)
	}
}

func TestConsignmentStore_Create(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.Create(ctx, "c-create", JSONB{"traderCompanyName": "ACME"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Create(ctx, "c-create", JSONB{"traderCompanyName": "OTHER"}); err != nil {
		t.Fatalf("Create again: %v", err)
	}

	rec, err := store.Get(ctx, "c-create")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.ID != "c-create" || rec.Status != "PENDING" {
		t.Errorf("unexpected record: %+v", rec)
	}
	if rec.NSWData["traderCompanyName"] != "ACME" {
		t.Errorf("Create on conflict must keep original extras, got %v", rec.NSWData["traderCompanyName"])
	}

	if _, err := store.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing): got %v, want ErrNotFound", err)
	}
}
