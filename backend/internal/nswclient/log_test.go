package nswclient

import (
	"bytes"
	"log/slog"
	"testing"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	return &output
}
