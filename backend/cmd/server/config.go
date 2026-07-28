package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/artifact/loaders/github"
	"github.com/OpenNSW/core/artifact/loaders/local"
	"github.com/OpenNSW/core/artifact/loaders/s3"
	"github.com/OpenNSW/nsw-agency/backend/internal/auth"
	"github.com/OpenNSW/nsw-agency/backend/internal/database"
	"github.com/OpenNSW/nsw-agency/backend/internal/nswclient"
	"github.com/OpenNSW/nsw-agency/backend/internal/web"
)

type Config struct {
	Port              string
	DB                database.Config
	ArtifactLoader    loaders.Config
	AllowedOrigins    []string
	NSW               nswclient.Config
	Auth              auth.Config
	Web               web.Config
	MaxRequestBytes   int64
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (Config, error) {

	cfg := Config{
		Port: envOrDefault("PORT", "8081"),
		// Populate every driver's settings from the environment; database.Config
		// reads only the selected driver's sub-config, and cfg.DB.Validate()
		// (below) enforces its requirements. This mirrors how the artifact loader
		// config is populated and validated, so there is no per-driver switch here.
		DB: database.Config{
			Driver: envOrDefault("DB_DRIVER", "sqlite"),
			SQLite: database.SQLiteConfig{
				Path: envOrDefault("DB_PATH", "./agency_applications.db"),
			},
			Postgres: database.PostgresConfig{
				Host:     envOrDefault("DB_HOST", "localhost"),
				Port:     envOrDefault("DB_PORT", "5432"),
				User:     envOrDefault("DB_USER", "postgres"),
				Password: os.Getenv("DB_PASSWORD"),
				Name:     envOrDefault("DB_NAME", "nsw_agency_db"),
				SSLMode:  envOrDefault("DB_SSLMODE", "require"),
			},
		},
		ArtifactLoader: loaders.Config{
			Type: envOrDefault("ARTIFACT_LOADER_TYPE", loaders.TypeLocal),
			Local: local.Config{
				// No default: artifacts live outside this repo, so the root must
				// be provided explicitly (by start-dev.sh or the deployment env).
				// An empty Root fails Validate with "Root is required".
				Root: os.Getenv("ARTIFACT_LOCAL_ROOT"),
			},
			GitHub: github.Config{
				Owner:      envOrDefault("ARTIFACT_GITHUB_OWNER", ""),
				Repo:       envOrDefault("ARTIFACT_GITHUB_REPO", ""),
				Ref:        envOrDefault("ARTIFACT_GITHUB_REF", ""),
				BasePath:   envOrDefault("ARTIFACT_GITHUB_BASE_PATH", ""),
				Token:      os.Getenv("ARTIFACT_GITHUB_TOKEN"),
				BaseURL:    envOrDefault("ARTIFACT_GITHUB_BASE_URL", ""),
				UseRawHost: getBoolOrDefault("ARTIFACT_GITHUB_USE_RAW_HOST", false),
				RawBaseURL: envOrDefault("ARTIFACT_GITHUB_RAW_BASE_URL", ""),
			},
			S3: s3.Config{
				Bucket:    envOrDefault("ARTIFACT_S3_BUCKET", ""),
				Region:    envOrDefault("ARTIFACT_S3_REGION", ""),
				Endpoint:  envOrDefault("ARTIFACT_S3_ENDPOINT", ""),
				AccessKey: envOrDefault("ARTIFACT_S3_ACCESS_KEY", ""),
				SecretKey: envOrDefault("ARTIFACT_S3_SECRET_KEY", ""),
				Prefix:    envOrDefault("ARTIFACT_S3_PREFIX", ""),
			},
		},
		AllowedOrigins: parseCommaSeparated(envOrDefault("ALLOWED_ORIGINS", "*")),
		NSW: nswclient.Config{
			BaseURL:      os.Getenv("NSW_API_BASE_URL"),
			ClientID:     os.Getenv("NSW_CLIENT_ID"),
			ClientSecret: os.Getenv("NSW_CLIENT_SECRET"),
			TokenURL:     os.Getenv("NSW_TOKEN_URL"),
			Scopes:       parseCommaSeparated(os.Getenv("NSW_SCOPES")),
		},
		Auth: auth.Config{
			JWKSURL:    os.Getenv("AUTH_JWKS_URL"),
			Issuer:     os.Getenv("AUTH_ISSUER"),
			Audience:   os.Getenv("AUTH_AUDIENCE"),
			ClientIDs:  parseCommaSeparated(os.Getenv("AUTH_CLIENT_IDS")),
			ExpectedOU: os.Getenv("AUTH_EXPECTED_OU"),
		},
		// Officer-portal SPA. WEB_DIR defaults to "web" (relative to the working
		// dir; /app/web in the image). The runtime config is served via
		// /runtime-env.js and validated only when the frontend is actually served
		// (see cmd/server/main.go), so API-only runs don't require these.
		Web: web.Config{
			Dir: envOrDefault("WEB_DIR", "web"),
			Runtime: web.RuntimeConfig{
				BrandingName:  os.Getenv("VITE_BRANDING_NAME"),
				APIBaseURL:    os.Getenv("VITE_API_BASE_URL"),
				IDPBaseURL:    os.Getenv("VITE_IDP_BASE_URL"),
				IDPClientID:   os.Getenv("VITE_IDP_CLIENT_ID"),
				IDPExpectedOU: os.Getenv("VITE_IDP_EXPECTED_OU_HANDLE"),
				AppURL:        os.Getenv("VITE_APP_URL"),
				IDPScopes:     os.Getenv("VITE_IDP_SCOPES"),
			},
		},
	}
	maxRequestBytes, err := parseInt64Env("MAX_REQUEST_BYTES", 32<<20)
	if err != nil {
		return Config{}, err
	}
	cfg.MaxRequestBytes = maxRequestBytes

	readHeaderTimeout, err := parseDurationEnv("SERVER_READ_HEADER_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg.ReadHeaderTimeout = readHeaderTimeout

	readTimeout, err := parseDurationEnv("SERVER_READ_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg.ReadTimeout = readTimeout

	writeTimeout, err := parseDurationEnv("SERVER_WRITE_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg.WriteTimeout = writeTimeout

	idleTimeout, err := parseDurationEnv("SERVER_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg.IdleTimeout = idleTimeout

	tokenInsecureSkipVerify, err := parseBoolEnv("NSW_TOKEN_INSECURE_SKIP_VERIFY", false)
	if err != nil {
		return Config{}, err
	}
	if tokenInsecureSkipVerify && !isDevEnvironment() {
		return Config{}, fmt.Errorf("NSW_TOKEN_INSECURE_SKIP_VERIFY: insecure TLS verification requested but APP_ENV is not \"development\" (unset or any other value is treated as production); refusing to start — provide a trusted certificate chain, or set APP_ENV=development for a non-production run")
	}
	cfg.NSW.TokenInsecureSkipVerify = tokenInsecureSkipVerify

	authInsecureSkipTLSVerify, err := parseBoolEnv("AUTH_JWKS_INSECURE_SKIP_VERIFY", false)
	if err != nil {
		return Config{}, err
	}
	if authInsecureSkipTLSVerify && !isDevEnvironment() {
		return Config{}, fmt.Errorf("AUTH_JWKS_INSECURE_SKIP_VERIFY: insecure TLS verification requested but APP_ENV is not \"development\" (unset or any other value is treated as production); refusing to start — provide a trusted certificate chain, or set APP_ENV=development for a non-production run")
	}
	cfg.Auth.InsecureSkipTLSVerify = authInsecureSkipTLSVerify
	if cfg.DB.Driver == "postgres" && cfg.DB.Postgres.SSLMode == "disable" && !isDevEnvironment() {
		return Config{}, fmt.Errorf("DB_SSLMODE=disable: insecure database connection requested but APP_ENV is not \"development\" (unset or any other value is treated as production); refusing to start — use a secure SSL mode like \"require\", or set APP_ENV=development for a non-production run")
	}
	if err := cfg.ArtifactLoader.Validate(); err != nil {
		return Config{}, err
	}
	if err := cfg.DB.Validate(); err != nil {
		return Config{}, err
	}
	if err := cfg.NSW.Validate(); err != nil {
		return Config{}, err
	}
	if err := cfg.Auth.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func envOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseCommaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseBoolEnv(key string, defaultValue bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid value for %s: %q", key, raw)
	}

	return value, nil
}

func parseInt64Env(key string, defaultValue int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue, nil
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %q", key, raw)
	}
	if value <= 0 {
		return 0, fmt.Errorf("invalid value for %s: %q", key, raw)
	}

	return value, nil
}

func parseDurationEnv(key string, defaultValue time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %q", key, raw)
	}
	if value <= 0 {
		return 0, fmt.Errorf("invalid value for %s: %q (must be greater than zero)", key, raw)
	}

	return value, nil
}

// getBoolOrDefault returns the boolean value of an environment variable or a default value.
// Invalid values are silently ignored and the default is returned.
func getBoolOrDefault(key string, defaultValue bool) bool {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// isDevEnvironment reports whether APP_ENV explicitly designates a development
// run (case-insensitive "development"). Unset or any other value is treated as
// production. This is the only place APP_ENV is read; it exists solely to gate
// the insecure-TLS escape hatches above, which must never be honored outside
// an explicit development run.
func isDevEnvironment() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "development")
}
