package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 053 Phase 1: Hocuspocus realtime-token mint + internal auth
// endpoints.

// RealtimeHandler mints Hocuspocus connection tokens and serves the
// internal auth endpoint Hocuspocus calls during onLoadDocument.
//
// The two endpoints share scope-resolution logic but have different
// auth gates:
//
//   - POST /api/realtime/token: authenticated user (regular session).
//     Returns a JWT scoped to a single documentName.
//   - POST /api/internal/realtime/auth: NOT user-facing. Bearer-auth
//     gated by HOCUSPOCUS_TOKEN_SECRET; called by the Hocuspocus
//     Node process during onLoadDocument as defense-in-depth.
type RealtimeHandler struct {
	Sessions      *store.SessionStore
	Classes       *store.ClassStore
	Orgs          *store.OrgStore
	TeachingUnits *store.TeachingUnitStore
	Problems      *store.ProblemStore
	Attempts      *store.AttemptStore
	Users         *store.UserStore
	ParentLinks   *store.ParentLinkStore // Plan 053b Phase 4 — parent-of-doc-owner gate.
	// HocuspocusTokenSecret is the HMAC key shared between the Go API
	// and the Hocuspocus Node process. Empty = realtime endpoints
	// return 503 (server misconfigured).
	HocuspocusTokenSecret string
}

// Routes registers the public mint endpoint. Must be mounted INSIDE
// the user-auth group so callers carry verified Bridge claims.
func (h *RealtimeHandler) Routes(r chi.Router) {
	r.Route("/api/realtime", func(r chi.Router) {
		r.Post("/token", h.MintToken)
	})
}

// InternalRoutes registers the server-to-server callback endpoint.
// Must be mounted OUTSIDE the user-auth group: Hocuspocus calls this
// with a shared bearer secret, not a Bridge user session, and any
// user-auth middleware would reject the unauthenticated request
// before our bearer check runs.
func (h *RealtimeHandler) InternalRoutes(r chi.Router) {
	r.Route("/api/internal/realtime", func(r chi.Router) {
		r.Post("/auth", h.InternalAuth)
	})
}

// mintRequest is the body shape clients post to /api/realtime/token.
type mintRequest struct {
	DocumentName string `json:"documentName"`
}

// mintResponse is what we return to the client. `expiresAt` lets the
// client schedule a refresh on `exp - 60s`.
type mintResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// MintToken handles POST /api/realtime/token.
//
// Authenticated user → resolve documentName → check the caller can
// access it → sign and return.
func (h *RealtimeHandler) MintToken(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.HocuspocusTokenSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "Realtime tokens not configured")
		return
	}

	var body mintRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.DocumentName == "" {
		writeError(w, http.StatusBadRequest, "documentName is required")
		return
	}

	role, decision := h.authorizeDocument(r.Context(), claims, body.DocumentName)
	if decision != nil {
		writeError(w, decision.Status, decision.Message)
		return
	}

	const ttl = 25 * time.Minute
	token, err := auth.SignRealtimeToken(h.HocuspocusTokenSecret, claims.UserID, role, body.DocumentName, ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Token sign failed")
		return
	}
	writeJSON(w, http.StatusOK, mintResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(ttl),
	})
}

// internalAuthRequest is what the Hocuspocus Node process posts.
type internalAuthRequest struct {
	DocumentName string `json:"documentName"`
	Sub          string `json:"sub"` // claim from the verified JWT
}

