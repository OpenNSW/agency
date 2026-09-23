package jsonschemautil

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// StripReadOnly parses rawSchema as a JSON Schema and deletes every field
// marked "readOnly": true from instance, in place, recursively through
// nested objects (via "properties", "patternProperties", and
// "additionalProperties"), array items (via "items"), and local "$ref"
// pointers into "$defs"/"definitions". It returns instance itself for
// convenience; callers that need the pre-stripped data for something else
// (logging, comparison) must copy it before calling.
//
// A nil/empty rawSchema means no schema is configured, so instance is
// returned unchanged (following the pattern of ValidateInstance). A nil instance
// returns nil.
//
// This is a pure data transform. Other composition keywords (allOf/anyOf/
// oneOf) and any "$ref" that isn't a direct "#/$defs/<name>" or
// "#/definitions/<name>" pointer (a nested-path or remote ref) are not
// followed.
func StripReadOnly(rawSchema json.RawMessage, instance map[string]any) (map[string]any, error) {
	if len(rawSchema) == 0 || instance == nil {
		return instance, nil
	}

	var sch jsonschema.Schema
	if err := json.Unmarshal(rawSchema, &sch); err != nil {
		return nil, fmt.Errorf("%w: parse schema: %w", ErrSchemaLoad, err)
	}

	stripValue(&sch, &sch, instance)
	return instance, nil
}

// stripObject deletes from obj every key whose matching property schema is
// marked readOnly, then recurses into the surviving values. The matching
// schema for a key is found via "properties", falling back to
// "patternProperties" and then "additionalProperties" for keys not named in
// "properties" - the same precedence JSON Schema itself uses to decide which
// schema applies to a given property name.
func stripObject(root, sch *jsonschema.Schema, obj map[string]any) {
	if sch == nil || obj == nil {
		return
	}
	for name, value := range obj {
		propSchema := resolveRef(root, propertySchema(sch, name))
		if propSchema == nil {
			continue
		}
		if propSchema.ReadOnly {
			delete(obj, name)
			continue
		}
		stripValue(root, propSchema, value)
	}
}

// propertySchema returns the schema that applies to instance property name,
// per JSON Schema's matching precedence: an explicit "properties" entry,
// else the first matching "patternProperties" regexp, else
// "additionalProperties" (nil if none of those are present).
func propertySchema(sch *jsonschema.Schema, name string) *jsonschema.Schema {
	if propSchema, ok := sch.Properties[name]; ok {
		return propSchema
	}
	for pattern, propSchema := range sch.PatternProperties {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(name) {
			return propSchema
		}
	}
	return sch.AdditionalProperties
}

// stripValue recurses into value if it's a JSON object or array and sch
// describes its shape; anything else (scalars, or no schema to recurse
// with) is left as-is.
func stripValue(root, sch *jsonschema.Schema, value any) {
	sch = resolveRef(root, sch)
	if sch == nil {
		return
	}
	switch v := value.(type) {
	case map[string]any:
		stripObject(root, sch, v)
	case []any:
		if sch.Items == nil {
			return
		}
		for _, item := range v {
			stripValue(root, sch.Items, item)
		}
	}
}

// resolveRef follows a direct "#/$defs/<name>" or "#/definitions/<name>"
// $ref against root, one level. Anything else (a nested-path or remote ref)
// is left unresolved, returning nil so the caller treats it like a schema
// with no properties/items.
func resolveRef(root, sch *jsonschema.Schema) *jsonschema.Schema {
	if sch == nil || sch.Ref == "" {
		return sch
	}
	name, ok := strings.CutPrefix(sch.Ref, "#/$defs/")
	if ok {
		return root.Defs[unescapeJSONPointerToken(name)]
	}
	if name, ok := strings.CutPrefix(sch.Ref, "#/definitions/"); ok {
		return root.Definitions[unescapeJSONPointerToken(name)]
	}
	return nil
}

// unescapeJSONPointerToken reverses the "~1"/"~0" escaping RFC 6901 requires
// for "/" and "~" within a single JSON Pointer token.
func unescapeJSONPointerToken(tok string) string {
	return strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
}
