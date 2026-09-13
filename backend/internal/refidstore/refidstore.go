// Package refidstore selects the refid store backends matching a GORM
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

// Stores holds one backend per stateful segment type. Both are always built:
// which of them a deployment uses is its refIDGen config's decision, not the
// binary's, and an unused one costs a struct and a query string — no
// connection, no round trip.
type Stores struct {
	Sequence refid.SequenceStore
	Random   refid.RandomStore
}

// New returns stores that share db's existing connection pool. Reusing the
// pool matters for SQLite: a second sql.Open on ":memory:" is a different
// database entirely, and on a file it is a second writer competing for the
// same lock.
//
// The refid_sequences and refid_random tables they read and write are created
// by this repo's own migrations, not by refid's Migrate helpers.
func New(db *gorm.DB) (Stores, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return Stores{}, fmt.Errorf("refidstore: failed to get sql.DB from gorm: %w", err)
	}

	var s Stores

	// db.Name() is the dialector name, "postgres" or "sqlite" — the same
	// values pkg/jsonquery switches on.
	switch name := db.Name(); name {
	case "postgres":
		s.Sequence, err = refidpg.NewSequence(sqlDB)
		if err == nil {
			s.Random, err = refidpg.NewRandom(sqlDB)
		}
	case "sqlite":
		s.Sequence, err = refidsqlite.NewSequence(sqlDB)
		if err == nil {
			s.Random, err = refidsqlite.NewRandom(sqlDB)
		}
	default:
		return Stores{}, fmt.Errorf("refidstore: unsupported driver %q", name)
	}
	if err != nil {
		return Stores{}, fmt.Errorf("refidstore: %w", err)
	}
	return s, nil
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
