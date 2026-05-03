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
	"github.com/weiboz0/bridge/platform/internal/store"
)

const rtSecret = "phase1-realtime-test-secret"

// --- mint endpoint -----------------------------------------------------------
//
// Plan 053 phase 1: POST /api/realtime/token gates per documentName.
// Tests use the existing sessionPageFixture (from
// sessions_page_integration_test.go) which wires teacher / student /
// outsider / admin / orgAdmin + an org / class / session.

func newRealtimeHandlerForFixture(fx *sessionPageFixture) *RealtimeHandler {
	return &RealtimeHandler{
		Sessions:              store.NewSessionStore(fx.db),
		Classes:               store.NewClassStore(fx.db),
		Orgs:                  store.NewOrgStore(fx.db),
		TeachingUnits:         store.NewTeachingUnitStore(fx.db),
		Problems:              store.NewProblemStore(fx.db),
		Attempts:              store.NewAttemptStore(fx.db),
		Users:                 store.NewUserStore(fx.db),
		ParentLinks:           store.NewParentLinkStore(fx.db), // Plan 053b Phase 4.
		HocuspocusTokenSecret: rtSecret,
	}
}

func callMintToken(t *testing.T, h *RealtimeHandler, docName string, claims *auth.Claims) (int, mintResponse) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"documentName": docName})
	req := httptest.NewRequest(http.MethodPost, "/api/realtime/token", bytes.NewReader(body))
	req = withClaims(req, claims)
	w := httptest.NewRecorder()
	h.MintToken(w, req)
	if w.Code != http.StatusOK {
		return w.Code, mintResponse{}
	}
	var resp mintResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func TestMintToken_NoClaims(t *testing.T) {
	h := &RealtimeHandler{HocuspocusTokenSecret: rtSecret}
	body, _ := json.Marshal(map[string]string{"documentName": "unit:x"})
	req := httptest.NewRequest(http.MethodPost, "/api/realtime/token", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.MintToken(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMintToken_NoSecret_503(t *testing.T) {
	h := &RealtimeHandler{HocuspocusTokenSecret: ""} // unconfigured
	body, _ := json.Marshal(map[string]string{"documentName": "unit:x"})
	req := httptest.NewRequest(http.MethodPost, "/api/realtime/token", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "u-1"})
	w := httptest.NewRecorder()
	h.MintToken(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestMintToken_MissingDocumentName(t *testing.T) {
	h := &RealtimeHandler{HocuspocusTokenSecret: rtSecret}
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/api/realtime/token", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "u-1"})
	w := httptest.NewRecorder()
	h.MintToken(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMintToken_BadDocumentNameShape(t *testing.T) {
	cases := []struct {
		name    string
		docName string
	}{
		{"unknown scope", "garbage:nope"},
		{"too few parts", "session"},
		{"session wrong shape", "session:abc:teacher:def"},
		{"broadcast trailing", "broadcast:abc:extra"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &RealtimeHandler{HocuspocusTokenSecret: rtSecret}
			body, _ := json.Marshal(map[string]string{"documentName": tc.docName})
			req := httptest.NewRequest(http.MethodPost, "/api/realtime/token", bytes.NewReader(body))
			req = withClaims(req, &auth.Claims{UserID: "u-1", IsPlatformAdmin: true})
			w := httptest.NewRecorder()
			h.MintToken(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code, "docName=%q", tc.docName)
		})
	}
}

// --- per-doc-name authorization (integration; uses the live fixture)

func TestMintToken_SessionDoc_StudentOpensOwn_OK(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-sess-own")
	h := newRealtimeHandlerForFixture(fx)
	docName := "session:" + fx.sessionID + ":user:" + fx.student.ID
	code, resp := callMintToken(t, h, docName, &auth.Claims{UserID: fx.student.ID})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Token)
	// Verify the token's claims are correct.
	claims, err := auth.VerifyRealtimeToken(rtSecret, resp.Token)
	require.NoError(t, err)
	assert.Equal(t, fx.student.ID, claims.Sub)
	assert.Equal(t, "user", claims.Role)
	assert.Equal(t, docName, claims.Scope)
}

func TestMintToken_SessionDoc_StudentOpensOther_403(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-sess-other")
	h := newRealtimeHandlerForFixture(fx)
	// student tries to open the OUTSIDER's session doc.
	docName := "session:" + fx.sessionID + ":user:" + fx.outsider.ID
	code, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.student.ID})
	assert.Equal(t, http.StatusForbidden, code)
}

func TestMintToken_SessionDoc_TeacherOpensAny_OK(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-sess-teacher")
	h := newRealtimeHandlerForFixture(fx)
	// teacher opens the student's doc.
	docName := "session:" + fx.sessionID + ":user:" + fx.student.ID
	code, resp := callMintToken(t, h, docName, &auth.Claims{UserID: fx.teacher.ID})
	require.Equal(t, http.StatusOK, code)
	claims, err := auth.VerifyRealtimeToken(rtSecret, resp.Token)
	require.NoError(t, err)
	assert.Equal(t, "teacher", claims.Role)
}

func TestMintToken_SessionDoc_OutsiderDenied(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-sess-outsider")
	h := newRealtimeHandlerForFixture(fx)
	docName := "session:" + fx.sessionID + ":user:" + fx.outsider.ID
	code, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.outsider.ID})
	assert.Equal(t, http.StatusForbidden, code)
}

