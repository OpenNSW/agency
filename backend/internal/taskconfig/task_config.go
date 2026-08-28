package taskconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OpenNSW/agency/backend/pkg/jsonpointer"
)

// CurrentSchemaVersion is the only TaskConfig shape this build understands.
// A breaking change to the struct (a field changing meaning or required-ness,
// not an additive optional field) bumps this and adds a case for the old
// value where a migration period is needed; until then, Validate rejects
// anything else outright rather than silently misinterpreting a shape it
// wasn't built for.
const CurrentSchemaVersion = 1

// TaskConfig is the per-taskCode configuration: UI metadata, references to
// forms, and outcome-to-status behavior.
type TaskConfig struct {
	// SchemaVersion declares which TaskConfig shape this file conforms to.
	// Required, and must equal CurrentSchemaVersion (enforced by Validate).
	SchemaVersion int       `json:"schemaVersion"`
	TaskCode      string    `json:"taskCode"`
	Meta          TaskMeta  `json:"meta"`
	Forms         TaskForms `json:"forms"`
	// Behavior is required (enforced by Validate) — every task is
	// reviewable, so every task needs a resolution mode for the review
	// outcome. A value type, not a pointer: there's no meaningful nil state
	// to represent now that it's mandatory, so an absent/malformed behavior
	// shows up as a zero-value Type that Validate's switch rejects like any
	// other unrecognized type.
	Behavior    TaskBehavior     `json:"behavior"`
	Permissions []Permission     `json:"permissions,omitempty"`
	Certificate *TaskCertificate `json:"certificate,omitempty"`
	// ConsignmentFields declares values to extract from this task's injected
	// data and push onto the parent consignment's own dynamic data, e.g. for
	// location-based access control. Optional — most tasks have nothing
	// consignment-relevant to contribute. See Validate for the constraints
	// on each entry, and docs/consignment-custom-data.md for the full
	// design (in particular: why arrays are unsupported by design here).
	ConsignmentFields []ConsignmentField `json:"consignmentFields,omitempty"`
}

// Action names a permission action a role can be granted. These are the
// only actions any route in this service checks (see the route table in
// docs/task-config-reference.md); Validate rejects anything else outright
// rather than letting a typo silently 403 an officer at request time.
type Action = string

const (
	ActionView     Action = "VIEW"
	ActionReview   Action = "REVIEW"
	ActionFeedback Action = "FEEDBACK"
)

// allowedActions is the closed set Validate checks Permissions actions
// against.
var allowedActions = map[Action]bool{
	ActionView:     true,
	ActionReview:   true,
	ActionFeedback: true,
}

// requiredActions is the subset of allowedActions every task config must
// collectively grant, regardless of which role(s) hold them: every task is
// reviewable, so every task needs someone who can view it and someone who
// can decide it. FEEDBACK is not required — not every task's officer workflow
// sends data back to the trader for changes.
var requiredActions = []Action{ActionView, ActionReview}

// Validate reports an error if the config is missing required fields. Every
// task config must explicitly declare who can access it: Permissions must be
// non-empty, each entry must name a role and at least one action, actions
// must come from the closed set above, and VIEW/REVIEW must each be granted
// to at least one role. This closes off the old implicit default of granting
// every authenticated user full access whenever a config omitted
// permissions.
//
// forms.review and behavior are both required unconditionally: every task
// in this system is something an officer reviews (even an autoApprove task
// has a review form — it just carries no decision), so there's no task
// shape where either is legitimately absent.
func (c TaskConfig) Validate() error {
	if c.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("taskconfig %q: schemaVersion must be %d, got %d", c.TaskCode, CurrentSchemaVersion, c.SchemaVersion)
	}
	if len(c.Permissions) == 0 {
		return fmt.Errorf("taskconfig %q: permissions is required and must include at least one entry", c.TaskCode)
	}
	if strings.TrimSpace(c.Forms.Review) == "" {
		return fmt.Errorf("taskconfig %q: forms.review is required", c.TaskCode)
	}
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
	granted := map[Action]bool{}
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
			if !allowedActions[action] {
				return fmt.Errorf("taskconfig %q: permissions[%d].actions[%d] must be one of %q, %q, %q, got %q", c.TaskCode, i, j, ActionView, ActionReview, ActionFeedback, action)
			}
			granted[action] = true
		}
	}
	for _, action := range requiredActions {
		if !granted[action] {
			return fmt.Errorf("taskconfig %q: permissions must grant %q to at least one role", c.TaskCode, action)
		}
	}
	for i, f := range c.ConsignmentFields {
		if !jsonpointer.Valid(f.Source) {
			return fmt.Errorf("taskconfig %q: consignmentFields[%d].source must be a JSON Pointer (e.g. \"/importer/district\"), got %q", c.TaskCode, i, f.Source)
		}
		if !jsonpointer.Valid(f.Target) {
			return fmt.Errorf("taskconfig %q: consignmentFields[%d].target must be a JSON Pointer (e.g. \"/district\"), got %q", c.TaskCode, i, f.Target)
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
	View string `json:"view,omitempty"`
	// Review is required (enforced by Validate): every task is reviewable,
	// so every task has a review form for the officer to act on.
	Review string `json:"review"`
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

// ConsignmentField declares one value to copy from this task's injected
// data onto the parent consignment's own dynamic data. Source and Target
// are both JSON Pointers (RFC 6901, e.g. "/importer/address/district").
// Source is resolved against the injected data at inject time — a source
// that doesn't resolve (missing, or the path passes through an array) is
// simply skipped, not an error, since that's a runtime data question, not
// a config one. Target must still be syntactically valid (enforced by
// Validate): an author writing a malformed target is a config bug, and
// should fail at load time rather than silently never taking effect.
type ConsignmentField struct {
	Source string `json:"source"`
	Target string `json:"target"`
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
