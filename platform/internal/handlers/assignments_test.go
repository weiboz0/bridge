package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 052 PR-B: GetAssignment auth matrix.
//
// Assignments are class-scoped; AccessRead on the assignment's class
// is required. Reuses sessionPageFixture and creates one assignment
// in the fixture's class.

func newAssignmentHandlerForFixture(fx *sessionPageFixture) *AssignmentHandler {
	return &AssignmentHandler{
		Assignments: store.NewAssignmentStore(fx.db),
		Classes:     store.NewClassStore(fx.db),
		Orgs:        store.NewOrgStore(fx.db),
	}
}

func seedAssignment(t *testing.T, fx *sessionPageFixture) string {
	t.Helper()
	assignments := store.NewAssignmentStore(fx.db)
	a, err := assignments.CreateAssignment(context.Background(), store.CreateAssignmentInput{
		ClassID: fx.classID,
		Title:   "Test Assignment",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(context.Background(), "DELETE FROM assignments WHERE id = $1", a.ID)
	})
	return a.ID
}

func callGetAssignment(t *testing.T, ch *AssignmentHandler, assignmentID string, claims *auth.Claims) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/assignments/"+assignmentID, nil)
	req = withChiParams(withClaims(req, claims), map[string]string{"id": assignmentID})
	w := httptest.NewRecorder()
	ch.GetAssignment(w, req)
	return w.Code
}

func TestGetAssignment_AuthMatrix(t *testing.T) {
	cases := []struct {
		role     string
		expected int
	}{
		// 403 on deny per `assignments.go:133-135` precedent.
		{"outsider", http.StatusForbidden},
		{"student", http.StatusOK},
		{"ta", http.StatusOK}, // TA: AccessRead passes (any class member)
		{"instructor", http.StatusOK},
		{"orgAdmin", http.StatusOK},
		{"platformAdmin", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			fx := newSessionPageFixture(t, "ga-"+tc.role)
			if tc.role == "ta" {
				addTAToFixture(t, fx)
			}
			assignmentID := seedAssignment(t, fx)
			ch := newAssignmentHandlerForFixture(fx)
			code := callGetAssignment(t, ch, assignmentID, authFxClaimsByRole(fx, tc.role))
			assert.Equal(t, tc.expected, code, "role=%s", tc.role)
		})
	}
}

func TestGetAssignment_NoClaims_Unauthorized(t *testing.T) {
	fx := newSessionPageFixture(t, "ga-noclaims")
	assignmentID := seedAssignment(t, fx)
	ch := newAssignmentHandlerForFixture(fx)
	code := callGetAssignment(t, ch, assignmentID, nil)
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestGetAssignment_NotFound_404(t *testing.T) {
	fx := newSessionPageFixture(t, "ga-missing")
	ch := newAssignmentHandlerForFixture(fx)
	bogus := "00000000-0000-0000-0000-000000000abc"
	code := callGetAssignment(t, ch, bogus, &auth.Claims{UserID: fx.teacher.ID})
	assert.Equal(t, http.StatusNotFound, code)
}
