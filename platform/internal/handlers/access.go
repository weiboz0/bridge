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
