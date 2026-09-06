// Package refidstore selects the refid.SequenceStore backend matching a GORM
// connection's dialect, so callers wire reference ID generation without
// branching on the driver themselves.
package refidstore

import (
	"context"
	"fmt"

	"github.com/OpenNSW/core/refid"
	refidpg "github.com/OpenNSW/core/refid/store/postgres"
	refidsqlite "github.com/OpenNSW/core/refid/store/sqlite"
	"gorm.io/gorm"
)

// New returns a refid.SequenceStore that shares db's existing connection pool.
// Reusing the pool matters for SQLite: a second sql.Open on ":memory:" is a
// different database entirely, and on a file it is a second writer competing
// for the same lock.
//
// The refid_sequences table it reads and writes is created by this repo's own
// migrations, not by refid's Migrate helpers.
func New(db *gorm.DB) (refid.SequenceStore, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("refidstore: failed to get sql.DB from gorm: %w", err)
	}

	// db.Name() is the dialector name, "postgres" or "sqlite" — the same
	// values pkg/jsonquery switches on.
	switch name := db.Name(); name {
	case "postgres":
		return refidpg.New(sqlDB)
	case "sqlite":
		return refidsqlite.New(sqlDB)
	default:
		return nil, fmt.Errorf("refidstore: unsupported driver %q", name)
	}
}

// Disabled returns a Registry for a deployment with no refIDGen section, where
// there is no format to generate from. Every Generate fails, so a task
// declaring a refid block is a loud misconfiguration rather than a silent
// no-op — and a value rather than a nil Registry keeps callers'
// non-nil-dependency invariants intact (see application.NewService).
func Disabled() refid.Registry { return disabledRegistry{} }

type disabledRegistry struct{}

// Generate wraps ErrUnknownIssuer so callers classifying refid errors need no
// special case; the message names the cause, which the sentinel alone doesn't.
func (disabledRegistry) Generate(_ context.Context, issuer, idType string, _ map[string]string) (string, error) {
	return "", fmt.Errorf("%w: no refIDGen section is configured for this deployment, so (%q, %q) cannot be generated",
		refid.ErrUnknownIssuer, issuer, idType)
}
