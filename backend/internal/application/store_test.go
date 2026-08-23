package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenNSW/nsw-agency/backend/internal/consignment"
	"github.com/OpenNSW/nsw-agency/backend/internal/database"
	"github.com/OpenNSW/nsw-agency/backend/internal/feedback"
	"github.com/OpenNSW/nsw-agency/backend/internal/rbac"
)

// ---------- helpers ----------

func testEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// newTestStore creates an ApplicationStore for tests.
// When AGENCY_DB_DRIVER=postgres (set via env), it connects to the configured
// PostgreSQL instance and truncates the table before each test.
// Otherwise it falls back to an in-memory SQLite database.
func newTestStore(t *testing.T) *ApplicationStore {
	t.Helper()

	var dbCfg database.Config
	if os.Getenv("AGENCY_DB_DRIVER") == "postgres" {
		password := os.Getenv("DB_PASSWORD")
		if password == "" {
			t.Fatal("DB_PASSWORD is required for postgres driver")
		}
		dbCfg = database.Config{
			Driver: "postgres",
			Postgres: database.PostgresConfig{
				Host:     testEnvOrDefault("DB_HOST", "localhost"),
				Port:     testEnvOrDefault("DB_PORT", "5432"),
				User:     testEnvOrDefault("DB_USER", "postgres"),
				Password: password,
				Name:     testEnvOrDefault("DB_NAME", "nsw_agency_db"),
				SSLMode:  testEnvOrDefault("DB_SSLMODE", "disable"),
			},
		}
	} else {
		dbCfg = database.Config{Driver: "sqlite", SQLite: database.SQLiteConfig{Path: ":memory:"}}
	}

	store, err := NewApplicationStore(dbCfg)
	if err != nil {
		t.Fatalf("failed to create store (driver=%s): %v", dbCfg.Driver, err)
	}

	if err := store.db.AutoMigrate(&consignment.ConsignmentRecord{}, &ApplicationRecord{}, &rbac.RoleRecord{}, &rbac.UserRoleRecord{}); err != nil {
		t.Fatalf("failed to migrate schema: %v", err)
	}

	// For persistent backends, clean tables before each test.
	if dbCfg.Driver != "sqlite" || dbCfg.SQLite.Path != ":memory:" {
		if err := store.db.Exec("TRUNCATE TABLE applications").Error; err != nil {
			t.Fatalf("failed to truncate applications table: %v", err)
		}
		if err := store.db.Exec("TRUNCATE TABLE consignments CASCADE").Error; err != nil {
			t.Fatalf("failed to truncate consignments table: %v", err)
		}
	}

	return store
}

// seedRecord inserts a minimal ApplicationRecord and fails the test on error.
func seedRecord(t *testing.T, store *ApplicationStore, taskID string, data JSONB) {
	t.Helper()
	if data == nil {
		data = JSONB{"key": "value"}
	}
	err := store.CreateOrUpdate(&ApplicationRecord{
		TaskID:        taskID,
		TaskCode:      "verification:123",
		ConsignmentID: "wf-seed",
		ServiceURL:    "http://test",
		Data:          data,
		Status:        "PENDING",
	})
	if err != nil {
		t.Fatalf("seedRecord(%s) failed: %v", taskID, err)
	}
}

// ---------- 1. Integration Testing: SQLite Connectivity ----------

func TestApplicationStore_SQLite_FileCreated(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_agency.db")

	store, err := NewApplicationStore(database.Config{Driver: "sqlite", SQLite: database.SQLiteConfig{Path: dbPath}})
	if err != nil {
		t.Fatalf("NewApplicationStore failed: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("expected .db file to be created at configured DBPath")
	}
}

// ---------- 2. Functional Testing: CRUD Operations ----------

func TestApplicationStore_CreateAndRetrieve(t *testing.T) {
	store := newTestStore(t)
	seedRecord(t, store, "task-crud-1", JSONB{"key": "value"})

	fetched, err := store.GetByTaskID("task-crud-1")
	if err != nil {
		t.Fatalf("GetByTaskID failed: %v", err)
	}
	if fetched.TaskID != "task-crud-1" {
		t.Errorf("expected TaskID 'task-crud-1', got %q", fetched.TaskID)
	}
	if fetched.Status != "PENDING" {
		t.Errorf("expected Status 'PENDING', got %q", fetched.Status)
	}
	if fetched.Data["key"] != "value" {
		t.Errorf("expected Data['key'] = 'value', got %v", fetched.Data["key"])
	}
}

func TestApplicationStore_GetByTaskID_NotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetByTaskID("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent task ID")
	}
}

