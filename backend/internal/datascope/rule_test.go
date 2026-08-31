package datascope

import (
	"errors"
	"testing"
)

func TestParseRules_Valid(t *testing.T) {
	raw := []byte(`{
		"rules": [
			{"consignmentField": "/location/district", "userField": "/assignedDistrict"},
			{"consignmentField": "/priority", "userField": "/level"}
		]
	}`)
	rules, err := ParseRules(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("len(rules) = %d, want 2", len(rules))
	}
}

func TestParseRules_Empty(t *testing.T) {
	rules, err := ParseRules([]byte(`{"rules": []}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("len(rules) = %d, want 0", len(rules))
	}
}

func TestParseRules_MalformedJSON(t *testing.T) {
	_, err := ParseRules([]byte(`not json`))
	if !errors.Is(err, ErrInvalidRules) {
		t.Errorf("error = %v, want it to wrap ErrInvalidRules", err)
	}
}

func TestParseRules_RejectsNullDocument(t *testing.T) {
	_, err := ParseRules([]byte(`null`))
	if !errors.Is(err, ErrInvalidRules) {
		t.Errorf("error = %v, want it to wrap ErrInvalidRules for a null document", err)
	}
}

func TestParseRules_RejectsNullRules(t *testing.T) {
	_, err := ParseRules([]byte(`{"rules": null}`))
	if !errors.Is(err, ErrInvalidRules) {
		t.Errorf("error = %v, want it to wrap ErrInvalidRules for an explicit null \"rules\"", err)
	}
}

func TestParseRules_RejectsMissingRulesKey(t *testing.T) {
	_, err := ParseRules([]byte(`{}`))
	if !errors.Is(err, ErrInvalidRules) {
		t.Errorf("error = %v, want it to wrap ErrInvalidRules when \"rules\" is absent", err)
	}
}

func TestParseRules_RejectsBareArray(t *testing.T) {
	// The old bare-array shape must not be silently accepted (it would
	// unmarshal into an object with no "rules" field, i.e. nil Rules).
	_, err := ParseRules([]byte(`[{"consignmentField": "/x", "userField": "/y"}]`))
	if !errors.Is(err, ErrInvalidRules) {
		t.Errorf("error = %v, want it to wrap ErrInvalidRules for a bare top-level array", err)
	}
}

func TestParseRules_InvalidPointer(t *testing.T) {
	tests := []string{
		`{"rules": [{"consignmentField": "no-leading-slash", "userField": "/x"}]}`,
		`{"rules": [{"consignmentField": "/x", "userField": ""}]}`,
		`{"rules": [{"consignmentField": "/a b", "userField": "/x"}]}`,
		`{"rules": [{"consignmentField": "/a'; DROP TABLE users; --", "userField": "/x"}]}`,
		`{"rules": [{"consignmentField": "/a.b", "userField": "/x"}]}`,
	}
	for _, raw := range tests {
		if _, err := ParseRules([]byte(raw)); !errors.Is(err, ErrInvalidRules) {
			t.Errorf("ParseRules(%s) error = %v, want it to wrap ErrInvalidRules", raw, err)
		}
	}
}

func TestParseRules_DuplicateConsignmentField(t *testing.T) {
	raw := []byte(`{
		"rules": [
			{"consignmentField": "/location/district", "userField": "/assignedDistrict"},
			{"consignmentField": "/location/district", "userField": "/otherField"}
		]
	}`)
	if _, err := ParseRules(raw); !errors.Is(err, ErrInvalidRules) {
		t.Errorf("error = %v, want it to wrap ErrInvalidRules for a duplicate consignmentField", err)
	}
}
