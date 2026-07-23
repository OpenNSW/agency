package main

import (
	"os"

	"github.com/OpenNSW/nsw-agency/backend/internal/database"
)

// Config holds all configuration for the CLI command.
type Config struct {
	DB database.Config
}

// LoadConfig loads configuration from environment variables.
func LoadConfig() (Config, error) {
	cfg := Config{
		// Populate every driver's settings; database.Config reads only the
		// selected driver's sub-config and cfg.DB.Validate() enforces its
		// requirements, so there is no per-driver switch here.
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
