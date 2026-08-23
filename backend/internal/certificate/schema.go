package certificate

import (
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

// validateAgainstSchema checks data against the JSON Schema in rawSchema. A
// nil/empty rawSchema means the task declares no schema, so nothing is
// enforced.
func validateAgainstSchema(rawSchema json.RawMessage, data map[string]any) error {
	if len(rawSchema) == 0 {
		return nil
	}

	var sch jsonschema.Schema
	if err := json.Unmarshal(rawSchema, &sch); err != nil {
		return fmt.Errorf("parse certificate data schema: %w", err)
	}
	resolved, err := sch.Resolve(nil)
	if err != nil {
		return fmt.Errorf("resolve certificate data schema: %w", err)
	}

	instance := data
	if instance == nil {
		instance = map[string]any{}
	}
	return resolved.Validate(instance)
}