// Broadcast docs are one-way (teacher writes, class reads). Two
// distinct roles in the JWT: "teacher" for the broadcaster, "user"
// for viewers. Both must be allowed to mint.
func TestMintToken_BroadcastDoc_TeacherWrites_StudentReads(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-bc")
	h := newRealtimeHandlerForFixture(fx)
	docName := "broadcast:" + fx.sessionID

	// Teacher → role=teacher.
	codeT, respT := callMintToken(t, h, docName, &auth.Claims{UserID: fx.teacher.ID})
	require.Equal(t, http.StatusOK, codeT)
	teacherClaims, err := auth.VerifyRealtimeToken(rtSecret, respT.Token)
	require.NoError(t, err)
	assert.Equal(t, "teacher", teacherClaims.Role)

	// Student (class member) → role=user. Phase-1 narrow rule was
	// "teacher only", which broke the legacy student-reads-broadcast
	// case. Phase 3 broadens to class-member-can-read.
	codeS, respS := callMintToken(t, h, docName, &auth.Claims{UserID: fx.student.ID})
	require.Equal(t, http.StatusOK, codeS)
	studentClaims, err := auth.VerifyRealtimeToken(rtSecret, respS.Token)
	require.NoError(t, err)
	assert.Equal(t, "user", studentClaims.Role)

	// Outsider (no membership) → still 403.
	codeO, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.outsider.ID})
	assert.Equal(t, http.StatusForbidden, codeO)
}

// org_admin without class membership: still allowed because they
// have class-read authority via org-admin status. (Different from
// pre-Phase-3 behavior where org_admin was deliberately denied to
// mirror the REST start/stop gate.)
func TestMintToken_BroadcastDoc_OrgAdmin_GetsReadRole(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-bc-oa")
	h := newRealtimeHandlerForFixture(fx)
	docName := "broadcast:" + fx.sessionID

	code, resp := callMintToken(t, h, docName, &auth.Claims{UserID: fx.orgAdmin.ID})
	require.Equal(t, http.StatusOK, code, "org_admin has class-read authority via org role")
	claims, err := auth.VerifyRealtimeToken(rtSecret, resp.Token)
	require.NoError(t, err)
	assert.Equal(t, "user", claims.Role, "org_admin gets reader role, NOT writer — start/stop is REST-gate only")

	// Platform admin → writer.
	codeA, respA := callMintToken(t, h, docName, &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true})
	require.Equal(t, http.StatusOK, codeA)
	adminClaims, err := auth.VerifyRealtimeToken(rtSecret, respA.Token)
	require.NoError(t, err)
	assert.Equal(t, "teacher", adminClaims.Role)
}

