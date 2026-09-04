package taskconfig

import "testing"

// validPermissions grants exactly the actions Validate requires every task
// config to collectively grant, so tests that aren't about permissions
// don't fail on that check instead of the one they mean to exercise.
func validPermissions() []Permission {
	return []Permission{{Role: "officer", Actions: []string{ActionView, ActionReview}}}
}

func validBehavior() TaskBehavior {
	return TaskBehavior{Type: BehaviorTypeStatusMap, StatusMap: map[string]string{"approve": "APPROVED"}}
}

func TestValidate_SchemaVersion_Missing(t *testing.T) {
	c := TaskConfig{
		TaskCode:    "alpha",
		Permissions: validPermissions(),
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when schemaVersion is omitted (zero value)")
	}
}

func TestValidate_SchemaVersion_Unsupported(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: 2,
		TaskCode:      "alpha",
		Permissions:   validPermissions(),
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for an unsupported schemaVersion")
	}
}

func TestValidate_MissingPermissions(t *testing.T) {
	c := TaskConfig{SchemaVersion: CurrentSchemaVersion, TaskCode: "alpha"}
	if err := c.Validate(); err == nil {
		t.Error("expected error when permissions is omitted")
	}
}

func TestValidate_EmptyPermissions(t *testing.T) {
	c := TaskConfig{SchemaVersion: CurrentSchemaVersion, TaskCode: "alpha", Permissions: []Permission{}}
	if err := c.Validate(); err == nil {
		t.Error("expected error when permissions is an empty slice")
	}
}

func TestValidate_MissingFormsReview(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Permissions:   validPermissions(),
		Behavior:      validBehavior(),
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when forms.review is omitted")
	}
}

func TestValidate_MissingBehavior(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Permissions:   validPermissions(),
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when behavior is omitted")
	}
}

func TestValidate_PermissionMissingRole(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Behavior:      validBehavior(),
		Permissions:   []Permission{{Role: "", Actions: []string{ActionView, ActionReview, ActionFeedback}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when a permission entry has no role")
	}
}

func TestValidate_PermissionWhitespaceRole(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Behavior:      validBehavior(),
		Permissions:   []Permission{{Role: "   ", Actions: []string{ActionView, ActionReview, ActionFeedback}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when a permission entry has a whitespace-only role")
	}
}

func TestValidate_PermissionMissingActions(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Behavior:      validBehavior(),
		Permissions:   []Permission{{Role: "officer", Actions: nil}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when a permission entry has no actions")
	}
}

func TestValidate_PermissionEmptyAction(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Behavior:      validBehavior(),
		Permissions:   []Permission{{Role: "officer", Actions: []string{ActionView, ""}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when a permission entry has an empty action")
	}
}

func TestValidate_PermissionUnknownAction(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Behavior:      validBehavior(),
		Permissions:   []Permission{{Role: "officer", Actions: []string{ActionView, ActionReview, ActionFeedback, "APPROVE"}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when a permission entry grants an action outside VIEW/REVIEW/FEEDBACK")
	}
}

func TestValidate_PermissionMissingViewOrReviewAction(t *testing.T) {
	cases := []struct {
		name    string
		actions []string
	}{
		{"no VIEW", []string{ActionReview}},
		{"no REVIEW", []string{ActionView}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := TaskConfig{
				SchemaVersion: CurrentSchemaVersion,
				TaskCode:      "alpha",
				Forms:         TaskForms{Review: "review-form"},
				Behavior:      validBehavior(),
				Permissions:   []Permission{{Role: "officer", Actions: tc.actions}},
			}
			if err := c.Validate(); err == nil {
				t.Errorf("expected error when no role is granted %s", tc.name)
			}
		})
	}
}

func TestValidate_PermissionFeedbackActionOptional(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Behavior:      validBehavior(),
		Permissions:   []Permission{{Role: "officer", Actions: []string{ActionView, ActionReview}}},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("expected no error when no role is granted FEEDBACK (not required), got %v", err)
	}
}

func TestValidate_PermissionActionsGrantedAcrossRoles(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Behavior:      validBehavior(),
		Permissions: []Permission{
			{Role: "trader", Actions: []string{ActionView}},
			{Role: "officer", Actions: []string{ActionReview, ActionFeedback}},
		},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("expected no error when VIEW/REVIEW/FEEDBACK are granted across different roles, got %v", err)
	}
}

func TestValidate_Valid(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Behavior:      validBehavior(),
		Permissions:   validPermissions(),
	}
	if err := c.Validate(); err != nil {
		t.Errorf("expected no error for a valid config, got %v", err)
	}
}

func TestValidate_Behavior_MissingType(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Permissions:   validPermissions(),
		Behavior:      TaskBehavior{StatusMap: map[string]string{"approve": "APPROVED"}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when behavior is present but type is empty")
	}
}

func TestValidate_Behavior_UnknownType(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Permissions:   validPermissions(),
		Behavior:      TaskBehavior{Type: "somethingElse"},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for an unrecognized behavior.type")
	}
}

func TestValidate_StatusMap_Valid(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Permissions:   validPermissions(),
		Behavior: TaskBehavior{
			Type:      BehaviorTypeStatusMap,
			StatusMap: map[string]string{"approve": "APPROVED"},
		},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("expected no error for a valid statusMap behavior, got %v", err)
	}
}

