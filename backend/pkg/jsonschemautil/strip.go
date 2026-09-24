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
	if instance == nil {
		instance = map[string]any{}
	}
	if len(rawSchema) == 0 {
		return instance, nil
	}
	var sch jsonschema.Schema
	if err := json.Unmarshal(rawSchema, &sch); err != nil {
		return nil, fmt.Errorf("%w: parse schema: %w", ErrSchemaLoad, err)
	}

	s := &stripper{root: &sch, patternProps: map[*jsonschema.Schema][]compiledPattern{}}
	s.stripValue(&sch, instance)
	return instance, nil
}

// stripper carries the state shared across one StripReadOnly call: the root
// schema (for resolving "$ref") and a cache of compiled "patternProperties"
// regexps keyed by schema node, so a schema visited from multiple instance
// paths (e.g. via "$ref" or repeated array items) only compiles its patterns
// once.
type stripper struct {
	root         *jsonschema.Schema
	patternProps map[*jsonschema.Schema][]compiledPattern
}

// compiledPattern pairs a compiled "patternProperties" regexp with the
// schema it maps to.
type compiledPattern struct {
	re     *regexp.Regexp
	schema *jsonschema.Schema
}

// stripObject deletes from obj every key any of whose applicable schemas
// (see applicableSchemas) marks readOnly, then recurses into the surviving
// values against every applicable schema.
func (s *stripper) stripObject(sch *jsonschema.Schema, obj map[string]any) {
	if sch == nil || obj == nil {
		return
	}
	for name, value := range obj {
		propSchemas := s.applicableSchemas(sch, name)
		if s.anyReadOnly(propSchemas) {
			delete(obj, name)
			continue
		}
		for _, propSchema := range propSchemas {
			// "$ref" doesn't replace sibling keywords: JSON Schema
			// 2020-12 applies "$ref" like any other keyword, so a
			// "properties"/"patternProperties"/etc. entry declared
			// alongside "$ref" still describes this same value and must
			// be walked too, not just whatever the $ref resolves to.
			if propSchema.Ref != "" {
				s.walk(propSchema, value)
			}
			if r := s.resolveRef(propSchema); r != nil {
				s.stripValue(r, value)
			}
		}
	}
}

// anyReadOnly reports whether any of propSchemas, or its one-hop $ref
// target, is marked readOnly.
func (s *stripper) anyReadOnly(propSchemas []*jsonschema.Schema) bool {
	for _, propSchema := range propSchemas {
		// readOnly is checked before resolving $ref: JSON Schema to address the cases like(e.g. {"$ref": "#/$defs/X", "readOnly": true}),
		// otherwise the readOnly property will be ignored if the $ref is resolved to a schema without readOnly.
		if propSchema.ReadOnly {
			return true
		}
		if r := s.resolveRef(propSchema); r != nil && r.ReadOnly {
			return true
		}
	}
	return false
}

// applicableSchemas returns every schema JSON Schema applies to instance
// property name: the explicit "properties" entry if present, plus every
// matching "patternProperties" regexp - both apply simultaneously, they are
// not mutually exclusive, matching the semantics the underlying
// jsonschema-go validator itself uses. "additionalProperties" only applies
// when neither of those matched (nil if nothing applies).
func (s *stripper) applicableSchemas(sch *jsonschema.Schema, name string) []*jsonschema.Schema {
	var matched []*jsonschema.Schema
	if propSchema, ok := sch.Properties[name]; ok {
		matched = append(matched, propSchema)
	}
	for _, cp := range s.compiledPatternProperties(sch) {
		if cp.re.MatchString(name) {
			matched = append(matched, cp.schema)
		}
	}
	if len(matched) > 0 {
		return matched
	}
	if sch.AdditionalProperties != nil {
		return []*jsonschema.Schema{sch.AdditionalProperties}
	}
	return nil
}

// compiledPatternProperties returns sch's "patternProperties" regexps,
// compiled once and cached per schema node. Schemas reach here only after
// validation, so they're guaranteed valid; a pattern that still fails to
// compile is skipped rather than treated as an error.
func (s *stripper) compiledPatternProperties(sch *jsonschema.Schema) []compiledPattern {
	if cached, ok := s.patternProps[sch]; ok {
		return cached
	}
	compiled := make([]compiledPattern, 0, len(sch.PatternProperties))
	for pattern, propSchema := range sch.PatternProperties {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		compiled = append(compiled, compiledPattern{re: re, schema: propSchema})
	}
	s.patternProps[sch] = compiled
	return compiled
}

// stripValue resolves sch's own "$ref" (if any) and then walks value
// against whatever that resolves to; anything unresolvable (or nil) is left
// as-is.
func (s *stripper) stripValue(sch *jsonschema.Schema, value any) {
	sch = s.resolveRef(sch)
	if sch == nil {
		return
	}
	s.walk(sch, value)
}

// walk applies sch's own object/array keywords to value without resolving
// sch's own "$ref" first. Used both by stripValue (after it has already
// resolved sch) and directly for a schema's "$ref"-sibling keywords, which
// apply to value independently of whatever that "$ref" itself resolves to.
func (s *stripper) walk(sch *jsonschema.Schema, value any) {
	switch v := value.(type) {
	case map[string]any:
		s.stripObject(sch, v)
	case []any:
		for i, item := range v {
			itemSch := itemSchema(sch, i)
			if itemSch == nil {
				continue
			}
			s.stripValue(itemSch, item)
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
func (s *stripper) resolveRef(sch *jsonschema.Schema) *jsonschema.Schema {
	if sch == nil || sch.Ref == "" {
		return sch
	}
	name, ok := strings.CutPrefix(sch.Ref, "#/$defs/")
	if ok {
		return s.root.Defs[unescapeJSONPointerToken(name)]
	}
	if name, ok := strings.CutPrefix(sch.Ref, "#/definitions/"); ok {
		return s.root.Definitions[unescapeJSONPointerToken(name)]
	}
	return nil
}

// unescapeJSONPointerToken reverses the "~1"/"~0" escaping RFC 6901 requires
// for "/" and "~" within a single JSON Pointer token.
func unescapeJSONPointerToken(tok string) string {
	return strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
}
