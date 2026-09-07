package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes yamlContent to a temp file and points CONFIG_PATH at
// it, so LoadConfig() reads exactly this fixture.
func writeConfig(t *testing.T, yamlContent string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o600); err != nil {
		t.Fatalf("writing config fixture: %v", err)
	}
	t.Setenv("CONFIG_PATH", path)
}

func TestCLILoadConfig_Defaults(t *testing.T) {
	writeConfig(t, "db:\n  driver: sqlite\n")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DB.Driver != "sqlite" {
		t.Errorf("DB.Driver = %q, want sqlite", cfg.DB.Driver)
	}
	if cfg.DB.SQLite.Path != "./agency_applications.db" {
		t.Errorf("DB.Path = %q, want ./agency_applications.db", cfg.DB.SQLite.Path)
	}
}

func TestCLILoadConfig_SQLite(t *testing.T) {
	writeConfig(t, `db:
  driver: sqlite
  sqlite:
    path: ./custom.db
`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DB.Driver != "sqlite" {
		t.Errorf("DB.Driver = %q, want sqlite", cfg.DB.Driver)
	}
	if cfg.DB.SQLite.Path != "./custom.db" {
		t.Errorf("DB.Path = %q, want ./custom.db", cfg.DB.SQLite.Path)
	}
}

func TestCLILoadConfig_Postgres(t *testing.T) {
	writeConfig(t, `db:
  driver: postgres
  postgres:
    host: db.example.com
    port: "5433"
    user: admin
    password: secret
    name: mydb
    sslMode: require
`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DB.Driver != "postgres" {
		t.Errorf("DB.Driver = %q, want postgres", cfg.DB.Driver)
	}
	if cfg.DB.Postgres.Host != "db.example.com" {
		t.Errorf("DB.Host = %q, want db.example.com", cfg.DB.Postgres.Host)
	}
	if cfg.DB.Postgres.Port != "5433" {
		t.Errorf("DB.Port = %q, want 5433", cfg.DB.Postgres.Port)
	}
	if cfg.DB.Postgres.User != "admin" {
		t.Errorf("DB.User = %q, want admin", cfg.DB.Postgres.User)
	}
	if cfg.DB.Postgres.Password != "secret" {
		t.Errorf("DB.Password = %q, want secret", cfg.DB.Postgres.Password)
	}
	if cfg.DB.Postgres.Name != "mydb" {
		t.Errorf("DB.Name = %q, want mydb", cfg.DB.Postgres.Name)
	}
	if cfg.DB.Postgres.SSLMode != "require" {
		t.Errorf("DB.SSLMode = %q, want require", cfg.DB.Postgres.SSLMode)
	}
}

// The db.postgres.password field can be a "{{env:NAME}}" placeholder,
// resolved the same way cmd/server's and cmd/migrate's config.yaml resolve
// their secrets.
func TestCLILoadConfig_Postgres_ResolvesPasswordFromEnv(t *testing.T) {
	t.Setenv("CLI_TEST_DB_PASSWORD", "s3cr3t")
	writeConfig(t, `db:
  driver: postgres
  postgres:
    password: "{{env:CLI_TEST_DB_PASSWORD}}"
`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DB.Postgres.Password != "s3cr3t" {
		t.Errorf("DB.Password = %q, want s3cr3t", cfg.DB.Postgres.Password)
	}
}

func TestCLILoadConfig_Postgres_RequiresPassword(t *testing.T) {
	writeConfig(t, "db:\n  driver: postgres\n")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error when password is missing, got nil")
	}
}

func TestCLILoadConfig_Postgres_DefaultSSLModeRequire(t *testing.T) {
	writeConfig(t, `db:
  driver: postgres
  postgres:
    password: secret
`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DB.Postgres.SSLMode != "require" {
		t.Errorf("DB.Postgres.SSLMode = %q, want require when unset", cfg.DB.Postgres.SSLMode)
	}
}

func TestCLILoadConfig_UnsupportedDriver(t *testing.T) {
	writeConfig(t, "db:\n  driver: mysql\n")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for unsupported driver, got nil")
	}
}

func TestCLILoadConfig_MissingConfigFile(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "does-not-exist.yaml"))

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}

func TestCLILoadConfig_CustomDataSchemaPath(t *testing.T) {
	writeConfig(t, `db:
  driver: sqlite
userCustomDataSchemaPath: ./config/npqs/user-custom-data-schema.json
`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.CustomDataSchemaPath != "./config/npqs/user-custom-data-schema.json" {
		t.Errorf("CustomDataSchemaPath = %q, want ./config/npqs/user-custom-data-schema.json", cfg.CustomDataSchemaPath)
	}
}
