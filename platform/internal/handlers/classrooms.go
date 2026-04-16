package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type ClassroomHandler struct {
	Classrooms *store.ClassroomStore
	Classes    *store.ClassStore
	Sessions   *store.SessionStore
}

func (h *ClassroomHandler) Routes(r chi.Router) {
	r.Route("/api/classrooms", func(r chi.Router) {
		r.Get("/", h.ListClassrooms)
		r.Post("/", h.CreateClassroom)
		r.Post("/join", h.JoinClassroom)
		r.Get("/by-class/{classId}", h.GetClassroomByClass)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("id"))
			r.Get("/", h.GetClassroom)
			r.Get("/members", h.GetClassroomMembers)
			r.Get("/active-session", h.GetActiveSession)
		})
	})
}

// GetClassroomByClass handles GET /api/classrooms/by-class/{classId}
// Returns the new_classrooms record associated with a class (1:1 relationship).
func (h *ClassroomHandler) GetClassroomByClass(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "classId")
	classroom, err := h.Classes.GetClassroom(r.Context(), classID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if classroom == nil {
		writeError(w, http.StatusNotFound, "Classroom not found")
		return
	}
	writeJSON(w, http.StatusOK, classroom)
}

func (h *ClassroomHandler) ListClassrooms(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classrooms, err := h.Classrooms.ListClassrooms(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, classrooms)
}

func (h *ClassroomHandler) CreateClassroom(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		GradeLevel  string `json:"gradeLevel"`
		EditorMode  string `json:"editorMode"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Name == "" || len(body.Name) > 255 {
		writeError(w, http.StatusBadRequest, "name is required (max 255 chars)")
		return
	}
	if !validGradeLevels[body.GradeLevel] {
		writeError(w, http.StatusBadRequest, "gradeLevel must be K-5, 6-8, or 9-12")
		return
	}
	validEditorModes := map[string]bool{"blockly": true, "python": true, "javascript": true}
	if body.EditorMode != "" && !validEditorModes[body.EditorMode] {
		writeError(w, http.StatusBadRequest, "editorMode must be blockly, python, or javascript")
		return
	}
	if body.EditorMode == "" {
		body.EditorMode = "python"
	}

	classroom, err := h.Classrooms.CreateClassroom(r.Context(), store.CreateClassroomInput{
		TeacherID:   claims.UserID,
		Name:        body.Name,
		Description: body.Description,
		GradeLevel:  body.GradeLevel,
		EditorMode:  body.EditorMode,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create classroom")
		return
	}
	writeJSON(w, http.StatusCreated, classroom)
}

func (h *ClassroomHandler) GetClassroom(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classroom, err := h.Classrooms.GetClassroom(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if classroom == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, classroom)
}

func (h *ClassroomHandler) JoinClassroom(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		JoinCode string `json:"joinCode"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.JoinCode == "" {
		writeError(w, http.StatusBadRequest, "joinCode is required")
		return
	}

	classroom, err := h.Classrooms.GetClassroomByJoinCode(r.Context(), body.JoinCode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if classroom == nil {
		writeError(w, http.StatusNotFound, "Invalid join code")
		return
	}

	_, err = h.Classrooms.JoinClassroom(r.Context(), classroom.ID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, classroom)
}

func (h *ClassroomHandler) GetClassroomMembers(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classroomID := chi.URLParam(r, "id")
	classroom, err := h.Classrooms.GetClassroom(r.Context(), classroomID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if classroom == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	members, err := h.Classrooms.GetClassroomMembers(r.Context(), classroomID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *ClassroomHandler) GetActiveSession(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classroomID := chi.URLParam(r, "id")
	session, err := h.Sessions.GetActiveSession(r.Context(), classroomID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, session) // returns null if no active session
}
