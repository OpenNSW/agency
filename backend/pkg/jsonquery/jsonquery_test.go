package jsonquery

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPredicate_Postgres(t *testing.T) {
	tests := []struct {
		name    string
		column  string
		pointer string
		value   any
		wantSQL string
		wantArg any
	}{
		{
			name:    "single segment string value",
			column:  "custom_data",
			pointer: "/district",
			value:   "Colombo",
			wantSQL: `custom_data#>'{district}' = ?::jsonb`,
			wantArg: `"Colombo"`,
		},
		{
			name:    "nested segments",
			column:  "consignments.custom_data",
			pointer: "/location/district",
			value:   "Colombo",
			wantSQL: `consignments.custom_data#>'{location,district}' = ?::jsonb`,
			wantArg: `"Colombo"`,
		},
		{
			name:    "numeric value",
			column:  "custom_data",
			pointer: "/count",
			value:   float64(3),
			wantSQL: `custom_data#>'{count}' = ?::jsonb`,
			wantArg: `3`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, arg, err := Predicate("postgres", tt.column, tt.pointer, tt.value)
			if err != nil {
				t.Fatalf("Predicate returned error: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("sql = %q, want %q", sql, tt.wantSQL)
			}
			if arg != tt.wantArg {
				t.Errorf("arg = %v, want %v", arg, tt.wantArg)
			}
		})
	}
}

func TestPredicate_SQLite(t *testing.T) {
	sql, arg, err := Predicate("sqlite", "custom_data", "/location/district", "Colombo")
	if err != nil {
		t.Fatalf("Predicate returned error: %v", err)
	}
	wantSQL := `json_extract(custom_data, '$.location.district') = ?`
	if sql != wantSQL {
		t.Errorf("sql = %q, want %q", sql, wantSQL)
	}
	if arg != "Colombo" {
		t.Errorf("arg = %v, want %q", arg, "Colombo")
	}
}

func TestPredicate_InvalidSegment(t *testing.T) {
	tests := []string{
		"/a'; DROP TABLE users; --",
		"/a b",
		"/a.b",
		"/a\"b",
	}
	for _, pointer := range tests {
		if _, _, err := Predicate("postgres", "custom_data", pointer, "x"); err == nil {
			t.Errorf("Predicate(%q) succeeded, want error", pointer)
		}
	}
}

func TestPredicate_UnsupportedDriver(t *testing.T) {
	if _, _, err := Predicate("mysql", "custom_data", "/district", "Colombo"); err == nil {
		t.Error("Predicate with unsupported driver succeeded, want error")
	}
}

func TestPredicate_InvalidPointer(t *testing.T) {
	if _, _, err := Predicate("postgres", "custom_data", "", "x"); err == nil {
		t.Error("Predicate with empty pointer succeeded, want error")
	}
}

// TestPredicate_SQLiteRoundTrip proves json_extract actually works against
// this module's exact SQLite driver (github.com/glebarez/sqlite, wrapping
// modernc.org/sqlite) — the whole feature's correctness on the default
// dev/test driver depends on this.
func TestPredicate_SQLiteRoundTrip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, custom_data TEXT)`).Error; err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	rows := []struct {
		id   string
		data string
	}{
		{"match", `{"location":{"district":"Colombo"}}`},
		{"no-match", `{"location":{"district":"Gampaha"}}`},
		{"missing-field", `{"location":{}}`},
	}
	for _, r := range rows {
		if err := db.Exec(`INSERT INTO items (id, custom_data) VALUES (?, ?)`, r.id, r.data).Error; err != nil {
			t.Fatalf("failed to insert row %q: %v", r.id, err)
		}
	}

	sql, arg, err := Predicate("sqlite", "custom_data", "/location/district", "Colombo")
	if err != nil {
		t.Fatalf("Predicate returned error: %v", err)
	}

	var ids []string
	if err := db.Raw(`SELECT id FROM items WHERE `+sql, arg).Scan(&ids).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(ids) != 1 || ids[0] != "match" {
		t.Errorf("matching ids = %v, want [match]", ids)
	}
}
