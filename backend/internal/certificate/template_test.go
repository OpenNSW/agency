package certificate

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
)

func TestLoad(t *testing.T) {
	t.Run("returns the raw bytes of an executable template", func(t *testing.T) {
		m := testutil.MemLoader{
			"certificates/welcome.gohtml": []byte(`Hello, {{.Name}}!`),
		}
		reg := artifact.NewRegistry(m)
		reg.RegisterArtifact("welcome", Kind, "", "certificates/welcome.gohtml")

		raw, err := Load(context.Background(), reg, "welcome")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if string(raw) != `Hello, {{.Name}}!` {
			t.Errorf("unexpected raw bytes: %q", raw)
		}
	})

	t.Run("a template using fromData/fromReview/today validates at load time", func(t *testing.T) {
		m := testutil.MemLoader{
			"certificates/cert.gohtml": []byte(`{{fromData "task_v1" "field"}}|{{fromReview "task_v1" "field"}}|{{today}}`),
		}
		reg := artifact.NewRegistry(m)
		reg.RegisterArtifact("cert", Kind, "", "certificates/cert.gohtml")

		if _, err := Load(context.Background(), reg, "cert"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("a call to an undeclared function fails to load", func(t *testing.T) {
		m := testutil.MemLoader{
			"certificates/cert.gohtml": []byte(`{{fromSomewhereElse "task_v1" "field"}}`),
		}
		reg := artifact.NewRegistry(m)
		reg.RegisterArtifact("cert", Kind, "", "certificates/cert.gohtml")

		if _, err := Load(context.Background(), reg, "cert"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("malformed template returns error", func(t *testing.T) {
		m := testutil.MemLoader{
			"certificates/broken.gohtml": []byte(`Hello, {{.Name`),
		}
		reg := artifact.NewRegistry(m)
		reg.RegisterArtifact("broken", Kind, "", "certificates/broken.gohtml")

		_, err := Load(context.Background(), reg, "broken")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing template returns ErrNotFound", func(t *testing.T) {
		reg := artifact.NewRegistry(testutil.MemLoader{})

		_, err := Load(context.Background(), reg, "missing")
		if !errors.Is(err, artifact.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}
