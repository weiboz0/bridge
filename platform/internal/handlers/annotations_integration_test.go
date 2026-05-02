package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 056 — annotation document access enforcement. Builds on the
// existing sessionPageFixture and exercises the authorization
// matrix at the handler level.

func newAnnotationHandlerForFixture(fx *sessionPageFixture) *AnnotationHandler {
	return &AnnotationHandler{
		Annotations: store.NewAnnotationStore(fx.db),
		Sessions:    store.NewSessionStore(fx.db),
		Classes:     store.NewClassStore(fx.db),
		Orgs:        store.NewOrgStore(fx.db),
	}
}

// docID returns the annotation documentId for the fixture's session
// + the given studentId.
func annotDocID(fx *sessionPageFixture, studentID string) string {
	return fmt.Sprintf("session:%s:user:%s", fx.sessionID, studentID)
}

// callCreate posts an annotation and returns the response code.
func callCreate(t *testing.T, h *AnnotationHandler, claims *auth.Claims, docID string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"documentId": docID,
		"lineStart":  "1",
		"lineEnd":    "5",
		"content":    "feedback",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/annotations", bytes.NewReader(body))
	if claims != nil {
		req = withClaims(req, claims)
	}
	w := httptest.NewRecorder()
	h.CreateAnnotation(w, req)
	return w.Code
}

// callList returns the response code for GET ?documentId=<docID>.
func callList(t *testing.T, h *AnnotationHandler, claims *auth.Claims, docID string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/annotations?documentId="+docID, nil)
	if claims != nil {
		req = withClaims(req, claims)
	}
	w := httptest.NewRecorder()
	h.ListAnnotations(w, req)
	return w.Code
}

// callDelete returns the response code for DELETE /api/annotations/{id}.
func callDelete(t *testing.T, h *AnnotationHandler, claims *auth.Claims, annotID string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/annotations/"+annotID, nil)
	req = withChiParams(req, map[string]string{"id": annotID})
	if claims != nil {
		req = withClaims(req, claims)
	}
	w := httptest.NewRecorder()
	h.DeleteAnnotation(w, req)
	return w.Code
}

// callResolve returns the response code for PATCH /api/annotations/{id}.
func callResolve(t *testing.T, h *AnnotationHandler, claims *auth.Claims, annotID string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/annotations/"+annotID, nil)
	req = withChiParams(req, map[string]string{"id": annotID})
	if claims != nil {
		req = withClaims(req, claims)
	}
	w := httptest.NewRecorder()
	h.ResolveAnnotation(w, req)
	return w.Code
}

// addAnnotTA creates a fresh user, makes them an active TA in the
// fixture's class, and returns their ID. Codex pass-1 flagged that
// the matrix never exercised the TA branch of RequireClassAuthority.
func addAnnotTA(t *testing.T, fx *sessionPageFixture, suffix string) string {
	t.Helper()
	users := store.NewUserStore(fx.db)
	classes := store.NewClassStore(fx.db)
	orgs := store.NewOrgStore(fx.db)
	ctx := context.Background()

	u, err := users.RegisterUser(ctx, store.RegisterInput{
		Name:     "TA " + suffix,
		Email:    "ta-" + suffix + "@example.com",
		Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM session_participants WHERE user_id = $1", u.ID)
		fx.db.ExecContext(ctx, "DELETE FROM class_memberships WHERE user_id = $1", u.ID)
		fx.db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1", u.ID)
		fx.db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
		fx.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
	})
	_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: fx.orgID, UserID: u.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)
	_, err = classes.AddClassMember(ctx, store.AddClassMemberInput{
		ClassID: fx.classID, UserID: u.ID, Role: "ta",
	})
	require.NoError(t, err)
	return u.ID
}

// addAnnotPeerStudent creates a second student-role user enrolled in
// the same class as fx.student. Used for the "another student in
// the same class looking at someone else's doc" denial — they have
// a class membership row but NO read access to fx.student's
// annotations because they're neither the doc owner nor class
// staff.
func addAnnotPeerStudent(t *testing.T, fx *sessionPageFixture, suffix string) string {
	t.Helper()
	users := store.NewUserStore(fx.db)
	classes := store.NewClassStore(fx.db)
	orgs := store.NewOrgStore(fx.db)
	ctx := context.Background()

	u, err := users.RegisterUser(ctx, store.RegisterInput{
		Name:     "Peer " + suffix,
		Email:    "peer-" + suffix + "@example.com",
		Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM session_participants WHERE user_id = $1", u.ID)
		fx.db.ExecContext(ctx, "DELETE FROM class_memberships WHERE user_id = $1", u.ID)
		fx.db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1", u.ID)
		fx.db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
		fx.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
	})
	_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: fx.orgID, UserID: u.ID, Role: "student", Status: "active",
	})
	require.NoError(t, err)
	_, err = classes.AddClassMember(ctx, store.AddClassMemberInput{
		ClassID: fx.classID, UserID: u.ID, Role: "student",
	})
	require.NoError(t, err)
	return u.ID
}

