package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenNSW/nsw-agency/backend/internal/nswclient"
	"github.com/OpenNSW/nsw-agency/backend/internal/rbac"
	"github.com/OpenNSW/nsw-agency/backend/pkg/httpclient"
)

// fakeFileResolver stubs FileResolver: it "resolves" any key to a canned URL,
// except keys equal to failingKey, which simulate an upstream failure.
type fakeFileResolver struct {
	failingKey string
	calls      []string
}

func (f *fakeFileResolver) GetDownloadURL(_ context.Context, key string) (*nswclient.DownloadMetadata, error) {
	f.calls = append(f.calls, key)
	if key == f.failingKey {
		return nil, fmt.Errorf("simulated upstream failure for key %q", key)
	}
	return &nswclient.DownloadMetadata{DownloadURL: "https://files.example.com/" + key, ExpiresAt: 123}, nil
}

// stubFileResolver returns a fixed metadata/error pair regardless of key,
// letting tests probe resolveKey's response to unusual resolver outputs.
type stubFileResolver struct {
	metadata *nswclient.DownloadMetadata
	err      error
}

func (s stubFileResolver) GetDownloadURL(_ context.Context, _ string) (*nswclient.DownloadMetadata, error) {
	return s.metadata, s.err
}

func TestResolveKey_NilOrEmptyMetadata_OmitsRatherThanPanics(t *testing.T) {
	tests := []struct {
		name     string
		resolver FileResolver
	}{
		{"nil metadata", stubFileResolver{metadata: nil, err: nil}},
		{"empty download URL", stubFileResolver{metadata: &nswclient.DownloadMetadata{}, err: nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveKey(context.Background(), tt.resolver, "key-1")
			if got != "" {
				t.Errorf("resolveKey: got %q, want empty string", got)
			}
		})
	}
}

func TestGetApplication_ResolvesFileKeysToPresignedURLs(t *testing.T) {
	root := t.TempDir()
	for _, sub := range []string{"task-configs", "forms"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("failed to create %s dir: %v", sub, err)
		}
	}
	writeTaskConfigFile(t, root, "alpha.json", `{
		"meta": {"title": "Alpha"},
		"forms": {"view": "alpha_view", "review": "alpha_review"}
	}`)
	writeFormFile(t, root, "alpha_view.json", `{
		"schema": {
			"type": "object",
			"properties": {
				"single_doc": {"type": "string", "format": "file"},
				"multi_docs": {"type": "array", "items": {"type": "string", "format": "file"}},
				"attachments": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"attachment_file_url": {"type": "string", "format": "file"},
							"label": {"type": "string"}
						}
					}
				},
				"broken_doc": {"type": "string", "format": "file"},
				"plain_field": {"type": "string"}
			}
		},
		"uiSchema": {"type": "VerticalLayout"}
	}`)
	writeFormFile(t, root, "alpha_review.json", `{
		"schema": {
			"type": "object",
			"properties": {
				"reviewer_doc": {"type": "string", "format": "file"}
			}
		},
		"uiSchema": {"type": "VerticalLayout"}
	}`)

	store := newTestStore(t)
	reg := newTestRegistry(t, root)
	roleService := rbac.NewRoleService(store.db)
	resolver := &fakeFileResolver{failingKey: "key-fail"}
	hc := httpclient.NewClientBuilder().Build()
	svc := NewService(store, reg, nswclient.NewWithClient(hc), roleService, resolver)
	t.Cleanup(func() { _ = svc.Close() })

	if err := store.CreateOrUpdate(&ApplicationRecord{
		TaskID:        "t-files",
		TaskCode:      "alpha",
		ConsignmentID: "wf-test",
		ServiceURL:    "http://unused.example",
		Data: JSONB{
			"single_doc": "key-1",
			"multi_docs": []any{"key-2", "key-3"},
			"attachments": []any{
				map[string]any{"attachment_file_url": "key-4", "label": "Invoice"},
			},
			"broken_doc":  "key-fail",
			"plain_field": "not a file",
		},
		Status: "PENDING",
	}); err != nil {
		t.Fatalf("failed to seed record: %v", err)
	}
	if err := store.UpdateStatus("t-files", "DONE", map[string]any{
		"reviewer_doc": "key-5",
	}); err != nil {
		t.Fatalf("failed to seed reviewer response: %v", err)
	}

	app, err := svc.GetApplication(context.Background(), "t-files")
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}

	if got := app.Data["single_doc"]; got != "https://files.example.com/key-1" {
		t.Errorf("single_doc: got %v, want resolved URL", got)
	}
	multiDocs, ok := app.Data["multi_docs"].([]any)
	if !ok || len(multiDocs) != 2 ||
		multiDocs[0] != "https://files.example.com/key-2" ||
		multiDocs[1] != "https://files.example.com/key-3" {
		t.Errorf("multi_docs: got %v, want both keys resolved", app.Data["multi_docs"])
	}
	attachments, ok := app.Data["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("attachments: got %v, want 1 entry", app.Data["attachments"])
	}
	entry, ok := attachments[0].(map[string]any)
	if !ok || entry["attachment_file_url"] != "https://files.example.com/key-4" {
		t.Errorf("attachments[0].attachment_file_url: got %v, want resolved URL", entry["attachment_file_url"])
	}
	if entry["label"] != "Invoice" {
		t.Errorf("attachments[0].label: got %v, want unchanged", entry["label"])
	}
	if got := app.Data["broken_doc"]; got != "" {
		t.Errorf("broken_doc: got %v, want raw key dropped on resolver failure", got)
	}
	if got := app.Data["plain_field"]; got != "not a file" {
		t.Errorf("plain_field: got %v, want unchanged (not a file field)", got)
	}

	if got := app.AgencyActionData["reviewer_doc"]; got != "https://files.example.com/key-5" {
		t.Errorf("agencyActionData.reviewer_doc: got %v, want resolved URL", got)
	}

	for _, key := range []string{"key-1", "key-2", "key-3", "key-4", "key-fail", "key-5"} {
		found := false
		for _, called := range resolver.calls {
			if called == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected resolver to be called with key %q, calls were %v", key, resolver.calls)
		}
	}
}
