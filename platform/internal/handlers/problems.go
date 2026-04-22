package handlers

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// ProblemHandler serves the problem bank and its nested test-case/attempt
// resources. Access control is scope-based: a problem belongs to one of
// "platform", "org", or "personal", and the handler enforces who can read /
// edit / publish / fork based on that scope plus the viewer's org memberships.
type ProblemHandler struct {
	Problems      *store.ProblemStore
	TestCases     *store.TestCaseStore
	Attempts      *store.AttemptStore
	Solutions     *store.ProblemSolutionStore
	TopicProblems *store.TopicProblemStore
	Topics        *store.TopicStore
	Courses       *store.CourseStore
	Orgs          *store.OrgStore
}

var (
	validProblemScopes       = map[string]bool{"platform": true, "org": true, "personal": true}
	validProblemDifficulties = map[string]bool{"easy": true, "medium": true, "hard": true}
	validProblemGradeLevels  = map[string]bool{"K-5": true, "6-8": true, "9-12": true}
)

type problemListResponse struct {
	Items      []store.Problem `json:"items"`
	NextCursor *string         `json:"nextCursor,omitempty"`
}

// maxProblemTitleLen mirrors the DB column limit (varchar(255)).
const maxProblemTitleLen = 255

// maxProblemTagLen caps the length of each tag string so callers can't stuff
// arbitrary prose into the tags column.
const maxProblemTagLen = 64

func (h *ProblemHandler) Routes(r chi.Router) {
	r.Route("/api/problems", func(r chi.Router) {
		r.Get("/", h.ListProblems)
		r.Post("/", h.CreateProblem)
	})

	r.Route("/api/problems/{id}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Get("/", h.GetProblem)
		r.Patch("/", h.UpdateProblem)
		r.Delete("/", h.DeleteProblem)

		r.Post("/publish", h.PublishProblem)
		r.Post("/archive", h.ArchiveProblem)
		r.Post("/unarchive", h.UnarchiveProblem)
		r.Post("/fork", h.ForkProblem)

		r.Get("/test-cases", h.ListTestCases)
		r.Post("/test-cases", h.CreateTestCase)
		r.Get("/attempts", h.ListAttempts)
		r.Post("/attempts", h.CreateAttempt)
	})

	r.Route("/api/topics/{topicId}/problems", func(r chi.Router) {
		r.Use(ValidateUUIDParam("topicId"))
		r.Get("/", h.ListProblemsByTopic)
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

// accessDeps constructs a problemAccessDeps from the handler's fields.
func (h *ProblemHandler) accessDeps() problemAccessDeps {
	return problemAccessDeps{
		Problems:      h.Problems,
		TopicProblems: h.TopicProblems,
		Topics:        h.Topics,
		Courses:       h.Courses,
		Orgs:          h.Orgs,
	}
}

// ---------- query helpers ----------

// orgIDs extracts the OrgIDs from a slice of UserMembershipWithOrg. Used to
// populate ListProblemsFilter.ViewerOrgs.
func orgIDs(ms []store.UserMembershipWithOrg) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		if m.Status == "active" {
			out = append(out, m.OrgID)
		}
	}
	return out
}

