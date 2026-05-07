package handlers

import (
	"context"
	"net/http"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// TODO(plan-075-followup): canViewProblemAtScope (line ~34) and
// canEditProblemAtScope (line ~144) each have an inline GetUserRolesInOrg in
// the org-scope branch of a switch over scope. Migrating to RequireOrgAuthority
// touches helper signatures (returns bool, not (bool, error)). See plan-075
// §Out of scope, Bucket 2.

// problemAccessDeps bundles the stores needed for the shared problem-access
// helpers. ProblemHandler, SolutionHandler, and TopicProblemHandler each
// construct one from their own fields.
type problemAccessDeps struct {
	Problems      *store.ProblemStore
	TopicProblems *store.TopicProblemStore
	Topics        *store.TopicStore
	Courses       *store.CourseStore
	Orgs          *store.OrgStore
}

// authorizedForScope reports whether the caller may author content under the
// given scope/scopeID. Platform scope requires IsPlatformAdmin; org scope
// requires an active org_admin or teacher membership in the org; personal
// scope requires the caller to be the scope owner.
func authorizedForScope(ctx context.Context, d problemAccessDeps, c *auth.Claims, scope string, scopeID *string) bool {
	switch scope {
	case "platform":
		return c.IsPlatformAdmin
	case "org":
		if scopeID == nil {
			return false
		}
		roles, err := d.Orgs.GetUserRolesInOrg(ctx, *scopeID, c.UserID)
		if err != nil || len(roles) == 0 {
			return false
		}
		for _, m := range roles {
			if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
				return true
			}
		}
		return false
	case "personal":
		return scopeID != nil && *scopeID == c.UserID
	default:
		return false
	}
}

// authorizedForProblemEdit loads the problem and delegates to
// authorizedForScope. A non-admin creator of a personal-scope problem is also
// always allowed to edit their own problem.
func authorizedForProblemEdit(ctx context.Context, d problemAccessDeps, c *auth.Claims, problemID string) bool {
	p, err := d.Problems.GetProblem(ctx, problemID)
	if err != nil || p == nil {
		return false
	}
	return authorizedForProblemEditRow(ctx, d, c, p)
}

// authorizedForProblemEditRow is the row-level variant of authorizedForProblemEdit
// that skips the DB fetch when the caller already has the problem row.
func authorizedForProblemEditRow(ctx context.Context, d problemAccessDeps, c *auth.Claims, p *store.Problem) bool {
	if authorizedForScope(ctx, d, c, p.Scope, p.ScopeID) {
		return true
	}
	return p.Scope == "personal" && p.CreatedBy == c.UserID
}

// canViewTopic returns true if the caller has read access to the topic's
// course (platform admin, course creator, or a member of a class in the
// course). Also returns an HTTP status hint (404 for missing topic/course)
// so callers can distinguish "not found" from "forbidden" when they care.
func canViewTopic(ctx context.Context, d problemAccessDeps, c *auth.Claims, topicID string) (bool, int, error) {
	if c.IsPlatformAdmin {
		return true, 0, nil
	}
	topic, err := d.Topics.GetTopic(ctx, topicID)
	if err != nil {
		return false, http.StatusInternalServerError, err
	}
	if topic == nil {
		return false, http.StatusNotFound, nil
	}
	course, err := d.Courses.GetCourse(ctx, topic.CourseID)
	if err != nil {
		return false, http.StatusInternalServerError, err
	}
	if course == nil {
		return false, http.StatusNotFound, nil
	}
	if course.CreatedBy == c.UserID {
		return true, 0, nil
	}
	hasAccess, err := d.Courses.UserHasAccessToCourse(ctx, course.ID, c.UserID)
	if err != nil {
		return false, http.StatusInternalServerError, err
	}
	return hasAccess, 0, nil
}

// canViewProblem applies the full view matrix:
//   - platform admin: always true
//   - platform scope: published/archived visible to anyone
//   - org scope (published/archived): visible to active org members
//   - personal scope: visible to the owner
//   - draft (any scope): visible only to editors
//   - attachment grant: any attached topic the caller can view grants read
func canViewProblem(ctx context.Context, d problemAccessDeps, c *auth.Claims, problemID string) (bool, *store.Problem, int) {
	p, err := d.Problems.GetProblem(ctx, problemID)
	if err != nil {
		return false, nil, http.StatusInternalServerError
	}
	if p == nil {
		return false, nil, http.StatusNotFound
	}
	if c.IsPlatformAdmin {
		return true, p, 0
	}
	if canViewProblemRow(ctx, d, c, p) {
		return true, p, 0
	}
	return false, p, 0
}

// canViewProblemRow is the row-level variant of canViewProblem that skips the
// DB fetch when the caller already has the problem row.
func canViewProblemRow(ctx context.Context, d problemAccessDeps, c *auth.Claims, p *store.Problem) bool {
	if c.IsPlatformAdmin {
		return true
	}
	// Drafts are editor-only, regardless of scope.
	if p.Status == "draft" {
		return authorizedForProblemEditRow(ctx, d, c, p)
	}
	switch p.Scope {
	case "platform":
		return p.Status == "published" || p.Status == "archived"
	case "org":
		if p.ScopeID == nil {
			return false
		}
		roles, err := d.Orgs.GetUserRolesInOrg(ctx, *p.ScopeID, c.UserID)
		if err != nil {
			return false
		}
		for _, m := range roles {
			if m.Status == "active" {
				return true
			}
		}
	case "personal":
		if p.ScopeID != nil && *p.ScopeID == c.UserID {
			return true
		}
	}
	// Attachment grant: if the caller can view any topic the problem is
	// attached to, that's sufficient.
	topicIDs, err := d.TopicProblems.ListTopicsByProblem(ctx, p.ID)
	if err != nil {
		return false
	}
	for _, topicID := range topicIDs {
		ok, _, err := canViewTopic(ctx, d, c, topicID)
		if err == nil && ok {
			return true
		}
	}
	return false
}