// Plan 053b broadened the Phase 1 owner-only rule. Attempt-doc mint
// now accepts: owner, platform admin, impersonator, class instructor/
// TA where attempt-owner is in the SAME class, and org_admin where
// attempt-owner is in some class for the relevant course. The new
// store helpers IsTeacherOfAttempt + IsOrgAdminOfAttempt enforce
// the popular-problem-leak constraint (both teacher and attempt-
// owner must share a class).
func TestMintToken_AttemptDoc_OwnerOnly_NoCourseBinding(t *testing.T) {
	// Personal-scope problem with no topic_problems row → no class
	// can possibly contain it. Owner gets 200; everyone else 403,
	// EXCEPT platform admin and impersonator (admin bypass).
	fx := newSessionPageFixture(t, "rt-att-bare")
	h := newRealtimeHandlerForFixture(fx)

	problems := store.NewProblemStore(fx.db)
	p, err := problems.CreateProblem(t.Context(), store.CreateProblemInput{
		Scope:       "personal",
		ScopeID:     &fx.student.ID,
		Title:       "Bare attempt problem",
		Description: "x",
		StarterCode: map[string]string{"python": ""},
		Difficulty:  "easy",
		Status:      "draft",
		CreatedBy:   fx.student.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(context.Background(), "DELETE FROM problems WHERE id = $1", p.ID)
	})

	attempts := store.NewAttemptStore(fx.db)
	a, err := attempts.CreateAttempt(t.Context(), store.CreateAttemptInput{
		ProblemID: p.ID, UserID: fx.student.ID, Title: "A1", Language: "python", PlainText: "",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(context.Background(), "DELETE FROM attempts WHERE id = $1", a.ID)
	})

	docName := "attempt:" + a.ID

	codeO, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.student.ID})
	assert.Equal(t, http.StatusOK, codeO, "owner")

	codeT, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.teacher.ID})
	assert.Equal(t, http.StatusForbidden, codeT, "teacher with no class binding")

	codeA, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true})
	assert.Equal(t, http.StatusOK, codeA, "platform admin (053b lifts the Phase-1 stricture)")

	codeImp, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.outsider.ID, ImpersonatedBy: fx.admin.ID})
	assert.Equal(t, http.StatusOK, codeImp, "impersonator (admin-driven)")
}

