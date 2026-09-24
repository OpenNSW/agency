package jsonschemautil

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// patternCache memoizes regexp.Compile results for a single StripReadOnly
// call, keyed by the pattern string.A cached nil records a pattern that failed to compile, so a
// bad pattern is only attempted (and warned about) once per call, not once
// per lookup.
type patternCache map[string]*regexp.Regexp

func (c patternCache) compile(pattern string) *regexp.Regexp {
	if re, ok := c[pattern]; ok {
		return re
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		slog.Warn("jsonschemautil: patternProperties pattern failed to compile, skipping",
			"pattern", pattern, "error", err)
		re = nil
	}
	c[pattern] = re
	return re
}

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

	stripValue(&sch, &sch, instance, patternCache{})
	return instance, nil
}

// stripObject deletes from obj every key for which any applicable property
// schema is marked readOnly, then recurses into the surviving values with
// every applicable schema, so a nested readOnly field declared by any of
// them is stripped.
func stripObject(root, sch *jsonschema.Schema, obj map[string]any, cache patternCache) {
	if sch == nil || obj == nil {
		return
	}
	for name, value := range obj {
		propSchemas := propertySchemas(sch, name, cache)
		if slices.ContainsFunc(propSchemas, func(s *jsonschema.Schema) bool { return isReadOnly(root, s) }) {
			delete(obj, name)
			continue
		}
		for _, propSchema := range propSchemas {
			stripValue(root, propSchema, value, cache)
		}
	}
}

// isReadOnly reports whether sch is marked "readOnly": true, checking both
// the schema as given and, if it's a "$ref", the schema it resolves to - a
// "readOnly" sibling of "$ref" and a "readOnly" declared on the referenced
// definition itself are both valid and must both be honored. 
func isReadOnly(root, sch *jsonschema.Schema) bool {
	if sch == nil {
		return false
	}
	if sch.ReadOnly {
		return true
	}
	resolved := resolveRef(root, sch)
	return resolved != nil && resolved.ReadOnly
}

// propertySchemas returns every (non-nil, not yet ref-resolved) schema that
// applies to instance property name, per JSON Schema's object applicator
// rules: the "properties" entry for name and every "patternProperties"
// entry whose regexp matches name all apply together; "additionalProperties"
// applies only when neither of those matched. Callers resolve "$ref"
// themselves (via isReadOnly and stripValue) rather than here, so a
// "readOnly" sibling of "$ref" isn't lost before it can be inspected.
func propertySchemas(sch *jsonschema.Schema, name string, cache patternCache) []*jsonschema.Schema {
	var matched []*jsonschema.Schema
	add := func(s *jsonschema.Schema) {
		if s != nil {
			matched = append(matched, s)
		}
	}

	evaluated := false
	if propSchema, ok := sch.Properties[name]; ok {
		evaluated = true
		add(propSchema)
	}
	for pattern, propSchema := range sch.PatternProperties {
		re := cache.compile(pattern)
		if re == nil {
			continue
		}
		if re.MatchString(name) {
			evaluated = true
			add(propSchema)
		}
	}
	if !evaluated {
		add(sch.AdditionalProperties)
	}
	return matched
}

// stripValue recurses into value if it's a JSON object or array and sch
// describes its shape; anything else (scalars, or no schema to recurse
// with) is left as-is.
func stripValue(root, sch *jsonschema.Schema, value any, cache patternCache) {
	sch = resolveRef(root, sch)
	if sch == nil {
		return
	}
	switch v := value.(type) {
	case map[string]any:
		stripObject(root, sch, v, cache)
	case []any:
		for i, item := range v {
			itemSch := itemSchema(sch, i)
			if itemSch == nil {
				continue
			}
			stripValue(root, itemSch, item, cache)
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
