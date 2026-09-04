package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/artifact/loaders/github"
	"github.com/OpenNSW/core/artifact/loaders/local"
	"github.com/OpenNSW/core/artifact/loaders/s3"
)

const defaultDB = `db:
  driver: sqlite
  sqlite:
    path: ./test.db
`

const defaultNSW = `nsw:
  baseURL: http://localhost:8080
  clientID: NPQS_TO_NSW
  clientSecret: secret
  tokenURL: https://localhost:8090/oauth2/token
`

const defaultAuthn = `authn:
  jwksURL: https://localhost:8090/.well-known/jwks.json
  issuer: https://localhost:8090
  audience: OGA_PORTAL_APP
  clientIDs: [OGA_PORTAL_APP]
  expectedOU: default
`

// buildConfig assembles a config.yaml fixture from one block per top-level
// section (db, nsw, authn) plus trailing top-level lines (extra, e.g.
// "readHeaderTimeout: 1s"). Passing "" for db/nsw/authn uses the corresponding
// default block above, so a test only has to spell out the section it's
// actually overriding.
func buildConfig(t *testing.T, db, nsw, authn, extra string) string {
	t.Helper()
	if db == "" {
		db = defaultDB
	}
	if nsw == "" {
		nsw = defaultNSW
	}
	if authn == "" {
		authn = defaultAuthn
	}
	artifactLoader := "artifactLoader:\n  type: local\n  local:\n    root: " + t.TempDir() + "\n"
	return db + artifactLoader + nsw + authn + extra
}

// writeConfig writes yamlContent to a temp file and points CONFIG_PATH at it,
// so LoadConfig() reads exactly this fixture.
func writeConfig(t *testing.T, yamlContent string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o600); err != nil {
		t.Fatalf("writing config fixture: %v", err)
	}
	t.Setenv("CONFIG_PATH", path)
}

func TestLoadConfig_RequiresNSWOAuth2Vars(t *testing.T) {
	testCases := []struct {
		name     string
		nsw      string
		expected string
	}{
		{name: "missing api base url", nsw: `nsw:
  clientID: NPQS_TO_NSW
  clientSecret: secret
  tokenURL: https://localhost:8090/oauth2/token
`, expected: "nsw.baseURL is required"},
		{name: "missing client id", nsw: `nsw:
  baseURL: http://localhost:8080
  clientSecret: secret
  tokenURL: https://localhost:8090/oauth2/token
`, expected: "nsw.clientID is required"},
		{name: "missing client secret", nsw: `nsw:
  baseURL: http://localhost:8080
  clientID: NPQS_TO_NSW
  tokenURL: https://localhost:8090/oauth2/token
`, expected: "nsw.clientSecret is required"},
		{name: "missing token url", nsw: `nsw:
  baseURL: http://localhost:8080
  clientID: NPQS_TO_NSW
  clientSecret: secret
`, expected: "nsw.tokenURL is required"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			writeConfig(t, buildConfig(t, "", tc.nsw, "", ""))

			_, err := LoadConfig()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != tc.expected {
				t.Fatalf("expected error %q, got %q", tc.expected, err.Error())
			}
		})
	}
}

func TestLoadConfig_RequiresAuthVars(t *testing.T) {
	testCases := []struct {
		name     string
		authn    string
		expected string
	}{
		{name: "missing jwks url", authn: `authn:
  issuer: https://localhost:8090
  audience: OGA_PORTAL_APP
  clientIDs: [OGA_PORTAL_APP]
  expectedOU: default
`, expected: "authn.jwksURL is required"},
		{name: "missing issuer", authn: `authn:
  jwksURL: https://localhost:8090/.well-known/jwks.json
  audience: OGA_PORTAL_APP
  clientIDs: [OGA_PORTAL_APP]
  expectedOU: default
`, expected: "authn.issuer is required"},
		{name: "missing audience", authn: `authn:
  jwksURL: https://localhost:8090/.well-known/jwks.json
  issuer: https://localhost:8090
  clientIDs: [OGA_PORTAL_APP]
  expectedOU: default
`, expected: "authn.audience is required"},
		{name: "missing client ids", authn: `authn:
  jwksURL: https://localhost:8090/.well-known/jwks.json
  issuer: https://localhost:8090
  audience: OGA_PORTAL_APP
  expectedOU: default
`, expected: "authn.clientIDs is required"},
		{name: "missing agency", authn: `authn:
  jwksURL: https://localhost:8090/.well-known/jwks.json
  issuer: https://localhost:8090
  audience: OGA_PORTAL_APP
  clientIDs: [OGA_PORTAL_APP]
`, expected: "authn.expectedOU is required"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			writeConfig(t, buildConfig(t, "", "", tc.authn, ""))

			_, err := LoadConfig()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != tc.expected {
				t.Fatalf("expected error %q, got %q", tc.expected, err.Error())
			}
		})
	}
}

