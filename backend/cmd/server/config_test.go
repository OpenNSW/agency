package main

import (
	"strings"
	"testing"
	"time"
)

func setBaseConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_PATH", "./test.db")
	// Point the artifact loader at a valid directory so its startup validation
	// passes and does not mask the config errors these tests assert on.
	t.Setenv("ARTIFACT_LOCAL_ROOT", t.TempDir())
}

func setRequiredNSWOAuth2Env(t *testing.T) {
	t.Helper()
	t.Setenv("NSW_API_BASE_URL", "http://localhost:8080/api/v1")
	t.Setenv("NSW_CLIENT_ID", "NPQS_TO_NSW")
	t.Setenv("NSW_CLIENT_SECRET", "secret")
	t.Setenv("NSW_TOKEN_URL", "https://localhost:8090/oauth2/token")
}

func setRequiredAuthEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AUTH_JWKS_URL", "https://localhost:8090/.well-known/jwks.json")
	t.Setenv("AUTH_ISSUER", "https://localhost:8090")
	t.Setenv("AUTH_AUDIENCE", "OGA_PORTAL_APP")
	t.Setenv("AUTH_CLIENT_IDS", "OGA_PORTAL_APP")
	t.Setenv("AUTH_EXPECTED_OU", "default")
}

func TestLoadConfig_RequiresNSWOAuth2Vars(t *testing.T) {
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)

	testCases := []struct {
		name     string
		missing  string
		expected string
	}{
		{name: "missing api base url", missing: "NSW_API_BASE_URL", expected: "NSW_API_BASE_URL is required"},
		{name: "missing client id", missing: "NSW_CLIENT_ID", expected: "NSW_CLIENT_ID is required"},
		{name: "missing client secret", missing: "NSW_CLIENT_SECRET", expected: "NSW_CLIENT_SECRET is required"},
		{name: "missing token url", missing: "NSW_TOKEN_URL", expected: "NSW_TOKEN_URL is required"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.missing, "")
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
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)

	testCases := []struct {
		name     string
		missing  string
		expected string
	}{
		{name: "missing jwks url", missing: "AUTH_JWKS_URL", expected: "AUTH_JWKS_URL is required"},
		{name: "missing issuer", missing: "AUTH_ISSUER", expected: "AUTH_ISSUER is required"},
		{name: "missing audience", missing: "AUTH_AUDIENCE", expected: "AUTH_AUDIENCE is required"},
		{name: "missing client ids", missing: "AUTH_CLIENT_IDS", expected: "AUTH_CLIENT_IDS is required"},
		{name: "missing agency", missing: "AUTH_EXPECTED_OU", expected: "ExpectedOU is required"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.missing, "")
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
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)
	t.Setenv("NSW_SCOPES", "scope.a, scope.b, ,scope.c")

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
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)
	t.Setenv("NSW_SCOPES", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(cfg.NSW.Scopes) != 0 {
		t.Fatalf("expected empty scopes, got %v", cfg.NSW.Scopes)
	}
}

func TestLoadConfig_ParsesTokenInsecureSkipVerify(t *testing.T) {
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)
	t.Setenv("APP_ENV", "development") // insecure TLS is only honored in development
	t.Setenv("NSW_TOKEN_INSECURE_SKIP_VERIFY", "true")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !cfg.NSW.TokenInsecureSkipVerify {
		t.Fatalf("expected TokenInsecureSkipVerify to be true")
	}
}

func TestServerLoadConfig_Postgres_DefaultSSLModeRequire(t *testing.T) {
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_SSLMODE", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DB.Postgres.SSLMode != "require" {
		t.Errorf("DB.Postgres.SSLMode = %q, want require when unset", cfg.DB.Postgres.SSLMode)
	}
}

func TestLoadConfig_RejectsDBSSLModeDisableOutsideDev(t *testing.T) {
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("APP_ENV", "production") // Or unset, acting as production

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected an error when DB_SSLMODE=disable outside development, but got nil")
	}
	if !strings.Contains(err.Error(), "DB_SSLMODE=disable") {
		t.Errorf("expected error to mention DB_SSLMODE=disable, got: %v", err)
	}
}

