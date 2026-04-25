package store

import (
	"context"
	"database/sql"
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
		db.ExecContext(ctx, `DELETE FROM unit_overlays WHERE child_unit_id IN (SELECT id FROM teaching_units WHERE created_by = $1)`, user.ID)
		db.ExecContext(ctx, `DELETE FROM unit_overlays WHERE parent_unit_id IN (SELECT id FROM teaching_units WHERE created_by = $1)`, user.ID)
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

// ── UpdateUnit extended coverage ─────────────────────────────────────────────

func TestTeachingUnitStore_UpdateUnit_SlugSetAndClear(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Slug Unit",
		CreatedBy: userID,
	})
	assert.Nil(t, u.Slug, "new unit should have nil slug")

	// Set slug
	slug := "my-slug"
	updated, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{Slug: &slug})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.Slug)
	assert.Equal(t, "my-slug", *updated.Slug)

	// Clear slug (empty string → NULL)
	emptySlug := ""
	cleared, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{Slug: &emptySlug})
	require.NoError(t, err)
	require.NotNil(t, cleared)
	assert.Nil(t, cleared.Slug, "slug should be cleared to nil when set to empty string")
}

func TestTeachingUnitStore_UpdateUnit_GradeLevelSetAndClear(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Grade Unit",
		CreatedBy: userID,
	})
	assert.Nil(t, u.GradeLevel, "new unit should have nil grade level")

	// Set grade level
	grade := "K-5"
	updated, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{GradeLevel: &grade})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.GradeLevel)
	assert.Equal(t, "K-5", *updated.GradeLevel)

	// Clear grade level (empty string → NULL)
	emptyGrade := ""
	cleared, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{GradeLevel: &emptyGrade})
	require.NoError(t, err)
	require.NotNil(t, cleared)
	assert.Nil(t, cleared.GradeLevel, "grade level should be cleared to nil when set to empty string")
}

func TestTeachingUnitStore_UpdateUnit_StandardsTags(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:         "personal",
		ScopeID:       &userID,
		Title:         "Standards Unit",
		StandardsTags: []string{"CCSS.1", "CCSS.2"},
		CreatedBy:     userID,
	})
	assert.Equal(t, []string{"CCSS.1", "CCSS.2"}, u.StandardsTags)

	// nil StandardsTags → leave unchanged
	unchanged, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{
		StandardsTags: nil,
	})
	require.NoError(t, err)
	require.NotNil(t, unchanged)
	assert.Equal(t, []string{"CCSS.1", "CCSS.2"}, unchanged.StandardsTags, "nil StandardsTags must leave tags unchanged")

	// Empty slice → clear
	cleared, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{
		StandardsTags: []string{},
	})
	require.NoError(t, err)
	require.NotNil(t, cleared)
	assert.Equal(t, []string{}, cleared.StandardsTags, "empty StandardsTags must clear the array")

	// Set new tags
	set, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{
		StandardsTags: []string{"NGSS.1"},
	})
	require.NoError(t, err)
	require.NotNil(t, set)
	assert.Equal(t, []string{"NGSS.1"}, set.StandardsTags)
}

func TestTeachingUnitStore_UpdateUnit_EstimatedMinutes(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Minutes Unit",
		CreatedBy: userID,
	})
	assert.Nil(t, u.EstimatedMinutes, "new unit should have nil estimated minutes")

	// Set estimated minutes
	mins := 45
	updated, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{EstimatedMinutes: &mins})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.EstimatedMinutes)
	assert.Equal(t, 45, *updated.EstimatedMinutes)

	// Clear estimated minutes (0 maps to SQL zero, which IS stored — but we can verify it round-trips)
	zero := 0
	zeroed, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{EstimatedMinutes: &zero})
	require.NoError(t, err)
	require.NotNil(t, zeroed)
	require.NotNil(t, zeroed.EstimatedMinutes)
	assert.Equal(t, 0, *zeroed.EstimatedMinutes)
}

func TestTeachingUnitStore_UpdateUnit_Status(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Status Unit",
		CreatedBy: userID,
	})
	assert.Equal(t, "draft", u.Status)

	status := "reviewed"
	updated, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{Status: &status})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "reviewed", updated.Status)
}

func TestTeachingUnitStore_UpdateUnit_MultipleFields(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Multi Unit",
		Summary:   "Old summary",
		CreatedBy: userID,
	})

	newTitle := "New Title"
	newSummary := "New summary"
	newSlug := "new-slug"
	grade := "6-8"
	mins := 60
	status := "classroom_ready"
	updated, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{
		Title:            &newTitle,
		Summary:          &newSummary,
		Slug:             &newSlug,
		GradeLevel:       &grade,
		SubjectTags:      []string{"science", "bio"},
		StandardsTags:    []string{"NGSS.1"},
		EstimatedMinutes: &mins,
		Status:           &status,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "New Title", updated.Title)
	assert.Equal(t, "New summary", updated.Summary)
	require.NotNil(t, updated.Slug)
	assert.Equal(t, "new-slug", *updated.Slug)
	require.NotNil(t, updated.GradeLevel)
	assert.Equal(t, "6-8", *updated.GradeLevel)
	assert.Equal(t, []string{"science", "bio"}, updated.SubjectTags)
	assert.Equal(t, []string{"NGSS.1"}, updated.StandardsTags)
	require.NotNil(t, updated.EstimatedMinutes)
	assert.Equal(t, 60, *updated.EstimatedMinutes)
	assert.Equal(t, "classroom_ready", updated.Status)
}