// Class-staff path: teacher + attempt-owner share a class for the
// problem's topic. Both constraints must hold.
func TestMintToken_AttemptDoc_TeacherOfAttemptOwnerInSameClass_OK(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-att-shared")
	h := newRealtimeHandlerForFixture(fx)
	ctx := context.Background()

	// Seed: an org problem linked to a topic of a course; the
	// fixture's class is for that course; both teacher and student
	// are class members. Then create an attempt by the student.
	courses := store.NewCourseStore(fx.db)
	topics := store.NewTopicStore(fx.db)
	classes := store.NewClassStore(fx.db)
	problems := store.NewProblemStore(fx.db)
	topicProblems := store.NewTopicProblemStore(fx.db)
	attempts := store.NewAttemptStore(fx.db)

	course, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID: fx.orgID, CreatedBy: fx.teacher.ID, Title: "Plan053b Course", GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { fx.db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	topic, err := topics.CreateTopic(ctx, store.CreateTopicInput{CourseID: course.ID, Title: "T"})
	require.NoError(t, err)
	t.Cleanup(func() { fx.db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic.ID) })

	cls, err := classes.CreateClass(ctx, store.CreateClassInput{
		CourseID: course.ID, OrgID: fx.orgID, Title: "Plan053b Class", Term: "fall", CreatedBy: fx.teacher.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id = $1", cls.ID)
		fx.db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", cls.ID)
		fx.db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", cls.ID)
	})
	_, err = classes.AddClassMember(ctx, store.AddClassMemberInput{ClassID: cls.ID, UserID: fx.teacher.ID, Role: "instructor"})
	require.NoError(t, err)
	_, err = classes.AddClassMember(ctx, store.AddClassMemberInput{ClassID: cls.ID, UserID: fx.student.ID, Role: "student"})
	require.NoError(t, err)

	p, err := problems.CreateProblem(ctx, store.CreateProblemInput{
		Scope: "org", ScopeID: &fx.orgID, Title: "Shared Problem", Description: "x",
		StarterCode: map[string]string{"python": ""}, Difficulty: "easy", Status: "draft", CreatedBy: fx.teacher.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() { fx.db.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", p.ID) })
	_, err = topicProblems.Attach(ctx, topic.ID, p.ID, 0, fx.teacher.ID)
	require.NoError(t, err)

	a, err := attempts.CreateAttempt(ctx, store.CreateAttemptInput{
		ProblemID: p.ID, UserID: fx.student.ID, Title: "A1", Language: "python", PlainText: "",
	})
	require.NoError(t, err)
	t.Cleanup(func() { fx.db.ExecContext(ctx, "DELETE FROM attempts WHERE id = $1", a.ID) })

	docName := "attempt:" + a.ID

	// Teacher (instructor in the class containing the student) → OK.
	codeT, respT := callMintToken(t, h, docName, &auth.Claims{UserID: fx.teacher.ID})
	require.Equal(t, http.StatusOK, codeT)
	teacherClaims, err := auth.VerifyRealtimeToken(rtSecret, respT.Token)
	require.NoError(t, err)
	assert.Equal(t, "teacher", teacherClaims.Role)

	// org_admin via the org membership path → OK (attempt owner is
	// in some class for that course).
	codeOrg, respOrg := callMintToken(t, h, docName, &auth.Claims{UserID: fx.orgAdmin.ID})
	require.Equal(t, http.StatusOK, codeOrg)
	orgClaims, err := auth.VerifyRealtimeToken(rtSecret, respOrg.Token)
	require.NoError(t, err)
	assert.Equal(t, "teacher", orgClaims.Role)

	// Outsider (not in the class, not in the org) → 403.
	codeOut, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.outsider.ID})
	assert.Equal(t, http.StatusForbidden, codeOut)
}

// Popular-problem leak guard: a teacher of OTHER classes for the
// SAME problem must NOT mint tokens for an attempt of a student in
// a DIFFERENT class. Codex caught this at Phase 0.
func TestMintToken_AttemptDoc_PopularProblemLeak_403(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-att-leak")
	h := newRealtimeHandlerForFixture(fx)
	ctx := context.Background()

	courses := store.NewCourseStore(fx.db)
	topics := store.NewTopicStore(fx.db)
	classes := store.NewClassStore(fx.db)
	problems := store.NewProblemStore(fx.db)
	topicProblems := store.NewTopicProblemStore(fx.db)
	attempts := store.NewAttemptStore(fx.db)
	users := store.NewUserStore(fx.db)

	// Course A — student's class. Course B — outsider's class. Same
	// problem linked to topics in BOTH courses.
	courseA, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID: fx.orgID, CreatedBy: fx.teacher.ID, Title: "Course A", GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	courseB, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID: fx.orgID, CreatedBy: fx.teacher.ID, Title: "Course B", GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM courses WHERE id IN ($1, $2)", courseA.ID, courseB.ID)
	})

	topicA, err := topics.CreateTopic(ctx, store.CreateTopicInput{CourseID: courseA.ID, Title: "TA"})
	require.NoError(t, err)
	topicB, err := topics.CreateTopic(ctx, store.CreateTopicInput{CourseID: courseB.ID, Title: "TB"})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM topics WHERE id IN ($1, $2)", topicA.ID, topicB.ID)
	})

	classA, err := classes.CreateClass(ctx, store.CreateClassInput{
		CourseID: courseA.ID, OrgID: fx.orgID, Title: "Class A", Term: "fall", CreatedBy: fx.teacher.ID,
	})
	require.NoError(t, err)
	classB, err := classes.CreateClass(ctx, store.CreateClassInput{
		CourseID: courseB.ID, OrgID: fx.orgID, Title: "Class B", Term: "fall", CreatedBy: fx.teacher.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id IN ($1, $2)", classA.ID, classB.ID)
		fx.db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id IN ($1, $2)", classA.ID, classB.ID)
		fx.db.ExecContext(ctx, "DELETE FROM classes WHERE id IN ($1, $2)", classA.ID, classB.ID)
	})

	// Student → Class A (Course A). Outsider → instructor of Class B
	// (Course B). Both classes have the same problem.
	_, err = classes.AddClassMember(ctx, store.AddClassMemberInput{ClassID: classA.ID, UserID: fx.student.ID, Role: "student"})
	require.NoError(t, err)
	// outsider must be an org member to be a class instructor.
	orgs := store.NewOrgStore(fx.db)
	_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{OrgID: fx.orgID, UserID: fx.outsider.ID, Role: "teacher", Status: "active"})
	require.NoError(t, err)
	_, err = classes.AddClassMember(ctx, store.AddClassMemberInput{ClassID: classB.ID, UserID: fx.outsider.ID, Role: "instructor"})
	require.NoError(t, err)
	_ = users // keep linter happy if not otherwise referenced

	p, err := problems.CreateProblem(ctx, store.CreateProblemInput{
		Scope: "platform", Title: "Popular", Description: "x",
		StarterCode: map[string]string{"python": ""}, Difficulty: "easy", Status: "classroom_ready", CreatedBy: fx.admin.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() { fx.db.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", p.ID) })

	_, err = topicProblems.Attach(ctx, topicA.ID, p.ID, 0, fx.teacher.ID)
	require.NoError(t, err)
	_, err = topicProblems.Attach(ctx, topicB.ID, p.ID, 0, fx.teacher.ID)
	require.NoError(t, err)

	a, err := attempts.CreateAttempt(ctx, store.CreateAttemptInput{
		ProblemID: p.ID, UserID: fx.student.ID, Title: "A", Language: "python", PlainText: "",
	})
	require.NoError(t, err)
	t.Cleanup(func() { fx.db.ExecContext(ctx, "DELETE FROM attempts WHERE id = $1", a.ID) })

	// Outsider is instructor of Class B but the student is in Class A.
	// The popular-problem-leak check rejects: 403.
	docName := "attempt:" + a.ID
	code, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.outsider.ID})
	assert.Equal(t, http.StatusForbidden, code,
		"teacher of OTHER class for same problem must NOT get tokens for student in DIFFERENT class")
}

