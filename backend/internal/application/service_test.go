package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/generictemplate"
	"github.com/OpenNSW/core/artifact/loaders/local"
	"github.com/OpenNSW/nsw-agency/backend/internal/authn"
	"github.com/OpenNSW/nsw-agency/backend/internal/nswclient"
	"github.com/OpenNSW/nsw-agency/backend/internal/rbac"
	"github.com/OpenNSW/nsw-agency/backend/internal/taskconfig/taskconfigart"
	"github.com/OpenNSW/nsw-agency/backend/internal/user"
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
	return authn.ContextWithPrincipal(ctx, &authn.Principal{Kind: authn.KindUser, UserID: userID})
}

// claimAs claims taskID on behalf of userID and returns an auth context
// carrying that principal, ready to pass to ReviewApplication (which
// requires the caller to currently hold the claim). Seeds a users row for
// userID if one doesn't already exist, since claimant name/email are now
// looked up live rather than stored on the claim.
func (h *serviceHarness) claimAs(taskID, userID string) context.Context {
	h.t.Helper()
	if err := h.store.db.FirstOrCreate(&user.UserRecord{UserID: userID, Name: "Test Officer", Email: "officer@example.com"}, "user_id = ?", userID).Error; err != nil {
		h.t.Fatalf("failed to seed user %s: %v", userID, err)
	}
	if err := h.store.ClaimApplication(taskID, userID); err != nil {
		h.t.Fatalf("failed to claim %s for %s: %v", taskID, userID, err)
	}
	return newAuthContext(context.Background(), userID)
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
			"permissions": [{"role": "officer", "actions": ["VIEW", "REVIEW"]}],
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
			ctx := h.claimAs(tc.taskID, "officer-1")
			err := h.service.ReviewApplication(ctx, tc.taskID, map[string]any{
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
			"permissions": [{"role": "officer", "actions": ["VIEW", "REVIEW"]}],
			"behavior": {"statusMap": {"approve": "APPROVED"}}
		}`)
	})
	h.seed("t-unknown", "alpha", nil)

	ctx := h.claimAs("t-unknown", "officer-1")
	err := h.service.ReviewApplication(ctx, "t-unknown", map[string]any{
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
		writeTaskConfigFile(t, root, "alpha.json", `{
			"meta": {"title": "Alpha"},
			"permissions": [{"role": "officer", "actions": ["VIEW", "REVIEW"]}]
		}`)
	})
	h.seed("t-no-map", "alpha", nil)

	ctx := h.claimAs("t-no-map", "officer-1")
	err := h.service.ReviewApplication(ctx, "t-no-map", map[string]any{
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

	ctx := h.claimAs("t-no-config", "officer-1")
	err := h.service.ReviewApplication(ctx, "t-no-config", map[string]any{
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
			"permissions": [{"role": "officer", "actions": ["VIEW", "REVIEW"]}],
			"behavior": {
				"outcomeField": "decision",
				"statusMap": {"pass": "APPROVED", "fail": "REJECTED"}
			}
		}`)
	})

	t.Run("custom field hit", func(t *testing.T) {
		h.seed("t-pass", "labs", nil)
		ctx := h.claimAs("t-pass", "officer-1")
		err := h.service.ReviewApplication(ctx, "t-pass", map[string]any{
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
		ctx := h.claimAs("t-defaultignored", "officer-1")
		err := h.service.ReviewApplication(ctx, "t-defaultignored", map[string]any{
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
			"permissions": [{"role": "officer", "actions": ["VIEW", "REVIEW"]}],
			"behavior": {"statusMap": {"approve": "APPROVED"}}
		}`)
	})
	h.seed("t-callback", "alpha", nil)

	ctx := h.claimAs("t-callback", "officer-1")
	err := h.service.ReviewApplication(ctx, "t-callback", map[string]any{
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
			"permissions": [{"role": "officer", "actions": ["VIEW", "REVIEW"]}],
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

func TestGetApplication_CertificateTemplateID(t *testing.T) {
	t.Run("populated when the task config declares a certificate", func(t *testing.T) {
		h := newServiceHarness(t, func(root string) {
			writeTaskConfigFile(t, root, "alpha.json", `{
				"meta": {"title": "Alpha"},
				"permissions": [{"role": "officer", "actions": ["VIEW", "REVIEW"]}],
				"certificate": {
					"templateId": "fcau-issue-certificate--certificate-template",
					"dataSchema": {
						"type": "object",
						"properties": {"certificate_id": {"type": "string", "minLength": 1}},
						"required": ["certificate_id"]
					}
				}
			}`)
		})
		h.seed("t-cert", "alpha", nil)

		app, err := h.service.GetApplication(context.Background(), "t-cert")
		if err != nil {
			t.Fatalf("GetApplication failed: %v", err)
		}
		if app.CertificateTemplateID != "fcau-issue-certificate--certificate-template" {
			t.Errorf("CertificateTemplateID: got %q, want %q", app.CertificateTemplateID, "fcau-issue-certificate--certificate-template")
		}
		var schema map[string]any
		if err := json.Unmarshal(app.CertificateDataSchema, &schema); err != nil {
			t.Fatalf("CertificateDataSchema is not valid JSON: %v", err)
		}
		required, _ := schema["required"].([]any)
		if len(required) != 1 || required[0] != "certificate_id" {
			t.Errorf("CertificateDataSchema.required: got %v, want [certificate_id]", schema["required"])
		}
	})

	t.Run("empty when the task config has no certificate", func(t *testing.T) {
		h := newServiceHarness(t, func(root string) {
			writeTaskConfigFile(t, root, "alpha.json", `{
				"meta": {"title": "Alpha"},
				"permissions": [{"role": "officer", "actions": ["VIEW", "REVIEW"]}]
			}`)
		})
		h.seed("t-no-cert", "alpha", nil)

		app, err := h.service.GetApplication(context.Background(), "t-no-cert")
		if err != nil {
			t.Fatalf("GetApplication failed: %v", err)
		}
		if app.CertificateTemplateID != "" {
			t.Errorf("expected empty CertificateTemplateID, got %q", app.CertificateTemplateID)
		}
	})
}

func TestGetApplication_MissingFormRef_OmitsForms(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "alpha.json", `{
			"meta": {"title": "Alpha"},
			"permissions": [{"role": "officer", "actions": ["VIEW", "REVIEW"]}],
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

// ---------- GetApplicationByTaskCode ----------

func TestGetApplicationByTaskCode(t *testing.T) {
	h := newServiceHarness(t, func(root string) {
		writeTaskConfigFile(t, root, "alpha.json", `{
			"meta": {"title": "Alpha"},
			"permissions": [{"role": "officer", "actions": ["VIEW", "REVIEW"]}]
		}`)
	})
	h.seed("t-by-code", "alpha", JSONB{"exporter_name": "ACME"})

	app, err := h.service.GetApplicationByTaskCode(context.Background(), "wf-test", "alpha")
	if err != nil {
		t.Fatalf("GetApplicationByTaskCode failed: %v", err)
	}
	if app.TaskID != "t-by-code" {
		t.Errorf("TaskID: got %q, want %q", app.TaskID, "t-by-code")
	}
	if app.Title != "Alpha" {
		t.Errorf("Title: got %q, want %q", app.Title, "Alpha")
	}
	if app.Data["exporter_name"] != "ACME" {
		t.Errorf("Data[exporter_name]: got %v, want ACME", app.Data["exporter_name"])
	}
}

func TestGetApplicationByTaskCode_NotFound(t *testing.T) {
	h := newServiceHarness(t, nil)
	_, err := h.service.GetApplicationByTaskCode(context.Background(), "wf-test", "no-such-code")
	if err != ErrApplicationNotFound {
		t.Errorf("expected ErrApplicationNotFound, got %v", err)
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
			"meta": {"title": "Open"},
			"permissions": [{"role": "officer", "actions": ["VIEW"]}]
		}`)
	})
	h.seed("t-open", "open", nil)

	roleService := rbac.NewRoleService(h.store.db)
	role, err := roleService.Create("officer")
	if err != nil {
		t.Fatalf("failed to create role: %v", err)
	}
	const userID = "user-with-access"
	if err := roleService.Assign(userID, role.ID); err != nil {
		t.Fatalf("failed to assign role: %v", err)
	}

	// User holds the role granted VIEW on this task's permissions.
	result, err := h.service.GetApplications(newAuthContext(context.Background(), userID), "", "", "", 1, 20)
	if err != nil {
		t.Fatalf("GetApplications failed: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("expected 1 accessible item, got %d", len(result.Items))
	}
}

func TestGetApplications_ConfigLoadError_FailsClosed(t *testing.T) {
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
	// (not ErrNotFound). GetApplications must surface the error rather than
	// silently filter the item out as if the config were absent.
	reg := artifact.NewRegistry(failingLoader{})
	reg.RegisterArtifact("alpha", taskconfigart.Kind, "", "alpha.json")

	hc := httpclient.NewClientBuilder().Build()
	svc := NewService(store, reg, nswclient.NewWithClient(hc), rbac.NewRoleService(store.db))
	t.Cleanup(func() { _ = svc.Close() })

	result, err := svc.GetApplications(context.Background(), "", "", "", 1, 20)
	if err == nil {
		t.Fatalf("expected an error when the task config fails to load, got result=%+v", result)
	}
	if result != nil {
		t.Errorf("expected no result on load error, got %+v", result)
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
	// No config → no permissions to check → denied by default.
	if len(app.AllowedActions) != 0 {
		t.Errorf("expected no allowed actions, got %v", app.AllowedActions)
	}
}

// ---------- ClaimApplication / ReleaseApplication ----------

func TestClaimApplication_Success(t *testing.T) {
	h := newServiceHarness(t, nil)
	h.seed("t-claim", "no-such-task", nil)
	seedUser(t, h.store, "officer-1", "Officer One", "officer@example.com")

	ctx := newAuthContext(context.Background(), "officer-1")
	if err := h.service.ClaimApplication(ctx, "t-claim"); err != nil {
		t.Fatalf("ClaimApplication failed: %v", err)
	}

	app, err := h.service.GetApplication(context.Background(), "t-claim")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if app.ClaimedByEmail == nil || *app.ClaimedByEmail != "officer@example.com" {
		t.Errorf("expected ClaimedByEmail looked up from users table, got %v", app.ClaimedByEmail)
	}
}

func TestClaimApplication_ConflictWithOtherOfficer(t *testing.T) {
	h := newServiceHarness(t, nil)
	h.seed("t-claim-conflict", "no-such-task", nil)

	ctx1 := newAuthContext(context.Background(), "officer-1")
	if err := h.service.ClaimApplication(ctx1, "t-claim-conflict"); err != nil {
		t.Fatalf("first ClaimApplication failed: %v", err)
	}

	ctx2 := newAuthContext(context.Background(), "officer-2")
	err := h.service.ClaimApplication(ctx2, "t-claim-conflict")
	if err != ErrApplicationAlreadyClaimed {
		t.Errorf("expected ErrApplicationAlreadyClaimed, got %v", err)
	}
}

func TestClaimApplication_NotFound(t *testing.T) {
	h := newServiceHarness(t, nil)
	ctx := newAuthContext(context.Background(), "officer-1")
	err := h.service.ClaimApplication(ctx, "does-not-exist")
	if err != ErrApplicationNotFound {
		t.Errorf("expected ErrApplicationNotFound, got %v", err)
	}
}

func TestReleaseApplication_Success(t *testing.T) {
	h := newServiceHarness(t, nil)
	h.seed("t-release", "no-such-task", nil)

	ctx := newAuthContext(context.Background(), "officer-1")
	if err := h.service.ClaimApplication(ctx, "t-release"); err != nil {
		t.Fatalf("ClaimApplication failed: %v", err)
	}
	if err := h.service.ReleaseApplication(ctx, "t-release"); err != nil {
		t.Fatalf("ReleaseApplication failed: %v", err)
	}

	app, err := h.service.GetApplication(context.Background(), "t-release")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if app.ClaimedByEmail != nil {
		t.Errorf("expected claim cleared, got %v", app.ClaimedByEmail)
	}
}

func TestReleaseApplication_NotClaimedByCaller(t *testing.T) {
	h := newServiceHarness(t, nil)
	h.seed("t-release-other", "no-such-task", nil)

	ctx1 := newAuthContext(context.Background(), "officer-1")
	if err := h.service.ClaimApplication(ctx1, "t-release-other"); err != nil {
		t.Fatalf("ClaimApplication failed: %v", err)
	}

	ctx2 := newAuthContext(context.Background(), "officer-2")
	err := h.service.ReleaseApplication(ctx2, "t-release-other")
	if err != ErrApplicationNotClaimedByYou {
		t.Errorf("expected ErrApplicationNotClaimedByYou, got %v", err)
	}
}

// ---------- ReviewApplication: claim enforcement ----------

func TestReviewApplication_RejectsWhenUnclaimed(t *testing.T) {
	h := newServiceHarness(t, nil)
	h.seed("t-review-unclaimed", "no-such-task", nil)

	ctx := newAuthContext(context.Background(), "officer-1")
	err := h.service.ReviewApplication(ctx, "t-review-unclaimed", map[string]any{"review_outcome": "approve"})
	if err != ErrApplicationNotClaimedByYou {
		t.Errorf("expected ErrApplicationNotClaimedByYou, got %v", err)
	}
}

func TestReviewApplication_RejectsWhenClaimedByAnotherOfficer(t *testing.T) {
	h := newServiceHarness(t, nil)
	h.seed("t-review-other-claim", "no-such-task", nil)

	if err := h.store.ClaimApplication("t-review-other-claim", "officer-1"); err != nil {
		t.Fatalf("ClaimApplication failed: %v", err)
	}

	ctx := newAuthContext(context.Background(), "officer-2")
	err := h.service.ReviewApplication(ctx, "t-review-other-claim", map[string]any{"review_outcome": "approve"})
	if err != ErrApplicationNotClaimedByYou {
		t.Errorf("expected ErrApplicationNotClaimedByYou, got %v", err)
	}
}

// TestReviewApplication_ConflictOnDoubleSubmit simulates a claimant
// submitting a review twice (e.g. a double-click, or two concurrent
// requests that both pass the initial ownership check). The claim is left
// in place after a review, so without an atomic finalize the second call
// would silently record a conflicting outcome; it must instead be rejected.
func TestReviewApplication_ConflictOnDoubleSubmit(t *testing.T) {
	h := newServiceHarness(t, nil)
	h.seed("t-review-double", "no-such-task", nil)

	ctx := h.claimAs("t-review-double", "officer-1")

	if err := h.service.ReviewApplication(ctx, "t-review-double", map[string]any{"review_outcome": "approve"}); err != nil {
		t.Fatalf("first ReviewApplication failed: %v", err)
	}

	err := h.service.ReviewApplication(ctx, "t-review-double", map[string]any{"review_outcome": "reject"})
	if !errors.Is(err, ErrApplicationReviewConflict) {
		t.Errorf("expected ErrApplicationReviewConflict on double submit, got %v", err)
	}

	// The first outcome must stick; the second call must not have overwritten it.
	if h.statusOf("t-review-double") != "DONE" {
		t.Errorf("expected status to remain 'DONE' from the first review, got %q", h.statusOf("t-review-double"))
	}
}

func TestReleaseApplication_RejectedOnceReviewed(t *testing.T) {
	h := newServiceHarness(t, nil)
	h.seed("t-release-reviewed", "no-such-task", nil)

	ctx := h.claimAs("t-release-reviewed", "officer-1")
	if err := h.service.ReviewApplication(ctx, "t-release-reviewed", map[string]any{
		"review_outcome": "approve",
	}); err != nil {
		t.Fatalf("ReviewApplication failed: %v", err)
	}

	err := h.service.ReleaseApplication(ctx, "t-release-reviewed")
	if err != ErrApplicationNotPending {
		t.Errorf("expected ErrApplicationNotPending, got %v", err)
	}

	app, err := h.service.GetApplication(context.Background(), "t-release-reviewed")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if app.ClaimedByEmail == nil {
		t.Error("expected claim to remain in place after rejected release")
	}
}
