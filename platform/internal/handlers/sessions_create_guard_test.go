package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
)

// Plan 047 phase 4: pre-create guard for unlinked focus areas.
// CreateSession returns 422 with code = "all_topics_unlinked" or
// "some_topics_unlinked" + unlinkedTopicTitles when the course has
// topics with no linked teaching_unit, unless the caller passes
// confirmUnlinkedTopics=true.

type guardResponse struct {
	Error               string   `json:"error"`
	Code                string   `json:"code"`
	UnlinkedTopicTitles []string `json:"unlinkedTopicTitles"`
}

func (fx *sessionFixture) postCreateSession(
	t *testing.T, claims *auth.Claims, body any,
) (int, []byte) {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(buf))
	w := httptest.NewRecorder()
	if claims != nil {
		req = withClaims(req, claims)
	}
	fx.h.CreateSession(w, req)
	return w.Code, w.Body.Bytes()
}

// Helper: insert a topic into the fixture's course and (optionally) link a
// teaching_unit. Returns the topic and (optional) unit IDs.
func (fx *sessionFixture) seedTopic(t *testing.T, title string, withUnit bool) (string, string) {
	t.Helper()
	ctx := context.Background()

	var topicID string
	err := fx.db.QueryRowContext(ctx,
		`INSERT INTO topics (id, course_id, title, sort_order, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, 0, now(), now())
		 RETURNING id`,
		fx.courseID, title,
	).Scan(&topicID)
	require.NoError(t, err)
	t.Cleanup(func() { fx.db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topicID) })

	var unitID string
	if withUnit {
		err = fx.db.QueryRowContext(ctx,
			`INSERT INTO teaching_units
			 (id, scope, scope_id, title, summary, material_type, status, created_by, topic_id, created_at, updated_at)
			 VALUES (gen_random_uuid(), 'org', $1, $2, '', 'notes', 'classroom_ready', $3, $4, now(), now())
			 RETURNING id`,
			fx.orgID, "Unit for "+title, fx.teacher.ID, topicID,
		).Scan(&unitID)
		require.NoError(t, err)
		t.Cleanup(func() { fx.db.ExecContext(ctx, "DELETE FROM teaching_units WHERE id = $1", unitID) })
	}
	return topicID, unitID
}

// All topics linked → session created normally (201, no 422).
func TestCreateSession_AllTopicsLinked_NoGuard(t *testing.T) {
	fx := newSessionFixture(t, "guardallok")
	fx.seedTopic(t, "Topic A", true)
	fx.seedTopic(t, "Topic B", true)

	code, body := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{
			"title":   "Linked Session",
			"classId": fx.classID,
		})
	require.Equal(t, http.StatusCreated, code, "all topics linked must allow create; body: %s", body)
}

// Empty course (zero topics) → no guard, session created.
func TestCreateSession_EmptyCourse_NoGuard(t *testing.T) {
	fx := newSessionFixture(t, "guardempty")
	// no topics seeded

	code, body := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{"title": "Empty Course Session", "classId": fx.classID})
	require.Equal(t, http.StatusCreated, code, "empty course must not trigger the guard; body: %s", body)
}

// All topics unlinked → 422 with code=all_topics_unlinked, session NOT created.
func TestCreateSession_AllTopicsUnlinked_Blocks(t *testing.T) {
	fx := newSessionFixture(t, "guardallunlink")
	fx.seedTopic(t, "Topic A", false)
	fx.seedTopic(t, "Topic B", false)

	code, body := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{"title": "Blocked", "classId": fx.classID})
	require.Equal(t, http.StatusUnprocessableEntity, code)

	var resp guardResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, "all_topics_unlinked", resp.Code)
	assert.Len(t, resp.UnlinkedTopicTitles, 2)
	assert.Contains(t, resp.UnlinkedTopicTitles, "Topic A")
	assert.Contains(t, resp.UnlinkedTopicTitles, "Topic B")

	// Verify no session row was inserted with this title.
	ctx := context.Background()
	var sessionsCount int
	err := fx.db.QueryRowContext(ctx,
		"SELECT count(*) FROM sessions WHERE class_id = $1 AND title = 'Blocked'",
		fx.classID,
	).Scan(&sessionsCount)
	require.NoError(t, err)
	assert.Equal(t, 0, sessionsCount, "no session should be created when guard blocks")
}

