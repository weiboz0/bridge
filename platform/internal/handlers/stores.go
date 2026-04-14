package handlers

import (
	"database/sql"

	"github.com/weiboz0/bridge/platform/internal/store"
)

// Stores holds all store instances for dependency injection.
type Stores struct {
	Orgs         *store.OrgStore
	Users        *store.UserStore
	Courses      *store.CourseStore
	Topics       *store.TopicStore
	Classes      *store.ClassStore
	Sessions     *store.SessionStore
	Documents    *store.DocumentStore
	Assignments  *store.AssignmentStore
	Annotations  *store.AnnotationStore
	Classrooms   *store.ClassroomStore
	Interactions *store.InteractionStore
	Reports      *store.ReportStore
	Stats        *store.StatsStore
}

// NewStores creates all stores from a database connection.
func NewStores(db *sql.DB) *Stores {
	return &Stores{
		Orgs:         store.NewOrgStore(db),
		Users:        store.NewUserStore(db),
		Courses:      store.NewCourseStore(db),
		Topics:       store.NewTopicStore(db),
		Classes:      store.NewClassStore(db),
		Sessions:     store.NewSessionStore(db),
		Documents:    store.NewDocumentStore(db),
		Assignments:  store.NewAssignmentStore(db),
		Annotations:  store.NewAnnotationStore(db),
		Classrooms:   store.NewClassroomStore(db),
		Interactions: store.NewInteractionStore(db),
		Reports:      store.NewReportStore(db),
		Stats:        store.NewStatsStore(db),
	}
}