func TestLoadConfig_ParsesOptionalScopes(t *testing.T) {
	nsw := defaultNSW + "  scopes: [scope.a, scope.b, scope.c]\n"
	writeConfig(t, buildConfig(t, "", nsw, "", ""))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{"scope.a", "scope.b", "scope.c"}
	if len(cfg.NSW.Scopes) != len(expected) {
		t.Fatalf("expected %d scopes, got %d", len(expected), len(cfg.NSW.Scopes))
	}
	for i := range expected {
		if cfg.NSW.Scopes[i] != expected[i] {
			t.Fatalf("expected scope[%d]=%q, got %q", i, expected[i], cfg.NSW.Scopes[i])
		}
	}
}

func TestLoadConfig_AllowsEmptyScopes(t *testing.T) {
	writeConfig(t, buildConfig(t, "", "", "", ""))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(cfg.NSW.Scopes) != 0 {
		t.Fatalf("expected empty scopes, got %v", cfg.NSW.Scopes)
	}
}

func TestLoadConfig_ParsesTokenInsecureSkipVerify(t *testing.T) {
	nsw := defaultNSW + "  tokenInsecureSkipVerify: true\n"
	// insecure TLS is only honored in development
	writeConfig(t, buildConfig(t, "", nsw, "", "environment: development\n"))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !cfg.NSW.TokenInsecureSkipVerify {
		t.Fatalf("expected TokenInsecureSkipVerify to be true")
	}
}

func TestServerLoadConfig_Postgres_DefaultSSLModeRequire(t *testing.T) {
	db := `db:
  driver: postgres
  postgres:
    password: secret
`
	writeConfig(t, buildConfig(t, db, "", "", ""))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DB.Postgres.SSLMode != "require" {
		t.Errorf("DB.Postgres.SSLMode = %q, want require when unset", cfg.DB.Postgres.SSLMode)
	}
}

func TestLoadConfig_RejectsDBSSLModeDisableOutsideDev(t *testing.T) {
	// environment left unset, acting as production
	db := `db:
  driver: postgres
  postgres:
    password: secret
    sslMode: disable
`
	writeConfig(t, buildConfig(t, db, "", "", ""))

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected an error when db.postgres.sslMode=disable outside development, but got nil")
	}
	if !strings.Contains(err.Error(), "db.postgres.sslMode=disable") {
		t.Errorf("expected error to mention db.postgres.sslMode=disable, got: %v", err)
	}
}

func TestLoadConfig_RejectsInvalidTokenInsecureSkipVerify(t *testing.T) {
	nsw := defaultNSW + "  tokenInsecureSkipVerify: not-a-bool\n"
	writeConfig(t, buildConfig(t, "", nsw, "", ""))

	_, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parsing config file") {
		t.Fatalf("expected a config-parse error, got: %v", err)
	}
}

// --- insecure-TLS guard (environment) ---
//
// authn.insecureSkipTLSVerify and nsw.tokenInsecureSkipVerify are only
// honored when environment: development; Config.Validate is the sole
// enforcement point — downstream (auth, nswclient, pkg/httpclient) simply
// trusts the flag it's handed. Unset/any other value is treated as
// production and fails closed.

