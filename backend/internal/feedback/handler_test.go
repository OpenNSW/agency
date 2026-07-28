package feedback

import (
	"context"
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

func TestHandleFeedback_BodyTooLarge(t *testing.T) {
	handler, err := NewHandler(&mockService{}, 10)
	if err != nil {
		t.Fatalf("unexpected error creating handler: %v", err)
	}

	// Valid JSON prefix that forces the decoder to read past the 10-byte limit.
	body := strings.NewReader(`{"feedback":"` + strings.Repeat("a", 100) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/applications/task-123/feedback", body)
	req.SetPathValue("taskId", "task-123")
	w := httptest.NewRecorder()

	handler.HandleFeedback(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status %d, got %d", http.StatusRequestEntityTooLarge, w.Code)
	}
}

func TestHandleFeedback_InvalidJSON(t *testing.T) {
	handler, err := NewHandler(&mockService{}, 32<<20)
	if err != nil {
		t.Fatalf("unexpected error creating handler: %v", err)
	}

	body := strings.NewReader(`not json`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/applications/task-123/feedback", body)
	req.SetPathValue("taskId", "task-123")
	w := httptest.NewRecorder()

	handler.HandleFeedback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleFeedback_Success(t *testing.T) {
	svc := &recordingService{}
	handler, err := NewHandler(svc, 32<<20)
	if err != nil {
		t.Fatalf("unexpected error creating handler: %v", err)
	}

	body := strings.NewReader(`{"feedback":"looks good"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/applications/task-123/feedback", body)
	req.SetPathValue("taskId", "task-123")
	w := httptest.NewRecorder()

	handler.HandleFeedback(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if svc.taskID != "task-123" {
		t.Errorf("expected taskID %q, got %q", "task-123", svc.taskID)
	}
}

type recordingService struct {
	taskID string
}

func (s *recordingService) FeedbackApplication(ctx context.Context, taskID string, content map[string]any) error {
	s.taskID = taskID
	return nil
}
