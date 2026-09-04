package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OpenNSW/agency/backend/internal/authn"
	"github.com/OpenNSW/agency/backend/internal/configyaml"
	"github.com/OpenNSW/agency/backend/internal/database"
	"github.com/OpenNSW/agency/backend/internal/nswclient"
	"github.com/OpenNSW/agency/backend/internal/web"
	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/artifact/loaders/github"
	"github.com/OpenNSW/core/artifact/loaders/local"
	"github.com/OpenNSW/core/artifact/loaders/s3"
	"github.com/OpenNSW/core/refid"
	"gopkg.in/yaml.v3"
)

// defaultConfigPath is where LoadConfig looks for its YAML file when
// CONFIG_PATH is unset.
const defaultConfigPath = "./config.yaml"

type Config struct {
	Port              string
	DB                database.Config
	ArtifactLoader    loaders.Config
	AllowedOrigins    []string
	NSW               nswclient.Config
	Authn             authn.Config
	Web               web.Config
	MaxRequestBytes   int64
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	// ConsignmentCustomDataSchemaPath is an optional path to a JSON Schema
	// file validating consignments' merged custom_data (see
	// internal/consignment.Store.MergeCustomData). Empty means no schema is
	// configured for this deployment, so it's not validated.
	ConsignmentCustomDataSchemaPath string
	// DataScopeRulesPath is an optional path to a JSON file of
	// internal/datascope.Rule restricting which consignments/applications an
	// officer may see, based on their own users.custom_data. Empty means no
	// rules are configured for this deployment, so scoping is a no-op.
	DataScopeRulesPath string
	// RefIDGen declares the reference ID formats this deployment can issue
	// (see github.com/OpenNSW/core/refid, and internal/taskconfig's refid
	// block for how a task opts in). Empty means no format is configured, so
	// no task can generate one.
	RefIDGen refid.Config
	// Environment designates the deployment environment. It exists solely to
	// gate the insecure-TLS/sslmode escape hatches (see isDevEnvironment) —
	// unset or any value other than "development" is treated as production.
	Environment string
}

// yamlConfig is the on-disk shape read from CONFIG_PATH. It's a separate type
// from Config because a few fields need translation plain yaml tags can't
// express: durations are authored as strings ("5s"), pointers distinguish an
// omitted value (use the default) from an explicit zero/invalid one (error),
// and ArtifactLoader's real type (loaders.Config) belongs to the external
// github.com/OpenNSW/core module and carries no yaml tags of its own.
type yamlConfig struct {
	Port                            string                   `yaml:"port"`
	AllowedOrigins                  []string                 `yaml:"allowedOrigins"`
	MaxRequestBytes                 *int64                   `yaml:"maxRequestBytes"`
	ReadHeaderTimeout               *yamlDuration            `yaml:"readHeaderTimeout"`
	ReadTimeout                     *yamlDuration            `yaml:"readTimeout"`
	WriteTimeout                    *yamlDuration            `yaml:"writeTimeout"`
	IdleTimeout                     *yamlDuration            `yaml:"idleTimeout"`
	ConsignmentCustomDataSchemaPath string                   `yaml:"consignmentCustomDataSchemaPath"`
	DataScopeRulesPath              string                   `yaml:"dataScopeRulesPath"`
	Environment                     string                   `yaml:"environment"`
	DB                              database.Config          `yaml:"db"`
	ArtifactLoader                  yamlArtifactLoaderConfig `yaml:"artifactLoader"`
	NSW                             nswclient.Config         `yaml:"nsw"`
	Authn                           authn.Config             `yaml:"authn"`
	Web                             web.Config               `yaml:"web"`
	RefIDGen                        refid.Config             `yaml:"refIDGen"`
}

