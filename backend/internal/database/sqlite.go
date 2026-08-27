package database

import (
	"github.com/glebarez/sqlite"
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
	return gorm.Open(sqlite.Open(path), &gorm.Config{})
}
