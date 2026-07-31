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

// mockService is a mock implementation of Service for testing.
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

func TestNewHandler_Validation(t *testing.T) {
	t.Run("valid maxRequestBytes", func(t *testing.T) {
		h, err := NewHandler(&mockService{}, 1024)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if h == nil {
			t.Fatal("expected non-nil handler")
		}
	})

	t.Run("invalid maxRequestBytes zero", func(t *testing.T) {
		_, err := NewHandler(&mockService{}, 0)
		if err == nil {
			t.Fatal("expected error for zero MaxRequestBytes, got nil")
		}
		if !strings.Contains(err.Error(), "invalid MaxRequestBytes") {
			t.Fatalf("expected invalid MaxRequestBytes error, got %v", err)
		}
	})

	t.Run("invalid maxRequestBytes negative", func(t *testing.T) {
		_, err := NewHandler(&mockService{}, -1)
		if err == nil {
			t.Fatal("expected error for negative MaxRequestBytes, got nil")
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

		var res FileMetadata
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if res.Key != "123-abc" || res.UploadURL != "http://test/upload" {
			t.Errorf("unexpected response: %+v", res)
		}
	})

	t.Run("service_error", func(t *testing.T) {
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

	t.Run("validation_error_maps_to_bad_request", func(t *testing.T) {
		mockSvc := &mockService{
			mockCreateUploadURL: func(ctx context.Context, req UploadRequest) (*FileMetadata, error) {
				return nil, ErrProhibitedFileType
			},
		}
		handler, err := NewHandler(mockSvc, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		body := []byte(`{"filename":"malware.exe","mime_type":"application/octet-stream","size":123}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/storage", bytes.NewBuffer(body))
		rec := httptest.NewRecorder()

		handler.HandleCreateUpload(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d for validation error, got %d", http.StatusBadRequest, rec.Code)
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
		req.SetPathValue("key", "550e8400-e29b-41d4-a716-446655440000.pdf")
		rec := httptest.NewRecorder()

		handler.HandleGetUploadURL(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}

		var res DownloadMetadata
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if res.DownloadURL != "http://test/download" {
			t.Errorf("unexpected download URL: %s", res.DownloadURL)
		}
	})
}