// seedAnnotation creates one teacher-authored annotation on the
// fixture's student doc; returns the annotation row for use as the
// target of delete/resolve tests.
func seedAnnotation(t *testing.T, h *AnnotationHandler, fx *sessionPageFixture) *store.Annotation {
	t.Helper()
	annot, err := h.Annotations.CreateAnnotation(context.Background(), store.CreateAnnotationInput{
		DocumentID: annotDocID(fx, fx.student.ID),
		AuthorID:   fx.teacher.ID,
		AuthorType: "teacher",
		LineStart:  "1",
		LineEnd:    "5",
		Content:    "test",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(context.Background(), "DELETE FROM code_annotations WHERE id = $1", annot.ID)
	})
	return annot
}

// --- Document-id shape rejection ---

func TestAnnotationAuth_RejectsNonSessionPrefix(t *testing.T) {
	fx := newSessionPageFixture(t, "anno-prefix")
	h := newAnnotationHandlerForFixture(fx)

	cases := []string{
		"attempt:abc-123",
		"unit:abc-123",
		"broadcast:abc-123",
		"d1",
		"session:abc",             // wrong shape (only 2 parts)
		"session:abc:teacher:def", // wrong middle (must be "user")
		"session::user:abc",       // empty session id
		"session:abc:user:",       // empty owner id
	}
	for _, doc := range cases {
		t.Run(doc, func(t *testing.T) {
			code := callCreate(t, h, &auth.Claims{UserID: fx.teacher.ID}, doc)
			assert.Equal(t, http.StatusBadRequest, code, "doc=%q", doc)
		})
	}
}

// --- Cross-user matrix on List + Create ---

func TestAnnotationAuth_ListMatrix(t *testing.T) {
	fx := newSessionPageFixture(t, "anno-list")
	h := newAnnotationHandlerForFixture(fx)
	taID := addAnnotTA(t, fx, "list")
	peerID := addAnnotPeerStudent(t, fx, "list")
	docID := annotDocID(fx, fx.student.ID)

	cases := []struct {
		name   string
		claims *auth.Claims
		want   int
	}{
		{"teacher of session", &auth.Claims{UserID: fx.teacher.ID}, http.StatusOK},
		{"platform admin", &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true}, http.StatusOK},
		{"org_admin (class staff via org role)", &auth.Claims{UserID: fx.orgAdmin.ID}, http.StatusOK},
		{"class TA", &auth.Claims{UserID: taID}, http.StatusOK},
		{"doc owner (student on own doc)", &auth.Claims{UserID: fx.student.ID}, http.StatusOK},
		{"peer student in same class (other student's doc)", &auth.Claims{UserID: peerID}, http.StatusNotFound},
		{"outsider (no membership)", &auth.Claims{UserID: fx.outsider.ID}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, callList(t, h, tc.claims, docID))
		})
	}
}

func TestAnnotationAuth_CreateMatrix(t *testing.T) {
	fx := newSessionPageFixture(t, "anno-create")
	h := newAnnotationHandlerForFixture(fx)
	taID := addAnnotTA(t, fx, "create")
	peerID := addAnnotPeerStudent(t, fx, "create")
	docID := annotDocID(fx, fx.student.ID)

	cases := []struct {
		name   string
		claims *auth.Claims
		want   int
	}{
		{"teacher of session", &auth.Claims{UserID: fx.teacher.ID}, http.StatusCreated},
		{"platform admin", &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true}, http.StatusCreated},
		{"org_admin (class staff via org role)", &auth.Claims{UserID: fx.orgAdmin.ID}, http.StatusCreated},
		{"class TA", &auth.Claims{UserID: taID}, http.StatusCreated},
		// Doc owner CAN read but CANNOT create — annotations are
		// teacher-only feedback. 403 (not 404) so the response
		// distinguishes "you have read access but no write" from
		// "no read access at all".
		{"doc owner (student on own doc)", &auth.Claims{UserID: fx.student.ID}, http.StatusForbidden},
		// Peer student is in the same class but isn't the doc
		// owner and isn't class staff — no read access → 404.
		{"peer student in same class (other student's doc)", &auth.Claims{UserID: peerID}, http.StatusNotFound},
		{"outsider (no membership)", &auth.Claims{UserID: fx.outsider.ID}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, callCreate(t, h, tc.claims, docID))
		})
	}
}

