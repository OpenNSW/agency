package web

import (
	"fmt"
	"net/url"
	"slices"
)

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

	// I18n is the public SPA language configuration served via /config.js as
	// window.__APP_CONFIG__.i18n.
	I18n I18nConfig `yaml:"i18n" json:"i18n"`
}

// Validate reports whether the config served at /config.js is usable. Called
// unconditionally at startup (cmd/server/main.go): /config.js is always
// served, in dev (proxied by Vite, see frontend/vite.config.ts) as much as in
// prod, so every deployment — including each agency's dev config.yaml — must
// supply a valid Runtime, Branding, and I18n.
func (c Config) Validate() error {
	if err := c.Runtime.Validate(); err != nil {
		return err
	}
	if err := c.Branding.Validate(); err != nil {
		return err
	}
	return c.I18n.Validate()
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

// FooterLink is one entry in Branding.FooterLinks — a link shown in the
// footer. Key selects a known page (e.g. "policy", "accessibility",
// "support"); its visible label is resolved from the frontend's own i18n
// bundles by that key, not carried in config, so it renders correctly in
// every supported language. URL is an absolute URL to where that content is
// hosted externally — this app has no pages of its own for these.
type FooterLink struct {
	Key string `json:"key" yaml:"key"`
	URL string `json:"url" yaml:"url"`
}

// validFooterLinkKeys are the only FooterLink.Key values the frontend has a
// translated label for (see Footer.tsx's footerLinkLabel); any other key
// would render nothing, silently, so Validate rejects it instead.
var validFooterLinkKeys = map[string]bool{
	"policy":        true,
	"accessibility": true,
	"support":       true,
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
// frontend's Zod schema as a defense in depth); the rest are optional. An
// unset PortalName/Description is omitted from /config.js and the frontend
// fills in its own default for it (see config.ts's DEFAULT_BRANDING) — those
// two are rendered unconditionally, so a blank value would be visible. The
// remaining cosmetic fields (logo/favicon/hero image/partner logos) are
// simply omitted from rendering when unset, with no substitute value needed.
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
	FooterLinks   []FooterLink  `json:"footerLinks,omitempty" yaml:"footerLinks"`
	// CopyrightNotice is an optional statement shown centered in the footer
	// (bottom-most element when the footer stacks on narrow screens — see
	// frontend/src/components/Layout/Footer.tsx). Free text: this app is not
	// specific to any one country or legal entity, so the exact wording is a
	// per-deployment choice, not something this app can derive on its own.
	CopyrightNotice string `json:"copyrightNotice,omitempty" yaml:"copyrightNotice"`
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
	for i, link := range b.FooterLinks {
		if link.Key == "" {
			return fmt.Errorf("web.branding.footerLinks[%d].key is required", i)
		}
		if !validFooterLinkKeys[link.Key] {
			return fmt.Errorf("web.branding.footerLinks[%d].key %q is not a supported footer link", i, link.Key)
		}
		if link.URL == "" {
			return fmt.Errorf("web.branding.footerLinks[%d].url is required", i)
		}
		parsed, err := url.Parse(link.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("web.branding.footerLinks[%d].url %q must be an absolute http(s) URL", i, link.URL)
		}
	}
	return nil
}

// validLanguageCodes are the languages the frontend ships a translation for
// (see frontend/src/i18n/locales) — supportedLanguages/defaultLanguage must
// be drawn from this set so a typo doesn't silently fall back to English.
var validLanguageCodes = map[string]bool{
	"en": true,
	"si": true,
}

// I18nConfig is the public SPA language configuration the browser reads from
// window.__APP_CONFIG__.i18n (see frontend/src/i18n/index.ts). This app is
// not specific to any one country, so which languages a deployment offers —
// and which one it starts in — is a deployment choice, not a hardcoded
// constant.
type I18nConfig struct {
	// SupportedLanguages is the language codes offered in the language
	// switcher. Optional; unset means every language the frontend ships a
	// translation for (see validLanguageCodes).
	SupportedLanguages []string `json:"supportedLanguages,omitempty" yaml:"supportedLanguages"`
	// DefaultLanguage is used when there's no saved preference and the
	// browser's language doesn't match a supported one. Optional; unset
	// defaults to "en".
	DefaultLanguage string `json:"defaultLanguage,omitempty" yaml:"defaultLanguage"`
}

// Validate enforces that every configured language is one the frontend
// actually has a translation for, and that DefaultLanguage (when both are
// set) is itself one of SupportedLanguages — an unreachable default would
// silently fall back to English.
func (c I18nConfig) Validate() error {
	seen := make(map[string]bool, len(c.SupportedLanguages))
	for i, lang := range c.SupportedLanguages {
		if !validLanguageCodes[lang] {
			return fmt.Errorf("web.i18n.supportedLanguages[%d] %q is not a language this app ships a translation for", i, lang)
		}
		if seen[lang] {
			return fmt.Errorf("web.i18n.supportedLanguages[%d] %q is duplicated", i, lang)
		}
		seen[lang] = true
	}
	if c.DefaultLanguage != "" {
		if !validLanguageCodes[c.DefaultLanguage] {
			return fmt.Errorf("web.i18n.defaultLanguage %q is not a language this app ships a translation for", c.DefaultLanguage)
		}
		if len(c.SupportedLanguages) > 0 && !slices.Contains(c.SupportedLanguages, c.DefaultLanguage) {
			return fmt.Errorf("web.i18n.defaultLanguage %q must be one of web.i18n.supportedLanguages", c.DefaultLanguage)
		}
	}
	return nil
}
