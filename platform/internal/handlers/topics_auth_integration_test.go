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

// Plan 052 PR-B: GetTopic auth matrix.
//
// Reuses the existing sessionPageFixture which wires org / course /
// class / teacher / student / outsider / orgAdmin / admin. The topic
// is created in the fixture's course; access depends on
// UserHasAccessToCourse(courseID, userID) — i.e., the caller must
// have a class membership in a class of the topic's course OR be the
// course's creator OR a platform admin.

func newTopicHandlerForFixture(fx *sessionPageFixture) *TopicHandler {
	return &TopicHandler{
		Topics:        store.NewTopicStore(fx.db),
		Courses:       store.NewCourseStore(fx.db),
		Orgs:          store.NewOrgStore(fx.db),
		TeachingUnits: store.NewTeachingUnitStore(fx.db),
	}
}

func callGetTopic(t *testing.T, ch *TopicHandler, courseID, topicID string, claims *auth.Claims) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/courses/"+courseID+"/topics/"+topicID, nil)
	req = withChiParams(withClaims(req, claims), map[string]string{"courseId": courseID, "topicId": topicID})
	w := httptest.NewRecorder()
	ch.GetTopic(w, req)
	return w.Code
}

// findFixtureTopicID resolves the fixture's loops topic so the test
// has a real topicId to fetch.
func findFixtureTopicID(t *testing.T, fx *sessionPageFixture) string {
	t.Helper()
	topics := store.NewTopicStore(fx.db)
	rows, err := topics.ListTopicsByCourse(context.Background(), fx.courseID)
	require.NoError(t, err)
	require.NotEmpty(t, rows, "fixture should have at least one topic")
	return rows[0].ID
}

func TestGetTopic_AuthMatrix(t *testing.T) {
	cases := []struct {
		role     string
		expected int
	}{
		// outsider: no class membership in any course of this fixture
		// → 403 (topic subsystem returns 403 on deny per topics.go:122-124).
		{"outsider", http.StatusForbidden},
		// student: class member; class is in the topic's course → passes
		// UserHasAccessToCourse → 200.
		{"student", http.StatusOK},
		// ta: class member (added inline) → passes UserHasAccessToCourse
		// just like student → 200. TA is a teaching role but for
		// course-content reads it has the same access as any class
		// member.
		{"ta", http.StatusOK},
		// instructor: created the course → passes via creator check → 200.
		{"instructor", http.StatusOK},
		// orgAdmin: not a class member of the test class. Course creator
		// check fails. UserHasAccessToCourse only checks class
		// memberships, not org-admin role → 403. (org_admin power over
		// course content is its own design question; not gated here.)
		{"orgAdmin", http.StatusForbidden},
		{"platformAdmin", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			fx := newSessionPageFixture(t, "gt-"+tc.role)
			if tc.role == "ta" {
				addTAToFixture(t, fx)
			}
			ch := newTopicHandlerForFixture(fx)
			topicID := findFixtureTopicID(t, fx)
			code := callGetTopic(t, ch, fx.courseID, topicID, authFxClaimsByRole(fx, tc.role))
			assert.Equal(t, tc.expected, code, "role=%s", tc.role)
		})
	}
}

func TestGetTopic_NonexistentTopic_404(t *testing.T) {
	fx := newSessionPageFixture(t, "gt-missing")
	ch := newTopicHandlerForFixture(fx)
	bogus := "00000000-0000-0000-0000-000000000abc"
	code := callGetTopic(t, ch, fx.courseID, bogus, &auth.Claims{UserID: fx.teacher.ID})
	assert.Equal(t, http.StatusNotFound, code)
}
