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

func TestResolvePushedFields_ResolvesKeyedByTargetPointer(t *testing.T) {
	rules := []taskconfig.ConsignmentField{
		{Source: "/importer/address/district", Target: "/district"},
	}
	data := map[string]any{
		"importer": map[string]any{
			"address": map[string]any{"district": "Colombo"},
		},
	}

	got := resolvePushedFields(rules, data)
	want := map[string]any{"/district": "Colombo"}
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
	want := map[string]any{"/district": "Colombo"}
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

// TestResolvePushedFields_NestedTargetsStayAsPointerStrings guards against
// resolvePushedFields pre-nesting targets into a document itself (the
// earlier, buggy design): the map it returns is keyed by the raw Target
// pointer string, not a nested structure — consignment.Store.MergeCustomData
// is what applies each pointer against the accumulated document, which is
// what actually preserves siblings under a shared nested parent across
// separate merges. Two rules targeting siblings under the same parent here
// must produce two independent entries, not collide or nest.
func TestResolvePushedFields_NestedTargetsStayAsPointerStrings(t *testing.T) {
	rules := []taskconfig.ConsignmentField{
		{Source: "/a", Target: "/location/district"},
		{Source: "/b", Target: "/location/portOfEntry"},
	}
	data := map[string]any{"a": "Colombo", "b": "BIA"}

	got := resolvePushedFields(rules, data)
	want := map[string]any{
		"/location/district":    "Colombo",
		"/location/portOfEntry": "BIA",
	}
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
