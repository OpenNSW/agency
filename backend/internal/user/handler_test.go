package user

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/agency/backend/internal/authn"
	"github.com/OpenNSW/agency/backend/internal/rbac"
)

func newTestProfileHandler(t *testing.T) (*ProfileHandler, *rbac.RoleService) {
	t.Helper()
	svc, roleService := newTestProfileService(t)
	return NewProfileHandler(svc), roleService
}

func TestHandleMe_NoAuthContext_Unauthorized(t *testing.T) {
	h, _ := newTestProfileHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	w := httptest.NewRecorder()
	h.HandleMe(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleMe_WithAuthContext_ReturnsProfile(t *testing.T) {
	h, roleService := newTestProfileHandler(t)

	const userID = "user-001"
	role, err := roleService.Create("lab_officer")
	if err != nil {
		t.Fatalf("failed to create role: %v", err)
	}
	if err := roleService.Assign(userID, role.ID); err != nil {
		t.Fatalf("failed to assign role: %v", err)
	}

	ctx := authn.ContextWithPrincipal(
		httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil).Context(), &authn.Principal{Kind: authn.KindUser, UserID: userID, Email: "jane@agency.gov.au", GivenName: "Jane"})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.HandleMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["email"] != "jane@agency.gov.au" {
		t.Errorf("email: got %v, want jane@agency.gov.au", body["email"])
	}
	if body["name"] != "Jane" {
		t.Errorf("name: got %v, want Jane", body["name"])
	}
	roles, ok := body["roles"].([]any)
	if !ok || len(roles) != 1 || roles[0] != "lab_officer" {
		t.Errorf("roles: got %v, want [lab_officer]", body["roles"])
	}
}

func TestHandleMe_WrongMethod_MethodNotAllowed(t *testing.T) {
	h, _ := newTestProfileHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/me", nil)
	w := httptest.NewRecorder()
	h.HandleMe(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}
