package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// User represents a row in the users table.
type User struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Email           string    `json:"email"`
	AvatarURL       *string   `json:"avatarUrl"`
	IsPlatformAdmin bool      `json:"isPlatformAdmin"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// AdminUser is the enriched user shape returned by platform-admin user endpoints.
type AdminUser struct {
	User
	OrgRole     *string `json:"orgRole"`
	OrgID       *string `json:"orgId"`
	OrgName     *string `json:"orgName"`
	HasPassword bool    `json:"hasPassword"`
}

// ListUsersFilter scopes the platform-admin user list.
type ListUsersFilter struct {
	Role  *string
	OrgID *string
}

// UserStore provides database operations for users.
type UserStore struct {
	db *sql.DB
}

// NewUserStore creates a new UserStore.
func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

// GetUserByID retrieves a user by ID.
func (s *UserStore) GetUserByID(ctx context.Context, userID string) (*User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, email, avatar_url, is_platform_admin, status, created_at, updated_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(&u.ID, &u.Name, &u.Email, &u.AvatarURL, &u.IsPlatformAdmin, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByEmail retrieves a user by email.
func (s *UserStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, email, avatar_url, is_platform_admin, status, created_at, updated_at
		 FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Name, &u.Email, &u.AvatarURL, &u.IsPlatformAdmin, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ListUsers returns enriched admin users ordered by creation date.
func (s *UserStore) ListUsers(ctx context.Context, filter ListUsersFilter) ([]AdminUser, error) {
	var args []any
	query := strings.Builder{}
	query.WriteString(`
		SELECT u.id, u.name, u.email, u.avatar_url, u.is_platform_admin, u.status,
		       u.created_at, u.updated_at,
		       m.role AS org_role, m.org_id, o.name AS org_name,
		       (u.password_hash IS NOT NULL AND u.password_hash != '') AS has_password
		FROM users u
		LEFT JOIN LATERAL (
			SELECT role, org_id, created_at
			FROM org_memberships
			WHERE user_id = u.id AND status = 'active'
			ORDER BY created_at ASC
			LIMIT 1
		) m ON TRUE
		LEFT JOIN organizations o ON o.id = m.org_id
		WHERE 1=1`)

	if filter.Role != nil {
		switch *filter.Role {
		case "platform_admin":
			query.WriteString(" AND u.is_platform_admin = TRUE")
		case "unassigned":
			query.WriteString(" AND m.role IS NULL AND u.is_platform_admin = FALSE")
		default:
			args = append(args, *filter.Role)
			query.WriteString(fmt.Sprintf(" AND m.role = $%d", len(args)))
		}
	}
	if filter.OrgID != nil {
		args = append(args, *filter.OrgID)
		query.WriteString(fmt.Sprintf(" AND m.org_id = $%d", len(args)))
	}
	query.WriteString(" ORDER BY u.created_at DESC")

	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []AdminUser
	for rows.Next() {
		var u AdminUser
		if err := scanAdminUser(rows, &u); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if users == nil {
		users = []AdminUser{}
	}
	return users, rows.Err()
}

// GetAdminUserByID retrieves one enriched admin user by ID.
func (s *UserStore) GetAdminUserByID(ctx context.Context, userID string) (*AdminUser, error) {
	var u AdminUser
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.name, u.email, u.avatar_url, u.is_platform_admin, u.status,
		       u.created_at, u.updated_at,
		       m.role AS org_role, m.org_id, o.name AS org_name,
		       (u.password_hash IS NOT NULL AND u.password_hash != '') AS has_password
		FROM users u
		LEFT JOIN LATERAL (
			SELECT role, org_id, created_at
			FROM org_memberships
			WHERE user_id = u.id AND status = 'active'
			ORDER BY created_at ASC
			LIMIT 1
		) m ON TRUE
		LEFT JOIN organizations o ON o.id = m.org_id
		WHERE u.id = $1`,
		userID,
	).Scan(
		&u.ID, &u.Name, &u.Email, &u.AvatarURL, &u.IsPlatformAdmin, &u.Status,
		&u.CreatedAt, &u.UpdatedAt, &u.OrgRole, &u.OrgID, &u.OrgName, &u.HasPassword,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateStatus updates a user's account status.
func (s *UserStore) UpdateStatus(ctx context.Context, userID string, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET status = $1, updated_at = NOW() WHERE id = $2`,
		status, userID,
	)
	return err
}

// UpdatePlatformAdmin updates a user's platform-admin flag.
func (s *UserStore) UpdatePlatformAdmin(ctx context.Context, userID string, isAdmin bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET is_platform_admin = $1, updated_at = NOW() WHERE id = $2`,
		isAdmin, userID,
	)
	return err
}

type adminUserScanner interface {
	Scan(dest ...any) error
}

func scanAdminUser(row adminUserScanner, u *AdminUser) error {
	return row.Scan(
		&u.ID, &u.Name, &u.Email, &u.AvatarURL, &u.IsPlatformAdmin, &u.Status,
		&u.CreatedAt, &u.UpdatedAt, &u.OrgRole, &u.OrgID, &u.OrgName, &u.HasPassword,
	)
}

// RegisterInput is the input for registering a new user.
//
// IntendedRole is optional ("teacher" | "student"); nil leaves the
// users.intended_role column NULL so the onboarding flow falls back
// to the role-selector menu (review 005 / plan 040 / plan 047).
type RegisterInput struct {
	Name         string  `json:"name"`
	Email        string  `json:"email"`
	Password     string  `json:"password"`
	IntendedRole *string `json:"intendedRole,omitempty"`
}

// RegisteredUser is the response from registering a user (no password hash).
type RegisteredUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// RegisterUser creates a new user with email/password credentials.
func (s *UserStore) RegisterUser(ctx context.Context, input RegisterInput) (*RegisteredUser, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
	now := time.Now()

	var user RegisteredUser
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO users (id, name, email, password_hash, intended_role, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, name, email`,
		id, input.Name, input.Email, string(hash), input.IntendedRole, now, now,
	).Scan(&user.ID, &user.Name, &user.Email)
	if err != nil {
		return nil, err
	}

	// Create auth_providers entry
	providerID := uuid.New().String()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO auth_providers (id, user_id, provider, provider_user_id, created_at)
		 VALUES ($1, $2, 'email', $3, $4)`,
		providerID, user.ID, user.ID, now,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}