func TestLoadConfig_NSWInsecureSkipVerify_FailsClosedOutsideDev(t *testing.T) {
	nsw := defaultNSW + "  tokenInsecureSkipVerify: true\n"
	writeConfig(t, buildConfig(t, "", nsw, "", ""))

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "nsw.tokenInsecureSkipVerify") {
		t.Fatalf("expected LoadConfig to fail closed with an nsw.tokenInsecureSkipVerify guard error, got: %v", err)
	}
}

// A stray space or newline is easy to introduce in a config file or a secret
// manager, and authn.expectedOU is compared to the token's ouHandle verbatim,
// so such a value denies every user. It must fail here, at startup, and not be
// silently trimmed into a working one: the handle also has to agree with the
// portal's VITE_IDP_EXPECTED_OU_HANDLE, and correcting one side of that on the
// fly would hide the divergence.
func TestLoadConfig_PaddedExpectedOU_FailsClosed(t *testing.T) {
	authn := `authn:
  jwksURL: https://localhost:8090/.well-known/jwks.json
  issuer: https://localhost:8090
  audience: OGA_PORTAL_APP
  clientIDs: [OGA_PORTAL_APP]
  expectedOU: "  fcau\n"
`
	writeConfig(t, buildConfig(t, "", "", authn, ""))

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected an error for an authn.expectedOU with surrounding whitespace")
	}
	if !strings.Contains(err.Error(), "authn.expectedOU must not have surrounding whitespace") {
		t.Fatalf("error = %q, want it to name the whitespace problem", err)
	}
}

func TestLoadConfig_AuthJWKSInsecureSkipVerify_AllowedInDev(t *testing.T) {
	authn := defaultAuthn + "  insecureSkipTLSVerify: true\n"
	writeConfig(t, buildConfig(t, "", "", authn, "environment: development\n"))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !cfg.Authn.InsecureSkipTLSVerify {
		t.Fatalf("expected InsecureSkipTLSVerify to be true")
	}
}

func TestLoadConfig_AuthJWKSInsecureSkipVerify_FailsClosedOutsideDev(t *testing.T) {
	authn := defaultAuthn + "  insecureSkipTLSVerify: true\n"
	writeConfig(t, buildConfig(t, "", "", authn, ""))

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "authn.insecureSkipTLSVerify") {
		t.Fatalf("expected LoadConfig to fail closed with an authn.insecureSkipTLSVerify guard error, got: %v", err)
	}
}

func TestLoadConfig_ServerTimeoutDefaults(t *testing.T) {
	writeConfig(t, buildConfig(t, "", "", "", ""))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want %v", cfg.ReadHeaderTimeout, 5*time.Second)
	}
	if cfg.ReadTimeout != 15*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 15*time.Second)
	}
	if cfg.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, 30*time.Second)
	}
	if cfg.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout = %v, want %v", cfg.IdleTimeout, 60*time.Second)
	}
}

func TestLoadConfig_ServerTimeoutsFromConfig(t *testing.T) {
	extra := "readHeaderTimeout: 1s\nreadTimeout: 2s\nwriteTimeout: 3s\nidleTimeout: 4s\n"
	writeConfig(t, buildConfig(t, "", "", "", extra))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.ReadHeaderTimeout != time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want %v", cfg.ReadHeaderTimeout, time.Second)
	}
	if cfg.ReadTimeout != 2*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 2*time.Second)
	}
	if cfg.WriteTimeout != 3*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, 3*time.Second)
	}
	if cfg.IdleTimeout != 4*time.Second {
		t.Errorf("IdleTimeout = %v, want %v", cfg.IdleTimeout, 4*time.Second)
	}
}

