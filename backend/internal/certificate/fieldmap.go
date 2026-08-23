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
// is used to find the (at most one) application in a consignment with a given
// TaskCode; GetApplication then fetches that task's full record, since list
// results carry neither Data nor AgencyActionData.
type ApplicationLookup interface {
	GetApplications(ctx context.Context, status, consignmentID, taskCode, search string, page, pageSize int) (*httputil.PagedResponse[application.Application], error)
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

// realFuncs builds the certificate template functions for one Generate call.
// fromData/fromReview resolve a taskCode within consignmentID on first
// reference, via a targeted lookup rather than listing the whole consignment,
// and cache the result so a taskCode referenced more than once (e.g. once via
// fromData and once via fromReview) is only fetched once.
func realFuncs(ctx context.Context, applications ApplicationLookup, consignmentID string) template.FuncMap {
	cache := map[string]*application.Application{}
	resolve := func(taskCode string) *application.Application {
		if app, ok := cache[taskCode]; ok {
			return app
		}
		app := lookupApplicationByTaskCode(ctx, applications, consignmentID, taskCode)
		cache[taskCode] = app
		return app
	}

	return template.FuncMap{
		"fromData": func(taskCode, field string) string {
			return fieldValue(resolve(taskCode), field, false)
		},
		"fromReview": func(taskCode, field string) string {
			return fieldValue(resolve(taskCode), field, true)
		},
		"today": func() string {
			return time.Now().Format(certificateDateFormat)
		},
	}
}

// fieldValue reads field off app.Data (or app.AgencyActionData, when review
// is true). It never fails on missing data — a certificate preview should
// still render with whatever is available — so a nil app or missing field
// simply yields an empty string.
func fieldValue(app *application.Application, field string, review bool) string {
	if app == nil {
		return ""
	}
	source := app.Data
	if review {
		source = app.AgencyActionData
	}
	v, _ := source[field].(string)
	return v
}

// lookupApplicationByTaskCode finds the application within consignmentID
// whose TaskCode is taskCode and fetches its full record (Data and
// AgencyActionData). It never fails on a miss — a certificate preview should
// still render with whatever is available — so a missing task or a lookup
// error is logged and nil is returned, not treated as an error.
func lookupApplicationByTaskCode(ctx context.Context, applications ApplicationLookup, consignmentID, taskCode string) *application.Application {
	if applications == nil || consignmentID == "" {
		return nil
	}

	page, err := applications.GetApplications(ctx, "", consignmentID, taskCode, "", 1, 1)
	if err != nil {
		slog.WarnContext(ctx, "failed to look up task for certificate generation",
			"consignmentId", consignmentID, "taskCode", taskCode, "error", err)
		return nil
	}
	if len(page.Items) == 0 {
		slog.WarnContext(ctx, "certificate template: task not found in consignment; leaving field unset",
			"consignmentId", consignmentID, "taskCode", taskCode)
		return nil
	}

	taskID := page.Items[0].TaskID
	app, err := applications.GetApplication(ctx, taskID)
	if err != nil {
		slog.WarnContext(ctx, "failed to fetch application for certificate generation",
			"taskId", taskID, "taskCode", taskCode, "error", err)
		return nil
	}
	return app
}
