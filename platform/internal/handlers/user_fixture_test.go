package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/weiboz0/bridge/platform/internal/store"
)

const fixtureUserPassword = "testpassword123"

const fixtureUserPasswordHash = "$2b$04$zw3jz9jL6DE8zreLSsCR8OneBJLuYm1DgVvctVGcO7ioeysEyCsGa"

func insertFixtureUser(t *testing.T, db *sql.DB, input store.RegisterInput) *store.RegisteredUser {
	t.Helper()
	if err := validateFixtureUserInput(input); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	userID := uuid.NewString()
	now := time.Now()
	var user store.RegisteredUser
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (id, name, email, password_hash, intended_role, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, name, email`,
		userID, input.Name, input.Email, fixtureUserPasswordHash, input.IntendedRole, now, now,
	).Scan(&user.ID, &user.Name, &user.Email)
	if err != nil {
		t.Fatal(err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO auth_providers (id, user_id, provider, provider_user_id, created_at)
		 VALUES ($1, $2, 'email', $3, $4)`,
		uuid.NewString(), user.ID, user.ID, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	return &user
}

func validateFixtureUserInput(input store.RegisterInput) error {
	if input.Password != fixtureUserPassword {
		return fmt.Errorf("fixture users require password %q", fixtureUserPassword)
	}
	if input.IntendedRole != nil && *input.IntendedRole != "teacher" && *input.IntendedRole != "student" {
		return fmt.Errorf("fixture users require nil, teacher, or student intended role")
	}
	return nil
}

func TestInsertFixtureUser_PersistsHashRoleAndProvider(t *testing.T) {
	db := integrationDB(t)
	role := "teacher"
	input := store.RegisterInput{
		Name:         "Fixture User",
		Email:        "fixture-user-" + uuid.NewString() + "@example.com",
		Password:     fixtureUserPassword,
		IntendedRole: &role,
	}
	user := insertFixtureUser(t, db, input)
	t.Cleanup(func() { cleanupFixtureUser(t, db, user.ID) })

	var hash string
	var persistedRole sql.NullString
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT password_hash, intended_role FROM users WHERE id = $1", user.ID,
	).Scan(&hash, &persistedRole))
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte(fixtureUserPassword)))
	cost, err := bcrypt.Cost([]byte(hash))
	require.NoError(t, err)
	require.Equal(t, bcrypt.MinCost, cost)
	require.True(t, persistedRole.Valid)
	require.Equal(t, role, persistedRole.String)

	var provider, providerUserID string
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT provider, provider_user_id FROM auth_providers WHERE user_id = $1", user.ID,
	).Scan(&provider, &providerUserID))
	require.Equal(t, "email", provider)
	require.Equal(t, user.ID, providerUserID)
}

func TestInsertFixtureUser_RejectsUnsupportedInputWithoutWrite(t *testing.T) {
	db := integrationDB(t)
	before := fixtureUserRowCount(t, db)
	unsupportedRole := "parent"

	for _, input := range []store.RegisterInput{
		{Name: "Wrong password", Email: "wrong-password-" + uuid.NewString() + "@example.com", Password: "not-the-fixture-password"},
		{Name: "Wrong role", Email: "wrong-role-" + uuid.NewString() + "@example.com", Password: fixtureUserPassword, IntendedRole: &unsupportedRole},
	} {
		require.Error(t, validateFixtureUserInput(input))
	}

	require.Equal(t, before, fixtureUserRowCount(t, db))
}

func TestInsertFixtureUser_MatchesRegisterUserPersistenceContract(t *testing.T) {
	db := integrationDB(t)
	role := "student"
	fixtureInput := store.RegisterInput{
		Name:         "Fixture Contract",
		Email:        "fixture-contract-" + uuid.NewString() + "@example.com",
		Password:     fixtureUserPassword,
		IntendedRole: &role,
	}
	realInput := fixtureInput
	realInput.Email = "real-contract-" + uuid.NewString() + "@example.com"

	fixtureUser := insertFixtureUser(t, db, fixtureInput)
	realUser, err := store.NewUserStore(db).RegisterUser(context.Background(), realInput)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupFixtureUser(t, db, realUser.ID)
		cleanupFixtureUser(t, db, fixtureUser.ID)
	})

	fixtureShape := readFixtureUserShape(t, db, fixtureUser.ID)
	realShape := readFixtureUserShape(t, db, realUser.ID)
	require.Equal(t, fixtureInput.Name, fixtureShape.name)
	require.Equal(t, realInput.Name, realShape.name)
	require.Equal(t, fixtureInput.Email, fixtureShape.email)
	require.Equal(t, realInput.Email, realShape.email)
	require.Equal(t, fixtureShape.intendedRole, realShape.intendedRole)
	require.Equal(t, "email", fixtureShape.provider)
	require.Equal(t, fixtureShape.provider, realShape.provider)
	require.Equal(t, fixtureUser.ID, fixtureShape.providerUserID)
	require.Equal(t, realUser.ID, realShape.providerUserID)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(fixtureShape.passwordHash), []byte(fixtureUserPassword)))
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(realShape.passwordHash), []byte(fixtureUserPassword)))
}

type fixtureUserShape struct {
	name           string
	email          string
	passwordHash   string
	intendedRole   sql.NullString
	provider       string
	providerUserID string
}

func readFixtureUserShape(t *testing.T, db *sql.DB, userID string) fixtureUserShape {
	t.Helper()
	var shape fixtureUserShape
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT u.name, u.email, u.password_hash, u.intended_role, p.provider, p.provider_user_id
		FROM users u JOIN auth_providers p ON p.user_id = u.id
		WHERE u.id = $1`, userID,
	).Scan(&shape.name, &shape.email, &shape.passwordHash, &shape.intendedRole, &shape.provider, &shape.providerUserID))
	return shape
}

func fixtureUserRowCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT count(*) FROM users").Scan(&count))
	return count
}

func cleanupFixtureUser(t *testing.T, db *sql.DB, userID string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), "DELETE FROM auth_providers WHERE user_id = $1", userID)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	require.NoError(t, err)
}