func TestTeachingUnitStore_UpdateUnit_NonExistent(t *testing.T) {
	units, _, _, _ := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	newTitle := "Ghost"
	result, err := units.UpdateUnit(ctx, "00000000-0000-0000-0000-000000000000", UpdateTeachingUnitInput{
		Title: &newTitle,
	})
	assert.NoError(t, err)
	assert.Nil(t, result, "updating non-existent unit should return nil")
}

func TestTeachingUnitStore_UpdateUnit_NoFields(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Unchanged Unit",
		CreatedBy: userID,
	})

	// No fields provided — should return existing unchanged
	result, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Unchanged Unit", result.Title)
	assert.Equal(t, u.ID, result.ID)
}

// ── ListUnitsForScope ────────────────────────────────────────────────────────

func TestTeachingUnitStore_ListUnitsForScope_OrgUnits(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	// Create a second org environment for isolation testing
	db := testDB(t)
	orgs := NewOrgStore(db)
	otherOrg := createTestOrg(t, db, orgs, t.Name()+"-other")

	// Create units in our org
	u1 := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Org Unit 1", CreatedBy: userID,
	})
	u2 := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Org Unit 2", CreatedBy: userID,
	})

	// Create unit in other org (should be excluded)
	otherUnits := NewTeachingUnitStore(db)
	otherUnit := mustCreateUnit(t, otherUnits, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &otherOrg.ID, Title: "Other Org Unit", CreatedBy: userID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM teaching_units WHERE id = $1`, otherUnit.ID)
	})

	list, err := units.ListUnitsForScope(ctx, "org", orgID)
	require.NoError(t, err)
	require.Len(t, list, 2)

	// Should be ordered by updated_at DESC
	ids := []string{list[0].ID, list[1].ID}
	assert.Contains(t, ids, u1.ID)
	assert.Contains(t, ids, u2.ID)

	// Ensure other org's unit is not included
	for _, u := range list {
		assert.NotEqual(t, otherUnit.ID, u.ID, "other org's unit must not appear")
	}
}

func TestTeachingUnitStore_ListUnitsForScope_PlatformUnits(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", ScopeID: nil, Title: "Platform List Unit", CreatedBy: userID,
	})

	list, err := units.ListUnitsForScope(ctx, "platform", "")
	require.NoError(t, err)
	require.NotEmpty(t, list)

	found := false
	for _, item := range list {
		if item.ID == u.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "platform unit should appear in platform scope listing")
}

func TestTeachingUnitStore_ListUnitsForScope_PersonalUnits(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	// Create a personal unit for this user
	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Personal Unit", CreatedBy: userID,
	})

	list, err := units.ListUnitsForScope(ctx, "personal", userID)
	require.NoError(t, err)
	require.NotEmpty(t, list)

	found := false
	for _, item := range list {
		if item.ID == u.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "personal unit should appear in user scope listing")
}

func TestTeachingUnitStore_ListUnitsForScope_EmptyScope(t *testing.T) {
	units, _, _, _ := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	// Use a non-existent scope_id for an org scope — should be empty
	list, err := units.ListUnitsForScope(ctx, "org", "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	require.NotNil(t, list)
	assert.Len(t, list, 0)
}

func TestTeachingUnitStore_ListUnitsForScope_OrderByUpdatedAtDesc(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()
	db := testDB(t)

	u1 := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "First", CreatedBy: userID,
	})
	u2 := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Second", CreatedBy: userID,
	})

	// Force u1 to have a newer updated_at than u2
	future := time.Now().Add(1 * time.Hour)
	_, err := db.ExecContext(ctx, "UPDATE teaching_units SET updated_at = $1 WHERE id = $2", future, u1.ID)
	require.NoError(t, err)

	list, err := units.ListUnitsForScope(ctx, "personal", userID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, u1.ID, list[0].ID, "unit with newer updated_at should come first")
	assert.Equal(t, u2.ID, list[1].ID)
}

// ── scanTeachingUnit nullable fields coverage ────────────────────────────────

func TestTeachingUnitStore_NullableFields_RoundTrip(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	// Create with all nullable fields as nil
	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Nullable Unit",
		CreatedBy: userID,
	})

	assert.Nil(t, u.Slug)
	assert.Nil(t, u.GradeLevel)
	assert.Nil(t, u.EstimatedMinutes)
	assert.Equal(t, []string{}, u.SubjectTags)
	assert.Equal(t, []string{}, u.StandardsTags)

	// Fetch and verify round-trip of nil fields
	fetched, err := units.GetUnit(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Nil(t, fetched.Slug)
	assert.Nil(t, fetched.GradeLevel)
	assert.Nil(t, fetched.EstimatedMinutes)

	// Now set all nullable fields to non-nil and verify
	slug := "test-slug"
	grade := "9-12"
	mins := 90
	updated, err := units.UpdateUnit(ctx, u.ID, UpdateTeachingUnitInput{
		Slug:             &slug,
		GradeLevel:       &grade,
		EstimatedMinutes: &mins,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.Slug)
	assert.Equal(t, "test-slug", *updated.Slug)
	require.NotNil(t, updated.GradeLevel)
	assert.Equal(t, "9-12", *updated.GradeLevel)
	require.NotNil(t, updated.EstimatedMinutes)
	assert.Equal(t, 90, *updated.EstimatedMinutes)
}

// ── GetUnitByTopicID ─────────────────────────────────────────────────────────

func TestTeachingUnitStore_GetUnitByTopicID(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()
	db := testDB(t)

	// Use valid UUIDs for the course and topic fixtures.
	courseID := "00000000-0000-0000-aaaa-000000000001"
	topicID := "00000000-0000-0000-aaaa-000000000002"

	_, err := db.ExecContext(ctx, `
		INSERT INTO courses (id, org_id, created_by, title, description, grade_level, language, is_published)
		VALUES ($1, $2, $3, 'GetByTopicID Course', '', '9-12', 'python', false)
		ON CONFLICT (id) DO NOTHING`,
		courseID, orgID, userID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO topics (id, course_id, title, description, sort_order, lesson_content)
		VALUES ($1, $2, 'Test Topic', 'desc', 0, '{}'::jsonb)
		ON CONFLICT (id) DO NOTHING`,
		topicID, courseID)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM topics WHERE id = $1`, topicID)
		db.ExecContext(ctx, `DELETE FROM courses WHERE id = $1`, courseID)
	})

	// Create a unit that references this topic.
	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope:     "personal",
		ScopeID:   &userID,
		Title:     "Topic-linked Unit",
		CreatedBy: userID,
	})

	// Set topic_id directly (CreateUnit doesn't expose it yet).
	_, err = db.ExecContext(ctx,
		`UPDATE teaching_units SET topic_id = $1 WHERE id = $2`, topicID, u.ID)
	require.NoError(t, err)

	// Happy path: should find the unit by topic_id.
	found, err := units.GetUnitByTopicID(ctx, topicID)
	require.NoError(t, err)
	require.NotNil(t, found, "GetUnitByTopicID must return unit when topic_id matches")
	assert.Equal(t, u.ID, found.ID)
	require.NotNil(t, found.TopicID, "TopicID field must be populated")
	assert.Equal(t, topicID, *found.TopicID)

	// Non-existent topic_id → nil, nil.
	missing, err := units.GetUnitByTopicID(ctx, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Nil(t, missing, "GetUnitByTopicID must return nil for unknown topic_id")
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

// ── SetUnitStatus ────────────────────────────────────────────────────────────

func TestTeachingUnitStore_SetUnitStatus_DraftToReviewed(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Draft→Reviewed", CreatedBy: userID,
	})
	assert.Equal(t, "draft", u.Status)

	updated, err := units.SetUnitStatus(ctx, u.ID, "reviewed", userID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "reviewed", updated.Status)

	// No revision should be created for reviewed.
	revs, err := units.ListRevisions(ctx, u.ID)
	require.NoError(t, err)
	assert.Empty(t, revs, "no revision on draft→reviewed")
}

func TestTeachingUnitStore_SetUnitStatus_ReviewedToClassroomReady_CreatesRevision(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Reviewed→CR", CreatedBy: userID,
	})

	// Save some blocks so the snapshot is non-trivial.
	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hello"}]}]}`)
	_, err := units.SaveDocument(ctx, u.ID, blocks)
	require.NoError(t, err)

	// draft→reviewed
	_, err = units.SetUnitStatus(ctx, u.ID, "reviewed", userID)
	require.NoError(t, err)

	// reviewed→classroom_ready
	updated, err := units.SetUnitStatus(ctx, u.ID, "classroom_ready", userID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "classroom_ready", updated.Status)

	// Verify revision was created.
	revs, err := units.ListRevisions(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, revs, 1)
	assert.Equal(t, u.ID, revs[0].UnitID)
	require.NotNil(t, revs[0].Reason)
	assert.Equal(t, "classroom_ready", *revs[0].Reason)
	assert.Equal(t, userID, revs[0].CreatedBy)

	// Blocks in the revision should match what we saved.
	var revBlocks map[string]interface{}
	require.NoError(t, json.Unmarshal(revs[0].Blocks, &revBlocks))
	assert.Equal(t, "doc", revBlocks["type"])
}