func TestApplicationStore_UpdateStatus(t *testing.T) {
	store := newTestStore(t)
	seedRecord(t, store, "task-status-1", nil)

	if err := store.UpdateStatus("task-status-1", "APPROVED", map[string]any{"reason": "ok"}); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	app, _ := store.GetByTaskID("task-status-1")
	if app.Status != "APPROVED" {
		t.Errorf("expected Status 'APPROVED', got %q", app.Status)
	}
	if app.ReviewedAt == nil {
		t.Error("expected ReviewedAt to be set after status update")
	}
}

func TestApplicationStore_UpdateStatus_NotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.UpdateStatus("nonexistent", "APPROVED", map[string]any{})
	if err == nil {
		t.Error("expected error when updating non-existent task")
	}
}

func TestApplicationStore_Delete(t *testing.T) {
	store := newTestStore(t)
	seedRecord(t, store, "task-delete-1", nil)

	if err := store.Delete("task-delete-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, err := store.GetByTaskID("task-delete-1")
	if err == nil {
		t.Error("expected error after deleting task")
	}
}

// ---------- 3. Functional Testing: JSONB Serialization ----------

func TestApplicationStore_JSONB_DeepNesting(t *testing.T) {
	store := newTestStore(t)

	deepData := JSONB{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": "deep_value",
				"array":  []any{"a", "b", "c"},
			},
		},
		"boolean": true,
		"number":  42.5,
	}

	seedRecord(t, store, "task-jsonb-1", deepData)

	fetched, err := store.GetByTaskID("task-jsonb-1")
	if err != nil {
		t.Fatalf("GetByTaskID failed: %v", err)
	}

	// Verify deep nesting round-trip
	level1, ok := fetched.Data["level1"].(map[string]any)
	if !ok {
		t.Fatalf("expected level1 to be map, got %T", fetched.Data["level1"])
	}
	level2, ok := level1["level2"].(map[string]any)
	if !ok {
		t.Fatalf("expected level2 to be map, got %T", level1["level2"])
	}
	if level2["level3"] != "deep_value" {
		t.Errorf("expected level3 = 'deep_value', got %v", level2["level3"])
	}

	// Verify array round-trip
	arr, ok := level2["array"].([]any)
	if !ok {
		t.Fatalf("expected array to be []any, got %T", level2["array"])
	}
	if len(arr) != 3 || arr[0] != "a" {
		t.Errorf("expected array [a,b,c], got %v", arr)
	}

	// Verify numeric round-trip (JSON numbers are float64 in Go)
	if fetched.Data["number"] != 42.5 {
		t.Errorf("expected number = 42.5, got %v", fetched.Data["number"])
	}

	// Verify boolean round-trip
	if fetched.Data["boolean"] != true {
		t.Errorf("expected boolean = true, got %v", fetched.Data["boolean"])
	}
}

func TestApplicationStore_JSONB_NilData(t *testing.T) {
	store := newTestStore(t)

	err := store.CreateOrUpdate(&ApplicationRecord{
		TaskID:        "task-nil-data",
		TaskCode:      "verification:123",
		ConsignmentID: "wf-1",
		ServiceURL:    "http://test",
		Data:          nil,
	})
	if err != nil {
		t.Fatalf("CreateOrUpdate with nil JSONB failed: %v", err)
	}

	fetched, _ := store.GetByTaskID("task-nil-data")
	if fetched.Data != nil {
		t.Errorf("expected nil Data, got %v", fetched.Data)
	}
}

// ---------- 4. Functional Testing: Pagination ----------

