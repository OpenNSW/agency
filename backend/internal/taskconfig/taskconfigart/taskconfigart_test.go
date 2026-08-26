package taskconfigart

import (
	"context"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
)

func TestLoad_MissingPermissions_Errors(t *testing.T) {
	mem := testutil.MemLoader{
		"alpha.json": []byte(`{"taskCode": "alpha", "meta": {"title": "Alpha"}}`),
	}
	reg := artifact.NewRegistry(mem)
	reg.RegisterArtifact("alpha", Kind, "", "alpha.json")

	if _, err := Load(context.Background(), reg, "alpha"); err == nil {
		t.Error("expected an error loading a task config with no permissions")
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	mem := testutil.MemLoader{
		"alpha.json": []byte(`{
			"schemaVersion": 1,
			"taskCode": "alpha",
			"meta": {"title": "Alpha"},
			"permissions": [{"role": "officer", "actions": ["VIEW"]}]
		}`),
	}
	reg := artifact.NewRegistry(mem)
	reg.RegisterArtifact("alpha", Kind, "", "alpha.json")

	cfg, err := Load(context.Background(), reg, "alpha")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Meta.Title != "Alpha" {
		t.Errorf("Meta.Title: got %q, want %q", cfg.Meta.Title, "Alpha")
	}
}