func TestValidate_AutoApprove_Valid(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Permissions:   validPermissions(),
		Behavior:      TaskBehavior{Type: BehaviorTypeAutoApprove},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("expected no error for autoApprove alone, got %v", err)
	}
}

func TestValidate_AutoApprove_ConflictsWithOutcomeField(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Permissions:   validPermissions(),
		Behavior:      TaskBehavior{Type: BehaviorTypeAutoApprove, OutcomeField: "decision"},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when autoApprove is combined with outcomeField")
	}
}

func TestValidate_AutoApprove_ConflictsWithStatusMap(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Permissions:   validPermissions(),
		Behavior:      TaskBehavior{Type: BehaviorTypeAutoApprove, StatusMap: map[string]string{"approve": "APPROVED"}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when autoApprove is combined with statusMap")
	}
}

func TestValidate_ConsignmentFields_Omitted(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Behavior:      validBehavior(),
		Permissions:   validPermissions(),
	}
	if err := c.Validate(); err != nil {
		t.Errorf("expected no error when consignmentFields is omitted, got %v", err)
	}
}

func TestValidate_ConsignmentFields_Valid(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Behavior:      validBehavior(),
		Permissions:   validPermissions(),
		ConsignmentFields: []ConsignmentField{
			{Source: "/importer/address/district", Target: "/district"},
			{Source: "/logistics/portOfEntry", Target: "/portOfEntry"},
		},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("expected no error for valid consignmentFields, got %v", err)
	}
}

func TestValidate_ConsignmentFields_MissingSlash(t *testing.T) {
	cases := []struct {
		name  string
		field ConsignmentField
	}{
		{"source missing leading slash", ConsignmentField{Source: "district", Target: "/district"}},
		{"target missing leading slash", ConsignmentField{Source: "/district", Target: "district"}},
		{"source empty", ConsignmentField{Source: "", Target: "/district"}},
		{"target empty", ConsignmentField{Source: "/district", Target: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := TaskConfig{
				SchemaVersion:     CurrentSchemaVersion,
				TaskCode:          "alpha",
				Forms:             TaskForms{Review: "review-form"},
				Behavior:          validBehavior(),
				Permissions:       validPermissions(),
				ConsignmentFields: []ConsignmentField{tc.field},
			}
			if err := c.Validate(); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

// baseConfigWithRefID returns a minimal valid config carrying refID, so each
// case below varies only the refid block.
func baseConfigWithRefID(refID *TaskRefID) TaskConfig {
	return TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Forms:         TaskForms{Review: "review-form"},
		Behavior:      validBehavior(),
		Permissions:   validPermissions(),
		RefID:         refID,
	}
}

func TestValidate_RefID_Omitted(t *testing.T) {
	if err := baseConfigWithRefID(nil).Validate(); err != nil {
		t.Errorf("expected no error when refid is omitted, got %v", err)
	}
}

func TestValidate_RefID_Valid(t *testing.T) {
	cases := []struct {
		name  string
		refID TaskRefID
	}{
		{"without params", TaskRefID{
			Issuer: "NPQS", IDType: "application_id", Path: "/reference_number",
		}},
		{"with params", TaskRefID{
			Issuer: "NPQS", IDType: "application_id", Path: "/reference_number",
			Params: map[string]string{"officeCode": "/nppo_office_location"},
		}},
		{"nested path", TaskRefID{
			Issuer: "NPQS", IDType: "application_id", Path: "/registration/reference_number",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := baseConfigWithRefID(&tc.refID).Validate(); err != nil {
				t.Errorf("expected no error for a valid refid, got %v", err)
			}
		})
	}
}

func TestValidate_RefID_Rejected(t *testing.T) {
	cases := []struct {
		name  string
		refID TaskRefID
	}{
		{"empty issuer", TaskRefID{
			Issuer: "", IDType: "application_id", Path: "/reference_number",
		}},
		{"blank issuer", TaskRefID{
			Issuer: "  ", IDType: "application_id", Path: "/reference_number",
		}},
		{"empty idType", TaskRefID{
			Issuer: "NPQS", IDType: "", Path: "/reference_number",
		}},
		{"empty path", TaskRefID{
			Issuer: "NPQS", IDType: "application_id", Path: "",
		}},
		{"path missing leading slash", TaskRefID{
			Issuer: "NPQS", IDType: "application_id", Path: "reference_number",
		}},
		{"path with a bad escape", TaskRefID{
			Issuer: "NPQS", IDType: "application_id", Path: "/ref~2num",
		}},
		{"param pointer missing leading slash", TaskRefID{
			Issuer: "NPQS", IDType: "application_id", Path: "/reference_number",
			Params: map[string]string{"officeCode": "nppo_office_location"},
		}},
		{"param pointer empty", TaskRefID{
			Issuer: "NPQS", IDType: "application_id", Path: "/reference_number",
			Params: map[string]string{"officeCode": ""},
		}},
		{"empty param name", TaskRefID{
			Issuer: "NPQS", IDType: "application_id", Path: "/reference_number",
			Params: map[string]string{"": "/nppo_office_location"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := baseConfigWithRefID(&tc.refID).Validate(); err == nil {
				t.Errorf("expected an error for %s, got nil", tc.name)
			}
		})
	}
}
