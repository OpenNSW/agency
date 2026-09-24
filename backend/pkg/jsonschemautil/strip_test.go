package jsonschemautil

import (
	"errors"
	"testing"
)

func TestStripReadOnly_NoSchemaIsNoop(t *testing.T) {
	instance := map[string]any{"anything": "goes"}
	got, err := StripReadOnly(nil, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	if got["anything"] != "goes" {
		t.Errorf("StripReadOnly(nil schema) = %v, want instance unchanged", got)
	}
}

func TestStripReadOnly_NilInstanceTreatedAsEmptyObject(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"injectedValue": {"type": "string", "readOnly": true}
		}
	}`)
	got, err := StripReadOnly(schema, nil)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	if got == nil {
		t.Fatalf("StripReadOnly(nil instance) = nil, want non-nil empty map (matches ValidateInstance's nil-as-{} treatment)")
	}
	if len(got) != 0 {
		t.Errorf("StripReadOnly(nil instance) = %v, want empty map", got)
	}
	// Must be safe to write into, unlike a nil map.
	got["x"] = 1
}

func TestStripReadOnly_MalformedSchemaErrorsEvenWithNilInstance(t *testing.T) {
	schema := []byte(`{not valid json`)
	got, err := StripReadOnly(schema, nil)
	if !errors.Is(err, ErrSchemaLoad) {
		t.Fatalf("StripReadOnly() error = %v, want ErrSchemaLoad", err)
	}
	if got != nil {
		t.Errorf("StripReadOnly() = %v, want nil on error", got)
	}
}

func TestStripReadOnly_TopLevelField(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"injectedValue": {"type": "string", "readOnly": true},
			"comment": {"type": "string"}
		}
	}`)
	instance := map[string]any{"injectedValue": "tampered", "comment": "looks fine"}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	if _, ok := got["injectedValue"]; ok {
		t.Errorf("StripReadOnly() kept readOnly field, got %v", got)
	}
	if got["comment"] != "looks fine" {
		t.Errorf("StripReadOnly() dropped editable field, got %v", got)
	}
}

func TestStripReadOnly_NestedObject(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"inspection": {
				"type": "object",
				"properties": {
					"officerId": {"type": "string", "readOnly": true},
					"notes": {"type": "string"}
				}
			}
		}
	}`)
	instance := map[string]any{
		"inspection": map[string]any{"officerId": "spoofed", "notes": "ok"},
	}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	inspection := got["inspection"].(map[string]any)
	if _, ok := inspection["officerId"]; ok {
		t.Errorf("StripReadOnly() kept nested readOnly field, got %v", inspection)
	}
	if inspection["notes"] != "ok" {
		t.Errorf("StripReadOnly() dropped nested editable field, got %v", inspection)
	}
}

func TestStripReadOnly_ArrayItems(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"lines": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"lockedTotal": {"type": "number", "readOnly": true},
						"description": {"type": "string"}
					}
				}
			}
		}
	}`)
	instance := map[string]any{
		"lines": []any{
			map[string]any{"lockedTotal": float64(999), "description": "a"},
			map[string]any{"lockedTotal": float64(999), "description": "b"},
		},
	}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	lines := got["lines"].([]any)
	for _, item := range lines {
		line := item.(map[string]any)
		if _, ok := line["lockedTotal"]; ok {
			t.Errorf("StripReadOnly() kept readOnly array item field, got %v", line)
		}
		if line["description"] != "a" && line["description"] != "b" {
			t.Errorf("StripReadOnly() dropped editable array item field, got %v", line)
		}
	}
}

