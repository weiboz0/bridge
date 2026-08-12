package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/events"
	"github.com/weiboz0/bridge/platform/internal/realtime"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type fakeCanvasControl struct {
	bundle                     realtime.FreezeBundle
	err                        error
	onFreeze                   func(realtime.FreezeRequest)
	onComplete                 func(string, string)
	onUnfreeze                 func(string, string)
	freeze, complete, unfreeze int
}

func (f *fakeCanvasControl) Freeze(_ context.Context, request realtime.FreezeRequest) (realtime.FreezeBundle, error) {
	f.freeze++
	if f.onFreeze != nil {
		f.onFreeze(request)
	}
	return f.bundle, f.err
}
func (f *fakeCanvasControl) Complete(_ context.Context, sessionID, token string) {
	f.complete++
	if f.onComplete != nil {
		f.onComplete(sessionID, token)
	}
}
func (f *fakeCanvasControl) Unfreeze(_ context.Context, sessionID, token string) {
	f.unfreeze++
	if f.onUnfreeze != nil {
		f.onUnfreeze(sessionID, token)
	}
}

// sessionFixture is the world a session integration test runs against.
type sessionFixture struct {
	db        *sql.DB
	h         *SessionHandler
	router    chi.Router
	orgs      *store.OrgStore
	classes   *store.ClassStore
	teacher   *store.RegisteredUser
	student   *store.RegisteredUser
	otherUser *store.RegisteredUser // not a participant or teacher
	orgID     string
	courseID  string
	classID   string
	sessionID string
	session   *store.LiveSession
}

type sessionListPayload struct {
	Items      []store.LiveSession `json:"items"`
	NextCursor *string             `json:"nextCursor"`
}

type publicSessionListPayload struct {
	Items      []store.PublicSessionListItem `json:"items"`
	NextCursor *string                       `json:"nextCursor"`
}

func strPtr(s string) *string { return &s }

func newSessionFixture(t *testing.T, suffix string) *sessionFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()

	orgs := store.NewOrgStore(db)
	courses := store.NewCourseStore(db)
	classes := store.NewClassStore(db)
	sessions := store.NewSessionStore(db)

	broadcaster := events.NewBroadcaster()

	h := &SessionHandler{
		Sessions:    sessions,
		Classes:     classes,
		Courses:     courses,
		Topics:      store.NewTopicStore(db),
		Chapters:    store.NewChapterStore(db),
		Orgs:        orgs,
		ParentLinks: store.NewParentLinkStore(db),
		Broadcaster: broadcaster,
	}

	// Create org, teacher, students
	org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
		Name: "Org " + suffix, Slug: "org-" + suffix,
		Type: "school", ContactEmail: suffix + "@example.com", ContactName: "Admin",
	})
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "UPDATE organizations SET status = 'active' WHERE id = $1", org.ID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})

	mkUser := func(label string) *store.RegisteredUser {
		u := insertFixtureUser(t, db, store.RegisterInput{
			Name: "User " + label, Email: label + "@example.com", Password: "testpassword123",
		})
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM session_topics WHERE session_id IN (SELECT id FROM sessions WHERE teacher_id = $1)", u.ID)
			db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id IN (SELECT id FROM sessions WHERE teacher_id = $1)", u.ID)
			db.ExecContext(ctx, "DELETE FROM sessions WHERE teacher_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM session_participants WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM parent_links WHERE parent_user_id = $1 OR child_user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}

	teacher := mkUser(suffix + "-teacher")
	student := mkUser(suffix + "-student")
	otherUser := mkUser(suffix + "-other")

	// Add teacher to org
	_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: org.ID, UserID: teacher.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)

	// Create course + class
	course, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID: org.ID, CreatedBy: teacher.ID, Title: "Course " + suffix, GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, err := classes.CreateClass(ctx, store.CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Class " + suffix, CreatedBy: teacher.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM sessions WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	// Create a live session
	session, err := sessions.CreateSession(ctx, store.CreateSessionInput{
		ClassID:   strPtr(class.ID),
		TeacherID: teacher.ID,
		Title:     "Fixture session",
	})
	require.NoError(t, err)

	// Build router with all session routes
	r := chi.NewRouter()
	h.Routes(r)

	fx := &sessionFixture{
		db:        db,
		h:         h,
		router:    r,
		orgs:      orgs,
		classes:   classes,
		teacher:   teacher,
		student:   student,
		otherUser: otherUser,
		orgID:     org.ID,
		courseID:  course.ID,
		classID:   class.ID,
		sessionID: session.ID,
		session:   session,
	}
	return fx
}

func (fx *sessionFixture) claims(u *store.RegisteredUser, admin bool) *auth.Claims {
	return &auth.Claims{UserID: u.ID, Email: u.Email, Name: u.Name, IsPlatformAdmin: admin}
}

func (fx *sessionFixture) createSession(t *testing.T, input store.CreateSessionInput) *store.LiveSession {
	t.Helper()
	session, err := fx.h.Sessions.CreateSession(context.Background(), input)
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(context.Background(), "DELETE FROM session_topics WHERE session_id = $1", session.ID)
		fx.db.ExecContext(context.Background(), "DELETE FROM session_participants WHERE session_id = $1", session.ID)
		fx.db.ExecContext(context.Background(), "DELETE FROM sessions WHERE id = $1", session.ID)
	})
	return session
}

// doRequest executes a request through the Chi router with auth claims injected.
func (fx *sessionFixture) doRequest(t *testing.T, method, path string, body any, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if claims != nil {
		req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))
	}
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, req)
	return w
}

// ------------------- POST /api/sessions + GET /api/sessions -------------------

func TestSessionHandler_CreateSession_Orphan201(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	w := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
		"title": "Office hours",
	}, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	assert.Equal(t, fx.teacher.ID, session.TeacherID)
	assert.Equal(t, "Office hours", session.Title)
	assert.Nil(t, session.ClassID)
	assert.Equal(t, "unlisted", session.Visibility)
}

func TestSessionHandler_CreateSession_PublicVisibility201(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	w := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
		"title":      "Public office hours",
		"visibility": "public",
	}, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	assert.Equal(t, "public", session.Visibility)
}