// Plan 053b Phase 4 — parent of the doc-owning student gets a
// `parent` role token IF the child is a participant in the session.
func TestMintToken_SessionDoc_LinkedParent_OK(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-sess-parent")
	h := newRealtimeHandlerForFixture(fx)
	ctx := context.Background()

	// Use outsider as the "parent": active parent_link from outsider
	// → student, and student is a session participant.
	parent := fx.outsider
	links := store.NewParentLinkStore(fx.db)
	_, err := links.CreateLink(ctx, parent.ID, fx.student.ID, fx.admin.ID)
	require.NoError(t, err)
	_, err = fx.h.Sessions.JoinSession(ctx, fx.sessionID, fx.student.ID)
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM parent_links WHERE parent_user_id = $1 OR child_user_id = $1", parent.ID)
		fx.db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1", fx.sessionID)
	})

	// Set RealtimeHandler.ParentLinks on the test handler.
	h.ParentLinks = links

	docName := "session:" + fx.sessionID + ":user:" + fx.student.ID
	code, resp := callMintToken(t, h, docName, &auth.Claims{UserID: parent.ID})
	require.Equal(t, http.StatusOK, code, "linked parent must mint a parent-role token for the child's session doc")
	claims, err := auth.VerifyRealtimeToken(rtSecret, resp.Token)
	require.NoError(t, err)
	assert.Equal(t, "parent", claims.Role)
}

// Codex post-impl pass-1 catch: GetSessionParticipant returns rows
// with status='left' too. A parent must NOT keep access after the
// child has left the session — only invited/present grants the
// parent token.
func TestMintToken_SessionDoc_ParentOfChildWhoLeft_403(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-sess-parent-left")
	h := newRealtimeHandlerForFixture(fx)
	ctx := context.Background()

	parent := fx.outsider
	links := store.NewParentLinkStore(fx.db)
	_, err := links.CreateLink(ctx, parent.ID, fx.student.ID, fx.admin.ID)
	require.NoError(t, err)

	// Child joined, then left.
	_, err = fx.h.Sessions.JoinSession(ctx, fx.sessionID, fx.student.ID)
	require.NoError(t, err)
	_, err = fx.h.Sessions.UpdateParticipantStatus(ctx, fx.sessionID, fx.student.ID, "left")
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM parent_links WHERE parent_user_id = $1 OR child_user_id = $1", parent.ID)
		fx.db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1", fx.sessionID)
	})

	h.ParentLinks = links

	docName := "session:" + fx.sessionID + ":user:" + fx.student.ID
	code, _ := callMintToken(t, h, docName, &auth.Claims{UserID: parent.ID})
	assert.Equal(t, http.StatusForbidden, code,
		"parent must NOT mint tokens after the child has LEFT the session")
}

