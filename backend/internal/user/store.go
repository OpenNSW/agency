package user

import (
	"errors"
	"fmt"
	"time"

	"github.com/OpenNSW/agency/backend/internal/database"
	"github.com/OpenNSW/agency/backend/pkg/dbtype"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrUserNotFound is returned when no user with the given email exists in the
// database. Users must be pre-seeded via the seed CLI before they can log in.
var ErrUserNotFound = errors.New("user not found — ensure the user has been seeded")

type UserRecord struct {
	UserID     string       `gorm:"type:text;primaryKey"`
	SSOID      *string      `gorm:"column:ssoid;type:text;uniqueIndex"`
	Email      string       `gorm:"type:text"`
	Name       string       `gorm:"type:text"`
	CustomData dbtype.JSONB `gorm:"column:custom_data;type:jsonb"` // agency-specific fields, validated against a configured JSON Schema
	CreatedAt  time.Time    `gorm:"autoCreateTime"`
	UpdatedAt  time.Time    `gorm:"autoUpdateTime"`
}

func (UserRecord) TableName() string {
	return "users"
}

// BeforeCreate generates a UUID v4 for UserID if one is not already set.
func (u *UserRecord) BeforeCreate(_ *gorm.DB) error {
	if u.UserID == "" {
		u.UserID = uuid.New().String()
	}
	return nil
}

type UserStore struct {
	db *gorm.DB
}

func NewUserStore(dbCfg database.Config) (*UserStore, error) {
	connector, err := database.NewConnector(dbCfg)
	if err != nil {
		return nil, err
	}

	db, err := connector.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &UserStore{db: db}, nil
}

// GetOrCreateUser resolves an authenticated caller to their seeded user record,
// backing authn.UserProfileService through cmd/server's adapter. It finds the
// user by email, syncs their SSOID from the token if not yet set, and returns
// the internal UserID. Returns an error if the user has not been seeded.
//
// It takes only the fields it persists, and makes no authorization decision.
// The caller's OU is checked by internal/authn, ahead of resolution, so a
// cross-agency token never reaches this store; and each agency has its own
// database, so a record found here belongs to this agency by construction.
func (s *UserStore) GetOrCreateUser(idpUserID, email, givenName string) (*string, error) {
	u, err := s.findAndSync(idpUserID, email, givenName)
	if err != nil {
		return nil, err
	}
	return &u.UserID, nil
}

// findAndSync looks up a pre-seeded user by email and syncs their SSOID from
// the token on first login. Returns ErrUserNotFound if no matching user exists.
func (s *UserStore) findAndSync(ssoid, email, name string) (*UserRecord, error) {
	var user UserRecord
	if err := s.db.First(&user, "email = ?", email).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	updates := map[string]any{}

	// Sync SSOID from token on first login (when not yet set).
	if ssoid != "" && (user.SSOID == nil || *user.SSOID == "") {
		updates["ssoid"] = ssoid
	}

	// Sync name if provided and changed.
	if name != "" && user.Name != name {
		updates["name"] = name
	}

	if len(updates) > 0 {
		if err := s.db.Model(&user).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to sync user attributes: %w", err)
		}
		if _, ok := updates["ssoid"]; ok {
			user.SSOID = &ssoid
		}
		if _, ok := updates["name"]; ok {
			user.Name = name
		}
	}

	return &user, nil
}

func (s *UserStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