// yamlArtifactLoaderConfig mirrors loaders.Config's shape (Type plus one
// sub-config per backend) purely for YAML decoding. loaders.Config itself is
// owned by github.com/OpenNSW/core and has no yaml tags, so this local copy
// is unmarshalled instead and converted via toLoadersConfig. Keep it in sync
// with loaders.Config/local.Config/github.Config/s3.Config if core changes
// their shape.
type yamlArtifactLoaderConfig struct {
	Type   string                 `yaml:"type"`
	Local  yamlLocalLoaderConfig  `yaml:"local"`
	GitHub yamlGitHubLoaderConfig `yaml:"github"`
	S3     yamlS3LoaderConfig     `yaml:"s3"`
}

type yamlLocalLoaderConfig struct {
	Root string `yaml:"root"`
}

type yamlGitHubLoaderConfig struct {
	Owner      string `yaml:"owner"`
	Repo       string `yaml:"repo"`
	Ref        string `yaml:"ref"`
	BasePath   string `yaml:"basePath"`
	Token      string `yaml:"token"`
	BaseURL    string `yaml:"baseURL"`
	UseRawHost bool   `yaml:"useRawHost"`
	RawBaseURL string `yaml:"rawBaseURL"`
}

type yamlS3LoaderConfig struct {
	Bucket    string `yaml:"bucket"`
	Region    string `yaml:"region"`
	Endpoint  string `yaml:"endpoint"`
	AccessKey string `yaml:"accessKey"`
	SecretKey string `yaml:"secretKey"`
	Prefix    string `yaml:"prefix"`
}

func (c yamlArtifactLoaderConfig) toLoadersConfig() loaders.Config {
	return loaders.Config{
		Type: c.Type,
		Local: local.Config{
			Root: c.Local.Root,
		},
		GitHub: github.Config{
			Owner:      c.GitHub.Owner,
			Repo:       c.GitHub.Repo,
			Ref:        c.GitHub.Ref,
			BasePath:   c.GitHub.BasePath,
			Token:      c.GitHub.Token,
			BaseURL:    c.GitHub.BaseURL,
			UseRawHost: c.GitHub.UseRawHost,
			RawBaseURL: c.GitHub.RawBaseURL,
		},
		S3: s3.Config{
			Bucket:    c.S3.Bucket,
			Region:    c.S3.Region,
			Endpoint:  c.S3.Endpoint,
			AccessKey: c.S3.AccessKey,
			SecretKey: c.S3.SecretKey,
			Prefix:    c.S3.Prefix,
		},
	}
}

// yamlDuration decodes a YAML string like "5s" into a time.Duration. Plain
// yaml tags can't do this: time.Duration's underlying kind is int64, so yaml
// would otherwise try to parse the string as a number and fail.
type yamlDuration time.Duration

func (d *yamlDuration) UnmarshalYAML(value *yaml.Node) error {
	var raw string
	if err := value.Decode(&raw); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", raw, err)
	}
	*d = yamlDuration(parsed)
	return nil
}

