package consignment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
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
	raw, err := encodeJSONB(data)
	if err != nil {
		t.Fatalf("encode nsw_data: %v", err)
	}
	tx := store.db.Begin()
	if err := tx.Create(&ConsignmentRecord{ID: id, Status: status, NSWData: raw}).Error; err != nil {
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
	rawNSWData, err := encodeJSONB(JSONB{"traderCompanyName": "INITIAL CORP"})
	if err != nil {
		t.Fatalf("encode nsw_data: %v", err)
	}
	if err := store.db.Create(&ConsignmentRecord{
		ID:        "wf-created",
		Status:    "PENDING",
		NSWData:   rawNSWData,
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
	decoded, err := decodeJSONB(got.NSWData)
	if err != nil {
		t.Fatalf("decode NSWData: %v", err)
	}
	if decoded["traderCompanyName"] != "INITIAL CORP" {
		t.Errorf("expected NSWData to be preserved as INITIAL CORP, got %v", decoded["traderCompanyName"])
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
	data, err := decodeJSONB(rec.NSWData)
	if err != nil {
		t.Fatalf("decode nsw_data: %v", err)
	}
	if data["traderCompanyName"] != "ACME" {
		t.Errorf("Create on conflict must keep original extras, got %v", data["traderCompanyName"])
	}

	if _, err := store.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing): got %v, want ErrNotFound", err)
	}
}

// ---------- MergeCustomData ----------

func mergeInTx(t *testing.T, store *Store, id string, fields map[string]any, schema json.RawMessage) error {
	t.Helper()
	tx := store.db.Begin()
	if err := store.MergeCustomData(tx, id, fields, schema); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	return nil
}

func getCustomData(t *testing.T, store *Store, id string) JSONB {
	t.Helper()
	var rec ConsignmentRecord
	if err := store.db.First(&rec, "id = ?", id).Error; err != nil {
		t.Fatalf("failed to fetch consignment %s: %v", id, err)
	}
	data, err := decodeJSONB(rec.CustomData)
	if err != nil {
		t.Fatalf("decode custom_data: %v", err)
	}
	return data
}

func TestMergeCustomData_FirstMerge(t *testing.T) {
	store := newTestStore(t)
	if err := store.db.Create(&ConsignmentRecord{ID: "c1", Status: "PENDING"}).Error; err != nil {
		t.Fatalf("seed consignment: %v", err)
	}

	if err := mergeInTx(t, store, "c1", map[string]any{"district": "Colombo"}, nil); err != nil {
		t.Fatalf("MergeCustomData: %v", err)
	}

	got := getCustomData(t, store, "c1")
	if got["district"] != "Colombo" {
		t.Errorf("custom_data[district] = %v, want Colombo", got["district"])
	}
}

func TestMergeCustomData_AccumulatesAcrossMerges(t *testing.T) {
	store := newTestStore(t)
	if err := store.db.Create(&ConsignmentRecord{ID: "c1", Status: "PENDING"}).Error; err != nil {
		t.Fatalf("seed consignment: %v", err)
	}

	if err := mergeInTx(t, store, "c1", map[string]any{"district": "Colombo"}, nil); err != nil {
		t.Fatalf("first MergeCustomData: %v", err)
	}
	if err := mergeInTx(t, store, "c1", map[string]any{"portOfEntry": "BIA"}, nil); err != nil {
		t.Fatalf("second MergeCustomData: %v", err)
	}

	got := getCustomData(t, store, "c1")
	if got["district"] != "Colombo" {
		t.Errorf("custom_data[district] = %v, want Colombo (should survive the second merge)", got["district"])
	}
	if got["portOfEntry"] != "BIA" {
		t.Errorf("custom_data[portOfEntry] = %v, want BIA", got["portOfEntry"])
	}
}

func TestMergeCustomData_LastWriteWinsOnRepeatedKey(t *testing.T) {
	store := newTestStore(t)
	if err := store.db.Create(&ConsignmentRecord{ID: "c1", Status: "PENDING"}).Error; err != nil {
		t.Fatalf("seed consignment: %v", err)
	}

	if err := mergeInTx(t, store, "c1", map[string]any{"district": "Colombo"}, nil); err != nil {
		t.Fatalf("first MergeCustomData: %v", err)
	}
	if err := mergeInTx(t, store, "c1", map[string]any{"district": "Gampaha"}, nil); err != nil {
		t.Fatalf("second MergeCustomData: %v", err)
	}

	got := getCustomData(t, store, "c1")
	if got["district"] != "Gampaha" {
		t.Errorf("custom_data[district] = %v, want Gampaha (the later merge should win)", got["district"])
	}
}

func TestMergeCustomData_EmptyFieldsIsNoOp(t *testing.T) {
	store := newTestStore(t)
	if err := store.db.Create(&ConsignmentRecord{ID: "c1", Status: "PENDING"}).Error; err != nil {
		t.Fatalf("seed consignment: %v", err)
	}

	if err := mergeInTx(t, store, "c1", nil, nil); err != nil {
		t.Fatalf("MergeCustomData(nil fields): %v", err)
	}

	var rec ConsignmentRecord
	if err := store.db.First(&rec, "id = ?", "c1").Error; err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(rec.CustomData) != 0 {
		t.Errorf("custom_data = %s, want untouched/empty", rec.CustomData)
	}
}

func TestMergeCustomData_SchemaMismatchSkipsWriteWithoutError(t *testing.T) {
	store := newTestStore(t)
	if err := store.db.Create(&ConsignmentRecord{ID: "c1", Status: "PENDING"}).Error; err != nil {
		t.Fatalf("seed consignment: %v", err)
	}
	// Seed a valid value first, so we can confirm a later failed merge leaves it alone.
	if err := mergeInTx(t, store, "c1", map[string]any{"district": "Colombo"}, nil); err != nil {
		t.Fatalf("seed merge: %v", err)
	}

	schema := json.RawMessage(`{"type":"object","properties":{"district":{"type":"string"}}}`)
	err := mergeInTx(t, store, "c1", map[string]any{"district": 12345}, schema)
	if err != nil {
		t.Fatalf("MergeCustomData with a schema mismatch should not error, got: %v", err)
	}

	got := getCustomData(t, store, "c1")
	if got["district"] != "Colombo" {
		t.Errorf("custom_data[district] = %v, want Colombo (mismatched merge must be skipped, not partially applied)", got["district"])
	}
}

func TestMergeCustomData_ValidAgainstSchemaIsPersisted(t *testing.T) {
	store := newTestStore(t)
	if err := store.db.Create(&ConsignmentRecord{ID: "c1", Status: "PENDING"}).Error; err != nil {
		t.Fatalf("seed consignment: %v", err)
	}

	schema := json.RawMessage(`{"type":"object","properties":{"district":{"type":"string"}}}`)
	if err := mergeInTx(t, store, "c1", map[string]any{"district": "Colombo"}, schema); err != nil {
		t.Fatalf("MergeCustomData: %v", err)
	}

	got := getCustomData(t, store, "c1")
	if got["district"] != "Colombo" {
		t.Errorf("custom_data[district] = %v, want Colombo", got["district"])
	}
}

// TestMergeCustomData_LocksAgainstConcurrentMerges exercises many concurrent
// merges into the same consignment and asserts none are lost. Note this
// doesn't actually prove the row lock (clause.Locking) is *necessary*:
// SQLite serializes writers at the connection/file level regardless of it,
// so a lost-update race isn't constructible against SQLite the way it would
// be against Postgres. What this does verify is that MergeCustomData's
// read-merge-write cycle behaves correctly under concurrent callers, which
// would still catch a bug like reading stale data outside the transaction.
func TestMergeCustomData_LocksAgainstConcurrentMerges(t *testing.T) {
	store := newTestStore(t)
	if err := store.db.Create(&ConsignmentRecord{ID: "c1", Status: "PENDING"}).Error; err != nil {
		t.Fatalf("seed consignment: %v", err)
	}

	// :memory: SQLite databases are per-connection; the default connection
	// pool can hand different goroutines different (empty) databases. Pin to
	// a single connection so they all see the same one.
	sqlDB, err := store.db.DB()
	if err != nil {
		t.Fatalf("failed to get underlying *sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	const n = 10
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("field%d", i)
			errs <- mergeInTx(t, store, "c1", map[string]any{key: i}, nil)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent MergeCustomData: %v", err)
		}
	}

	got := getCustomData(t, store, "c1")
	if len(got) != n {
		t.Fatalf("custom_data has %d keys, want %d (a concurrent merge lost an update): %#v", len(got), n, got)
	}
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("field%d", i)
		if _, ok := got[key]; !ok {
			t.Errorf("custom_data missing %q", key)
		}
	}
}