func TestStripReadOnly_RefDefs(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"$defs": {
			"Inspection": {
				"type": "object",
				"properties": {
					"officerId": {"type": "string", "readOnly": true},
					"notes": {"type": "string"}
				}
			}
		},
		"properties": {
			"inspection": {"$ref": "#/$defs/Inspection"}
		}
	}`)
	instance := map[string]any{
		"inspection": map[string]any{"officerId": "spoofed", "notes": "ok"},
	}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	inspection := got["inspection"].(map[string]any)
	if _, ok := inspection["officerId"]; ok {
		t.Errorf("StripReadOnly() kept readOnly field behind $defs $ref, got %v", inspection)
	}
	if inspection["notes"] != "ok" {
		t.Errorf("StripReadOnly() dropped editable field behind $defs $ref, got %v", inspection)
	}
}

// When there are keywords alongside "$ref"; a readOnly sitting
// next to a $ref must still be honored even though the $ref's target itself
// is editable.
func TestStripReadOnly_ReadOnlySiblingOfRef(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"$defs": {
			"Inspection": {
				"type": "object",
				"properties": {
					"notes": {"type": "string"}
				}
			}
		},
		"properties": {
			"inspection": {"$ref": "#/$defs/Inspection", "readOnly": true}
		}
	}`)
	instance := map[string]any{
		"inspection": map[string]any{"notes": "spoofed"},
	}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	if _, ok := got["inspection"]; ok {
		t.Errorf("StripReadOnly() kept field marked readOnly alongside $ref, got %v", got)
	}
}

// A "properties" entry declared alongside "$ref" applies independently of
// the $ref's target - not just a bare "readOnly" sibling (already covered by
// TestStripReadOnly_ReadOnlySiblingOfRef), but a nested readOnly field
// declared in that sibling "properties".
func TestStripReadOnly_ReadOnlyInRefSiblingProperties(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"$defs": {
			"Inspection": {
				"type": "object",
				"properties": {
					"notes": {"type": "string"}
				}
			}
		},
		"properties": {
			"inspection": {
				"$ref": "#/$defs/Inspection",
				"properties": {
					"officerId": {"type": "string", "readOnly": true}
				}
			}
		}
	}`)
	instance := map[string]any{
		"inspection": map[string]any{"notes": "keep", "officerId": "spoofed"},
	}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	inspection := got["inspection"].(map[string]any)
	if _, ok := inspection["officerId"]; ok {
		t.Errorf("StripReadOnly() kept readOnly field declared in $ref sibling properties, got %v", inspection)
	}
	if inspection["notes"] != "keep" {
		t.Errorf("StripReadOnly() dropped editable field behind $ref target, got %v", inspection)
	}
}

// for old JSON Schema drafts that use "definitions" instead of "$defs"
func TestStripReadOnly_RefDefinitions(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"definitions": {
			"Inspection": {
				"type": "object",
				"properties": {
					"officerId": {"type": "string", "readOnly": true}
				}
			}
		},
		"properties": {
			"inspection": {"$ref": "#/definitions/Inspection"}
		}
	}`)
	instance := map[string]any{
		"inspection": map[string]any{"officerId": "spoofed"},
	}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	inspection := got["inspection"].(map[string]any)
	if _, ok := inspection["officerId"]; ok {
		t.Errorf("StripReadOnly() kept readOnly field behind definitions $ref, got %v", inspection)
	}
}

func TestStripReadOnly_RefArrayItems(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"$defs": {
			"Line": {
				"type": "object",
				"properties": {
					"lockedTotal": {"type": "number", "readOnly": true},
					"description": {"type": "string"}
				}
			}
		},
		"properties": {
			"lines": {"type": "array", "items": {"$ref": "#/$defs/Line"}}
		}
	}`)
	instance := map[string]any{
		"lines": []any{
			map[string]any{"lockedTotal": float64(999), "description": "a"},
		},
	}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	line := got["lines"].([]any)[0].(map[string]any)
	if _, ok := line["lockedTotal"]; ok {
		t.Errorf("StripReadOnly() kept readOnly field behind array-item $ref, got %v", line)
	}
	if line["description"] != "a" {
		t.Errorf("StripReadOnly() dropped editable field behind array-item $ref, got %v", line)
	}
}

func TestStripReadOnly_UnsupportedRefIsSkipped(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"inspection": {"$ref": "jsonpath/that/does/not/exist"}
		}
	}`)
	instance := map[string]any{
		"inspection": map[string]any{"officerId": "unchanged"},
	}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	inspection := got["inspection"].(map[string]any)
	if inspection["officerId"] != "unchanged" {
		t.Errorf("StripReadOnly() should leave unresolvable $ref subtree untouched, got %v", inspection)
	}
}

