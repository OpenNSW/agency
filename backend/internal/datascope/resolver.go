package datascope

import (
	"context"
	"fmt"

	"github.com/OpenNSW/agency/backend/internal/authn"
	"github.com/OpenNSW/agency/backend/pkg/jsonpointer"
)

// UserAttributes resolves an officer's own agency-specific custom data,
// keyed by their internal user ID. Implemented by internal/user.UserStore.
type UserAttributes interface {
	GetCustomData(ctx context.Context, userID string) (map[string]any, error)
}

// Resolution is the outcome of resolving a Resolver's configured rules
// against one request's caller.
type Resolution struct {
	// Unrestricted is true when no scoping applies to this request at all:
	// no rules are configured, or the caller isn't a human user principal
	// (e.g. a machine/M2M client). Filter and Satisfiable are meaningless
	// when this is true.
	Unrestricted bool

	// Satisfiable is false when at least one configured rule's UserField
	// didn't resolve on this officer (attribute never set on them). This is
	// this package's fail-closed behavior: a caller must treat this as "the
	// officer can see nothing for this request" and skip querying entirely,
	// never fall through to an unfiltered query.
	Satisfiable bool

	// Filter maps each rule's ConsignmentField pointer to the value it must
	// equal, resolved against this officer's own custom data. Only
	// meaningful when Satisfiable is true and Unrestricted is false.
	Filter map[string]any
}

// Resolver turns a deployer's configured Rules, together with the current
// request's caller, into a concrete Resolution.
type Resolver struct {
	rules []Rule
	attrs UserAttributes
}

// NewResolver creates a Resolver. rules may be nil/empty, in which case
// every Resolve call returns Unrestricted — the no-op state used when a
// deployment hasn't configured this feature at all.
func NewResolver(rules []Rule, attrs UserAttributes) *Resolver {
	return &Resolver{rules: rules, attrs: attrs}
}

// Resolve resolves r's configured rules against ctx's caller.
//
// A genuine error from the UserAttributes lookup (an infra failure, not a
// policy outcome) is returned as-is — callers must not treat it the same as
// Satisfiable=false, and should propagate it as a normal error (e.g. a 500),
// not silently empty the result set.
func (r *Resolver) Resolve(ctx context.Context) (Resolution, error) {
	if len(r.rules) == 0 {
		return Resolution{Unrestricted: true}, nil
	}

	principal, ok := authn.FromContext(ctx)
	if !ok || principal.Kind != authn.KindUser {
		return Resolution{Unrestricted: true}, nil
	}

	attrs, err := r.attrs.GetCustomData(ctx, principal.UserID)
	if err != nil {
		return Resolution{}, fmt.Errorf("datascope: failed to resolve caller's attributes: %w", err)
	}

	filter := make(map[string]any, len(r.rules))
	for _, rule := range r.rules {
		value, ok := jsonpointer.Get(attrs, rule.UserField)
		if !ok {
			return Resolution{Satisfiable: false}, nil
		}
		filter[rule.ConsignmentField] = value
	}

	return Resolution{Satisfiable: true, Filter: filter}, nil
}
