package certificate

import (
	"bytes"
	"context"
	"fmt"
	"html/template"

	"github.com/OpenNSW/core/artifact"
)

// Service generates certificates by populating a named template with data.
type Service interface {
	// Generate fetches the template with templateID, executes it against data
	// (available as the template's dot value) and, when consignmentID is set,
	// the consignment's application history (available via the fromData/
	// fromReview template functions), and returns the populated HTML.
	Generate(ctx context.Context, templateID, consignmentID string, data map[string]any) (string, error)
}

// service is the default Service backed by the shared artifact registry and,
// optionally, a consignment's application history.
type service struct {
	registry     *artifact.Registry
	applications ApplicationLookup
}

// NewService creates a certificate Service backed by registry and applications.
func NewService(registry *artifact.Registry, applications ApplicationLookup) *service {
	return &service{registry: registry, applications: applications}
}

// Generate implements Service.
func (s *service) Generate(ctx context.Context, templateID, consignmentID string, data map[string]any) (string, error) {
	raw, err := Load(ctx, s.registry, templateID)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New(templateID).Funcs(realFuncs(ctx, s.applications, consignmentID)).Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parse certificate template %q: %w", templateID, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute certificate template %q: %w", templateID, err)
	}
	return buf.String(), nil
}
