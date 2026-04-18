package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// ProblemHandler serves three related resources that share access-control
// helpers: problems, test_cases, and attempts.
type ProblemHandler struct {
	Problems  *store.ProblemStore
	TestCases *store.TestCaseStore
	Attempts  *store.AttemptStore
	Topics    *store.TopicStore
	Courses   *store.CourseStore
}

var validProblemLanguages = map[string]bool{
	"python": true, "javascript": true, "blockly": true,
}

func (h *ProblemHandler) Routes(r chi.Router) {
	// Under a topic: list + create problems
	r.Route("/api/topics/{topicId}/problems", func(r chi.Router) {
		r.Use(ValidateUUIDParam("topicId"))
		r.Get("/", h.ListProblems)
		r.Post("/", h.CreateProblem)
	})

	// Individual problem + nested resources
	r.Route("/api/problems/{id}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Get("/", h.GetProblem)
		r.Patch("/", h.UpdateProblem)
		r.Delete("/", h.DeleteProblem)

		r.Get("/test-cases", h.ListTestCases)
		r.Post("/test-cases", h.CreateTestCase)
		r.Get("/attempts", h.ListAttempts)
		r.Post("/attempts", h.CreateAttempt)
	})

	r.Route("/api/test-cases/{id}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Patch("/", h.UpdateTestCase)
		r.Delete("/", h.DeleteTestCase)
	})

	r.Route("/api/attempts/{id}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Get("/", h.GetAttempt)
		r.Patch("/", h.UpdateAttempt)
		r.Delete("/", h.DeleteAttempt)
	})
}

// ---------- access helpers ----------

// canViewTopic returns true if the caller has read access to the topic's
// course (creator, platform admin, or active class member in the course).
func (h *ProblemHandler) canViewTopic(r *http.Request, topicID string, claims *auth.Claims) (bool, int, error) {
	if claims.IsPlatformAdmin {
		return true, 0, nil
	}
	topic, err := h.Topics.GetTopic(r.Context(), topicID)
	if err != nil {
		return false, http.StatusInternalServerError, err
	}
	if topic == nil {
		return false, http.StatusNotFound, nil
	}
	course, err := h.Courses.GetCourse(r.Context(), topic.CourseID)
	if err != nil {
		return false, http.StatusInternalServerError, err
	}
	if course == nil {
		return false, http.StatusNotFound, nil
	}
	if course.CreatedBy == claims.UserID {
		return true, 0, nil
	}
	hasAccess, err := h.Courses.UserHasAccessToCourse(r.Context(), course.ID, claims.UserID)
	if err != nil {
		return false, http.StatusInternalServerError, err
	}
	return hasAccess, 0, nil
}

// canViewProblem returns (canView, course, problem). The returned course
// lets the caller check author rights without a second lookup.
func (h *ProblemHandler) canViewProblem(r *http.Request, problemID string, claims *auth.Claims) (bool, *store.Problem, *store.Course, int, error) {
	p, err := h.Problems.GetProblem(r.Context(), problemID)
	if err != nil {
		return false, nil, nil, http.StatusInternalServerError, err
	}
	if p == nil {
		return false, nil, nil, http.StatusNotFound, nil
	}
	if claims.IsPlatformAdmin || p.CreatedBy == claims.UserID {
		return true, p, nil, 0, nil
	}
	topic, err := h.Topics.GetTopic(r.Context(), p.TopicID)
	if err != nil {
		return false, nil, nil, http.StatusInternalServerError, err
	}
	if topic == nil {
		return false, nil, nil, http.StatusNotFound, nil
	}
	course, err := h.Courses.GetCourse(r.Context(), topic.CourseID)
	if err != nil {
		return false, nil, nil, http.StatusInternalServerError, err
	}
	if course == nil {
		return false, nil, nil, http.StatusNotFound, nil
	}
	if course.CreatedBy == claims.UserID {
		return true, p, course, 0, nil
	}
	hasAccess, err := h.Courses.UserHasAccessToCourse(r.Context(), course.ID, claims.UserID)
	if err != nil {
		return false, nil, nil, http.StatusInternalServerError, err
	}
	return hasAccess, p, course, 0, nil
}

// canAuthorProblem returns true if the caller can create/modify canonical
// content under a problem (the problem's creator, the course's creator,
// or a platform admin).
func (h *ProblemHandler) canAuthorProblem(r *http.Request, p *store.Problem, claims *auth.Claims) (bool, int, error) {
	if claims.IsPlatformAdmin || p.CreatedBy == claims.UserID {
		return true, 0, nil
	}
	topic, err := h.Topics.GetTopic(r.Context(), p.TopicID)
	if err != nil {
		return false, http.StatusInternalServerError, err
	}
	if topic == nil {
		return false, http.StatusNotFound, nil
	}
	course, err := h.Courses.GetCourse(r.Context(), topic.CourseID)
	if err != nil {
		return false, http.StatusInternalServerError, err
	}
	if course == nil {
		return false, http.StatusNotFound, nil
	}
	return course.CreatedBy == claims.UserID, 0, nil
}

