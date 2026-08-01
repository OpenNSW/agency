package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenNSW/nsw-agency/backend/internal/nswclient"
	"github.com/OpenNSW/nsw-agency/backend/internal/rbac"
	"github.com/OpenNSW/nsw-agency/backend/pkg/httpclient"
)

// --- validateServiceURLOrigin (unit) ---

func TestValidateServiceURLOrigin(t *testing.T) {
	tests := []struct {
		name       string
		serviceURL string
		baseURL    string
		wantErr    bool
	}{
		{
			name:       "no base URL configured is rejected",
			serviceURL: "https://anywhere.example/callback",
			baseURL:    "",
			wantErr:    true,
		},
		{
			name:       "matching scheme and host",
			serviceURL: "https://nsw.example/api/v1/tasks",
			baseURL:    "https://nsw.example/api/v1",
			wantErr:    false,
		},
		{
			name:       "matching host and port is case-insensitive on scheme/host",
			serviceURL: "HTTPS://NSW.EXAMPLE:8443/tasks",
			baseURL:    "https://nsw.example:8443/api/v1",
			wantErr:    false,
		},
		{
			name:       "different host is rejected",
			serviceURL: "https://attacker.example/callback",
			baseURL:    "https://nsw.example/api/v1",
			wantErr:    true,
		},
		{
			name:       "different scheme is rejected",
			serviceURL: "http://nsw.example/tasks",
			baseURL:    "https://nsw.example/api/v1",
			wantErr:    true,
		},
		{
			name:       "different port is rejected",
			serviceURL: "https://nsw.example:9000/tasks",
			baseURL:    "https://nsw.example/api/v1",
			wantErr:    true,
		},
		{
			name:       "relative URL (no scheme/host) is rejected",
			serviceURL: "/tasks",
			baseURL:    "https://nsw.example/api/v1",
			wantErr:    true,
		},
		{
			name:       "cloud metadata host is rejected",
			serviceURL: "http://169.254.169.254/latest/meta-data",
			baseURL:    "https://nsw.example/api/v1",
			wantErr:    true,
		},
		{
			name:       "subdomain prefix spoofing is rejected",
			serviceURL: "https://evil.nsw.example/callback",
			baseURL:    "https://nsw.example/api/v1",
			wantErr:    true,
		},
		{
			name:       "domain suffix spoofing is rejected",
			serviceURL: "https://nsw.example.evil.com/callback",
			baseURL:    "https://nsw.example/api/v1",
			wantErr:    true,
		},
		{
			name:       "hostname-prefixed lookalike domain is rejected",
			serviceURL: "https://nsw.example-evil.com/callback",
			baseURL:    "https://nsw.example/api/v1",
			wantErr:    true,
		},
		{
			name:       "trailing slash on configured NSW_API_BASE_URL does not affect the origin match",
			serviceURL: "https://nsw.example/tasks",
			baseURL:    "https://nsw.example/",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServiceURLOrigin(tt.serviceURL, tt.baseURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateServiceURLOrigin(%q, %q) error = %v, wantErr %v", tt.serviceURL, tt.baseURL, err, tt.wantErr)
			}
		})
	}
}

// --- CreateApplication (integration through the real Service) ---

// newServiceWithBaseURL builds a Service backed by an nswclient.Client
// configured with baseURL, isolated from the shared harness so the ServiceURL
// origin check can be exercised with a real (non-empty) base URL. It also
// returns the underlying store so tests can inspect or force record state
// without routing through Service methods that make real outbound calls.
func newServiceWithBaseURL(t *testing.T, baseURL string) (Service, *ApplicationStore) {
	t.Helper()
	store := newTestStore(t)
	root := t.TempDir()
	for _, sub := range []string{"task-configs", "forms"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("failed to create %s dir: %v", sub, err)
		}
	}
	reg := newTestRegistry(t, root)
	hc := httpclient.NewClientBuilder().WithBaseURL(baseURL).Build()
	svc := NewService(store, reg, nswclient.NewWithClient(hc), rbac.NewRoleService(store.db))
	t.Cleanup(func() { _ = svc.Close() })
	return svc, store
}

func TestCreateApplication_RejectsMismatchedServiceURLOrigin(t *testing.T) {
	svc, _ := newServiceWithBaseURL(t, "https://nsw.example/api/v1")

	err := svc.CreateApplication(context.Background(), &InjectRequest{
		TaskID:        "t-ssrf",
		TaskCode:      "alpha",
		ConsignmentID: "wf-test",
		ServiceURL:    "http://169.254.169.254/latest/meta-data",
		Data:          map[string]any{"field": "value"},
	})
	if err == nil {
		t.Fatal("expected an error for a ServiceURL that does not match the configured NSW base URL, got nil")
	}
	if !errors.Is(err, ErrInvalidServiceURL) {
		t.Errorf("expected ErrInvalidServiceURL, got %v", err)
	}

	if _, err := svc.GetApplication(context.Background(), "t-ssrf"); !errors.Is(err, ErrApplicationNotFound) {
		t.Errorf("expected the rejected application to not be persisted, got err=%v", err)
	}
}

