package web

import "fmt"

// Config holds everything needed to serve the officer-portal SPA: where the
// built assets live plus the public runtime config exposed to the browser.
// Runtime and Branding (never Dir, a server-only path) are marshaled as-is to
// become window.__APP_CONFIG__ (see Handler.NewHandler/ServeConfig) — so this
// struct's JSON shape IS the wire contract with frontend/src/runtimeConfig.ts:
// window.__APP_CONFIG__ = { runtime: {...}, branding: {...} }, one object
// mirroring config.yaml's own web.runtime/web.branding nesting, rather than
// two separate globals.
type Config struct {
	// Dir is where the built SPA is served from, relative to the server's working
	// directory. In the image the binary runs with WORKDIR /app and the Dockerfile
	// copies the build to /app/web, so the default ("web") resolves there. Locally
	// it usually doesn't exist (the frontend runs via its own dev server), so the
	// server serves API-only — see Handler / cmd/server/main.go. Never sent to the
	// browser (json:"-"): it's a server-side filesystem path, not client config.
	Dir string `yaml:"dir" json:"-"`

	// Runtime is the public SPA config served via /config.js as
	// window.__APP_CONFIG__.runtime.
	Runtime RuntimeConfig `yaml:"runtime" json:"runtime"`

	// Branding is the public SPA branding served via /config.js as
	// window.__APP_CONFIG__.branding.
	Branding Branding `yaml:"branding" json:"branding"`
}

// Validate reports whether the config served at /config.js is usable. Called
// unconditionally at startup (cmd/server/main.go): /config.js is always
// served, in dev (proxied by Vite, see frontend/vite.config.ts) as much as in
// prod, so every deployment — including each agency's dev config.yaml — must
// supply a valid Runtime and Branding.
func (c Config) Validate() error {
	if err := c.Runtime.Validate(); err != nil {
		return err
	}
	return c.Branding.Validate()
}

// RuntimeConfig is the public SPA config the browser reads from
// window.__APP_CONFIG__.runtime (see frontend/src/runtimeConfig.ts). Every
// field is public client config (no secrets), so /config.js needs no auth.
//
// The JSON tags are the VITE_* names the frontend looks up. omitempty means an
// unset optional value is omitted from /config.js entirely, so the frontend
// falls back to its own default (see getEnv's fallback param). Required values
// are enforced by Validate at startup rather than failing later in the browser.
type RuntimeConfig struct {
	APIBaseURL    string `json:"VITE_API_BASE_URL,omitempty" yaml:"apiBaseURL"`
	IDPBaseURL    string `json:"VITE_IDP_BASE_URL,omitempty" yaml:"idpBaseURL"`
	IDPClientID   string `json:"VITE_IDP_CLIENT_ID,omitempty" yaml:"idpClientID"`
	IDPExpectedOU string `json:"VITE_IDP_EXPECTED_OU_HANDLE,omitempty" yaml:"idpExpectedOU"`
	AppURL        string `json:"VITE_APP_URL,omitempty" yaml:"appURL"`
	IDPScopes     string `json:"VITE_IDP_SCOPES,omitempty" yaml:"idpScopes"`
	// Query-string-encoded extra parameters for the IdP's authorization request, e.g.
	// "resource=https://api.nsw-agency.local". Optional; nothing here is IdP-specific.
	IDPExtraQueryParams string `json:"VITE_IDP_EXTRA_QUERY_PARAMS,omitempty" yaml:"idpExtraQueryParams"`
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

// PartnerLogo is one entry in Branding.PartnerLogos.
type PartnerLogo struct {
	URL string `json:"url" yaml:"url"`
	Alt string `json:"alt" yaml:"alt"`
}

// Branding is the public SPA branding the browser reads from
// window.__APP_CONFIG__.branding (see frontend/src/runtimeConfig.ts and
// frontend/src/config.ts, which validates this shape with a Zod schema).
// Formerly delivered as a separate, per-agency static
// frontend/public/configs/<name>.branding.json file fetched at startup; that
// mechanism never actually reached production (the per-agency files were
// gitignored and nothing in the build/deploy pipeline generated them), so
// branding now travels through the same config.yaml -> /config.js channel as
// RuntimeConfig instead of a second, parallel one.
//
// SystemName and AppName are required (enforced by Validate, and by the
// frontend's Zod schema as a defense in depth); the rest are optional cosmetic
// fields — an unset one is omitted from /config.js and the frontend's own
// fallback (see config.ts) fills in for it.
type Branding struct {
	SystemName    string        `json:"systemName" yaml:"systemName"`
	AppName       string        `json:"appName" yaml:"appName"`
	LogoURL       string        `json:"logoUrl,omitempty" yaml:"logoUrl"`
	SystemLogoURL string        `json:"systemLogoUrl,omitempty" yaml:"systemLogoUrl"`
	Favicon       string        `json:"favicon,omitempty" yaml:"favicon"`
	PortalName    string        `json:"portalName,omitempty" yaml:"portalName"`
	Description   string        `json:"description,omitempty" yaml:"description"`
	HeroImageURL  string        `json:"heroImageUrl,omitempty" yaml:"heroImageUrl"`
	PartnerLogos  []PartnerLogo `json:"partnerLogos,omitempty" yaml:"partnerLogos"`
}

// Validate enforces the fields frontend/src/config.ts's Zod schema also
// requires (systemName, appName) — enforced here too so a misconfigured
// deployment fails fast at startup rather than in the browser.
func (b Branding) Validate() error {
	if b.SystemName == "" {
		return fmt.Errorf("web.branding.systemName is required")
	}
	if b.AppName == "" {
		return fmt.Errorf("web.branding.appName is required")
	}
	return nil
}
