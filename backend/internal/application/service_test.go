package application

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/generictemplate"
	"github.com/OpenNSW/core/artifact/loaders/local"
	"github.com/OpenNSW/nsw-agency/backend/internal/auth"
	"github.com/OpenNSW/nsw-agency/backend/internal/nswclient"
	"github.com/OpenNSW/nsw-agency/backend/internal/rbac"
	"github.com/OpenNSW/nsw-agency/backend/internal/taskconfig/taskconfigart"
	"github.com/OpenNSW/nsw-agency/backend/pkg/httpclient"
)

// writeTaskConfigFile writes content to <root>/task-configs/<name>.
func writeTaskConfigFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, "task-configs", name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

// writeFormFile writes content to <root>/forms/<name>.
func writeFormFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, "forms", name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

// ---------- service test harness ----------

// callbackCapture records the body of POSTs made to the test callback server.
type callbackCapture struct {
	mu    sync.Mutex
	calls [][]byte
	paths []string
}

func (c *callbackCapture) record(body []byte, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, body)
	c.paths = append(c.paths, path)
}

func (c *callbackCapture) lastCall() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.calls) == 0 {
		return nil
	}
	var got map[string]any
	_ = json.Unmarshal(c.calls[len(c.calls)-1], &got)
	return got
}

func (c *callbackCapture) lastPath() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.paths) == 0 {
		return ""
	}
	return c.paths[len(c.paths)-1]
}

// newCallbackServer returns an httptest server that responds 200 OK to any POST
// and captures the request body for assertions.
func newCallbackServer(t *testing.T) (*httptest.Server, *callbackCapture) {
	t.Helper()
	capture := &callbackCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capture.record(body, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, capture
}

// serviceHarness wires the in-memory dependencies required to exercise
// Service end-to-end against a stub callback server.
type serviceHarness struct {
	t           *testing.T
	store       *ApplicationStore
	httpClient  *httpclient.Client
	callbackURL string
	capture     *callbackCapture
	service     Service
}

// newTestRegistry builds an artifact registry backed by a local loader rooted at
// root, registering every JSON file under root/task-configs as a task_config
// artifact and every file under root/forms as a generic_template artifact. Ids
// are derived the same way the production loader does: the task config's
// taskCode (or filename) and the form's top-level "id" (or filename).
func newTestRegistry(t *testing.T, root string) *artifact.Registry {
	t.Helper()
	loader, err := local.New(local.Config{Root: root})
	if err != nil {
		t.Fatalf("failed to create local loader: %v", err)
	}
	reg := artifact.NewRegistry(loader)

	register := func(dir string, idFrom func(data []byte, name string) string, kind artifact.Kind) {
		walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".json") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			reg.RegisterArtifact(idFrom(data, d.Name()), kind, "", rel)
			return nil
		})
		if walkErr != nil {
			t.Fatalf("failed to register artifacts in %s: %v", dir, walkErr)
		}
	}

	register(filepath.Join(root, "task-configs"), func(data []byte, name string) string {
		var cfg struct {
			TaskCode string `json:"taskCode"`
		}
		_ = json.Unmarshal(data, &cfg)
		if cfg.TaskCode != "" {
			return cfg.TaskCode
		}
		return strings.TrimSuffix(name, ".json")
	}, taskconfigart.Kind)

	register(filepath.Join(root, "forms"), func(data []byte, name string) string {
		var doc struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(data, &doc)
		if doc.ID != "" {
			return doc.ID
		}
		return strings.TrimSuffix(name, ".json")
	}, generictemplate.Kind)

	return reg
}

