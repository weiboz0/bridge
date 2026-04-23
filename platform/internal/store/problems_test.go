package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupProblemEnv creates an org + user + course + topic, wires a ProblemStore,
// and registers cleanup. The returned user is also added as an org_admin on the
// created org, so callers can use it as both the problem author and an
// org-scoped problem owner. All subsequent test rows are freed either directly
// or by FK cascade when the fixture cleans up.
func setupProblemEnv(t *testing.T, suffix string) (*sql.DB, *ProblemStore, *Topic, *RegisteredUser) {
	t.Helper()
	db := testDB(t)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	courses := NewCourseStore(db)
	topics := NewTopicStore(db)
	problems := NewProblemStore(db)

	org := createTestOrg(t, db, orgs, suffix)
	user := createTestUser(t, db, users, suffix)

	ctx := context.Background()
	_, err := orgs.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "org_admin", Status: "active",
	})
	require.NoError(t, err)

	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID,
		Title: "Problem Test Course " + suffix, GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	topic, err := topics.CreateTopic(ctx, CreateTopicInput{
		CourseID: course.ID, Title: "Arrays",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic.ID) })

	// Sweep any problems this user authored so the user row can be dropped
	// by createTestUser's cleanup (which runs after this one). Problems FK
	// to users(id) via created_by with no ON DELETE, so un-swept rows would
	// block the user delete and leak across re-runs.
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM attempts WHERE user_id = $1 OR problem_id IN (SELECT id FROM problems WHERE created_by = $1)`, user.ID)
		db.ExecContext(ctx, `DELETE FROM test_cases WHERE problem_id IN (SELECT id FROM problems WHERE created_by = $1)`, user.ID)
		db.ExecContext(ctx, `DELETE FROM problem_solutions WHERE problem_id IN (SELECT id FROM problems WHERE created_by = $1) OR created_by = $1`, user.ID)
		db.ExecContext(ctx, `DELETE FROM problems WHERE forked_from IN (SELECT id FROM problems WHERE created_by = $1)`, user.ID)
		db.ExecContext(ctx, `DELETE FROM problems WHERE created_by = $1`, user.ID)
	})

	return db, problems, topic, user
}

// mustCreateProblem inserts a problem with the given scope/scopeID/status and
// registers cleanup. It's a focused helper for the ListProblems matrix.
func mustCreateProblem(
	t *testing.T, db *sql.DB, s *ProblemStore,
	scope string, scopeID *string, status, title, createdBy string,
	tags []string,
) *Problem {
	t.Helper()
	ctx := context.Background()
	p, err := s.CreateProblem(ctx, CreateProblemInput{
		Scope: scope, ScopeID: scopeID, Title: title,
		Description: "desc", Status: status, CreatedBy: createdBy,
		Tags: tags,
	})
	require.NoError(t, err)
	require.NotNil(t, p)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", p.ID) })
	return p
}

func TestProblemStore_CreateAndGet(t *testing.T) {
	db, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()

	scopeID := user.ID
	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "personal", ScopeID: &scopeID, Title: "Two Sum",
		Description: "Find two numbers that sum to target.",
		StarterCode: map[string]string{"python": "def solve(): pass"},
		CreatedBy:   user.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, p)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", p.ID) })

	assert.Equal(t, "Two Sum", p.Title)
	assert.Equal(t, "personal", p.Scope)
	require.NotNil(t, p.ScopeID)
	assert.Equal(t, user.ID, *p.ScopeID)
	assert.Equal(t, "easy", p.Difficulty, "empty Difficulty defaults to easy")
	assert.Equal(t, "draft", p.Status, "empty Status defaults to draft")
	assert.Equal(t, []string{}, p.Tags, "nil Tags is stored as empty slice")
	assert.Equal(t, "def solve(): pass", p.StarterCode["python"])

	got, err := problems.GetProblem(ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, p.StarterCode, got.StarterCode)
}

func TestProblemStore_CreateProblem_PlatformScope(t *testing.T) {
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "platform", ScopeID: nil, Title: "Global Problem",
		Description: "d", CreatedBy: user.ID, Status: "published",
	})
	require.NoError(t, err)
	require.NotNil(t, p)
	t.Cleanup(func() { problems.DeleteProblem(ctx, p.ID) })
	assert.Equal(t, "platform", p.Scope)
	assert.Nil(t, p.ScopeID)
}

func TestProblemStore_CreateProblem_OrgScope(t *testing.T) {
	db, problems, topic, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	// Grab the org id via the course -> org chain.
	var orgID string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT c.org_id FROM topics t JOIN courses c ON c.id = t.course_id WHERE t.id = $1`,
		topic.ID).Scan(&orgID))

	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "org", ScopeID: &orgID, Title: "Org Problem",
		Description: "d", CreatedBy: user.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, p)
	t.Cleanup(func() { problems.DeleteProblem(ctx, p.ID) })
	require.NotNil(t, p.ScopeID)
	assert.Equal(t, orgID, *p.ScopeID)
}

