package user

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/agency/backend/internal/authn"
	"github.com/OpenNSW/agency/backend/internal/rbac"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newTestUserService creates a UserService backed by an in-memory SQLite DB
// with all required tables migrated, with no custom-data schema configured.
func newTestUserService(t *testing.T) *UserService {
	t.Helper()
	return newTestUserServiceWithSchema(t, nil)
}

// newTestUserServiceWithSchema is like newTestUserService but configures the
// given custom-data schema (nil to skip validation, same as
// newTestUserService).
func newTestUserServiceWithSchema(t *testing.T, customDataSchema json.RawMessage) *UserService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&UserRecord{}, &rbac.RoleRecord{}, &rbac.UserRoleRecord{}); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}
	// Enable foreign key enforcement for SQLite (required for ON DELETE CASCADE).
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	return NewUserService(db, customDataSchema)
}

// ---------- CreateBulk ----------

func TestUserService_CreateBulk_NewUsers(t *testing.T) {
	svc := newTestUserService(t)

	inserted, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}},
		{Name: "John Doe", Email: "john@agency.gov.au", Roles: []string{"lab_officer"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inserted != 2 {
		t.Errorf("expected 2 inserted, got %d", inserted)
	}
}

func TestUserService_CreateBulk_CreatesRoles(t *testing.T) {
	svc := newTestUserService(t)

	_, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer", "lab_manager"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	roleService := rbac.NewRoleService(svc.db)
	roles, err := roleService.List()
	if err != nil {
		t.Fatalf("unexpected error listing roles: %v", err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2 roles created, got %d", len(roles))
	}
}

func TestUserService_CreateBulk_ReusesExistingRoles(t *testing.T) {
	svc := newTestUserService(t)

	roleService := rbac.NewRoleService(svc.db)
	if _, err := roleService.Create("lab_officer"); err != nil {
		t.Fatalf("failed to pre-create role: %v", err)
	}

	_, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	roles, err := roleService.List()
	if err != nil {
		t.Fatalf("unexpected error listing roles: %v", err)
	}
	if len(roles) != 1 {
		t.Errorf("expected 1 role (reused), got %d", len(roles))
	}
}

func TestUserService_CreateBulk_ExistingUserSkipped(t *testing.T) {
	svc := newTestUserService(t)

	if _, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}},
	}); err != nil {
		t.Fatalf("unexpected error on first seed: %v", err)
	}

	inserted, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}},
	})
	if err != nil {
		t.Fatalf("unexpected error on second seed: %v", err)
	}
	if inserted != 0 {
		t.Errorf("expected 0 inserted for existing user, got %d", inserted)
	}
}

func TestUserService_CreateBulk_DeduplicatesEmailsInInput(t *testing.T) {
	svc := newTestUserService(t)

	inserted, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}},
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inserted != 1 {
		t.Errorf("expected 1 inserted after dedup, got %d", inserted)
	}
}

func TestUserService_CreateBulk_AssignsRolesToUser(t *testing.T) {
	svc := newTestUserService(t)

	if _, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer", "lab_manager"}},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var u UserRecord
	if err := svc.db.First(&u, "email = ?", "jane@agency.gov.au").Error; err != nil {
		t.Fatalf("failed to fetch user: %v", err)
	}

	roleService := rbac.NewRoleService(svc.db)
	roles, err := roleService.GetRolesForUser(u.UserID)
	if err != nil {
		t.Fatalf("failed to get roles: %v", err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2 role assignments, got %d", len(roles))
	}
}

func TestUserService_CreateBulk_EmptyInput(t *testing.T) {
	svc := newTestUserService(t)

	inserted, err := svc.CreateBulk([]BulkInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inserted != 0 {
		t.Errorf("expected 0 inserted for empty input, got %d", inserted)
	}
}

// ---------- CreateBulk: custom data validation ----------

const testCustomDataSchema = `{"type":"object","required":["badgeNumber"],"properties":{"badgeNumber":{"type":"string"}}}`

func TestUserService_CreateBulk_NoSchemaSkipsCustomDataValidation(t *testing.T) {
	svc := newTestUserService(t) // no schema configured

	inserted, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}, CustomData: map[string]any{"anything": "goes"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inserted != 1 {
		t.Errorf("expected 1 inserted, got %d", inserted)
	}
}

func TestUserService_CreateBulk_ValidCustomDataIsPersisted(t *testing.T) {
	svc := newTestUserServiceWithSchema(t, json.RawMessage(testCustomDataSchema))

	if _, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}, CustomData: map[string]any{"badgeNumber": "12345"}},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var u UserRecord
	if err := svc.db.First(&u, "email = ?", "jane@agency.gov.au").Error; err != nil {
		t.Fatalf("failed to fetch user: %v", err)
	}
	if u.CustomData["badgeNumber"] != "12345" {
		t.Errorf("CustomData[badgeNumber] = %v, want 12345", u.CustomData["badgeNumber"])
	}
}

func TestUserService_CreateBulk_InvalidCustomDataAbortsBatch(t *testing.T) {
	svc := newTestUserServiceWithSchema(t, json.RawMessage(testCustomDataSchema))

	_, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}, CustomData: map[string]any{"badgeNumber": "12345"}},
		{Name: "John Doe", Email: "john@agency.gov.au", Roles: []string{"lab_officer"}, CustomData: map[string]any{"wrongField": "x"}},
	})
	if !errors.Is(err, ErrInvalidCustomData) {
		t.Fatalf("expected ErrInvalidCustomData, got %v", err)
	}

	// The returned count is not meaningful on error (it reflects progress
	// before the transaction rolled back) — what matters is that no rows
	// were actually persisted.
	var count int64
	svc.db.Model(&UserRecord{}).Count(&count)
	if count != 0 {
		t.Errorf("expected no users persisted after an aborted batch, got %d", count)
	}
}