// newServiceHarness constructs the harness with config and form files placed
// under writeFn before the stores are initialized.
//
// writeFn receives the config root path and is expected to populate
// <root>/task-configs/ and <root>/forms/ as needed.
func newServiceHarness(t *testing.T, writeFn func(root string)) *serviceHarness {
	t.Helper()

	root := t.TempDir()
	for _, sub := range []string{"task-configs", "forms"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("failed to create %s dir: %v", sub, err)
		}
	}
	if writeFn != nil {
		writeFn(root)
	}

	store := newTestStore(t)

	reg := newTestRegistry(t, root)

	srv, capture := newCallbackServer(t)
	hc := httpclient.NewClientBuilder().Build()

	roleService := rbac.NewRoleService(store.db)
	svc := NewService(store, reg, nswclient.NewWithClient(hc), roleService)
	t.Cleanup(func() { _ = svc.Close() })

	return &serviceHarness{
		t:           t,
		store:       store,
		httpClient:  hc,
		callbackURL: srv.URL,
		capture:     capture,
		service:     svc,
	}
}

// newAuthContext injects a minimal auth context carrying the given userID.
func newAuthContext(ctx context.Context, userID string) context.Context {
	return auth.WithAuthContext(ctx, &auth.AuthContext{
		User: &auth.UserContext{ID: userID},
	})
}

// seed inserts an application record with the harness's callback URL as ServiceURL.
func (h *serviceHarness) seed(taskID, taskCode string, data JSONB) {
	h.t.Helper()
	if data == nil {
		data = JSONB{"field": "value"}
	}
	err := h.store.CreateOrUpdate(&ApplicationRecord{
		TaskID:        taskID,
		TaskCode:      taskCode,
		ConsignmentID: "wf-test",
		ServiceURL:    h.callbackURL,
		Data:          data,
		Status:        "PENDING",
	})
	if err != nil {
		h.t.Fatalf("failed to seed record: %v", err)
	}
}

// statusOf reads the latest status of the record from the database.
func (h *serviceHarness) statusOf(taskID string) string {
	h.t.Helper()
	rec, err := h.store.GetByTaskID(taskID)
	if err != nil {
		h.t.Fatalf("failed to load record: %v", err)
	}
	return rec.Status
}

// ---------- ReviewApplication: status derivation ----------