// Privacy guard: a parent of one child must NOT mint tokens for an
// unrelated child's session doc, even if the unrelated child is a
// participant in some session. Both checks (IsParentOf AND child-
// in-session) must hold.
func TestMintToken_SessionDoc_ParentOfDifferentChild_403(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-sess-other-parent")
	h := newRealtimeHandlerForFixture(fx)
	ctx := context.Background()

	// Outsider is a parent of someone — but NOT of fx.student.
	parent := fx.outsider
	users := store.NewUserStore(fx.db)
	otherChild, err := users.RegisterUser(ctx, store.RegisterInput{
		Name: "Other Child", Email: "other-child-" + fx.sessionID[:8] + "@example.com", Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM parent_links WHERE child_user_id = $1", otherChild.ID)
		fx.db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", otherChild.ID)
		fx.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", otherChild.ID)
	})

	links := store.NewParentLinkStore(fx.db)
	_, err = links.CreateLink(ctx, parent.ID, otherChild.ID, fx.admin.ID)
	require.NoError(t, err)
	_, err = fx.h.Sessions.JoinSession(ctx, fx.sessionID, fx.student.ID)
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM parent_links WHERE parent_user_id = $1", parent.ID)
		fx.db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1", fx.sessionID)
	})

	h.ParentLinks = links

	// Try to open fx.student's doc — parent has no parent_link to
	// fx.student, so the IsParentOf check fails.
	docName := "session:" + fx.sessionID + ":user:" + fx.student.ID
	code, _ := callMintToken(t, h, docName, &auth.Claims{UserID: parent.ID})
	assert.Equal(t, http.StatusForbidden, code,
		"parent of one child must NOT mint tokens for another child's session doc")
}

func TestMintToken_UnitDoc_OrgTeacherOK_StudentDenied(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-unit")
	h := newRealtimeHandlerForFixture(fx)

	// Seed an org-scope unit in the fixture's org.
	units := store.NewTeachingUnitStore(fx.db)
	u, err := units.CreateUnit(t.Context(), store.CreateTeachingUnitInput{
		Scope:     "org",
		ScopeID:   &fx.orgID,
		Title:     "Realtime Unit",
		Status:    "draft",
		CreatedBy: fx.teacher.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(context.Background(), "DELETE FROM teaching_units WHERE id = $1", u.ID)
	})

	docName := "unit:" + u.ID

	// teacher (org member with role=teacher) OK
	codeT, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.teacher.ID})
	assert.Equal(t, http.StatusOK, codeT)

	// student (org member but role=student) denied
	codeS, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.student.ID})
	assert.Equal(t, http.StatusForbidden, codeS)

	// platform admin OK
	codeA, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true})
	assert.Equal(t, http.StatusOK, codeA)
}

// --- internal auth endpoint --------------------------------------------------

