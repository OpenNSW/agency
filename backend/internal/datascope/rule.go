// Package datascope restricts which consignments (and, transitively,
// applications) an officer may see, by comparing their own users.custom_data
// against a consignment's consignments.custom_data — e.g. an officer whose
// custom data says district "Colombo" only sees consignments whose custom
// data agrees. See Resolver for the per-request policy, and docs/data-scoping.md
// for the deployer-facing configuration format.
package datascope

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/OpenNSW/agency/backend/pkg/jsonpointer"
)

// ErrInvalidRules is returned when a deployer's configured rules file fails
// validation.
var ErrInvalidRules = fmt.Errorf("datascope: invalid rules")

// segmentPattern restricts pointer segments beyond what jsonpointer.Valid
// requires, mirroring pkg/jsonquery's own allowlist: these pointers end up
// spliced directly into SQL path syntax, not bound as parameters, so an
// author is limited to safe identifier characters rather than full RFC 6901
// generality.
var segmentPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Rule declares one attribute-equality constraint restricting consignment
// (and application) visibility: the value at ConsignmentField (a JSON
// Pointer into consignments.custom_data) must equal the value at UserField
// (a JSON Pointer into the requesting officer's own users.custom_data).
// Multiple configured rules AND together — see Resolver.
type Rule struct {
	ConsignmentField string `json:"consignmentField"`
	UserField        string `json:"userField"`
}

// rulesDocument is the on-disk shape ParseRules reads: an object with a
// top-level "rules" array, not a bare array — so the format can gain a
// sibling field later (e.g. a version marker) without a breaking change,
// consistent with every other config/artifact shape in this codebase (task
// configs, schemas) always being an object at the root.
type rulesDocument struct {
	Rules []Rule `json:"rules"`
}

// ParseRules parses and validates a deployer's rules file (see
// rulesDocument). Each pointer must be valid RFC 6901 syntax whose segments
// are all restricted to a safe identifier charset (see segmentPattern), and
// no two rules may target the same ConsignmentField — a second rule
// targeting an already-used ConsignmentField would silently clobber the
// first's resolved value, which is a config bug to catch at load time, not a
// supported override mechanism.
func ParseRules(raw []byte) ([]Rule, error) {
	var doc rulesDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRules, err)
	}
	rules := doc.Rules
	// A nil Rules field covers the document being null, "rules" being
	// explicitly null, or "rules" being absent entirely — all three leave a
	// []Rule field at its nil zero value, since json.Unmarshal only ever
	// produces a non-nil empty slice for an explicit "[]". Rejecting nil
	// here means a deployer who intends to configure this feature but gets
	// the file wrong (missing key, null) gets a startup failure telling them
	// so, rather than scoping silently disabled.
	if rules == nil {
		return nil, fmt.Errorf("%w: document must have a non-null \"rules\" array (use [] for no rules)", ErrInvalidRules)
	}

	seen := make(map[string]bool, len(rules))
	for i, r := range rules {
		if err := validatePointer(r.ConsignmentField); err != nil {
			return nil, fmt.Errorf("%w: rule[%d].consignmentField: %v", ErrInvalidRules, i, err)
		}
		if err := validatePointer(r.UserField); err != nil {
			return nil, fmt.Errorf("%w: rule[%d].userField: %v", ErrInvalidRules, i, err)
		}
		if seen[r.ConsignmentField] {
			return nil, fmt.Errorf("%w: rule[%d]: consignmentField %q is already targeted by an earlier rule", ErrInvalidRules, i, r.ConsignmentField)
		}
		seen[r.ConsignmentField] = true
	}
	return rules, nil
}

func validatePointer(pointer string) error {
	segments, err := jsonpointer.Segments(pointer)
	if err != nil {
		return err
	}
	for _, seg := range segments {
		if !segmentPattern.MatchString(seg) {
			return fmt.Errorf("segment %q is not allowed (must match %s)", seg, segmentPattern.String())
		}
	}
	return nil
}
