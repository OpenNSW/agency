package web

import (
	"encoding/json"
	"strings"
	"testing"
)

// /config.js is the SPA's only channel for these values, so a key the frontend
// reads but RuntimeConfig does not carry is invisible in a deployed run.
func TestRuntimeConfigMarshalsFrontendKeys(t *testing.T) {
	cfg := RuntimeConfig{
		APIBaseURL:          "http://localhost:8081",
		IDPBaseURL:          "https://localhost:8090",
		IDPClientID:         "FCAU_PORTAL_APP",
		IDPExpectedOU:       "fcau",
		IDPExtraQueryParams: "resource=https://api.nsw-agency.local&prompt=consent",
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{
		"VITE_API_BASE_URL",
		"VITE_IDP_BASE_URL",
		"VITE_IDP_CLIENT_ID",
		"VITE_IDP_EXPECTED_OU_HANDLE",
		"VITE_IDP_EXTRA_QUERY_PARAMS",
	} {
		if _, ok := got[key]; !ok {
			t.Errorf("%s missing from /config.js payload", key)
		}
	}

	// Must survive verbatim, especially the "&": the frontend re-parses it.
	if want := "resource=https://api.nsw-agency.local&prompt=consent"; got["VITE_IDP_EXTRA_QUERY_PARAMS"] != want {
		t.Errorf("extra query params mangled: got %q, want %q", got["VITE_IDP_EXTRA_QUERY_PARAMS"], want)
	}
}

// Unset means "send nothing extra", so the key is omitted rather than emitted empty.
func TestRuntimeConfigOmitsUnsetExtraQueryParams(t *testing.T) {
	cfg := RuntimeConfig{
		APIBaseURL:    "http://localhost:8081",
		IDPBaseURL:    "https://localhost:8090",
		IDPClientID:   "FCAU_PORTAL_APP",
		IDPExpectedOU: "fcau",
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "VITE_IDP_EXTRA_QUERY_PARAMS") {
		t.Errorf("unset extra query params should be omitted, got %s", raw)
	}

	// An IdP needing no extra parameter is valid, so Validate must not require it.
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate should not require extra query params: %v", err)
	}
}

// window.__APP_CONFIG__.branding is the SPA's only channel for branding since
// the per-agency static branding.json files were retired — a key the
// frontend's Zod schema (config.ts) expects but Branding does not carry is
// invisible in a deployed run, same risk TestRuntimeConfigMarshalsFrontendKeys
// guards.
func TestBrandingMarshalsFrontendKeys(t *testing.T) {
	cfg := Branding{
		SystemName:  "NSW",
		AppName:     "FCAU Officer Portal",
		PortalName:  "FCAU Portal",
		Description: "desc",
		PartnerLogos: []PartnerLogo{
			{URL: "https://example.com/logo.png", Alt: "Partner"},
		},
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"systemName", "appName", "portalName", "description", "partnerLogos"} {
		if _, ok := got[key]; !ok {
			t.Errorf("%s missing from /config.js branding payload", key)
		}
	}
}

// Unset optional fields are omitted rather than emitted empty, so the
// frontend's own per-field fallback (see config.ts) kicks in for them.
func TestBrandingOmitsUnsetOptionalFields(t *testing.T) {
	cfg := Branding{SystemName: "NSW", AppName: "FCAU Officer Portal"}

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"logoUrl", "systemLogoUrl", "favicon", "portalName", "description", "heroImageUrl", "partnerLogos"} {
		if strings.Contains(string(raw), key) {
			t.Errorf("unset %s should be omitted, got %s", key, raw)
		}
	}
}

// Config is the wire shape marshaled straight to window.__APP_CONFIG__ (see
// Handler.NewHandler/ServeConfig) — Runtime and Branding must nest under
// "runtime"/"branding" (matching config.yaml's own web.runtime/web.branding),
// and Dir (a server-side filesystem path) must never reach the browser.
func TestConfigMarshalsNestedRuntimeAndBrandingWithoutDir(t *testing.T) {
	cfg := Config{
		Dir: "/secret/server/path",
		Runtime: RuntimeConfig{
			APIBaseURL:    "http://localhost:8081",
			IDPBaseURL:    "https://localhost:8090",
			IDPClientID:   "FCAU_PORTAL_APP",
			IDPExpectedOU: "fcau",
		},
		Branding: Branding{SystemName: "NSW", AppName: "FCAU Officer Portal"},
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if strings.Contains(string(raw), "/secret/server/path") {
		t.Errorf("Dir leaked into /config.js payload: %s", raw)
	}

	var got struct {
		Runtime  map[string]string `json:"runtime"`
		Branding map[string]string `json:"branding"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Runtime["VITE_API_BASE_URL"] != cfg.Runtime.APIBaseURL {
		t.Errorf("runtime not nested under \"runtime\": got %#v", got.Runtime)
	}
	if got.Branding["systemName"] != cfg.Branding.SystemName {
		t.Errorf("branding not nested under \"branding\": got %#v", got.Branding)
	}
}

func TestBrandingValidateRequiresSystemAndAppName(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Branding
		wantErr bool
	}{
		{"both set", Branding{SystemName: "NSW", AppName: "Portal"}, false},
		{"missing systemName", Branding{AppName: "Portal"}, true},
		{"missing appName", Branding{SystemName: "NSW"}, true},
		{"both missing", Branding{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr && err == nil {
				t.Error("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}
