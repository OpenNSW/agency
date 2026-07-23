package main

import (
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"

	"github.com/OpenNSW/nsw-agency/backend/internal/database"
	"github.com/OpenNSW/nsw-agency/backend/internal/migrator"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "up":
		if err := runMigrateUp(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "down":
		if err := runMigrateDown(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "status":
		if err := runMigrateStatus(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func runMigrateUp() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	db, err := openDB(cfg.DB)
	if err != nil {
		return err
	}
	defer db.Close()

	m, err := migrator.New(db, cfg.Dir, cfg.DB.Driver)
	if err != nil {
		return err
	}
	return m.Up()
}

func runMigrateDown() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	db, err := openDB(cfg.DB)
	if err != nil {
		return err
	}
	defer db.Close()

	m, err := migrator.New(db, cfg.Dir, cfg.DB.Driver)
	if err != nil {
		return err
	}
	return m.Down()
}

func runMigrateStatus() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	db, err := openDB(cfg.DB)
	if err != nil {
		return err
	}
	defer db.Close()

	m, err := migrator.New(db, cfg.Dir, cfg.DB.Driver)
	if err != nil {
		return err
	}
	return m.Status()
}

func openDB(cfg database.Config) (*sql.DB, error) {
	var (
		db  *sql.DB
		err error
	)
	switch cfg.Driver {
	case "sqlite":
		db, err = sql.Open("sqlite3", cfg.SQLite.Path)
	case "postgres":
		pg := cfg.Postgres
		u := &url.URL{
			Scheme:   "postgres",
			User:     url.UserPassword(pg.User, pg.Password),
			Host:     net.JoinHostPort(pg.Host, pg.Port),
			Path:     "/" + pg.Name,
			RawQuery: "sslmode=" + url.QueryEscape(pg.SSLMode),
		}
		db, err = sql.Open("pgx", u.String())
	default:
		return nil, fmt.Errorf("unsupported driver %q", cfg.Driver)
	}
	if err != nil {
		return nil, database.SanitizeError(err, cfg.Postgres.Password)
	}
	if err := db.Ping(); err != nil {
		db.Close() //nolint:errcheck
		return nil, database.SanitizeError(fmt.Errorf("database unreachable: %w", err), cfg.Postgres.Password)
	}
	return db, nil
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage: migrate <command>

Commands:
  up               Apply all pending migrations
  down             Roll back the last applied migration
  status           Print the applied/pending state of all migrations
  generate <name>  Create a new migration file with the next version number

Environment variables:
  MIGRATION_DIR   Path to SQL migration files (default: ./migrations)
  DB_DRIVER       sqlite or postgres (default: sqlite)
  DB_PATH         SQLite file path (default: ./agency_applications.db)
  DB_HOST         PostgreSQL host (default: localhost)
  DB_PORT         PostgreSQL port (default: 5432)
  DB_USER         PostgreSQL user (default: postgres)
  DB_PASSWORD     PostgreSQL password (required for postgres)
  DB_NAME         PostgreSQL database name (default: nsw_agency_db)
  DB_SSLMODE      PostgreSQL SSL mode (default: require)
`)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "migrate: "+format+"\n", args...)
	os.Exit(1)
}