// encodeCursor packs (createdAt, id) into a base64url string suitable for
// echoing back to the client. The format is an intentionally-opaque
// "createdAt|id" pair — we may add a version prefix later.
func encodeCursor(createdAt time.Time, id string) string {
	s := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// decodeCursor reverses encodeCursor. Returns (nil, nil) for empty input.
// Malformed cursors return an error — callers should map to 400.
func decodeCursor(raw string) (*time.Time, *string, error) {
	if raw == "" {
		return nil, nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, nil, err
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return nil, nil, errors.New("cursor: expected createdAt|id")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, nil, err
	}
	id := parts[1]
	return &t, &id, nil
}

// parseListFilterFromQuery pulls the supported query params (scope,
// difficulty, gradeLevel, tags, q, limit, cursor) off the request URL into a
// store.ListProblemsFilter. Viewer fields (ViewerID, ViewerOrgs,
// IsPlatformAdmin) are NOT populated here — the handler fills them from
// claims. Unknown params are silently ignored.
func parseListFilterFromQuery(r *http.Request) (store.ListProblemsFilter, error) {
	q := r.URL.Query()
	f := store.ListProblemsFilter{
		Scope:      q.Get("scope"),
		Difficulty: q.Get("difficulty"),
		GradeLevel: q.Get("gradeLevel"),
		Search:     q.Get("q"),
		Status:     q.Get("status"),
	}
	if s := q.Get("scopeId"); s != "" {
		f.ScopeID = &s
	}
	if tagsRaw := q.Get("tags"); tagsRaw != "" {
		tags := []string{}
		for _, t := range strings.Split(tagsRaw, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
		f.Tags = tags
	}
	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil {
			return f, errors.New("limit must be an integer")
		}
		f.Limit = n
	}
	if c := q.Get("cursor"); c != "" {
		ts, id, err := decodeCursor(c)
		if err != nil {
			return f, err
		}
		f.CursorCreatedAt = ts
		f.CursorID = id
	}
	return f, nil
}

// ---------- Problem handlers ----------

// ListProblems — GET /api/problems?scope=&difficulty=&gradeLevel=&tags=a,b&q=&limit=&cursor=
// Returns the accessible browse/search set plus an opaque nextCursor when
// another page exists.
func (h *ProblemHandler) ListProblems(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	f, err := parseListFilterFromQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid query: "+err.Error())
		return
	}
	if f.Scope != "" && !validProblemScopes[f.Scope] {
		writeError(w, http.StatusBadRequest, "scope must be platform, org, or personal")
		return
	}

	f.ViewerID = claims.UserID
	f.IsPlatformAdmin = claims.IsPlatformAdmin
	orgs, err := h.Orgs.GetUserMemberships(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	f.ViewerOrgs = orgIDs(orgs)

	list, hasMore, err := h.Problems.ListProblems(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	var nextCursor *string
	if hasMore && len(list) > 0 {
		last := list[len(list)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		nextCursor = &c
	}
	writeJSON(w, http.StatusOK, problemListResponse{
		Items:      list,
		NextCursor: nextCursor,
	})
}

// ListProblemsByTopic — GET /api/topics/{topicId}/problems.
// Returns the attached problems (in sort_order) for a topic the caller can view.
func (h *ProblemHandler) ListProblemsByTopic(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	topicID := chi.URLParam(r, "topicId")

	canView, status, err := canViewTopic(r.Context(), h.accessDeps(), claims, topicID)
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

// CreateProblem — POST /api/problems.
// Authorization depends on the requested scope (see authorizedForScope).
func (h *ProblemHandler) CreateProblem(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body struct {
		Scope         string            `json:"scope"`
		ScopeID       *string           `json:"scopeId"`
		Title         string            `json:"title"`
		Description   string            `json:"description"`
		StarterCode   map[string]string `json:"starterCode"`
		Difficulty    string            `json:"difficulty"`
		GradeLevel    *string           `json:"gradeLevel"`
		Tags          []string          `json:"tags"`
		TimeLimitMs   *int              `json:"timeLimitMs"`
		MemoryLimitMb *int              `json:"memoryLimitMb"`
		Slug          *string           `json:"slug"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if !validProblemScopes[body.Scope] {
		writeError(w, http.StatusBadRequest, "scope must be platform, org, or personal")
		return
	}
	if body.Title == "" || len(body.Title) > maxProblemTitleLen {
		writeError(w, http.StatusBadRequest, "title is required (max 255 chars)")
		return
	}
	if body.Difficulty != "" && !validProblemDifficulties[body.Difficulty] {
		writeError(w, http.StatusBadRequest, "difficulty must be easy, medium, or hard")
		return
	}
	if body.GradeLevel != nil && *body.GradeLevel != "" && !validProblemGradeLevels[*body.GradeLevel] {
		writeError(w, http.StatusBadRequest, "gradeLevel must be K-5, 6-8, or 9-12")
		return
	}
	for _, t := range body.Tags {
		if len(t) > maxProblemTagLen {
			writeError(w, http.StatusBadRequest, "tag exceeds 64-char limit")
			return
		}
	}

	if !authorizedForScope(r.Context(), h.accessDeps(), claims, body.Scope, body.ScopeID) {
		writeError(w, http.StatusForbidden, "not authorized for scope")
		return
	}

	p, err := h.Problems.CreateProblem(r.Context(), store.CreateProblemInput{
		Scope:         body.Scope,
		ScopeID:       body.ScopeID,
		Title:         body.Title,
		Slug:          body.Slug,
		Description:   body.Description,
		StarterCode:   body.StarterCode,
		Difficulty:    body.Difficulty,
		GradeLevel:    body.GradeLevel,
		Tags:          body.Tags,
		Status:        "draft",
		TimeLimitMs:   body.TimeLimitMs,
		MemoryLimitMb: body.MemoryLimitMb,
		CreatedBy:     claims.UserID,
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
	canView, p, status := canViewProblem(r.Context(), h.accessDeps(), claims, chi.URLParam(r, "id"))
	if status == http.StatusInternalServerError {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !canView {
		writeError(w, http.StatusNotFound, "Not found") // don't leak existence
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
	if !authorizedForProblemEditRow(r.Context(), h.accessDeps(), claims, p) {
		writeError(w, http.StatusForbidden, "Not authorized to edit")
		return
	}

	var body store.UpdateProblemInput
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Title != nil && (*body.Title == "" || len(*body.Title) > maxProblemTitleLen) {
		writeError(w, http.StatusBadRequest, "title must be 1-255 chars")
		return
	}
	if body.Difficulty != nil && *body.Difficulty != "" && !validProblemDifficulties[*body.Difficulty] {
		writeError(w, http.StatusBadRequest, "difficulty must be easy, medium, or hard")
		return
	}
	if body.GradeLevel != nil && *body.GradeLevel != "" && !validProblemGradeLevels[*body.GradeLevel] {
		writeError(w, http.StatusBadRequest, "gradeLevel must be K-5, 6-8, or 9-12")
		return
	}
	for _, t := range body.Tags {
		if len(t) > maxProblemTagLen {
			writeError(w, http.StatusBadRequest, "tag exceeds 64-char limit")
			return
		}
	}

	updated, err := h.Problems.UpdateProblem(r.Context(), problemID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteProblem — strict delete: fails 409 if the problem is attached to any
// topic OR has any attempts. Callers must detach / let attempts be deleted
// first.
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
	if !authorizedForProblemEditRow(r.Context(), h.accessDeps(), claims, p) {
		writeError(w, http.StatusForbidden, "Not authorized to delete")
		return
	}

	topics, err := h.TopicProblems.ListTopicsByProblem(r.Context(), problemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if len(topics) > 0 {
		writeError(w, http.StatusConflict, "problem is attached to topics")
		return
	}
	n, err := h.Attempts.CountAttemptsByProblem(r.Context(), problemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, "problem has attempts")
		return
	}

	if _, err := h.Problems.DeleteProblem(r.Context(), problemID); err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- Lifecycle handlers ----------

// PublishProblem — POST /api/problems/{id}/publish. Moves draft (or archived)
// → published. Invalid transitions → 409.
func (h *ProblemHandler) PublishProblem(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, "published", "already published")
}

// ArchiveProblem — POST /api/problems/{id}/archive. Moves published → archived.
func (h *ProblemHandler) ArchiveProblem(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, "archived", "cannot archive from current status")
}

// UnarchiveProblem — POST /api/problems/{id}/unarchive. Moves archived →
// published. ("unarchive" is just a second entry point to the "published"
// status.)
func (h *ProblemHandler) UnarchiveProblem(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, "published", "cannot unarchive — not archived")
}

func (h *ProblemHandler) setStatus(w http.ResponseWriter, r *http.Request, target, conflictMsg string) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	if !authorizedForProblemEdit(r.Context(), h.accessDeps(), claims, id) {
		writeError(w, http.StatusForbidden, "Not authorized")
		return
	}
	p, err := h.Problems.SetStatus(r.Context(), id, target)
	switch {
	case errors.Is(err, store.ErrInvalidTransition):
		writeError(w, http.StatusConflict, conflictMsg)
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	case p == nil:
		// SetStatus returns (nil, nil) for a missing row; the edit-auth check
		// above already loaded the problem once, so this is a race (deleted
		// between calls) — treat as 404.
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ForkProblem — POST /api/problems/{id}/fork.
// Body: { targetScope?, targetScopeId?, title? }. If targetScope is empty, the
// default target is inferred from the caller's org memberships: exactly one
// org → "org" / that orgID; otherwise "personal" / callerID.
func (h *ProblemHandler) ForkProblem(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	sourceID := chi.URLParam(r, "id")

	var body struct {
		TargetScope   string  `json:"targetScope"`
		TargetScopeID *string `json:"targetScopeId"`
		Title         *string `json:"title"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	// Default target inference.
	if body.TargetScope == "" {
		orgs, err := h.Orgs.GetUserMemberships(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		activeOrgs := orgIDs(orgs)
		if len(activeOrgs) == 1 {
			body.TargetScope = "org"
			orgID := activeOrgs[0]
			body.TargetScopeID = &orgID
		} else {
			body.TargetScope = "personal"
			uid := claims.UserID
			body.TargetScopeID = &uid
		}
	}

	if !validProblemScopes[body.TargetScope] {
		writeError(w, http.StatusBadRequest, "targetScope must be platform, org, or personal")
		return
	}

	// Source must be visible to the caller. We hide existence via 404 for
	// unauthorized callers (matches GetProblem behavior).
	canView, p, status := canViewProblem(r.Context(), h.accessDeps(), claims, sourceID)
	if status == http.StatusInternalServerError {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil || !canView {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	if !authorizedForScope(r.Context(), h.accessDeps(), claims, body.TargetScope, body.TargetScopeID) {
		writeError(w, http.StatusForbidden, "not authorized for target scope")
		return
	}

	newP, err := h.Problems.ForkProblem(r.Context(), sourceID, store.ForkTarget{
		Scope:    body.TargetScope,
		ScopeID:  body.TargetScopeID,
		Title:    body.Title,
		CallerID: claims.UserID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fork problem")
		return
	}
	if newP == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusCreated, newP)
}

// ---------- TestCase handlers ----------

// ListTestCases returns all test cases visible to the caller. Editors see full
// content for every case. Non-editors receive hidden canonical cases (owner_id
// IS NULL and is_example = false) with Stdin and ExpectedStdout blanked out
// so the client knows the case exists without being able to read the secret
// I/O.
func (h *ProblemHandler) ListTestCases(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	canView, p, status := canViewProblem(r.Context(), h.accessDeps(), claims, chi.URLParam(r, "id"))
	if status == http.StatusInternalServerError {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !canView {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	all, err := h.TestCases.ListForViewer(r.Context(), p.ID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	isEditor := authorizedForProblemEditRow(r.Context(), h.accessDeps(), claims, p)
	if !isEditor {
		// Shell out hidden canonical cases: include the row but blank the I/O
		// so students know hidden tests exist without leaking the test data.
		for i := range all {
			if all[i].OwnerID == nil && !all[i].IsExample {
				all[i].Stdin = ""
				all[i].ExpectedStdout = nil
			}
		}
	}
	writeJSON(w, http.StatusOK, all)
}

func (h *ProblemHandler) CreateTestCase(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	canView, p, status := canViewProblem(r.Context(), h.accessDeps(), claims, chi.URLParam(r, "id"))
	if status == http.StatusInternalServerError {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil || !canView {
		writeError(w, http.StatusNotFound, "Not found")
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
		if !authorizedForProblemEditRow(r.Context(), h.accessDeps(), claims, p) {
			writeError(w, http.StatusForbidden, "Only the problem editor can add canonical test cases")
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

// testCaseGuard loads a test-case and verifies the caller can modify it:
// private cases require the owner (or platform admin); canonical cases
// require the problem's editor.
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
	// Canonical — check problem editor rights.
	p, err := h.Problems.GetProblem(r.Context(), c.ProblemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return nil
	}
	if !authorizedForProblemEditRow(r.Context(), h.accessDeps(), claims, p) {
		writeError(w, http.StatusForbidden, "Only the problem editor can modify canonical cases")
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
	canView, p, status := canViewProblem(r.Context(), h.accessDeps(), claims, chi.URLParam(r, "id"))
	if status == http.StatusInternalServerError {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil || !canView {
		writeError(w, http.StatusNotFound, "Not found")
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
	canView, p, status := canViewProblem(r.Context(), h.accessDeps(), claims, chi.URLParam(r, "id"))
	if status == http.StatusInternalServerError {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil || !canView {
		writeError(w, http.StatusNotFound, "Not found")
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
		// Language is now attempt-local (problems carry starter_code keyed by
		// language but no default) — fall back to python for back-compat.
		body.Language = "python"
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
