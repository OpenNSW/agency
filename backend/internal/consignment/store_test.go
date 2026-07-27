package consignment

import (
	"context"
	"testing"

	"github.com/OpenNSW/nsw-agency/backend/internal/database"
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

func seedConsignment(t *testing.T, store *Store, id, status string, taskIDs ...string) {
	t.Helper()
	tx := store.db.Begin()
	if err := store.Upsert(tx, id, status); err != nil {
		t.Fatalf("Upsert(%s) failed: %v", id, err)
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

func TestConsignmentStore_List(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	seedConsignment(t, store, "wf1", "PENDING", "wf1-t1", "wf1-t2")
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
	for _, s := range summaries {
		if s.ConsignmentID == "wf1" {
			foundWF1 = true
			if s.TaskCount != 2 {
				t.Errorf("expected 2 tasks for wf1, got %d", s.TaskCount)
			}
		}
	}
	if !foundWF1 {
		t.Error("wf1 not found in summaries")
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
