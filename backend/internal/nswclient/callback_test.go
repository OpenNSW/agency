package nswclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/agency/backend/pkg/httpclient"
)

func TestBuildCallbackURL(t *testing.T) {
	tests := []struct {
		name       string
		serviceURL string
		taskID     string
		want       string
	}{
		{
			name:       "simple URL",
			serviceURL: "http://example.com/callback",
			taskID:     "task-123",
			want:       "http://example.com/callback/task-123",
		},
		{
			name:       "simple URL with trailing slash",
			serviceURL: "http://example.com/callback/",
			taskID:     "task-123",
			want:       "http://example.com/callback/task-123",
		},
		{
			name:       "URL with placeholder",
			serviceURL: "http://example.com/callback/{id}/submit",
			taskID:     "task-123",
			want:       "http://example.com/callback/task-123/submit",
		},
		{
			name:       "URL with query parameters",
			serviceURL: "http://example.com/callback?token=xyz",
			taskID:     "task-123",
			want:       "http://example.com/callback/task-123?token=xyz",
		},
		{
			name:       "URL with query parameters and trailing slash in path",
			serviceURL: "http://example.com/callback/?token=xyz",
			taskID:     "task-123",
			want:       "http://example.com/callback/task-123?token=xyz",
		},
		{
			name:       "task ID containing a slash stays one segment",
			serviceURL: "http://example.com/callback",
			taskID:     "tenant/task-123",
			want:       "http://example.com/callback/tenant%2Ftask-123",
		},
		{
			name:       "task ID containing a slash with placeholder",
			serviceURL: "http://example.com/callback/{id}/submit",
			taskID:     "tenant/task-123",
			want:       "http://example.com/callback/tenant%2Ftask-123/submit",
		},
		{
			name:       "invalid URL fallback",
			serviceURL: ":invalid-url",
			taskID:     "task-123",
			want:       ":invalid-url/task-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCallbackURL(tt.serviceURL, tt.taskID)
			if got != tt.want {
				t.Errorf("buildCallbackURL(%q, %q) = %q, want %q", tt.serviceURL, tt.taskID, got, tt.want)
			}
		})
	}
}

// callbackCapture records requests made to the stub NSW service.
type callbackCapture struct {
	path string
	body map[string]any
}

func newCaptureServer(t *testing.T, capture *callbackCapture) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capture.path = r.URL.Path
		_ = json.Unmarshal(body, &capture.body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClient_SendOutcome(t *testing.T) {
	var capture callbackCapture
	srv := newCaptureServer(t, &capture)
	logs := captureLogs(t)

	client := NewWithClient(httpclient.NewClientBuilder().Build())
	const sensitiveResponse = "sensitive reviewer response"
	err := client.SendOutcome(context.Background(), srv.URL+"?token=secret-callback-token", "task-123", CommandApprove, map[string]any{"comment": sensitiveResponse})
	if err != nil {
		t.Fatalf("SendOutcome failed: %v", err)
	}

	if capture.path != "/task-123" {
		t.Errorf("callback path: got %q, want %q", capture.path, "/task-123")
	}
	if capture.body["command"] != CommandApprove {
		t.Errorf("command: got %v, want %v", capture.body["command"], CommandApprove)
	}
	payload, ok := capture.body["payload"].(map[string]any)
	if !ok || payload["comment"] != sensitiveResponse {
		t.Errorf("payload forwarded incorrectly: got %v", capture.body["payload"])
	}

	logOutput := logs.String()
	if !strings.Contains(logOutput, `"taskID":"task-123"`) {
		t.Errorf("log does not contain task identifier: %s", logOutput)
	}
	for _, sensitiveValue := range []string{srv.URL, "secret-callback-token", sensitiveResponse} {
		if strings.Contains(logOutput, sensitiveValue) {
			t.Errorf("log contains sensitive value %q: %s", sensitiveValue, logOutput)
		}
	}
}

func TestClient_RequestAmendment(t *testing.T) {
	var capture callbackCapture
	srv := newCaptureServer(t, &capture)

	client := NewWithClient(httpclient.NewClientBuilder().Build())
	err := client.RequestAmendment(context.Background(), srv.URL, "task-abc", map[string]any{"feedback": "fix it"})
	if err != nil {
		t.Fatalf("RequestAmendment failed: %v", err)
	}

	if capture.body["command"] != CommandRequestAmendment {
		t.Errorf("command: got %v, want %v", capture.body["command"], CommandRequestAmendment)
	}
}

func TestClient_SendOutcome_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewWithClient(httpclient.NewClientBuilder().Build())
	if err := client.SendOutcome(context.Background(), srv.URL, "task-123", CommandApprove, nil); err == nil {
		t.Fatal("expected error on non-2xx response, got nil")
	}
}