func TestApplicationStore_List_Pagination(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Seed 5 records with different statuses
	for i := 0; i < 3; i++ {
		seedRecord(t, store, fmt.Sprintf("task-pend-%d", i), nil)
	}
	for i := 0; i < 2; i++ {
		taskID := fmt.Sprintf("task-approved-%d", i)
		seedRecord(t, store, taskID, nil)
		_ = store.UpdateStatus(taskID, "APPROVED", map[string]any{})
	}

	// List all
	apps, total, err := store.List(ctx, "", "", "", "", 0, 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(apps) != 5 {
		t.Errorf("expected 5 apps, got %d", len(apps))
	}

	// List with status filter
	_, total, err = store.List(ctx, "APPROVED", "", "", "", 0, 10)
	if err != nil {
		t.Fatalf("List with status filter failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 approved, got %d", total)
	}

	// List with pagination
	apps, _, err = store.List(ctx, "", "", "", "", 0, 2)
	if err != nil {
		t.Fatalf("List with limit failed: %v", err)
	}
	if len(apps) != 2 {
		t.Errorf("expected 2 apps with limit=2, got %d", len(apps))
	}

	// List with offset
	apps, _, err = store.List(ctx, "", "", "", "", 3, 10)
	if err != nil {
		t.Fatalf("List with offset failed: %v", err)
	}
	if len(apps) != 2 {
		t.Errorf("expected 2 apps with offset=3, got %d", len(apps))
	}
}

func TestApplicationStore_List_OrderingPriority(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Seed records out of order
	_ = store.CreateOrUpdate(&ApplicationRecord{TaskID: "task-done", TaskCode: "test", ConsignmentID: "wf-order", ServiceURL: "http://test", Status: "DONE"})
	_ = store.CreateOrUpdate(&ApplicationRecord{TaskID: "task-feedback", TaskCode: "test", ConsignmentID: "wf-order", ServiceURL: "http://test", Status: "FEEDBACK_REQUESTED"})
	_ = store.CreateOrUpdate(&ApplicationRecord{TaskID: "task-pending", TaskCode: "test", ConsignmentID: "wf-order", ServiceURL: "http://test", Status: "PENDING"})

	apps, _, err := store.List(ctx, "", "wf-order", "", "", 0, 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(apps) != 3 {
		t.Fatalf("expected 3 apps, got %d", len(apps))
	}

	if apps[0].TaskID != "task-pending" {
		t.Errorf("expected first app to be task-pending, got %s", apps[0].TaskID)
	}
	if apps[1].TaskID != "task-feedback" {
		t.Errorf("expected second app to be task-feedback, got %s", apps[1].TaskID)
	}
	if apps[2].TaskID != "task-done" {
		t.Errorf("expected third app to be task-done, got %s", apps[2].TaskID)
	}
}

func TestApplicationStore_List_ConsignmentFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	seedRecord(t, store, "t1", nil) // consignment: wf-seed (default from seedRecord)
	seedRecord(t, store, "t2", nil)

	// Create another consignment
	err := store.CreateOrUpdate(&ApplicationRecord{
		TaskID:        "t3",
		ConsignmentID: "wf-custom",
		Status:        "PENDING",
	})
	if err != nil {
		t.Fatalf("failed to seed wf-custom: %v", err)
	}

	// Filter by wf-seed
	apps, total, err := store.List(ctx, "", "wf-seed", "", "", 0, 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 apps for wf-seed, got %d", total)
	}
	if len(apps) != 2 {
		t.Errorf("expected 2 apps returned, got %d", len(apps))
	}

	// Filter by wf-custom
	_, total, err = store.List(ctx, "", "wf-custom", "", "", 0, 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 app for wf-custom, got %d", total)
	}
}

// ---------- 5. Functional Testing: Feedback & Transactions ----------

func TestApplicationStore_AppendFeedback(t *testing.T) {
	store := newTestStore(t)
	seedRecord(t, store, "task-fb-1", nil)

	feedback1 := feedback.Entry{Content: map[string]any{"comment": "needs revision"}, Round: 1}
	if err := store.AppendFeedback("task-fb-1", feedback1); err != nil {
		t.Fatalf("AppendFeedback round 1 failed: %v", err)
	}

	app, _ := store.GetByTaskID("task-fb-1")
	if app.Status != "FEEDBACK_REQUESTED" {
		t.Errorf("expected FEEDBACK_REQUESTED after feedback, got %q", app.Status)
	}
	if len(app.AgencyFeedbackHistory) != 1 {
		t.Errorf("expected 1 feedback entry, got %d", len(app.AgencyFeedbackHistory))
	}

	// Append a second round
	feedback2 := feedback.Entry{Content: map[string]any{"comment": "still needs work"}, Round: 2}
	if err := store.AppendFeedback("task-fb-1", feedback2); err != nil {
		t.Fatalf("AppendFeedback round 2 failed: %v", err)
	}

	app, _ = store.GetByTaskID("task-fb-1")
	if len(app.AgencyFeedbackHistory) != 2 {
		t.Errorf("expected 2 feedback entries, got %d", len(app.AgencyFeedbackHistory))
	}
	if app.AgencyFeedbackHistory[1].Content["comment"] != "still needs work" {
		t.Errorf("unexpected second feedback comment: %v", app.AgencyFeedbackHistory[1])
	}
}

func TestApplicationStore_AppendFeedback_NonExistent(t *testing.T) {
	store := newTestStore(t)

	err := store.AppendFeedback("nonexistent", feedback.Entry{Content: map[string]any{"comment": "nope"}})
	if err == nil {
		t.Error("expected error for feedback on non-existent task")
	}
}

// ---------- 6. Functional Testing: Resubmission Flow ----------

func TestApplicationStore_UpdateDataAndResetStatus(t *testing.T) {
	store := newTestStore(t)
	seedRecord(t, store, "task-resub-1", JSONB{"old": "data"})

	// Simulate Agency requesting feedback
	_ = store.AppendFeedback("task-resub-1", feedback.Entry{Content: map[string]any{"comment": "fix it"}})

	app, _ := store.GetByTaskID("task-resub-1")
	if app.Status != "FEEDBACK_REQUESTED" {
		t.Fatalf("expected FEEDBACK_REQUESTED, got %q", app.Status)
	}

	// Simulate trader resubmission
	newData := map[string]any{"new": "data", "updated": true}
	if err := store.UpdateDataAndResetStatus("task-resub-1", newData); err != nil {
		t.Fatalf("UpdateDataAndResetStatus failed: %v", err)
	}

	app, _ = store.GetByTaskID("task-resub-1")
	if app.Status != "PENDING" {
		t.Errorf("expected PENDING after resubmission, got %q", app.Status)
	}
	if app.Data["new"] != "data" {
		t.Errorf("expected updated data, got %v", app.Data)
	}
}

// ---------- 7. Functional Testing: Consignment Table ----------

func TestApplicationStore_ConsignmentUpsert(t *testing.T) {
	store := newTestStore(t)

	// Two CreateOrUpdate calls with the same consignment_id should result in one consignment row.
	if err := store.CreateOrUpdate(&ApplicationRecord{
		TaskID:        "dup-t1",
		TaskCode:      "test",
		ConsignmentID: "dup-wf",
		ServiceURL:    "http://test",
		Status:        "PENDING",
	}); err != nil {
		t.Fatalf("first CreateOrUpdate failed: %v", err)
	}
	if err := store.CreateOrUpdate(&ApplicationRecord{
		TaskID:        "dup-t2",
		TaskCode:      "test",
		ConsignmentID: "dup-wf",
		ServiceURL:    "http://test",
		Status:        "PENDING",
	}); err != nil {
		t.Fatalf("second CreateOrUpdate failed: %v", err)
	}

	var count int64
	if err := store.db.Model(&consignment.ConsignmentRecord{}).Where("id = ?", "dup-wf").Count(&count).Error; err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 consignment row for dup-wf, got %d", count)
	}
}

func TestApplicationStore_UpdateStatus_PropagatesConsignment(t *testing.T) {
	store := newTestStore(t)
	seedRecord(t, store, "task-prop-1", nil)

	if err := store.UpdateStatus("task-prop-1", "APPROVED", map[string]any{}); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	var cr consignment.ConsignmentRecord
	if err := store.db.First(&cr, "id = ?", "wf-seed").Error; err != nil {
		t.Fatalf("failed to fetch consignment: %v", err)
	}
	if cr.Status != "APPROVED" {
		t.Errorf("expected consignment status 'APPROVED', got %q", cr.Status)
	}
}
