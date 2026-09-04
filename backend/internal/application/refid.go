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
// Unlike resolvePushedFields, an unresolved pointer here is an error rather
// than a silent skip: a params entry the configured format requires is the
// difference between a correct ID and none at all, and CreateApplication
// fails the whole inject rather than persisting an application without one.
//
// Param resolution errors and refid.ErrInvalidParam wrap
// ErrInvalidInjectRequest (a 400 — the injected data couldn't supply a value
// the format needs). Everything else — an issuer/idType this deployment
// hasn't configured, counter overflow, a database failure — stays unwrapped
// and surfaces as a 500, since those are deployment or infrastructure faults
// rather than anything wrong with the request.
func generateRefID(ctx context.Context, reg refid.Registry, cfg *taskconfig.TaskRefID, data map[string]any) (JSONB, error) {
	params := make(map[string]string, len(cfg.Params))
	for param, pointer := range cfg.Params {
		value, ok := jsonpointer.Get(data, pointer)
		if !ok {
			return nil, fmt.Errorf("%w: refid param %q: injected data has no value at %q", ErrInvalidInjectRequest, param, pointer)
		}
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%w: refid param %q: value at %q must be a string, got %T", ErrInvalidInjectRequest, param, pointer, value)
		}
		params[param] = str
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
		// Unreachable: the document is empty and Validate already checked
		// Path is a well-formed pointer. Still an error rather than a
		// discard — losing an already-issued ID would be silent corruption.
		return nil, fmt.Errorf("failed to write reference ID to %q", cfg.Path)
	}
	return reviewerResponse, nil
}
