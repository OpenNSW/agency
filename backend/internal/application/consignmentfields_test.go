package application

import (
	"reflect"
	"testing"

	"github.com/OpenNSW/agency/backend/internal/taskconfig"
)

func TestResolvePushedFields_NoRules(t *testing.T) {
	got := resolvePushedFields(nil, map[string]any{"district": "Colombo"})
	if got != nil {
		t.Errorf("resolvePushedFields(nil rules) = %#v, want nil", got)
	}
}

func TestResolvePushedFields_ResolvesAndRenames(t *testing.T) {
	rules := []taskconfig.ConsignmentField{
		{Source: "/importer/address/district", Target: "/district"},
	}
	data := map[string]any{
		"importer": map[string]any{
			"address": map[string]any{"district": "Colombo"},
		},
	}

	got := resolvePushedFields(rules, data)
	want := map[string]any{"district": "Colombo"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("resolvePushedFields() = %#v, want %#v", got, want)
	}
}

func TestResolvePushedFields_SkipsUnresolvedSourcesWithoutError(t *testing.T) {
	rules := []taskconfig.ConsignmentField{
		{Source: "/district", Target: "/district"},     // resolves
		{Source: "/nope", Target: "/missing"},          // absent
		{Source: "/items/0/name", Target: "/itemName"}, // passes through an array
	}
	data := map[string]any{
		"district": "Colombo",
		"items":    []any{map[string]any{"name": "widget"}},
	}

	got := resolvePushedFields(rules, data)
	want := map[string]any{"district": "Colombo"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("resolvePushedFields() = %#v, want %#v", got, want)
	}
}

func TestResolvePushedFields_NothingResolvesReturnsNil(t *testing.T) {
	rules := []taskconfig.ConsignmentField{
		{Source: "/nope", Target: "/x"},
	}
	got := resolvePushedFields(rules, map[string]any{"district": "Colombo"})
	if got != nil {
		t.Errorf("resolvePushedFields() = %#v, want nil", got)
	}
}

func TestResolvePushedFields_NestedTarget(t *testing.T) {
	rules := []taskconfig.ConsignmentField{
		{Source: "/district", Target: "/location/district"},
	}
	got := resolvePushedFields(rules, map[string]any{"district": "Colombo"})
	want := map[string]any{"location": map[string]any{"district": "Colombo"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("resolvePushedFields() = %#v, want %#v", got, want)
	}
}

func TestResolvePushedFields_NilData(t *testing.T) {
	rules := []taskconfig.ConsignmentField{
		{Source: "/district", Target: "/district"},
	}
	got := resolvePushedFields(rules, nil)
	if got != nil {
		t.Errorf("resolvePushedFields(nil data) = %#v, want nil", got)
	}
}
