package main

import (
	"context"

	"github.com/OpenNSW/nsw-agency/backend/internal/authn"
	"github.com/OpenNSW/nsw-agency/backend/internal/user"
)

// userProfileAdapter adapts user.UserStore to authn.UserProfileService,
// spreading the authenticated principal's identity fields across the named
// parameters the store already takes. Keeps internal/user free of any
// dependency on the authentication layer's types.
type userProfileAdapter struct {
	store *user.UserStore
}

func (a *userProfileAdapter) GetOrCreateUser(_ context.Context, p *authn.Principal) (string, error) {
	userID, err := a.store.GetOrCreateUser(p.IDPUserID, p.Email, p.GivenName, p.PhoneNumber, p.OUID, p.OUHandle)
	if err != nil {
		return "", err
	}
	if userID == nil {
		return "", nil
	}
	return *userID, nil
}
