package jsonschemautil

import (
	"errors"
	"testing"
)

func TestValidateInstance_NoSchemaSkipsValidation(t *testing.T) {
	if err := ValidateInstance(nil, map[string]any{"anything": "goes"}); err != nil {
		t.Errorf("ValidateInstance(nil schema) error = %v, want nil", err)
	}
}

func TestValidateInstance_ValidData(t *testing.T) {
	schema := []byte(`{"type":"object","required":["badge"],"properties":{"badge":{"type":"string"}}}`)
	if err := ValidateInstance(schema, map[string]any{"badge": "123"}); err != nil {
		t.Errorf("ValidateInstance() error = %v, want nil", err)
	}
}

func TestValidateInstance_MismatchIsUnwrapped(t *testing.T) {
	schema := []byte(`{"type":"object","required":["badge"]}`)
	err := ValidateInstance(schema, map[string]any{})
	if err == nil {
		t.Fatal("ValidateInstance() expected a mismatch error, got nil")
	}
	if errors.Is(err, ErrSchemaLoad) {
		t.Errorf("ValidateInstance() mismatch error should not be ErrSchemaLoad, got %v", err)
	}
}

func TestValidateInstance_NilInstanceTreatedAsEmptyObject(t *testing.T) {
	schema := []byte(`{"type":"object","required":["badge"]}`)
	if err := ValidateInstance(schema, nil); err == nil {
		t.Error("ValidateInstance(nil instance) expected a required-field mismatch, got nil")
	}
}

func TestValidateInstance_UnparsableSchemaIsErrSchemaLoad(t *testing.T) {
	err := ValidateInstance([]byte(`not json`), map[string]any{})
	if err == nil {
		t.Fatal("ValidateInstance() expected an error, got nil")
	}
	if !errors.Is(err, ErrSchemaLoad) {
		t.Errorf("ValidateInstance() error = %v, want ErrSchemaLoad", err)
	}
}

func TestValidateInstance_UnresolvableSchemaIsErrSchemaLoad(t *testing.T) {
	// A $ref to a schema that doesn't exist fails at Resolve time, not parse time.
	schema := []byte(`{"$ref":"#/does-not-exist"}`)
	err := ValidateInstance(schema, map[string]any{})
	if err == nil {
		t.Fatal("ValidateInstance() expected an error, got nil")
	}
	if !errors.Is(err, ErrSchemaLoad) {
		t.Errorf("ValidateInstance() error = %v, want ErrSchemaLoad", err)
	}
}
