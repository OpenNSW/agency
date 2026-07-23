package main

import "testing"

func TestCLILoadConfig_Defaults(t *testing.T) {
	t.Setenv("DB_DRIVER", "")

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
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_PATH", "./custom.db")

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
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "admin")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "mydb")
	t.Setenv("DB_SSLMODE", "require")

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

func TestCLILoadConfig_Postgres_RequiresPassword(t *testing.T) {
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_PASSWORD", "")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error when DB_PASSWORD is missing, got nil")
	}
}

func TestCLILoadConfig_Postgres_DefaultSSLModeRequire(t *testing.T) {
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_SSLMODE", "") // Unset -> should default to require

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DB.Postgres.SSLMode != "require" {
		t.Errorf("DB.Postgres.SSLMode = %q, want require when unset", cfg.DB.Postgres.SSLMode)
	}
}

func TestCLILoadConfig_UnsupportedDriver(t *testing.T) {
	t.Setenv("DB_DRIVER", "mysql")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for unsupported driver, got nil")
	}
}