func TestTeachingUnitStore_SetUnitStatus_ReviewedToCoachReady_CreatesRevision(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Reviewed→Coach", CreatedBy: userID,
	})

	_, err := units.SetUnitStatus(ctx, u.ID, "reviewed", userID)
	require.NoError(t, err)

	updated, err := units.SetUnitStatus(ctx, u.ID, "coach_ready", userID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "coach_ready", updated.Status)

	revs, err := units.ListRevisions(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, revs, 1)
	require.NotNil(t, revs[0].Reason)
	assert.Equal(t, "coach_ready", *revs[0].Reason)
}

func TestTeachingUnitStore_SetUnitStatus_InvalidTransitions(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	tests := []struct {
		name          string
		initialStatus string
		targetStatus  string
	}{
		{"draft→classroom_ready (skips reviewed)", "draft", "classroom_ready"},
		{"draft→coach_ready (skips reviewed)", "draft", "coach_ready"},
		{"draft→archived", "draft", "archived"},
		{"classroom_ready→reviewed (backwards)", "classroom_ready", "reviewed"},
		{"classroom_ready→coach_ready (lateral)", "classroom_ready", "coach_ready"},
		{"coach_ready→classroom_ready (lateral)", "coach_ready", "classroom_ready"},
		{"reviewed→draft (backwards)", "reviewed", "draft"},
		{"classroom_ready→draft (backwards)", "classroom_ready", "draft"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := mustCreateUnit(t, units, CreateTeachingUnitInput{
				Scope: "personal", ScopeID: &userID, Title: "Invalid-" + tt.name,
				Status: tt.initialStatus, CreatedBy: userID,
			})

			_, err := units.SetUnitStatus(ctx, u.ID, tt.targetStatus, userID)
			assert.ErrorIs(t, err, ErrInvalidTransition, "transition %s should be invalid", tt.name)
		})
	}
}