func TestSessionHandler_CreateSession_InvalidVisibility400(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	w := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
		"title":      "Bad visibility",
		"visibility": "private",
	}, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
}

func TestSessionHandler_CreateSession_OrphanPlainRegisteredUser201(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	w := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
		"title": "Plain user office hours",
	}, fx.claims(fx.otherUser, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	assert.Equal(t, fx.otherUser.ID, session.TeacherID)
	assert.Nil(t, session.ClassID)
}

func TestSessionHandler_CreateSession_OrphanStudentOnlyUser201(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()
	_, err := fx.orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: fx.orgID, UserID: fx.student.ID, Role: "student", Status: "active",
	})
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
		"title": "Student study session",
	}, fx.claims(fx.student, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	assert.Equal(t, fx.student.ID, session.TeacherID)
	assert.Nil(t, session.ClassID)
}

func TestSessionHandler_CreateSession_OrphanConcurrentLiveCap429(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	for i := 0; i < 5; i++ {
		w := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
			"title": "Ad-hoc capped " + strconv.Itoa(i+1),
		}, fx.claims(fx.otherUser, false))
		require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	}

	w := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
		"title": "Ad-hoc capped 6",
	}, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "body=%s", w.Body.String())
}

func TestSessionHandler_CreateSession_PlatformAdminExemptFromOrphanConcurrentLiveCap(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	for i := 0; i < 6; i++ {
		w := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
			"title": "Admin ad-hoc " + strconv.Itoa(i+1),
		}, fx.claims(fx.otherUser, true))
		require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	}
}

func TestSessionHandler_CreateSession_ConcurrentLiveCapIgnoresClassBoundSessions(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	for i := 0; i < 5; i++ {
		w := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
			"title":   "Class-bound " + strconv.Itoa(i+1),
			"classId": fx.classID,
		}, fx.claims(fx.teacher, false))
		require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	}

	for i := 0; i < 5; i++ {
		w := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
			"title": "Ad-hoc after class-bound " + strconv.Itoa(i+1),
		}, fx.claims(fx.teacher, false))
		require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	}

	w := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
		"title": "Ad-hoc after class-bound 6",
	}, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "body=%s", w.Body.String())
}

func TestSessionHandler_CreateSession_ClassBoundNonMember403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	w := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
		"title":   "Forbidden class-bound",
		"classId": fx.classID,
	}, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
}

func TestSessionHandler_GetSession_OrphanAccessibleByCreator(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	createResp := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
		"title": "Office hours",
	}, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusCreated, createResp.Code, "body=%s", createResp.Body.String())

	var created store.LiveSession
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))

	getResp := fx.doRequest(t, http.MethodGet, "/api/sessions/"+created.ID, nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusOK, getResp.Code)
}

func TestSessionHandler_GetSession_OrphanRandomUser404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	createResp := fx.doRequest(t, http.MethodPost, "/api/sessions", map[string]any{
		"title": "Office hours",
	}, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusCreated, createResp.Code, "body=%s", createResp.Body.String())

	var created store.LiveSession
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))

	getResp := fx.doRequest(t, http.MethodGet, "/api/sessions/"+created.ID, nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusNotFound, getResp.Code)
}

func TestSessionHandler_ListSessions_DefaultsToCaller(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	ownOrphan := fx.createSession(t, store.CreateSessionInput{
		TeacherID: fx.teacher.ID,
		Title:     "Own orphan",
	})
	otherTeacherSession := fx.createSession(t, store.CreateSessionInput{
		TeacherID: fx.otherUser.ID,
		Title:     "Other orphan",
	})

	w := fx.doRequest(t, http.MethodGet, "/api/sessions", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var payload sessionListPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Len(t, payload.Items, 2)

	gotIDs := []string{payload.Items[0].ID, payload.Items[1].ID}
	assert.Contains(t, gotIDs, fx.sessionID)
	assert.Contains(t, gotIDs, ownOrphan.ID)
	assert.NotContains(t, gotIDs, otherTeacherSession.ID)
	for _, item := range payload.Items {
		assert.Equal(t, fx.teacher.ID, item.TeacherID)
	}
}

func TestSessionHandler_ListSessions_ClassFilterOnlyReturnsClassLinkedSessions(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	classSession := fx.createSession(t, store.CreateSessionInput{
		ClassID:   strPtr(fx.classID),
		TeacherID: fx.teacher.ID,
		Title:     "Second class session",
	})
	orphanSession := fx.createSession(t, store.CreateSessionInput{
		TeacherID: fx.teacher.ID,
		Title:     "Own orphan",
	})

	w := fx.doRequest(t, http.MethodGet, "/api/sessions?classId="+fx.classID, nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var payload sessionListPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Len(t, payload.Items, 2)

	gotIDs := []string{payload.Items[0].ID, payload.Items[1].ID}
	assert.Contains(t, gotIDs, fx.sessionID)
	assert.Contains(t, gotIDs, classSession.ID)
	assert.NotContains(t, gotIDs, orphanSession.ID)
	for _, item := range payload.Items {
		require.NotNil(t, item.ClassID)
		assert.Equal(t, fx.classID, *item.ClassID)
	}
}

// ------------------- GET /api/sessions/public -------------------

func TestSessionHandler_ListPublicSessions_OnlyLivePublicAndStaticRoute(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	publicSession := fx.createSession(t, store.CreateSessionInput{
		TeacherID:  fx.teacher.ID,
		Title:      "Public live",
		Visibility: "public",
	})
	unlistedSession := fx.createSession(t, store.CreateSessionInput{
		TeacherID:  fx.teacher.ID,
		Title:      "Unlisted live",
		Visibility: "unlisted",
	})
	endedPublic := fx.createSession(t, store.CreateSessionInput{
		TeacherID:  fx.teacher.ID,
		Title:      "Public ended",
		Visibility: "public",
	})
	_, err := fx.h.Sessions.EndSession(ctx, endedPublic.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/sessions/public", nil, fx.claims(fx.otherUser, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var payload publicSessionListPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	gotIDs := make([]string, 0, len(payload.Items))
	for _, item := range payload.Items {
		gotIDs = append(gotIDs, item.ID)
	}
	assert.Contains(t, gotIDs, publicSession.ID)
	assert.NotContains(t, gotIDs, unlistedSession.ID)
	assert.NotContains(t, gotIDs, endedPublic.ID)
}

func TestSessionHandler_ListPublicSessions_PaginatesWithDisjointPages(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	for i := 0; i < 3; i++ {
		fx.createSession(t, store.CreateSessionInput{
			TeacherID:  fx.teacher.ID,
			Title:      "Public page " + strconv.Itoa(i+1),
			Visibility: "public",
		})
		time.Sleep(time.Millisecond)
	}

	first := fx.doRequest(t, http.MethodGet, "/api/sessions/public?limit=2", nil, fx.claims(fx.otherUser, false))
	require.Equal(t, http.StatusOK, first.Code, "body=%s", first.Body.String())
	var firstPayload publicSessionListPayload
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstPayload))
	require.Len(t, firstPayload.Items, 2)
	require.NotNil(t, firstPayload.NextCursor)

	second := fx.doRequest(t, http.MethodGet, "/api/sessions/public?limit=2&cursor="+*firstPayload.NextCursor, nil, fx.claims(fx.otherUser, false))
	require.Equal(t, http.StatusOK, second.Code, "body=%s", second.Body.String())
	var secondPayload publicSessionListPayload
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondPayload))
	require.NotEmpty(t, secondPayload.Items)

	firstIDs := map[string]bool{}
	for _, item := range firstPayload.Items {
		firstIDs[item.ID] = true
	}
	for _, item := range secondPayload.Items {
		assert.False(t, firstIDs[item.ID], "session %s appeared on both pages", item.ID)
	}
}

