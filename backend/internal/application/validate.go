package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OpenNSW/agency/backend/pkg/jsonschemautil"
	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/generictemplate"
)

// validateAgainstViewForm validates injected data against the task's view
// form schema (the same form later attached to the application as
// DataForm). A load, parse, or resolve failure is returned unwrapped so the
// caller treats it as an internal/config error (fail closed on config
// drift); a schema mismatch is wrapped in ErrInvalidInjectRequest since it
// reflects bad caller data rather than a broken config.
func validateAgainstViewForm(ctx context.Context, reg *artifact.Registry, formID string, data map[string]any) error {
	raw, err := generictemplate.Load(ctx, reg, formID)
	if err != nil {
		return fmt.Errorf("failed to load view form %q: %w", formID, err)
	}

	var form struct {
		Schema json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal(raw, &form); err != nil {
		return fmt.Errorf("failed to parse view form %q: %w", formID, err)
	}

	if err := jsonschemautil.ValidateInstance(form.Schema, data); err != nil {
		if errors.Is(err, jsonschemautil.ErrSchemaLoad) {
			return fmt.Errorf("failed to load view form %q schema: %w", formID, err)
		}
		return fmt.Errorf("%w: data does not match view form %q schema: %v", ErrInvalidInjectRequest, formID, err)
	}
	return nil
}
