package taskconfig

import (
	"encoding/json"
	"fmt"
	"strings"
)

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

// Validate reports an error if the config is missing required fields. Every
// task config must explicitly declare who can access it: Permissions must be
// non-empty, and each entry must name a role and at least one action. This
// closes off the old implicit default of granting every authenticated user
// full access whenever a config omitted permissions.
func (c TaskConfig) Validate() error {
	if len(c.Permissions) == 0 {
		return fmt.Errorf("taskconfig %q: permissions is required and must include at least one entry", c.TaskCode)
	}
	if c.Behavior != nil {
		switch c.Behavior.Type {
		case BehaviorTypeStatusMap:
			// OutcomeField/StatusMap are both optional here: an unmatched or
			// absent outcome simply falls through to the DONE default.
		case BehaviorTypeAutoApprove:
			if c.Behavior.OutcomeField != "" || len(c.Behavior.StatusMap) > 0 {
				return fmt.Errorf("taskconfig %q: behavior.type %q cannot be combined with outcomeField or statusMap", c.TaskCode, BehaviorTypeAutoApprove)
			}
		default:
			return fmt.Errorf("taskconfig %q: behavior.type must be %q or %q, got %q", c.TaskCode, BehaviorTypeStatusMap, BehaviorTypeAutoApprove, c.Behavior.Type)
		}
	}
	for i, p := range c.Permissions {
		if strings.TrimSpace(p.Role) == "" {
			return fmt.Errorf("taskconfig %q: permissions[%d].role must not be empty", c.TaskCode, i)
		}
		if len(p.Actions) == 0 {
			return fmt.Errorf("taskconfig %q: permissions[%d].actions must include at least one entry", c.TaskCode, i)
		}
		for j, action := range p.Actions {
			if strings.TrimSpace(action) == "" {
				return fmt.Errorf("taskconfig %q: permissions[%d].actions[%d] must not be empty", c.TaskCode, i, j)
			}
		}
	}
	return nil
}

// Permission defines which actions a role is allowed to perform on a task.
// Every TaskConfig must declare at least one Permission (enforced by
// Validate) — a task code with no config at all is a separate case, denied
// by default by rbac.Middleware and the application service, since there
// are no permissions to grant anyone.
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
// while reviewing this task, e.g. via POST /api/v1/applications/{taskId}/certificate.
type TaskCertificate struct {
	TemplateID string `json:"templateId"`
	// DataSchema is a JSON Schema, validated client-side before generation,
	// describing what POST /api/v1/applications/{taskId}/certificate's data payload must
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

// BehaviorType selects how a review submission resolves to a final
// application status. Required whenever Behavior is present (enforced by
// Validate); the two variants are mutually exclusive by construction.
type BehaviorType string

const (
	// BehaviorTypeStatusMap resolves the outcome by reading OutcomeField
	// from the review submission body and looking its value up in
	// StatusMap.
	BehaviorTypeStatusMap BehaviorType = "statusMap"

	// BehaviorTypeAutoApprove declares that the task's review form carries
	// no officer decision (pure data-capture/confirmation/issuance). Any
	// successful review submission resolves unconditionally to APPROVED —
	// no body field is read, no StatusMap lookup happens.
	BehaviorTypeAutoApprove BehaviorType = "autoApprove"
)

// TaskBehavior defines automated logic based on task outcomes. Type
// selects the resolution mode; OutcomeField/StatusMap are only meaningful
// for BehaviorTypeStatusMap (enforced by Validate).
type TaskBehavior struct {
	Type BehaviorType `json:"type"`

	// OutcomeField names the key in the review submission body whose value
	// is looked up in StatusMap. Defaults to "review_outcome" when empty.
	OutcomeField string            `json:"outcomeField,omitempty"`
	StatusMap    map[string]string `json:"statusMap,omitempty"`
}