// ------------------- Phase 2 class-less access consistency -------------------

func TestSessionHandler_JoinSession_OrphanInvitedParticipantAllowed(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()
	session := fx.createSession(t, store.CreateSessionInput{
		TeacherID: fx.teacher.ID,
		Title:     "Ad-hoc invited join",
	})
	_, err := fx.h.Sessions.AddParticipant(ctx, session.ID, fx.otherUser.ID, fx.teacher.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+session.ID+"/join", nil, fx.claims(fx.otherUser, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	participant, err := fx.h.Sessions.GetSessionParticipant(ctx, session.ID, fx.otherUser.ID)
	require.NoError(t, err)
	require.NotNil(t, participant)
	assert.Equal(t, "present", participant.Status)
}

func TestSessionHandler_JoinSession_OrphanPresentParticipantAllowed(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()
	session := fx.createSession(t, store.CreateSessionInput{
		TeacherID: fx.teacher.ID,
		Title:     "Ad-hoc present join",
	})
	_, err := fx.h.Sessions.JoinSession(ctx, session.ID, fx.otherUser.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+session.ID+"/join", nil, fx.claims(fx.otherUser, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
}

func TestSessionHandler_JoinSession_OrphanLeftParticipantDenied(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()
	session := fx.createSession(t, store.CreateSessionInput{
		TeacherID: fx.teacher.ID,
		Title:     "Ad-hoc left join",
	})
	_, err := fx.h.Sessions.JoinSession(ctx, session.ID, fx.otherUser.ID)
	require.NoError(t, err)
	_, err = fx.h.Sessions.LeaveSession(ctx, session.ID, fx.otherUser.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+session.ID+"/join", nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
}

func TestSessionHandler_GetSessionTopics_OrphanParticipantAllowed(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()
	session := fx.createSession(t, store.CreateSessionInput{
		TeacherID: fx.teacher.ID,
		Title:     "Ad-hoc topics participant",
	})
	_, err := fx.h.Sessions.AddParticipant(ctx, session.ID, fx.otherUser.ID, fx.teacher.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+session.ID+"/topics", nil, fx.claims(fx.otherUser, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var topics []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &topics))
}

func TestSessionHandler_GetSessionTopics_OrphanRandomUserDenied(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	session := fx.createSession(t, store.CreateSessionInput{
		TeacherID: fx.teacher.ID,
		Title:     "Ad-hoc topics outsider",
	})

	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+session.ID+"/topics", nil, fx.claims(fx.otherUser, false))
	assert.Contains(t, []int{http.StatusNotFound, http.StatusForbidden}, w.Code, "body=%s", w.Body.String())
}

func TestSessionHandler_JoinSession_OrphanEndedSessionGone(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()
	session := fx.createSession(t, store.CreateSessionInput{
		TeacherID: fx.teacher.ID,
		Title:     "Ad-hoc ended join",
	})
	_, err := fx.h.Sessions.AddParticipant(ctx, session.ID, fx.otherUser.ID, fx.teacher.ID)
	require.NoError(t, err)
	_, err = fx.h.Sessions.EndSession(ctx, session.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+session.ID+"/join", nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusGone, w.Code, "body=%s", w.Body.String())
}

func TestSessionHandler_JoinSession_ClassBoundBehaviorUnchanged(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()
	_, err := fx.classes.AddClassMember(ctx, store.AddClassMemberInput{
		ClassID: fx.classID,
		UserID:  fx.student.ID,
		Role:    "student",
	})
	require.NoError(t, err)

	member := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/join", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusOK, member.Code, "body=%s", member.Body.String())

	outsider := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/join", nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusForbidden, outsider.Code, "body=%s", outsider.Body.String())
}

func TestSessionHandler_JoinSession_PublicOpenJoinCreatesParticipantIdempotently(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()
	session := fx.createSession(t, store.CreateSessionInput{
		TeacherID:  fx.teacher.ID,
		Title:      "Public open join",
		Visibility: "public",
	})

	first := fx.doRequest(t, http.MethodPost, "/api/sessions/"+session.ID+"/join", nil, fx.claims(fx.otherUser, false))
	require.Equal(t, http.StatusOK, first.Code, "body=%s", first.Body.String())

	participant, err := fx.h.Sessions.GetSessionParticipant(ctx, session.ID, fx.otherUser.ID)
	require.NoError(t, err)
	require.NotNil(t, participant)
	assert.Equal(t, "present", participant.Status)

	second := fx.doRequest(t, http.MethodPost, "/api/sessions/"+session.ID+"/join", nil, fx.claims(fx.otherUser, false))
	require.Equal(t, http.StatusOK, second.Code, "body=%s", second.Body.String())

	var count int
	err = fx.db.QueryRowContext(ctx,
		`SELECT count(*) FROM session_participants WHERE session_id = $1 AND user_id = $2`,
		session.ID, fx.otherUser.ID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestSessionHandler_JoinSession_UnlistedRandomUserDenied(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	session := fx.createSession(t, store.CreateSessionInput{
		TeacherID:  fx.teacher.ID,
		Title:      "Unlisted closed join",
		Visibility: "unlisted",
	})

	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+session.ID+"/join", nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
}

func TestSessionHandler_GetSessionTopics_ClassBoundParentOfParticipantAllowed(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()
	_, err := fx.h.Sessions.JoinSession(ctx, fx.sessionID, fx.student.ID)
	require.NoError(t, err)
	_, err = fx.h.ParentLinks.CreateLink(ctx, fx.otherUser.ID, fx.student.ID, fx.teacher.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/topics", nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
}

// ------------------- PATCH /api/sessions/{id} -------------------

func TestSessionHandler_PatchSession_TeacherUpdatesTitle(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"title": "New Title"}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	assert.Equal(t, "New Title", session.Title)
}

func TestSessionHandler_PatchSession_TeacherUpdatesSettings(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"settings": `{"mode":"collaborative"}`}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	assert.Contains(t, session.Settings, `"mode"`)
	assert.Contains(t, session.Settings, `"collaborative"`)
}

func TestSessionHandler_PatchSession_TeacherUpdatesInviteExpiry(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	future := time.Now().Add(24 * time.Hour).Truncate(time.Second).UTC()
	body := map[string]any{"inviteExpiresAt": future.Format(time.RFC3339)}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	require.NotNil(t, session.InviteExpiresAt)
	assert.WithinDuration(t, future, *session.InviteExpiresAt, time.Second)
}

func TestSessionHandler_PatchSession_HostUpdatesVisibility(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	// Visibility toggling to public is a class-LESS (ad-hoc) feature only; the
	// cross-org leak fix rejects public on a class-bound session (see
	// TestSessionHandler_PatchSession_ClassBoundCannotBePublic400). Exercise the
	// happy path on a class-less session the host owns.
	session := fx.createSession(t, store.CreateSessionInput{
		TeacherID: fx.teacher.ID,
		Title:     "Ad-hoc visibility toggle",
	})

	toPublic := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+session.ID, map[string]any{
		"visibility": "public",
	}, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, toPublic.Code, "body=%s", toPublic.Body.String())
	var publicSession store.LiveSession
	require.NoError(t, json.Unmarshal(toPublic.Body.Bytes(), &publicSession))
	assert.Equal(t, "public", publicSession.Visibility)

	toUnlisted := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+session.ID, map[string]any{
		"visibility": "unlisted",
	}, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, toUnlisted.Code, "body=%s", toUnlisted.Body.String())
	var unlistedSession store.LiveSession
	require.NoError(t, json.Unmarshal(toUnlisted.Body.Bytes(), &unlistedSession))
	assert.Equal(t, "unlisted", unlistedSession.Visibility)
}

func TestSessionHandler_PatchSession_NonHostCannotUpdateVisibility(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"visibility": "public"}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSessionHandler_PatchSession_InvalidVisibility400(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"visibility": "private"}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
}

func TestSessionHandler_PatchSession_NonTeacher403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"title": "Hacked"}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSessionHandler_PatchSession_AdminAllowed(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"title": "Admin Edit"}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, fx.claims(fx.otherUser, true))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_PatchSession_NotFound(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"title": "Ghost"}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/00000000-0000-0000-0000-000000000000", body, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSessionHandler_PatchSession_Unauthenticated(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"title": "Anon"}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ------------------- POST /api/sessions/{id}/rotate-invite -------------------

