package main

import (
	"context"

	"github.com/OpenNSW/agency/backend/internal/authn"
)

// userStore is the slice of user.UserStore the adapter needs. Declaring it here
// rather than taking the concrete type keeps the adapter testable without a
// database.
type userStore interface {
	GetOrCreateUser(idpUserID, email, givenName string) (*string, error)
}

// userProfileAdapter adapts user.UserStore to authn.UserProfileService,
// spreading the identity fields the store persists across the named parameters
// it takes. Keeps internal/user free of any dependency on the authentication
// layer's types.
//
// The principal's remaining claims stop here: the OU gate is internal/authn's,
// and the store has no column for a phone number.
type userProfileAdapter struct {
	store userStore
}

func (a *userProfileAdapter) GetOrCreateUser(_ context.Context, p *authn.Principal) (string, error) {
	userID, err := a.store.GetOrCreateUser(p.IDPUserID, p.Email, p.GivenName)
	if err != nil {
		return "", err
	}
	if userID == nil {
		return "", nil
	}
	return *userID, nil
}
