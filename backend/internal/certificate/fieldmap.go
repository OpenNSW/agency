package certificate

import (
	"context"
	"html/template"
	"log/slog"
	"time"

	"github.com/OpenNSW/nsw-agency/backend/internal/application"
	"github.com/OpenNSW/nsw-agency/backend/pkg/httputil"
)

// certificateDateFormat matches the certificate spec's date style (e.g. "09/07/2026").
const certificateDateFormat = "02/01/2006"

// ApplicationLookup is the subset of application.Service this package needs to
// resolve a certificate template's fromData/fromReview calls. GetApplications
// only returns a lean list (no Data/AgencyActionData), so it's used solely to
// discover which tasks belong to a consignment; GetApplication then fetches
// each task's full record.
type ApplicationLookup interface {
	GetApplications(ctx context.Context, status, consignmentID, search string, page, pageSize int) (*httputil.PagedResponse[application.Application], error)
	GetApplication(ctx context.Context, taskID string) (*application.Application, error)
}

// stubFuncs registers every certificate template function with a no-op body,
// just so a template calling them parses successfully. Used to validate a
// template's syntax at artifact-load time, before any request-specific
// consignment data exists to build the real functions.
func stubFuncs() template.FuncMap {
	return template.FuncMap{
		"fromData":   func(taskCode, field string) string { return "" },
		"fromReview": func(taskCode, field string) string { return "" },
		"today":      func() string { return "" },
	}
}

// realFuncs builds the certificate template functions for one Generate call:
// fromData/fromReview resolve against byTaskCode, today against the current time.
func realFuncs(ctx context.Context, byTaskCode map[string]application.Application) template.FuncMap {
	return template.FuncMap{
		"fromData": func(taskCode, field string) string {
			return lookupField(ctx, byTaskCode, taskCode, field, false)
		},
		"fromReview": func(taskCode, field string) string {
			return lookupField(ctx, byTaskCode, taskCode, field, true)
		},
		"today": func() string {
			return time.Now().Format(certificateDateFormat)
		},
	}
}

// lookupField reads field off the named task's Application.Data (or
// AgencyActionData, when review is true). It never fails on missing data — a
// certificate preview should still render with whatever is available, so gaps
// are logged, not returned as errors.
func lookupField(ctx context.Context, byTaskCode map[string]application.Application, taskCode, field string, review bool) string {
	app, ok := byTaskCode[taskCode]
	if !ok {
		slog.WarnContext(ctx, "certificate template: task not found in consignment; leaving field unset",
			"taskCode", taskCode, "field", field)
		return ""
	}
	source := app.Data
	if review {
		source = app.AgencyActionData
	}
	v, _ := source[field].(string)
	return v
}

// indexApplicationsByTaskCode discovers every application for consignmentID
// and indexes its full record (Data and AgencyActionData) by TaskCode. The
// list call only reveals which tasks exist in the consignment; each one's
// full record is then fetched individually, since list results carry neither
// Data nor AgencyActionData.
func indexApplicationsByTaskCode(ctx context.Context, applications ApplicationLookup, consignmentID string) map[string]application.Application {
	byTaskCode := map[string]application.Application{}
	if applications == nil || consignmentID == "" {
		return byTaskCode
	}

	page, err := applications.GetApplications(ctx, "", consignmentID, "", 1, 100)
	if err != nil {
		slog.WarnContext(ctx, "failed to list consignment applications for certificate generation",
			"consignmentId", consignmentID, "error", err)
		return byTaskCode
	}
	for _, summary := range page.Items {
		app, err := applications.GetApplication(ctx, summary.TaskID)
		if err != nil {
			slog.WarnContext(ctx, "failed to fetch application for certificate generation",
				"taskId", summary.TaskID, "taskCode", summary.TaskCode, "error", err)
			continue
		}
		byTaskCode[app.TaskCode] = *app
	}
	return byTaskCode
}
