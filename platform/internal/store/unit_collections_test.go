package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupCollectionEnv creates a user + org and wires a UnitCollectionStore.
func setupCollectionEnv(t *testing.T, suffix string) (*UnitCollectionStore, *TeachingUnitStore, string /* orgID */, string /* userID */) {
	t.Helper()
	db := testDB(t)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	collections := NewUnitCollectionStore(db)
	units := NewTeachingUnitStore(db)

	org := createTestOrg(t, db, orgs, suffix)
	user := createTestUser(t, db, users, suffix)

	ctx := context.Background()
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM unit_collection_items WHERE collection_id IN (SELECT id FROM unit_collections WHERE created_by = $1)`, user.ID)
		db.ExecContext(ctx, `DELETE FROM unit_collections WHERE created_by = $1`, user.ID)
		db.ExecContext(ctx, `DELETE FROM teaching_units WHERE created_by = $1`, user.ID)
	})

	return collections, units, org.ID, user.ID
}

// ── CreateCollection ────────────────────────────────────────────────────────

func TestUnitCollectionStore_Create_Org(t *testing.T) {
	collections, _, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	c, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope:       "org",
		ScopeID:     &orgID,
		Title:       "My Collection",
		Description: "A curated set of units",
		CreatedBy:   userID,
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "org", c.Scope)
	require.NotNil(t, c.ScopeID)
	assert.Equal(t, orgID, *c.ScopeID)
	assert.Equal(t, "My Collection", c.Title)
	assert.Equal(t, "A curated set of units", c.Description)
	assert.Equal(t, userID, c.CreatedBy)
	assert.NotEmpty(t, c.ID)
}

func TestUnitCollectionStore_Create_Platform(t *testing.T) {
	collections, _, _, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	c, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope:     "platform",
		Title:     "Platform Collection",
		CreatedBy: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "platform", c.Scope)
	assert.Nil(t, c.ScopeID)
}

func TestUnitCollectionStore_Create_Personal(t *testing.T) {
	collections, _, _, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	c, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Personal Collection",
		CreatedBy: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "personal", c.Scope)
	require.NotNil(t, c.ScopeID)
	assert.Equal(t, userID, *c.ScopeID)
}

func TestUnitCollectionStore_Create_CheckConstraint(t *testing.T) {
	collections, _, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	// scope=platform + scope_id non-nil should be rejected.
	_, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope:     "platform",
		ScopeID:   &orgID,
		Title:     "Bad Collection",
		CreatedBy: userID,
	})
	require.Error(t, err, "expected CHECK constraint violation")
	assert.Contains(t, err.Error(), "23514")
}

// ── GetCollection ───────────────────────────────────────────────────────────

func TestUnitCollectionStore_GetCollection(t *testing.T) {
	collections, _, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	created, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope:     "org",
		ScopeID:   &orgID,
		Title:     "Get Test",
		CreatedBy: userID,
	})
	require.NoError(t, err)

	got, err := collections.GetCollection(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "Get Test", got.Title)
}

func TestUnitCollectionStore_GetCollection_NotFound(t *testing.T) {
	collections, _, _, _ := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	got, err := collections.GetCollection(ctx, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// ── UpdateCollection ────────────────────────────────────────────────────────

func TestUnitCollectionStore_UpdateCollection_Partial(t *testing.T) {
	collections, _, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	c, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope:       "org",
		ScopeID:     &orgID,
		Title:       "Original",
		Description: "Original desc",
		CreatedBy:   userID,
	})
	require.NoError(t, err)

	newTitle := "Updated"
	updated, err := collections.UpdateCollection(ctx, c.ID, UpdateCollectionInput{
		Title: &newTitle,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Updated", updated.Title)
	assert.Equal(t, "Original desc", updated.Description, "description should remain unchanged")
}

func TestUnitCollectionStore_UpdateCollection_NoFields(t *testing.T) {
	collections, _, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	c, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope:     "org",
		ScopeID:   &orgID,
		Title:     "No Change",
		CreatedBy: userID,
	})
	require.NoError(t, err)

	updated, err := collections.UpdateCollection(ctx, c.ID, UpdateCollectionInput{})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "No Change", updated.Title)
}

func TestUnitCollectionStore_UpdateCollection_NonExistent(t *testing.T) {
	collections, _, _, _ := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	newTitle := "Ghost"
	result, err := collections.UpdateCollection(ctx, "00000000-0000-0000-0000-000000000000", UpdateCollectionInput{
		Title: &newTitle,
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

// ── DeleteCollection ────────────────────────────────────────────────────────

func TestUnitCollectionStore_DeleteCollection(t *testing.T) {
	collections, _, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	c, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope:     "org",
		ScopeID:   &orgID,
		Title:     "Delete Me",
		CreatedBy: userID,
	})
	require.NoError(t, err)

	deleted, err := collections.DeleteCollection(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, deleted)
	assert.Equal(t, c.ID, deleted.ID)

	gone, err := collections.GetCollection(ctx, c.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
}

func TestUnitCollectionStore_DeleteCollection_NotFound(t *testing.T) {
	collections, _, _, _ := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	deleted, err := collections.DeleteCollection(ctx, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

// ── ListCollections ─────────────────────────────────────────────────────────

func TestUnitCollectionStore_ListCollections(t *testing.T) {
	collections, _, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	_, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope: "org", ScopeID: &orgID, Title: "Col A", CreatedBy: userID,
	})
	require.NoError(t, err)

	_, err = collections.CreateCollection(ctx, CreateCollectionInput{
		Scope: "org", ScopeID: &orgID, Title: "Col B", CreatedBy: userID,
	})
	require.NoError(t, err)

	list, err := collections.ListCollections(ctx, "org", orgID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 2)
}

func TestUnitCollectionStore_ListCollections_Empty(t *testing.T) {
	collections, _, _, _ := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	list, err := collections.ListCollections(ctx, "org", "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Empty(t, list)
}

// ── Collection items ────────────────────────────────────────────────────────

func TestUnitCollectionStore_AddItem(t *testing.T) {
	collections, units, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	col, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope: "org", ScopeID: &orgID, Title: "Items Col", CreatedBy: userID,
	})
	require.NoError(t, err)

	unit, err := units.CreateUnit(ctx, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Unit for Item", CreatedBy: userID,
	})
	require.NoError(t, err)

	item, err := collections.AddItem(ctx, col.ID, unit.ID, 0)
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, col.ID, item.CollectionID)
	assert.Equal(t, unit.ID, item.UnitID)
	assert.Equal(t, 0, item.SortOrder)
}

func TestUnitCollectionStore_AddItem_Upsert(t *testing.T) {
	collections, units, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	col, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope: "org", ScopeID: &orgID, Title: "Upsert Col", CreatedBy: userID,
	})
	require.NoError(t, err)

	unit, err := units.CreateUnit(ctx, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Upsert Unit", CreatedBy: userID,
	})
	require.NoError(t, err)

	// First add.
	_, err = collections.AddItem(ctx, col.ID, unit.ID, 0)
	require.NoError(t, err)

	// Upsert with different sort_order.
	item, err := collections.AddItem(ctx, col.ID, unit.ID, 5)
	require.NoError(t, err)
	assert.Equal(t, 5, item.SortOrder, "upsert should update sort_order")
}

func TestUnitCollectionStore_RemoveItem(t *testing.T) {
	collections, units, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	col, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope: "org", ScopeID: &orgID, Title: "Remove Col", CreatedBy: userID,
	})
	require.NoError(t, err)

	unit, err := units.CreateUnit(ctx, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Remove Unit", CreatedBy: userID,
	})
	require.NoError(t, err)

	_, err = collections.AddItem(ctx, col.ID, unit.ID, 0)
	require.NoError(t, err)

	removed, err := collections.RemoveItem(ctx, col.ID, unit.ID)
	require.NoError(t, err)
	assert.True(t, removed)

	// Second remove should return false.
	removed2, err := collections.RemoveItem(ctx, col.ID, unit.ID)
	require.NoError(t, err)
	assert.False(t, removed2)
}

func TestUnitCollectionStore_ReorderItem(t *testing.T) {
	collections, units, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	col, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope: "org", ScopeID: &orgID, Title: "Reorder Col", CreatedBy: userID,
	})
	require.NoError(t, err)

	unit, err := units.CreateUnit(ctx, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Reorder Unit", CreatedBy: userID,
	})
	require.NoError(t, err)

	_, err = collections.AddItem(ctx, col.ID, unit.ID, 0)
	require.NoError(t, err)

	reordered, err := collections.ReorderItem(ctx, col.ID, unit.ID, 10)
	require.NoError(t, err)
	require.NotNil(t, reordered)
	assert.Equal(t, 10, reordered.SortOrder)
}

func TestUnitCollectionStore_ReorderItem_NotFound(t *testing.T) {
	collections, _, _, _ := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	result, err := collections.ReorderItem(ctx, "00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000001", 5)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestUnitCollectionStore_ListItems_Ordered(t *testing.T) {
	collections, units, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	col, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope: "org", ScopeID: &orgID, Title: "List Items Col", CreatedBy: userID,
	})
	require.NoError(t, err)

	u1, err := units.CreateUnit(ctx, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Item Unit A", CreatedBy: userID,
	})
	require.NoError(t, err)

	u2, err := units.CreateUnit(ctx, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Item Unit B", CreatedBy: userID,
	})
	require.NoError(t, err)

	// Add in reverse sort order.
	_, err = collections.AddItem(ctx, col.ID, u2.ID, 2)
	require.NoError(t, err)
	_, err = collections.AddItem(ctx, col.ID, u1.ID, 1)
	require.NoError(t, err)

	items, err := collections.ListItems(ctx, col.ID)
	require.NoError(t, err)
	require.Len(t, items, 2)

	// Ordered by sort_order ASC.
	assert.Equal(t, u1.ID, items[0].UnitID, "sort_order=1 should come first")
	assert.Equal(t, u2.ID, items[1].UnitID, "sort_order=2 should come second")
}

func TestUnitCollectionStore_ListItems_Empty(t *testing.T) {
	collections, _, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	col, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope: "org", ScopeID: &orgID, Title: "Empty Col", CreatedBy: userID,
	})
	require.NoError(t, err)

	items, err := collections.ListItems(ctx, col.ID)
	require.NoError(t, err)
	assert.NotNil(t, items)
	assert.Empty(t, items)
}

func TestUnitCollectionStore_DeleteCollection_CascadesItems(t *testing.T) {
	collections, units, orgID, userID := setupCollectionEnv(t, t.Name())
	ctx := context.Background()

	col, err := collections.CreateCollection(ctx, CreateCollectionInput{
		Scope: "org", ScopeID: &orgID, Title: "Cascade Col", CreatedBy: userID,
	})
	require.NoError(t, err)

	unit, err := units.CreateUnit(ctx, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Cascade Unit", CreatedBy: userID,
	})
	require.NoError(t, err)

	_, err = collections.AddItem(ctx, col.ID, unit.ID, 0)
	require.NoError(t, err)

	// Delete the collection — items should cascade.
	_, err = collections.DeleteCollection(ctx, col.ID)
	require.NoError(t, err)

	// Items should be gone.
	items, err := collections.ListItems(ctx, col.ID)
	require.NoError(t, err)
	assert.Empty(t, items)
}
