package authn

import (
	"context"
	"slices"
	"testing"
)

// The error strings below are asserted verbatim by cmd/server's config tests,
// so this table is the contract for both.
func TestConfig_Validate_RequiredFields(t *testing.T) {
	base := func() Config {
		return Config{
			JWKSURL:    "https://idp.example.com/jwks",
			Issuer:     "https://idp.example.com",
			Audience:   "AGENCY_API",
			ClientIDs:  []string{"AGENCY_PORTAL"},
			ExpectedOU: "fcau",
		}
	}

	if err := base().Validate(); err != nil {
		t.Fatalf("expected the base config to be valid, got %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(c *Config)
		wantErr string
	}{
		{"missing JWKS URL", func(c *Config) { c.JWKSURL = "" }, "AUTH_JWKS_URL is required"},
		{"relative JWKS URL", func(c *Config) { c.JWKSURL = "/jwks" }, "AUTH_JWKS_URL must be a valid absolute URL"},
		{"non-http JWKS URL", func(c *Config) { c.JWKSURL = "ftp://idp.example.com" }, "AUTH_JWKS_URL must use http or https"},
		{"missing issuer", func(c *Config) { c.Issuer = "" }, "AUTH_ISSUER is required"},
		{"relative issuer", func(c *Config) { c.Issuer = "idp" }, "AUTH_ISSUER must be a valid absolute URL"},
		{"missing audience", func(c *Config) { c.Audience = "" }, "AUTH_AUDIENCE is required"},
		{"missing client ids", func(c *Config) { c.ClientIDs = nil }, "AUTH_CLIENT_IDS is required"},
		{"missing expected OU", func(c *Config) { c.ExpectedOU = "" }, "ExpectedOU is required"},
		{"whitespace-only expected OU", func(c *Config) { c.ExpectedOU = "   " }, "ExpectedOU is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.wantErr)
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("expected error %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

// Every claim Principal surfaces must also be declared to core/authn, or it is
// never extracted and silently reads as empty at runtime.
func TestConfig_DeclaresEveryClaimPrincipalReads(t *testing.T) {
	spec := Config{}.coreConfig().UserClaims
	declared := slices.Concat(spec.Required, spec.Optional)

	for _, claim := range []string{claimEmail, claimGivenName, claimPhoneNumber, claimOUID, claimOUHandle} {
		if !slices.Contains(declared, claim) {
			t.Errorf("claim %q is read into Principal but never declared to core/authn", claim)
		}
	}
}

// Preserves the rejection this service guaranteed before adopting the shared
// library: a user token without these claims is not usable.
func TestConfig_RequiresIdentityClaims(t *testing.T) {
	spec := Config{}.coreConfig().UserClaims

	for _, claim := range []string{claimEmail, claimOUID, claimOUHandle} {
		if !slices.Contains(spec.Required, claim) {
			t.Errorf("%q must be required, got required=%v optional=%v", claim, spec.Required, spec.Optional)
		}
	}
	for _, claim := range []string{claimGivenName, claimPhoneNumber} {
		if !slices.Contains(spec.Optional, claim) {
			t.Errorf("%q should be optional, got required=%v optional=%v", claim, spec.Required, spec.Optional)
		}
	}
}

func TestFromContext_Unauthenticated(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("expected no principal on a bare context")
	}
	// An explicitly stored nil must still read as unauthenticated rather than
	// handing callers a nil pointer to dereference.
	if _, ok := FromContext(ContextWithPrincipal(context.Background(), nil)); ok {
		t.Fatal("expected a nil principal to read as unauthenticated")
	}
}

func TestContextWithPrincipal_RoundTrip(t *testing.T) {
	want := &Principal{Kind: KindUser, UserID: "user-1", OUHandle: "fcau"}
	got, ok := FromContext(ContextWithPrincipal(context.Background(), want))
	if !ok {
		t.Fatal("expected a principal")
	}
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
