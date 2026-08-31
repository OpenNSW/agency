// Package jsonquery builds dialect-appropriate SQL predicates for testing
// equality against a value nested inside a JSON/JSONB column, given an RFC
// 6901 JSON Pointer (see pkg/jsonpointer) into that column.
package jsonquery

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/OpenNSW/agency/backend/pkg/jsonpointer"
)

// segmentPattern restricts pointer segments beyond what jsonpointer.Valid
// requires: RFC 6901 permits arbitrary characters in a segment (escaped via
// "~0"/"~1"), but here segments are spliced directly into SQL path syntax
// ("{a,b}" for Postgres, "$.a.b" for SQLite) rather than bound as a
// parameter — neither dialect's JSON path operators accept a bound parameter
// for the path itself, only for the compared value. Rejecting anything
// outside this allowlist closes that off entirely rather than trying to
// escape/quote arbitrary segments per dialect.
var segmentPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Predicate returns a SQL WHERE fragment (with a single "?" placeholder) and
// its bind argument, asserting that column's JSON value at pointer equals
// value. driver is "postgres" or "sqlite" (as returned by
// gorm.DB.Name(), promoted from the embedded Dialector).
//
// Every pointer segment is validated against segmentPattern here, even
// though callers (see internal/datascope.ParseRules) are expected to have
// already validated it at config-load time — this is the function actually
// vulnerable to injection if that expectation is ever violated, so it does
// not trust its caller blindly.
func Predicate(driver, column, pointer string, value any) (sql string, arg any, err error) {
	segments, err := jsonpointer.Segments(pointer)
	if err != nil {
		return "", nil, err
	}
	for _, seg := range segments {
		if !segmentPattern.MatchString(seg) {
			return "", nil, fmt.Errorf("jsonquery: pointer segment %q is not allowed (must match %s)", seg, segmentPattern.String())
		}
	}

	switch driver {
	case "postgres":
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", nil, fmt.Errorf("jsonquery: failed to encode value: %w", err)
		}
		// Compared as jsonb (not extracted as text via #>>) so string,
		// number, and boolean values all compare correctly without a
		// separate text-coercion step.
		path := "{" + strings.Join(segments, ",") + "}"
		return column + "#>'" + path + "' = ?::jsonb", string(encoded), nil
	case "sqlite":
		// json_extract already returns the value typed as a native SQLite
		// type, so it compares directly against the same Go scalar value
		// gorm binds — no casting needed.
		path := "$." + strings.Join(segments, ".")
		return "json_extract(" + column + ", '" + path + "') = ?", value, nil
	default:
		return "", nil, fmt.Errorf("jsonquery: unsupported driver %q", driver)
	}
}
