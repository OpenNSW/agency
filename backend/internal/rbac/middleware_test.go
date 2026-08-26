package rbac

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/OpenNSW/nsw-agency/backend/internal/authn"
	"github.com/OpenNSW/nsw-agency/backend/internal/taskconfig"
	"github.com/OpenNSW/nsw-agency/backend/internal/taskconfig/taskconfigart"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ---------- Unit tests: ResolveAccess ----------

func TestResolveAccess_NoRoles(t *testing.T) {
	permissions := []taskconfig.Permission{
		{Role: "lab_officer", Actions: []string{"VIEW", "REVIEW"}},
	}
	accessible, actions := ResolveAccess(nil, permissions)
	if accessible {
		t.Error("expected isAccessible false when user has no roles")
	}
	if len(actions) != 0 {
		t.Errorf("expected no actions, got %v", actions)
	}
}

func TestResolveAccess_MatchingRole(t *testing.T) {
	roles := []RoleRecord{{Name: "lab_officer"}}
	permissions := []taskconfig.Permission{
		{Role: "lab_officer", Actions: []string{"VIEW", "REVIEW"}},
	}
	accessible, actions := ResolveAccess(roles, permissions)
	if !accessible {
		t.Error("expected isAccessible true when role matches")
	}
	if len(actions) != 2 {
		t.Errorf("expected 2 actions, got %v", actions)
	}
}

func TestResolveAccess_MultipleRoles_Union(t *testing.T) {
	roles := []RoleRecord{
		{Name: "lab_officer"},
		{Name: "lab_manager"},
	}
	permissions := []taskconfig.Permission{
		{Role: "lab_officer", Actions: []string{"VIEW"}},
		{Role: "lab_manager", Actions: []string{"VIEW", "REVIEW"}},
	}
	// VIEW appears in both roles but should be deduplicated — expect 2 unique actions.
	_, actions := ResolveAccess(roles, permissions)
	if len(actions) != 2 {
		t.Errorf("expected 2 unique actions, got %v", actions)
	}
}

func TestResolveAccess_NoMatch(t *testing.T) {
	roles := []RoleRecord{{Name: "unrelated_role"}}
	permissions := []taskconfig.Permission{
		{Role: "lab_officer", Actions: []string{"VIEW"}},
	}
	accessible, actions := ResolveAccess(roles, permissions)
	if accessible {
		t.Error("expected isAccessible false when no role matches")
	}
	if len(actions) != 0 {
		t.Errorf("expected no actions, got %v", actions)
	}
}

func TestResolveAccess_EmptyPermissions(t *testing.T) {
	roles := []RoleRecord{{Name: "lab_officer"}}
	accessible, actions := ResolveAccess(roles, nil)
	if accessible {
		t.Error("expected isAccessible false for empty permissions")
	}
	if len(actions) != 0 {
		t.Errorf("expected no actions for empty permissions, got %v", actions)
	}
}

// ---------- Unit tests: hasAction ----------

func TestHasAction_Present(t *testing.T) {
	if !hasAction([]string{"VIEW", "REVIEW"}, "VIEW") {
		t.Error("expected hasAction to return true for VIEW")
	}
}

func TestHasAction_Absent(t *testing.T) {
	if hasAction([]string{"VIEW"}, "REVIEW") {
		t.Error("expected hasAction to return false for REVIEW")
	}
}

func TestHasAction_EmptySlice(t *testing.T) {
	if hasAction(nil, "VIEW") {
		t.Error("expected hasAction to return false for empty actions")
	}
}

// ---------- Helpers ----------

type mockTaskCodeResolver struct {
	taskCode string
	err      error
}

func (m *mockTaskCodeResolver) GetTaskCode(_ context.Context, _ string) (string, error) {
	return m.taskCode, m.err
}

// newTestRegistry builds an artifact registry backed by an in-memory loader,
// registering each task config under its map key as the artifact id.
func newTestRegistry(t *testing.T, configs map[string]taskconfig.TaskConfig) *artifact.Registry {
	t.Helper()
	mem := testutil.MemLoader{}
	reg := artifact.NewRegistry(mem)
	for id, cfg := range configs {
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatalf("failed to marshal task config %q: %v", id, err)
		}
		path := id + ".json"
		mem[path] = data
		reg.RegisterArtifact(id, taskconfigart.Kind, "", path)
	}
	return reg
}

func newMiddlewareTestDB(t *testing.T) *RoleService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&RoleRecord{}, &UserRoleRecord{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	return NewRoleService(db)
}

// ---------- Integration tests: RequireAction ----------

// TestRequireAction_ConfigMissingPermissions_FailsClosed covers a task config
// that exists but omits permissions. TaskConfig.Validate rejects this at
// parse time, so the registry load fails with a genuine (non-ErrNotFound)
// error, and the middleware must fail closed rather than fall back to
// allowing every authenticated user.
func TestRequireAction_ConfigMissingPermissions_FailsClosed(t *testing.T) {
	svc := newMiddlewareTestDB(t)
	m := NewMiddleware(svc,
		&mockTaskCodeResolver{taskCode: "fcau_lab_test_v1"},
		newTestRegistry(t, map[string]taskconfig.TaskConfig{
			"fcau_lab_test_v1": {SchemaVersion: taskconfig.CurrentSchemaVersion, TaskCode: "fcau_lab_test_v1", Permissions: nil},
		}),
	)

	called := false
	handler := m.RequireAction("VIEW")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("taskId", "task-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if called {
		t.Error("expected handler NOT to be called for a task config missing permissions")
	}
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 (invalid config fails closed), got %d", w.Code)
	}
}

