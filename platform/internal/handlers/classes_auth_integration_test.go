package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 052 PR-A: auth matrix for the five class handlers that
// previously checked only `claims != nil`. Each handler is exercised
// with every "interesting" caller role: outsider, student member,
// instructor (auto-added by CreateClass), org_admin, and platform
// admin. Plus `claims == nil` (Unauthorized).
//
// Reuses `newSessionPageFixture` so the test classes have realistic
// org / course / class / membership rows. Each test sub-name
// describes the (handler, caller-role, expected-status) tuple.

// --- helpers ---------------------------------------------------------------

func authFxClaimsByRole(fx *sessionPageFixture, role string) *auth.Claims {
	switch role {
	case "instructor":
		return &auth.Claims{UserID: fx.teacher.ID}
	case "student":
		return &auth.Claims{UserID: fx.student.ID}
	case "outsider":
		return &auth.Claims{UserID: fx.outsider.ID}
	case "orgAdmin":
		return &auth.Claims{UserID: fx.orgAdmin.ID}
	case "platformAdmin":
		return &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true}
	}
	return nil
}

func callArchiveClass(t *testing.T, ch *ClassHandler, classID string, claims *auth.Claims) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/classes/"+classID, nil)
	req = withChiParams(withClaims(req, claims), map[string]string{"id": classID})
	w := httptest.NewRecorder()
	ch.ArchiveClass(w, req)
	return w.Code
}

func callListMembers(t *testing.T, ch *ClassHandler, classID string, claims *auth.Claims) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/classes/"+classID+"/members", nil)
	req = withChiParams(withClaims(req, claims), map[string]string{"id": classID})
	w := httptest.NewRecorder()
	ch.ListMembers(w, req)
	return w.Code
}

func callAddMember(t *testing.T, ch *ClassHandler, classID, email string, claims *auth.Claims) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "role": "student"})
	req := httptest.NewRequest(http.MethodPost, "/api/classes/"+classID+"/members", bytes.NewReader(body))
	req = withChiParams(withClaims(req, claims), map[string]string{"id": classID})
	w := httptest.NewRecorder()
	ch.AddMember(w, req)
	return w.Code
}

func callUpdateMemberRole(t *testing.T, ch *ClassHandler, classID, memberID, role string, claims *auth.Claims) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"role": role})
	req := httptest.NewRequest(http.MethodPatch, "/api/classes/"+classID+"/members/"+memberID, bytes.NewReader(body))
	req = withChiParams(withClaims(req, claims), map[string]string{"id": classID, "memberId": memberID})
	w := httptest.NewRecorder()
	ch.UpdateMemberRole(w, req)
	return w.Code
}

func callRemoveMember(t *testing.T, ch *ClassHandler, classID, memberID string, claims *auth.Claims) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/classes/"+classID+"/members/"+memberID, nil)
	req = withChiParams(withClaims(req, claims), map[string]string{"id": classID, "memberId": memberID})
	w := httptest.NewRecorder()
	ch.RemoveMember(w, req)
	return w.Code
}

// findStudentMembershipID looks up the auto-created student membership
// in the fixture so the role-update / remove tests have a real
// memberID to operate on.
func findStudentMembershipID(t *testing.T, fx *sessionPageFixture) string {
	t.Helper()
	classes := store.NewClassStore(fx.db)
	members, err := classes.ListClassMembers(t.Context(), fx.classID)
	require.NoError(t, err)
	for _, m := range members {
		if m.UserID == fx.student.ID {
			return m.ID
		}
	}
	t.Fatalf("student membership not found in fixture")
	return ""
}

// --- ArchiveClass (mutate level) -------------------------------------------

func TestArchiveClass_AuthMatrix(t *testing.T) {
	cases := []struct {
		role     string
		expected int
	}{
		{"outsider", http.StatusNotFound},  // not a class member
		{"student", http.StatusNotFound},   // member but not instructor
		{"instructor", http.StatusOK},      // class instructor
		{"orgAdmin", http.StatusOK},        // org_admin of class's org
		{"platformAdmin", http.StatusOK},   // platform admin
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			fx := newSessionPageFixture(t, "arch-"+tc.role)
			ch := newClassHandlerForFixture(fx)
			code := callArchiveClass(t, ch, fx.classID, authFxClaimsByRole(fx, tc.role))
			assert.Equal(t, tc.expected, code, "role=%s", tc.role)
		})
	}
}

