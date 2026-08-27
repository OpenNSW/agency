package rbac

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestRoleService(t *testing.T) *RoleService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&RoleRecord{}, &UserRoleRecord{}); err != nil {
		t.Fatalf("failed to migrate test tables: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	return NewRoleService(db)
}

func TestRoleService_Create(t *testing.T) {
	svc := newTestRoleService(t)

	role, err := svc.Create("lab_officer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role.ID == "" {
		t.Error("expected role ID to be generated")
	}
	if role.Name != "lab_officer" {
		t.Errorf("expected name %q, got %q", "lab_officer", role.Name)
	}
}

func TestRoleService_FindByName_Found(t *testing.T) {
	svc := newTestRoleService(t)

	created, err := svc.Create("lab_officer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found, err := svc.FindByName("lab_officer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, found.ID)
	}
}

func TestRoleService_FindByName_NotFound(t *testing.T) {
	svc := newTestRoleService(t)

	_, err := svc.FindByName("nonexistent")
	if !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("expected ErrRoleNotFound, got %v", err)
	}
}

func TestRoleService_List(t *testing.T) {
	svc := newTestRoleService(t)

	if _, err := svc.Create("lab_officer"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.Create("lab_manager"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	roles, err := svc.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}
}

func TestRoleService_Assign(t *testing.T) {
	svc := newTestRoleService(t)

	role, err := svc.Create("lab_officer")
	if err != nil {
		t.Fatalf("unexpected error creating role: %v", err)
	}

	if err := svc.Assign("user-1", role.ID); err != nil {
		t.Fatalf("unexpected error assigning role: %v", err)
	}
}

func TestRoleService_GetRolesForUser(t *testing.T) {
	svc := newTestRoleService(t)

	r1, _ := svc.Create("lab_officer")
	r2, _ := svc.Create("lab_manager")

	_ = svc.Assign("user-1", r1.ID)
	_ = svc.Assign("user-1", r2.ID)

	roles, err := svc.GetRolesForUser("user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}
}
