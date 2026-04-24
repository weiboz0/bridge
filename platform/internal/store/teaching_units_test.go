package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupUnitEnv creates a user + org and wires a TeachingUnitStore.
// It registers cleanup so all units created by the user are swept before the
// user row is dropped (teaching_units FK to users(id) via created_by has no ON
// DELETE, so un-swept rows would block the user delete and leak).
func setupUnitEnv(t *testing.T, suffix string) (*TeachingUnitStore, *OrgStore, string /* orgID */, string /* userID */) {
	t.Helper()
	db := testDB(t)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	units := NewTeachingUnitStore(db)

	org := createTestOrg(t, db, orgs, suffix)
	user := createTestUser(t, db, users, suffix)

	ctx := context.Background()
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM teaching_units WHERE created_by = $1`, user.ID)
	})

	return units, orgs, org.ID, user.ID
}

// mustCreateUnit is a focused helper used by multiple tests.
func mustCreateUnit(t *testing.T, units *TeachingUnitStore, in CreateTeachingUnitInput) *TeachingUnit {
	t.Helper()
	ctx := context.Background()
	u, err := units.CreateUnit(ctx, in)
	require.NoError(t, err)
	require.NotNil(t, u)
	return u
}

// ── Create / scope tests ──────────────────────────────────────────────────────

func TestTeachingUnitStore_Create_Platform(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "platform",
		ScopeID:   nil,
		Title:     "Platform Unit",
		Summary:   "A global unit",
		CreatedBy: userID,
	})

	assert.Equal(t, "platform", u.Scope)
	assert.Nil(t, u.ScopeID)
	assert.Equal(t, "Platform Unit", u.Title)
	assert.Equal(t, "draft", u.Status, "empty Status defaults to draft")
	assert.Equal(t, []string{}, u.SubjectTags)
	assert.Equal(t, []string{}, u.StandardsTags)
	assert.NotEmpty(t, u.ID)

	// Confirm it survives a round-trip.
	got, err := units.GetUnit(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, u.ID, got.ID)
}

func TestTeachingUnitStore_Create_Org(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "org",
		ScopeID:   &orgID,
		Title:     "Org Unit",
		CreatedBy: userID,
	})

	assert.Equal(t, "org", u.Scope)
	require.NotNil(t, u.ScopeID)
	assert.Equal(t, orgID, *u.ScopeID)
}

func TestTeachingUnitStore_Create_Personal(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Personal Unit",
		CreatedBy: userID,
	})

	assert.Equal(t, "personal", u.Scope)
	require.NotNil(t, u.ScopeID)
	assert.Equal(t, userID, *u.ScopeID)
}

func TestTeachingUnitStore_Create_CheckConstraint(t *testing.T) {
	// scope=platform + scope_id non-nil must be rejected by the DB CHECK constraint.
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	_, err := units.CreateUnit(ctx, CreateTeachingUnitInput{
		Scope:     "platform",
		ScopeID:   &orgID, // violates constraint
		Title:     "Bad Unit",
		CreatedBy: userID,
	})
	require.Error(t, err, "expected CHECK constraint violation")
	// pq error code 23514 = check_violation
	assert.Contains(t, err.Error(), "23514")
}

// ── Document seeding ──────────────────────────────────────────────────────────

func TestTeachingUnitStore_Create_Seeds_Document(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Seeded Doc Unit",
		CreatedBy: userID,
	})

	ctx := context.Background()
	doc, err := units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, doc, "unit_documents row must exist after CreateUnit")
	assert.Equal(t, u.ID, doc.UnitID)
	// The default document must be valid JSON with type="doc".
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(doc.Blocks, &parsed))
	assert.Equal(t, "doc", parsed["type"])
}

func TestTeachingUnitStore_GetDocument(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "GetDoc Unit",
		CreatedBy: userID,
	})

	ctx := context.Background()
	doc, err := units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, u.ID, doc.UnitID)
	assert.NotEmpty(t, doc.Blocks)
}

// ── SaveDocument ──────────────────────────────────────────────────────────────

func TestTeachingUnitStore_SaveDocument_Upsert(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Save Doc Unit",
		CreatedBy: userID,
	})

	ctx := context.Background()
	blocks1 := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hello"}]}]}`)

	// First save — inserts (upsert over the seeded default).
	doc1, err := units.SaveDocument(ctx, u.ID, blocks1)
	require.NoError(t, err)
	require.NotNil(t, doc1)
	assert.Equal(t, u.ID, doc1.UnitID)
	var parsed1 map[string]interface{}
	require.NoError(t, json.Unmarshal(doc1.Blocks, &parsed1))
	assert.Equal(t, "doc", parsed1["type"])

	// Give the clock a moment to advance so updated_at differences are detectable.
	// (In practice Postgres now() is sub-millisecond accurate.)
	firstUpdatedAt := doc1.UpdatedAt

	time.Sleep(5 * time.Millisecond)

	blocks2 := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"world"}]}]}`)

	// Second save — updates.
	doc2, err := units.SaveDocument(ctx, u.ID, blocks2)
	require.NoError(t, err)
	require.NotNil(t, doc2)

	assert.True(t, doc2.UpdatedAt.After(firstUpdatedAt) || doc2.UpdatedAt.Equal(firstUpdatedAt),
		"second save updated_at must be >= first save")

	// Fetch and confirm the stored content matches the second save.
	fetched, err := units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	var parsedFetched map[string]interface{}
	require.NoError(t, json.Unmarshal(fetched.Blocks, &parsedFetched))
	content := parsedFetched["content"].([]interface{})
	para := content[0].(map[string]interface{})
	inner := para["content"].([]interface{})
	text := inner[0].(map[string]interface{})
	assert.Equal(t, "world", text["text"])
}

func TestTeachingUnitStore_SaveDocument_BumpsUnitUpdatedAt(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Bump Unit",
		CreatedBy: userID,
	})

	originalUpdatedAt := u.UpdatedAt

	time.Sleep(5 * time.Millisecond)

	ctx := context.Background()
	blocks := json.RawMessage(`{"type":"doc","content":[]}`)
	_, err := units.SaveDocument(ctx, u.ID, blocks)
	require.NoError(t, err)

	refreshed, err := units.GetUnit(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed)
	assert.True(t, refreshed.UpdatedAt.After(originalUpdatedAt),
		"teaching_units.updated_at must be bumped by SaveDocument")
}

// ── UpdateUnit ────────────────────────────────────────────────────────────────

func TestTeachingUnitStore_UpdateUnit_Partial(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Original Title",
		Summary:   "Original summary",
		CreatedBy: userID,
	})

	ctx := context.Background()
	newTitle := "Updated Title"
	updated, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{
		Title: &newTitle,
		// Summary intentionally omitted — must remain unchanged.
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Updated Title", updated.Title)
	assert.Equal(t, "Original summary", updated.Summary, "untouched field must not change")
}

func TestTeachingUnitStore_UpdateUnit_SubjectTags(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:       "personal",
		ScopeID:     &userID,
		Title:       "Tags Unit",
		SubjectTags: []string{"math", "cs"},
		CreatedBy:   userID,
	})
	assert.Equal(t, []string{"math", "cs"}, u.SubjectTags)

	ctx := context.Background()

	// nil SubjectTags → leave unchanged.
	unchanged, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{
		SubjectTags: nil,
	})
	require.NoError(t, err)
	require.NotNil(t, unchanged)
	assert.Equal(t, []string{"math", "cs"}, unchanged.SubjectTags, "nil SubjectTags must leave tags unchanged")

	// Empty slice → clear to '{}'.
	cleared, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{
		SubjectTags: []string{},
	})
	require.NoError(t, err)
	require.NotNil(t, cleared)
	assert.Equal(t, []string{}, cleared.SubjectTags, "empty SubjectTags must clear the array")
}

// ── DeleteUnit ────────────────────────────────────────────────────────────────

func TestTeachingUnitStore_DeleteUnit_Cascades(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Cascade Unit",
		CreatedBy: userID,
	})

	ctx := context.Background()

	// Confirm document exists.
	doc, err := units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, doc, "document must exist before delete")

	// Delete the unit.
	deleted, err := units.DeleteUnit(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, deleted)
	assert.Equal(t, u.ID, deleted.ID)

	// Unit must be gone.
	gone, err := units.GetUnit(ctx, u.ID)
	require.NoError(t, err)
	assert.Nil(t, gone, "GetUnit must return nil after delete")

	// Document must be cascaded away.
	docGone, err := units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	assert.Nil(t, docGone, "unit_documents row must be cascaded on unit delete")
}