// LoadConfig loads configuration from the YAML file at CONFIG_PATH (default
// "./config.yaml"), the only setting that has to be an env var since it's
// needed to find the file itself. Any scalar value in the file may be a
// "{{env:NAME}}" (read from an env var) or "{{file:/path}}" (read from a
// mounted file) placeholder instead of a literal — see
// internal/configyaml.LoadAndExpand — which is how secrets
// (db.postgres.password, nsw.clientSecret, ...) are kept out of the file;
// nothing here decides which fields are secrets, that's up to however
// config.yaml was authored. cmd/migrate reads the same file the same way,
// for its db section.
func LoadConfig() (Config, error) {
	path := envOrDefault("CONFIG_PATH", defaultConfigPath)

	var raw yamlConfig
	if err := configyaml.LoadAndExpand(path, &raw); err != nil {
		return Config{}, err
	}

	maxRequestBytes := int64(32 << 20)
	if raw.MaxRequestBytes != nil {
		maxRequestBytes = *raw.MaxRequestBytes
	}
	if maxRequestBytes <= 0 {
		return Config{}, fmt.Errorf("maxRequestBytes must be greater than zero, got %d", maxRequestBytes)
	}

	readHeaderTimeout, err := resolveDuration(raw.ReadHeaderTimeout, 5*time.Second, "readHeaderTimeout")
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := resolveDuration(raw.ReadTimeout, 15*time.Second, "readTimeout")
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := resolveDuration(raw.WriteTimeout, 30*time.Second, "writeTimeout")
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := resolveDuration(raw.IdleTimeout, 60*time.Second, "idleTimeout")
	if err != nil {
		return Config{}, err
	}

	allowedOrigins := raw.AllowedOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}

	port := raw.Port
	if port == "" {
		port = "8081"
	}

	db := raw.DB
	if db.Driver == "" {
		db.Driver = "sqlite"
	}
	if db.SQLite.Path == "" {
		db.SQLite.Path = "./agency_applications.db"
	}
	if db.Postgres.SSLMode == "" {
		db.Postgres.SSLMode = "require"
	}

	artifactLoader := raw.ArtifactLoader
	if artifactLoader.Type == "" {
		artifactLoader.Type = loaders.TypeLocal
	}

	web := raw.Web
	if web.Dir == "" {
		web.Dir = "web"
	}

	cfg := Config{
		Port:                            port,
		ConsignmentCustomDataSchemaPath: raw.ConsignmentCustomDataSchemaPath,
		DataScopeRulesPath:              raw.DataScopeRulesPath,
		Environment:                     raw.Environment,
		RefIDGen:                        raw.RefIDGen,
		DB:                              db,
		ArtifactLoader:                  artifactLoader.toLoadersConfig(),
		AllowedOrigins:                  allowedOrigins,
		NSW:                             raw.NSW,
		Authn:                           raw.Authn,
		Web:                             web,
		MaxRequestBytes:                 maxRequestBytes,
		ReadHeaderTimeout:               readHeaderTimeout,
		ReadTimeout:                     readTimeout,
		WriteTimeout:                    writeTimeout,
		IdleTimeout:                     idleTimeout,
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Validate enforces the settings that depend on more than one value (the
// dev-only insecure-TLS escape hatches, and disabling Postgres SSL) and
// delegates the rest to each sub-config's own Validate.
func (c Config) Validate() error {
	if c.NSW.TokenInsecureSkipVerify && !isDevEnvironment(c.Environment) {
		return fmt.Errorf("nsw.tokenInsecureSkipVerify: insecure TLS verification requested but environment is not \"development\" (unset or any other value is treated as production); refusing to start — provide a trusted certificate chain, or set environment: development for a non-production run")
	}
	if c.Authn.InsecureSkipTLSVerify && !isDevEnvironment(c.Environment) {
		return fmt.Errorf("authn.insecureSkipTLSVerify: insecure TLS verification requested but environment is not \"development\" (unset or any other value is treated as production); refusing to start — provide a trusted certificate chain, or set environment: development for a non-production run")
	}
	if c.DB.Driver == "postgres" && c.DB.Postgres.SSLMode == "disable" && !isDevEnvironment(c.Environment) {
		return fmt.Errorf("db.postgres.sslMode=disable: insecure database connection requested but environment is not \"development\" (unset or any other value is treated as production); refusing to start — use a secure SSL mode like \"require\", or set environment: development for a non-production run")
	}

	if err := c.ArtifactLoader.Validate(); err != nil {
		return err
	}
	if err := c.DB.Validate(); err != nil {
		return err
	}
	if err := c.NSW.Validate(); err != nil {
		return err
	}
	if err := c.Authn.Validate(); err != nil {
		return err
	}

	return nil
}

// resolveDuration applies defaultValue when raw is nil (the yaml key was
// omitted) and rejects an explicit non-positive duration.
func resolveDuration(raw *yamlDuration, defaultValue time.Duration, field string) (time.Duration, error) {
	if raw == nil {
		return defaultValue, nil
	}
	value := time.Duration(*raw)
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", field)
	}
	return value, nil
}

func envOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// isDevEnvironment reports whether environment explicitly designates a
// development run (case-insensitive "development"). Unset or any other value
// is treated as production. It exists solely to gate the insecure-TLS
// escape hatches above, which must never be honored outside an explicit
// development run.
func isDevEnvironment(environment string) bool {
	return strings.EqualFold(strings.TrimSpace(environment), "development")
}
