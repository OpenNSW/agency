package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockService is a mock implementation of Service for testing
type mockService struct {
	mockCreateUploadURL func(ctx context.Context, req UploadRequest) (*FileMetadata, error)
	mockGetDownloadURL  func(ctx context.Context, key string) (*DownloadMetadata, error)
}

func (m *mockService) CreateUploadURL(ctx context.Context, req UploadRequest) (*FileMetadata, error) {
	if m.mockCreateUploadURL != nil {
		return m.mockCreateUploadURL(ctx, req)
	}
	return nil, nil
}

func (m *mockService) GetDownloadURL(ctx context.Context, key string) (*DownloadMetadata, error) {
	if m.mockGetDownloadURL != nil {
		return m.mockGetDownloadURL(ctx, key)
	}
	return nil, nil
}

func TestNewHandler(t *testing.T) {
	t.Run("invalid config - negative", func(t *testing.T) {
		_, err := NewHandler(&mockService{}, -1)
		if err == nil {
			t.Fatal("expected error for negative MaxRequestBytes, got nil")
		}
		if !strings.Contains(err.Error(), "invalid MaxRequestBytes") {
			t.Fatalf("expected invalid MaxRequestBytes error, got %v", err)
		}
	})

	t.Run("invalid config - zero", func(t *testing.T) {
		_, err := NewHandler(&mockService{}, 0)
		if err == nil {
			t.Fatal("expected error for zero MaxRequestBytes, got nil")
		}
		if !strings.Contains(err.Error(), "invalid MaxRequestBytes") {
			t.Fatalf("expected invalid MaxRequestBytes error, got %v", err)
		}
	})
}

func TestHandleCreateUpload(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockSvc := &mockService{
			mockCreateUploadURL: func(ctx context.Context, req UploadRequest) (*FileMetadata, error) {
				return &FileMetadata{
					Key:       "123-abc",
					Name:      "test.txt",
					UploadURL: "http://test/upload",
				}, nil
			},
		}
		handler, err := NewHandler(mockSvc, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		body := []byte(`{"filename":"test.txt","mime_type":"text/plain","size":123}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/storage", bytes.NewBuffer(body))
		rec := httptest.NewRecorder()

		handler.HandleCreateUpload(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}

		var resp FileMetadata
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response body: %v", err)
		}

		if resp.Key != "123-abc" {
			t.Errorf("expected key '123-abc', got %v", resp.Key)
		}
		if resp.UploadURL != "http://test/upload" {
			t.Errorf("expected upload_url 'http://test/upload', got %v", resp.UploadURL)
		}
	})

	t.Run("service error", func(t *testing.T) {
		mockSvc := &mockService{
			mockCreateUploadURL: func(ctx context.Context, req UploadRequest) (*FileMetadata, error) {
				return nil, errors.New("upstream error")
			},
		}
		handler, err := NewHandler(mockSvc, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		body := []byte(`{"filename":"test.txt","mime_type":"text/plain","size":123}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/storage", bytes.NewBuffer(body))
		rec := httptest.NewRecorder()

		handler.HandleCreateUpload(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
		}
	})
}

func TestHandleGetUploadURL(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockSvc := &mockService{
			mockGetDownloadURL: func(ctx context.Context, key string) (*DownloadMetadata, error) {
				return &DownloadMetadata{
					DownloadURL: "http://test/download",
					ExpiresAt:   1234567890,
				}, nil
			},
		}
		handler, err := NewHandler(mockSvc, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/storage/550e8400-e29b-41d4-a716-446655440000.pdf", nil)
		req.SetPathValue("key", "550e8400-e29b-41d4-a716-446655440000.pdf") // Set the mux path value
		rec := httptest.NewRecorder()

		handler.HandleGetUploadURL(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d. Body: %s", http.StatusOK, rec.Code, rec.Body.String())
		}

		var resp DownloadMetadata
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response body: %v", err)
		}

		if resp.DownloadURL != "http://test/download" {
			t.Errorf("expected download_url 'http://test/download', got %v", resp.DownloadURL)
		}
		if resp.ExpiresAt != 1234567890 {
			t.Errorf("expected expires_at 1234567890, got %v", resp.ExpiresAt)
		}
	})
}
