package certificate

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/core/artifact"
)

// mockService is a mock implementation of Service for testing.
type mockService struct {
	mockGenerate func(ctx context.Context, templateID, consignmentID string, data map[string]any) (string, error)
}

func (m *mockService) Generate(ctx context.Context, templateID, consignmentID string, data map[string]any) (string, error) {
	if m.mockGenerate != nil {
		return m.mockGenerate(ctx, templateID, consignmentID, data)
	}
	return "", nil
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

func TestHandleGenerate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockSvc := &mockService{
			mockGenerate: func(ctx context.Context, templateID, consignmentID string, data map[string]any) (string, error) {
				if templateID != "welcome" {
					t.Errorf("expected templateId 'welcome', got %v", templateID)
				}
				return "<html>Congratulations, Officer!</html>", nil
			},
		}
		handler, err := NewHandler(mockSvc, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		body := []byte(`{"templateId":"welcome","data":{"Name":"Officer"}}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates/generate", bytes.NewBuffer(body))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("expected Content-Type 'text/html; charset=utf-8', got %v", ct)
		}
		if rec.Body.String() != "<html>Congratulations, Officer!</html>" {
			t.Errorf("unexpected body: %v", rec.Body.String())
		}
	})

	t.Run("invalid request body", func(t *testing.T) {
		handler, err := NewHandler(&mockService{}, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates/generate", bytes.NewBuffer([]byte(`not json`)))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("missing templateId", func(t *testing.T) {
		handler, err := NewHandler(&mockService{}, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates/generate", bytes.NewBuffer([]byte(`{"data":{}}`)))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("template not found", func(t *testing.T) {
		mockSvc := &mockService{
			mockGenerate: func(ctx context.Context, templateID, consignmentID string, data map[string]any) (string, error) {
				return "", artifact.ErrNotFound
			},
		}
		handler, err := NewHandler(mockSvc, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		body := []byte(`{"templateId":"missing"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates/generate", bytes.NewBuffer(body))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
		}
	})

	t.Run("service error", func(t *testing.T) {
		mockSvc := &mockService{
			mockGenerate: func(ctx context.Context, templateID, consignmentID string, data map[string]any) (string, error) {
				return "", errors.New("execution failure")
			},
		}
		handler, err := NewHandler(mockSvc, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		body := []byte(`{"templateId":"welcome"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates/generate", bytes.NewBuffer(body))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
		}
	})

	t.Run("consignmentId is passed through from the request body", func(t *testing.T) {
		var gotConsignmentID string
		mockSvc := &mockService{
			mockGenerate: func(ctx context.Context, templateID, consignmentID string, data map[string]any) (string, error) {
				gotConsignmentID = consignmentID
				return "<html></html>", nil
			},
		}
		handler, err := NewHandler(mockSvc, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		body := []byte(`{"templateId":"welcome","consignmentId":"CONSIGNMENT-123"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates/generate", bytes.NewBuffer(body))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
		if gotConsignmentID != "CONSIGNMENT-123" {
			t.Errorf("expected consignmentId 'CONSIGNMENT-123', got %v", gotConsignmentID)
		}
	})
}
