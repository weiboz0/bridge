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
