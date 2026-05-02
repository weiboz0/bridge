package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 052 PR-B: CloneCourse auth matrix.
//
// CloneCourse previously checked only `claims != nil`, allowing any
// authenticated user to clone any course by ID and walk away with a
// private copy. Now requires course access (creator, class member of
// a class using the course, or platform admin).

func newCourseHandlerForFixture(fx *sessionPageFixture) *CourseHandler {
	return &CourseHandler{
		Courses: store.NewCourseStore(fx.db),
		Orgs:    store.NewOrgStore(fx.db),
	}
}

func callCloneCourse(t *testing.T, ch *CourseHandler, courseID string, claims *auth.Claims) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/courses/"+courseID+"/clone", nil)
	req = withChiParams(withClaims(req, claims), map[string]string{"id": courseID})
	w := httptest.NewRecorder()
	ch.CloneCourse(w, req)
	return w.Code
}

func TestCloneCourse_AuthMatrix(t *testing.T) {
	cases := []struct {
		role     string
		expected int
	}{
		// 403 on deny per `courses.go:160-168` precedent.
		{"outsider", http.StatusForbidden},
		// student passes via class-membership-implies-course-access
		// (UserHasAccessToCourse uses class memberships).
		{"student", http.StatusCreated},
		// ta: class member (added inline) → passes UserHasAccessToCourse
		// just like student → 201. Whether TAs SHOULD be able to clone
		// is a product question; the auth model treats them the same
		// as students for course reads.
		{"ta", http.StatusCreated},
		// instructor: class member AND course creator (CreateCourse
		// in the fixture sets created_by = teacher.ID).
		{"instructor", http.StatusCreated},
		// orgAdmin: not a class member, not the course creator,
		// UserHasAccessToCourse returns false → 403. (Same as
		// GetTopic above; org-admin's relation to courses is its
		// own design question.)
		{"orgAdmin", http.StatusForbidden},
		{"platformAdmin", http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			fx := newSessionPageFixture(t, "cc-"+tc.role)
			if tc.role == "ta" {
				addTAToFixture(t, fx)
			}
			ch := newCourseHandlerForFixture(fx)
			code := callCloneCourse(t, ch, fx.courseID, authFxClaimsByRole(fx, tc.role))
			assert.Equal(t, tc.expected, code, "role=%s", tc.role)
			// Cleanup the cloned course rows that successful runs created.
			if tc.expected == http.StatusCreated {
				t.Cleanup(func() {
					fx.db.ExecContext(t.Context(), `DELETE FROM topics WHERE course_id IN (SELECT id FROM courses WHERE org_id = $1 AND id != $2)`, fx.orgID, fx.courseID)
					fx.db.ExecContext(t.Context(), `DELETE FROM courses WHERE org_id = $1 AND id != $2`, fx.orgID, fx.courseID)
				})
			}
		})
	}
}

func TestCloneCourse_NoClaims_Unauthorized(t *testing.T) {
	fx := newSessionPageFixture(t, "cc-noclaims")
	ch := newCourseHandlerForFixture(fx)
	code := callCloneCourse(t, ch, fx.courseID, nil)
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestCloneCourse_NonexistentCourse_404(t *testing.T) {
	fx := newSessionPageFixture(t, "cc-missing")
	ch := newCourseHandlerForFixture(fx)
	bogus := "00000000-0000-0000-0000-000000000abc"
	// Use a non-admin caller so we hit the GetCourse → nil branch.
	code := callCloneCourse(t, ch, bogus, &auth.Claims{UserID: fx.teacher.ID})
	assert.Equal(t, http.StatusNotFound, code)
}