func TestSessionHandler_RotateInviteToken_Teacher(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	// First rotation
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var session1 store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session1))
	require.NotNil(t, session1.InviteToken)
	firstToken := *session1.InviteToken

	// Second rotation — token should change
	w2 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w2.Code)

	var session2 store.LiveSession
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &session2))
	require.NotNil(t, session2.InviteToken)
	assert.NotEqual(t, firstToken, *session2.InviteToken, "rotated token should differ")
}

func TestSessionHandler_RotateInviteToken_NonTeacher403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSessionHandler_RotateInviteToken_NotFound(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/00000000-0000-0000-0000-000000000000/rotate-invite", nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ------------------- DELETE /api/sessions/{id}/invite -------------------

func TestSessionHandler_RevokeInviteToken_Teacher(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	// Generate a token first
	fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))

	// Revoke
	w := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/invite", nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestSessionHandler_RevokeInviteToken_NonTeacher403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/invite", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSessionHandler_RevokeInviteToken_ThenTokenJoin404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	// Rotate to get a token
	w1 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w1.Code)
	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &session))
	token := *session.InviteToken

	// Revoke
	w2 := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusNoContent, w2.Code)

	// Try joining with revoked token
	w3 := fx.doRequest(t, http.MethodPost, "/api/s/"+token+"/join", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusNotFound, w3.Code)
}

// ------------------- POST /api/s/{token}/join -------------------

func TestSessionHandler_TokenJoin_HappyPath(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	// Rotate to get a token
	w1 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w1.Code)
	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &session))
	token := *session.InviteToken

	// Join via token
	w2 := fx.doRequest(t, http.MethodPost, "/api/s/"+token+"/join", nil, fx.claims(fx.student, false))
	require.Equal(t, http.StatusOK, w2.Code, "body=%s", w2.Body.String())

	var result map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &result))
	assert.Equal(t, fx.sessionID, result["sessionId"])
	assert.NotNil(t, result["participant"])
}