func TestLoadConfig_RejectsInvalidTokenInsecureSkipVerify(t *testing.T) {
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)
	t.Setenv("NSW_TOKEN_INSECURE_SKIP_VERIFY", "not-a-bool")

	_, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != "invalid value for NSW_TOKEN_INSECURE_SKIP_VERIFY: \"not-a-bool\"" {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- insecure-TLS guard (APP_ENV) ---
//
// AUTH_JWKS_INSECURE_SKIP_VERIFY and NSW_TOKEN_INSECURE_SKIP_VERIFY are only
// honored when APP_ENV=development; LoadConfig is the sole place APP_ENV is
// read (isDevEnvironment, below), so it is also the sole enforcement point —
// downstream (auth, nswclient, pkg/httpclient) simply trusts the flag it's
// handed. Unset/any other value is treated as production and fails closed.

func TestLoadConfig_NSWInsecureSkipVerify_FailsClosedOutsideDev(t *testing.T) {
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)
	t.Setenv("NSW_TOKEN_INSECURE_SKIP_VERIFY", "true")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "NSW_TOKEN_INSECURE_SKIP_VERIFY") {
		t.Fatalf("expected LoadConfig to fail closed with an NSW_TOKEN_INSECURE_SKIP_VERIFY guard error, got: %v", err)
	}
}

// AUTH_EXPECTED_OU is compared to the token's ouHandle verbatim, so it must be
// normalised here, where it is read. A stray space or newline (easily introduced
// by an env file or a secret manager) would otherwise deny every user with a
// blanket 403 that reads as an authorization bug rather than the configuration
// error it is.
func TestLoadConfig_TrimsExpectedOU(t *testing.T) {
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)
	t.Setenv("AUTH_EXPECTED_OU", "  fcau\n")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Authn.ExpectedOU != "fcau" {
		t.Fatalf("ExpectedOU = %q, want %q", cfg.Authn.ExpectedOU, "fcau")
	}
}

func TestLoadConfig_AuthJWKSInsecureSkipVerify_AllowedInDev(t *testing.T) {
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("AUTH_JWKS_INSECURE_SKIP_VERIFY", "true")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !cfg.Authn.InsecureSkipTLSVerify {
		t.Fatalf("expected InsecureSkipTLSVerify to be true")
	}
}

func TestLoadConfig_AuthJWKSInsecureSkipVerify_FailsClosedOutsideDev(t *testing.T) {
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)
	t.Setenv("AUTH_JWKS_INSECURE_SKIP_VERIFY", "true")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "AUTH_JWKS_INSECURE_SKIP_VERIFY") {
		t.Fatalf("expected LoadConfig to fail closed with an AUTH_JWKS_INSECURE_SKIP_VERIFY guard error, got: %v", err)
	}
}

func TestLoadConfig_ServerTimeoutDefaults(t *testing.T) {
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)

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

func TestLoadConfig_ServerTimeoutsFromEnv(t *testing.T) {
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)
	t.Setenv("SERVER_READ_HEADER_TIMEOUT", "1s")
	t.Setenv("SERVER_READ_TIMEOUT", "2s")
	t.Setenv("SERVER_WRITE_TIMEOUT", "3s")
	t.Setenv("SERVER_IDLE_TIMEOUT", "4s")

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
	setBaseConfigEnv(t)
	setRequiredNSWOAuth2Env(t)
	setRequiredAuthEnv(t)
	t.Setenv("SERVER_READ_TIMEOUT", "not-a-duration")

	_, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != `invalid value for SERVER_READ_TIMEOUT: "not-a-duration"` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfig_RejectsInvalidMaxRequestBytes(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		expected string
	}{
		{name: "zero", value: "0", expected: `invalid value for MAX_REQUEST_BYTES: "0"`},
		{name: "negative", value: "-1", expected: `invalid value for MAX_REQUEST_BYTES: "-1"`},
		{name: "invalid integer", value: "abc", expected: `invalid value for MAX_REQUEST_BYTES: "abc"`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setBaseConfigEnv(t)
			setRequiredNSWOAuth2Env(t)
			setRequiredAuthEnv(t)
			t.Setenv("MAX_REQUEST_BYTES", tc.value)

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
			t.Setenv("APP_ENV", c.val)
			if got := isDevEnvironment(); got != c.want {
				t.Fatalf("isDevEnvironment() with APP_ENV=%q = %v, want %v", c.val, got, c.want)
			}
		})
	}
}
