package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 052 PR-B: Schedule.List + Schedule.ListUpcoming auth matrix.
//
// Schedule rows are class-scoped; access requires AccessRead on the
// class. Reuses sessionPageFixture and seeds one scheduled session
// in the fixture's class so List has something to return for
// authorized callers.

func newScheduleHandlerForFixture(fx *sessionPageFixture) *ScheduleHandler {
	return &ScheduleHandler{
		Schedules: store.NewScheduleStore(fx.db),
		Classes:   store.NewClassStore(fx.db),
		Orgs:      store.NewOrgStore(fx.db),
	}
}

// seedSchedule creates one scheduled session in the fixture's class
// so authorized List calls return a non-empty list.
func seedSchedule(t *testing.T, fx *sessionPageFixture) {
	t.Helper()
	schedules := store.NewScheduleStore(fx.db)
	_, err := schedules.CreateSchedule(context.Background(), store.CreateScheduleInput{
		ClassID:        fx.classID,
		TeacherID:      fx.teacher.ID,
		ScheduledStart: time.Now().Add(time.Hour),
		ScheduledEnd:   time.Now().Add(2 * time.Hour),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(context.Background(), "DELETE FROM scheduled_sessions WHERE class_id = $1", fx.classID)
	})
}

func callScheduleList(t *testing.T, ch *ScheduleHandler, classID string, claims *auth.Claims) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/classes/"+classID+"/schedule", nil)
	req = withChiParams(withClaims(req, claims), map[string]string{"classId": classID})
	w := httptest.NewRecorder()
	ch.List(w, req)
	return w.Code
}

func callScheduleListUpcoming(t *testing.T, ch *ScheduleHandler, classID string, claims *auth.Claims) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/classes/"+classID+"/schedule/upcoming", nil)
	req = withChiParams(withClaims(req, claims), map[string]string{"classId": classID})
	w := httptest.NewRecorder()
	ch.ListUpcoming(w, req)
	return w.Code
}

func TestScheduleList_AuthMatrix(t *testing.T) {
	cases := []struct {
		role     string
		expected int
	}{
		// 403 on deny per `schedule.go:85-87` precedent.
		{"outsider", http.StatusForbidden},
		{"student", http.StatusOK},      // class member, AccessRead passes
		{"instructor", http.StatusOK},   // class instructor
		{"orgAdmin", http.StatusOK},     // org_admin of class's org
		{"platformAdmin", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			fx := newSessionPageFixture(t, "sl-"+tc.role)
			seedSchedule(t, fx)
			ch := newScheduleHandlerForFixture(fx)
			code := callScheduleList(t, ch, fx.classID, authFxClaimsByRole(fx, tc.role))
			assert.Equal(t, tc.expected, code, "role=%s", tc.role)
		})
	}
}

func TestScheduleListUpcoming_AuthMatrix(t *testing.T) {
	cases := []struct {
		role     string
		expected int
	}{
		{"outsider", http.StatusForbidden},
		{"student", http.StatusOK},
		{"instructor", http.StatusOK},
		{"orgAdmin", http.StatusOK},
		{"platformAdmin", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			fx := newSessionPageFixture(t, "slu-"+tc.role)
			seedSchedule(t, fx)
			ch := newScheduleHandlerForFixture(fx)
			code := callScheduleListUpcoming(t, ch, fx.classID, authFxClaimsByRole(fx, tc.role))
			assert.Equal(t, tc.expected, code, "role=%s", tc.role)
		})
	}
}