func TestStripReadOnly_AdditionalProperties(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"comment": {"type": "string"}
		},
		"additionalProperties": {
			"type": "object",
			"properties": {
				"lockedTotal": {"type": "number", "readOnly": true},
				"description": {"type": "string"}
			}
		}
	}`)
	instance := map[string]any{
		"comment": "ok",
		"extra1":  map[string]any{"lockedTotal": float64(999), "description": "a"},
	}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	extra := got["extra1"].(map[string]any)
	if _, ok := extra["lockedTotal"]; ok {
		t.Errorf("StripReadOnly() kept readOnly field behind additionalProperties, got %v", extra)
	}
	if extra["description"] != "a" {
		t.Errorf("StripReadOnly() dropped editable field behind additionalProperties, got %v", extra)
	}
}

func TestStripReadOnly_PatternProperties(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"patternProperties": {
			"^item_": {
				"type": "object",
				"properties": {
					"lockedTotal": {"type": "number", "readOnly": true},
					"description": {"type": "string"}
				}
			}
		}
	}`)
	instance := map[string]any{
		"item_1": map[string]any{"lockedTotal": float64(999), "description": "a"},
	}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	item := got["item_1"].(map[string]any)
	if _, ok := item["lockedTotal"]; ok {
		t.Errorf("StripReadOnly() kept readOnly field behind patternProperties, got %v", item)
	}
	if item["description"] != "a" {
		t.Errorf("StripReadOnly() dropped editable field behind patternProperties, got %v", item)
	}
}

func TestStripReadOnly_PropertiesTakesPrecedenceOverAdditionalProperties(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"comment": {"type": "string"}
		},
		"additionalProperties": {"type": "string", "readOnly": true}
	}`)
	instance := map[string]any{"comment": "ok"}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	if got["comment"] != "ok" {
		t.Errorf("StripReadOnly() incorrectly applied additionalProperties to a named property, got %v", got)
	}
}

func TestStripReadOnly_PropertiesAndPatternPropertiesBothApply(t *testing.T) {
	// "id" matches both the explicit "properties" entry (not readOnly) and
	// the "^i" patternProperties entry (readOnly): JSON Schema applies both
	// simultaneously, so the readOnly one must win and strip the field.
	schema := []byte(`{
		"type": "object",
		"properties": {
			"id": {"type": "string"}
		},
		"patternProperties": {
			"^i": {"type": "string", "readOnly": true}
		}
	}`)
	instance := map[string]any{"id": "abc"}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	if _, ok := got["id"]; ok {
		t.Errorf("StripReadOnly() kept field readOnly via patternProperties despite also matching properties, got %v", got)
	}
}

func TestStripReadOnly_MultiplePatternPropertiesMatchSameName(t *testing.T) {
	// "item_1" matches two patternProperties regexps; only one marks the
	// nested field readOnly. Both must be consulted regardless of map
	// iteration order.
	schema := []byte(`{
		"type": "object",
		"patternProperties": {
			"^item_": {
				"type": "object",
				"properties": {
					"description": {"type": "string"}
				}
			},
			"_1$": {
				"type": "object",
				"properties": {
					"description": {"type": "string", "readOnly": true}
				}
			}
		}
	}`)
	instance := map[string]any{"item_1": map[string]any{"description": "a"}}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	item := got["item_1"].(map[string]any)
	if _, ok := item["description"]; ok {
		t.Errorf("StripReadOnly() kept field readOnly via one of several matching patternProperties, got %v", item)
	}
}

func TestStripReadOnly_PrefixItems(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"tuple": {
				"type": "array",
				"prefixItems": [
					{"type": "object", "properties": {"lockedTotal": {"type": "number", "readOnly": true}, "description": {"type": "string"}}},
					{"type": "object", "properties": {"note": {"type": "string"}}}
				],
				"items": {"type": "object", "properties": {"overflowLocked": {"type": "string", "readOnly": true}, "extra": {"type": "string"}}}
			}
		}
	}`)
	instance := map[string]any{
		"tuple": []any{
			map[string]any{"lockedTotal": float64(999), "description": "a"},
			map[string]any{"note": "b"},
			map[string]any{"overflowLocked": "tampered", "extra": "c"},
		},
	}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	tuple := got["tuple"].([]any)
	first := tuple[0].(map[string]any)
	if _, ok := first["lockedTotal"]; ok {
		t.Errorf("StripReadOnly() kept readOnly field in prefixItems[0], got %v", first)
	}
	if first["description"] != "a" {
		t.Errorf("StripReadOnly() dropped editable field in prefixItems[0], got %v", first)
	}
	second := tuple[1].(map[string]any)
	if second["note"] != "b" {
		t.Errorf("StripReadOnly() dropped editable field in prefixItems[1], got %v", second)
	}
	third := tuple[2].(map[string]any)
	if _, ok := third["overflowLocked"]; ok {
		t.Errorf("StripReadOnly() kept readOnly field in overflow item, got %v", third)
	}
	if third["extra"] != "c" {
		t.Errorf("StripReadOnly() dropped editable field in overflow item, got %v", third)
	}
}