func TestTeachingUnitStore_SetUnitStatus_ArchiveFromReviewed(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Archive-Reviewed", CreatedBy: userID,
	})
	_, err := units.SetUnitStatus(ctx, u.ID, "reviewed", userID)
	require.NoError(t, err)

	archived, err := units.SetUnitStatus(ctx, u.ID, "archived", userID)
	require.NoError(t, err)
	require.NotNil(t, archived)
	assert.Equal(t, "archived", archived.Status)
}

func TestTeachingUnitStore_SetUnitStatus_ArchiveFromClassroomReady(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Archive-CR",
		Status: "classroom_ready", CreatedBy: userID,
	})

	archived, err := units.SetUnitStatus(ctx, u.ID, "archived", userID)
	require.NoError(t, err)
	require.NotNil(t, archived)
	assert.Equal(t, "archived", archived.Status)
}

func TestTeachingUnitStore_SetUnitStatus_UnarchiveToClassroomReady_CreatesRevision(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Unarchive",
		Status: "archived", CreatedBy: userID,
	})

	// Save blocks before unarchive so the snapshot captures them.
	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph"}]}`)
	_, err := units.SaveDocument(ctx, u.ID, blocks)
	require.NoError(t, err)

	updated, err := units.SetUnitStatus(ctx, u.ID, "classroom_ready", userID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "classroom_ready", updated.Status)

	// Revision should exist.
	revs, err := units.ListRevisions(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, revs, 1)
	require.NotNil(t, revs[0].Reason)
	assert.Equal(t, "classroom_ready", *revs[0].Reason)
}

func TestTeachingUnitStore_SetUnitStatus_NonExistentUnit(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	_, err := units.SetUnitStatus(ctx, "00000000-0000-0000-0000-000000000000", "reviewed", userID)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

// ── ListRevisions / GetRevision ─────────────────────────────────────────────

func TestTeachingUnitStore_ListRevisions_Ordered(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Rev-Order", CreatedBy: userID,
	})

	// draft→reviewed→classroom_ready (revision 1)
	_, err := units.SetUnitStatus(ctx, u.ID, "reviewed", userID)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, u.ID, "classroom_ready", userID)
	require.NoError(t, err)

	// classroom_ready→archived→classroom_ready (revision 2)
	_, err = units.SetUnitStatus(ctx, u.ID, "archived", userID)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, u.ID, "classroom_ready", userID)
	require.NoError(t, err)

	revs, err := units.ListRevisions(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, revs, 2)

	// Ordered by created_at DESC — newest first.
	assert.True(t, !revs[0].CreatedAt.Before(revs[1].CreatedAt),
		"revisions should be ordered DESC by created_at")
}

func TestTeachingUnitStore_ListRevisions_EmptyForUnpublished(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Never Published", CreatedBy: userID,
	})

	revs, err := units.ListRevisions(ctx, u.ID)
	require.NoError(t, err)
	assert.Empty(t, revs)
}

func TestTeachingUnitStore_GetRevision(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Get-Rev", CreatedBy: userID,
	})

	_, err := units.SetUnitStatus(ctx, u.ID, "reviewed", userID)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, u.ID, "classroom_ready", userID)
	require.NoError(t, err)

	revs, err := units.ListRevisions(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, revs, 1)

	got, err := units.GetRevision(ctx, revs[0].ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, revs[0].ID, got.ID)
	assert.Equal(t, u.ID, got.UnitID)
}

