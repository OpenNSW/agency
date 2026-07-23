package database

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestNewConnector(t *testing.T) {
	tests := []struct {
		name         string
		cfg          Config
		wantErr      bool
		expectedType string
	}{
		{
			name:         "valid sqlite",
			cfg:          Config{Driver: "sqlite", SQLite: SQLiteConfig{Path: ":memory:"}},
			wantErr:      false,
			expectedType: "*database.SQLiteConnector",
		},
		{
			name:         "valid postgres",
			cfg:          Config{Driver: "postgres"},
			wantErr:      false,
			expectedType: "*database.PostgresConnector",
		},
		{
			name:    "invalid driver",
			cfg:     Config{Driver: "mysql"},
			wantErr: true,
		},
		{
			name:    "empty driver",
			cfg:     Config{Driver: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector, err := NewConnector(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewConnector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if connector != nil {
					t.Error("expected nil connector on error")
				}
				return
			}
			if gotType := fmt.Sprintf("%T", connector); gotType != tt.expectedType {
				t.Errorf("NewConnector() = %v, want %v", gotType, tt.expectedType)
			}
		})
	}
}

func TestNewConnector_ErrorMessage(t *testing.T) {
	_, err := NewConnector(Config{Driver: "mysql"})
	if err == nil {
		t.Fatal("expected error for unsupported driver")
	}
	if !strings.Contains(err.Error(), "mysql") {
		t.Errorf("error message should contain driver name 'mysql', got: %s", err.Error())
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "sqlite ok", cfg: Config{Driver: "sqlite"}, wantErr: false},
		{
			name:    "postgres with password ok",
			cfg:     Config{Driver: "postgres", Postgres: PostgresConfig{Password: "secret"}},
			wantErr: false,
		},
		{
			name:    "postgres missing password",
			cfg:     Config{Driver: "postgres"},
			wantErr: true,
		},
		{name: "unsupported driver", cfg: Config{Driver: "mysql"}, wantErr: true},
		{name: "empty driver", cfg: Config{}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestPostgresDSN verifies postgres connector fields match config.
func TestPostgresDSN(t *testing.T) {
	cfg := Config{
		Driver: "postgres",
		Postgres: PostgresConfig{
			Host:     "localhost",
			Port:     "5432",
			User:     "testuser",
			Password: "testpassword",
			Name:     "testdb",
			SSLMode:  "disable",
		},
	}

	connector, err := NewConnector(cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	pgConn, ok := connector.(*PostgresConnector)
	if !ok {
		t.Fatal("expected *PostgresConnector type")
	}

	if pgConn.Host != "localhost" || pgConn.User != "testuser" {
		t.Errorf("config mismatch: %+v", pgConn)
	}
	if pgConn.SSLMode != "disable" {
		t.Errorf("expected SSLMode 'disable', got %q", pgConn.SSLMode)
	}
}

// TestStoreDecoupling verifies that store.go does not import any GORM driver
// packages directly, ensuring the store remains driver-agnostic.
func TestStoreDecoupling(t *testing.T) {
	content, err := os.ReadFile("../application/store.go")
	if err != nil {
		t.Fatalf("failed to read store.go: %v", err)
	}

	src := string(content)
	forbidden := []string{
		"gorm.io/driver/sqlite",
		"gorm.io/driver/postgres",
		"gorm.io/driver/mysql",
	}
	for _, pkg := range forbidden {
		if strings.Contains(src, pkg) {
			t.Errorf("store.go imports forbidden driver package %q — "+
				"the store should be decoupled from concrete drivers", pkg)
		}
	}
}

// Compile-time interface compliance checks.
var (
	_ DBConnector = (*SQLiteConnector)(nil)
	_ DBConnector = (*PostgresConnector)(nil)
)
