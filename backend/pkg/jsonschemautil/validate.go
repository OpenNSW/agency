// Package jsonschemautil provides a shared helper for validating instance
// data against a JSON Schema, used by any package that stores a
// caller-supplied JSON Schema and needs to validate arbitrary data against it.
package jsonschemautil

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

// ErrSchemaLoad wraps a JSON Schema parse or resolve failure. Callers that
// need to distinguish a broken/misconfigured schema (typically treated as a
// fail-closed internal/config error) from a validation mismatch (bad
// instance data, typically a 400) can check for it with errors.Is.
var ErrSchemaLoad = errors.New("jsonschemautil: failed to load schema")

// ValidateInstance parses rawSchema as a JSON Schema, resolves it, and
// validates instance against it. A nil/empty rawSchema means no schema is
// configured, so validation is skipped (nil error). A nil instance is
// treated as an empty object, so a schema requiring properties still gets
// checked against "no data provided".
//
// A parse or resolve failure is returned wrapped in ErrSchemaLoad; a schema
// mismatch is returned as the underlying jsonschema-go validation error,
// unwrapped, so the caller can apply its own context/sentinel.
func ValidateInstance(rawSchema json.RawMessage, instance map[string]any) error {
	if len(rawSchema) == 0 {
		return nil
	}

	var sch jsonschema.Schema
	if err := json.Unmarshal(rawSchema, &sch); err != nil {
		return fmt.Errorf("%w: parse schema: %w", ErrSchemaLoad, err)
	}
	resolved, err := sch.Resolve(nil)
	if err != nil {
		return fmt.Errorf("%w: resolve schema: %w", ErrSchemaLoad, err)
	}

	if instance == nil {
		instance = map[string]any{}
	}
	return resolved.Validate(instance)
}
