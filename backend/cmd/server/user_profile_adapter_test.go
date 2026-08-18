package main

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/nsw-agency/backend/internal/authn"
)

// fakeUserStore returns whatever the test dictates and records the arguments it
// was handed.
type fakeUserStore struct {
	returnID  *string
	returnErr error
	args      []string
}

func (f *fakeUserStore) GetOrCreateUser(idpUserID, email, givenName string) (*string, error) {
	f.args = []string{idpUserID, email, givenName}
	return f.returnID, f.returnErr
}

func principal() *authn.Principal {
	return &authn.Principal{
		Kind:        authn.KindUser,
		IDPUserID:   "sub-001",
		Email:       "officer@fcau.gov",
		GivenName:   "Alice",
		PhoneNumber: "+94770000000",
		OUID:        "ou-123",
		OUHandle:    "fcau",
	}
}

// The store's errors reach authn unchanged; authn treats any of them, like an
// empty ID, as "not a user of this agency" and answers 403.
func TestUserProfileAdapter_OtherErrorsPropagate(t *testing.T) {
	outage := errors.New("failed to query user: connection refused")
	a := &userProfileAdapter{store: &fakeUserStore{returnErr: outage}}

	id, err := a.GetOrCreateUser(context.Background(), principal())

	if !errors.Is(err, outage) {
		t.Fatalf("err = %v, want %v", err, outage)
	}
	if id != "" {
		t.Fatalf("id = %q, want empty on error", id)
	}
}

func TestUserProfileAdapter_ResolvesUserID(t *testing.T) {
	want := "user-1"
	store := &fakeUserStore{returnID: &want}
	a := &userProfileAdapter{store: store}

	id, err := a.GetOrCreateUser(context.Background(), principal())

	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	if id != want {
		t.Fatalf("id = %q, want %q", id, want)
	}
	// The store takes three positional strings, so a mis-ordered argument would
	// otherwise pass silently — writing the given name into the email, for
	// instance. principal() carries a phone number and both OU claims too:
	// those stop here, so the store must see exactly these three.
	wantArgs := []string{"sub-001", "officer@fcau.gov", "Alice"}
	if len(store.args) != len(wantArgs) {
		t.Fatalf("store saw %d arguments, want %d: %q", len(store.args), len(wantArgs), store.args)
	}
	for i, arg := range wantArgs {
		if store.args[i] != arg {
			t.Errorf("arg %d = %q, want %q", i, store.args[i], arg)
		}
	}
}

// The store's contract permits (nil, nil); treat it as a denial rather than
// dereferencing it.
func TestUserProfileAdapter_NilIDBecomesEmptyID(t *testing.T) {
	a := &userProfileAdapter{store: &fakeUserStore{}}

	id, err := a.GetOrCreateUser(context.Background(), principal())

	if err != nil || id != "" {
		t.Fatalf("id = %q, err = %v; want empty id and nil error", id, err)
	}
}