func TestRequireAction_UserHasRole_Allows(t *testing.T) {
	svc := newMiddlewareTestDB(t)

	role, err := svc.Create("lab_officer")
	if err != nil {
		t.Fatalf("failed to create role: %v", err)
	}
	const testUserID = "user-001"
	if err := svc.Assign(testUserID, role.ID); err != nil {
		t.Fatalf("failed to assign role: %v", err)
	}

	m := NewMiddleware(svc,
		&mockTaskCodeResolver{taskCode: "fcau_lab_test_v1"},
		newTestRegistry(t, map[string]taskconfig.TaskConfig{
			"fcau_lab_test_v1": {
				SchemaVersion: taskconfig.CurrentSchemaVersion,
				TaskCode:      "fcau_lab_test_v1",
				Permissions:   []taskconfig.Permission{{Role: "lab_officer", Actions: []string{"VIEW", "REVIEW"}}},
			},
		}),
	)

	called := false
	handler := m.RequireAction("VIEW")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("taskId", "task-1")
	r = r.WithContext(authn.ContextWithPrincipal(r.Context(), &authn.Principal{Kind: authn.KindUser, UserID: testUserID}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Error("expected handler to be called when user has required role")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireAction_UserLacksRole_Forbidden(t *testing.T) {
	svc := newMiddlewareTestDB(t)

	m := NewMiddleware(svc,
		&mockTaskCodeResolver{taskCode: "fcau_lab_test_v1"},
		newTestRegistry(t, map[string]taskconfig.TaskConfig{
			"fcau_lab_test_v1": {
				SchemaVersion: taskconfig.CurrentSchemaVersion,
				TaskCode:      "fcau_lab_test_v1",
				Permissions:   []taskconfig.Permission{{Role: "lab_officer", Actions: []string{"VIEW"}}},
			},
		}),
	)

	handler := m.RequireAction("VIEW")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("taskId", "task-1")
	r = r.WithContext(authn.ContextWithPrincipal(r.Context(), &authn.Principal{Kind: authn.KindUser, UserID: "user-no-roles"}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRequireAction_NoAuthContext_Unauthorized(t *testing.T) {
	svc := newMiddlewareTestDB(t)

	m := NewMiddleware(svc,
		&mockTaskCodeResolver{taskCode: "fcau_lab_test_v1"},
		newTestRegistry(t, map[string]taskconfig.TaskConfig{
			"fcau_lab_test_v1": {
				SchemaVersion: taskconfig.CurrentSchemaVersion,
				TaskCode:      "fcau_lab_test_v1",
				Permissions:   []taskconfig.Permission{{Role: "lab_officer", Actions: []string{"VIEW"}}},
			},
		}),
	)

	handler := m.RequireAction("VIEW")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("taskId", "task-1")
	// No auth context injected.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAction_ResolverError_InternalServerError(t *testing.T) {
	svc := newMiddlewareTestDB(t)

	m := NewMiddleware(svc,
		&mockTaskCodeResolver{err: fmt.Errorf("db unavailable")},
		newTestRegistry(t, nil),
	)

	handler := m.RequireAction("VIEW")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("taskId", "task-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// failingLoader is an artifact.Loader that returns a non-ErrNotFound I/O error
// for every path, simulating a transient remote-store failure (network, rate
// limit, expired credentials).
type failingLoader struct{}

func (failingLoader) Load(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("simulated remote store failure")
}

func TestRequireAction_ConfigNotFound_DeniesAccess(t *testing.T) {
	svc := newMiddlewareTestDB(t)

	// Empty registry: the resolved task code is not registered, so the loader
	// reports ErrNotFound. A genuinely-absent config has no permissions to
	// check, so access is denied by default rather than opened to everyone.
	m := NewMiddleware(svc,
		&mockTaskCodeResolver{taskCode: "fcau_lab_test_v1"},
		newTestRegistry(t, nil),
	)

	called := false
	handler := m.RequireAction("VIEW")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("taskId", "task-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if called {
		t.Error("expected handler NOT to be called when no task config exists")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRequireAction_ConfigLoadError_FailsClosed(t *testing.T) {
	svc := newMiddlewareTestDB(t)

	// The task code is registered, but fetching its bytes fails with a real I/O
	// error (not ErrNotFound). This must fail closed — allowing the request
	// through would silently bypass RBAC on a transient loader failure.
	reg := artifact.NewRegistry(failingLoader{})
	reg.RegisterArtifact("fcau_lab_test_v1", taskconfigart.Kind, "", "fcau_lab_test_v1.json")

	m := NewMiddleware(svc,
		&mockTaskCodeResolver{taskCode: "fcau_lab_test_v1"},
		reg,
	)

	called := false
	handler := m.RequireAction("VIEW")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("taskId", "task-1")
	r = r.WithContext(authn.ContextWithPrincipal(r.Context(), &authn.Principal{Kind: authn.KindUser, UserID: "user-001"}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if called {
		t.Error("expected handler NOT to be called when the task config fails to load")
	}
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 (fail closed), got %d", w.Code)
	}
}
