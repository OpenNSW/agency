package application

import (
	"github.com/OpenNSW/agency/backend/internal/taskconfig"
	"github.com/OpenNSW/agency/backend/pkg/jsonpointer"
)

// resolvePushedFields evaluates rules against data, returning the values to
// merge onto the parent consignment's own dynamic data (see
// consignment.Store.MergeCustomData). A rule whose source doesn't resolve —
// missing, or the path passes through an array — is silently skipped, not
// an error: source resolution is a runtime data question, not a config one
// (config-time syntax is already enforced by taskconfig.TaskConfig.Validate).
// Returns nil when there's nothing to push, so the common case (a task with
// no ConsignmentFields, or none of them resolving) costs nothing downstream.
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
		// Set's ok is intentionally ignored: a failure here means two of
		// this task's own rules have overlapping targets (one target is a
		// scalar prefix of another), which is the same class of authoring
		// conflict as two different tasks colliding on a target — silently
		// dropped, not an error, consistent throughout this feature.
		jsonpointer.Set(pushed, rule.Target, value)
	}
	return pushed
}
