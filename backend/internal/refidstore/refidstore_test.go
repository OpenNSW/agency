package refidstore_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenNSW/agency/backend/internal/refidstore"
	"github.com/OpenNSW/core/refid"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// refidSequencesDDL mirrors the sqlite branch of the counter-table migration.
// Unit tests don't replay the migrator, so the table is created here — keep
// the two in sync.
const refidSequencesDDL = `
CREATE TABLE IF NOT EXISTS refid_sequences (
    scope_key  TEXT    NOT NULL PRIMARY KEY,
    counter    INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
)`

// newTestStore builds a SequenceStore over this module's actual SQLite driver
// (github.com/glebarez/sqlite), which is the whole point of these tests:
// refid's queries use RETURNING and ?N ordinal placeholders, and upstream only
// exercises them against modernc.org/sqlite. An on-disk file rather than
// ":memory:" so every pooled connection sees the same database.
func newTestStore(t *testing.T) refid.SequenceStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "refid.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.Exec(refidSequencesDDL).Error; err != nil {
		t.Fatalf("failed to create refid_sequences: %v", err)
	}
	store, err := refidstore.New(db)
	if err != nil {
		t.Fatalf("refidstore.New: %v", err)
	}
	return store
}

func TestNext_StartsAtOneAndIncrements(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for want := int64(1); want <= 3; want++ {
		got, err := store.Next(ctx, "NPQS:application_id:NPQS-KAT:20260904", 999999)
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if got != want {
			t.Fatalf("Next returned %d, want %d", got, want)
		}
	}
}

func TestNext_IsolatesScopeKeys(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Two offices on the same day must not share a counter.
	for range 3 {
		if _, err := store.Next(ctx, "NPQS:application_id:NPQS-KAT:20260904", 999999); err != nil {
			t.Fatalf("Next: %v", err)
		}
	}
	got, err := store.Next(ctx, "NPQS:application_id:SEA-CMB:20260904", 999999)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got != 1 {
		t.Fatalf("second scope key started at %d, want 1", got)
	}
}

func TestNext_CounterOverflow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Next(ctx, "scope", 1); err != nil {
		t.Fatalf("first Next: %v", err)
	}
	_, err := store.Next(ctx, "scope", 1)
	if !errors.Is(err, refid.ErrCounterOverflow) {
		t.Fatalf("Next past max returned %v, want refid.ErrCounterOverflow", err)
	}
}
func TestDisabled_GenerateAlwaysFails(t *testing.T) {
	_, err := refidstore.Disabled().Generate(context.Background(), "NPQS", "application_id", nil)
	if err == nil {
		t.Fatal("Disabled().Generate returned no error, want one")
	}
	// Classified like any other unknown format, so callers mapping refid
	// errors to HTTP statuses need no special case.
	if !errors.Is(err, refid.ErrUnknownIssuer) {
		t.Errorf("error does not wrap refid.ErrUnknownIssuer: %v", err)
	}
	// ...but the message must name the real cause, not just "unknown issuer".
	if !strings.Contains(err.Error(), "no refIDGen section is configured") {
		t.Errorf("error message doesn't explain the cause: %v", err)
	}
}