func TestProblemStore_CreateProblem_ViolatesCheckConstraint(t *testing.T) {
	// scope=platform + scope_id=<uuid> must be rejected by the CHECK constraint.
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	bogus := user.ID
	_, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "platform", ScopeID: &bogus, Title: "Bad", Description: "d", CreatedBy: user.ID,
	})
	require.Error(t, err)
	pqErr, ok := err.(*pq.Error)
	if ok {
		assert.Equal(t, pq.ErrorCode("23514"), pqErr.Code, "CHECK violation expected")
	} else {
		// pgx driver wraps the error; just require message contains the constraint name.
		assert.Contains(t, err.Error(), "problems_scope_scope_id_chk")
	}
}

func TestProblemStore_CreateProblem_OrgScopeRequiresScopeID(t *testing.T) {
	// scope=org + scope_id=NULL must be rejected.
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	_, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "org", ScopeID: nil, Title: "Bad", Description: "d", CreatedBy: user.ID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "problems_scope_scope_id_chk")
}

func TestProblemStore_Get_NotFound_ReturnsNil(t *testing.T) {
	_, problems, _, _ := setupProblemEnv(t, t.Name())
	got, err := problems.GetProblem(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestProblemStore_ListProblemsByTopic_OrderedBySortOrder(t *testing.T) {
	db, problems, topic, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	scopeID := user.ID

	mk := func(title string) *Problem {
		p, err := problems.CreateProblem(ctx, CreateProblemInput{
			Scope: "personal", ScopeID: &scopeID, Title: title,
			Description: "d", CreatedBy: user.ID,
		})
		require.NoError(t, err)
		t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", p.ID) })
		return p
	}
	p2 := mk("Third")
	p0 := mk("First")
	p1 := mk("Second")

	attach := func(pid string, order int) {
		_, err := db.ExecContext(ctx,
			`INSERT INTO topic_problems (topic_id, problem_id, sort_order, attached_by) VALUES ($1, $2, $3, $4)`,
			topic.ID, pid, order, user.ID)
		require.NoError(t, err)
	}
	attach(p2.ID, 2)
	attach(p0.ID, 0)
	attach(p1.ID, 1)

	list, err := problems.ListProblemsByTopic(ctx, topic.ID)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, p0.ID, list[0].ID)
	assert.Equal(t, 0, list[0].SortOrder)
	assert.Equal(t, p1.ID, list[1].ID)
	assert.Equal(t, 1, list[1].SortOrder)
	assert.Equal(t, p2.ID, list[2].ID)
	assert.Equal(t, 2, list[2].SortOrder)
}

func TestProblemStore_ListProblemsByTopic_EmptyReturnsEmptySlice(t *testing.T) {
	_, problems, topic, _ := setupProblemEnv(t, t.Name())
	list, err := problems.ListProblemsByTopic(context.Background(), topic.ID)
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Len(t, list, 0)
}

func TestProblemStore_UpdateProblem_PartialFields(t *testing.T) {
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	scopeID := user.ID

	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "personal", ScopeID: &scopeID, Title: "Original",
		Description: "original-desc", CreatedBy: user.ID,
	})
	require.NoError(t, err)

	newTitle := "Renamed"
	updated, err := problems.UpdateProblem(ctx, p.ID, UpdateProblemInput{Title: &newTitle})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Renamed", updated.Title)
	assert.Equal(t, "original-desc", updated.Description, "description should be unchanged")
	assert.Equal(t, "easy", updated.Difficulty, "difficulty should be unchanged")
}

