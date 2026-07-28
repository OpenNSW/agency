package database

import (
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// SQLiteConnector implements DBConnector for SQLite.
type SQLiteConnector struct {
	Path string
}

// Open establishes a connection to the SQLite database.
func (c *SQLiteConnector) Open() (*gorm.DB, error) {
	path := c.Path
	if path == "" {
		path = "agency_applications.db"
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance for pooling: %w", err)
	}

	// SQLite admits a single writer, and ":memory:" gives every connection its
	// own private database. Capping the pool at one connection keeps concurrent
	// callers serialized on the same database instead of racing for the write
	// lock or, in tests, silently diverging.
	sqlDB.SetMaxOpenConns(1)

	return db, nil
}