func TestTeachingUnitStore_GetRevision_NotFound(t *testing.T) {
	units, _, _, _ := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	got, err := units.GetRevision(ctx, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// ── ForkUnit ────────────────────────────────────────────────────────────────

func TestTeachingUnitStore_ForkUnit_CreatesChildOverlayDoc(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	source := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "Source Unit", Summary: "Source summary", CreatedBy: userID,
	})

	child, err := units.ForkUnit(ctx, source.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, child)
	assert.Equal(t, "draft", child.Status)
	assert.Equal(t, "org", child.Scope)
	require.NotNil(t, child.ScopeID)
	assert.Equal(t, orgID, *child.ScopeID)
	assert.Equal(t, "Source Unit (fork)", child.Title)
	assert.Equal(t, "Source summary", child.Summary)

	// Overlay must exist.
	ov, err := units.GetOverlay(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, ov)
	assert.Equal(t, child.ID, ov.ChildUnitID)
	assert.Equal(t, source.ID, ov.ParentUnitID)
	assert.Nil(t, ov.ParentRevisionID, "fork should be floating (nil revision)")
	assert.Equal(t, json.RawMessage(`{}`), ov.BlockOverrides)

	// Document must exist.
	doc, err := units.GetDocument(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, doc)
}

func TestTeachingUnitStore_ForkUnit_CustomTitle(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	source := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "Source", CreatedBy: userID,
	})

	title := "My Adaptation"
	child, err := units.ForkUnit(ctx, source.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, Title: &title, CallerID: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, child)
	assert.Equal(t, "My Adaptation", child.Title)
}

func TestTeachingUnitStore_ForkUnit_SourceNotFound(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	child, err := units.ForkUnit(ctx, "00000000-0000-0000-0000-000000000000", ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)
	assert.Nil(t, child)
}

// ── GetOverlay / UpdateOverlay ──────────────────────────────────────────────

func TestTeachingUnitStore_GetOverlay_NoOverlay(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "No Overlay", CreatedBy: userID,
	})

	ov, err := units.GetOverlay(ctx, u.ID)
	require.NoError(t, err)
	assert.Nil(t, ov, "non-forked unit should have no overlay")
}

func TestTeachingUnitStore_UpdateOverlay_PinRevision(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	source := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "Pin Source", CreatedBy: userID,
	})

	// Save blocks and publish to create a revision.
	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}}]}`)
	_, err := units.SaveDocument(ctx, source.ID, blocks)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "reviewed", userID)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "classroom_ready", userID)
	require.NoError(t, err)

	revs, err := units.ListRevisions(ctx, source.ID)
	require.NoError(t, err)
	require.Len(t, revs, 1)
	revID := revs[0].ID

	child, err := units.ForkUnit(ctx, source.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, child)

	// Pin to the revision.
	updated, err := units.UpdateOverlay(ctx, child.ID, UpdateOverlayInput{
		ParentRevisionID: &revID,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.ParentRevisionID)
	assert.Equal(t, revID, *updated.ParentRevisionID)
}

func TestTeachingUnitStore_UpdateOverlay_Float(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	source := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "Float Source", CreatedBy: userID,
	})

	// Save blocks, publish, create a revision.
	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}}]}`)
	_, err := units.SaveDocument(ctx, source.ID, blocks)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "reviewed", userID)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "classroom_ready", userID)
	require.NoError(t, err)

	revs, err := units.ListRevisions(ctx, source.ID)
	require.NoError(t, err)
	require.Len(t, revs, 1)
	revID := revs[0].ID

	child, err := units.ForkUnit(ctx, source.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, child)

	// Pin first.
	_, err = units.UpdateOverlay(ctx, child.ID, UpdateOverlayInput{
		ParentRevisionID: &revID,
	})
	require.NoError(t, err)

	// Float back (empty string = NULL).
	emptyRev := ""
	updated, err := units.UpdateOverlay(ctx, child.ID, UpdateOverlayInput{
		ParentRevisionID: &emptyRev,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Nil(t, updated.ParentRevisionID, "empty string should set to NULL (floating)")
}

func TestTeachingUnitStore_UpdateOverlay_BlockOverrides(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	source := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "Override Source", CreatedBy: userID,
	})

	child, err := units.ForkUnit(ctx, source.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, child)

	overrides := json.RawMessage(`{"b1":{"action":"hide"}}`)
	updated, err := units.UpdateOverlay(ctx, child.ID, UpdateOverlayInput{
		BlockOverrides: overrides,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)

	// Parse back the overrides.
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(updated.BlockOverrides, &parsed))
	assert.Contains(t, parsed, "b1")
}

