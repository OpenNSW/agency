package nswclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/nsw-agency/backend/pkg/httpclient"
)

func TestClient_CreateUploadURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/storage", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"key":"123-abc", "name":"test.txt", "upload_url":"http://test/upload"}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewWithClient(httpclient.NewClientBuilder().WithBaseURL(server.URL + "/").Build())

	result, err := client.CreateUploadURL(context.Background(), UploadRequest{
		Filename: "test.txt",
		MimeType: "text/plain",
		Size:     123,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.Key != "123-abc" {
		t.Errorf("expected key '123-abc', got %v", result.Key)
	}
	if result.UploadURL != "http://test/upload" {
		t.Errorf("expected upload_url 'http://test/upload', got %v", result.UploadURL)
	}
}

func TestClient_CreateUploadURL_InvalidRequest(t *testing.T) {
	client := NewWithClient(httpclient.NewClientBuilder().Build())

	if _, err := client.CreateUploadURL(context.Background(), UploadRequest{}); err == nil {
		t.Fatal("expected error for missing required fields, got nil")
	}
}

func TestClient_GetDownloadURL(t *testing.T) {
	const (
		storageKey  = "550e8400-e29b-41d4-a716-446655440000.pdf"
		downloadURL = "http://test/download?X-Amz-Signature=secret-signature"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/storage/"+storageKey, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"download_url":"` + downloadURL + `", "expires_at": 1234567890}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()
	logs := captureLogs(t)

	client := NewWithClient(httpclient.NewClientBuilder().WithBaseURL(server.URL + "/").Build())

	metadata, err := client.GetDownloadURL(context.Background(), storageKey)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if metadata.DownloadURL != downloadURL {
		t.Errorf("expected download_url %q, got %q", downloadURL, metadata.DownloadURL)
	}
	if metadata.ExpiresAt != 1234567890 {
		t.Errorf("expected expires_at 1234567890, got %v", metadata.ExpiresAt)
	}

	logOutput := logs.String()
	if !strings.Contains(logOutput, `"key":"`+storageKey+`"`) {
		t.Errorf("log does not contain storage key: %s", logOutput)
	}
	if strings.Contains(logOutput, downloadURL) || strings.Contains(logOutput, "secret-signature") {
		t.Errorf("log contains presigned download URL: %s", logOutput)
	}
}
