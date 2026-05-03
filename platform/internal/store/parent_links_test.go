package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Plan 064 — ParentLinkStore integration tests. Run against
// `bridge_test` (DATABASE_URL gated).

// mkParentLinkUser inserts a fresh user via the store and registers
// cleanup. Each test gets its own users so parallel runs don't
// collide on emails.
func mkParentLinkUser(t *testing.T, ctx context.Context, users *UserStore, label string) *RegisteredUser {
	t.Helper()
	u, err := users.RegisterUser(ctx, RegisterInput{
		Name:     "PLink " + label,
		Email:    "plink-" + label + "-" + uuid.NewString()[:8] + "@example.com",
		Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		users.db.ExecContext(ctx, "DELETE FROM parent_links WHERE parent_user_id = $1 OR child_user_id = $1 OR created_by = $1", u.ID)
		users.db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
		users.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
	})
	return u
}

func TestParentLinkStore_CreateAndIsParentOf(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	store := NewParentLinkStore(db)
	ctx := context.Background()

	parent := mkParentLinkUser(t, ctx, users, "p-create")
	child := mkParentLinkUser(t, ctx, users, "c-create")
	admin := mkParentLinkUser(t, ctx, users, "a-create")

	// Pre-create: no link.
	ok, err := store.IsParentOf(ctx, parent.ID, child.ID)
	require.NoError(t, err)
	assert.False(t, ok, "no link yet → IsParentOf false")

	link, err := store.CreateLink(ctx, parent.ID, child.ID, admin.ID)
	require.NoError(t, err)
	require.NotNil(t, link)
	assert.Equal(t, "active", link.Status)
	assert.Equal(t, parent.ID, link.ParentUserID)
	assert.Equal(t, child.ID, link.ChildUserID)
	assert.Equal(t, admin.ID, link.CreatedBy)
	assert.Nil(t, link.RevokedAt)

	// Post-create: link grants IsParentOf.
	ok, err = store.IsParentOf(ctx, parent.ID, child.ID)
	require.NoError(t, err)
	assert.True(t, ok)

	// Reverse direction is NOT symmetric.
	ok, err = store.IsParentOf(ctx, child.ID, parent.ID)
	require.NoError(t, err)
	assert.False(t, ok, "child→parent must NOT pass (relation is directional)")
}

func TestParentLinkStore_CreateLink_RejectsDuplicateActive(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	store := NewParentLinkStore(db)
	ctx := context.Background()

	parent := mkParentLinkUser(t, ctx, users, "p-dup")
	child := mkParentLinkUser(t, ctx, users, "c-dup")
	admin := mkParentLinkUser(t, ctx, users, "a-dup")

	_, err := store.CreateLink(ctx, parent.ID, child.ID, admin.ID)
	require.NoError(t, err)

	_, err = store.CreateLink(ctx, parent.ID, child.ID, admin.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrParentLinkExists), "duplicate active link must return ErrParentLinkExists, got: %v", err)
}

func TestParentLinkStore_RevokeLink_ThenIsParentOfFalse(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	store := NewParentLinkStore(db)
	ctx := context.Background()

	parent := mkParentLinkUser(t, ctx, users, "p-revoke")
	child := mkParentLinkUser(t, ctx, users, "c-revoke")
	admin := mkParentLinkUser(t, ctx, users, "a-revoke")

	link, err := store.CreateLink(ctx, parent.ID, child.ID, admin.ID)
	require.NoError(t, err)

	revoked, err := store.RevokeLink(ctx, link.ID)
	require.NoError(t, err)
	require.NotNil(t, revoked)
	assert.Equal(t, "revoked", revoked.Status)
	assert.NotNil(t, revoked.RevokedAt)

	ok, err := store.IsParentOf(ctx, parent.ID, child.ID)
	require.NoError(t, err)
	assert.False(t, ok, "revoked link must NOT grant IsParentOf")
}

func TestParentLinkStore_ReLinkAfterRevoke(t *testing.T) {
	// Plan 064: partial-unique allows re-linking after revoke.
	// The old revoked row stays (audit); a new active row gets
	// inserted.
	db := testDB(t)
	users := NewUserStore(db)
	store := NewParentLinkStore(db)
	ctx := context.Background()

	parent := mkParentLinkUser(t, ctx, users, "p-relink")
	child := mkParentLinkUser(t, ctx, users, "c-relink")
	admin := mkParentLinkUser(t, ctx, users, "a-relink")

	first, err := store.CreateLink(ctx, parent.ID, child.ID, admin.ID)
	require.NoError(t, err)
	_, err = store.RevokeLink(ctx, first.ID)
	require.NoError(t, err)

	second, err := store.CreateLink(ctx, parent.ID, child.ID, admin.ID)
	require.NoError(t, err, "re-linking after revoke must succeed via the partial-unique index")
	assert.NotEqual(t, first.ID, second.ID, "re-link inserts a new row, not an update")

	ok, err := store.IsParentOf(ctx, parent.ID, child.ID)
	require.NoError(t, err)
	assert.True(t, ok, "fresh active link grants IsParentOf again")

	// Both rows should be in ListByParent (audit trail).
	links, err := store.ListByParent(ctx, parent.ID)
	require.NoError(t, err)
	assert.Len(t, links, 2, "both revoked and active rows visible in ListByParent")
}

