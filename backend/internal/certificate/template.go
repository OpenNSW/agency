// Package certificate lets an agency officer populate a certificate template
// with data and get back the resulting HTML. Rendering the HTML to a final
// document (e.g. a PDF) is left to the frontend.
package certificate

import (
	"context"
	"fmt"
	"html/template"

	"github.com/OpenNSW/core/artifact"
)

// Kind is the artifact kind owned by this package.
const Kind artifact.Kind = "certificate_template"

// loadable wraps a certificate template's raw bytes so it satisfies
// artifact.Artifact and artifact.Parser. The template isn't parsed with its
// real functions here — those need per-request consignment data that doesn't
// exist yet at artifact-load time (see fieldmap.go's realFuncs) — but it is
// validated by parsing with stub functions, so a malformed template (or a
// call to an undeclared function) still fails loudly at load time.
type loadable struct {
	raw []byte
}

// Kind reports a constant kind from a value receiver, as the registry requires.
func (loadable) Kind() artifact.Kind { return Kind }

// Parse validates the raw bytes as an html/template and stores them for
// later, real parsing in Service.Generate.
func (l *loadable) Parse(raw []byte) error {
	if _, err := template.New("certificate").Funcs(stubFuncs()).Parse(string(raw)); err != nil {
		return fmt.Errorf("parse certificate template: %w", err)
	}
	l.raw = raw
	return nil
}

// Load fetches the raw bytes of the newest version of the certificate
// template with the given id.
func Load(ctx context.Context, reg *artifact.Registry, id string) ([]byte, error) {
	w, err := artifact.Latest[loadable](ctx, reg, id)
	if err != nil {
		return nil, err
	}
	return w.raw, nil
}