// All topics unlinked + override flag → 201 (session created).
func TestCreateSession_AllTopicsUnlinked_OverrideAllows(t *testing.T) {
	fx := newSessionFixture(t, "guardalloverride")
	fx.seedTopic(t, "Topic A", false)

	code, _ := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{
			"title":                 "Overridden",
			"classId":               fx.classID,
			"confirmUnlinkedTopics": true,
		})
	require.Equal(t, http.StatusCreated, code)
}

// Some topics unlinked → 422 with code=some_topics_unlinked.
func TestCreateSession_SomeTopicsUnlinked_Warns(t *testing.T) {
	fx := newSessionFixture(t, "guardsome")
	fx.seedTopic(t, "Linked One", true)
	fx.seedTopic(t, "Unlinked Two", false)

	code, body := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{"title": "Partial", "classId": fx.classID})
	require.Equal(t, http.StatusUnprocessableEntity, code)

	var resp guardResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, "some_topics_unlinked", resp.Code)
	assert.Equal(t, []string{"Unlinked Two"}, resp.UnlinkedTopicTitles)
}

// Some topics unlinked + override → 201.
func TestCreateSession_SomeTopicsUnlinked_OverrideAllows(t *testing.T) {
	fx := newSessionFixture(t, "guardsomeoverride")
	fx.seedTopic(t, "Linked One", true)
	fx.seedTopic(t, "Unlinked Two", false)

	code, _ := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{
			"title":                 "Partial Override",
			"classId":               fx.classID,
			"confirmUnlinkedTopics": true,
		})
	require.Equal(t, http.StatusCreated, code)
}

// No classID → ad-hoc session, guard does not run at all.
func TestCreateSession_NoClassID_NoGuard(t *testing.T) {
	fx := newSessionFixture(t, "guardnoclass")
	fx.seedTopic(t, "Lonely Topic", false) // even an unlinked topic doesn't matter

	code, _ := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{
			"title": "Ad-hoc Session",
			// classId intentionally omitted
		})
	require.Equal(t, http.StatusCreated, code)

	// Cleanup: the session this test created.
	ctx := context.Background()
	t.Cleanup(func() {
		fx.db.ExecContext(ctx,
			"DELETE FROM sessions WHERE teacher_id = $1 AND title = 'Ad-hoc Session'",
			fx.teacher.ID)
	})
}

// Misconfiguration: if Topics or TeachingUnits is nil on the handler,
// CreateSession returns 500 instead of silently bypassing the guard.
// Codex post-impl review caught this — silent skip would let an
// unguarded session creation through if someone forgot to wire the
// stores. Production main.go wires both; this test is a contract
// belt-and-suspenders.
func TestCreateSession_NilStores_FailsLoud(t *testing.T) {
	fx := newSessionFixture(t, "guardnilstore")
	fx.seedTopic(t, "Topic A", false)

	// Detach the topic store; the handler should now fail loud.
	saved := fx.h.Topics
	fx.h.Topics = nil
	t.Cleanup(func() { fx.h.Topics = saved })

	code, _ := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{"title": "Misconfigured", "classId": fx.classID})
	require.Equal(t, http.StatusInternalServerError, code,
		"missing Topics store must NOT silently skip the guard")
}

