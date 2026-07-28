package taskconfig

import "fmt"

// TaskConfig is the per-taskCode configuration: UI metadata, references to
// forms, and outcome-to-status behavior.
type TaskConfig struct {
	TaskCode        string           `json:"taskCode"`
	Meta            TaskMeta         `json:"meta"`
	Forms           TaskForms        `json:"forms"`
	Behavior        *TaskBehavior    `json:"behavior,omitempty"`
	ReferenceNumber *ReferenceNumber `json:"referenceNumber,omitempty"`
	Permissions     []Permission     `json:"permissions,omitempty"`
}

// Permission defines which actions a role is allowed to perform on a task.
// If a TaskConfig has no Permissions, all authenticated users can perform any action.
type Permission struct {
	Role    string   `json:"role"`
	Actions []string `json:"actions"`
}

// TaskMeta contains UI metadata for the task.
type TaskMeta struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Category    string `json:"category,omitempty"`
}

// TaskForms holds form IDs referenced by the task config.
type TaskForms struct {
	View   string `json:"view,omitempty"`
	Review string `json:"review,omitempty"`
}

// DefaultOutcomeField is the field name read from the review submission
// body when TaskBehavior.OutcomeField is not set.
const DefaultOutcomeField = "review_outcome"

// TaskBehavior defines automated logic based on task outcomes.
type TaskBehavior struct {
	// OutcomeField names the key in the review submission body whose value
	// is looked up in StatusMap. Defaults to "review_outcome" when empty.
	OutcomeField string            `json:"outcomeField,omitempty"`
	StatusMap    map[string]string `json:"statusMap,omitempty"`
}

// DefaultReferenceNumberField is the review-form field populated with the
// generated reference number when ReferenceNumber.Field is not set.
const DefaultReferenceNumberField = "reference_number"

// ReferenceNumber enables server-side allocation of a sequential reference
// number for every application of a task. Tasks without this block get no
// reference number at all.
type ReferenceNumber struct {
	// Field names the review-form property the generated value is written to.
	// Defaults to DefaultReferenceNumberField.
	Field string `json:"field,omitempty"`
	// AgencyCode identifies the counter the value is drawn from. Task codes
	// sharing an agency code share a single series. Defaults to the task code.
	AgencyCode string `json:"agencyCode,omitempty"`
	// Prefix is prepended verbatim to the zero-padded sequence value.
	Prefix string `json:"prefix,omitempty"`
	// MinDigits is the zero-padded width of the sequence value. Values that
	// no longer fit grow past it instead of being truncated.
	MinDigits int `json:"minDigits,omitempty"`
}

// FieldName returns the review-form field to populate.
func (r ReferenceNumber) FieldName() string {
	if r.Field != "" {
		return r.Field
	}
	return DefaultReferenceNumberField
}

// SequenceKey returns the counter identifier, falling back to the task code so
// that a config only has to name an agency code when sharing a series.
func (r ReferenceNumber) SequenceKey(taskCode string) string {
	if r.AgencyCode != "" {
		return r.AgencyCode
	}
	return taskCode
}

// Format renders a sequence value, e.g. prefix "034/" with MinDigits 5 turns
// sequence 1 into "034/00001" and sequence 100000 into "034/100000".
func (r ReferenceNumber) Format(seq int64) string {
	width := r.MinDigits
	if width < 1 {
		width = 1
	}
	return fmt.Sprintf("%s%0*d", r.Prefix, width, seq)
}