// --- Cross-user matrix on Delete + Resolve (annotation looked up first) ---

func TestAnnotationAuth_DeleteMatrix(t *testing.T) {
	fx := newSessionPageFixture(t, "anno-del")
	h := newAnnotationHandlerForFixture(fx)
	taID := addAnnotTA(t, fx, "del")
	peerID := addAnnotPeerStudent(t, fx, "del")

	cases := []struct {
		name   string
		claims *auth.Claims
		want   int
	}{
		{"teacher of session", &auth.Claims{UserID: fx.teacher.ID}, http.StatusOK},
		{"platform admin", &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true}, http.StatusOK},
		{"org_admin", &auth.Claims{UserID: fx.orgAdmin.ID}, http.StatusOK},
		{"class TA", &auth.Claims{UserID: taID}, http.StatusOK},
		{"doc owner (student on own doc)", &auth.Claims{UserID: fx.student.ID}, http.StatusForbidden},
		{"peer student in same class", &auth.Claims{UserID: peerID}, http.StatusNotFound},
		{"outsider", &auth.Claims{UserID: fx.outsider.ID}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Each iteration seeds a fresh annotation so the OK
			// cases don't fail downstream cases by leaving the row
			// gone.
			annot := seedAnnotation(t, h, fx)
			assert.Equal(t, tc.want, callDelete(t, h, tc.claims, annot.ID))
		})
	}
}

func TestAnnotationAuth_ResolveMatrix(t *testing.T) {
	fx := newSessionPageFixture(t, "anno-res")
	h := newAnnotationHandlerForFixture(fx)
	taID := addAnnotTA(t, fx, "res")
	peerID := addAnnotPeerStudent(t, fx, "res")
	annot := seedAnnotation(t, h, fx)

	cases := []struct {
		name   string
		claims *auth.Claims
		want   int
	}{
		{"teacher of session", &auth.Claims{UserID: fx.teacher.ID}, http.StatusOK},
		{"platform admin", &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true}, http.StatusOK},
		{"org_admin", &auth.Claims{UserID: fx.orgAdmin.ID}, http.StatusOK},
		{"class TA", &auth.Claims{UserID: taID}, http.StatusOK},
		{"doc owner (student on own doc)", &auth.Claims{UserID: fx.student.ID}, http.StatusForbidden},
		{"peer student in same class", &auth.Claims{UserID: peerID}, http.StatusNotFound},
		{"outsider", &auth.Claims{UserID: fx.outsider.ID}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, callResolve(t, h, tc.claims, annot.ID))
		})
	}
}

// --- Auth required (no claims) preserved ---

func TestAnnotationAuth_NoClaims(t *testing.T) {
	fx := newSessionPageFixture(t, "anno-noauth")
	h := newAnnotationHandlerForFixture(fx)
	docID := annotDocID(fx, fx.student.ID)
	annot := seedAnnotation(t, h, fx)

	assert.Equal(t, http.StatusUnauthorized, callList(t, h, nil, docID))
	assert.Equal(t, http.StatusUnauthorized, callCreate(t, h, nil, docID))
	assert.Equal(t, http.StatusUnauthorized, callDelete(t, h, nil, annot.ID))
	assert.Equal(t, http.StatusUnauthorized, callResolve(t, h, nil, annot.ID))
}

// --- Other-student-in-same-class returns 404 (not 403) ---
//
// The matrix tests above cover this with `addAnnotPeerStudent`. This
// dedicated test pins the load-bearing distinction down: a real
// student-role member of the doc-owner's class who is NOT the doc
// owner gets 404 across every endpoint. Belt-and-suspenders to
// guard against a future refactor that accidentally collapses
// "doc owner" with "any class member".

func TestAnnotationAuth_OtherStudentSameClass_404(t *testing.T) {
	fx := newSessionPageFixture(t, "anno-othr")
	h := newAnnotationHandlerForFixture(fx)
	peerID := addAnnotPeerStudent(t, fx, "othr")
	annot := seedAnnotation(t, h, fx)
	peerClaims := &auth.Claims{UserID: peerID}

	docID := annotDocID(fx, fx.student.ID)
	assert.Equal(t, http.StatusNotFound, callList(t, h, peerClaims, docID))
	assert.Equal(t, http.StatusNotFound, callCreate(t, h, peerClaims, docID))
	assert.Equal(t, http.StatusNotFound, callDelete(t, h, peerClaims, annot.ID))
	assert.Equal(t, http.StatusNotFound, callResolve(t, h, peerClaims, annot.ID))
}