// Plan 048 phase 1: CreateSession atomically snapshots the course's
// topics into session_topics. After create, the new session's agenda
// matches the course's topics list (NOT empty as it was pre-048).
func TestCreateSession_SnapshotsSessionTopics(t *testing.T) {
	fx := newSessionFixture(t, "snapshot")
	topicA, _ := fx.seedTopic(t, "Topic A", true)
	topicB, _ := fx.seedTopic(t, "Topic B", true)

	code, body := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{"title": "Snapshot Session", "classId": fx.classID})
	require.Equal(t, http.StatusCreated, code)

	var session map[string]any
	require.NoError(t, json.Unmarshal(body, &session))
	sessionID := session["id"].(string)

	// Both topics ended up in session_topics.
	rows, err := fx.db.Query(
		"SELECT topic_id FROM session_topics WHERE session_id = $1 ORDER BY topic_id",
		sessionID,
	)
	require.NoError(t, err)
	defer rows.Close()

	var got []string
	for rows.Next() {
		var topicID string
		require.NoError(t, rows.Scan(&topicID))
		got = append(got, topicID)
	}
	expected := []string{topicA, topicB}
	if expected[0] > expected[1] {
		expected[0], expected[1] = expected[1], expected[0]
	}
	assert.Equal(t, expected, got, "all course topics should be snapshotted")
}

// Override path also snapshots — the teacher consciously chose to
// start with a partial syllabus, so empty-agenda is not the answer.
func TestCreateSession_OverrideAlsoSnapshots(t *testing.T) {
	fx := newSessionFixture(t, "overridesnap")
	fx.seedTopic(t, "Unlinked A", false)

	code, body := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{
			"title":                 "Override Snapshot",
			"classId":               fx.classID,
			"confirmUnlinkedTopics": true,
		})
	require.Equal(t, http.StatusCreated, code)

	var session map[string]any
	require.NoError(t, json.Unmarshal(body, &session))
	sessionID := session["id"].(string)

	var count int
	err := fx.db.QueryRow(
		"SELECT count(*) FROM session_topics WHERE session_id = $1",
		sessionID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "override path must still snapshot the agenda")
}

// Empty course → no snapshot rows.
func TestCreateSession_EmptyCourse_NoSnapshot(t *testing.T) {
	fx := newSessionFixture(t, "emptynosnap")
	// no topics seeded

	code, body := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{"title": "Empty Course Snapshot", "classId": fx.classID})
	require.Equal(t, http.StatusCreated, code)

	var session map[string]any
	require.NoError(t, json.Unmarshal(body, &session))
	sessionID := session["id"].(string)

	var count int
	err := fx.db.QueryRow(
		"SELECT count(*) FROM session_topics WHERE session_id = $1",
		sessionID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// Ad-hoc session (no classId) → no snapshot.
func TestCreateSession_NoClassID_NoSnapshot(t *testing.T) {
	fx := newSessionFixture(t, "adhocnosnap")
	// Seed a topic on the fixture's course — should NOT be snapshotted
	// because the session has no classId.
	fx.seedTopic(t, "Ignored Topic", true)

	ctx := context.Background()
	code, body := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{"title": "Ad-hoc"})
	require.Equal(t, http.StatusCreated, code)

	var session map[string]any
	require.NoError(t, json.Unmarshal(body, &session))
	sessionID := session["id"].(string)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", sessionID)
	})

	var count int
	err := fx.db.QueryRowContext(ctx,
		"SELECT count(*) FROM session_topics WHERE session_id = $1",
		sessionID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// Only the unlinked titles surface in the response — never linked ones.
func TestCreateSession_OnlyUnlinkedTitlesInResponse(t *testing.T) {
	fx := newSessionFixture(t, "guardonlyunlink")
	fx.seedTopic(t, "Should Not Appear", true)
	fx.seedTopic(t, "Should Appear", false)

	_, body := fx.postCreateSession(t,
		fx.claims(fx.teacher, false),
		map[string]any{"title": "Mixed", "classId": fx.classID})

	var resp guardResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.NotContains(t, resp.UnlinkedTopicTitles, "Should Not Appear")
	assert.Contains(t, resp.UnlinkedTopicTitles, "Should Appear")
}