func TestSessionHandler_TokenJoin_UnknownToken404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/s/nonexistent_token_12345/join", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSessionHandler_TokenJoin_ExpiredToken410(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Rotate to get a token
	w1 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w1.Code)
	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &session))
	token := *session.InviteToken

	// Set expiry to the past
	past := time.Now().Add(-1 * time.Hour)
	_, err := fx.h.Sessions.SetInviteExpiry(ctx, fx.sessionID, &past)
	require.NoError(t, err)

	// Try joining
	w2 := fx.doRequest(t, http.MethodPost, "/api/s/"+token+"/join", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusGone, w2.Code)
}

func TestSessionHandler_TokenJoin_EndedSession410(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Rotate to get a token
	w1 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w1.Code)
	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &session))
	token := *session.InviteToken

	// End the session
	_, err := fx.h.Sessions.EndSession(ctx, fx.sessionID)
	require.NoError(t, err)

	// Try joining
	w2 := fx.doRequest(t, http.MethodPost, "/api/s/"+token+"/join", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusGone, w2.Code)
}

func TestSessionHandler_TokenJoin_AlreadyParticipant_Idempotent(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	// Rotate to get a token
	w1 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w1.Code)
	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &session))
	token := *session.InviteToken

	// Join twice
	w2 := fx.doRequest(t, http.MethodPost, "/api/s/"+token+"/join", nil, fx.claims(fx.student, false))
	require.Equal(t, http.StatusOK, w2.Code)

	w3 := fx.doRequest(t, http.MethodPost, "/api/s/"+token+"/join", nil, fx.claims(fx.student, false))
	require.Equal(t, http.StatusOK, w3.Code, "second join should also succeed (idempotent)")
}

func TestSessionHandler_TokenJoin_Unauthenticated401(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/s/some_token/join", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ------------------- POST /api/sessions/{id}/end (moved from PATCH) -------------------

func TestSessionHandler_EndSession_ViaPost(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/end", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	assert.Equal(t, "ended", session.Status)
}

func TestEndSession_DegradedWhenHocuspocusUnavailableWarnsTeacher(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	// Phase 9 must treat control unavailability as a degraded successful end,
	// not as a failed session end.  The public response is the handoff contract
	// for the warning UI and must expose the durable false value explicitly.
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/end", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, false, body["whiteboardServerArchiveComplete"])
	require.Equal(t, "whiteboard_server_archive_incomplete", body["warning"])
	_, hasWarningCode := body["warningCode"]
	require.False(t, hasWarningCode)

	var status string
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT status FROM sessions WHERE id = $1`, fx.sessionID).Scan(&status))
	require.Equal(t, "ended", status)
}

func TestEndSession_DatabaseFailureLeavesLiveClearsLeaseAndEmitsNoEvent(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	// The durable end statement is the only failing step.  Cleanup must use the
	// operation token to release the live lease, unfreeze Hocuspocus, and avoid
	// publishing a false session_ended event.
	trigger := "plan094_reject_end_" + strings.ReplaceAll(fx.sessionID, "-", "")
	function := trigger + "_fn"
	_, err := fx.db.ExecContext(context.Background(), fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'planned durable end failure'; END; $$;
		CREATE TRIGGER %s BEFORE UPDATE OF status ON sessions
		FOR EACH ROW WHEN (NEW.id = '%s'::uuid AND NEW.status = 'ended')
		EXECUTE FUNCTION %s();`, function, trigger, fx.sessionID, function))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = fx.db.ExecContext(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON sessions; DROP FUNCTION IF EXISTS %s();`, trigger, function))
	})

	event := make(chan struct{}, 1)
	unsubscribe := fx.h.Broadcaster.Subscribe(fx.sessionID, func(name string, _ interface{}) {
		if name == "session_ended" {
			select {
			case event <- struct{}{}:
			default:
			}
		}
	})
	defer unsubscribe()
	unfrozen := make(chan struct{}, 1)
	fx.h.CanvasControl = &fakeCanvasControl{onUnfreeze: func(_, _ string) {
		select {
		case unfrozen <- struct{}{}:
		default:
		}
	}}

	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/end", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	select {
	case <-unfrozen:
	case <-time.After(time.Second):
		t.Fatal("failed durable end did not unfreeze its matching operation")
	}
	var status string
	var token sql.NullString
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT status, canvas_freeze_token FROM sessions WHERE id = $1`, fx.sessionID).Scan(&status, &token))
	assert.Equal(t, "live", status)
	assert.False(t, token.Valid, "cleanup must clear its own lease after durable failure")
	select {
	case <-event:
		t.Fatal("session_ended emitted despite the durable transition failure")
	default:
	}
}

func TestEndSession_ConfirmedBundlePersistsBeforeCommit(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	canvases := store.NewCanvasStore(fx.db)
	canvas, err := canvases.CreateCanvas(context.Background(), store.CreateCanvasInput{SessionID: fx.sessionID, OwnerID: fx.teacher.ID, Title: "captured", Visibility: "private"})
	require.NoError(t, err)
	control := &fakeCanvasControl{bundle: realtime.FreezeBundle{Snapshots: []realtime.CanvasSnapshot{{CanvasID: canvas.ID, State: []byte("final")}}}}
	fx.h.CanvasControl = control
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/end", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, true, body["whiteboardServerArchiveComplete"])
	_, warned := body["warning"]
	require.False(t, warned)
	var state string
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT yjs_state FROM session_canvases WHERE id=$1`, canvas.ID).Scan(&state))
	require.Equal(t, "ZmluYWw=", state)
}

func TestEndSession_LeaseExpiryUsesSeparateDegradedTransaction(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	fx.h.CanvasControl = &fakeCanvasControl{onFreeze: func(request realtime.FreezeRequest) {
		_, err := fx.db.ExecContext(context.Background(), `
			UPDATE sessions
			SET canvas_freeze_until = clock_timestamp() - interval '1 millisecond'
			WHERE id = $1 AND canvas_freeze_token = $2::uuid`, request.SessionID, request.FreezeToken)
		require.NoError(t, err)
	}}

	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/end", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, false, body["whiteboardServerArchiveComplete"])
	require.Equal(t, "whiteboard_server_archive_incomplete", body["warning"])

	var status string
	var archiveComplete bool
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT status, whiteboard_server_archive_complete FROM sessions WHERE id = $1`, fx.sessionID).Scan(&status, &archiveComplete))
	require.Equal(t, "ended", status)
	require.False(t, archiveComplete)
}

