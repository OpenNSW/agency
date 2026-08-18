package main

import (
	"context"

	"github.com/OpenNSW/nsw-agency/backend/internal/authn"
)

// userStore is the slice of user.UserStore the adapter needs. Declaring it here
// rather than taking the concrete type keeps the adapter testable without a
// database.
type userStore interface {
	GetOrCreateUser(idpUserID, email, givenName, phone, organizationID, ouHandle string) (*string, error)
}

// userProfileAdapter adapts user.UserStore to authn.UserProfileService,
// spreading the authenticated principal's identity fields across the named
// parameters the store already takes. Keeps internal/user free of any
// dependency on the authentication layer's types.
type userProfileAdapter struct {
	store userStore
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
