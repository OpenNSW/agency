package certificate

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/OpenNSW/nsw-agency/backend/internal/application"
	"github.com/OpenNSW/nsw-agency/backend/pkg/httputil"
)

// fakeApplicationLookup is a fake ApplicationLookup for testing. It mirrors
// the real service's split: GetApplications returns lean summaries (TaskID
// and TaskCode only), GetApplication returns the full record.
type fakeApplicationLookup struct {
	items  []application.Application
	err    error            // returned by GetApplications
	getErr map[string]error // returned by GetApplication, keyed by TaskID
}

func (f *fakeApplicationLookup) GetApplications(ctx context.Context, status, consignmentID, search string, page, pageSize int) (*httputil.PagedResponse[application.Application], error) {
	if f.err != nil {
		return nil, f.err
	}
	summaries := make([]application.Application, len(f.items))
	for i, app := range f.items {
		summaries[i] = application.Application{TaskID: app.TaskID, TaskCode: app.TaskCode}
	}
	return &httputil.PagedResponse[application.Application]{
		Items:    summaries,
		Total:    int64(len(summaries)),
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (f *fakeApplicationLookup) GetApplication(ctx context.Context, taskID string) (*application.Application, error) {
	if err, ok := f.getErr[taskID]; ok {
		return nil, err
	}
	for _, app := range f.items {
		if app.TaskID == taskID {
			a := app
			return &a, nil
		}
	}
	return nil, fmt.Errorf("application not found: %s", taskID)
}

func TestIndexApplicationsByTaskCode(t *testing.T) {
	t.Run("no consignmentId returns an empty index", func(t *testing.T) {
		byTaskCode := indexApplicationsByTaskCode(context.Background(), &fakeApplicationLookup{}, "")
		if len(byTaskCode) != 0 {
			t.Errorf("expected an empty index, got %v", byTaskCode)
		}
	})

	t.Run("indexes applications by TaskCode, fetching each one's full record", func(t *testing.T) {
		lookup := &fakeApplicationLookup{
			items: []application.Application{
				{TaskID: "task-a", TaskCode: "task_a", Data: map[string]any{"x": "1"}},
				{TaskID: "task-b", TaskCode: "task_b", Data: map[string]any{"x": "2"}},
			},
		}

		byTaskCode := indexApplicationsByTaskCode(context.Background(), lookup, "CONSIGNMENT-1")

		if byTaskCode["task_a"].Data["x"] != "1" {
			t.Errorf("task_a = %v, want x=1", byTaskCode["task_a"])
		}
		if byTaskCode["task_b"].Data["x"] != "2" {
			t.Errorf("task_b = %v, want x=2", byTaskCode["task_b"])
		}
	})

	t.Run("GetApplications error yields an empty index, not a panic", func(t *testing.T) {
		lookup := &fakeApplicationLookup{err: errors.New("boom")}

		byTaskCode := indexApplicationsByTaskCode(context.Background(), lookup, "CONSIGNMENT-1")

		if len(byTaskCode) != 0 {
			t.Errorf("expected an empty index, got %v", byTaskCode)
		}
	})

	t.Run("a GetApplication error for one task skips it without failing the rest", func(t *testing.T) {
		lookup := &fakeApplicationLookup{
			items: []application.Application{
				{TaskID: "task-a", TaskCode: "task_a", Data: map[string]any{"x": "1"}},
				{TaskID: "task-b", TaskCode: "task_b", Data: map[string]any{"x": "2"}},
			},
			getErr: map[string]error{"task-a": fmt.Errorf("boom")},
		}

		byTaskCode := indexApplicationsByTaskCode(context.Background(), lookup, "CONSIGNMENT-1")

		if _, ok := byTaskCode["task_a"]; ok {
			t.Errorf("expected task_a to be skipped, got %v", byTaskCode["task_a"])
		}
		if byTaskCode["task_b"].Data["x"] != "2" {
			t.Errorf("task_b = %v, want x=2", byTaskCode["task_b"])
		}
	})
}

func TestLookupField(t *testing.T) {
	byTaskCode := map[string]application.Application{
		"task_v1": {
			Data:             map[string]any{"exporter_name": "ACME"},
			AgencyActionData: map[string]any{"reference_number": "034/00481"},
		},
	}

	t.Run("resolves from Data when review is false", func(t *testing.T) {
		got := lookupField(context.Background(), byTaskCode, "task_v1", "exporter_name", false)
		if got != "ACME" {
			t.Errorf("got %q, want ACME", got)
		}
	})

	t.Run("resolves from AgencyActionData when review is true", func(t *testing.T) {
		got := lookupField(context.Background(), byTaskCode, "task_v1", "reference_number", true)
		if got != "034/00481" {
			t.Errorf("got %q, want 034/00481", got)
		}
	})

	t.Run("unknown taskCode returns empty string without panicking", func(t *testing.T) {
		got := lookupField(context.Background(), byTaskCode, "unknown_task", "exporter_name", false)
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("unknown field returns empty string", func(t *testing.T) {
		got := lookupField(context.Background(), byTaskCode, "task_v1", "unknown_field", false)
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}

func TestRealFuncsToday(t *testing.T) {
	today, ok := realFuncs(context.Background(), nil)["today"].(func() string)
	if !ok {
		t.Fatal("expected today to be a func() string")
	}
	if today() == "" {
		t.Error("expected today() to return a non-empty date")
	}
}