func TestPostCommitSettlementUsesFreshContextForExplicitAndReplacementEnds(t *testing.T) {
	t.Run("explicit end", func(t *testing.T) {
		fx := newSessionFixture(t, t.Name())
		schedules := store.NewScheduleStore(fx.db)
		fx.h.Schedules = schedules
		schedule, err := schedules.CreateSchedule(context.Background(), store.CreateScheduleInput{
			ClassID: fx.classID, TeacherID: fx.teacher.ID,
			ScheduledStart: time.Now().Add(time.Hour), ScheduledEnd: time.Now().Add(2 * time.Hour),
		})
		require.NoError(t, err)
		_, err = fx.db.ExecContext(context.Background(), `UPDATE scheduled_sessions SET status = 'in_progress' WHERE id = $1`, schedule.ID)
		require.NoError(t, err)
		_, err = fx.db.ExecContext(context.Background(), `UPDATE sessions SET scheduled_session_id = $2 WHERE id = $1`, fx.sessionID, schedule.ID)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		unsubscribe := fx.h.Broadcaster.Subscribe(fx.sessionID, func(event string, _ interface{}) {
			require.Equal(t, "session_ended", event)
			var status string
			require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT status FROM sessions WHERE id = $1`, fx.sessionID).Scan(&status))
			require.Equal(t, "ended", status, "event must follow the durable transition")
			cancel()
		})
		defer unsubscribe()
		completed := make(chan error, 1)
		fx.h.CanvasControl = &fakeCanvasControl{onComplete: func(sessionID, _ string) {
			completedSchedule, err := schedules.GetSchedule(context.Background(), schedule.ID)
			if err != nil {
				completed <- err
				return
			}
			if sessionID != fx.sessionID || completedSchedule.Status != "completed" {
				completed <- fmt.Errorf("complete ran before schedule settlement: session=%s schedule=%s", sessionID, completedSchedule.Status)
				return
			}
			completed <- nil
		}}

		req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+fx.sessionID+"/end", nil)
		req = req.WithContext(auth.ContextWithClaims(ctx, fx.claims(fx.teacher, false)))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		select {
		case err := <-completed:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("post-commit terminal completion did not run")
		}
	})

	t.Run("replacement end", func(t *testing.T) {
		fx := newSessionFixture(t, t.Name())
		schedules := store.NewScheduleStore(fx.db)
		schedule, err := schedules.CreateSchedule(context.Background(), store.CreateScheduleInput{
			ClassID: fx.classID, TeacherID: fx.teacher.ID,
			ScheduledStart: time.Now().Add(time.Hour), ScheduledEnd: time.Now().Add(2 * time.Hour),
		})
		require.NoError(t, err)
		_, err = fx.db.ExecContext(context.Background(), `UPDATE scheduled_sessions SET status = 'in_progress' WHERE id = $1`, schedule.ID)
		require.NoError(t, err)
		_, err = fx.db.ExecContext(context.Background(), `UPDATE sessions SET scheduled_session_id = $2, canvas_freeze_token = $3::uuid, canvas_freeze_until = clock_timestamp() + interval '15 seconds' WHERE id = $1`, fx.sessionID, schedule.ID, "11111111-1111-4111-8111-111111111111")
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		broadcaster := events.NewBroadcaster()
		eventDelivered := make(chan error, 1)
		unsubscribe := broadcaster.Subscribe(fx.sessionID, func(event string, _ interface{}) {
			if event != "session_ended" {
				eventDelivered <- fmt.Errorf("unexpected replacement event %q", event)
				return
			}
			settled, err := schedules.GetSchedule(context.Background(), schedule.ID)
			if err != nil {
				eventDelivered <- err
				return
			}
			if settled.Status != "completed" {
				eventDelivered <- fmt.Errorf("replacement event preceded schedule settlement: %s", settled.Status)
				return
			}
			eventDelivered <- nil
		})
		defer unsubscribe()
		completed := make(chan error, 1)
		control := &fakeCanvasControl{onComplete: func(sessionID, _ string) {
			settled, err := schedules.GetSchedule(context.Background(), schedule.ID)
			if err != nil {
				completed <- err
				return
			}
			select {
			case err := <-eventDelivered:
				if err != nil {
					completed <- err
					return
				}
			default:
				completed <- fmt.Errorf("replacement complete ran before event delivery")
				return
			}
			if sessionID != fx.sessionID || settled.Status != "completed" {
				completed <- fmt.Errorf("replacement complete ran before schedule settlement: session=%s schedule=%s", sessionID, settled.Status)
				return
			}
			completed <- nil
		}}

		settleReplacedSessions(ctx, []store.ReplacedSession{{ID: fx.sessionID, ClearedFreezeToken: ptr("11111111-1111-4111-8111-111111111111")}}, schedules, broadcaster, control)
		select {
		case err := <-completed:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("replacement terminal completion did not run")
		}
	})
}

func TestEndSession_ControlFailuresAreDurablyDegradedAndTerminalized(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"timeout", context.DeadlineExceeded},
		{"transport", errors.New("connection reset by peer")},
		{"non-2xx", errors.New("control returned HTTP 503")},
		{"malformed bundle", errors.New("invalid canvas control bundle")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newSessionFixture(t, t.Name())
			completed := make(chan error, 1)
			fx.h.CanvasControl = &fakeCanvasControl{
				err: tc.err,
				onComplete: func(sessionID, _ string) {
					if sessionID != fx.sessionID {
						completed <- fmt.Errorf("completed unexpected session %q", sessionID)
						return
					}
					var status string
					if err := fx.db.QueryRowContext(context.Background(), `SELECT status FROM sessions WHERE id=$1`, sessionID).Scan(&status); err != nil {
						completed <- err
						return
					}
					if status != "ended" {
						completed <- fmt.Errorf("terminal cleanup observed status %q before durable end", status)
						return
					}
					completed <- nil
				},
			}
			w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/end", nil, fx.claims(fx.teacher, false))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, false, body["whiteboardServerArchiveComplete"])
			require.Equal(t, "whiteboard_server_archive_incomplete", body["warning"])
			select {
			case err := <-completed:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("durable degraded end did not asynchronously complete its matching control token")
			}
		})
	}
}

func TestEndSession_ResponseUsesDurableEndedAtAndTopLevelContract(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	completed := make(chan error, 1)
	fx.h.CanvasControl = &fakeCanvasControl{onComplete: func(sessionID, _ string) {
		var status string
		if err := fx.db.QueryRowContext(context.Background(), `SELECT status FROM sessions WHERE id=$1`, sessionID).Scan(&status); err != nil {
			completed <- err
			return
		}
		if status != "ended" {
			completed <- fmt.Errorf("terminal complete observed status %q", status)
			return
		}
		completed <- nil
	}}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/end", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		ID                              string     `json:"id"`
		Status                          string     `json:"status"`
		EndedAt                         *time.Time `json:"endedAt"`
		WhiteboardServerArchiveComplete bool       `json:"whiteboardServerArchiveComplete"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, fx.sessionID, response.ID)
	require.Equal(t, "ended", response.Status)
	require.NotNil(t, response.EndedAt)
	require.True(t, response.WhiteboardServerArchiveComplete, "an exact empty freeze bundle is a confirmed archive")
	var stored time.Time
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT ended_at FROM sessions WHERE id=$1`, fx.sessionID).Scan(&stored))
	require.True(t, response.EndedAt.Equal(stored), "response must use the durable transaction timestamp")
	select {
	case err := <-completed:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("terminal complete did not run asynchronously after the durable response handoff")
	}
}

func TestEndSession_LateFreezeResponseCannotReinstallLease(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	// A control response can race a replacement operation after this request
	// has already acquired its lease.  The stale token must become the stable
	// retryable conflict, not a generic database 500 that hides the live owner.
	fx.h.CanvasControl = &fakeCanvasControl{onFreeze: func(request realtime.FreezeRequest) {
		_, err := fx.db.ExecContext(context.Background(), `
			UPDATE sessions
			SET canvas_freeze_token = '11111111-1111-4111-8111-111111111111',
			    canvas_freeze_until = clock_timestamp() + interval '15 seconds'
			WHERE id = $1`, request.SessionID)
		require.NoError(t, err)
	}}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/end", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	require.JSONEq(t, `{"error":"Session end in progress","code":"session_end_in_progress"}`, w.Body.String())

	var status string
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT status FROM sessions WHERE id = $1`, fx.sessionID).Scan(&status))
	require.Equal(t, "live", status, "a stale control token must not end a different live operation")
}

func TestSessionHandler_EndSession_NonTeacher403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/end", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ------------------- POST /api/sessions/{id}/participants -------------------

func TestSessionHandler_AddParticipant_TeacherAddsByUserId(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"userId": fx.student.ID}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var p store.SessionParticipant
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	assert.Equal(t, fx.student.ID, p.StudentID)
	assert.Equal(t, "invited", p.Status)
	assert.NotNil(t, p.InvitedBy)
	assert.Equal(t, fx.teacher.ID, *p.InvitedBy)
}

