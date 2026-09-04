package refidstore_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/OpenNSW/agency/backend/internal/refidstore"
	"github.com/OpenNSW/core/refid"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// refidSequencesDDL mirrors the sqlite branch of
// migrations/000010_create_refid_sequences.sql. Unit tests don't replay the
// migrator, so the table is created here — keep the two in sync.
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

// TestRegistry_GeneratesFullID drives a real refid config end to end, so the
// padding, list validation and scope-key resolution are all exercised against
// this module's driver rather than just the raw counter.
func TestRegistry_GeneratesFullID(t *testing.T) {
	cfg := refid.Config{
		Issuers: []refid.IssuerConfig{{
			Issuer: "NPQS",
			Formats: []refid.FormatConfig{{
				IDType: "application_id",
				Segments: []refid.SegmentConfig{
					{Type: "literal", Value: "NPQS/"},
					{Type: "list", List: "office_location", Param: "officeCode"},
					{Type: "literal", Value: "/"},
					{Type: "sequence", ScopeKey: "{issuer}:{idType}:{officeCode}:{yyyy}", Padding: 6},
				},
			}},
		}},
		Lists: map[string][]string{"office_location": {"NPQS-KAT", "SEA-CMB"}},
	}

	reg, err := refid.NewRegistry(cfg, newTestStore(t))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	ctx := context.Background()

	got, err := reg.Generate(ctx, "NPQS", "application_id", map[string]string{"officeCode": "NPQS-KAT"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "NPQS/NPQS-KAT/000001"; got != want {
		t.Fatalf("Generate returned %q, want %q", got, want)
	}

	// A value outside the configured list must not reach the counter.
	if _, err := reg.Generate(ctx, "NPQS", "application_id", map[string]string{"officeCode": "NOPE"}); !errors.Is(err, refid.ErrInvalidParam) {
		t.Fatalf("Generate with unlisted office returned %v, want refid.ErrInvalidParam", err)
	}

	// ... and the next valid call is 2, not 3 — the rejected call was side-effect free.
	got, err = reg.Generate(ctx, "NPQS", "application_id", map[string]string{"officeCode": "NPQS-KAT"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "NPQS/NPQS-KAT/000002"; got != want {
		t.Fatalf("Generate returned %q, want %q", got, want)
	}
}
