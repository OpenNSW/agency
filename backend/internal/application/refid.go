package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/OpenNSW/agency/backend/internal/taskconfig"
	"github.com/OpenNSW/agency/backend/pkg/jsonpointer"
	"github.com/OpenNSW/core/refid"
)

// generateRefID mints this task's reference ID and returns the reviewer
// response document to store it in, with the ID written at cfg.Path.
//
// A params pointer that doesn't resolve to a string is skipped rather than
// rejected: refid ignores params the configured format doesn't consume, so a
// task may declare more than any one format needs. Whether an absent value
// matters is refid's call — it returns ErrInvalidParam for a param a segment
// requires, and for an unresolved scope-key placeholder.
//
// ErrInvalidParam maps to a 400; every other failure stays unwrapped and
// surfaces as a 500.
func generateRefID(ctx context.Context, reg refid.Registry, cfg *taskconfig.TaskRefID, data map[string]any) (JSONB, error) {
	params := make(map[string]string, len(cfg.Params))
	for param, pointer := range cfg.Params {
		value, ok := jsonpointer.Get(data, pointer)
		if !ok {
			continue
		}
		if str, ok := value.(string); ok {
			params[param] = str
		}
	}

	id, err := reg.Generate(ctx, cfg.Issuer, cfg.IDType, params)
	if err != nil {
		if errors.Is(err, refid.ErrInvalidParam) {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInjectRequest, err)
		}
		return nil, fmt.Errorf("failed to generate reference ID for issuer %q idType %q: %w", cfg.Issuer, cfg.IDType, err)
	}

	reviewerResponse := JSONB{}
	if !jsonpointer.Set(reviewerResponse, cfg.Path, id) {
		// Unreachable — the document is empty and Validate already checked
		// Path. Still an error: dropping an issued ID would be silent loss.
		return nil, fmt.Errorf("failed to write reference ID to %q", cfg.Path)
	}
	return reviewerResponse, nil
}