func TestLoadConfig_RejectsInvalidServerTimeout(t *testing.T) {
	writeConfig(t, buildConfig(t, "", "", "", "readTimeout: not-a-duration\n"))

	_, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `invalid duration "not-a-duration"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfig_RejectsInvalidMaxRequestBytes(t *testing.T) {
	testCases := []struct {
		name  string
		value string
	}{
		{name: "zero", value: "0"},
		{name: "negative", value: "-1"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			writeConfig(t, buildConfig(t, "", "", "", "maxRequestBytes: "+tc.value+"\n"))

			_, err := LoadConfig()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "maxRequestBytes must be greater than zero") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadConfig_MissingConfigFile(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "does-not-exist.yaml"))

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected an error for a missing config file")
	}
	if !strings.Contains(err.Error(), "reading config file") {
		t.Fatalf("expected a file-read error, got: %v", err)
	}
}

func TestLoadConfig_ResolvesSecretFromEnv(t *testing.T) {
	t.Setenv("DB_PASSWORD_FOR_TEST", "s3cr3t")
	db := `db:
  driver: postgres
  postgres:
    password: "{{env:DB_PASSWORD_FOR_TEST}}"
`
	writeConfig(t, buildConfig(t, db, "", "", ""))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DB.Postgres.Password != "s3cr3t" {
		t.Fatalf("expected resolved password %q, got %q", "s3cr3t", cfg.DB.Postgres.Password)
	}
}

// Placeholder resolution isn't limited to fields the app considers
// "secrets" — any scalar, including a non-string one, can be sourced from an
// env var. Exercises maxRequestBytes (an *int64) to prove the resolved
// node's tag/style reset lets the real type still get inferred correctly.
func TestLoadConfig_ResolvesPlaceholderOnNonStringField(t *testing.T) {
	t.Setenv("MAX_REQUEST_BYTES_FOR_TEST", "12345")
	writeConfig(t, buildConfig(t, "", "", "", "maxRequestBytes: \"{{env:MAX_REQUEST_BYTES_FOR_TEST}}\"\n"))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.MaxRequestBytes != 12345 {
		t.Fatalf("expected MaxRequestBytes 12345, got %d", cfg.MaxRequestBytes)
	}
}

func TestLoadConfig_UnsetSecretEnvFailsClosed(t *testing.T) {
	nsw := `nsw:
  baseURL: http://localhost:8080
  clientID: NPQS_TO_NSW
  clientSecret: "{{env:NSW_CLIENT_SECRET_DOES_NOT_EXIST}}"
  tokenURL: https://localhost:8090/oauth2/token
`
	writeConfig(t, buildConfig(t, "", nsw, "", ""))

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected an error for an unset secret env var")
	}
	if !strings.Contains(err.Error(), "nsw.clientSecret") {
		t.Fatalf("error = %q, want it to name the failing field", err)
	}
}

// A value that only partly looks like a placeholder (extra text alongside
// the braces) is left as a literal, not resolved — the whole scalar must be
// the placeholder.
func TestLoadConfig_PartialPlaceholderIsLiteral(t *testing.T) {
	nsw := `nsw:
  baseURL: http://localhost:8080
  clientID: NPQS_TO_NSW
  clientSecret: "prefix-{{env:SOME_VAR}}-suffix"
  tokenURL: https://localhost:8090/oauth2/token
`
	writeConfig(t, buildConfig(t, "", nsw, "", ""))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.NSW.ClientSecret != "prefix-{{env:SOME_VAR}}-suffix" {
		t.Fatalf("expected literal passthrough, got %q", cfg.NSW.ClientSecret)
	}
}

func TestIsDevEnvironment(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"development", true},
		{"Development", true},
		{" development ", true},
		{"production", false},
		{"", false},
		{"staging", false},
	}
	for _, c := range cases {
		t.Run(c.val, func(t *testing.T) {
			if got := isDevEnvironment(c.val); got != c.want {
				t.Fatalf("isDevEnvironment(%q) = %v, want %v", c.val, got, c.want)
			}
		})
	}
}

// TestYamlLoaderMirrors_MatchUpstreamFields guards against
// yamlArtifactLoaderConfig (and its sub-configs) silently missing a field
// added to the upstream loaders.Config/local.Config/github.Config/s3.Config
// types it mirrors for YAML decoding (see toLoadersConfig): a new upstream
// field must be deliberately handled here — either mirrored, or added to
// intentionallyOmittedUpstreamFields with a reason.
func TestYamlLoaderMirrors_MatchUpstreamFields(t *testing.T) {
	// Fields that exist upstream but are deliberately not part of the YAML
	// shape (not something a config file can express, e.g. an injected
	// *http.Client).
	intentionallyOmittedUpstreamFields := map[string][]string{
		"github.Config": {"HTTPClient"},
	}

	cases := []struct {
		upstreamName string
		upstream     any
		mirror       any
	}{
		{"loaders.Config", loaders.Config{}, yamlArtifactLoaderConfig{}},
		{"local.Config", local.Config{}, yamlLocalLoaderConfig{}},
		{"github.Config", github.Config{}, yamlGitHubLoaderConfig{}},
		{"s3.Config", s3.Config{}, yamlS3LoaderConfig{}},
	}

	for _, tc := range cases {
		t.Run(tc.upstreamName, func(t *testing.T) {
			mirrorFields := fieldNameSet(tc.mirror)
			omitted := intentionallyOmittedUpstreamFields[tc.upstreamName]

			ut := reflect.TypeOf(tc.upstream)
			for i := range ut.NumField() {
				name := ut.Field(i).Name
				if slices.Contains(omitted, name) {
					continue
				}
				if !mirrorFields[name] {
					t.Errorf("%s.%s has no matching field in its yaml mirror struct — mirror it in cmd/server/config.go, or add it to intentionallyOmittedUpstreamFields with a reason", tc.upstreamName, name)
				}
			}
		})
	}
}

func fieldNameSet(v any) map[string]bool {
	t := reflect.TypeOf(v)
	set := make(map[string]bool, t.NumField())
	for i := range t.NumField() {
		set[t.Field(i).Name] = true
	}
	return set
}

func TestLoadConfig_RefIDGen_Omitted(t *testing.T) {
	writeConfig(t, buildConfig(t, "", "", "", ""))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	// No section means the feature is simply off — not an error.
	if len(cfg.RefIDGen.Issuers) != 0 {
		t.Errorf("RefIDGen.Issuers = %d, want 0 when no refIDGen section is present", len(cfg.RefIDGen.Issuers))
	}
}

func TestLoadConfig_RefIDGen_Decoded(t *testing.T) {
	writeConfig(t, buildConfig(t, "", "", "", `refIDGen:
  issuers:
    - issuer: NPQS
      formats:
        - idType: application_id
          segments:
            - {type: literal, value: "NPQS/"}
            - {type: list, list: office_location, param: officeCode}
            - {type: sequence, scopeKey: "{issuer}:{idType}:{officeCode}:{yyyyMMdd}", padding: 6}
  lists:
    office_location: [NPQS-KAT, SEA-CMB]
`))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(cfg.RefIDGen.Issuers) != 1 {
		t.Fatalf("RefIDGen.Issuers = %d, want 1", len(cfg.RefIDGen.Issuers))
	}
	issuer := cfg.RefIDGen.Issuers[0]
	if issuer.Issuer != "NPQS" {
		t.Errorf("issuer = %q, want \"NPQS\"", issuer.Issuer)
	}
	if len(issuer.Formats) != 1 || issuer.Formats[0].IDType != "application_id" {
		t.Fatalf("formats = %+v, want one application_id format", issuer.Formats)
	}
	segments := issuer.Formats[0].Segments
	if len(segments) != 3 {
		t.Fatalf("segments = %d, want 3", len(segments))
	}
	// The camelCase yaml tags are the reason refid.Config can be inlined
	// rather than needing a mirror struct — check the ones that would break.
	if segments[2].ScopeKey != "{issuer}:{idType}:{officeCode}:{yyyyMMdd}" {
		t.Errorf("scopeKey = %q, want the configured template", segments[2].ScopeKey)
	}
	if segments[2].Padding != 6 {
		t.Errorf("padding = %d, want 6", segments[2].Padding)
	}
	if got := cfg.RefIDGen.Lists["office_location"]; len(got) != 2 || got[0] != "NPQS-KAT" {
		t.Errorf("lists[office_location] = %v, want [NPQS-KAT SEA-CMB]", got)
	}
}
