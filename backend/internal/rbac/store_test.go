package rbac

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newTestStores creates a shared in-memory SQLite DB and returns both stores
// wired to the same connection, which is needed for the JOIN in GetRolesForUser.
func newTestStores(t *testing.T) (*RoleStore, *UserRoleStore) {
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
	return NewRoleStore(db), NewUserRoleStore(db)
}

// ---------- RoleStore ----------

func TestRoleStore_Create(t *testing.T) {
	rs, _ := newTestStores(t)

	role, err := rs.Create("lab_officer")
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

func TestRoleStore_Create_DuplicateName(t *testing.T) {
	rs, _ := newTestStores(t)

	if _, err := rs.Create("lab_officer"); err != nil {
		t.Fatalf("unexpected error on first create: %v", err)
	}
	if _, err := rs.Create("lab_officer"); err == nil {
		t.Error("expected error on duplicate role name, got nil")
	}
}

func TestRoleStore_FindByName_Found(t *testing.T) {
	rs, _ := newTestStores(t)

	created, err := rs.Create("lab_officer")
	if err != nil {
		t.Fatalf("unexpected error creating role: %v", err)
	}

	found, err := rs.FindByName("lab_officer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, found.ID)
	}
}

func TestRoleStore_FindByName_NotFound(t *testing.T) {
	rs, _ := newTestStores(t)

	_, err := rs.FindByName("nonexistent")
	if !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("expected ErrRoleNotFound, got %v", err)
	}
}

func TestRoleStore_List(t *testing.T) {
	rs, _ := newTestStores(t)

	if _, err := rs.Create("lab_officer"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := rs.Create("lab_manager"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	roles, err := rs.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}
}

// ---------- UserRoleStore ----------

func TestUserRoleStore_Assign(t *testing.T) {
	rs, urs := newTestStores(t)

	role, err := rs.Create("lab_officer")
	if err != nil {
		t.Fatalf("unexpected error creating role: %v", err)
	}

	if err := urs.Assign("user-1", role.ID); err != nil {
		t.Fatalf("unexpected error assigning role: %v", err)
	}
}

func TestUserRoleStore_Assign_Duplicate(t *testing.T) {
	rs, urs := newTestStores(t)

	role, err := rs.Create("lab_officer")
	if err != nil {
		t.Fatalf("unexpected error creating role: %v", err)
	}

	if err := urs.Assign("user-1", role.ID); err != nil {
		t.Fatalf("unexpected error on first assign: %v", err)
	}
	// Second assign of the same role is silently ignored (ON CONFLICT DO NOTHING).
	if err := urs.Assign("user-1", role.ID); err != nil {
		t.Errorf("expected no error on duplicate assignment, got: %v", err)
	}
}

func TestUserRoleStore_GetRolesForUser(t *testing.T) {
	rs, urs := newTestStores(t)

	r1, _ := rs.Create("lab_officer")
	r2, _ := rs.Create("lab_manager")

	_ = urs.Assign("user-1", r1.ID)
	_ = urs.Assign("user-1", r2.ID)

	roles, err := urs.GetRolesForUser("user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}
}

func TestUserRoleStore_GetRolesForUser_NoRoles(t *testing.T) {
	_, urs := newTestStores(t)

	roles, err := urs.GetRolesForUser("user-unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 0 {
		t.Errorf("expected 0 roles, got %d", len(roles))
	}
}
