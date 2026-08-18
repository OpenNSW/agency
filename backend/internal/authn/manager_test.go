package authn

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func newManager(t *testing.T, profiles UserProfileService) *Manager {
	t.Helper()
	key := generateKey(t)
	srv := jwksServer(t, key)
	m, err := NewManager(profiles, validConfig(srv.URL))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

// newManagerWithKey is newManager for tests that also need to mint tokens.
func newManagerWithKey(t *testing.T, profiles UserProfileService) (*Manager, *keyring) {
	t.Helper()
	key := generateKey(t)
	srv := jwksServer(t, key)
	m, err := NewManager(profiles, validConfig(srv.URL))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, &keyring{t: t, key: key}
}

func TestNewManager_InvalidConfigFails(t *testing.T) {
	// core/authn's own constructor does not validate, so this asserts the
	// validation this package adds back.
	if _, err := NewManager(nil, Config{}); err == nil {
		t.Fatal("expected an error for an empty config")
	}
}

func TestNewManager_NilUserProfileServiceIsAllowed(t *testing.T) {
	m := newManager(t, nil)
	if m == nil {
		t.Fatal("expected a manager")
	}
}

// The OU gate is this package's reason for existing: a user token from another
// agency must be rejected outright.
func TestRequireAuth_WrongOU_Returns403(t *testing.T) {
	profiles := &stubProfiles{returnID: "user-1"}
	m, ring := newManagerWithKey(t, profiles)

	status, body, reached, _ := serve(t, m, ring.sign(userClaims("npqs")))

	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
	if body != forbiddenBody {
		t.Fatalf("body = %q, want %q", body, forbiddenBody)
	}
	if reached {
		t.Fatal("handler must not run for a cross-agency token")
	}
	// The whole point of gating before resolution: a foreign token never
	// reaches the database.
	if profiles.calls != 0 {
		t.Fatalf("user profile service called %d times; a cross-agency token must be rejected first", profiles.calls)
	}
}

func TestRequireAuth_MatchingOU_Passes(t *testing.T) {
	profiles := &stubProfiles{returnID: "user-1"}
	m, ring := newManagerWithKey(t, profiles)

	status, _, reached, seen := serve(t, m, ring.sign(userClaims(testOU)))

	if status != http.StatusOK || !reached {
		t.Fatalf("status = %d, reached = %v; want 200 and handler reached", status, reached)
	}
	if seen == nil || seen.UserID != "user-1" {
		t.Fatalf("principal = %#v, want UserID user-1", seen)
	}
	if seen.Kind != KindUser {
		t.Fatalf("kind = %q, want %q", seen.Kind, KindUser)
	}
}

// A caller the profile service cannot resolve is not a user of this agency.
func TestRequireAuth_ProfileResolutionFails_Returns403(t *testing.T) {
	profiles := &stubProfiles{returnErr: errors.New("user does not belong to this agency")}
	m, ring := newManagerWithKey(t, profiles)

	status, body, reached, _ := serve(t, m, ring.sign(userClaims(testOU)))

	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
	if body != forbiddenBody {
		t.Fatalf("body = %q, want %q", body, forbiddenBody)
	}
	if reached {
		t.Fatal("handler must not run when the user could not be resolved")
	}
}

// An empty ID with no error is the same signal as an error: the caller is not a
// known user of this agency.
func TestRequireAuth_ProfileReturnsEmptyID_Returns403(t *testing.T) {
	m, ring := newManagerWithKey(t, &stubProfiles{returnID: ""})

	status, body, reached, _ := serve(t, m, ring.sign(userClaims(testOU)))

	if status != http.StatusForbidden || reached {
		t.Fatalf("status = %d, reached = %v; want 403 and handler not reached", status, reached)
	}
	if body != forbiddenBody {
		t.Fatalf("body = %q, want %q", body, forbiddenBody)
	}
}

// Without a profile service there is nothing to resolve against, so the
// resolution gate must not fire — otherwise every request would 403.
func TestRequireAuth_NilProfileService_Passes(t *testing.T) {
	m, ring := newManagerWithKey(t, nil)

	status, _, reached, seen := serve(t, m, ring.sign(userClaims(testOU)))

	if status != http.StatusOK || !reached {
		t.Fatalf("status = %d, reached = %v; want 200 and handler reached", status, reached)
	}
	if seen.UserID != "" {
		t.Fatalf("UserID = %q, want empty without a profile service", seen.UserID)
	}
}

// Machine clients carry no OU and no user record; both gates must skip them.
func TestRequireAuth_ClientToken_BypassesUserGates(t *testing.T) {
	profiles := &stubProfiles{returnErr: errors.New("must not be called")}
	m, ring := newManagerWithKey(t, profiles)

	status, _, reached, seen := serve(t, m, ring.sign(clientClaims()))

	if status != http.StatusOK || !reached {
		t.Fatalf("status = %d, reached = %v; want 200 and handler reached", status, reached)
	}
	if seen.Kind != KindClient || seen.ClientID != testClientID {
		t.Fatalf("principal = %#v, want a client principal for %s", seen, testClientID)
	}
	if profiles.calls != 0 {
		t.Fatal("user profile service must not be called for a client token")
	}
}

func TestRequireAuth_NoToken_Returns401(t *testing.T) {
	m := newManager(t, &stubProfiles{returnID: "user-1"})

	status, _, reached, _ := serve(t, m, "")

	if status != http.StatusUnauthorized || reached {
		t.Fatalf("status = %d, reached = %v; want 401 and handler not reached", status, reached)
	}
}

// Every other test here signs a fully valid token, so a Config field mapped
// into the wrong core/authn slot — the issuer into the audience, say, or
// ClientIDs dropped altogether — would leave the allowlist and the issuer check
// looking like they work while admitting anyone. These are the negative cases
// that notice. The rules themselves belong to core/authn; what is under test is
// coreConfig's wiring of them.
func TestRequireAuth_RejectsTokensCoreConfigMustReject(t *testing.T) {
	for _, tc := range []struct {
		name  string
		claim string
		value any
	}{
		{"wrong issuer", "iss", "https://attacker.example.com"},
		{"wrong audience", "aud", "OTHER_API"},
		{"unlisted client_id", "client_id", "SOME_OTHER_APP"},
		{"expired token", "exp", time.Now().Add(-time.Hour).Unix()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profiles := &stubProfiles{returnID: "user-1"}
			m, ring := newManagerWithKey(t, profiles)

			claims := userClaims(testOU)
			claims[tc.claim] = tc.value

			status, _, reached, _ := serve(t, m, ring.sign(claims))

			if status != http.StatusUnauthorized || reached {
				t.Fatalf("status = %d, reached = %v; want 401 and handler not reached", status, reached)
			}
			if profiles.calls != 0 {
				t.Fatalf("user profile service called %d times; a rejected token must not reach the database", profiles.calls)
			}
		})
	}
}