func TestTeachingUnitStore_UpdateOverlay_NoFields(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	source := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "NoField Source", CreatedBy: userID,
	})

	child, err := units.ForkUnit(ctx, source.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, child)

	// No fields → returns existing overlay unchanged.
	updated, err := units.UpdateOverlay(ctx, child.ID, UpdateOverlayInput{})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, child.ID, updated.ChildUnitID)
}

// ── GetComposedDocument ─────────────────────────────────────────────────────

func TestTeachingUnitStore_GetComposedDocument_NoOverlay(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Plain Unit", CreatedBy: userID,
	})

	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"p1"}}]}`)
	_, err := units.SaveDocument(ctx, u.ID, blocks)
	require.NoError(t, err)

	composed, err := units.GetComposedDocument(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, composed)

	// Should be the unit's own document.
	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(composed, &doc))
	assert.Equal(t, "doc", doc["type"])
}

func TestTeachingUnitStore_GetComposedDocument_ForkedFloating(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	source := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "Compose Source", CreatedBy: userID,
	})

	// Save blocks to source and publish.
	sourceBlocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"},"content":[{"type":"text","text":"hello"}]}]}`)
	_, err := units.SaveDocument(ctx, source.ID, sourceBlocks)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "reviewed", userID)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "classroom_ready", userID)
	require.NoError(t, err)

	// Fork.
	child, err := units.ForkUnit(ctx, source.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, child)

	// Composed doc should equal parent's published revision blocks.
	composed, err := units.GetComposedDocument(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, composed)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(composed, &doc))
	assert.Equal(t, "doc", doc["type"])
	content, ok := doc["content"].([]interface{})
	require.True(t, ok)
	require.Len(t, content, 1, "composed should have 1 parent block")
}

func TestTeachingUnitStore_GetComposedDocument_HideOverride(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	source := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "Hide Source", CreatedBy: userID,
	})

	sourceBlocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}},{"type":"prose","attrs":{"id":"b2"}}]}`)
	_, err := units.SaveDocument(ctx, source.ID, sourceBlocks)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "reviewed", userID)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "classroom_ready", userID)
	require.NoError(t, err)

	child, err := units.ForkUnit(ctx, source.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)

	// Add hide override on b1.
	overrides := json.RawMessage(`{"b1":{"action":"hide"}}`)
	_, err = units.UpdateOverlay(ctx, child.ID, UpdateOverlayInput{
		BlockOverrides: overrides,
	})
	require.NoError(t, err)

	composed, err := units.GetComposedDocument(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, composed)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(composed, &doc))
	content := doc["content"].([]interface{})
	// Only b2 should remain.
	assert.Len(t, content, 1, "hide override should omit b1")
	b := content[0].(map[string]interface{})
	attrs := b["attrs"].(map[string]interface{})
	assert.Equal(t, "b2", attrs["id"])
}

func TestTeachingUnitStore_GetComposedDocument_ReplaceOverride(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	source := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "Replace Source", CreatedBy: userID,
	})

	sourceBlocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}},{"type":"prose","attrs":{"id":"b2"}}]}`)
	_, err := units.SaveDocument(ctx, source.ID, sourceBlocks)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "reviewed", userID)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "classroom_ready", userID)
	require.NoError(t, err)

	child, err := units.ForkUnit(ctx, source.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)

	// Replace b1 with a different block.
	overrides := json.RawMessage(`{"b1":{"action":"replace","block":{"type":"prose","attrs":{"id":"b1-replaced"},"content":[{"type":"text","text":"replaced"}]}}}`)
	_, err = units.UpdateOverlay(ctx, child.ID, UpdateOverlayInput{
		BlockOverrides: overrides,
	})
	require.NoError(t, err)

	composed, err := units.GetComposedDocument(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, composed)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(composed, &doc))
	content := doc["content"].([]interface{})
	assert.Len(t, content, 2)
	// First block should be the replacement.
	b := content[0].(map[string]interface{})
	attrs := b["attrs"].(map[string]interface{})
	assert.Equal(t, "b1-replaced", attrs["id"])
}

func TestTeachingUnitStore_GetComposedDocument_PinnedRevision(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	source := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "Pinned Source", CreatedBy: userID,
	})

	// Version 1: save blocks and publish.
	blocksV1 := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"},"content":[{"type":"text","text":"v1"}]}]}`)
	_, err := units.SaveDocument(ctx, source.ID, blocksV1)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "reviewed", userID)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "classroom_ready", userID)
	require.NoError(t, err)

	revsV1, err := units.ListRevisions(ctx, source.ID)
	require.NoError(t, err)
	require.Len(t, revsV1, 1)
	revV1ID := revsV1[0].ID

	// Version 2: update blocks and re-publish (archive → classroom_ready creates a new revision).
	_, err = units.SetUnitStatus(ctx, source.ID, "archived", userID)
	require.NoError(t, err)
	blocksV2 := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"},"content":[{"type":"text","text":"v2"}]}]}`)
	_, err = units.SaveDocument(ctx, source.ID, blocksV2)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "classroom_ready", userID)
	require.NoError(t, err)

	// Fork and pin to v1.
	child, err := units.ForkUnit(ctx, source.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)

	_, err = units.UpdateOverlay(ctx, child.ID, UpdateOverlayInput{
		ParentRevisionID: &revV1ID,
	})
	require.NoError(t, err)

	composed, err := units.GetComposedDocument(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, composed)

	// Composed should show v1 content, not v2.
	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(composed, &doc))
	content := doc["content"].([]interface{})
	require.Len(t, content, 1)
	block := content[0].(map[string]interface{})
	inner := block["content"].([]interface{})
	text := inner[0].(map[string]interface{})
	assert.Equal(t, "v1", text["text"], "pinned revision should use v1 blocks")
}

