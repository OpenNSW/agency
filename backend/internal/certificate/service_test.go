package certificate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/OpenNSW/nsw-agency/backend/internal/application"
)

func TestServiceGenerate(t *testing.T) {
	t.Run("populates the template with data", func(t *testing.T) {
		m := testutil.MemLoader{
			"certificates/welcome.gohtml": []byte(`Congratulations, {{.Name}}!`),
		}
		reg := artifact.NewRegistry(m)
		reg.RegisterArtifact("welcome", Kind, "", "certificates/welcome.gohtml")

		svc := NewService(reg, nil)
		html, err := svc.Generate(context.Background(), "welcome", "", map[string]any{"Name": "Officer"})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if html != "Congratulations, Officer!" {
			t.Errorf("expected %q, got %q", "Congratulations, Officer!", html)
		}
	})

	t.Run("missing template surfaces ErrNotFound", func(t *testing.T) {
		reg := artifact.NewRegistry(testutil.MemLoader{})
		svc := NewService(reg, nil)

		_, err := svc.Generate(context.Background(), "missing", "", nil)
		if !errors.Is(err, artifact.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("execution error is surfaced", func(t *testing.T) {
		m := testutil.MemLoader{
			"certificates/strict.gohtml": []byte(`{{.Count.Field}}`),
		}
		reg := artifact.NewRegistry(m)
		reg.RegisterArtifact("strict", Kind, "", "certificates/strict.gohtml")

		svc := NewService(reg, nil)
		_, err := svc.Generate(context.Background(), "strict", "", map[string]any{"Count": 5})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "execute certificate template") {
			t.Errorf("expected execution error, got %v", err)
		}
	})

	t.Run("resolves fromData/fromReview/today, with plain data fields passed through untouched", func(t *testing.T) {
		m := testutil.MemLoader{
			"certificates/cert.gohtml": []byte(
				`{{fromData "fcau_application_review_v1" "exporter_name"}}|` +
					`{{fromData "fcau_application_review_v1" "exporter_address"}}|` +
					`{{fromReview "fcau_application_review_v1" "reference_number"}}|` +
					`{{.certificate_id}}`,
			),
		}
		reg := artifact.NewRegistry(m)
		reg.RegisterArtifact("cert", Kind, "", "certificates/cert.gohtml")

		lookup := &fakeApplicationLookup{
			items: []application.Application{
				{
					TaskID:   "task-1",
					TaskCode: "fcau_application_review_v1",
					Data: map[string]any{
						"exporter_name":    "STAY NATURALS PRIVATE LIMITED",
						"exporter_address": "MATALE",
					},
					AgencyActionData: map[string]any{
						"reference_number": "034/00481",
					},
				},
			},
		}
		svc := NewService(reg, lookup)

		html, err := svc.Generate(context.Background(), "cert", "CONSIGNMENT-1", map[string]any{
			"certificate_id": "CERT-FCAU-9024",
		})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		want := "STAY NATURALS PRIVATE LIMITED|MATALE|034/00481|CERT-FCAU-9024"
		if html != want {
			t.Errorf("expected %q, got %q", want, html)
		}
	})

	t.Run("a task absent from the consignment leaves fromData/fromReview calls blank, not erroring", func(t *testing.T) {
		m := testutil.MemLoader{
			"certificates/cert.gohtml": []byte(`[{{fromData "fcau_application_review_v1" "exporter_name"}}]`),
		}
		reg := artifact.NewRegistry(m)
		reg.RegisterArtifact("cert", Kind, "", "certificates/cert.gohtml")

		svc := NewService(reg, &fakeApplicationLookup{})

		html, err := svc.Generate(context.Background(), "cert", "CONSIGNMENT-1", nil)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if html != "[]" {
			t.Errorf("expected %q, got %q", "[]", html)
		}
	})
}