// email, ouId and ouHandle are declared Required, so a token missing any of
// them is rejected before this package's own gates run.
func TestRequireAuth_MissingRequiredClaim_Returns401(t *testing.T) {
	for _, claim := range []string{"email", "ouId", "ouHandle"} {
		t.Run("missing "+claim, func(t *testing.T) {
			m, ring := newManagerWithKey(t, &stubProfiles{returnID: "user-1"})

			claims := userClaims(testOU)
			delete(claims, claim)

			status, _, reached, _ := serve(t, m, ring.sign(claims))

			if status != http.StatusUnauthorized || reached {
				t.Fatalf("status = %d, reached = %v; want 401 and handler not reached", status, reached)
			}
		})
	}
}

// given_name is optional: it must reach the principal when present, and its
// absence must not reject the token.
func TestRequireAuth_GivenNameIsOptional(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		m, ring := newManagerWithKey(t, &stubProfiles{returnID: "user-1"})

		claims := userClaims(testOU)
		claims["given_name"] = "Alice"

		_, _, _, seen := serve(t, m, ring.sign(claims))

		if seen == nil {
			t.Fatal("handler saw no principal")
		}
		if seen.GivenName != "Alice" {
			t.Fatalf("GivenName = %q, want Alice", seen.GivenName)
		}
	})

	t.Run("absent", func(t *testing.T) {
		m, ring := newManagerWithKey(t, &stubProfiles{returnID: "user-1"})

		status, _, _, seen := serve(t, m, ring.sign(userClaims(testOU)))

		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200: an absent given_name must not reject the token", status)
		}
		if seen.GivenName != "" {
			t.Fatalf("GivenName = %q, want empty", seen.GivenName)
		}
	})
}

// The identity fields handlers actually read must survive extraction, and the
// profile service must see them too.
func TestRequireAuth_FlattensDeclaredClaims(t *testing.T) {
	profiles := &stubProfiles{returnID: "user-1"}
	m, ring := newManagerWithKey(t, profiles)

	claims := userClaims(testOU)
	claims["given_name"] = "Alice"
	claims["phone_number"] = "+61400111222"

	_, _, _, seen := serve(t, m, ring.sign(claims))

	if seen.Email != "officer@fcau.gov" || seen.GivenName != "Alice" ||
		seen.PhoneNumber != "+61400111222" || seen.OUID != "ou-123" || seen.OUHandle != testOU {
		t.Fatalf("unexpected principal: %#v", seen)
	}
	if seen.IDPUserID != "sub-001" {
		t.Fatalf("IDPUserID = %q, want sub-001", seen.IDPUserID)
	}
	if profiles.lastSeen == nil || profiles.lastSeen.Email != "officer@fcau.gov" || profiles.lastSeen.OUHandle != testOU {
		t.Fatalf("profile service saw %#v", profiles.lastSeen)
	}
}