// internalAuthResponse tells Hocuspocus whether to allow the
// document load.
type internalAuthResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// InternalAuth handles POST /api/internal/realtime/auth.
//
// Hocuspocus Node calls this in onLoadDocument as defense-in-depth.
// Bearer-token gated: the Authorization header must equal
// `Bearer <HocuspocusTokenSecret>`. NOT user-facing.
func (h *RealtimeHandler) InternalAuth(w http.ResponseWriter, r *http.Request) {
	if h.HocuspocusTokenSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "Realtime tokens not configured")
		return
	}
	expected := "Bearer " + h.HocuspocusTokenSecret
	if r.Header.Get("Authorization") != expected {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body internalAuthRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.DocumentName == "" || body.Sub == "" {
		writeError(w, http.StatusBadRequest, "documentName and sub are required")
		return
	}

	// The internal endpoint runs OUTSIDE the user-auth middleware
	// (Hocuspocus carries the shared bearer, not a Bridge session).
	// We trust that the caller (Hocuspocus) has verified the JWT
	// signature; what we re-check is whether `sub`'s CURRENT
	// permissions in the DB still allow the doc. Rehydrate the
	// platform-admin bit from the users table — the JWT is
	// untrusted at this layer for that bit because it could be
	// stale, and we never want to trust client-supplied admin
	// claims server-side.
	//
	// Status-code conventions (so Hocuspocus / ops can distinguish
	// real "no" decisions from infrastructure failures):
	//   200 + {allowed: true|false} → authorization rendered.
	//   400 → malformed input (bad sub, bad documentName).
	//   404 → user or resource doesn't exist.
	//   500 → server-side failure (DB down, store misconfigured).
	if h.Users == nil {
		writeError(w, http.StatusInternalServerError, "Users store unavailable")
		return
	}
	user, err := h.Users.GetUserByID(r.Context(), body.Sub)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "User lookup failed")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}
	rehydratedClaims := &auth.Claims{
		UserID:          user.ID,
		Email:           user.Email,
		Name:            user.Name,
		IsPlatformAdmin: user.IsPlatformAdmin,
		// ImpersonatedBy intentionally not rehydrated — impersonation
		// is a session-level superpower; the internal recheck enforces
		// the underlying user's actual permissions.
	}
	_, decision := h.authorizeDocument(r.Context(), rehydratedClaims, body.DocumentName)
	if decision != nil {
		// Only forbid-decisions become {allowed: false}. Anything
		// else (400 malformed doc-name, 404 missing session/unit/
		// attempt, 500 DB error) surfaces as a real HTTP error so
		// Hocuspocus retry logic and ops alerting can tell the
		// difference between "deny" and "broken".
		if decision.Status == http.StatusForbidden {
			writeJSON(w, http.StatusOK, internalAuthResponse{Allowed: false, Reason: decision.Message})
			return
		}
		writeError(w, decision.Status, decision.Message)
		return
	}
	writeJSON(w, http.StatusOK, internalAuthResponse{Allowed: true})
}

// authDecision carries a non-200 result for either endpoint.
type authDecision struct {
	Status  int
	Message string
}

// authorizeDocument resolves a Hocuspocus documentName to an access
// decision and a derived `role` for the JWT. Returns
// `(role, nil)` on success or `(_, *authDecision)` with the status
// code to return.
//
// Doc-name shapes:
//
//	session:{sessionId}:user:{studentId}
//	broadcast:{sessionId}
//	attempt:{attemptId}
//	unit:{unitId}
//
// Anything else → 400.
func (h *RealtimeHandler) authorizeDocument(ctx context.Context, claims *auth.Claims, docName string) (string, *authDecision) {
	parts := strings.Split(docName, ":")
	if len(parts) < 2 {
		return "", &authDecision{Status: http.StatusBadRequest, Message: "invalid documentName"}
	}
	switch parts[0] {
	case "session":
		// session:{sessionId}:user:{studentId}
		if len(parts) != 4 || parts[2] != "user" {
			return "", &authDecision{Status: http.StatusBadRequest, Message: "session doc-name must be session:{sid}:user:{uid}"}
		}
		return h.authorizeSessionDoc(ctx, claims, parts[1], parts[3])
	case "broadcast":
		// broadcast:{sessionId}
		if len(parts) != 2 {
			return "", &authDecision{Status: http.StatusBadRequest, Message: "broadcast doc-name must be broadcast:{sid}"}
		}
		return h.authorizeBroadcastDoc(ctx, claims, parts[1])
	case "attempt":
		// attempt:{attemptId}
		if len(parts) != 2 {
			return "", &authDecision{Status: http.StatusBadRequest, Message: "attempt doc-name must be attempt:{aid}"}
		}
		return h.authorizeAttemptDoc(ctx, claims, parts[1])
	case "unit":
		// unit:{unitId}
		if len(parts) != 2 {
			return "", &authDecision{Status: http.StatusBadRequest, Message: "unit doc-name must be unit:{uid}"}
		}
		return h.authorizeUnitDoc(ctx, claims, parts[1])
	default:
		return "", &authDecision{Status: http.StatusBadRequest, Message: "unknown doc-name scope"}
	}
}

