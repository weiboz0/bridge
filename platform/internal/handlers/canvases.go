package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// CanvasHandler serves the persisted whiteboard metadata surface. Store
// mutations lock the session row and re-check its status, so these handler
// checks provide response shaping rather than being the lifecycle boundary.
type CanvasHandler struct {
	Sessions *store.SessionStore
	Canvases *store.CanvasStore
}

func (h *CanvasHandler) Routes(r chi.Router) {
	r.With(ValidateUUIDParam("id")).Get("/api/sessions/{id}/canvases", h.ListCanvases)
	r.With(ValidateUUIDParam("id")).Post("/api/sessions/{id}/canvases", h.CreateCanvas)
	r.With(ValidateUUIDParam("id")).Get("/api/sessions/{id}/canvas-settings", h.GetCanvasSettings)
	r.With(ValidateUUIDParam("id")).Patch("/api/sessions/{id}/canvas-settings", h.PatchCanvasSettings)
	r.With(ValidateUUIDParam("id"), ValidateUUIDParam("canvasID")).Patch("/api/sessions/{id}/canvases/{canvasID}", h.UpdateCanvas)
	r.With(ValidateUUIDParam("id"), ValidateUUIDParam("canvasID")).Delete("/api/sessions/{id}/canvases/{canvasID}", h.DeleteCanvas)
}

func (h *CanvasHandler) sessionForMutation(w http.ResponseWriter, r *http.Request) (*store.LiveSession, bool) {
	if h.Sessions == nil || h.Canvases == nil {
		writeError(w, http.StatusInternalServerError, "Canvas handler misconfigured")
		return nil, false
	}
	session, err := h.Sessions.GetSession(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil, false
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return nil, false
	}
	if session.Status == "ended" {
		writeError(w, http.StatusConflict, "Session has ended")
		return nil, false
	}
	return session, true
}

func (h *CanvasHandler) CreateCanvas(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body struct {
		Title      string `json:"title"`
		Visibility string `json:"visibility"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Title == "" || !canvasVisibility(body.Visibility) {
		writeError(w, http.StatusBadRequest, "title and supported visibility are required")
		return
	}
	canvas, err := h.Canvases.CreateCanvas(r.Context(), store.CreateCanvasInput{SessionID: chi.URLParam(r, "id"), OwnerID: claims.UserID, Title: body.Title, Visibility: body.Visibility})
	if err != nil {
		h.writeCanvasMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, canvas)
}

// GetCanvasSettings has an intentionally exact, teacher-only response. It is
// available after end so the archive can display the durable completeness
// outcome, but never grants a non-teacher an archive-status oracle.
func (h *CanvasHandler) GetCanvasSettings(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.Sessions == nil || h.Canvases == nil {
		writeError(w, http.StatusInternalServerError, "Canvas handler misconfigured")
		return
	}
	session, err := h.Sessions.GetSession(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}
	if session.TeacherID != claims.UserID {
		writeError(w, http.StatusForbidden, "Not authorized")
		return
	}
	settings, err := h.Canvases.GetCanvasSettings(r.Context(), session.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if settings == nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}
	response := map[string]any{"canvasFloor": settings.CanvasFloor}
	if settings.WhiteboardServerArchiveComplete != nil {
		response["whiteboardServerArchiveComplete"] = *settings.WhiteboardServerArchiveComplete
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *CanvasHandler) ListCanvases(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.Canvases == nil {
		writeError(w, http.StatusInternalServerError, "Canvas handler misconfigured")
		return
	}
	canvases, err := h.Canvases.ListVisibleCanvases(r.Context(), chi.URLParam(r, "id"), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": canvases})
}

func (h *CanvasHandler) UpdateCanvas(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if _, ok := h.sessionForMutation(w, r); !ok {
		return
	}
	canvas, err := h.Canvases.GetCanvas(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "canvasID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if canvas == nil {
		writeError(w, http.StatusNotFound, "Canvas not found")
		return
	}
	if canvas.OwnerID != claims.UserID {
		writeError(w, http.StatusForbidden, "Not authorized")
		return
	}
	var body struct {
		Title      *string `json:"title"`
		Visibility *string `json:"visibility"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Title == nil && body.Visibility == nil {
		writeError(w, http.StatusBadRequest, "title or visibility is required")
		return
	}
	if body.Visibility != nil && !canvasVisibility(*body.Visibility) {
		writeError(w, http.StatusBadRequest, "unsupported canvas visibility")
		return
	}
	updated, err := h.Canvases.UpdateCanvas(r.Context(), chi.URLParam(r, "id"), canvas.ID, claims.UserID, body.Title, body.Visibility)
	if err != nil {
		h.writeCanvasMutationError(w, err)
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "Canvas not found")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *CanvasHandler) DeleteCanvas(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if _, ok := h.sessionForMutation(w, r); !ok {
		return
	}
	canvas, err := h.Canvases.GetCanvas(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "canvasID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if canvas == nil {
		writeError(w, http.StatusNotFound, "Canvas not found")
		return
	}
	if canvas.OwnerID != claims.UserID {
		writeError(w, http.StatusForbidden, "Not authorized")
		return
	}
	deleted, err := h.Canvases.DeleteCanvas(r.Context(), chi.URLParam(r, "id"), canvas.ID, claims.UserID)
	if err != nil {
		h.writeCanvasMutationError(w, err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "Canvas not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CanvasHandler) PatchCanvasSettings(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.Sessions == nil || h.Canvases == nil {
		writeError(w, http.StatusInternalServerError, "Canvas handler misconfigured")
		return
	}
	session, err := h.Sessions.GetSession(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}
	if session.TeacherID != claims.UserID {
		writeError(w, http.StatusForbidden, "Not authorized")
		return
	}
	if session.Status == "ended" {
		writeError(w, http.StatusConflict, "Session has ended")
		return
	}
	var body struct {
		CanvasFloor string `json:"canvasFloor"`
	}
	if !decodeJSONStrict(w, r, &body) {
		return
	}
	if body.CanvasFloor != "private" && body.CanvasFloor != "host" && body.CanvasFloor != "participants" {
		writeError(w, http.StatusBadRequest, "unsupported canvas floor")
		return
	}
	floor, err := h.Canvases.SetSessionCanvasFloor(r.Context(), chi.URLParam(r, "id"), claims.UserID, body.CanvasFloor)
	if err != nil {
		h.writeCanvasMutationError(w, err)
		return
	}
	if floor == "" {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"canvasFloor": floor})
}

func canvasVisibility(visibility string) bool {
	switch visibility {
	case "private", "host", "participants", "session":
		return true
	default:
		return false
	}
}

func (h *CanvasHandler) writeCanvasMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrSessionEndInProgress):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Session end in progress", "code": "session_end_in_progress"})
	case errors.Is(err, store.ErrSessionEnded):
		writeError(w, http.StatusConflict, "Session has ended")
	case errors.Is(err, store.ErrCanvasCapReached):
		writeError(w, http.StatusConflict, "Session canvas cap reached")
	case errors.Is(err, store.ErrCanvasVisibilityTighten), errors.Is(err, store.ErrCanvasBelowFloor), errors.Is(err, store.ErrCanvasFloorTooLoose), errors.Is(err, store.ErrCanvasTitleRequired), errors.Is(err, store.ErrCanvasTitleTooLong):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrCanvasFloorUnauthorized):
		writeError(w, http.StatusForbidden, "Not authorized")
	case errors.Is(err, store.ErrCanvasCreatorUnauthorized):
		writeError(w, http.StatusForbidden, "Not authorized")
	default:
		writeError(w, http.StatusInternalServerError, "Database error")
	}
}
