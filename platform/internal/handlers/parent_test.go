package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 064 — parent reports endpoints re-enabled with parent_links
// auth gate. Pre-064 these returned 501 (plan 047 disabled them
// because there was no parent ↔ child link in the DB).

// --- Unit tests (no DB) ---

func TestListReports_NoClaims(t *testing.T) {
	h := &ParentHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/parent/children/00000000-0000-0000-0000-000000000001/reports", nil)
	req = withChiParams(req, map[string]string{"childId": "00000000-0000-0000-0000-000000000001"})
	w := httptest.NewRecorder()
	h.ListReports(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateReport_NoClaims(t *testing.T) {
	h := &ParentHandler{}
	body, _ := json.Marshal(map[string]string{
		"periodStart": "2026-01-01T00:00:00Z",
		"periodEnd":   "2026-01-31T00:00:00Z",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/parent/children/00000000-0000-0000-0000-000000000001/reports", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"childId": "00000000-0000-0000-0000-000000000001"})
	w := httptest.NewRecorder()
	h.CreateReport(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- Integration tests with parent_links auth matrix ---

// parentReportsFixture spins up the world: ReportStore +
// ParentLinkStore + a parent, child, unrelated user, and admin.
type parentReportsFixture struct {
	db          *sql.DB
	h           *ParentHandler
	parent      *store.RegisteredUser
	child       *store.RegisteredUser
	unrelated   *store.RegisteredUser
	admin       *store.RegisteredUser
	parentLinks *store.ParentLinkStore
}

func newParentReportsFixture(t *testing.T, suffix string) *parentReportsFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()
	users := store.NewUserStore(db)
	links := store.NewParentLinkStore(db)
	reports := store.NewReportStore(db)
	h := &ParentHandler{Reports: reports, ParentLinks: links}

	mkUser := func(label string) *store.RegisteredUser {
		u, err := users.RegisterUser(ctx, store.RegisterInput{
			Name:     "PReport " + label,
			Email:    "preport-" + label + "-" + uuid.NewString()[:8] + "@example.com",
			Password: "testpassword123",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM parent_reports WHERE student_id = $1 OR generated_by = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM parent_links WHERE parent_user_id = $1 OR child_user_id = $1 OR created_by = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}

	parent := mkUser(suffix + "-parent")
	child := mkUser(suffix + "-child")
	unrelated := mkUser(suffix + "-unrelated")
	admin := mkUser(suffix + "-admin")

	// Active parent_link parent → child.
	_, err := links.CreateLink(ctx, parent.ID, child.ID, admin.ID)
	require.NoError(t, err)

	return &parentReportsFixture{
		db: db, h: h,
		parent: parent, child: child, unrelated: unrelated, admin: admin,
		parentLinks: links,
	}
}

func (fx *parentReportsFixture) callList(t *testing.T, claims *auth.Claims, childID string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/parent/children/"+childID+"/reports", nil)
	req = withChiParams(req, map[string]string{"childId": childID})
	if claims != nil {
		req = withClaims(req, claims)
	}
	w := httptest.NewRecorder()
	fx.h.ListReports(w, req)
	return w.Code
}

func (fx *parentReportsFixture) callCreate(t *testing.T, claims *auth.Claims, childID string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"periodStart": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		"periodEnd":   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		"content":     "Test report content",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/parent/children/"+childID+"/reports", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"childId": childID})
	if claims != nil {
		req = withClaims(req, claims)
	}
	w := httptest.NewRecorder()
	fx.h.CreateReport(w, req)
	return w.Code
}

func TestParentReports_ListMatrix(t *testing.T) {
	fx := newParentReportsFixture(t, t.Name())

	cases := []struct {
		name   string
		claims *auth.Claims
		want   int
	}{
		{"linked parent", &auth.Claims{UserID: fx.parent.ID}, http.StatusOK},
		{"platform admin (bypass)", &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true}, http.StatusOK},
		{"unrelated user (no link)", &auth.Claims{UserID: fx.unrelated.ID}, http.StatusForbidden},
		{"the child themselves (no self-parent-link)", &auth.Claims{UserID: fx.child.ID}, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, fx.callList(t, tc.claims, fx.child.ID))
		})
	}
}

func TestParentReports_CreateMatrix(t *testing.T) {
	fx := newParentReportsFixture(t, t.Name())

	cases := []struct {
		name   string
		claims *auth.Claims
		want   int
	}{
		{"linked parent", &auth.Claims{UserID: fx.parent.ID}, http.StatusCreated},
		{"platform admin", &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true}, http.StatusCreated},
		{"unrelated user", &auth.Claims{UserID: fx.unrelated.ID}, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, fx.callCreate(t, tc.claims, fx.child.ID))
		})
	}
}

func TestParentReports_RevokedLinkDenies(t *testing.T) {
	fx := newParentReportsFixture(t, t.Name())
	ctx := context.Background()

	// Find the link and revoke it.
	links, err := fx.parentLinks.ListByParent(ctx, fx.parent.ID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	_, err = fx.parentLinks.RevokeLink(ctx, links[0].ID)
	require.NoError(t, err)

	assert.Equal(t, http.StatusForbidden, fx.callList(t, &auth.Claims{UserID: fx.parent.ID}, fx.child.ID),
		"revoked link must NOT grant access — but admin bypass still works")
	assert.Equal(t, http.StatusOK, fx.callList(t, &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true}, fx.child.ID))
}

func TestParentReports_CreateValidatesBody(t *testing.T) {
	fx := newParentReportsFixture(t, t.Name())
	parentClaims := &auth.Claims{UserID: fx.parent.ID}

	// Missing content.
	body, _ := json.Marshal(map[string]any{
		"periodStart": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		"periodEnd":   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/parent/children/"+fx.child.ID+"/reports", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"childId": fx.child.ID})
	req = withClaims(req, parentClaims)
	w := httptest.NewRecorder()
	fx.h.CreateReport(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, "missing content → 400")

	// periodEnd before periodStart.
	body, _ = json.Marshal(map[string]any{
		"periodStart": time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		"periodEnd":   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		"content":     "x",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/parent/children/"+fx.child.ID+"/reports", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"childId": fx.child.ID})
	req = withClaims(req, parentClaims)
	w = httptest.NewRecorder()
	fx.h.CreateReport(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, "periodEnd before periodStart → 400")
}