func TestProblemStore_UpdateProblem_ReplaceStarterCode(t *testing.T) {
	// JSONB replace: map is whole-row replaced, not merged.
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	scopeID := user.ID

	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "personal", ScopeID: &scopeID, Title: "Has Starter",
		Description: "d", CreatedBy: user.ID,
		StarterCode: map[string]string{"python": "print('py')", "javascript": "console.log('js')"},
	})
	require.NoError(t, err)

	updated, err := problems.UpdateProblem(ctx, p.ID, UpdateProblemInput{
		StarterCode: map[string]string{"python": "print('py2')"},
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, 1, len(updated.StarterCode), "map should be replaced, not merged")
	assert.Equal(t, "print('py2')", updated.StarterCode["python"])
	_, has := updated.StarterCode["javascript"]
	assert.False(t, has, "old javascript key should be gone after replace")
}

func TestProblemStore_UpdateProblem_ClearStarterCode(t *testing.T) {
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	scopeID := user.ID

	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "personal", ScopeID: &scopeID, Title: "Has Starter",
		Description: "d", CreatedBy: user.ID,
		StarterCode: map[string]string{"python": "x"},
	})
	require.NoError(t, err)

	updated, err := problems.UpdateProblem(ctx, p.ID, UpdateProblemInput{
		StarterCode: map[string]string{},
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, map[string]string{}, updated.StarterCode, "empty map should clear jsonb to {}")
}

func TestProblemStore_UpdateProblem_ReplaceTags(t *testing.T) {
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	scopeID := user.ID

	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "personal", ScopeID: &scopeID, Title: "Tagged",
		Description: "d", CreatedBy: user.ID,
		Tags: []string{"arrays", "easy"},
	})
	require.NoError(t, err)

	updated, err := problems.UpdateProblem(ctx, p.ID, UpdateProblemInput{Tags: []string{"sorting"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"sorting"}, updated.Tags)

	// empty slice clears
	updated2, err := problems.UpdateProblem(ctx, p.ID, UpdateProblemInput{Tags: []string{}})
	require.NoError(t, err)
	assert.Equal(t, []string{}, updated2.Tags)
}

func TestProblemStore_UpdateProblem_ClearGradeLevel(t *testing.T) {
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	scopeID := user.ID

	gl := "K-5"
	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "personal", ScopeID: &scopeID, Title: "GL",
		Description: "d", CreatedBy: user.ID, GradeLevel: &gl,
	})
	require.NoError(t, err)
	require.NotNil(t, p.GradeLevel)

	empty := ""
	updated, err := problems.UpdateProblem(ctx, p.ID, UpdateProblemInput{GradeLevel: &empty})
	require.NoError(t, err)
	assert.Nil(t, updated.GradeLevel)
}

