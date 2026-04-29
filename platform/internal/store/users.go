package store

import (
	"context"
	"database/sql"
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
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
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
		`SELECT id, name, email, avatar_url, is_platform_admin, created_at, updated_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(&u.ID, &u.Name, &u.Email, &u.AvatarURL, &u.IsPlatformAdmin, &u.CreatedAt, &u.UpdatedAt)
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
		`SELECT id, name, email, avatar_url, is_platform_admin, created_at, updated_at
		 FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Name, &u.Email, &u.AvatarURL, &u.IsPlatformAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ListUsers returns all users ordered by creation date.
func (s *UserStore) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, email, avatar_url, is_platform_admin, created_at, updated_at
		 FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.AvatarURL, &u.IsPlatformAdmin, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if users == nil {
		users = []User{}
	}
	return users, rows.Err()
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
