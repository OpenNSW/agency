package application

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockService is a mock implementation of Service for testing
type mockService struct {
	// embed the interface so we don't have to implement everything
	Service
}

// stubCreateApplicationService overrides CreateApplication to return a fixed
// error, so handler tests can exercise error-mapping without a real Service.
type stubCreateApplicationService struct {
	mockService
	err error
}

func (s *stubCreateApplicationService) CreateApplication(ctx context.Context, req *InjectRequest) error {
	return s.err
}

func TestNewHandler(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		handler, err := NewHandler(&mockService{}, 32<<20)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if handler == nil {
			t.Fatalf("expected handler to be non-nil")
		}
		if handler.MaxRequestBytes != 32<<20 {
			t.Errorf("expected MaxRequestBytes %d, got %d", 32<<20, handler.MaxRequestBytes)
		}
	})

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

func TestHandleInjectData_BodyTooLarge(t *testing.T) {
	handler, err := NewHandler(&mockService{}, 10)
	if err != nil {
		t.Fatalf("unexpected error creating handler: %v", err)
	}

	// Valid JSON prefix that forces the decoder to read past the 10-byte limit.
	body := strings.NewReader(`{"key":"` + strings.Repeat("a", 100) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inject", body)
	w := httptest.NewRecorder()

	handler.HandleInjectData(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status %d, got %d", http.StatusRequestEntityTooLarge, w.Code)
	}
}

func TestHandleInjectData_InvalidServiceURL(t *testing.T) {
	handler, err := NewHandler(&stubCreateApplicationService{
		err: fmt.Errorf("%w: service URL origin is not the configured NSW service", ErrInvalidServiceURL),
	}, 32<<20)
	if err != nil {
		t.Fatalf("unexpected error creating handler: %v", err)
	}

	body := strings.NewReader(`{
		"taskId": "task-123",
		"taskCode": "alpha",
		"consignmentId": "wf-test",
		"serviceUrl": "http://evil.example/callback",
		"data": {}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inject", body)
	w := httptest.NewRecorder()

	handler.HandleInjectData(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleReviewApplication_BodyTooLarge(t *testing.T) {
	handler, err := NewHandler(&mockService{}, 10)
	if err != nil {
		t.Fatalf("unexpected error creating handler: %v", err)
	}

	// Valid JSON prefix that forces the decoder to read past the 10-byte limit.
	body := strings.NewReader(`{"key":"` + strings.Repeat("a", 100) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/applications/task-123/review", body)
	req.SetPathValue("taskId", "task-123")
	w := httptest.NewRecorder()

	handler.HandleReviewApplication(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status %d, got %d", http.StatusRequestEntityTooLarge, w.Code)
	}
}