func TestSessionHandler_AddParticipant_TeacherAddsByEmail(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"email": fx.student.Email}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var p store.SessionParticipant
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	assert.Equal(t, fx.student.ID, p.StudentID)
}

func TestSessionHandler_AddParticipant_UnknownEmail404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"email": "nonexistent-user@example.com"}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSessionHandler_AddParticipant_NonTeacher403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"userId": fx.otherUser.ID}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSessionHandler_AddParticipant_PlatformAdmin(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"userId": fx.student.ID}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.otherUser, true))
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestSessionHandler_AddParticipant_Unauthenticated401(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"userId": fx.student.ID}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSessionHandler_AddParticipant_Idempotent(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"userId": fx.student.ID}

	w1 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusCreated, w1.Code)

	// Adding the same user again should also succeed (idempotent).
	w2 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusCreated, w2.Code, "second add should be idempotent")
}

// ------------------- DELETE /api/sessions/{id}/participants/{userId} -------------------

func TestSessionHandler_RemoveParticipant_Teacher(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Add a participant first
	_, err := fx.h.Sessions.AddParticipant(ctx, fx.sessionID, fx.student.ID, fx.teacher.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/participants/"+fx.student.ID, nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestSessionHandler_RemoveParticipant_VerifyAccessRevoked(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Add student as a participant (direct add, not class member)
	_, err := fx.h.Sessions.AddParticipant(ctx, fx.sessionID, fx.otherUser.ID, fx.teacher.ID)
	require.NoError(t, err)

	// Verify access before removal
	allowed, _, err := fx.h.Sessions.CanAccessSession(ctx, fx.sessionID, fx.otherUser.ID)
	require.NoError(t, err)
	assert.True(t, allowed, "participant should have access before removal")

	// Remove the participant
	w := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/participants/"+fx.otherUser.ID, nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusNoContent, w.Code)

	// Verify access is revoked
	allowed, _, err = fx.h.Sessions.CanAccessSession(ctx, fx.sessionID, fx.otherUser.ID)
	require.NoError(t, err)
	assert.False(t, allowed, "participant should lose access after removal")
}

func TestSessionHandler_RemoveParticipant_NonTeacher403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/participants/"+fx.student.ID, nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSessionHandler_RemoveParticipant_NotFound404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	// Try to remove a non-existent participant
	w := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/participants/"+fx.otherUser.ID, nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ------------------- GET /api/sessions/{id} (tightened access) -------------------

func TestSessionHandler_GetSession_AccessTeacher(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID, nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetSession_AccessClassMember(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Add student to the class
	_, err := fx.classes.AddClassMember(ctx, store.AddClassMemberInput{
		ClassID: fx.classID, UserID: fx.student.ID, Role: "student",
	})
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID, nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetSession_AccessTokenJoinedParticipant(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Add otherUser as a participant (simulating token join)
	_, err := fx.h.Sessions.AddParticipant(ctx, fx.sessionID, fx.otherUser.ID, fx.teacher.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID, nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetSession_AccessRandomUser404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	// otherUser is not teacher, not a class member, not a participant
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID, nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusNotFound, w.Code, "random user should get 404 (not leak existence)")
}

func TestSessionHandler_GetSession_AccessPlatformAdmin(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID, nil, fx.claims(fx.otherUser, true))
	assert.Equal(t, http.StatusOK, w.Code)
}

// ------------------- GET /api/sessions/{id}/participants (tightened roster access) -------------------

func TestSessionHandler_GetParticipants_AccessTeacher(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/participants", nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetParticipants_AccessRegularParticipant403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Add student as a participant
	_, err := fx.h.Sessions.AddParticipant(ctx, fx.sessionID, fx.student.ID, fx.teacher.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/participants", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code, "regular participant should not read the roster")
}

func TestSessionHandler_GetParticipants_AccessPlatformAdmin(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/participants", nil, fx.claims(fx.otherUser, true))
	assert.Equal(t, http.StatusOK, w.Code)
}

// ------------------- GET /api/sessions/{id}/student-page (plan 090 public admission) -------------------

// Headline plan-090 regression guard: a class-less, live, public session must
// admit a first-time browser — not the host, not a class member, no participant
// row — so the room dispatcher can reach the /join POST. Before the public-
// admission clause in GetStudentPage this returned 403 and browse->join 404'd.
// This test would FAIL without that clause.
func TestSessionHandler_GetStudentPage_ClassLessPublicNonParticipant200(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	session := fx.createSession(t, store.CreateSessionInput{
		TeacherID:  fx.teacher.ID,
		Title:      "Public open room",
		Visibility: "public",
	})

	// otherUser is not the host, not a class member, and has no participant row.
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+session.ID+"/student-page", nil, fx.claims(fx.otherUser, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var payload studentPagePayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, session.ID, payload.Session.ID)
	assert.Nil(t, payload.ClassID)
	assert.Equal(t, "/student", payload.ReturnPath)
}

// The public-admission clause is visibility-gated, not merely class-gated: an
// UNLISTED class-less session must still 403 a non-participant. Without the
// visibility check the clause would open every ad-hoc session to any
// authenticated user.
func TestSessionHandler_GetStudentPage_ClassLessUnlistedNonParticipant403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	session := fx.createSession(t, store.CreateSessionInput{
		TeacherID:  fx.teacher.ID,
		Title:      "Unlisted closed room",
		Visibility: "unlisted",
	})

	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+session.ID+"/student-page", nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
}

// ------------------- PATCH visibility guard (cross-org leak fix) -------------------

// A class-bound session must never be made public: that would surface it in the
// global browse list and let any authenticated user across orgs join it. The
// PatchSession guard returns 400 and leaves visibility unchanged.
func TestSessionHandler_PatchSession_ClassBoundCannotBePublic400(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	// fx.sessionID is the fixture's class-bound session.
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, map[string]any{
		"visibility": "public",
	}, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())

	// The session must remain non-public — the guard runs before UpdateSession.
	got := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID, nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, got.Code, "body=%s", got.Body.String())
	var session store.LiveSession
	require.NoError(t, json.Unmarshal(got.Body.Bytes(), &session))
	assert.Equal(t, "unlisted", session.Visibility, "class-bound session must not have been made public")
}

