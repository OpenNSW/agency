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

func TestStripReadOnly_NilInstanceReturnsNil(t *testing.T) {
	schema := []byte(`{"type":"object"}`)
	got, err := StripReadOnly(schema, nil)
	if err != nil {
		t.Fatalf("StripReadOnly() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("StripReadOnly(nil instance) = %v, want nil", got)
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