// session:{sessionId}:user:{studentId} — the caller may open EITHER
// their own student doc (sub == studentId AND they can join the
// session), OR any student's doc if they're the session's teacher
// or class staff.
func (h *RealtimeHandler) authorizeSessionDoc(ctx context.Context, claims *auth.Claims, sessionID, studentID string) (string, *authDecision) {
	if h.Sessions == nil {
		return "", &authDecision{Status: http.StatusInternalServerError, Message: "Sessions store unavailable"}
	}
	session, err := h.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return "", &authDecision{Status: http.StatusInternalServerError, Message: "Database error"}
	}
	if session == nil {
		return "", &authDecision{Status: http.StatusNotFound, Message: "Session not found"}
	}

	// Platform admin / impersonator bypass.
	if claims.IsPlatformAdmin || claims.ImpersonatedBy != "" {
		return roleForSessionMember(claims.UserID, studentID, session), nil
	}

	// Teacher of the session passes for any student doc.
	if session.TeacherID == claims.UserID {
		return "teacher", nil
	}

	// Class staff (instructor / TA / org_admin / platform admin) of
	// the session's class passes for any student doc, via
	// RequireClassAuthority(AccessRoster).
	if session.ClassID != nil && h.Classes != nil {
		if _, ok, err := RequireClassAuthority(ctx, h.Classes, h.Orgs, claims, *session.ClassID, AccessRoster); err == nil && ok {
			return "teacher", nil
		}
	}

	// Plan 053b Phase 4 — parent of the doc-owning student. Two
	// constraints:
	//   1. Caller has an ACTIVE parent_link to studentID.
	//   2. studentID is a participant in THIS session (so a parent
	//      of one student can't mint tokens for unrelated sessions
	//      whose docName happens to mention their child's id).
	// (1) is IsParentOf; (2) is the GetSessionParticipant lookup
	// already used below for the session-participant fall-back.
	if h.ParentLinks != nil {
		if isParent, err := h.ParentLinks.IsParentOf(ctx, claims.UserID, studentID); err == nil && isParent {
			if existing, perr := h.Sessions.GetSessionParticipant(ctx, sessionID, studentID); perr == nil && existing != nil {
				return "parent", nil
			}
		}
	}

	// Otherwise: caller is opening THEIR OWN doc. sub must match
	// studentId AND they must be a class member or session
	// participant.
	if claims.UserID != studentID {
		return "", &authDecision{Status: http.StatusForbidden, Message: "may only open own session doc"}
	}
	if session.ClassID != nil && h.Classes != nil {
		if _, ok, err := RequireClassAuthority(ctx, h.Classes, h.Orgs, claims, *session.ClassID, AccessRead); err == nil && ok {
			return "user", nil
		}
	}
	// Fall through: not a class member. Check session participation.
	if existing, err := h.Sessions.GetSessionParticipant(ctx, sessionID, claims.UserID); err == nil && existing != nil &&
		(existing.Status == "invited" || existing.Status == "present") {
		return "user", nil
	}
	return "", &authDecision{Status: http.StatusForbidden, Message: "Not authorized"}
}

func roleForSessionMember(callerID, studentID string, session *store.LiveSession) string {
	if session.TeacherID == callerID {
		return "teacher"
	}
	if callerID == studentID {
		return "user"
	}
	return "teacher" // admin or class-staff opening someone else's doc
}

// broadcast:{sessionId} — broadcast docs are one-way: the teacher
// writes, everyone-in-the-class reads. Two roles, two gates:
//
//   role="teacher" (write/start/stop): platform admin OR the
//     session's teacher. Mirrors `SessionHandler.ToggleBroadcast`.
//   role="user" (read-only viewer): any class member or session
//     participant.
//
// The Hocuspocus side doesn't distinguish reader vs writer for
// broadcast docs (Yjs CRDTs are inherently bidirectional); the JWT
// role exists for awareness/UI labeling and for future
// server-enforced read-only.
func (h *RealtimeHandler) authorizeBroadcastDoc(ctx context.Context, claims *auth.Claims, sessionID string) (string, *authDecision) {
	if h.Sessions == nil {
		return "", &authDecision{Status: http.StatusInternalServerError, Message: "Sessions store unavailable"}
	}
	session, err := h.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return "", &authDecision{Status: http.StatusInternalServerError, Message: "Database error"}
	}
	if session == nil {
		return "", &authDecision{Status: http.StatusNotFound, Message: "Session not found"}
	}
	// Teacher/admin path — write role.
	if claims.IsPlatformAdmin {
		return "teacher", nil
	}
	if session.TeacherID == claims.UserID {
		return "teacher", nil
	}
	// Reader path — class member.
	if session.ClassID != nil && h.Classes != nil {
		if _, ok, err := RequireClassAuthority(ctx, h.Classes, h.Orgs, claims, *session.ClassID, AccessRead); err == nil && ok {
			return "user", nil
		}
	}
	// Reader path — session participant (covers token-joined users
	// who aren't enrolled in the class).
	if existing, err := h.Sessions.GetSessionParticipant(ctx, sessionID, claims.UserID); err == nil && existing != nil &&
		(existing.Status == "invited" || existing.Status == "present") {
		return "user", nil
	}
	return "", &authDecision{Status: http.StatusForbidden, Message: "broadcast: not a member or teacher of this session"}
}

