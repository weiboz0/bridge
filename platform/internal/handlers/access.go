package handlers

import (
	"context"
	"errors"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// ErrAccessHelperMisconfigured is returned by RequireClassAuthority
// when a required store dependency is nil. Callers see this as a 500
// (Database error) rather than a silent 404; a misconfigured handler
// is a programming bug, not an access decision.
var ErrAccessHelperMisconfigured = errors.New("access helper misconfigured: required store is nil")

// AccessLevel describes how privileged the caller must be to pass a
// `RequireClassAuthority` check. Plan 052.
//
// Levels are NOT a hierarchy in the strict sense — `roster` is not a
// strict subset of `mutate`. Each level has its own membership rule.
// The names describe the *use case*, not a privilege ladder:
//
//   - AccessRead:   "view class metadata"
//                   any class member; plus org_admin / platform admin.
//   - AccessRoster: "view who is in the class"
//                   instructor or TA only; plus org_admin / platform admin.
//                   Students DO NOT pass — `ListMembers` returns email +
//                   name PII (`store/classes.go:45-52`); the help-queue
//                   UI uses `session_participants`, not class members,
//                   so students don't need this view.
//   - AccessMutate: "change class membership or class state"
//                   instructor only; plus org_admin / platform admin.
//                   TAs DO NOT pass — TA is a teaching role, not a
//                   class-admin role.
//
// Platform admin and impersonator-of-admin (per plan 039) bypass all
// three levels — admins inspecting a class while impersonating a
// student should retain the access they had before impersonation.
type AccessLevel string

const (
	AccessRead   AccessLevel = "read"
	AccessRoster AccessLevel = "roster"
	AccessMutate AccessLevel = "mutate"
)

// RequireClassAuthority resolves the class and applies the access
// rule for `level`. Returns:
//
//   - (class, true,  nil)  — access granted; caller can use the class.
//   - (nil,   false, nil)  — class not found OR caller has no access
//                            at this level. Per the plan-052 deny
//                            convention, the CALLER decides whether to
//                            return 404 (class subsystem) or 403
//                            (other subsystems). 404 is preferred for
//                            the class subsystem because the existing
//                            `CanAccessClass` precedent at
//                            `classes.go:218-225` does so.
//   - (nil,   false, err)  — DB error.
//
// This function is the per-plan-052 free-function replacement for
// `ClassHandler.CanAccessClass`. ClassHandler keeps its own thin
// wrapper for backwards compatibility with existing call sites; new
// call sites in other handler types (Schedule, Assignment) call this
// directly.
func RequireClassAuthority(
	ctx context.Context,
	classes *store.ClassStore,
	orgs *store.OrgStore,
	claims *auth.Claims,
	classID string,
	level AccessLevel,
) (*store.Class, bool, error) {
	if claims == nil {
		return nil, false, nil
	}
	if classes == nil {
		// Handler wired without a class store — surface a real error
		// so the caller writes 500. Silently returning (nil, false, nil)
		// would mask the misconfiguration as a 404.
		return nil, false, ErrAccessHelperMisconfigured
	}

	class, err := classes.GetClass(ctx, classID)
	if err != nil {
		return nil, false, err
	}
	if class == nil {
		return nil, false, nil
	}

	// Platform admin / impersonator-of-admin bypass at every level.
	// Plan 039 carved out impersonator access; preserve it across
	// roster and mutate too — the underlying admin retained those
	// privileges before they impersonated.
	if claims.IsPlatformAdmin || claims.ImpersonatedBy != "" {
		return class, true, nil
	}

	// Org admin of the class's owning org bypasses at every level.
	if orgs != nil {
		roles, err := orgs.GetUserRolesInOrg(ctx, class.OrgID, claims.UserID)
		if err != nil {
			return nil, false, err
		}
		for _, role := range roles {
			if role.Role == "org_admin" && role.Status == "active" {
				return class, true, nil
			}
		}
	}

	// Class-membership lookup. Find the caller's row (if any) and
	// apply the level-specific rule.
	members, err := classes.ListClassMembers(ctx, classID)
	if err != nil {
		return nil, false, err
	}
	for _, m := range members {
		if m.UserID != claims.UserID {
			continue
		}
		switch level {
		case AccessRead:
			return class, true, nil
		case AccessRoster:
			if m.Role == "instructor" || m.Role == "ta" {
				return class, true, nil
			}
			return nil, false, nil
		case AccessMutate:
			if m.Role == "instructor" {
				return class, true, nil
			}
			return nil, false, nil
		}
	}

	return nil, false, nil
}

// OrgAccessLevel describes how privileged the caller must be to pass
// a `RequireOrgAuthority` check. Plan 075.
//
// Levels are NOT a hierarchy in the strict sense — each level has its
// own membership rule. The names describe the *use case*:
//
//   - OrgRead:  "view org metadata or list members"
//               any active member of any role; plus platform admin and
//               impersonator-of-admin.
//   - OrgTeach: "create classes/courses scoped to this org"
//               active teacher OR active org_admin; plus platform admin
//               and impersonator-of-admin. Students and parents do NOT
//               pass — class/course creation is a teaching action.
//   - OrgAdmin: "mutate org metadata or manage memberships and parent links"
//               active org_admin only; plus platform admin and
//               impersonator-of-admin.
//
// Bypass order:
//
//  1. claims == nil  → deny (caller writes 401 separately).
//  2. claims.IsPlatformAdmin → grant.
//  3. claims.ImpersonatedBy != "" → grant (per plan 039 — the underlying
//     admin retained these privileges before they impersonated).
//  4. otherwise: scan the caller's roles in the target org and apply the
//     level rule below. EVERY level requires `Status == "active"` —
//     suspended members do not pass any check, mirroring the
//     class-side helper's class-membership branch and plan 069 phase 4's
//     self-action guard.
//
// Note: `RequireClassAuthority` filters on `Status == "active"` only
// inside its class-membership branch; this helper applies the active
// filter uniformly across all org-role checks. The uniform behavior is
// more correct (suspended ≠ allowed) and matches user expectations.
type OrgAccessLevel string

const (
	OrgRead  OrgAccessLevel = "read"
	OrgTeach OrgAccessLevel = "teach"
	OrgAdmin OrgAccessLevel = "admin"
)

// RequireOrgAuthority applies the access rule for `level`. Returns:
//
//   - (true,  nil) — access granted.
//   - (false, nil) — access denied. Caller writes the 403 (or 404 if
//     the subsystem prefers, per plan 052's deny convention).
//   - (false, err) — DB error from `GetUserRolesInOrg`, OR
//     `ErrAccessHelperMisconfigured` when `orgs == nil` AND the caller is
//     not bypassing via `IsPlatformAdmin`/`ImpersonatedBy`. The bypass
//     fires before the nil-orgs guard (lines 206-215), so platform admins
//     can reach downstream input validation even when a unit-test handler
//     is wired without a store. Callers writes the 500.
//
// Plan 075: replaces the inline `GetUserRolesInOrg` + role-loop pattern
// repeated across `OrgHandler`, `OrgParentLinksHandler`, `ClassHandler`,
// `CourseHandler`, `OrgDashboardHandler`, and `TopicProblemsHandler`.
func RequireOrgAuthority(
	ctx context.Context,
	orgs *store.OrgStore,
	claims *auth.Claims,
	orgID string,
	level OrgAccessLevel,
) (bool, error) {
	if claims == nil {
		return false, nil
	}

	// Platform admin / impersonator-of-admin bypass at every level —
	// checked before the nil-orgs guard so that platform admins can
	// reach downstream input validation even when the handler under
	// test is wired without a store.
	// Plan 039 carved out impersonator access for the class subsystem;
	// extending it to org-side is consistent with that intent — the
	// underlying admin retained these privileges before impersonating.
	if claims.IsPlatformAdmin || claims.ImpersonatedBy != "" {
		return true, nil
	}

	if orgs == nil {
		return false, ErrAccessHelperMisconfigured
	}

	roles, err := orgs.GetUserRolesInOrg(ctx, orgID, claims.UserID)
	if err != nil {
		return false, err
	}

	for _, m := range roles {
		if m.Status != "active" {
			continue
		}
		switch level {
		case OrgRead:
			// Any active member passes.
			return true, nil
		case OrgTeach:
			if m.Role == "teacher" || m.Role == "org_admin" {
				return true, nil
			}
		case OrgAdmin:
			if m.Role == "org_admin" {
				return true, nil
			}
		}
	}

	return false, nil
}

// CanViewUnit reports whether `claims` may view `unit`. The rules
// mirror spec 012 §Access:
//
//   - platform scope: classroom_ready/coach_ready/archived
//     → any authenticated viewer; draft/reviewed → platform admin only.
//   - org scope: active teacher or org_admin in the unit's org.
//     Students pass via Plan 061 — verified class binding (any
//     class_membership row in a class whose course owns the unit's
//     topic; existence-only since class_memberships has no status
//     column), limited to classroom_ready / coach_ready / archived
//     units.
//   - personal scope: owner only.
//   - platform admin: bypass at every scope/status.
//
// Plan 052 PR-C: free-function form so non-TeachingUnitHandler
// callers (UnitCollectionHandler.AddItem) can apply the same rule.
// `TeachingUnitHandler.canViewUnit` is a thin wrapper around this.
//
// Plan 061: takes a TeachingUnitStore so the student-binding check
// can be done in a single SQL query without callers wiring it
// themselves. Pass nil if the caller doesn't have one wired —
// student-binding will be skipped (so callers that don't yet take
// the store fall back to the pre-061 behavior).
func CanViewUnit(ctx context.Context, orgs *store.OrgStore, units *store.TeachingUnitStore, claims *auth.Claims, unit *store.TeachingUnit) bool {
	if claims == nil || unit == nil {
		return false
	}
	if claims.IsPlatformAdmin {
		return true
	}
	switch unit.Scope {
	case "platform":
		// Public reading-room content — visible to any authed user
		// once published. (Draft/reviewed are platform-admin-only;
		// we already returned true above for IsPlatformAdmin.)
		if unit.Status == "classroom_ready" ||
			unit.Status == "coach_ready" ||
			unit.Status == "archived" {
			return true
		}
		return false
	case "org":
		if unit.ScopeID == nil || orgs == nil {
			return false
		}
		// Org teachers and org_admins always pass (any unit status,
		// editing or otherwise).
		roles, _ := orgs.GetUserRolesInOrg(ctx, *unit.ScopeID, claims.UserID)
		for _, m := range roles {
			if m.Status != "active" {
				continue
			}
			if m.Role == "org_admin" || m.Role == "teacher" {
				return true
			}
		}
		// Plan 061 — student-binding path. Student passes when the
		// unit is linked to a topic owned by a course they're
		// enrolled in (via class_membership), AND the unit is in a
		// student-readable status.
		if units != nil && unit.TopicID != nil &&
			(unit.Status == "classroom_ready" ||
				unit.Status == "coach_ready" ||
				unit.Status == "archived") {
			ok, err := units.IsStudentInTopicCourse(ctx, claims.UserID, *unit.TopicID)
			if err == nil && ok {
				return true
			}
		}
		return false
	case "personal":
		return unit.ScopeID != nil && *unit.ScopeID == claims.UserID
	}
	return false
}
