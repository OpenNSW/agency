package web

import (
	"encoding/json"
	"strings"
	"testing"
)

// /runtime-env.js is the SPA's only channel for these values, so a key the frontend
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
			t.Errorf("%s missing from /runtime-env.js payload", key)
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
