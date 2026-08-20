package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/OpenNSW/nsw-agency/backend/internal/authn"
	"github.com/OpenNSW/nsw-agency/backend/internal/rbac"
	"gorm.io/gorm"
)

// BulkInput represents user data used for bulk creation.
type BulkInput struct {
	SSOID string
	Name  string
	Email string
	Roles []string
}

// UserService handles user seeding and management operations.
type UserService struct {
	db *gorm.DB
}

func NewUserService(db *gorm.DB) *UserService {
	return &UserService{db: db}
}

// CreateBulk creates users and their role assignments in a single transaction.
// Returns the number of newly created users.
func (s *UserService) CreateBulk(users []BulkInput) (int, error) {
	var inserted int
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		inserted, err = createBulkInTx(tx, users)
		return err
	})
	return inserted, err
}

// DropUser removes a user and their role assignments by email.
func (s *UserService) DropUser(email string) error {
	var u UserRecord
	if err := s.db.First(&u, "email = ?", email).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("no user found with email %q", email)
		}
		return fmt.Errorf("failed to find user: %w", err)
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", u.UserID).Delete(&rbac.UserRoleRecord{}).Error; err != nil {
			return fmt.Errorf("failed to remove role assignments for %q: %w", email, err)
		}
		if err := tx.Delete(&u).Error; err != nil {
			return fmt.Errorf("failed to drop user %q: %w", email, err)
		}
		return nil
	})
}

func createBulkInTx(tx *gorm.DB, users []BulkInput) (int, error) {
	roleService := rbac.NewRoleService(tx)

	// Deduplicate users by email — keep the first occurrence.
	seen := make(map[string]struct{})
	deduped := users[:0]
	for _, u := range users {
		if _, exists := seen[u.Email]; !exists {
			seen[u.Email] = struct{}{}
			deduped = append(deduped, u)
		}
	}
	users = deduped

	// Collect unique role names across all users.
	roleNames := make(map[string]struct{})
	for _, u := range users {
		for _, r := range u.Roles {
			roleNames[r] = struct{}{}
		}
	}

	// Upsert roles — create if not exists, reuse existing.
	roleIndex := make(map[string]*rbac.RoleRecord)
	for name := range roleNames {
		role, err := roleService.FindByName(name)
		if errors.Is(err, rbac.ErrRoleNotFound) {
			role, err = roleService.Create(name)
		}
		if err != nil {
			return 0, fmt.Errorf("upsert role %q: %w", name, err)
		}
		roleIndex[name] = role
	}

	// Upsert users and assign roles.
	inserted := 0
	for _, u := range users {
		var existing UserRecord
		err := tx.First(&existing, "email = ?", u.Email).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			newUser := UserRecord{Email: u.Email, Name: u.Name, SSOID: nullableSSID(u.SSOID)}
			if err := tx.Create(&newUser).Error; err != nil {
				return inserted, fmt.Errorf("create user %q: %w", u.Email, err)
			}
			existing = newUser
			inserted++
		} else if err != nil {
			return inserted, fmt.Errorf("fetch user %q: %w", u.Email, err)
		}

		for _, roleName := range u.Roles {
			role := roleIndex[roleName]
			if err := roleService.Assign(existing.UserID, role.ID); err != nil {
				return inserted, fmt.Errorf("assign role %q to user %q: %w", roleName, u.Email, err)
			}
		}
	}
	return inserted, nil
}

func nullableSSID(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ErrUnauthenticated is returned when the auth context is missing or invalid.
var ErrUnauthenticated = errors.New("unauthenticated")

// ---------- ProfileService ----------

// ProfileService handles request-time user profile queries.
type ProfileService struct {
	roleService *rbac.RoleService
}

// NewProfileService creates a ProfileService for use in HTTP handlers.
func NewProfileService(roleService *rbac.RoleService) *ProfileService {
	return &ProfileService{roleService: roleService}
}

// GetMe returns the authenticated user's email, name, and assigned roles.
func (s *ProfileService) GetMe(ctx context.Context) (map[string]any, error) {
	principal, authenticated := authn.FromContext(ctx)
	if !authenticated || principal.Kind != authn.KindUser {
		return nil, ErrUnauthenticated
	}

	roles, err := s.roleService.GetRolesForUser(principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get roles: %w", err)
	}

	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = r.Name
	}

	return map[string]any{
		"email": principal.Email,
		"name":  principal.GivenName,
		"roles": roleNames,
	}, nil
}
