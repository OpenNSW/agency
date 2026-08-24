package taskconfig

import "encoding/json"

// TaskConfig is the per-taskCode configuration: UI metadata, references to
// forms, and outcome-to-status behavior.
type TaskConfig struct {
	TaskCode    string           `json:"taskCode"`
	Meta        TaskMeta         `json:"meta"`
	Forms       TaskForms        `json:"forms"`
	Behavior    *TaskBehavior    `json:"behavior,omitempty"`
	Permissions []Permission     `json:"permissions,omitempty"`
	Certificate *TaskCertificate `json:"certificate,omitempty"`
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

// TaskCertificate references a certificate template an officer can generate
// while reviewing this task, e.g. via POST /api/v1/certificates/generate.
type TaskCertificate struct {
	TemplateID string `json:"templateId"`
	// DataSchema is a JSON Schema, validated client-side before generation,
	// describing what POST /api/v1/certificates/generate's data payload must
	// look like for this task (e.g. requiring "certificate_id"). This is
	// deliberately separate from the review form's own schema: the review
	// form's required fields include things — a signed certificate upload,
	// an authorized signature — that can only be provided after the
	// certificate has been generated and printed, so reusing that schema
	// verbatim would block generation on fields that come later.
	DataSchema json.RawMessage `json:"dataSchema,omitempty"`
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
