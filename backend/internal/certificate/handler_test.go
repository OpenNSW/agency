package certificate

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/agency/backend/internal/application"
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

// newRequestWithTaskID builds a request carrying taskId as a path value, the
// way it arrives once routed through http.ServeMux's {taskId} pattern.
func newRequestWithTaskID(taskID string, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/applications/"+taskID+"/certificate", bytes.NewBuffer(body))
	req.SetPathValue("taskId", taskID)
	return req
}

func TestNewHandler(t *testing.T) {
	t.Run("invalid config - negative", func(t *testing.T) {
		_, err := NewHandler(&mockService{}, &fakeApplicationLookup{}, -1)
		if err == nil {
			t.Fatal("expected error for negative MaxRequestBytes, got nil")
		}
		if !strings.Contains(err.Error(), "invalid MaxRequestBytes") {
			t.Fatalf("expected invalid MaxRequestBytes error, got %v", err)
		}
	})

	t.Run("invalid config - zero", func(t *testing.T) {
		_, err := NewHandler(&mockService{}, &fakeApplicationLookup{}, 0)
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
		lookup := &fakeApplicationLookup{items: []application.Application{
			{TaskID: "task-1", ConsignmentID: "CONSIGNMENT-1", CertificateTemplateID: "welcome"},
		}}
		mockSvc := &mockService{
			mockGenerate: func(ctx context.Context, templateID, consignmentID string, data map[string]any) (string, error) {
				if templateID != "welcome" {
					t.Errorf("expected templateId 'welcome', got %v", templateID)
				}
				if consignmentID != "CONSIGNMENT-1" {
					t.Errorf("expected consignmentId 'CONSIGNMENT-1', got %v", consignmentID)
				}
				return "<html>Congratulations, Officer!</html>", nil
			},
		}
		handler, err := NewHandler(mockSvc, lookup, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		body := []byte(`{"data":{"Name":"Officer"}}`)
		req := newRequestWithTaskID("task-1", body)
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

	t.Run("missing taskId", func(t *testing.T) {
		handler, err := NewHandler(&mockService{}, &fakeApplicationLookup{}, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/applications//certificate", bytes.NewBuffer([]byte(`{}`)))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("invalid request body", func(t *testing.T) {
		handler, err := NewHandler(&mockService{}, &fakeApplicationLookup{}, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		req := newRequestWithTaskID("task-1", []byte(`not json`))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("application not found", func(t *testing.T) {
		handler, err := NewHandler(&mockService{}, &fakeApplicationLookup{}, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		req := newRequestWithTaskID("missing-task", []byte(`{}`))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
		}
	})

	t.Run("task has no certificate configured", func(t *testing.T) {
		lookup := &fakeApplicationLookup{items: []application.Application{
			{TaskID: "task-1"},
		}}
		handler, err := NewHandler(&mockService{}, lookup, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		req := newRequestWithTaskID("task-1", []byte(`{}`))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("data failing the configured schema is rejected", func(t *testing.T) {
		lookup := &fakeApplicationLookup{items: []application.Application{
			{
				TaskID:                "task-1",
				CertificateTemplateID: "welcome",
				CertificateDataSchema: []byte(`{"type":"object","required":["certificate_id"]}`),
			},
		}}
		handler, err := NewHandler(&mockService{}, lookup, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		req := newRequestWithTaskID("task-1", []byte(`{"data":{}}`))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("data satisfying the configured schema is accepted", func(t *testing.T) {
		lookup := &fakeApplicationLookup{items: []application.Application{
			{
				TaskID:                "task-1",
				CertificateTemplateID: "welcome",
				CertificateDataSchema: []byte(`{"type":"object","required":["certificate_id"]}`),
			},
		}}
		mockSvc := &mockService{
			mockGenerate: func(ctx context.Context, templateID, consignmentID string, data map[string]any) (string, error) {
				return "<html></html>", nil
			},
		}
		handler, err := NewHandler(mockSvc, lookup, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		req := newRequestWithTaskID("task-1", []byte(`{"data":{"certificate_id":"CERT-1"}}`))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})

	t.Run("template not found", func(t *testing.T) {
		lookup := &fakeApplicationLookup{items: []application.Application{
			{TaskID: "task-1", CertificateTemplateID: "missing"},
		}}
		mockSvc := &mockService{
			mockGenerate: func(ctx context.Context, templateID, consignmentID string, data map[string]any) (string, error) {
				return "", artifact.ErrNotFound
			},
		}
		handler, err := NewHandler(mockSvc, lookup, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		req := newRequestWithTaskID("task-1", []byte(`{}`))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
		}
	})

	t.Run("service error", func(t *testing.T) {
		lookup := &fakeApplicationLookup{items: []application.Application{
			{TaskID: "task-1", CertificateTemplateID: "welcome"},
		}}
		mockSvc := &mockService{
			mockGenerate: func(ctx context.Context, templateID, consignmentID string, data map[string]any) (string, error) {
				return "", errors.New("execution failure")
			},
		}
		handler, err := NewHandler(mockSvc, lookup, 32<<20)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		req := newRequestWithTaskID("task-1", []byte(`{}`))
		rec := httptest.NewRecorder()

		handler.HandleGenerate(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
		}
	})
}
