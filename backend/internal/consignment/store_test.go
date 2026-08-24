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
	Data          JSONB  `gorm:"column:data;type:text"`
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
	if err := store.UpsertWithData(tx, id, status, data); err != nil {
		t.Fatalf("UpsertWithData(%s) failed: %v", id, err)
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

func TestConsignmentStore_Upsert_PreservesCreatedAtAndData(t *testing.T) {
	store := newTestStore(t)

	// Seed directly with a fixed, distinctive CreatedAt and Data
	fixedCreatedAt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	initialData := JSONB{"traderCompanyName": "INITIAL CORP"}
	if err := store.db.Create(&ConsignmentRecord{ID: "wf-created", Status: "PENDING", Data: initialData, CreatedAt: fixedCreatedAt}).Error; err != nil {
		t.Fatalf("failed to seed consignment: %v", err)
	}

	// Re-upsert as if a second application landed in the same consignment.
	tx := store.db.Begin()
	if err := store.UpsertWithData(tx, "wf-created", "APPROVED", JSONB{"traderCompanyName": "NEW CORP"}); err != nil {
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
	if got.Data["traderCompanyName"] != "INITIAL CORP" {
		t.Errorf("expected Data to be preserved as INITIAL CORP, got %v", got.Data["traderCompanyName"])
	}
}

func TestConsignmentStore_GetDataAndFillData(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.GetData(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetData(missing): got %v, want ErrNotFound", err)
	}

	seedConsignment(t, store, "wf-empty", "PENDING")
	data, err := store.GetData(ctx, "wf-empty")
	if err != nil {
		t.Fatalf("GetData(empty): %v", err)
	}
	if data != nil {
		t.Fatalf("expected nil data for unfilled row, got %v", data)
	}

	extras := JSONB{"traderCompanyName": "ADAM PVT LTD"}
	if err := store.FillData(ctx, "wf-empty", extras); err != nil {
		t.Fatalf("FillData: %v", err)
	}
	data, err = store.GetData(ctx, "wf-empty")
	if err != nil {
		t.Fatalf("GetData after fill: %v", err)
	}
	if data["traderCompanyName"] != "ADAM PVT LTD" {
		t.Errorf("expected ADAM PVT LTD, got %v", data["traderCompanyName"])
	}

	if err := store.FillData(ctx, "wf-empty", JSONB{"traderCompanyName": "OTHER"}); err != nil {
		t.Fatalf("FillData overwrite attempt: %v", err)
	}
	data, err = store.GetData(ctx, "wf-empty")
	if err != nil {
		t.Fatalf("GetData after overwrite attempt: %v", err)
	}
	if data["traderCompanyName"] != "ADAM PVT LTD" {
		t.Errorf("FillData must not overwrite existing extras, got %v", data["traderCompanyName"])
	}
}

func TestConsignmentStore_MergeData(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	seedConsignment(t, store, "wf-merge", "PENDING")
	if err := store.MergeData(ctx, "wf-merge", JSONB{
		"exporterRegistrationNo": "SLTB/EXP/2026/0498",
		"cusdecNumber":           "CUSDEC-2026-778120",
	}); err != nil {
		t.Fatalf("MergeData form fields: %v", err)
	}
	if err := store.MergeData(ctx, "wf-merge", JSONB{"traderCompanyName": "ADAM PVT LTD"}); err != nil {
		t.Fatalf("MergeData company name: %v", err)
	}

	data, err := store.GetData(ctx, "wf-merge")
	if err != nil {
		t.Fatalf("GetData: %v", err)
	}
	if data["exporterRegistrationNo"] != "SLTB/EXP/2026/0498" {
		t.Errorf("expected exporter registration, got %v", data["exporterRegistrationNo"])
	}
	if data["cusdecNumber"] != "CUSDEC-2026-778120" {
		t.Errorf("expected CUSDEC, got %v", data["cusdecNumber"])
	}
	if data["traderCompanyName"] != "ADAM PVT LTD" {
		t.Errorf("expected ADAM PVT LTD, got %v", data["traderCompanyName"])
	}

	if err := store.MergeData(ctx, "wf-merge", JSONB{"traderCompanyName": "OTHER"}); err != nil {
		t.Fatalf("MergeData overwrite: %v", err)
	}
	data, err = store.GetData(ctx, "wf-merge")
	if err != nil {
		t.Fatalf("GetData after overwrite: %v", err)
	}
	if data["traderCompanyName"] != "OTHER" {
		t.Errorf("MergeData should overwrite, got %v", data["traderCompanyName"])
	}
}

func TestConsignmentStore_List_OverlaysFormFieldsFromApplication(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	seedConsignmentWithData(t, store, "wf-overlay", "PENDING", JSONB{
		"traderCompanyName": "ADAM PVT LTD",
	}, "t1")
	if err := store.db.Model(&testApplicationRow{}).Where("task_id = ?", "t1").Update("data", JSONB{
		"exporter_registration_no": "SLTB/EXP/2026/0498",
		"cusdec_number":            "CUSDEC-2026-778120",
	}).Error; err != nil {
		t.Fatalf("seed application data: %v", err)
	}

	summaries, _, err := store.List(ctx, "", 0, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].TraderCompanyName != "ADAM PVT LTD" {
		t.Errorf("expected ADAM PVT LTD, got %q", summaries[0].TraderCompanyName)
	}
	if summaries[0].ExporterRegistrationNo != "SLTB/EXP/2026/0498" {
		t.Errorf("expected exporter registration from application data, got %q", summaries[0].ExporterRegistrationNo)
	}
	if summaries[0].CusdecNumber != "CUSDEC-2026-778120" {
		t.Errorf("expected CUSDEC from application data, got %q", summaries[0].CusdecNumber)
	}

	got, err := store.Get(ctx, "wf-overlay")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ExporterRegistrationNo != "SLTB/EXP/2026/0498" || got.CusdecNumber != "CUSDEC-2026-778120" {
		t.Errorf("Get overlay failed: %+v", got)
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

func TestConsignmentStore_Get(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedConsignmentWithData(t, store, "wf-get", "PENDING", JSONB{"traderCompanyName": "ACME LTD"}, "t1", "t2")

	got, err := store.Get(ctx, "wf-get")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ConsignmentID != "wf-get" || got.TaskCount != 2 || got.Status != "PENDING" || got.TraderCompanyName != "ACME LTD" {
		t.Errorf("unexpected summary: %+v", got)
	}

	if _, err := store.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
