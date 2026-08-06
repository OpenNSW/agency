package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/trace"
)

func TestConfigureLogging_ErrorResponseCorrelationIDMatchesTraceID(t *testing.T) {
	// The correlationId returned to a client must equal the traceId in server
	// logs, or a client-reported ID can't be looked up server-side.
	oldLogger := slog.Default()
	oldCorrelationIDFunc := httputil.CorrelationIDFunc
	t.Cleanup(func() {
		slog.SetDefault(oldLogger)
		httputil.CorrelationIDFunc = oldCorrelationIDFunc
	})

	var logOutput bytes.Buffer
	ConfigureLogging(&logOutput)

	handler := trace.TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httputil.InternalServerError(w, r, "handling request failed", errors.New("boom"))
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))

	var resp httputil.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.CorrelationID == "" {
		t.Fatal("expected a non-empty correlationId in the error response")
	}

	var logLine map[string]any
	if err := json.Unmarshal(logOutput.Bytes(), &logLine); err != nil {
		t.Fatalf("failed to decode log line: %v (raw: %s)", err, logOutput.String())
	}
	traceID, _ := logLine["traceId"].(string)
	if traceID == "" {
		t.Fatal("expected server log line to contain a traceId")
	}

	if resp.CorrelationID != traceID {
		t.Fatalf("correlationId %q does not match server log traceId %q", resp.CorrelationID, traceID)
	}
}
