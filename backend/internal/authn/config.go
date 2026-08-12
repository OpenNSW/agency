package authn

import (
	"fmt"
	"strings"

	coreauthn "github.com/OpenNSW/core/authn"
)

// Config is the authentication configuration this service supplies, loaded from
// the environment by cmd/server.
//
// It omits core/authn's claim-declaration fields: which claims to extract is
// this package's own business, not a per-deployment setting, so coreConfig
// declares them from the claim constants in principal.go.
type Config struct {
	JWKSURL               string
	Issuer                string
	Audience              string
	ClientIDs             []string
	InsecureSkipTLSVerify bool

	// ExpectedOU is the OU handle every user token must carry. It has no
	// equivalent in core/authn — enforcing it is the main reason this package
	// exists. See Manager.RequireAuthMiddleware.
	ExpectedOU string
}

// Validate reports whether the configuration is usable.
//
// The shared settings are validated by core/authn so their messages stay in
// step with the library, and so the claim declarations coreConfig adds are
// checked too — a claim name colliding with core's fixed schema is rejected
// here rather than silently shadowing a built-in field. Only ExpectedOU, which
// core knows nothing about, is validated locally.
//
// The resulting error strings are asserted verbatim by this package's tests and
// by cmd/server's, so they are part of this function's contract.
func (c Config) Validate() error {
	if err := c.coreConfig().Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.ExpectedOU) == "" {
		return fmt.Errorf("ExpectedOU is required")
	}
	return nil
}

// coreConfig maps Config onto core/authn's Config and declares the claims this
// package reads. Declaration and consumption live together on purpose: a claim
// surfaced on Principal but not declared here is silently never extracted.
//
// Required mirrors what this service guaranteed before adopting the shared
// library: a user token without email, ouId or ouHandle is rejected outright.
// ouHandle in particular backs the OU gate and the seeded-user lookup, so a
// missing value must fail at the token boundary rather than deeper in a
// request. given_name and phone_number are best-effort.
func (c Config) coreConfig() coreauthn.Config {
	return coreauthn.Config{
		JWKSURL:               c.JWKSURL,
		Issuer:                c.Issuer,
		Audience:              c.Audience,
		ClientIDs:             c.ClientIDs,
		InsecureSkipTLSVerify: c.InsecureSkipTLSVerify,
		UserClaims: coreauthn.ClaimSpec{
			Required: []string{claimEmail, claimOUID, claimOUHandle},
			Optional: []string{claimGivenName, claimPhoneNumber},
		},
	}
}
