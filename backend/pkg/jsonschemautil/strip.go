package jsonschemautil

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// StripReadOnly parses rawSchema as a JSON Schema and deletes every field
// marked "readOnly": true from instance, recursively through nested objects
// (via "properties", "patternProperties", and "additionalProperties"),
// array items (via "items", "prefixItems", and the legacy tuple form of
// "items"), and local "$ref" pointers into "$defs"/"definitions".
//
// Callers must always use the returned map, not the instance argument as
// passed in: for a non-nil instance the two happen to be the same
// underlying map (mutated in place), but a nil instance returns a distinct, newly
// allocated empty map instead. Callers that need the pre-stripped data for
// something else (logging, comparison) must copy it before calling.
//
// A nil/empty rawSchema means no schema is configured, so instance is
// returned unchanged (following the pattern of ValidateInstance). A nil
// instance is treated as an empty object (matching ValidateInstance).
//
// This is a pure data transform. Other composition keywords (allOf/anyOf/
// oneOf) and any "$ref" that isn't a direct "#/$defs/<name>" or
// "#/definitions/<name>" pointer (a nested-path or remote ref) are not
// followed.
func StripReadOnly(rawSchema json.RawMessage, instance map[string]any) (map[string]any, error) {
	if len(rawSchema) == 0 {
		return instance, nil
	}
	if instance == nil {
		instance = map[string]any{}
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
		for i, item := range v {
			itemSch := itemSchema(sch, i)
			if itemSch == nil {
				continue
			}
			stripValue(root, itemSch, item)
		}
	}
}

// itemSchema returns the schema that applies to the array item at index,
// per JSON Schema's positional-array precedence: "prefixItems" (2020-12
// tuple form) takes priority for its covered indices, falling back to
// "items" as the overflow schema past the prefix; the legacy tuple form
// ("items" itself given as an array of schemas, parsed into ItemsArray)
// falls back to "additionalItems" past the tuple; otherwise "items" applies
// uniformly to every index (nil if none of those are present).
func itemSchema(sch *jsonschema.Schema, index int) *jsonschema.Schema {
	if len(sch.PrefixItems) > 0 {
		if index < len(sch.PrefixItems) {
			return sch.PrefixItems[index]
		}
		return sch.Items
	}
	if len(sch.ItemsArray) > 0 {
		if index < len(sch.ItemsArray) {
			return sch.ItemsArray[index]
		}
		return sch.AdditionalItems
	}
	return sch.Items
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
