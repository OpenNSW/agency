package database

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// PostgresConnector implements DBConnector for PostgreSQL.
type PostgresConnector struct {
	Host, Port, User, Password, Name, SSLMode string
}

// getEnvAsInt is a helper to safely parse environment variables to integers with a fallback.
func getEnvAsInt(key string, fallback int) int {
	if valStr := os.Getenv(key); valStr != "" {
		if val, err := strconv.Atoi(valStr); err == nil {
			return val
		}
	}
	return fallback
}

// Open establishes a connection to the PostgreSQL database with connection pooling.
func (c *PostgresConnector) Open() (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Configuring Connection Pooling for Production
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance for pooling: %w", err)
	}

	// Dynamically configure connection pool settings
	maxIdle := getEnvAsInt("AGENCY_DB_MAX_IDLE_CONNS", 10)
	maxOpen := getEnvAsInt("AGENCY_DB_MAX_OPEN_CONNS", 100)
	maxLifetimeMin := getEnvAsInt("AGENCY_DB_CONN_MAX_LIFETIME_MIN", 60)

	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetConnMaxLifetime(time.Duration(maxLifetimeMin) * time.Minute)

	return db, nil
}
