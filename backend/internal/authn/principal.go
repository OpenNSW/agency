// Package authn is this service's boundary onto the shared authentication
// library, github.com/OpenNSW/core/authn. It is the only package that imports
// it; handlers, domain services and the composition root depend on the
// first-party types declared here instead.
//
// It exists because this agency needs two things the shared library
// deliberately does not provide: every user token must belong to this agency's
// OU (see Config.ExpectedOU and Manager.RequireAuthMiddleware), and the caller
// must resolve to a seeded user record. Keeping both here means the agency's
// access policy lives in one file rather than spread across handlers, and the
// shared library stays generic.
//
// It also owns the IdP claim vocabulary: the claim names this deployment reads
// are unexported constants here, declared to core/authn in config.go and
// flattened onto Principal, so no caller ever spells a claim name.
package authn

import "context"

// IdP claim names read by this deployment. Adding one means declaring it in
// Config.coreConfig — an undeclared claim is never extracted and silently
// reads as empty — and surfacing it on Principal.
const (
	claimEmail       = "email"
	claimGivenName   = "given_name"
	claimPhoneNumber = "phone_number"
	claimOUID        = "ouId"
	claimOUHandle    = "ouHandle"
)

// Kind distinguishes a human caller from a machine (M2M) client.
type Kind string

const (
	KindUser   Kind = "user"
	KindClient Kind = "client"
)

// Principal is this service's view of the authenticated caller for one request.
// Identity claims are flattened onto named fields so callers depend on this
// struct rather than on the IdP's claim spellings or on the shared library's
// context shape.
//
// Fields not applicable to a Kind are zero: a client principal has no OUHandle,
// a user principal has no ClientID.
//
// Note there is deliberately no Subject() accessor. UserID is the key RBAC uses
// for role lookups, and an accessor that fell back to IDPUserID when UserID was
// empty would silently resolve roles against the wrong identifier.
type Principal struct {
	Kind Kind

	// UserID is the internally persisted user ID resolved by
	// UserProfileService, and the identifier RBAC keys role lookups on. Empty
	// for client principals.
	UserID string
	// IDPUserID is the identity provider's ID for the user (the JWT "sub"
	// claim), which is not the same as UserID.
	IDPUserID string
	// ClientID identifies a machine client. Empty for user principals.
	ClientID string

	Roles  []string
	Scopes []string

	Email       string
	GivenName   string
	PhoneNumber string
	OUID        string
	OUHandle    string
}

// contextKey is unexported so a Principal can only reach a request context via
// ContextWithPrincipal, keeping this package the single writer.
type contextKey struct{}

// FromContext returns the Principal attached by the authentication middleware.
// ok is false for an unauthenticated request — a public route, or middleware
// that was never applied.
func FromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(*Principal)
	if !ok || p == nil {
		return nil, false
	}
	return p, true
}

// ContextWithPrincipal attaches p to ctx. Production code should not call this
// directly — Manager.RequireAuthMiddleware does. It is exported for tests that
// need an authenticated request without minting a real token.
func ContextWithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}
