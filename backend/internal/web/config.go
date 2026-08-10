package web

import "fmt"

// Config holds everything needed to serve the officer-portal SPA: where the
// built assets live plus the public runtime config exposed to the browser.
type Config struct {
	// Dir is where the built SPA is served from, relative to the server's working
	// directory. In the image the binary runs with WORKDIR /app and the Dockerfile
	// copies the build to /app/web, so the default ("web") resolves there. Locally
	// it usually doesn't exist (the frontend runs via its own dev server), so the
	// server serves API-only — see Handler / cmd/server/main.go.
	Dir string

	// Runtime is the public SPA config served via /runtime-env.js.
	Runtime RuntimeConfig
}

// Validate reports whether the frontend can be served with a usable runtime
// config. Call it only when actually serving the frontend (the assets exist);
// API-only runs don't need these values.
func (c Config) Validate() error {
	return c.Runtime.Validate()
}

// RuntimeConfig is the public SPA config the browser reads from
// window.__APP_CONFIG__ (see frontend/src/runtimeConfig.ts). Every field is
// public client config (no secrets), so /runtime-env.js needs no auth.
//
// The JSON tags are the VITE_* names the frontend looks up. omitempty means an
// unset optional value is omitted from /runtime-env.js entirely, so the frontend
// falls back to its build-time value (getEnv). Required values are enforced by
// Validate at startup rather than failing later in the browser.
type RuntimeConfig struct {
	BrandingName  string `json:"VITE_BRANDING_NAME,omitempty"`
	APIBaseURL    string `json:"VITE_API_BASE_URL,omitempty"`
	IDPBaseURL    string `json:"VITE_IDP_BASE_URL,omitempty"`
	IDPClientID   string `json:"VITE_IDP_CLIENT_ID,omitempty"`
	IDPExpectedOU string `json:"VITE_IDP_EXPECTED_OU_HANDLE,omitempty"`
	AppURL        string `json:"VITE_APP_URL,omitempty"`
	IDPScopes     string `json:"VITE_IDP_SCOPES,omitempty"`
	// Query-string-encoded extra parameters for the IdP's authorization request, e.g.
	// "resource=https://api.nsw-agency.local". Optional; nothing here is IdP-specific.
	IDPExtraQueryParams string `json:"VITE_IDP_EXTRA_QUERY_PARAMS,omitempty"`
}

// Validate enforces the keys the frontend reads via getRequiredEnv (see
// constants/index.ts and features/user/oidcUserManager.ts). The rest are
// optional — the SPA has its own fallbacks for them.
func (c RuntimeConfig) Validate() error {
	if c.APIBaseURL == "" {
		return fmt.Errorf("VITE_API_BASE_URL is required")
	}
	if c.IDPBaseURL == "" {
		return fmt.Errorf("VITE_IDP_BASE_URL is required")
	}
	if c.IDPClientID == "" {
		return fmt.Errorf("VITE_IDP_CLIENT_ID is required")
	}
	if c.IDPExpectedOU == "" {
		return fmt.Errorf("VITE_IDP_EXPECTED_OU_HANDLE is required")
	}
	return nil
}
