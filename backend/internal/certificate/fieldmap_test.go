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
// and TaskCode only), optionally filtered by TaskCode; GetApplication returns
// the full record. getApplicationsCalls counts calls per taskCode, so tests
// can assert a taskCode is looked up at most once per template render.
type fakeApplicationLookup struct {
	items                []application.Application
	err                  error            // returned by GetApplications
	getErr               map[string]error // returned by GetApplication, keyed by TaskID
	getApplicationsCalls map[string]int
}

func (f *fakeApplicationLookup) GetApplications(ctx context.Context, status, consignmentID, taskCode, search string, page, pageSize int) (*httputil.PagedResponse[application.Application], error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.getApplicationsCalls == nil {
		f.getApplicationsCalls = map[string]int{}
	}
	f.getApplicationsCalls[taskCode]++

	var matched []application.Application
	for _, app := range f.items {
		if taskCode != "" && app.TaskCode != taskCode {
			continue
		}
		matched = append(matched, application.Application{TaskID: app.TaskID, TaskCode: app.TaskCode})
	}

	start := (page - 1) * pageSize
	if start > len(matched) {
		start = len(matched)
	}
	end := start + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	return &httputil.PagedResponse[application.Application]{
		Items:    matched[start:end],
		Total:    int64(len(matched)),
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
	return nil, application.ErrApplicationNotFound
}

func TestLookupApplicationByTaskCode(t *testing.T) {
	t.Run("no consignmentId returns nil", func(t *testing.T) {
		app := lookupApplicationByTaskCode(context.Background(), &fakeApplicationLookup{}, "", "task_a")
		if app != nil {
			t.Errorf("expected nil, got %v", app)
		}
	})

	t.Run("finds the application by taskCode within the consignment and fetches its full record", func(t *testing.T) {
		lookup := &fakeApplicationLookup{
			items: []application.Application{
				{TaskID: "task-a", TaskCode: "task_a", Data: map[string]any{"x": "1"}},
				{TaskID: "task-b", TaskCode: "task_b", Data: map[string]any{"x": "2"}},
			},
		}

		app := lookupApplicationByTaskCode(context.Background(), lookup, "CONSIGNMENT-1", "task_b")

		if app == nil || app.Data["x"] != "2" {
			t.Errorf("task_b = %v, want x=2", app)
		}
	})

	t.Run("GetApplications error yields nil, not a panic", func(t *testing.T) {
		lookup := &fakeApplicationLookup{err: errors.New("boom")}

		app := lookupApplicationByTaskCode(context.Background(), lookup, "CONSIGNMENT-1", "task_a")

		if app != nil {
			t.Errorf("expected nil, got %v", app)
		}
	})

	t.Run("a taskCode absent from the consignment yields nil, not an error", func(t *testing.T) {
		lookup := &fakeApplicationLookup{items: []application.Application{
			{TaskID: "task-a", TaskCode: "task_a"},
		}}

		app := lookupApplicationByTaskCode(context.Background(), lookup, "CONSIGNMENT-1", "unknown_task")

		if app != nil {
			t.Errorf("expected nil, got %v", app)
		}
	})

	t.Run("a GetApplication error yields nil, not a panic", func(t *testing.T) {
		lookup := &fakeApplicationLookup{
			items:  []application.Application{{TaskID: "task-a", TaskCode: "task_a"}},
			getErr: map[string]error{"task-a": fmt.Errorf("boom")},
		}

		app := lookupApplicationByTaskCode(context.Background(), lookup, "CONSIGNMENT-1", "task_a")

		if app != nil {
			t.Errorf("expected nil, got %v", app)
		}
	})

	t.Run("finds a task regardless of how many other applications are in the consignment", func(t *testing.T) {
		// Regression test: this task previously went missing when a
		// consignment had more applications than a single list page (100),
		// because the old implementation indexed one page up front instead
		// of looking the task up directly.
		items := make([]application.Application, 0, 150)
		for i := 0; i < 149; i++ {
			items = append(items, application.Application{TaskID: fmt.Sprintf("task-%d", i), TaskCode: fmt.Sprintf("task_%d", i)})
		}
		items = append(items, application.Application{
			TaskID:   "task-last",
			TaskCode: "fcau_application_review_v1",
			Data:     map[string]any{"exporter_name": "STAY NATURALS PRIVATE LIMITED"},
		})
		lookup := &fakeApplicationLookup{items: items}

		app := lookupApplicationByTaskCode(context.Background(), lookup, "CONSIGNMENT-1", "fcau_application_review_v1")

		if app == nil || app.Data["exporter_name"] != "STAY NATURALS PRIVATE LIMITED" {
			t.Errorf("expected the task to be found regardless of consignment size, got %v", app)
		}
	})
}

func TestFieldValue(t *testing.T) {
	app := &application.Application{
		Data:             map[string]any{"exporter_name": "ACME"},
		AgencyActionData: map[string]any{"reference_number": "034/00481"},
	}

	t.Run("resolves from Data when review is false", func(t *testing.T) {
		got := fieldValue(app, "exporter_name", false)
		if got != "ACME" {
			t.Errorf("got %q, want ACME", got)
		}
	})

	t.Run("resolves from AgencyActionData when review is true", func(t *testing.T) {
		got := fieldValue(app, "reference_number", true)
		if got != "034/00481" {
			t.Errorf("got %q, want 034/00481", got)
		}
	})

	t.Run("nil app returns empty string without panicking", func(t *testing.T) {
		got := fieldValue(nil, "exporter_name", false)
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("unknown field returns empty string", func(t *testing.T) {
		got := fieldValue(app, "unknown_field", false)
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}

func TestRealFuncsFromDataFromReview(t *testing.T) {
	t.Run("resolves fields and caches the lookup per taskCode", func(t *testing.T) {
		lookup := &fakeApplicationLookup{
			items: []application.Application{
				{
					TaskID:   "task-1",
					TaskCode: "fcau_application_review_v1",
					Data:     map[string]any{"exporter_name": "ACME"},
					AgencyActionData: map[string]any{
						"reference_number": "034/00481",
					},
				},
			},
		}

		funcs := realFuncs(context.Background(), lookup, "CONSIGNMENT-1")
		fromData := funcs["fromData"].(func(string, string) string)
		fromReview := funcs["fromReview"].(func(string, string) string)

		if got := fromData("fcau_application_review_v1", "exporter_name"); got != "ACME" {
			t.Errorf("fromData = %q, want ACME", got)
		}
		if got := fromReview("fcau_application_review_v1", "reference_number"); got != "034/00481" {
			t.Errorf("fromReview = %q, want 034/00481", got)
		}
		if calls := lookup.getApplicationsCalls["fcau_application_review_v1"]; calls != 1 {
			t.Errorf("expected the taskCode to be looked up once and cached, got %d GetApplications calls", calls)
		}
	})

	t.Run("a task absent from the consignment leaves fromData/fromReview calls blank, not erroring", func(t *testing.T) {
		funcs := realFuncs(context.Background(), &fakeApplicationLookup{}, "CONSIGNMENT-1")
		fromData := funcs["fromData"].(func(string, string) string)

		if got := fromData("fcau_application_review_v1", "exporter_name"); got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}

func TestRealFuncsToday(t *testing.T) {
	today, ok := realFuncs(context.Background(), nil, "")["today"].(func() string)
	if !ok {
		t.Fatal("expected today to be a func() string")
	}
	if today() == "" {
		t.Error("expected today() to return a non-empty date")
	}
}