// for old JSON Schema drafts that use "items" as a tuple instead of "prefixItems"
func TestStripReadOnly_LegacyItemsArray(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"tuple": {
				"type": "array",
				"items": [
					{"type": "object", "properties": {"lockedTotal": {"type": "number", "readOnly": true}, "description": {"type": "string"}}},
					{"type": "object", "properties": {"note": {"type": "string"}}}
				],
				"additionalItems": {"type": "object", "properties": {"overflowLocked": {"type": "string", "readOnly": true}, "extra": {"type": "string"}}}
			}
		}
	}`)
	instance := map[string]any{
		"tuple": []any{
			map[string]any{"lockedTotal": float64(999), "description": "a"},
			map[string]any{"note": "b"},
			map[string]any{"overflowLocked": "tampered", "extra": "c"},
		},
	}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	tuple := got["tuple"].([]any)
	first := tuple[0].(map[string]any)
	if _, ok := first["lockedTotal"]; ok {
		t.Errorf("StripReadOnly() kept readOnly field in legacy items[0], got %v", first)
	}
	if first["description"] != "a" {
		t.Errorf("StripReadOnly() dropped editable field in legacy items[0], got %v", first)
	}
	second := tuple[1].(map[string]any)
	if second["note"] != "b" {
		t.Errorf("StripReadOnly() dropped editable field in legacy items[1], got %v", second)
	}
	third := tuple[2].(map[string]any)
	if _, ok := third["overflowLocked"]; ok {
		t.Errorf("StripReadOnly() kept readOnly field in additionalItems overflow, got %v", third)
	}
	if third["extra"] != "c" {
		t.Errorf("StripReadOnly() dropped editable field in additionalItems overflow, got %v", third)
	}
}

func TestStripReadOnly_MutatesInputInPlace(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"injectedValue": {"type": "string", "readOnly": true}
		}
	}`)
	instance := map[string]any{"injectedValue": "original"}

	got, err := StripReadOnly(schema, instance)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	if _, ok := instance["injectedValue"]; ok {
		t.Errorf("StripReadOnly() did not mutate caller's instance in place, got %v", instance)
	}
	got["addedAfter"] = true
	if instance["addedAfter"] != true {
		t.Error("StripReadOnly() return value is not the same underlying map as the input")
	}
}

func TestStripReadOnly_UnparsableSchemaIsErrSchemaLoad(t *testing.T) {
	_, err := StripReadOnly([]byte(`not json`), map[string]any{})
	if err == nil {
		t.Fatal("StripReadOnly() expected an error, got nil")
	}
	if !errors.Is(err, ErrSchemaLoad) {
		t.Errorf("StripReadOnly() error = %v, want ErrSchemaLoad", err)
	}
}