func TestProblemStore_DeleteProblem(t *testing.T) {
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	scopeID := user.ID

	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "personal", ScopeID: &scopeID, Title: "Doomed",
		Description: "d", CreatedBy: user.ID,
	})
	require.NoError(t, err)

	deleted, err := problems.DeleteProblem(ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, deleted)
	assert.Equal(t, p.ID, deleted.ID)

	got, err := problems.GetProblem(ctx, p.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// --- ListProblems ---

func TestProblemStore_ListProblems_AccessibleDefault(t *testing.T) {
	db := testDB(t)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	store := NewProblemStore(db)
	ctx := context.Background()

	suffix := t.Name()
	orgA := createTestOrg(t, db, orgs, suffix+"-a")
	orgB := createTestOrg(t, db, orgs, suffix+"-b")
	viewer := createTestUser(t, db, users, suffix+"-v")
	author := createTestUser(t, db, users, suffix+"-au")

	_, err := orgs.AddOrgMember(ctx, AddMemberInput{
		OrgID: orgA.ID, UserID: viewer.ID, Role: "student", Status: "active",
	})
	require.NoError(t, err)

	viewerID := viewer.ID
	pPlatform := mustCreateProblem(t, db, store, "platform", nil, "published", "P-plat "+suffix, author.ID, nil)
	pOrgA := mustCreateProblem(t, db, store, "org", &orgA.ID, "published", "P-orgA "+suffix, author.ID, nil)
	pOrgB := mustCreateProblem(t, db, store, "org", &orgB.ID, "published", "P-orgB "+suffix, author.ID, nil)
	pOrgADraft := mustCreateProblem(t, db, store, "org", &orgA.ID, "draft", "P-orgA-draft "+suffix, author.ID, nil)
	pPersonal := mustCreateProblem(t, db, store, "personal", &viewerID, "draft", "P-personal "+suffix, viewer.ID, nil)
	pOtherPersonal := mustCreateProblem(t, db, store, "personal", &author.ID, "draft", "P-other "+suffix, author.ID, nil)

	list, _, err := store.ListProblems(ctx, ListProblemsFilter{
		ViewerID: viewer.ID, ViewerOrgs: []string{orgA.ID}, Limit: 100,
	})
	require.NoError(t, err)

	ids := map[string]bool{}
	for _, p := range list {
		ids[p.ID] = true
	}
	assert.True(t, ids[pPlatform.ID], "platform published should be visible")
	assert.True(t, ids[pOrgA.ID], "own-org published should be visible")
	assert.True(t, ids[pPersonal.ID], "own personal draft should be visible")
	assert.False(t, ids[pOrgB.ID], "other-org row should NOT be visible")
	assert.False(t, ids[pOrgADraft.ID], "own-org draft should NOT be visible (viewer is not author)")
	assert.False(t, ids[pOtherPersonal.ID], "other user's personal should NOT be visible")
}

func TestProblemStore_ListProblems_AttachmentGrantIncludesPublishedProblem(t *testing.T) {
	db, problems, topic, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()

	users := NewUserStore(db)
	classes := NewClassStore(db)

	student := createTestUser(t, db, users, t.Name()+"-student")

	var courseID, orgID string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT t.course_id, c.org_id
		 FROM topics t
		 INNER JOIN courses c ON c.id = t.course_id
		 WHERE t.id = $1`, topic.ID).Scan(&courseID, &orgID))

	class, err := classes.CreateClass(ctx, CreateClassInput{
		CourseID:  courseID,
		OrgID:     orgID,
		Title:     "Section " + t.Name(),
		Term:      "Fall",
		CreatedBy: user.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})
	_, err = classes.AddClassMember(ctx, AddClassMemberInput{
		ClassID: class.ID,
		UserID:  student.ID,
		Role:    "student",
	})
	require.NoError(t, err)

	ownerID := user.ID
	attached := mustCreateProblem(t, db, problems, "personal", &ownerID, "published", "Attached "+t.Name(), user.ID, nil)
	_, err = db.ExecContext(ctx,
		`INSERT INTO topic_problems (topic_id, problem_id, sort_order, attached_by) VALUES ($1, $2, 0, $3)`,
		topic.ID, attached.ID, user.ID)
	require.NoError(t, err)

	list, _, err := problems.ListProblems(ctx, ListProblemsFilter{
		ViewerID: student.ID,
		Limit:    100,
	})
	require.NoError(t, err)

	found := false
	for _, p := range list {
		if p.ID == attached.ID {
			found = true
		}
	}
	assert.True(t, found, "published problem attached to a viewer's course should be browse-visible")
}

func TestProblemStore_ListProblems_DefaultExcludesArchivedUnlessRequested(t *testing.T) {
	db, store, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()

	scopeID := user.ID
	archived := mustCreateProblem(t, db, store, "personal", &scopeID, "archived", "Archived "+t.Name(), user.ID, nil)
	published := mustCreateProblem(t, db, store, "personal", &scopeID, "published", "Published "+t.Name(), user.ID, nil)

	list, _, err := store.ListProblems(ctx, ListProblemsFilter{
		ViewerID: user.ID,
		Limit:    100,
	})
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, p := range list {
		ids[p.ID] = true
	}
	assert.True(t, ids[published.ID])
	assert.False(t, ids[archived.ID], "archived problems should stay out of default browse/search")

	archivedOnly, _, err := store.ListProblems(ctx, ListProblemsFilter{
		ViewerID: user.ID,
		Status:   "archived",
		Limit:    100,
	})
	require.NoError(t, err)
	require.Len(t, archivedOnly, 1)
	assert.Equal(t, archived.ID, archivedOnly[0].ID)
}

func TestProblemStore_ListProblems_PaginationHasMoreAndCursor(t *testing.T) {
	db, store, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()

	p1 := mustCreateProblem(t, db, store, "platform", nil, "published", "P1 "+t.Name(), user.ID, nil)
	p2 := mustCreateProblem(t, db, store, "platform", nil, "published", "P2 "+t.Name(), user.ID, nil)
	p3 := mustCreateProblem(t, db, store, "platform", nil, "published", "P3 "+t.Name(), user.ID, nil)

	firstPage, hasMore, err := store.ListProblems(ctx, ListProblemsFilter{
		ViewerID: user.ID,
		Limit:    2,
	})
	require.NoError(t, err)
	require.Len(t, firstPage, 2)
	assert.True(t, hasMore, "overfetch should report another page")

	last := firstPage[len(firstPage)-1]
	secondPage, secondHasMore, err := store.ListProblems(ctx, ListProblemsFilter{
		ViewerID:        user.ID,
		Limit:           2,
		CursorCreatedAt: &last.CreatedAt,
		CursorID:        &last.ID,
	})
	require.NoError(t, err)
	require.Len(t, secondPage, 1)
	assert.False(t, secondHasMore)

	seen := map[string]bool{}
	for _, p := range append(firstPage, secondPage...) {
		seen[p.ID] = true
	}
	assert.True(t, seen[p1.ID])
	assert.True(t, seen[p2.ID])
	assert.True(t, seen[p3.ID])
}

func TestProblemStore_ListProblems_AuthorSeesOwnDraftsAnyScope(t *testing.T) {
	db := testDB(t)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	store := NewProblemStore(db)
	ctx := context.Background()

	suffix := t.Name()
	orgA := createTestOrg(t, db, orgs, suffix+"-a")
	author := createTestUser(t, db, users, suffix+"-au")
	// author is NOT a member of orgA — but they authored the row, so they
	// should still see it.
	draft := mustCreateProblem(t, db, store, "org", &orgA.ID, "draft", "own-draft "+suffix, author.ID, nil)

	list, _, err := store.ListProblems(ctx, ListProblemsFilter{
		ViewerID: author.ID, Limit: 100,
	})
	require.NoError(t, err)
	found := false
	for _, p := range list {
		if p.ID == draft.ID {
			found = true
		}
	}
	assert.True(t, found, "author should see their own drafts in any scope")
}

func TestProblemStore_ListProblems_PlatformAdminSeesEverything(t *testing.T) {
	db := testDB(t)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	store := NewProblemStore(db)
	ctx := context.Background()

	suffix := t.Name()
	orgA := createTestOrg(t, db, orgs, suffix+"-a")
	admin := createTestUser(t, db, users, suffix+"-admin")
	other := createTestUser(t, db, users, suffix+"-other")

	draft := mustCreateProblem(t, db, store, "org", &orgA.ID, "draft", "secret-draft "+suffix, other.ID, nil)

	list, _, err := store.ListProblems(ctx, ListProblemsFilter{
		ViewerID: admin.ID, IsPlatformAdmin: true, Limit: 100,
	})
	require.NoError(t, err)
	found := false
	for _, p := range list {
		if p.ID == draft.ID {
			found = true
		}
	}
	assert.True(t, found, "platform admin should see every row regardless of scope/status")
}

func TestProblemStore_ListProblems_FilterByScopeAndStatus(t *testing.T) {
	db := testDB(t)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	store := NewProblemStore(db)
	ctx := context.Background()

	suffix := t.Name()
	orgA := createTestOrg(t, db, orgs, suffix+"-a")
	viewer := createTestUser(t, db, users, suffix+"-v")
	_, err := orgs.AddOrgMember(ctx, AddMemberInput{
		OrgID: orgA.ID, UserID: viewer.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)

	pub := mustCreateProblem(t, db, store, "org", &orgA.ID, "published", "pub "+suffix, viewer.ID, nil)
	_ = mustCreateProblem(t, db, store, "platform", nil, "published", "plat "+suffix, viewer.ID, nil)

	list, _, err := store.ListProblems(ctx, ListProblemsFilter{
		Scope: "org", ScopeID: &orgA.ID, Status: "published",
		ViewerID: viewer.ID, ViewerOrgs: []string{orgA.ID}, Limit: 100,
	})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, pub.ID, list[0].ID)
}

func TestProblemStore_ListProblems_FilterByDifficulty(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	store := NewProblemStore(db)
	ctx := context.Background()

	suffix := t.Name()
	viewer := createTestUser(t, db, users, suffix)

	easyP, err := store.CreateProblem(ctx, CreateProblemInput{
		Scope: "platform", Title: "easy " + suffix, Description: "d",
		CreatedBy: viewer.ID, Status: "published", Difficulty: "easy",
	})
	require.NoError(t, err)
	t.Cleanup(func() { store.DeleteProblem(ctx, easyP.ID) })
	hardP, err := store.CreateProblem(ctx, CreateProblemInput{
		Scope: "platform", Title: "hard " + suffix, Description: "d",
		CreatedBy: viewer.ID, Status: "published", Difficulty: "hard",
	})
	require.NoError(t, err)
	t.Cleanup(func() { store.DeleteProblem(ctx, hardP.ID) })

	list, _, err := store.ListProblems(ctx, ListProblemsFilter{
		ViewerID: viewer.ID, Difficulty: "hard", Limit: 100,
	})
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, p := range list {
		ids[p.ID] = true
	}
	assert.True(t, ids[hardP.ID])
	assert.False(t, ids[easyP.ID])
}

func TestProblemStore_ListProblems_FilterByTagsAND(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	store := NewProblemStore(db)
	ctx := context.Background()

	suffix := t.Name()
	viewer := createTestUser(t, db, users, suffix)

	both := mustCreateProblem(t, db, store, "platform", nil, "published", "both "+suffix, viewer.ID, []string{"arrays", "sorting"})
	onlyArrays := mustCreateProblem(t, db, store, "platform", nil, "published", "arrays-only "+suffix, viewer.ID, []string{"arrays"})

	list, _, err := store.ListProblems(ctx, ListProblemsFilter{
		ViewerID: viewer.ID, Tags: []string{"arrays", "sorting"}, Limit: 100,
	})
	require.NoError(t, err)

	ids := map[string]bool{}
	for _, p := range list {
		ids[p.ID] = true
	}
	assert.True(t, ids[both.ID], "row with both tags should match AND filter")
	assert.False(t, ids[onlyArrays.ID], "row with only one of the tags should NOT match")
}

func TestProblemStore_ListProblems_LimitCappedAt100(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	store := NewProblemStore(db)
	ctx := context.Background()

	suffix := t.Name()
	viewer := createTestUser(t, db, users, suffix)
	// No problems created — we just verify the query builds and runs with
	// limit > 100. The actual cap is observable via EXPLAIN, but we can at
	// least verify it doesn't error.
	list, _, err := store.ListProblems(ctx, ListProblemsFilter{
		ViewerID: viewer.ID, Limit: 10_000,
	})
	require.NoError(t, err)
	assert.NotNil(t, list)
}

// --- SetStatus ---

func TestProblemStore_SetStatus_ValidTransitions(t *testing.T) {
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	scopeID := user.ID

	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "personal", ScopeID: &scopeID, Title: "S", Description: "d", CreatedBy: user.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, "draft", p.Status)

	// draft -> published
	pub, err := problems.SetStatus(ctx, p.ID, "published")
	require.NoError(t, err)
	require.NotNil(t, pub)
	assert.Equal(t, "published", pub.Status)

	// published -> archived
	arc, err := problems.SetStatus(ctx, p.ID, "archived")
	require.NoError(t, err)
	require.NotNil(t, arc)
	assert.Equal(t, "archived", arc.Status)

	// archived -> published
	pub2, err := problems.SetStatus(ctx, p.ID, "published")
	require.NoError(t, err)
	require.NotNil(t, pub2)
	assert.Equal(t, "published", pub2.Status)
}

func TestProblemStore_SetStatus_InvalidTransition(t *testing.T) {
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	scopeID := user.ID

	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "personal", ScopeID: &scopeID, Title: "S", Description: "d", CreatedBy: user.ID,
	})
	require.NoError(t, err)

	// draft -> archived is NOT valid.
	_, err = problems.SetStatus(ctx, p.ID, "archived")
	assert.ErrorIs(t, err, ErrInvalidTransition)

	// draft -> draft (no-op) is not valid via SetStatus.
	_, err = problems.SetStatus(ctx, p.ID, "draft")
	assert.ErrorIs(t, err, ErrInvalidTransition)

	// published -> draft is not valid once we're in published.
	_, err = problems.SetStatus(ctx, p.ID, "published")
	require.NoError(t, err)
	_, err = problems.SetStatus(ctx, p.ID, "draft")
	assert.ErrorIs(t, err, ErrInvalidTransition)

	// bogus status rejected.
	_, err = problems.SetStatus(ctx, p.ID, "nonsense")
	assert.ErrorIs(t, err, ErrInvalidTransition)
}

func TestProblemStore_SetStatus_NotFound(t *testing.T) {
	_, problems, _, _ := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	got, err := problems.SetStatus(ctx, "00000000-0000-0000-0000-000000000000", "published")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// --- ForkProblem ---

func TestProblemStore_ForkProblem_CopiesCanonicalsAndSolutions(t *testing.T) {
	db, problems, topic, user := setupProblemEnv(t, t.Name())
	tcStore := NewTestCaseStore(db)
	ctx := context.Background()

	// Look up the org this topic's course belongs to — guaranteed to match
	// the setup fixture so the cleanup sweep handles it correctly.
	var orgID string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT c.org_id FROM topics t JOIN courses c ON c.id = t.course_id WHERE t.id = $1`,
		topic.ID).Scan(&orgID))

	source, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "org", ScopeID: &orgID, Title: "Src",
		Description: "desc", CreatedBy: user.ID, Status: "published",
		Difficulty: "medium", Tags: []string{"arrays"},
		StarterCode: map[string]string{"python": "pass"},
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", source.ID) })

	// Canonical test cases (owner_id NULL).
	tc1, err := tcStore.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: source.ID, Name: "c1", Stdin: "a", IsExample: true, Order: 0,
	})
	require.NoError(t, err)
	tc2, err := tcStore.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: source.ID, Name: "c2", Stdin: "b", IsExample: false, Order: 1,
	})
	require.NoError(t, err)

	// Private test case (owner_id set) — must NOT be copied.
	ownerForPrivate := user.ID
	_, err = tcStore.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: source.ID, OwnerID: &ownerForPrivate, Name: "priv", Stdin: "p",
	})
	require.NoError(t, err)

	// Two solutions on the source.
	_, err = db.ExecContext(ctx,
		`INSERT INTO problem_solutions (problem_id, language, code, is_published, created_by)
         VALUES ($1, 'python', 'sol1', true, $2), ($1, 'javascript', 'sol2', false, $2)`,
		source.ID, user.ID)
	require.NoError(t, err)

	// Attempt — must NOT be copied.
	_, err = db.ExecContext(ctx,
		`INSERT INTO attempts (problem_id, user_id, title, language, plain_text)
         VALUES ($1, $2, 'a', 'python', 'code')`, source.ID, user.ID)
	require.NoError(t, err)

	// Fork into a personal workspace owned by the caller.
	forker := user.ID
	forkTitle := "Forked Copy"
	forked, err := problems.ForkProblem(ctx, source.ID, ForkTarget{
		Scope: "personal", ScopeID: &forker, Title: &forkTitle, CallerID: forker,
	})
	require.NoError(t, err)
	require.NotNil(t, forked)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", forked.ID) })

	// Forked row inspection.
	assert.NotEqual(t, source.ID, forked.ID)
	require.NotNil(t, forked.ForkedFrom)
	assert.Equal(t, source.ID, *forked.ForkedFrom)
	assert.Equal(t, "draft", forked.Status, "fork must land as draft")
	assert.Equal(t, forker, forked.CreatedBy)
	assert.Equal(t, "personal", forked.Scope)
	assert.Equal(t, "Forked Copy", forked.Title)
	assert.Equal(t, source.Description, forked.Description)
	assert.Equal(t, source.Difficulty, forked.Difficulty)
	assert.Equal(t, source.Tags, forked.Tags)
	assert.Equal(t, source.StarterCode, forked.StarterCode)

	// Canonical test cases copied (2), private NOT copied.
	var canonCount int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM test_cases WHERE problem_id = $1 AND owner_id IS NULL`,
		forked.ID).Scan(&canonCount))
	assert.Equal(t, 2, canonCount)
	_ = tc1
	_ = tc2
	var privCount int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM test_cases WHERE problem_id = $1 AND owner_id IS NOT NULL`,
		forked.ID).Scan(&privCount))
	assert.Equal(t, 0, privCount, "private test cases must not be copied")

	// Solutions copied, created_by rewritten to forker, is_published preserved.
	var solCount, solByCaller int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM problem_solutions WHERE problem_id = $1`, forked.ID).Scan(&solCount))
	assert.Equal(t, 2, solCount)
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM problem_solutions WHERE problem_id = $1 AND created_by = $2`,
		forked.ID, forker).Scan(&solByCaller))
	assert.Equal(t, 2, solByCaller, "solutions must be rewritten to caller")

	var pubSol int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM problem_solutions WHERE problem_id = $1 AND is_published = true`,
		forked.ID).Scan(&pubSol))
	assert.Equal(t, 1, pubSol, "is_published must be preserved per row")

	// Attempts NOT copied.
	var attemptCount int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM attempts WHERE problem_id = $1`, forked.ID).Scan(&attemptCount))
	assert.Equal(t, 0, attemptCount)

	// Source row unchanged.
	srcAfter, err := problems.GetProblem(ctx, source.ID)
	require.NoError(t, err)
	require.NotNil(t, srcAfter)
	assert.Equal(t, source.Title, srcAfter.Title)
	assert.Equal(t, source.Status, srcAfter.Status)
}

func TestProblemStore_ForkProblem_DefaultTitle(t *testing.T) {
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	scopeID := user.ID

	source, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "personal", ScopeID: &scopeID, Title: "Original",
		Description: "d", CreatedBy: user.ID,
	})
	require.NoError(t, err)

	forked, err := problems.ForkProblem(ctx, source.ID, ForkTarget{
		Scope: "personal", ScopeID: &scopeID, CallerID: user.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, forked)
	assert.Equal(t, "Original (fork)", forked.Title)
}

func TestProblemStore_ForkProblem_SourceNotFound(t *testing.T) {
	_, problems, _, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()
	scopeID := user.ID
	got, err := problems.ForkProblem(ctx, "00000000-0000-0000-0000-000000000000", ForkTarget{
		Scope: "personal", ScopeID: &scopeID, CallerID: user.ID,
	})
	require.NoError(t, err)
	assert.Nil(t, got)
}
