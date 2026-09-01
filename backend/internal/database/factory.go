package database

import (
	"fmt"
	"strings"
)

// SQLiteConfig holds SQLite-specific settings.
type SQLiteConfig struct {
	Path string `yaml:"path"` // SQLite file path
}

// PostgresConfig holds PostgreSQL-specific settings.
type PostgresConfig struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
	SSLMode  string `yaml:"sslMode"`
}

// Config selects a database driver and carries the per-driver settings. Only
// the selected driver's sub-config is read; the others are ignored. This
// mirrors the artifact loaders.Config shape (a discriminator plus one
// sub-config per backend).
type Config struct {
	Driver   string         `yaml:"driver"` // "sqlite" or "postgres"
	SQLite   SQLiteConfig   `yaml:"sqlite"`
	Postgres PostgresConfig `yaml:"postgres"`
}

// Validate reports whether the selected driver is supported and its required
// settings are present. It is the single place driver requirements live, so
// callers (e.g. LoadConfig) no longer need a per-driver switch.
func (c Config) Validate() error {
	switch c.Driver {
	case "sqlite":
		return nil
	case "postgres":
		if strings.TrimSpace(c.Postgres.Password) == "" {
			return fmt.Errorf("database password secret is missing: DB_PASSWORD is required for postgres driver")
		}
		return nil
	default:
		return fmt.Errorf("unsupported database driver: %s", c.Driver)
	}
}

// NewConnector creates a new DBConnector based on the configuration driver.
func NewConnector(cfg Config) (DBConnector, error) {
	switch cfg.Driver {
	case "sqlite":
		return &SQLiteConnector{Path: cfg.SQLite.Path}, nil
	case "postgres":
		return &PostgresConnector{
			Host:     cfg.Postgres.Host,
			Port:     cfg.Postgres.Port,
			User:     cfg.Postgres.User,
			Password: cfg.Postgres.Password,
			Name:     cfg.Postgres.Name,
			SSLMode:  cfg.Postgres.SSLMode,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}
}