func callInternalAuth(t *testing.T, h *RealtimeHandler, secretHeader, docName, sub string) (int, internalAuthResponse) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"documentName": docName, "sub": sub})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/realtime/auth", bytes.NewReader(body))
	if secretHeader != "" {
		req.Header.Set("Authorization", "Bearer "+secretHeader)
	}
	w := httptest.NewRecorder()
	h.InternalAuth(w, req)
	if w.Code != http.StatusOK {
		return w.Code, internalAuthResponse{}
	}
	var resp internalAuthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func TestInternalAuth_RejectsMissingBearer(t *testing.T) {
	h := &RealtimeHandler{HocuspocusTokenSecret: rtSecret}
	code, _ := callInternalAuth(t, h, "", "unit:x", "u-1")
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestInternalAuth_RejectsWrongBearer(t *testing.T) {
	h := &RealtimeHandler{HocuspocusTokenSecret: rtSecret}
	code, _ := callInternalAuth(t, h, "wrong-secret", "unit:x", "u-1")
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestInternalAuth_RejectsMissingFields(t *testing.T) {
	h := &RealtimeHandler{HocuspocusTokenSecret: rtSecret}
	body, _ := json.Marshal(map[string]string{"documentName": "unit:x"})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/realtime/auth", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+rtSecret)
	w := httptest.NewRecorder()
	h.InternalAuth(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestInternalAuth_AllowedAndDenied(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-int-auth")
	h := newRealtimeHandlerForFixture(fx)

	docName := "session:" + fx.sessionID + ":user:" + fx.student.ID

	// student opening own doc → allowed.
	code, resp := callInternalAuth(t, h, rtSecret, docName, fx.student.ID)
	require.Equal(t, http.StatusOK, code)
	assert.True(t, resp.Allowed, "student opening own session doc")

	// outsider → denied.
	code2, resp2 := callInternalAuth(t, h, rtSecret, docName, fx.outsider.ID)
	require.Equal(t, http.StatusOK, code2)
	assert.False(t, resp2.Allowed)
	assert.NotEmpty(t, resp2.Reason)
}

// The internal endpoint runs OUTSIDE the user-auth middleware, so it
// must rebuild claims from the DB rather than trust the JWT's sub
// alone. Specifically `IsPlatformAdmin` must be re-read from
// `users.is_platform_admin` — otherwise an admin token minted at
// `t-1` (when the user was admin) would still pass the recheck at
// `t+10min` after the user was demoted.
func TestInternalAuth_RehydratesPlatformAdminFromDB(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-int-admin")
	h := newRealtimeHandlerForFixture(fx)

	// Seed an org-scope unit in the fixture's org; the outsider is
	// NOT an org member and would otherwise fail authorizeUnitDoc.
	units := store.NewTeachingUnitStore(fx.db)
	u, err := units.CreateUnit(t.Context(), store.CreateTeachingUnitInput{
		Scope:     "org",
		ScopeID:   &fx.orgID,
		Title:     "Internal Auth Unit",
		Status:    "draft",
		CreatedBy: fx.teacher.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(context.Background(), "DELETE FROM teaching_units WHERE id = $1", u.ID)
	})
	docName := "unit:" + u.ID

	// Without DB admin: outsider is denied.
	code, resp := callInternalAuth(t, h, rtSecret, docName, fx.outsider.ID)
	require.Equal(t, http.StatusOK, code)
	assert.False(t, resp.Allowed, "outsider without DB admin must be denied")

	// Promote outsider to platform admin in the DB.
	_, err = fx.db.ExecContext(context.Background(),
		"UPDATE users SET is_platform_admin = true WHERE id = $1", fx.outsider.ID)
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(context.Background(),
			"UPDATE users SET is_platform_admin = false WHERE id = $1", fx.outsider.ID)
	})

	// Now the same call should pass via DB-rehydrated admin status.
	code2, resp2 := callInternalAuth(t, h, rtSecret, docName, fx.outsider.ID)
	require.Equal(t, http.StatusOK, code2)
	assert.True(t, resp2.Allowed, "DB-rehydrated platform admin must pass")
}

// Internal endpoint distinguishes real authorization denials (200 +
// allowed:false) from infrastructure failures (4xx/5xx) so Hocuspocus
// retry logic and ops alerting don't conflate "deny" with "broken".
func TestInternalAuth_UnknownSub_404(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-int-unknown")
	h := newRealtimeHandlerForFixture(fx)
	docName := "session:" + fx.sessionID + ":user:" + fx.student.ID

	// uuid-shaped but never-registered sub → 404 (user missing).
	code, _ := callInternalAuth(t, h, rtSecret, docName, "00000000-0000-0000-0000-000000000000")
	assert.Equal(t, http.StatusNotFound, code)
}

// Malformed documentName → 400, not 200/Allowed:false. Hocuspocus
// shouldn't be told "this user can't access this doc" when the
// problem is really that the doc-name is garbage.
func TestInternalAuth_BadDocName_400(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-int-baddoc")
	h := newRealtimeHandlerForFixture(fx)

	cases := []string{
		"garbage:nope",
		"session",
		"session:abc:teacher:def",
		"broadcast:abc:extra",
	}
	for _, doc := range cases {
		t.Run(doc, func(t *testing.T) {
			code, _ := callInternalAuth(t, h, rtSecret, doc, fx.student.ID)
			assert.Equal(t, http.StatusBadRequest, code, "doc=%q should yield 400", doc)
		})
	}
}

// Missing target resource (e.g. session deleted between mint and
// recheck) → 404, not 200/Allowed:false.
func TestInternalAuth_MissingResource_404(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-int-missing")
	h := newRealtimeHandlerForFixture(fx)

	docName := "session:00000000-0000-0000-0000-000000000000:user:" + fx.student.ID
	code, _ := callInternalAuth(t, h, rtSecret, docName, fx.student.ID)
	assert.Equal(t, http.StatusNotFound, code)
}

// Misconfigured handler (Users store nil) → 500, not silent
// allowed:false. Note: this is a programming error, not a runtime
// state — but we want it to surface loudly if someone re-wires the
// handler without populating Users.
func TestInternalAuth_NilUsersStore_500(t *testing.T) {
	h := &RealtimeHandler{HocuspocusTokenSecret: rtSecret /* Users intentionally nil */}
	code, _ := callInternalAuth(t, h, rtSecret, "session:x:user:y", "u-1")
	assert.Equal(t, http.StatusInternalServerError, code)
}
