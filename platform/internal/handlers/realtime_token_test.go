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

func TestMintToken_BroadcastDoc_TeacherOK_StudentDenied(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-bc")
	h := newRealtimeHandlerForFixture(fx)
	docName := "broadcast:" + fx.sessionID

	codeT, respT := callMintToken(t, h, docName, &auth.Claims{UserID: fx.teacher.ID})
	require.Equal(t, http.StatusOK, codeT)
	claims, err := auth.VerifyRealtimeToken(rtSecret, respT.Token)
	require.NoError(t, err)
	assert.Equal(t, "teacher", claims.Role)

	codeS, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.student.ID})
	assert.Equal(t, http.StatusForbidden, codeS)
}

func TestMintToken_AttemptDoc_OwnerOK_OthersDenied(t *testing.T) {
	fx := newSessionPageFixture(t, "rt-att")
	h := newRealtimeHandlerForFixture(fx)

	// Seed a problem + attempt owned by the student.
	problems := store.NewProblemStore(fx.db)
	p, err := problems.CreateProblem(t.Context(), store.CreateProblemInput{
		Scope:       "personal",
		ScopeID:     &fx.student.ID,
		Title:       "Realtime Test Problem",
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
		ProblemID: p.ID,
		UserID:    fx.student.ID,
		Title:     "Attempt 1",
		Language:  "python",
		PlainText: "",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(context.Background(), "DELETE FROM attempts WHERE id = $1", a.ID)
	})

	docName := "attempt:" + a.ID

	// owner OK
	codeO, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.student.ID})
	assert.Equal(t, http.StatusOK, codeO)

	// non-owner non-admin denied (Phase-1 narrow rule; teacher-watch
	// path is deferred to phase-2)
	codeT, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.teacher.ID})
	assert.Equal(t, http.StatusForbidden, codeT)

	// platform admin OK
	codeA, _ := callMintToken(t, h, docName, &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true})
	assert.Equal(t, http.StatusOK, codeA)
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
