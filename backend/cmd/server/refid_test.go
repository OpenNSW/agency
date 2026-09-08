package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/OpenNSW/core/refid"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestInitRefIDs_NotConfigured passes a nil *gorm.DB on purpose: with no
// refIDGen section the database must never be reached, so a nil handle is
// safe. Generate still fails, which is what makes a task declaring refid on
// such a deployment a loud misconfiguration rather than a silent no-op.
func TestInitRefIDs_NotConfigured(t *testing.T) {
	registry, err := initRefIDs(refid.Config{}, nil)
	if err != nil {
		t.Fatalf("initRefIDs with no issuers: %v", err)
	}
	if registry == nil {
		t.Fatal("initRefIDs returned a nil Registry; NewService panics on nil dependencies")
	}
	if _, err := registry.Generate(context.Background(), "NPQS", "application_id", nil); !errors.Is(err, refid.ErrUnknownIssuer) {
		t.Errorf("Generate returned %v, want an error wrapping refid.ErrUnknownIssuer", err)
	}
}

func TestInitRefIDs_Configured(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	registry, err := initRefIDs(refid.Config{
		Issuers: []refid.IssuerConfig{{
			Issuer: "NPQS",
			Formats: []refid.FormatConfig{{
				IDType:   "application_id",
				Segments: []refid.SegmentConfig{{Type: "literal", Value: "NPQS/"}},
			}},
		}},
	}, db)
	if err != nil {
		t.Fatalf("initRefIDs: %v", err)
	}
	if _, err := registry.Generate(context.Background(), "NPQS", "application_id", nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}
}

// TestInitRefIDs_MalformedConfig pins the fail-at-boot behaviour: a bad format
// must be rejected here rather than at the first inject that needs it.
func TestInitRefIDs_MalformedConfig(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	// A sequence segment with no scopeKey is what NewRegistry rejects.
	if _, err := initRefIDs(refid.Config{
		Issuers: []refid.IssuerConfig{{
			Issuer: "NPQS",
			Formats: []refid.FormatConfig{{
				IDType:   "application_id",
				Segments: []refid.SegmentConfig{{Type: "sequence", Padding: 6}},
			}},
		}},
	}, db); err == nil {
		t.Fatal("initRefIDs accepted a sequence segment with no scopeKey, want an error")
	}
}