func TestTeachingUnitStore_GetComposedDocument_FloatingUsesLatest(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	source := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "Float Latest Source", CreatedBy: userID,
	})

	// Version 1: save blocks and publish.
	blocksV1 := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"},"content":[{"type":"text","text":"v1"}]}]}`)
	_, err := units.SaveDocument(ctx, source.ID, blocksV1)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "reviewed", userID)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "classroom_ready", userID)
	require.NoError(t, err)

	// Fork (floating by default).
	child, err := units.ForkUnit(ctx, source.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)

	// Version 2: update parent and re-publish.
	_, err = units.SetUnitStatus(ctx, source.ID, "archived", userID)
	require.NoError(t, err)
	blocksV2 := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"},"content":[{"type":"text","text":"v2"}]}]}`)
	_, err = units.SaveDocument(ctx, source.ID, blocksV2)
	require.NoError(t, err)
	_, err = units.SetUnitStatus(ctx, source.ID, "classroom_ready", userID)
	require.NoError(t, err)

	// Composed should now use v2 (latest published).
	composed, err := units.GetComposedDocument(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, composed)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(composed, &doc))
	content := doc["content"].([]interface{})
	require.Len(t, content, 1)
	block := content[0].(map[string]interface{})
	inner := block["content"].([]interface{})
	text := inner[0].(map[string]interface{})
	assert.Equal(t, "v2", text["text"], "floating should use latest published revision")
}

// ── GetLineage ──────────────────────────────────────────────────────────────

func TestTeachingUnitStore_GetLineage_NoOverlay(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	u := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Lone Unit", CreatedBy: userID,
	})

	lineage, err := units.GetLineage(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, lineage, 1, "non-forked unit should have lineage of just itself")
	assert.Equal(t, u.ID, lineage[0].UnitID)
	assert.Equal(t, "Lone Unit", lineage[0].Title)
}

func TestTeachingUnitStore_GetLineage_ChildParent(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	parent := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "Parent", CreatedBy: userID,
	})

	child, err := units.ForkUnit(ctx, parent.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, child)

	lineage, err := units.GetLineage(ctx, child.ID)
	require.NoError(t, err)
	require.Len(t, lineage, 2, "child → parent = 2 entries")
	// Root-first.
	assert.Equal(t, parent.ID, lineage[0].UnitID)
	assert.Equal(t, child.ID, lineage[1].UnitID)
}

func TestTeachingUnitStore_GetLineage_ThreeGenerations(t *testing.T) {
	units, _, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	grandparent := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "platform", Title: "Grandparent", CreatedBy: userID,
	})

	parentUnit, err := units.ForkUnit(ctx, grandparent.ID, ForkTarget{
		Scope: "org", ScopeID: &orgID, CallerID: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, parentUnit)

	child, err := units.ForkUnit(ctx, parentUnit.ID, ForkTarget{
		Scope: "personal", ScopeID: &userID, CallerID: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, child)

	lineage, err := units.GetLineage(ctx, child.ID)
	require.NoError(t, err)
	require.Len(t, lineage, 3, "child → parent → grandparent = 3 entries")
	// Root-first.
	assert.Equal(t, grandparent.ID, lineage[0].UnitID)
	assert.Equal(t, parentUnit.ID, lineage[1].UnitID)
	assert.Equal(t, child.ID, lineage[2].UnitID)
}

// ── SearchUnits ────────────────────────────────────────────────────────────

func TestTeachingUnitStore_SearchUnits_FTS(t *testing.T) {
	units, orgs, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()
	db := testDB(t)

	// Add viewer as teacher in org so org-scoped units are visible.
	_, err := orgs.AddOrgMember(ctx, AddMemberInput{
		OrgID: orgID, UserID: userID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1 AND org_id = $2", userID, orgID)
	})

	// Create units with distinct titles and summaries for FTS matching.
	mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Introduction to Python Loops",
		Summary: "Learn about for and while loops", CreatedBy: userID,
	})
	mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Data Structures Arrays",
		Summary: "Working with arrays and lists", CreatedBy: userID,
	})

	// Search for "loops" — should match the first unit.
	results, err := units.SearchUnits(ctx, SearchUnitsFilter{
		Query:      "loops",
		ViewerID:   userID,
		ViewerOrgs: []string{orgID},
	})
	require.NoError(t, err)
	require.NotEmpty(t, results, "FTS search for 'loops' should return results")

	found := false
	for _, u := range results {
		if u.Title == "Introduction to Python Loops" {
			found = true
			break
		}
	}
	assert.True(t, found, "should find the loops unit via FTS")
}

