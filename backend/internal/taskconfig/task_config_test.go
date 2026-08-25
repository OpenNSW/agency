package taskconfig

import "testing"

func TestValidate_MissingPermissions(t *testing.T) {
	c := TaskConfig{TaskCode: "alpha"}
	if err := c.Validate(); err == nil {
		t.Error("expected error when permissions is omitted")
	}
}

func TestValidate_EmptyPermissions(t *testing.T) {
	c := TaskConfig{TaskCode: "alpha", Permissions: []Permission{}}
	if err := c.Validate(); err == nil {
		t.Error("expected error when permissions is an empty slice")
	}
}

func TestValidate_PermissionMissingRole(t *testing.T) {
	c := TaskConfig{
		TaskCode:    "alpha",
		Permissions: []Permission{{Role: "", Actions: []string{"VIEW"}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when a permission entry has no role")
	}
}

func TestValidate_PermissionWhitespaceRole(t *testing.T) {
	c := TaskConfig{
		TaskCode:    "alpha",
		Permissions: []Permission{{Role: "   ", Actions: []string{"VIEW"}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when a permission entry has a whitespace-only role")
	}
}

func TestValidate_PermissionMissingActions(t *testing.T) {
	c := TaskConfig{
		TaskCode:    "alpha",
		Permissions: []Permission{{Role: "officer", Actions: nil}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when a permission entry has no actions")
	}
}

func TestValidate_PermissionEmptyAction(t *testing.T) {
	c := TaskConfig{
		TaskCode:    "alpha",
		Permissions: []Permission{{Role: "officer", Actions: []string{"VIEW", ""}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when a permission entry has an empty action")
	}
}

func TestValidate_Valid(t *testing.T) {
	c := TaskConfig{
		TaskCode:    "alpha",
		Permissions: []Permission{{Role: "officer", Actions: []string{"VIEW", "REVIEW"}}},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("expected no error for a valid config, got %v", err)
	}
}