// ---------- Problem handlers ----------

func (h *ProblemHandler) ListProblems(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	topicID := chi.URLParam(r, "topicId")

	canView, status, err := h.canViewTopic(r, topicID, claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !canView {
		if status == http.StatusNotFound {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	list, err := h.Problems.ListProblemsByTopic(r.Context(), topicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *ProblemHandler) CreateProblem(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	topicID := chi.URLParam(r, "topicId")

	// Only the course creator (or platform admin) may author problems.
	topic, err := h.Topics.GetTopic(r.Context(), topicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if topic == nil {
		writeError(w, http.StatusNotFound, "Topic not found")
		return
	}
	course, err := h.Courses.GetCourse(r.Context(), topic.CourseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if course == nil {
		writeError(w, http.StatusNotFound, "Course not found")
		return
	}
	if !claims.IsPlatformAdmin && course.CreatedBy != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the course creator can add problems")
		return
	}

	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		StarterCode string `json:"starterCode"`
		Language    string `json:"language"`
		Order       int    `json:"order"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Title == "" || len(body.Title) > 255 {
		writeError(w, http.StatusBadRequest, "title is required (max 255 chars)")
		return
	}
	if body.Language == "" {
		body.Language = course.Language
	}
	if !validProblemLanguages[body.Language] {
		writeError(w, http.StatusBadRequest, "language must be python, javascript, or blockly")
		return
	}

	p, err := h.Problems.CreateProblem(r.Context(), store.CreateProblemInput{
		TopicID:     topicID,
		CreatedBy:   claims.UserID,
		Title:       body.Title,
		Description: body.Description,
		StarterCode: body.StarterCode,
		Language:    body.Language,
		Order:       body.Order,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create problem")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *ProblemHandler) GetProblem(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	canView, p, _, status, err := h.canViewProblem(r, chi.URLParam(r, "id"), claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !canView {
		if status == http.StatusNotFound {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *ProblemHandler) UpdateProblem(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	problemID := chi.URLParam(r, "id")
	p, err := h.Problems.GetProblem(r.Context(), problemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	canAuthor, _, err := h.canAuthorProblem(r, p, claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !canAuthor {
		writeError(w, http.StatusForbidden, "Only the problem creator can update")
		return
	}

	var body store.UpdateProblemInput
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Title != nil && (*body.Title == "" || len(*body.Title) > 255) {
		writeError(w, http.StatusBadRequest, "title must be 1-255 chars")
		return
	}
	if body.Language != nil && !validProblemLanguages[*body.Language] {
		writeError(w, http.StatusBadRequest, "language must be python, javascript, or blockly")
		return
	}

	updated, err := h.Problems.UpdateProblem(r.Context(), problemID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *ProblemHandler) DeleteProblem(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	problemID := chi.URLParam(r, "id")
	p, err := h.Problems.GetProblem(r.Context(), problemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	canAuthor, _, err := h.canAuthorProblem(r, p, claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !canAuthor {
		writeError(w, http.StatusForbidden, "Only the problem creator can delete")
		return
	}

	deleted, err := h.Problems.DeleteProblem(r.Context(), problemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

// ---------- TestCase handlers ----------

// ListTestCases returns canonical-example cases + the caller's private cases
// to a regular viewer. To the problem author (or platform admin), hidden
// canonical cases are also returned so the author can edit them.
func (h *ProblemHandler) ListTestCases(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	canView, p, _, status, err := h.canViewProblem(r, chi.URLParam(r, "id"), claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !canView {
		if status == http.StatusNotFound {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	all, err := h.TestCases.ListForViewer(r.Context(), p.ID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	canAuthor, _, err := h.canAuthorProblem(r, p, claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if canAuthor {
		writeJSON(w, http.StatusOK, all)
		return
	}

	out := make([]store.TestCase, 0, len(all))
	for _, c := range all {
		if c.OwnerID == nil && !c.IsExample {
			continue // hidden canonical: redact from non-author
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ProblemHandler) CreateTestCase(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	canView, p, _, status, err := h.canViewProblem(r, chi.URLParam(r, "id"), claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !canView {
		if status == http.StatusNotFound {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var body struct {
		Name           string  `json:"name"`
		Stdin          string  `json:"stdin"`
		ExpectedStdout *string `json:"expectedStdout"`
		IsExample      bool    `json:"isExample"`
		Order          int     `json:"order"`
		IsCanonical    bool    `json:"isCanonical"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	input := store.CreateTestCaseInput{
		ProblemID:      p.ID,
		Name:           body.Name,
		Stdin:          body.Stdin,
		ExpectedStdout: body.ExpectedStdout,
		IsExample:      body.IsExample,
		Order:          body.Order,
	}
	if body.IsCanonical {
		canAuthor, _, err := h.canAuthorProblem(r, p, claims)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if !canAuthor {
			writeError(w, http.StatusForbidden, "Only the problem creator can add canonical test cases")
			return
		}
		// owner_id stays NULL
	} else {
		input.OwnerID = &claims.UserID
	}

	c, err := h.TestCases.CreateTestCase(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create test case")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// UpdateTestCase / DeleteTestCase share a guard: owner-of-case for private,
// problem-author for canonical.
func (h *ProblemHandler) testCaseGuard(w http.ResponseWriter, r *http.Request, caseID string, claims *auth.Claims) *store.TestCase {
	c, err := h.TestCases.GetTestCase(r.Context(), caseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil
	}
	if c == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return nil
	}
	if c.OwnerID != nil {
		if *c.OwnerID != claims.UserID && !claims.IsPlatformAdmin {
			writeError(w, http.StatusNotFound, "Not found") // don't leak private case existence
			return nil
		}
		return c
	}
	// Canonical — check problem author rights.
	p, err := h.Problems.GetProblem(r.Context(), c.ProblemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return nil
	}
	canAuthor, _, err := h.canAuthorProblem(r, p, claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil
	}
	if !canAuthor {
		writeError(w, http.StatusForbidden, "Only the problem creator can modify canonical cases")
		return nil
	}
	return c
}

func (h *ProblemHandler) UpdateTestCase(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	c := h.testCaseGuard(w, r, chi.URLParam(r, "id"), claims)
	if c == nil {
		return
	}
	var body store.UpdateTestCaseInput
	if !decodeJSON(w, r, &body) {
		return
	}
	updated, err := h.TestCases.UpdateTestCase(r.Context(), c.ID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *ProblemHandler) DeleteTestCase(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	c := h.testCaseGuard(w, r, chi.URLParam(r, "id"), claims)
	if c == nil {
		return
	}
	deleted, err := h.TestCases.DeleteTestCase(r.Context(), c.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

// ---------- Attempt handlers ----------

func (h *ProblemHandler) ListAttempts(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	canView, p, _, status, err := h.canViewProblem(r, chi.URLParam(r, "id"), claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !canView {
		if status == http.StatusNotFound {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}
	list, err := h.Attempts.ListByUserAndProblem(r.Context(), p.ID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *ProblemHandler) CreateAttempt(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	canView, p, _, status, err := h.canViewProblem(r, chi.URLParam(r, "id"), claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !canView {
		if status == http.StatusNotFound {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var body struct {
		Title     string `json:"title"`
		Language  string `json:"language"`
		PlainText string `json:"plainText"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Language == "" {
		body.Language = p.Language
	}
	a, err := h.Attempts.CreateAttempt(r.Context(), store.CreateAttemptInput{
		ProblemID: p.ID,
		UserID:    claims.UserID,
		Title:     body.Title,
		Language:  body.Language,
		PlainText: body.PlainText,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create attempt")
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// ownerOnlyAttempt loads an attempt and verifies the caller is the owner
// (or platform admin). Returns 404 for non-owners to avoid leaking existence.
func (h *ProblemHandler) ownerOnlyAttempt(w http.ResponseWriter, r *http.Request, attemptID string, claims *auth.Claims) *store.Attempt {
	a, err := h.Attempts.GetAttempt(r.Context(), attemptID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil
	}
	if a == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return nil
	}
	if a.UserID != claims.UserID && !claims.IsPlatformAdmin {
		writeError(w, http.StatusNotFound, "Not found")
		return nil
	}
	return a
}

func (h *ProblemHandler) GetAttempt(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	a := h.ownerOnlyAttempt(w, r, chi.URLParam(r, "id"), claims)
	if a == nil {
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *ProblemHandler) UpdateAttempt(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	a := h.ownerOnlyAttempt(w, r, chi.URLParam(r, "id"), claims)
	if a == nil {
		return
	}
	var body store.UpdateAttemptInput
	if !decodeJSON(w, r, &body) {
		return
	}
	updated, err := h.Attempts.UpdateAttempt(r.Context(), a.ID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *ProblemHandler) DeleteAttempt(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	a := h.ownerOnlyAttempt(w, r, chi.URLParam(r, "id"), claims)
	if a == nil {
		return
	}
	deleted, err := h.Attempts.DeleteAttempt(r.Context(), a.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}
