package application

import (
	"github.com/OpenNSW/agency/backend/internal/taskconfig"
	"github.com/OpenNSW/agency/backend/pkg/jsonpointer"
)

// resolvePushedFields evaluates rules against data, returning the values to
// merge onto the parent consignment's own dynamic data (see
// consignment.Store.MergeCustomData), keyed by the rule's raw Target JSON
// Pointer string (e.g. "/location/district") rather than pre-nested into a
// document. consignment.Store.MergeCustomData applies each pointer directly
// against the accumulated consignment document via jsonpointer.Set, so a
// nested target's siblings (e.g. a prior push's "/location/portOfEntry")
// survive — building a nested map here and merging it in wholesale would
// clobber them one level up.
//
// A rule whose source doesn't resolve — missing, or the path passes through
// an array — is silently skipped, not an error: source resolution is a
// runtime data question, not a config one (config-time syntax is already
// enforced by taskconfig.TaskConfig.Validate). Returns nil when there's
// nothing to push, so the common case (a task with no ConsignmentFields, or
// none of them resolving) costs nothing downstream.
func resolvePushedFields(rules []taskconfig.ConsignmentField, data map[string]any) map[string]any {
	var pushed map[string]any
	for _, rule := range rules {
		value, ok := jsonpointer.Get(data, rule.Source)
		if !ok {
			continue
		}
		if pushed == nil {
			pushed = map[string]any{}
		}
		pushed[rule.Target] = value
	}
	return pushed
}
