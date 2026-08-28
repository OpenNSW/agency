package dbtype

import (
	"reflect"
	"testing"
)

func TestJSONB_ValueRoundTrip(t *testing.T) {
	j := JSONB{"foo": "bar", "n": float64(1)}

	v, err := j.Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}

	var got JSONB
	if err := got.Scan(v); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if !reflect.DeepEqual(got, j) {
		t.Errorf("round-tripped = %#v, want %#v", got, j)
	}
}

func TestJSONB_ValueNil(t *testing.T) {
	var j JSONB
	v, err := j.Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	if v != nil {
		t.Errorf("Value() = %v, want nil", v)
	}
}

func TestJSONB_ScanNil(t *testing.T) {
	j := JSONB{"foo": "bar"}
	if err := j.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error = %v", err)
	}
	if j != nil {
		t.Errorf("Scan(nil) = %#v, want nil", j)
	}
}

func TestJSONB_ScanStringAndBytes(t *testing.T) {
	want := JSONB{"foo": "bar"}

	var fromBytes JSONB
	if err := fromBytes.Scan([]byte(`{"foo":"bar"}`)); err != nil {
		t.Fatalf("Scan([]byte) error = %v", err)
	}
	if !reflect.DeepEqual(fromBytes, want) {
		t.Errorf("Scan([]byte) = %#v, want %#v", fromBytes, want)
	}

	var fromString JSONB
	if err := fromString.Scan(`{"foo":"bar"}`); err != nil {
		t.Fatalf("Scan(string) error = %v", err)
	}
	if !reflect.DeepEqual(fromString, want) {
		t.Errorf("Scan(string) = %#v, want %#v", fromString, want)
	}
}

func TestJSONB_ScanUnsupportedType(t *testing.T) {
	var j JSONB
	if err := j.Scan(42); err == nil {
		t.Error("Scan(int) expected error, got nil")
	}
}

func TestJSONB_ScanReplacesRatherThanMerges(t *testing.T) {
	j := JSONB{"old": true}
	if err := j.Scan([]byte(`{"new":true}`)); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	want := JSONB{"new": true}
	if !reflect.DeepEqual(j, want) {
		t.Errorf("Scan() = %#v, want %#v (stale key from a previous Scan must not survive)", j, want)
	}
}
