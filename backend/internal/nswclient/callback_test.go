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

// callbackCapture records requests made to the stub NSW service.
type callbackCapture struct {
	path string
	body map[string]any
}

func newCaptureServer(t *testing.T, capture *callbackCapture) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// EscapedPath, not Path: Path is decoded, which would hide whether a
		// slash inside the task ID was escaped into a single segment.
		capture.path = r.URL.EscapedPath()
		_ = json.Unmarshal(body, &capture.body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newTestClient returns a Client whose callbacks resolve against srv.
func newTestClient(srv *httptest.Server) *Client {
	return NewWithClient(httpclient.NewClientBuilder().WithBaseURL(srv.URL).Build())
}

func TestClient_SendOutcome(t *testing.T) {
	var capture callbackCapture
	srv := newCaptureServer(t, &capture)
	logs := captureLogs(t)

	client := newTestClient(srv)
	const sensitiveResponse = "sensitive reviewer response"
	err := client.SendOutcome(context.Background(), "token-123", CommandApprove, map[string]any{"comment": sensitiveResponse})
	if err != nil {
		t.Fatalf("SendOutcome failed: %v", err)
	}

	if capture.path != "/api/v1/callbacks/token-123" {
		t.Errorf("callback path: got %q, want %q", capture.path, "/api/v1/callbacks/token-123")
	}
	if capture.body["command"] != CommandApprove {
		t.Errorf("command: got %v, want %v", capture.body["command"], CommandApprove)
	}
	payload, ok := capture.body["payload"].(map[string]any)
	if !ok || payload["comment"] != sensitiveResponse {
		t.Errorf("payload forwarded incorrectly: got %v", capture.body["payload"])
	}

	logOutput := logs.String()
	if !strings.Contains(logOutput, `"callbackToken":"token-123"`) {
		t.Errorf("log does not contain the callback token: %s", logOutput)
	}
	if strings.Contains(logOutput, sensitiveResponse) {
		t.Errorf("log contains reviewer response: %s", logOutput)
	}
}

// The token is opaque, so one containing a slash must stay within one path
// segment rather than being split into two.
func TestClient_SendOutcome_EscapesCallbackToken(t *testing.T) {
	var capture callbackCapture
	srv := newCaptureServer(t, &capture)

	client := newTestClient(srv)
	if err := client.SendOutcome(context.Background(), "tenant/token-123", CommandApprove, nil); err != nil {
		t.Fatalf("SendOutcome failed: %v", err)
	}

	if capture.path != "/api/v1/callbacks/tenant%2Ftoken-123" {
		t.Errorf("callback path: got %q, want %q", capture.path, "/api/v1/callbacks/tenant%2Ftoken-123")
	}
}

// Without a token there is no step to address, so nothing is sent.
func TestClient_SendOutcome_RequiresCallbackToken(t *testing.T) {
	var capture callbackCapture
	srv := newCaptureServer(t, &capture)

	client := newTestClient(srv)
	if err := client.SendOutcome(context.Background(), "", CommandApprove, nil); err == nil {
		t.Fatal("expected an error for an empty callback token, got nil")
	}
	if capture.path != "" {
		t.Errorf("a request was sent without a callback token: %q", capture.path)
	}
}

func TestClient_RequestAmendment(t *testing.T) {
	var capture callbackCapture
	srv := newCaptureServer(t, &capture)

	client := newTestClient(srv)
	err := client.RequestAmendment(context.Background(), "token-abc", map[string]any{"feedback": "fix it"})
	if err != nil {
		t.Fatalf("RequestAmendment failed: %v", err)
	}

	if capture.path != "/api/v1/callbacks/token-abc" {
		t.Errorf("callback path: got %q, want %q", capture.path, "/api/v1/callbacks/token-abc")
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

	client := newTestClient(srv)
	if err := client.SendOutcome(context.Background(), "token-123", CommandApprove, nil); err == nil {
		t.Fatal("expected error on non-2xx response, got nil")
	}
}
