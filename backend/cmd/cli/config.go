package main

import (
	"os"

	"github.com/OpenNSW/agency/backend/internal/configyaml"
	"github.com/OpenNSW/agency/backend/internal/database"
)

// defaultConfigPath is where LoadConfig looks for its YAML file when
// CONFIG_PATH is unset. Same file and default as cmd/server/cmd/migrate —
// this command only reads its db and userCustomDataSchemaPath fields from
// it, ignoring everything else.
const defaultConfigPath = "./config.yaml"

// Config holds all configuration for the CLI command.
type Config struct {
	DB database.Config `yaml:"db"`
	// CustomDataSchemaPath is an optional path to a JSON Schema file that
	// validates BulkInput.CustomData on `user add`. Empty means no schema is
	// configured for this deployment, so custom data is not validated.
	CustomDataSchemaPath string `yaml:"userCustomDataSchemaPath"`
}

// LoadConfig loads configuration from the YAML file at CONFIG_PATH (default
// "./config.yaml") — the same file, and the same "{{env:NAME}}" /
// "{{file:/path}}" placeholder resolution, cmd/server and cmd/migrate use
// (see internal/configyaml.LoadAndExpand). Reading one shared file means a
// deployment's DB connection is defined in one place rather than duplicated
// across a separate CLI-only config.
func LoadConfig() (Config, error) {
	path := envOrDefault("CONFIG_PATH", defaultConfigPath)

	var cfg Config
	if err := configyaml.LoadAndExpand(path, &cfg); err != nil {
		return Config{}, err
	}

	if cfg.DB.Driver == "" {
		cfg.DB.Driver = "sqlite"
	}
	if cfg.DB.SQLite.Path == "" {
		cfg.DB.SQLite.Path = "./agency_applications.db"
	}
	if cfg.DB.Postgres.SSLMode == "" {
		cfg.DB.Postgres.SSLMode = "require"
	}

	if err := cfg.DB.Validate(); err != nil {
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