func TestUserService_CreateBulk_MissingCustomDataFailsRequiredSchema(t *testing.T) {
	svc := newTestUserServiceWithSchema(t, json.RawMessage(testCustomDataSchema))

	_, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}},
	})
	if !errors.Is(err, ErrInvalidCustomData) {
		t.Fatalf("expected ErrInvalidCustomData for missing required customData, got %v", err)
	}
}

func TestUserService_CreateBulk_CustomDataNotReappliedOnExistingUser(t *testing.T) {
	svc := newTestUserServiceWithSchema(t, json.RawMessage(testCustomDataSchema))

	if _, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}, CustomData: map[string]any{"badgeNumber": "12345"}},
	}); err != nil {
		t.Fatalf("unexpected error on first seed: %v", err)
	}

	// Re-seeding with no customData at all must not fail schema validation for
	// the already-existing user (only newly-created users are validated).
	inserted, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}},
	})
	if err != nil {
		t.Fatalf("unexpected error on second seed: %v", err)
	}
	if inserted != 0 {
		t.Errorf("expected 0 inserted for existing user, got %d", inserted)
	}

	var u UserRecord
	if err := svc.db.First(&u, "email = ?", "jane@agency.gov.au").Error; err != nil {
		t.Fatalf("failed to fetch user: %v", err)
	}
	if u.CustomData["badgeNumber"] != "12345" {
		t.Errorf("CustomData should be unchanged after re-seeding, got %v", u.CustomData)
	}
}

// ---------- DropUser ----------

func TestUserService_DropUser_ExistingUser(t *testing.T) {
	svc := newTestUserService(t)

	if _, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}},
	}); err != nil {
		t.Fatalf("unexpected error seeding user: %v", err)
	}

	if err := svc.DropUser("jane@agency.gov.au"); err != nil {
		t.Fatalf("unexpected error dropping user: %v", err)
	}

	var u UserRecord
	err := svc.db.First(&u, "email = ?", "jane@agency.gov.au").Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("expected user to be deleted, got err: %v", err)
	}
}

func TestUserService_DropUser_RemovesRoleAssignments(t *testing.T) {
	svc := newTestUserService(t)

	if _, err := svc.CreateBulk([]BulkInput{
		{Name: "Jane Doe", Email: "jane@agency.gov.au", Roles: []string{"lab_officer"}},
	}); err != nil {
		t.Fatalf("unexpected error seeding user: %v", err)
	}

	var u UserRecord
	if err := svc.db.First(&u, "email = ?", "jane@agency.gov.au").Error; err != nil {
		t.Fatalf("failed to fetch user: %v", err)
	}

	if err := svc.DropUser("jane@agency.gov.au"); err != nil {
		t.Fatalf("unexpected error dropping user: %v", err)
	}

	roleService := rbac.NewRoleService(svc.db)
	roles, err := roleService.GetRolesForUser(u.UserID)
	if err != nil {
		t.Fatalf("unexpected error fetching roles: %v", err)
	}
	if len(roles) != 0 {
		t.Errorf("expected role assignments to be removed via CASCADE, got %d", len(roles))
	}
}

func TestUserService_DropUser_NotFound(t *testing.T) {
	svc := newTestUserService(t)

	err := svc.DropUser("nonexistent@agency.gov.au")
	if err == nil {
		t.Error("expected error for non-existent user, got nil")
	}
}

// ---------- ProfileService ----------

func newTestProfileService(t *testing.T) (*ProfileService, *rbac.RoleService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&rbac.RoleRecord{}, &rbac.UserRoleRecord{}); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	roleService := rbac.NewRoleService(db)
	return NewProfileService(roleService), roleService
}

func TestProfileService_GetMe_NoAuthContext(t *testing.T) {
	svc, _ := newTestProfileService(t)

	_, err := svc.GetMe(context.Background())
	if err == nil {
		t.Error("expected error for missing auth context, got nil")
	}
}

func TestProfileService_GetMe_ReturnsProfile(t *testing.T) {
	svc, roleService := newTestProfileService(t)

	const userID = "user-001"
	role, err := roleService.Create("lab_officer")
	if err != nil {
		t.Fatalf("failed to create role: %v", err)
	}
	if err := roleService.Assign(userID, role.ID); err != nil {
		t.Fatalf("failed to assign role: %v", err)
	}

	ctx := authn.ContextWithPrincipal(context.Background(), &authn.Principal{Kind: authn.KindUser, UserID: userID, Email: "jane@agency.gov.au", GivenName: "Jane"})

	result, err := svc.GetMe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["email"] != "jane@agency.gov.au" {
		t.Errorf("email: got %v, want jane@agency.gov.au", result["email"])
	}
	if result["name"] != "Jane" {
		t.Errorf("name: got %v, want Jane", result["name"])
	}
	roles, ok := result["roles"].([]string)
	if !ok || len(roles) != 1 || roles[0] != "lab_officer" {
		t.Errorf("roles: got %v, want [lab_officer]", result["roles"])
	}
}

func TestProfileService_GetMe_NoRoles(t *testing.T) {
	svc, _ := newTestProfileService(t)

	ctx := authn.ContextWithPrincipal(context.Background(), &authn.Principal{Kind: authn.KindUser, UserID: "user-no-roles", Email: "nobody@agency.gov.au", GivenName: "Nobody"})

	result, err := svc.GetMe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	roles, ok := result["roles"].([]string)
	if !ok {
		t.Fatalf("roles: expected []string, got %T", result["roles"])
	}
	if len(roles) != 0 {
		t.Errorf("roles: expected empty, got %v", roles)
	}
}
