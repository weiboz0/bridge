package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserStore_RegisterAndGetByEmail(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db)
	ctx := context.Background()

	email := "register-test-" + t.Name() + "@example.com"
	user, err := store.RegisterUser(ctx, RegisterInput{
		Name:     "Test User",
		Email:    email,
		Password: "securepassword123",
	})
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "Test User", user.Name)
	assert.Equal(t, email, user.Email)

	// Get by email
	fetched, err := store.GetUserByEmail(ctx, email)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, user.ID, fetched.ID)

	// Get by ID
	fetchedByID, err := store.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedByID)
	assert.Equal(t, user.ID, fetchedByID.ID)

	// Cleanup
	_, _ = db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", user.ID)
	_, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
}

func TestUserStore_RegisterUser_DuplicateEmail(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db)
	ctx := context.Background()

	email := "dup-test-" + t.Name() + "@example.com"
	user, err := store.RegisterUser(ctx, RegisterInput{
		Name: "First User", Email: email, Password: "password123",
	})
	require.NoError(t, err)
	require.NotNil(t, user)

	// Second registration should fail (unique constraint on email)
	_, err = store.RegisterUser(ctx, RegisterInput{
		Name: "Second User", Email: email, Password: "password456",
	})
	assert.Error(t, err)

	// Cleanup
	_, _ = db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", user.ID)
	_, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
}

func TestUserStore_GetUserByID_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db)
	ctx := context.Background()

	user, err := store.GetUserByID(ctx, "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, user)
}

func TestUserStore_GetUserByEmail_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db)
	ctx := context.Background()

	user, err := store.GetUserByEmail(ctx, "nonexistent@example.com")
	assert.NoError(t, err)
	assert.Nil(t, user)
}
