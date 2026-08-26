package main

import "github.com/OpenNSW/agency/backend/internal/authn"

// authzPrincipal adapts *authn.Principal to core/authz.Principal so the
// generic authz package can gate routes on OAuth2 scopes without importing
// internal/authn. It exists here, in the composition root, rather than as a
// method on authn.Principal itself, because authn.Principal deliberately has
// no Subject() accessor (see its doc comment): RBAC must key off UserID
// directly with no fallback, and a shared Subject() risked being reused for
// that. Subject() here is used only for authz's own logging, not for role
// lookups.
type authzPrincipal struct {
	p *authn.Principal
}

func (a authzPrincipal) Subject() string {
	if a.p.Kind == authn.KindUser {
		return a.p.UserID
	}
	return a.p.ClientID
}

func (a authzPrincipal) Roles() []string  { return a.p.Roles }
func (a authzPrincipal) Scopes() []string { return a.p.Scopes }