func TestCreateApplication_AcceptsMatchingServiceURLOrigin(t *testing.T) {
	svc, _ := newServiceWithBaseURL(t, "https://nsw.example/api/v1")

	err := svc.CreateApplication(context.Background(), &InjectRequest{
		TaskID:        "t-ok",
		TaskCode:      "alpha",
		ConsignmentID: "wf-test",
		ServiceURL:    "https://nsw.example/api/v1/tasks",
		Data:          map[string]any{"field": "value"},
	})
	if err != nil {
		t.Fatalf("expected no error for a ServiceURL matching the configured NSW base URL, got %v", err)
	}

	app, err := svc.GetApplication(context.Background(), "t-ok")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if app.ServiceURL != "https://nsw.example/api/v1/tasks" {
		t.Errorf("ServiceURL: got %q", app.ServiceURL)
	}
}

func TestCreateApplication_MissingRequiredFields(t *testing.T) {
	svc, _ := newServiceWithBaseURL(t, "")

	tests := []struct {
		name string
		req  InjectRequest
	}{
		{name: "missing taskId", req: InjectRequest{TaskCode: "alpha", ConsignmentID: "c", ServiceURL: "https://x.example"}},
		{name: "missing taskCode", req: InjectRequest{TaskID: "t", ConsignmentID: "c", ServiceURL: "https://x.example"}},
		{name: "missing consignmentId", req: InjectRequest{TaskID: "t", TaskCode: "alpha", ServiceURL: "https://x.example"}},
		{name: "missing serviceUrl", req: InjectRequest{TaskID: "t", TaskCode: "alpha", ConsignmentID: "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := svc.CreateApplication(context.Background(), &tt.req); err == nil {
				t.Error("expected an error for missing required field, got nil")
			}
		})
	}
}

// --- CreateApplication (resubmission after feedback) ---

func TestCreateApplication_Resubmission_UpdatesDataAndServiceURL(t *testing.T) {
	svc, store := newServiceWithBaseURL(t, "https://nsw.example/api/v1")

	if err := svc.CreateApplication(context.Background(), &InjectRequest{
		TaskID:        "t-resubmit",
		TaskCode:      "alpha",
		ConsignmentID: "wf-test",
		ServiceURL:    "https://nsw.example/api/v1/tasks",
		Data:          map[string]any{"field": "original"},
	}); err != nil {
		t.Fatalf("initial CreateApplication failed: %v", err)
	}

	// Drive the record into FEEDBACK_REQUESTED via the store directly: the
	// officer-facing path (FeedbackApplication) posts a real outbound
	// callback to app.ServiceURL, which this test deliberately points at a
	// non-resolving host to isolate the origin-validation behavior under test.
	if err := store.UpdateStatus("t-resubmit", "FEEDBACK_REQUESTED", nil); err != nil {
		t.Fatalf("failed to force FEEDBACK_REQUESTED: %v", err)
	}

	newServiceURL := "https://nsw.example/api/v1/tasks/resubmitted"
	if err := svc.CreateApplication(context.Background(), &InjectRequest{
		TaskID:        "t-resubmit",
		TaskCode:      "alpha",
		ConsignmentID: "wf-test",
		ServiceURL:    newServiceURL,
		Data:          map[string]any{"field": "resubmitted"},
	}); err != nil {
		t.Fatalf("resubmission CreateApplication failed: %v", err)
	}

	app, err := svc.GetApplication(context.Background(), "t-resubmit")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if app.Status != "PENDING" {
		t.Errorf("Status: got %q, want PENDING (resubmission should reset status)", app.Status)
	}
	if app.ServiceURL != newServiceURL {
		t.Errorf("ServiceURL: got %q, want %q", app.ServiceURL, newServiceURL)
	}
	if app.Data["field"] != "resubmitted" {
		t.Errorf("Data: got %v, want field=resubmitted", app.Data)
	}
}

func TestCreateApplication_Resubmission_RejectsMismatchedServiceURLOrigin(t *testing.T) {
	svc, store := newServiceWithBaseURL(t, "https://nsw.example/api/v1")

	if err := svc.CreateApplication(context.Background(), &InjectRequest{
		TaskID:        "t-resubmit-invalid",
		TaskCode:      "alpha",
		ConsignmentID: "wf-test",
		ServiceURL:    "https://nsw.example/api/v1/tasks",
		Data:          map[string]any{"field": "original"},
	}); err != nil {
		t.Fatalf("initial CreateApplication failed: %v", err)
	}
	if err := store.UpdateStatus("t-resubmit-invalid", "FEEDBACK_REQUESTED", nil); err != nil {
		t.Fatalf("failed to force FEEDBACK_REQUESTED: %v", err)
	}

	err := svc.CreateApplication(context.Background(), &InjectRequest{
		TaskID:        "t-resubmit-invalid",
		TaskCode:      "alpha",
		ConsignmentID: "wf-test",
		ServiceURL:    "http://169.254.169.254/latest/meta-data",
		Data:          map[string]any{"field": "resubmitted"},
	})
	if !errors.Is(err, ErrInvalidServiceURL) {
		t.Fatalf("expected ErrInvalidServiceURL, got %v", err)
	}

	app, err := svc.GetApplication(context.Background(), "t-resubmit-invalid")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if app.Status != "FEEDBACK_REQUESTED" {
		t.Errorf("Status should be unchanged after a rejected resubmission: got %q, want FEEDBACK_REQUESTED", app.Status)
	}
	if app.ServiceURL != "https://nsw.example/api/v1/tasks" {
		t.Errorf("ServiceURL should be unchanged after a rejected resubmission: got %q", app.ServiceURL)
	}
	if app.Data["field"] != "original" {
		t.Errorf("Data should be unchanged after a rejected resubmission: got %v", app.Data)
	}
}