func TestReviewApplication_StatusFromStatusMap(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "alpha.json", `{
			"meta": {"title": "Alpha"},
			"behavior": {
				"statusMap": {
					"approve": "APPROVED",
					"reject":  "REJECTED",
					"needs_more_info": "FEEDBACK_REQUESTED"
				}
			}
		}`)
	})
	h.seed("t-approve", "alpha", nil)
	h.seed("t-reject", "alpha", nil)
	h.seed("t-feedback", "alpha", nil)

	cases := []struct {
		taskID  string
		outcome string
		want    string
	}{
		{"t-approve", "approve", "APPROVED"},
		{"t-reject", "reject", "REJECTED"},
		{"t-feedback", "needs_more_info", "FEEDBACK_REQUESTED"},
	}
	for _, tc := range cases {
		t.Run(tc.outcome, func(t *testing.T) {
			err := h.service.ReviewApplication(context.Background(), tc.taskID, map[string]any{
				"review_outcome": tc.outcome,
			})
			if err != nil {
				t.Fatalf("ReviewApplication failed: %v", err)
			}
			if got := h.statusOf(tc.taskID); got != tc.want {
				t.Errorf("status: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReviewApplication_DefaultsToDONE_OutcomeNotInMap(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "alpha.json", `{
			"meta": {"title": "Alpha"},
			"behavior": {"statusMap": {"approve": "APPROVED"}}
		}`)
	})
	h.seed("t-unknown", "alpha", nil)

	err := h.service.ReviewApplication(context.Background(), "t-unknown", map[string]any{
		"review_outcome": "totally_made_up",
	})
	if err != nil {
		t.Fatalf("ReviewApplication failed: %v", err)
	}
	if got := h.statusOf("t-unknown"); got != "DONE" {
		t.Errorf("status: got %q, want DONE (unmapped outcome should fall through)", got)
	}
}

func TestReviewApplication_DefaultsToDONE_NoStatusMap(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		// Config exists but defines no behavior/statusMap.
		writeTaskConfigFile(t, root, "alpha.json", `{"meta": {"title": "Alpha"}}`)
	})
	h.seed("t-no-map", "alpha", nil)

	err := h.service.ReviewApplication(context.Background(), "t-no-map", map[string]any{
		"review_outcome": "approve",
	})
	if err != nil {
		t.Fatalf("ReviewApplication failed: %v", err)
	}
	if got := h.statusOf("t-no-map"); got != "DONE" {
		t.Errorf("status: got %q, want DONE", got)
	}
}

func TestReviewApplication_DefaultsToDONE_NoConfig(t *testing.T) {
	h := newServiceHarness(t, nil)
	h.seed("t-no-config", "no-such-task", nil)

	err := h.service.ReviewApplication(context.Background(), "t-no-config", map[string]any{
		"review_outcome": "approve",
	})
	if err != nil {
		t.Fatalf("ReviewApplication failed: %v", err)
	}
	if got := h.statusOf("t-no-config"); got != "DONE" {
		t.Errorf("status: got %q, want DONE", got)
	}
}

// ---------- ReviewApplication: outcomeField override ----------

func TestReviewApplication_OutcomeFieldOverride(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "labs.json", `{
			"meta": {"title": "Lab Results"},
			"behavior": {
				"outcomeField": "decision",
				"statusMap": {"pass": "APPROVED", "fail": "REJECTED"}
			}
		}`)
	})

	t.Run("custom field hit", func(t *testing.T) {
		h.seed("t-pass", "labs", nil)
		err := h.service.ReviewApplication(context.Background(), "t-pass", map[string]any{
			"decision": "pass",
		})
		if err != nil {
			t.Fatalf("ReviewApplication failed: %v", err)
		}
		if got := h.statusOf("t-pass"); got != "APPROVED" {
			t.Errorf("status: got %q, want APPROVED (decision=pass)", got)
		}
	})

	t.Run("default field ignored when override set", func(t *testing.T) {
		h.seed("t-defaultignored", "labs", nil)
		// review_outcome is the default name but the config asked for "decision",
		// so the default name should NOT be honored.
		err := h.service.ReviewApplication(context.Background(), "t-defaultignored", map[string]any{
			"review_outcome": "pass",
		})
		if err != nil {
			t.Fatalf("ReviewApplication failed: %v", err)
		}
		if got := h.statusOf("t-defaultignored"); got != "DONE" {
			t.Errorf("status: got %q, want DONE (review_outcome should be ignored when outcomeField=decision)", got)
		}
	})
}

// ---------- ReviewApplication: callback dispatch ----------

func TestReviewApplication_CallsServiceURL(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "alpha.json", `{
			"meta": {"title": "Alpha"},
			"behavior": {"statusMap": {"approve": "APPROVED"}}
		}`)
	})
	h.seed("t-callback", "alpha", nil)

	err := h.service.ReviewApplication(context.Background(), "t-callback", map[string]any{
		"review_outcome": "approve",
		"comment":        "lgtm",
	})
	if err != nil {
		t.Fatalf("ReviewApplication failed: %v", err)
	}

	lastPath := h.capture.lastPath()
	expectedPath := "/t-callback"
	if lastPath != expectedPath {
		t.Errorf("callback URL path: got %q, want %q", lastPath, expectedPath)
	}

	body := h.capture.lastCall()
	if body == nil {
		t.Fatalf("expected callback to be invoked, got no calls")
	}
	if body["command"] != "approve" {
		t.Errorf("callback command: got %v, want approve", body["command"])
	}
	payload, ok := body["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload object, got %T", body["payload"])
	}
	if payload["review_outcome"] != "approve" || payload["comment"] != "lgtm" {
		t.Errorf("callback payload forwarded incorrectly: got %v", payload)
	}
}

func TestReviewApplication_ServerReferenceNumberOverridesClient(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "permit.json", `{
			"meta": {"title": "Permit Review"},
			"behavior": {"outcomeField": "application_review_outcome"},
			"referenceNumber": {"agencyCode": "agency-a", "prefix": "034/", "minDigits": 5}
		}`)
	})
	h.seed("t-ref-callback", "permit", nil)

	err := h.service.ReviewApplication(context.Background(), "t-ref-callback", map[string]any{
		"application_review_outcome": "needs_more_info",
		"reference_number":           "manually-overridden",
	})
	if err != nil {
		t.Fatalf("ReviewApplication failed: %v", err)
	}

	body := h.capture.lastCall()
	if body["command"] != "needs_more_info" {
		t.Errorf("callback command: got %v, want needs_more_info", body["command"])
	}
	payload, ok := body["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload object, got %T", body["payload"])
	}
	if payload["reference_number"] != "034/00001" {
		t.Errorf("callback reference: got %v, want 034/00001 (server value must win)", payload["reference_number"])
	}
}

func TestReviewApplication_ServerReferenceNumberUsesConfiguredField(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "permit.json", `{
			"meta": {"title": "Permit Review"},
			"referenceNumber": {"field": "permit_no", "agencyCode": "agency-b", "prefix": "AB-", "minDigits": 3}
		}`)
	})
	h.seed("t-ref-field", "permit", nil)

	err := h.service.ReviewApplication(context.Background(), "t-ref-field", map[string]any{
		"review_outcome": "approve",
	})
	if err != nil {
		t.Fatalf("ReviewApplication failed: %v", err)
	}

	payload, ok := h.capture.lastCall()["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload object")
	}
	if payload["permit_no"] != "AB-001" {
		t.Errorf("callback reference: got %v, want AB-001", payload["permit_no"])
	}
	if _, exists := payload["reference_number"]; exists {
		t.Errorf("default field should not be populated when config names another field: %v", payload)
	}
}

func TestFeedbackApplication_CallsServiceURL(t *testing.T) {
	h := newServiceHarness(t, nil)
	h.seed("t-feedback-cb", "alpha", nil)

	err := h.service.FeedbackApplication(context.Background(), "t-feedback-cb", map[string]any{
		"feedback": "please correct container numbers",
	})
	if err != nil {
		t.Fatalf("FeedbackApplication failed: %v", err)
	}

	lastPath := h.capture.lastPath()
	expectedPath := "/t-feedback-cb"
	if lastPath != expectedPath {
		t.Errorf("callback URL path: got %q, want %q", lastPath, expectedPath)
	}

	body := h.capture.lastCall()
	if body == nil {
		t.Fatalf("expected callback to be invoked, got no calls")
	}
	if body["command"] != "request-amendment" {
		t.Errorf("callback command: got %v, want request-amendment", body["command"])
	}
	payload, ok := body["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload object, got %T", body["payload"])
	}
	if payload["feedback"] != "please correct container numbers" {
		t.Errorf("callback payload forwarded incorrectly: got %v", payload)
	}
}

// ---------- GetApplication: form resolution ----------

func TestGetApplication_ResolvesFormReferences(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "alpha.json", `{
			"meta": {"title": "Alpha", "category": "Test", "description": "Test task", "icon": "emoji:📋"},
			"forms": {"view": "alpha_view", "review": "alpha_review"}
		}`)
		writeFormFile(t, root, "alpha_view.json", `{"schema":{"type":"object","title":"View"},"uiSchema":{"type":"VerticalLayout"}}`)
		writeFormFile(t, root, "alpha_review.json", `{"schema":{"type":"object","title":"Review"},"uiSchema":{"type":"VerticalLayout"}}`)
	})
	h.seed("t-1", "alpha", JSONB{"submittedField": "submittedValue"})

	app, err := h.service.GetApplication(context.Background(), "t-1")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}

	if app.Title != "Alpha" {
		t.Errorf("Title: got %q, want %q", app.Title, "Alpha")
	}
	if app.Description != "Test task" {
		t.Errorf("Description: got %q, want %q", app.Description, "Test task")
	}
	if app.Icon != "emoji:📋" {
		t.Errorf("Icon: got %q, want %q", app.Icon, "emoji:📋")
	}
	if app.Category != "Test" {
		t.Errorf("Category: got %q, want %q", app.Category, "Test")
	}

	if app.DataForm == nil {
		t.Errorf("expected DataForm to be attached")
	} else {
		var view map[string]any
		if err := json.Unmarshal(app.DataForm, &view); err != nil {
			t.Errorf("DataForm not valid JSON: %v", err)
		}
		if schema, ok := view["schema"].(map[string]any); !ok || schema["title"] != "View" {
			t.Errorf("DataForm content unexpected: %v", view)
		}
	}
	if app.AgencyForm == nil {
		t.Errorf("expected AgencyForm to be attached")
	} else {
		var review map[string]any
		if err := json.Unmarshal(app.AgencyForm, &review); err != nil {
			t.Errorf("AgencyForm not valid JSON: %v", err)
		}
		if schema, ok := review["schema"].(map[string]any); !ok || schema["title"] != "Review" {
			t.Errorf("AgencyForm content unexpected: %v", review)
		}
	}
}

func TestGetApplication_MissingFormRef_OmitsForms(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "alpha.json", `{
			"meta": {"title": "Alpha"},
			"forms": {"view": "missing_view", "review": "missing_review"}
		}`)
	})
	h.seed("t-missing-forms", "alpha", nil)

	app, err := h.service.GetApplication(context.Background(), "t-missing-forms")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if app.Title != "Alpha" {
		t.Errorf("Title: got %q, want %q", app.Title, "Alpha")
	}
	if app.DataForm != nil || app.AgencyForm != nil {
		t.Errorf("expected forms to be omitted when referenced forms are missing, got dataForm=%v agencyForm=%v",
			app.DataForm, app.AgencyForm)
	}
}

// failingLoader is an artifact.Loader that returns a non-ErrNotFound I/O error
// for every path, simulating a transient remote-store failure.
type failingLoader struct{}

func (failingLoader) Load(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("simulated remote store failure")
}

func TestGetApplication_ConfigLoadError_FailsClosed(t *testing.T) {
	store := newTestStore(t)
	if err := store.CreateOrUpdate(&ApplicationRecord{
		TaskID:        "t-load-fail",
		TaskCode:      "alpha",
		ConsignmentID: "wf-test",
		ServiceURL:    "http://unused.example",
		Data:          JSONB{"field": "value"},
		Status:        "PENDING",
	}); err != nil {
		t.Fatalf("failed to seed record: %v", err)
	}

	// Config is registered but its bytes fail to load with a real I/O error
	// (not ErrNotFound). GetApplication must surface the error rather than fall
	// back to nil permissions, which would grant full access to any user.
	reg := artifact.NewRegistry(failingLoader{})
	reg.RegisterArtifact("alpha", taskconfigart.Kind, "", "alpha.json")

	hc := httpclient.NewClientBuilder().Build()
	svc := NewService(store, reg, nswclient.NewWithClient(hc), rbac.NewRoleService(store.db))
	t.Cleanup(func() { _ = svc.Close() })

	app, err := svc.GetApplication(context.Background(), "t-load-fail")
	if err == nil {
		t.Fatalf("expected an error when the task config fails to load, got app=%+v", app)
	}
	if app != nil {
		t.Errorf("expected no application on load error, got %+v", app)
	}
}

func TestGetApplication_NoConfig_OmitsMetadata(t *testing.T) {
	h := newServiceHarness(t, nil)
	h.seed("t-orphan", "no-config-for-this", nil)

	app, err := h.service.GetApplication(context.Background(), "t-orphan")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if app.Title != "" || app.Category != "" || app.Icon != "" || app.Description != "" {
		t.Errorf("expected empty metadata when no config found, got title=%q desc=%q icon=%q cat=%q",
			app.Title, app.Description, app.Icon, app.Category)
	}
	if app.DataForm != nil || app.AgencyForm != nil {
		t.Errorf("expected nil forms when no config found")
	}
}

func TestGetApplication_NotFound(t *testing.T) {
	h := newServiceHarness(t, nil)
	_, err := h.service.GetApplication(context.Background(), "does-not-exist")
	if err != ErrApplicationNotFound {
		t.Errorf("expected ErrApplicationNotFound, got %v", err)
	}
}

// ---------- GetApplication: reference numbers ----------

// referenceHarness serves a task whose config asks for reference numbers in the
// "034/%05d" shape, plus a second task code with no reference-number block.
func referenceHarness(t *testing.T) *serviceHarness {
	t.Helper()
	return newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "permit.json", `{
			"meta": {"title": "Permit Review"},
			"forms": {"review": "permit_review"},
			"referenceNumber": {"agencyCode": "agency-a", "prefix": "034/", "minDigits": 5}
		}`)
		writeTaskConfigFile(t, root, "plain.json", `{"meta": {"title": "Plain Review"}}`)
		writeFormFile(t, root, "permit_review.json", `{
			"schema": {"type": "object", "properties": {"reference_number": {"type": "string"}}},
			"uiSchema": {
				"type": "VerticalLayout",
				"elements": [{
					"type": "Control",
					"scope": "#/properties/reference_number",
					"options": {"readonly": true}
				}]
			}
		}`)
	})
}

func TestGetApplication_GeneratesConfiguredReferenceNumber(t *testing.T) {
	h := referenceHarness(t)
	h.seed("t-ref-1", "permit", nil)

	app, err := h.service.GetApplication(context.Background(), "t-ref-1")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if app.ReferenceNumber != "034/00001" {
		t.Errorf("reference number: got %q, want %q", app.ReferenceNumber, "034/00001")
	}
	if app.ReferenceNumberField != "reference_number" {
		t.Errorf("reference field: got %q, want %q", app.ReferenceNumberField, "reference_number")
	}

	// The value must also be persisted, so the list view and review submission
	// see the same reference.
	stored, err := h.store.GetByTaskID("t-ref-1")
	if err != nil {
		t.Fatalf("GetByTaskID failed: %v", err)
	}
	if stored.ReferenceNumber != "034/00001" {
		t.Errorf("persisted reference: got %q, want %q", stored.ReferenceNumber, "034/00001")
	}
}

func TestGetApplication_ReferenceNumberIsStableAcrossReads(t *testing.T) {
	h := referenceHarness(t)
	h.seed("t-ref-stable", "permit", nil)

	for i := 0; i < 3; i++ {
		app, err := h.service.GetApplication(context.Background(), "t-ref-stable")
		if err != nil {
			t.Fatalf("GetApplication #%d failed: %v", i+1, err)
		}
		if app.ReferenceNumber != "034/00001" {
			t.Fatalf("GetApplication #%d reference: got %q, want %q", i+1, app.ReferenceNumber, "034/00001")
		}
	}

	// Repeated reads must not have burned sequence values: the next
	// application is still number two.
	h.seed("t-ref-next", "permit", nil)
	app, err := h.service.GetApplication(context.Background(), "t-ref-next")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if app.ReferenceNumber != "034/00002" {
		t.Errorf("second application reference: got %q, want %q", app.ReferenceNumber, "034/00002")
	}
}

func TestGetApplication_ConcurrentReadsShareOneReferenceNumber(t *testing.T) {
	h := referenceHarness(t)
	h.seed("t-ref-race", "permit", nil)

	const readers = 10
	var wg sync.WaitGroup
	refs := make([]string, readers)
	errs := make([]error, readers)

	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func(i int) {
			defer wg.Done()
			app, err := h.service.GetApplication(context.Background(), "t-ref-race")
			if err != nil {
				errs[i] = err
				return
			}
			refs[i] = app.ReferenceNumber
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("reader %d failed: %v", i, err)
		}
		if refs[i] != refs[0] {
			t.Fatalf("readers disagreed on the reference number: %q vs %q", refs[i], refs[0])
		}
	}
	if refs[0] == "" {
		t.Fatal("expected a reference number to be generated")
	}
}

// TestGetApplication_ReferenceNumberGrowsPastMinDigits checks the sequence is
// widened rather than rejected or truncated once it outgrows minDigits.
func TestGetApplication_ReferenceNumberGrowsPastMinDigits(t *testing.T) {
	h := referenceHarness(t)
	h.seed("t-ref-big", "permit", nil)

	if err := h.store.DB().Exec(
		`INSERT INTO reference_sequences (agency_code, prefix, current_value, updated_at) VALUES (?, ?, ?, ?)`,
		"agency-a", "034/", 99999, time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("failed to seed sequence: %v", err)
	}

	app, err := h.service.GetApplication(context.Background(), "t-ref-big")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if app.ReferenceNumber != "034/100000" {
		t.Errorf("reference number: got %q, want %q", app.ReferenceNumber, "034/100000")
	}
}

func TestGetApplication_NoReferenceConfig_NoReferenceNumber(t *testing.T) {
	h := referenceHarness(t)
	h.seed("t-ref-none", "plain", nil)

	app, err := h.service.GetApplication(context.Background(), "t-ref-none")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if app.ReferenceNumber != "" || app.ReferenceNumberField != "" {
		t.Errorf("expected no reference number without config, got %q (field %q)",
			app.ReferenceNumber, app.ReferenceNumberField)
	}
}

// ---------- GetApplications: RBAC filtering ----------

func TestGetApplications_FiltersInaccessibleItems(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "restricted.json", `{
			"meta": {"title": "Restricted"},
			"permissions": [{"role": "manager", "actions": ["VIEW"]}]
		}`)
	})
	h.seed("t-restricted", "restricted", nil)

	// No auth context — user has no roles, task requires manager.
	result, err := h.service.GetApplications(context.Background(), "", "", "", 1, 20)
	if err != nil {
		t.Fatalf("GetApplications failed: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected inaccessible item to be filtered out, got %d items", len(result.Items))
	}
}

func TestGetApplications_IncludesAccessibleItems(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "open.json", `{
			"meta": {"title": "Open"}
		}`)
	})
	h.seed("t-open", "open", nil)

	// No permissions config — all users have access.
	result, err := h.service.GetApplications(context.Background(), "", "", "", 1, 20)
	if err != nil {
		t.Fatalf("GetApplications failed: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("expected 1 accessible item, got %d", len(result.Items))
	}
}

// ---------- GetApplication: AllowedActions ----------

func TestGetApplication_PopulatesAllowedActions(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "alpha.json", `{
			"meta": {"title": "Alpha"},
			"permissions": [{"role": "officer", "actions": ["VIEW", "REVIEW"]}]
		}`)
	})
	h.seed("t-actions", "alpha", nil)

	roleService := rbac.NewRoleService(h.store.db)
	role, err := roleService.Create("officer")
	if err != nil {
		t.Fatalf("failed to create role: %v", err)
	}

	const userID = "user-001"
	if err := roleService.Assign(userID, role.ID); err != nil {
		t.Fatalf("failed to assign role: %v", err)
	}

	ctx := newAuthContext(context.Background(), userID)

	app, err := h.service.GetApplication(ctx, "t-actions")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if len(app.AllowedActions) != 2 {
		t.Errorf("expected 2 allowed actions, got %v", app.AllowedActions)
	}
}

func TestGetApplication_NoConfig_EmptyAllowedActions(t *testing.T) {
	h := newServiceHarness(t, nil)
	h.seed("t-noconfig", "no-such-task", nil)

	app, err := h.service.GetApplication(context.Background(), "t-noconfig")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	// No config → falls back to full access.
	if len(app.AllowedActions) != 3 {
		t.Errorf("expected 3 default allowed actions, got %v", app.AllowedActions)
	}
}
