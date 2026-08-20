package authn

import "context"

// UserProfileService resolves an authenticated caller to a persisted user
// record. It is optional — pass nil to NewManager to skip user resolution, in
// which case Principal.UserID stays empty and the middleware does not enforce
// that the caller is a known user.
//
// Taking a *Principal rather than a list of named identity values means adding
// a claim never changes this signature. Implementations must not mutate the
// principal.
type UserProfileService interface {
	// GetOrCreateUser returns the persisted user ID for p. An error, or an
	// empty ID, means the caller is not a user of this service and the request
	// is rejected with 403 — see Manager.RequireAuthMiddleware.
	GetOrCreateUser(ctx context.Context, p *Principal) (string, error)
}
