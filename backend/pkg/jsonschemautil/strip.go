package jsonschemautil

import (
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

// StripReadOnly parses rawSchema as a JSON Schema and deletes every field
// marked "readOnly": true from instance, in place, recursively through
// nested objects (via "properties") and array items (via "items"). It
// returns instance itself for convenience; callers that need the
// pre-stripped data for something else (logging, comparison) must copy it
// before calling.
//
// A nil/empty rawSchema means no schema is configured, so instance is
// returned unchanged (following the pattern of ValidateInstance). A nil instance
// returns nil.
//
// This is a pure data transform.
// (composition keywords like $ref/allOf/anyOf/oneOf are not followed).
func StripReadOnly(rawSchema json.RawMessage, instance map[string]any) (map[string]any, error) {
	if len(rawSchema) == 0 || instance == nil {
		return instance, nil
	}

	var sch jsonschema.Schema
	if err := json.Unmarshal(rawSchema, &sch); err != nil {
		return nil, fmt.Errorf("%w: parse schema: %w", ErrSchemaLoad, err)
	}

	stripObject(&sch, instance)
	return instance, nil
}

// stripObject deletes from obj every key whose matching property schema is
// marked readOnly, then recurses into the surviving values.
func stripObject(sch *jsonschema.Schema, obj map[string]any) {
	if sch == nil || obj == nil {
		return
	}
	for name, propSchema := range sch.Properties {
		value, ok := obj[name]
		if !ok {
			continue
		}
		if propSchema != nil && propSchema.ReadOnly {
			delete(obj, name)
			continue
		}
		stripValue(propSchema, value)
	}
}

// stripValue recurses into value if it's a JSON object or array and sch
// describes its shape; anything else (scalars, or no schema to recurse
// with) is left as-is.
func stripValue(sch *jsonschema.Schema, value any) {
	if sch == nil {
		return
	}
	switch v := value.(type) {
	case map[string]any:
		stripObject(sch, v)
	case []any:
		if sch.Items == nil {
			return
		}
		for _, item := range v {
			stripValue(sch.Items, item)
		}
	}
}
