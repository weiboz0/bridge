package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// AnnotationHandler — code feedback on student session docs.
//
// Plan 056: per-document authorization. Annotations target a
// `documentId` of the form `session:<sessionId>:user:<userId>`. The
// handler resolves the underlying session and applies role-aware
// access:
//
//	List   — doc owner OR session teacher OR class staff OR admin.
//	         Anyone else → 404 (don't leak existence).
//	Create / Delete / Resolve — TEACHER ONLY (session teacher OR
//	         class staff OR admin). Doc owner who CAN list gets 403
//	         on mutations. Anyone without read access gets 404.
type AnnotationHandler struct {
	Annotations *store.AnnotationStore
	Sessions    *store.SessionStore
	Classes     *store.ClassStore
	Orgs        *store.OrgStore
}

func (h *AnnotationHandler) Routes(r chi.Router) {
	r.Route("/api/annotations", func(r chi.Router) {
		r.Post("/", h.CreateAnnotation)
		r.Get("/", h.ListAnnotations)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("id"))
			r.Delete("/", h.DeleteAnnotation)
			r.Patch("/", h.ResolveAnnotation)
		})
	})
}

// annotationAccess captures the resolved authorization decision.
type annotationAccess struct {
	// canRead == true when the caller is the doc owner or has
	// class-roster authority. Determines List visibility.
	canRead bool
	// canMutate == true when the caller is the session teacher,
	// class staff, or platform admin. Required for Create / Delete
	// / Resolve.
	canMutate bool
}

// resolveAnnotationAccess parses the documentId, looks up the
// session, and returns an access decision. Returns an authDecision
// with the right HTTP status if anything is wrong (bad prefix → 400,
// missing session → 404, no read access → 404).
func (h *AnnotationHandler) resolveAnnotationAccess(ctx context.Context, claims *auth.Claims, documentID string) (annotationAccess, *authDecision) {
	// Plan 056 — explicitly reject any documentId that is not a
	// student session doc. attempt:*, unit:*, and broadcast:* are
	// realtime doc shapes that don't carry annotations.
	parts := strings.Split(documentID, ":")
	if len(parts) != 4 || parts[0] != "session" || parts[2] != "user" {
		return annotationAccess{}, &authDecision{
			Status:  http.StatusBadRequest,
			Message: "documentId must be of the form session:<sessionId>:user:<userId>",
		}
	}
	sessionID, ownerID := parts[1], parts[3]
	if sessionID == "" || ownerID == "" {
		return annotationAccess{}, &authDecision{Status: http.StatusBadRequest, Message: "documentId is malformed"}
	}

	if h.Sessions == nil {
		return annotationAccess{}, &authDecision{Status: http.StatusInternalServerError, Message: "Sessions store unavailable"}
	}
	session, err := h.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return annotationAccess{}, &authDecision{Status: http.StatusInternalServerError, Message: "Database error"}
	}
	if session == nil {
		// Don't leak the existence of an annotation on a missing
		// session — collapse to 404.
		return annotationAccess{}, &authDecision{Status: http.StatusNotFound, Message: "Not found"}
	}

	// Platform admin / session teacher → full access.
	if claims.IsPlatformAdmin {
		return annotationAccess{canRead: true, canMutate: true}, nil
	}
	if session.TeacherID == claims.UserID {
		return annotationAccess{canRead: true, canMutate: true}, nil
	}

	// Class staff path — instructor / TA / org_admin / platform
	// admin via the standard helper. AccessRoster is the right
	// level (matches grading and teacher-page patterns).
	if session.ClassID != nil && h.Classes != nil {
		if _, ok, err := RequireClassAuthority(ctx, h.Classes, h.Orgs, claims, *session.ClassID, AccessRoster); err == nil && ok {
			return annotationAccess{canRead: true, canMutate: true}, nil
		}
	}

	// Doc owner — student opening their own annotations. Can READ
	// but cannot MUTATE.
	if claims.UserID == ownerID {
		return annotationAccess{canRead: true, canMutate: false}, nil
	}

	// Everyone else — including students in the same class but
	// looking at someone else's doc — is denied. 404 not 403 so the
	// existence of the doc isn't leaked.
	return annotationAccess{}, &authDecision{Status: http.StatusNotFound, Message: "Not found"}
}