// ------------------- CanAccessSession / ListPublicSessions store-level guards -------------------

// Direct store test for the cross-org leak fix: a class-bound session that
// carries visibility='public' (constructed at the store layer, bypassing the
// PatchSession guard) must NOT grant "public" access to a non-member. Only class
// membership opens a class-bound session.
func TestSessionStore_CanAccessSession_ClassBoundPublicDeniesNonMember(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()
	classBoundPublic := fx.createSession(t, store.CreateSessionInput{
		ClassID:    strPtr(fx.classID),
		TeacherID:  fx.teacher.ID,
		Title:      "Class-bound but public",
		Visibility: "public",
	})

	allowed, reason, err := fx.h.Sessions.CanAccessSession(ctx, classBoundPublic.ID, fx.otherUser.ID)
	require.NoError(t, err)
	assert.False(t, allowed, "class-bound public session must not admit a non-member via the public clause")
	assert.Equal(t, "no_access", reason)
}

// ListPublicSessions must exclude class-bound sessions even when one carries
// visibility='public' (constructed at the store layer). Defense in depth: the
// PatchSession guard stops the API from ever setting this, but the browse query
// must independently refuse to surface it.
func TestSessionStore_ListPublicSessions_ExcludesClassBound(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	classBoundPublic := fx.createSession(t, store.CreateSessionInput{
		ClassID:    strPtr(fx.classID),
		TeacherID:  fx.teacher.ID,
		Title:      "Class-bound public (leaky)",
		Visibility: "public",
	})
	classLessPublic := fx.createSession(t, store.CreateSessionInput{
		TeacherID:  fx.teacher.ID,
		Title:      "Class-less public (browseable)",
		Visibility: "public",
	})

	items, err := fx.h.Sessions.ListPublicSessions(ctx, 100, nil, nil)
	require.NoError(t, err)
	gotIDs := make([]string, 0, len(items))
	for _, item := range items {
		gotIDs = append(gotIDs, item.ID)
	}
	assert.Contains(t, gotIDs, classLessPublic.ID, "class-less public session should be browseable")
	assert.NotContains(t, gotIDs, classBoundPublic.ID, "class-bound session must never surface in the public browse list")
}

// ------------------- GET /api/sessions/public (browse endpoint guards) -------------------

func TestSessionHandler_ListPublicSessions_Unauthenticated401(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/public", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
}

func TestSessionHandler_ListPublicSessions_MalformedCursor400(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	// %40%40%40 decodes to "@@@", which is not valid base64url -> decodeCursor
	// errors -> handler maps to 400.
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/public?cursor=%40%40%40", nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
}