func TestTeachingUnitStore_SearchUnits_BrowseMode(t *testing.T) {
	units, orgs, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()
	db := testDB(t)

	_, err := orgs.AddOrgMember(ctx, AddMemberInput{
		OrgID: orgID, UserID: userID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1 AND org_id = $2", userID, orgID)
	})

	u1 := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Browse Unit A", CreatedBy: userID,
	})
	u2 := mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Browse Unit B", CreatedBy: userID,
	})

	// Force u1 to have a newer updated_at so it comes first.
	future := time.Now().Add(1 * time.Hour)
	_, err = db.ExecContext(ctx, "UPDATE teaching_units SET updated_at = $1 WHERE id = $2", future, u1.ID)
	require.NoError(t, err)

	results, err := units.SearchUnits(ctx, SearchUnitsFilter{
		Scope:      "org",
		ScopeID:    &orgID,
		ViewerID:   userID,
		ViewerOrgs: []string{orgID},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 2)

	// u1 should come before u2 because it has a newer updated_at.
	var u1Idx, u2Idx int
	for i, u := range results {
		if u.ID == u1.ID {
			u1Idx = i
		}
		if u.ID == u2.ID {
			u2Idx = i
		}
	}
	assert.Less(t, u1Idx, u2Idx, "u1 (newer updated_at) should come before u2")
}

func TestTeachingUnitStore_SearchUnits_ScopeVisibility(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	// Create a personal unit.
	mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "My Personal Unit", CreatedBy: userID,
	})

	// Search as the owner — should see personal unit.
	results, err := units.SearchUnits(ctx, SearchUnitsFilter{
		Scope:    "personal",
		ScopeID:  &userID,
		ViewerID: userID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, results)

	// Search as a different user — should NOT see the personal unit.
	otherUser := "00000000-0000-0000-0000-000000000099"
	results2, err := units.SearchUnits(ctx, SearchUnitsFilter{
		Scope:    "personal",
		ScopeID:  &userID,
		ViewerID: otherUser,
	})
	require.NoError(t, err)
	assert.Empty(t, results2, "other user should not see personal unit")
}

func TestTeachingUnitStore_SearchUnits_EmptyResults(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	results, err := units.SearchUnits(ctx, SearchUnitsFilter{
		Query:    "xyznonexistentquerythatmatchesnothing",
		ViewerID: userID,
	})
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.NotNil(t, results, "empty results should be non-nil slice")
}

func TestTeachingUnitStore_SearchUnits_SubjectTagFilter(t *testing.T) {
	units, orgs, orgID, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()
	db := testDB(t)

	_, err := orgs.AddOrgMember(ctx, AddMemberInput{
		OrgID: orgID, UserID: userID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1 AND org_id = $2", userID, orgID)
	})

	mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Tagged Unit",
		SubjectTags: []string{"math", "cs"}, CreatedBy: userID,
	})
	mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "org", ScopeID: &orgID, Title: "Other Tagged Unit",
		SubjectTags: []string{"science"}, CreatedBy: userID,
	})

	results, err := units.SearchUnits(ctx, SearchUnitsFilter{
		SubjectTags: []string{"math"},
		ViewerID:    userID,
		ViewerOrgs:  []string{orgID},
	})
	require.NoError(t, err)

	for _, u := range results {
		found := false
		for _, tag := range u.SubjectTags {
			if tag == "math" {
				found = true
				break
			}
		}
		assert.True(t, found, "all results should have the 'math' tag, got %v for %s", u.SubjectTags, u.Title)
	}
}

func TestTeachingUnitStore_SearchUnits_PlatformAdminSeesAll(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "Admin Visible Unit",
		CreatedBy: userID,
	})

	results, err := units.SearchUnits(ctx, SearchUnitsFilter{
		IsPlatformAdmin: true,
		ViewerID:        "00000000-0000-0000-0000-000000000001",
	})
	require.NoError(t, err)
	require.NotEmpty(t, results, "platform admin should see units across scopes")
}

func TestTeachingUnitStore_SearchUnits_GradeLevelFilter(t *testing.T) {
	units, _, _, userID := setupUnitEnv(t, t.Name())
	ctx := context.Background()

	grade := "K-5"
	mustCreateUnit(t, units, CreateTeachingUnitInput{
		Scope: "personal", ScopeID: &userID, Title: "K5 Unit",
		GradeLevel: &grade, CreatedBy: userID,
	})

	results, err := units.SearchUnits(ctx, SearchUnitsFilter{
		GradeLevel: "K-5",
		ViewerID:   userID,
	})
	require.NoError(t, err)
	for _, u := range results {
		require.NotNil(t, u.GradeLevel)
		assert.Equal(t, "K-5", *u.GradeLevel)
	}
}