func TestParentLinkStore_CreateLink_RejectsSelfLink(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	store := NewParentLinkStore(db)
	ctx := context.Background()

	user := mkParentLinkUser(t, ctx, users, "self")

	_, err := store.CreateLink(ctx, user.ID, user.ID, user.ID)
	assert.Error(t, err, "cannot link a user to themselves")
}

func TestParentLinkStore_CreateLink_RejectsEmptyArgs(t *testing.T) {
	db := testDB(t)
	store := NewParentLinkStore(db)
	ctx := context.Background()

	_, err := store.CreateLink(ctx, "", "child", "admin")
	assert.Error(t, err)
	_, err = store.CreateLink(ctx, "parent", "", "admin")
	assert.Error(t, err)
	_, err = store.CreateLink(ctx, "parent", "child", "")
	assert.Error(t, err)
}

func TestParentLinkStore_IsParentOf_EmptyArgs(t *testing.T) {
	db := testDB(t)
	store := NewParentLinkStore(db)
	ctx := context.Background()

	ok, err := store.IsParentOf(ctx, "", "child")
	assert.NoError(t, err)
	assert.False(t, ok)
	ok, err = store.IsParentOf(ctx, "parent", "")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestParentLinkStore_ListChildrenForParent(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	store := NewParentLinkStore(db)
	ctx := context.Background()

	parent := mkParentLinkUser(t, ctx, users, "p-list")
	child1 := mkParentLinkUser(t, ctx, users, "c1-list")
	child2 := mkParentLinkUser(t, ctx, users, "c2-list")
	child3 := mkParentLinkUser(t, ctx, users, "c3-list")
	admin := mkParentLinkUser(t, ctx, users, "a-list")

	// Active links to two children.
	_, err := store.CreateLink(ctx, parent.ID, child1.ID, admin.ID)
	require.NoError(t, err)
	_, err = store.CreateLink(ctx, parent.ID, child2.ID, admin.ID)
	require.NoError(t, err)
	// Revoked link to child3 — should NOT appear.
	revLink, err := store.CreateLink(ctx, parent.ID, child3.ID, admin.ID)
	require.NoError(t, err)
	_, err = store.RevokeLink(ctx, revLink.ID)
	require.NoError(t, err)

	ids, err := store.ListChildrenForParent(ctx, parent.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{child1.ID, child2.ID}, ids)
}

func TestParentLinkStore_GetLink_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewParentLinkStore(db)
	ctx := context.Background()

	link, err := store.GetLink(ctx, "00000000-0000-0000-0000-000000000099")
	assert.NoError(t, err)
	assert.Nil(t, link)
}

func TestParentLinkStore_RevokeLink_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewParentLinkStore(db)
	ctx := context.Background()

	link, err := store.RevokeLink(ctx, "00000000-0000-0000-0000-000000000099")
	assert.NoError(t, err)
	assert.Nil(t, link)
}

func TestParentLinkStore_RevokeLink_Idempotent(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	store := NewParentLinkStore(db)
	ctx := context.Background()

	parent := mkParentLinkUser(t, ctx, users, "p-idem")
	child := mkParentLinkUser(t, ctx, users, "c-idem")
	admin := mkParentLinkUser(t, ctx, users, "a-idem")

	link, err := store.CreateLink(ctx, parent.ID, child.ID, admin.ID)
	require.NoError(t, err)

	first, err := store.RevokeLink(ctx, link.ID)
	require.NoError(t, err)
	require.NotNil(t, first.RevokedAt)
	firstStamp := *first.RevokedAt

	// Re-revoke — revoked_at must NOT advance (COALESCE preserves
	// the original timestamp).
	second, err := store.RevokeLink(ctx, link.ID)
	require.NoError(t, err)
	require.NotNil(t, second.RevokedAt)
	assert.True(t, firstStamp.Equal(*second.RevokedAt), "re-revoke must NOT update revoked_at")
}
