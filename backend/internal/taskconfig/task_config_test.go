package taskconfig

import "testing"

func TestValidate_SchemaVersion_Missing(t *testing.T) {
	c := TaskConfig{
		TaskCode:    "alpha",
		Permissions: []Permission{{Role: "officer", Actions: []string{"VIEW"}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when schemaVersion is omitted (zero value)")
	}
}

func TestValidate_SchemaVersion_Unsupported(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: 2,
		TaskCode:      "alpha",
		Permissions:   []Permission{{Role: "officer", Actions: []string{"VIEW"}}},
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

func TestValidate_PermissionMissingRole(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Permissions:   []Permission{{Role: "", Actions: []string{"VIEW"}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when a permission entry has no role")
	}
}

func TestValidate_PermissionWhitespaceRole(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Permissions:   []Permission{{Role: "   ", Actions: []string{"VIEW"}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when a permission entry has a whitespace-only role")
	}
}

func TestValidate_PermissionMissingActions(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
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
		Permissions:   []Permission{{Role: "officer", Actions: []string{"VIEW", ""}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when a permission entry has an empty action")
	}
}

func TestValidate_Valid(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Permissions:   []Permission{{Role: "officer", Actions: []string{"VIEW", "REVIEW"}}},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("expected no error for a valid config, got %v", err)
	}
}

func TestValidate_Behavior_MissingType(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Permissions:   []Permission{{Role: "officer", Actions: []string{"VIEW", "REVIEW"}}},
		Behavior:      &TaskBehavior{StatusMap: map[string]string{"approve": "APPROVED"}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when behavior is present but type is empty")
	}
}

func TestValidate_Behavior_UnknownType(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Permissions:   []Permission{{Role: "officer", Actions: []string{"VIEW", "REVIEW"}}},
		Behavior:      &TaskBehavior{Type: "somethingElse"},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for an unrecognized behavior.type")
	}
}

func TestValidate_StatusMap_Valid(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Permissions:   []Permission{{Role: "officer", Actions: []string{"VIEW", "REVIEW"}}},
		Behavior: &TaskBehavior{
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
		Permissions:   []Permission{{Role: "officer", Actions: []string{"VIEW", "REVIEW"}}},
		Behavior:      &TaskBehavior{Type: BehaviorTypeAutoApprove},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("expected no error for autoApprove alone, got %v", err)
	}
}

func TestValidate_AutoApprove_ConflictsWithOutcomeField(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Permissions:   []Permission{{Role: "officer", Actions: []string{"VIEW", "REVIEW"}}},
		Behavior:      &TaskBehavior{Type: BehaviorTypeAutoApprove, OutcomeField: "decision"},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when autoApprove is combined with outcomeField")
	}
}

func TestValidate_AutoApprove_ConflictsWithStatusMap(t *testing.T) {
	c := TaskConfig{
		SchemaVersion: CurrentSchemaVersion,
		TaskCode:      "alpha",
		Permissions:   []Permission{{Role: "officer", Actions: []string{"VIEW", "REVIEW"}}},
		Behavior:      &TaskBehavior{Type: BehaviorTypeAutoApprove, StatusMap: map[string]string{"approve": "APPROVED"}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when autoApprove is combined with statusMap")
	}
}