func TestArchiveClass_NoClaims_Unauthorized(t *testing.T) {
	fx := newSessionPageFixture(t, "arch-noclaims")
	ch := newClassHandlerForFixture(fx)
	code := callArchiveClass(t, ch, fx.classID, nil)
	assert.Equal(t, http.StatusUnauthorized, code)
}

// --- ListMembers (roster level) --------------------------------------------

func TestListMembers_AuthMatrix(t *testing.T) {
	cases := []struct {
		role     string
		expected int
	}{
		{"outsider", http.StatusNotFound},
		{"student", http.StatusNotFound}, // students do NOT pass roster level
		{"instructor", http.StatusOK},
		{"orgAdmin", http.StatusOK},
		{"platformAdmin", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			fx := newSessionPageFixture(t, "lm-"+tc.role)
			ch := newClassHandlerForFixture(fx)
			code := callListMembers(t, ch, fx.classID, authFxClaimsByRole(fx, tc.role))
			assert.Equal(t, tc.expected, code, "role=%s", tc.role)
		})
	}
}

// --- AddMember (mutate level) ----------------------------------------------

func TestAddMember_AuthMatrix(t *testing.T) {
	cases := []struct {
		role     string
		expected int
	}{
		{"outsider", http.StatusNotFound},
		{"student", http.StatusNotFound},
		{"instructor", http.StatusCreated},
		{"orgAdmin", http.StatusCreated},
		{"platformAdmin", http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			fx := newSessionPageFixture(t, "am-"+tc.role)
			ch := newClassHandlerForFixture(fx)
			// Use the outsider's email as the new member to add — they
			// exist as a user (so GetUserByEmail returns them) but
			// aren't yet a class member.
			code := callAddMember(t, ch, fx.classID, fx.outsider.Email, authFxClaimsByRole(fx, tc.role))
			assert.Equal(t, tc.expected, code, "role=%s", tc.role)
		})
	}
}

// --- UpdateMemberRole (mutate level) ---------------------------------------

func TestUpdateMemberRole_AuthMatrix(t *testing.T) {
	cases := []struct {
		role     string
		expected int
	}{
		{"outsider", http.StatusNotFound},
		{"student", http.StatusNotFound},
		{"instructor", http.StatusOK},
		{"orgAdmin", http.StatusOK},
		{"platformAdmin", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			fx := newSessionPageFixture(t, "umr-"+tc.role)
			ch := newClassHandlerForFixture(fx)
			memberID := findStudentMembershipID(t, fx)
			code := callUpdateMemberRole(t, ch, fx.classID, memberID, "ta", authFxClaimsByRole(fx, tc.role))
			assert.Equal(t, tc.expected, code, "role=%s", tc.role)
		})
	}
}

// --- RemoveMember (mutate level) -------------------------------------------

func TestRemoveMember_AuthMatrix(t *testing.T) {
	cases := []struct {
		role     string
		expected int
	}{
		{"outsider", http.StatusNotFound},
		{"student", http.StatusNotFound},
		{"instructor", http.StatusOK},
		{"orgAdmin", http.StatusOK},
		{"platformAdmin", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			fx := newSessionPageFixture(t, "rm-"+tc.role)
			ch := newClassHandlerForFixture(fx)
			memberID := findStudentMembershipID(t, fx)
			code := callRemoveMember(t, ch, fx.classID, memberID, authFxClaimsByRole(fx, tc.role))
			assert.Equal(t, tc.expected, code, "role=%s", tc.role)
		})
	}
}

// --- impersonator carve-out ------------------------------------------------

// Plan 039 carve-out: a platform admin who is impersonating a non-admin
// retains class access for all three levels (read / roster / mutate).
// Plan 052 preserves this through `RequireClassAuthority`.
func TestClassAuthority_ImpersonatorBypassesAllLevels(t *testing.T) {
	fx := newSessionPageFixture(t, "imp-bypass")
	ch := newClassHandlerForFixture(fx)
	// Admin impersonating outsider: ImpersonatedBy is set, IsPlatformAdmin
	// is false on the impersonated identity (per middleware behavior).
	claims := &auth.Claims{
		UserID:         fx.outsider.ID,
		ImpersonatedBy: fx.admin.ID,
	}
	assert.Equal(t, http.StatusOK, callListMembers(t, ch, fx.classID, claims), "roster")
	memberID := findStudentMembershipID(t, fx)
	assert.Equal(t, http.StatusOK, callRemoveMember(t, ch, fx.classID, memberID, claims), "mutate")
}
