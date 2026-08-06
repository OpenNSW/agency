package logging

import (
	"io"
	"log/slog"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/trace"
	"github.com/OpenNSW/core/trace/logging"
)

// configureLogging installs a trace-aware slog handler writing to dest as the
// process default, and wires httputil's correlation-ID hook to
// trace.GetTraceID, so the correlationId returned in error responses matches
// the traceId written to server logs. Must be called before any request is
// served.
func ConfigureLogging(dest io.Writer) {
	slog.SetDefault(slog.New(logging.NewHandler(slog.NewJSONHandler(dest, nil))))
	httputil.CorrelationIDFunc = trace.GetTraceID
}