func (h *AnnotationHandler) CreateAnnotation(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		DocumentID string `json:"documentId"`
		LineStart  string `json:"lineStart"`
		LineEnd    string `json:"lineEnd"`
		Content    string `json:"content"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.DocumentID == "" {
		writeError(w, http.StatusBadRequest, "documentId is required")
		return
	}
	if body.LineStart == "" || body.LineEnd == "" {
		writeError(w, http.StatusBadRequest, "lineStart and lineEnd are required")
		return
	}
	if body.Content == "" || len(body.Content) > 2000 {
		writeError(w, http.StatusBadRequest, "content is required (max 2000 chars)")
		return
	}

	access, decision := h.resolveAnnotationAccess(r.Context(), claims, body.DocumentID)
	if decision != nil {
		writeError(w, decision.Status, decision.Message)
		return
	}
	if !access.canMutate {
		// Caller can read but not write — student on own doc
		// trying to create a teacher feedback annotation.
		writeError(w, http.StatusForbidden, "Annotations are teacher-only")
		return
	}

	annotation, err := h.Annotations.CreateAnnotation(r.Context(), store.CreateAnnotationInput{
		DocumentID: body.DocumentID,
		AuthorID:   claims.UserID,
		AuthorType: "teacher",
		LineStart:  body.LineStart,
		LineEnd:    body.LineEnd,
		Content:    body.Content,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create annotation")
		return
	}
	writeJSON(w, http.StatusCreated, annotation)
}

func (h *AnnotationHandler) ListAnnotations(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	documentID := r.URL.Query().Get("documentId")
	if documentID == "" {
		writeError(w, http.StatusBadRequest, "documentId query parameter is required")
		return
	}

	access, decision := h.resolveAnnotationAccess(r.Context(), claims, documentID)
	if decision != nil {
		writeError(w, decision.Status, decision.Message)
		return
	}
	if !access.canRead {
		// Defense-in-depth — resolveAnnotationAccess already returns
		// 404 for non-readers via authDecision; this branch should
		// be unreachable. Surface as 404 if it ever fires.
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	annotations, err := h.Annotations.ListAnnotations(r.Context(), documentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, annotations)
}

// loadAndAuthorizeMutation is shared by Delete and Resolve. Both
// take an annotation `id` (not a documentID), so we look up the
// row first, then authorize against its documentID.
func (h *AnnotationHandler) loadAndAuthorizeMutation(ctx context.Context, claims *auth.Claims, annotationID string) (*store.Annotation, *authDecision) {
	annotation, err := h.Annotations.GetAnnotation(ctx, annotationID)
	if err != nil {
		return nil, &authDecision{Status: http.StatusInternalServerError, Message: "Database error"}
	}
	if annotation == nil {
		return nil, &authDecision{Status: http.StatusNotFound, Message: "Not found"}
	}
	access, decision := h.resolveAnnotationAccess(ctx, claims, annotation.DocumentID)
	if decision != nil {
		return nil, decision
	}
	if !access.canMutate {
		// Reader (doc owner) trying to modify — 403, not 404.
		if access.canRead {
			return nil, &authDecision{Status: http.StatusForbidden, Message: "Annotations are teacher-only"}
		}
		return nil, &authDecision{Status: http.StatusNotFound, Message: "Not found"}
	}
	return annotation, nil
}

func (h *AnnotationHandler) DeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if _, decision := h.loadAndAuthorizeMutation(r.Context(), claims, chi.URLParam(r, "id")); decision != nil {
		writeError(w, decision.Status, decision.Message)
		return
	}

	deleted, err := h.Annotations.DeleteAnnotation(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if deleted == nil {
		// Race — annotation existed at lookup, gone by the time we
		// deleted. Treat as 404 (caller's intent to delete is met).
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (h *AnnotationHandler) ResolveAnnotation(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if _, decision := h.loadAndAuthorizeMutation(r.Context(), claims, chi.URLParam(r, "id")); decision != nil {
		writeError(w, decision.Status, decision.Message)
		return
	}

	resolved, err := h.Annotations.ResolveAnnotation(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if resolved == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, resolved)
}