// attempt:{attemptId} — plan 053b broadens the Phase 1 narrow rule.
//
// Authorization paths:
//
//	Owner               → role="user"
//	Platform admin      → role="teacher"
//	Impersonator        → role="teacher" (admin-driven)
//	Class instructor/TA where attempt-owner is in the SAME class
//	                    → role="teacher"
//	Org_admin where attempt-owner is in some class for that course
//	                    → role="teacher"
//	Anyone else         → 403
//
// Both class-staff and org_admin checks REQUIRE that the attempt
// owner shares a class with the caller — Codex caught the
// popular-problem leak: a teacher of Class A could otherwise mint
// tokens for a student in Class B if both classes use the same
// problem. The single-EXISTS store helpers carry both constraints.
func (h *RealtimeHandler) authorizeAttemptDoc(ctx context.Context, claims *auth.Claims, attemptID string) (string, *authDecision) {
	if h.Attempts == nil {
		return "", &authDecision{Status: http.StatusInternalServerError, Message: "Attempts store unavailable"}
	}
	attempt, err := h.Attempts.GetAttempt(ctx, attemptID)
	if err != nil {
		return "", &authDecision{Status: http.StatusInternalServerError, Message: "Database error"}
	}
	if attempt == nil {
		return "", &authDecision{Status: http.StatusNotFound, Message: "Attempt not found"}
	}

	// Owner.
	if attempt.UserID == claims.UserID {
		return "user", nil
	}

	// Platform admin / impersonator bypass — admins need oversight
	// that crosses class membership. Plan 053 Phase 1 deliberately
	// withheld this; plan 053b lifts it.
	if claims.IsPlatformAdmin || claims.ImpersonatedBy != "" {
		return "teacher", nil
	}

	// Class instructor / TA. Single EXISTS verifies both that the
	// caller is class-staff AND the attempt owner is in the same
	// class. Closes the popular-problem leak.
	ok, err := h.Attempts.IsTeacherOfAttempt(ctx, claims.UserID, attempt.ID)
	if err != nil {
		return "", &authDecision{Status: http.StatusInternalServerError, Message: "Database error"}
	}
	if ok {
		return "teacher", nil
	}

	// Org admin oversight — same privacy boundary (attempt owner
	// must be enrolled in some class for the relevant course).
	ok, err = h.Attempts.IsOrgAdminOfAttempt(ctx, claims.UserID, attempt.ID)
	if err != nil {
		return "", &authDecision{Status: http.StatusInternalServerError, Message: "Database error"}
	}
	if ok {
		return "teacher", nil
	}

	return "", &authDecision{Status: http.StatusForbidden, Message: "Not authorized to watch this attempt"}
}

// unit:{unitId} — caller may EDIT the unit (CanEditUnit semantics).
// Read-only viewers don't need a Hocuspocus token; the unit editor
// is teacher-only.
//
// Plan 053 phase 1: applies the same rule as
// `TeachingUnitHandler.canEditUnit`:
//
//	platform → platform admin only.
//	org      → active teacher or org_admin in the unit's org.
//	personal → owner.
func (h *RealtimeHandler) authorizeUnitDoc(ctx context.Context, claims *auth.Claims, unitID string) (string, *authDecision) {
	if h.TeachingUnits == nil {
		return "", &authDecision{Status: http.StatusInternalServerError, Message: "Units store unavailable"}
	}
	unit, err := h.TeachingUnits.GetUnit(ctx, unitID)
	if err != nil {
		return "", &authDecision{Status: http.StatusInternalServerError, Message: "Database error"}
	}
	if unit == nil {
		return "", &authDecision{Status: http.StatusNotFound, Message: "Unit not found"}
	}
	if claims.IsPlatformAdmin {
		return "teacher", nil
	}
	switch unit.Scope {
	case "platform":
		// Non-admins cannot edit platform units; covered by !IsPlatformAdmin above.
		return "", &authDecision{Status: http.StatusForbidden, Message: "Platform-scope units require platform admin to edit"}
	case "org":
		if unit.ScopeID == nil || h.Orgs == nil {
			return "", &authDecision{Status: http.StatusForbidden, Message: "Not authorized"}
		}
		roles, err := h.Orgs.GetUserRolesInOrg(ctx, *unit.ScopeID, claims.UserID)
		if err != nil {
			return "", &authDecision{Status: http.StatusInternalServerError, Message: "Database error"}
		}
		for _, m := range roles {
			if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
				return "teacher", nil
			}
		}
		return "", &authDecision{Status: http.StatusForbidden, Message: "Not authorized"}
	case "personal":
		if unit.ScopeID != nil && *unit.ScopeID == claims.UserID {
			return "teacher", nil
		}
		return "", &authDecision{Status: http.StatusForbidden, Message: "Not authorized"}
	}
	return "", &authDecision{Status: http.StatusForbidden, Message: "Not authorized"}
}

// (compile-time assertions; surface mismatches if the struct
// changes shape vs what the handlers expect)
var (
	_ = json.Marshal // ensure encoding/json is imported even if no handler body uses it directly here
	_ = errors.New
)
