// Package dbtype holds small database column types shared across domain
// packages.
package dbtype

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONB is a custom type for storing arbitrary JSON data in a database
// column. It marshals to JSON bytes on write, and on read accepts either
// []byte or string from the driver (covering both a text-affinity column,
// e.g. SQLite, and a native jsonb column, e.g. Postgres).
type JSONB map[string]any

// Value implements the driver.Valuer interface.
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface.
func (j *JSONB) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to unmarshal JSONB value: %v", value)
	}

	// Unmarshal into a fresh map and replace *j wholesale: json.Unmarshal into
	// an existing non-nil map merges keys rather than clearing it first, so
	// unmarshaling straight into *j would leave stale keys from a previous
	// Scan behind.
	var decoded JSONB
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		return err
	}
	*j = decoded
	return nil
}
