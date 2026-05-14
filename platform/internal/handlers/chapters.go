package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/projection"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// ChapterHandler serves teaching unit CRUD and document endpoints.
// Access is scope-based (platform / org / personal) per spec 012.
// Lifecycle transitions (publish/archive) and overlay/fork routes are
// excluded from this handler — they land in plans 033 and 034.
type ChapterHandler struct {
	Units   *store.ChapterStore
	Orgs    *store.OrgStore
	Courses *store.CourseStore // Plan 045: backs the SearchChapters ?linkableForCourse= gate.
}

// validChapterScopes is the allowed set of scope values.
var validChapterScopes = map[string]bool{
	"platform": true,
	"org":      true,
	"personal": true,
}

// validChapterStatuses is the allowed set of status values for initial creation /
// manual updates in plan 031. Lifecycle transitions (draft → reviewed → ready)
// land in plan 033.
var validChapterStatuses = map[string]bool{
	"draft":           true,
	"reviewed":        true,
	"classroom_ready": true,
	"coach_ready":     true,
	"archived":        true,
}

// knownBlockTypes is the allowlist for top-level block types in a unit
// document. It is kept as a package-level variable so tests can inspect it
// directly. Expanded in plan 033b to include solution-ref, test-case-ref,
// live-cue, and assignment-variant.
var knownBlockTypes = map[string]bool{
	"prose":              true,
	"problem-ref":        true,
	"paragraph":          true,
	"heading":            true,
	"bulletList":         true,
	"orderedList":        true,
	"listItem":           true,
	"codeBlock":          true,
	"blockquote":         true,
	"horizontalRule":     true,
	"hardBreak":          true,
	"teacher-note":       true,
	"code-snippet":       true,
	"media-embed":        true,
	"solution-ref":       true,
	"test-case-ref":      true,
	"live-cue":           true,
	"assignment-variant": true,
	// Table (StarterKit-adjacent, no custom ID needed)
	"table":       true,
	"tableRow":    true,
	"tableCell":   true,
	"tableHeader": true,
	// Task list (StarterKit-adjacent, no custom ID needed)
	"taskList": true,
	"taskItem": true,
	// Phase 3 custom block types
	"callout":      true,
	"toggle-block": true,
	"bookmark":     true,
	"toc":          true,
	"columns":      true,
	"column":       true,
	// Math / KaTeX nodes
	"math-block":  true,
	"math-inline": true,
}

const maxChapterTitleLen = 255

func (h *ChapterHandler) Routes(r chi.Router) {
	r.Route("/api/chapters", func(r chi.Router) {
		r.Get("/", h.ListChapters)
		r.Post("/", h.CreateChapter)

		// Static sub-paths MUST be registered before the {id} wildcard
		r.Get("/search", h.SearchChapters)

		r.Route("/by-topic/{topicId}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("topicId"))
			r.Get("/", h.GetChapterByTopic)
		})

		r.Route("/{id}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("id"))
			r.Get("/", h.GetChapter)
			r.Patch("/", h.UpdateChapter)
			r.Delete("/", h.DeleteChapter)
			r.Get("/document", h.GetDocument)
			r.Put("/document", h.SaveDocument)
			r.Get("/projected", h.GetProjectedDocument)
			r.Post("/transition", h.TransitionChapter)
			r.Get("/revisions", h.ListRevisions)
			r.Route("/revisions/{revisionId}", func(r chi.Router) {
				r.Use(ValidateUUIDParam("revisionId"))
				r.Get("/", h.GetRevision)
			})
			r.Post("/fork", h.ForkChapter)
			r.Get("/overlay", h.GetOverlay)
			r.Patch("/overlay", h.PatchOverlay)
			r.Get("/composed", h.GetComposedDocument)
			r.Get("/lineage", h.GetLineage)
		})
	})
}

// ---------- Access helpers ----------

// canViewChapter applies the Access table from spec 012 §Access, with the
// plan-031-specific narrowing: org students are denied access entirely
// until class-binding lands in plan 032.
//
// Platform scope: published/archived/coach_ready/classroom_ready → any auth;
// draft/reviewed → platform admin only.
// Org scope: teachers/org_admins see all statuses; students denied.
// Personal scope: owner only.
// Platform admin bypass applies everywhere.
func (h *ChapterHandler) canViewChapter(ctx context.Context, c *auth.Claims, u *store.Chapter) bool {
	// Plan 052 PR-C: thin wrapper around the free `CanViewChapter`
	// helper so non-handler-method callers (e.g., ChapterCollectionHandler)
	// can apply the same rule.
	// Plan 061: passes h.Units so the helper can run the
	// student-binding check.
	return CanViewChapter(ctx, h.Orgs, h.Units, c, u)
}

// TODO(plan-075-followup): canEditChapter (line ~164) and resolveViewerRole
// (line ~871) each have an inline GetUserRolesInOrg in the org-scope branch
// of a switch over scope. Migrating to RequireOrgAuthority touches helper
// signatures (returns bool / projection.ViewerRole). See plan-075 §Out of
// scope, Bucket 2.

// canEditChapter checks whether the caller may create, update, or delete a unit
// in the given scope. Platform requires platform admin; org requires an active
// org_admin or teacher; personal requires the caller to be the scope owner.
func (h *ChapterHandler) canEditChapter(ctx context.Context, c *auth.Claims, scope string, scopeID *string) bool {
	if c.IsPlatformAdmin && scope == "platform" {
		return true
	}
	switch scope {
	case "platform":
		return c.IsPlatformAdmin
	case "org":
		if scopeID == nil {
			return false
		}
		roles, _ := h.Orgs.GetUserRolesInOrg(ctx, *scopeID, c.UserID)
		for _, m := range roles {
			if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
				return true
			}
		}
		return false
	case "personal":
		return scopeID != nil && *scopeID == c.UserID
	}
	return false
}

// ---------- Document validation ----------

// validateBlockDocument enforces the spec-012 block-ID invariant and the
// plan-031 block-type allowlist. Returns a non-nil error with a descriptive
// message if the document fails validation.
func validateBlockDocument(raw json.RawMessage) error {
	// Decode just enough to inspect the envelope.
	var envelope struct {
		Type    string            `json:"type"`
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("document must be valid JSON")
	}
	if envelope.Type != "doc" {
		return fmt.Errorf("document envelope must have \"type\":\"doc\"")
	}

	// Block types that carry attrs.id (custom teaching-unit blocks).
	// Standard StarterKit structural blocks (paragraph, heading, etc.)
	// don't need IDs — they're just rich text. Table/taskList extensions
	// are StarterKit-adjacent and also don't use custom IDs.
	blockTypesRequiringID := map[string]bool{
		"prose":              true,
		"problem-ref":        true,
		"teacher-note":       true,
		"code-snippet":       true,
		"media-embed":        true,
		"solution-ref":       true,
		"test-case-ref":      true,
		"live-cue":           true,
		"assignment-variant": true,
		// Phase 3 custom block types
		"callout":      true,
		"toggle-block": true,
		"bookmark":     true,
		"toc":          true,
		"columns":      true,
		// Math / KaTeX blocks
		"math-block":  true,
		"math-inline": true,
	}

	// Walk top-level blocks.
	for i, rawBlock := range envelope.Content {
		var block struct {
			Type  string `json:"type"`
			Attrs struct {
				ID string `json:"id"`
			} `json:"attrs"`
		}
		if err := json.Unmarshal(rawBlock, &block); err != nil {
			return fmt.Errorf("block at index %d is not valid JSON", i)
		}
		if blockTypesRequiringID[block.Type] && block.Attrs.ID == "" {
			return fmt.Errorf("block at index %d (%s) is missing attrs.id", i, block.Type)
		}
		if !knownBlockTypes[block.Type] {
			return fmt.Errorf("block at index %d has unknown type %q (allowed: %s)",
				i, block.Type, joinBlockTypes())
		}
	}
	return nil
}

func joinBlockTypes() string {
	types := make([]string, 0, len(knownBlockTypes))
	for t := range knownBlockTypes {
		types = append(types, t)
	}
	return strings.Join(types, ", ")
}

// ---------- Handlers ----------

// ListChapters — GET /api/chapters?scope=&scopeId=&bookId=
// Returns units the caller can view. Scope + scopeId filter to a specific
// bucket; omitting both returns all units visible to the caller (this plan
// returns units from the requested bucket only for simplicity). bookId
// optionally filters to chapters in a specific book; pass `unfiled` to
// match chapters with NULL book_id.
func (h *ChapterHandler) ListChapters(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	scope := r.URL.Query().Get("scope")
	scopeIDRaw := r.URL.Query().Get("scopeId")
	bookIDRaw := r.URL.Query().Get("bookId")

	if scope != "" && !validChapterScopes[scope] {
		writeError(w, http.StatusBadRequest, "scope must be platform, org, or personal")
		return
	}

	var bookFilter store.ChapterBookFilter
	if bookIDRaw != "" {
		if bookIDRaw == "unfiled" {
			empty := ""
			bookFilter = &empty
		} else {
			if _, err := uuid.Parse(bookIDRaw); err != nil {
				writeError(w, http.StatusBadRequest, "bookId must be a UUID or 'unfiled'")
				return
			}
			bookFilter = &bookIDRaw
		}
	}

	// If no scope is provided, default to returning units across all accessible
	// buckets. For plan 031 simplicity, require at least a scope.
	if scope == "" {
		// Return an empty list with a hint rather than a 400 so clients can
		// call without a scope and learn they need to specify one.
		writeJSON(w, http.StatusOK, map[string]any{"items": []store.Chapter{}})
		return
	}

	units, err := h.Units.ListChaptersForScope(r.Context(), scope, scopeIDRaw, bookFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Filter by view access.
	visible := make([]store.Chapter, 0, len(units))
	for _, u := range units {
		u := u // capture
		if h.canViewChapter(r.Context(), claims, &u) {
			visible = append(visible, u)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": visible})
}

// SearchChapters — GET /api/chapters/search?q=&scope=&gradeLevel=&materialType=&tags=&limit=&cursor=&linkableForCourse=
// Returns units matching FTS query and/or structured filters, with visibility
// filtering. When q is non-empty, results are ranked by FTS relevance;
// otherwise they are ordered by updated_at DESC.
//
// Plan 045 additions:
//   - materialType filter (notes / slides / worksheet / reference).
//   - cursor query param is now actually parsed (was previously emitted
//     by the handler but never read back — Load More was a no-op).
//   - linkableForCourse=<courseId> switches into "picker mode": loads
//     the course, gates the caller to creator-or-platform-admin, and
//     returns Units linkable to that course (platform-scope OR same
//     org) decorated with linkedTopicId/linkedTopicTitle/canLink.
func (h *ChapterHandler) SearchChapters(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	q := r.URL.Query()
	filter := store.SearchChaptersFilter{
		Query:           q.Get("q"),
		Scope:           q.Get("scope"),
		Status:          q.Get("status"),
		GradeLevel:      q.Get("gradeLevel"),
		MaterialType:    q.Get("materialType"),
		ViewerID:        claims.UserID,
		IsPlatformAdmin: claims.IsPlatformAdmin,
	}

	if scopeID := q.Get("scopeId"); scopeID != "" {
		filter.ScopeID = &scopeID
	}

	if filter.Scope != "" && !validChapterScopes[filter.Scope] {
		writeError(w, http.StatusBadRequest, "scope must be platform, org, or personal")
		return
	}

	if bookIDRaw := q.Get("bookId"); bookIDRaw != "" {
		if bookIDRaw == "unfiled" {
			empty := ""
			filter.BookFilter = &empty
		} else {
			if _, err := uuid.Parse(bookIDRaw); err != nil {
				writeError(w, http.StatusBadRequest, "bookId must be a UUID or 'unfiled'")
				return
			}
			filter.BookFilter = &bookIDRaw
		}
	}

	if tags := q.Get("tags"); tags != "" {
		filter.SubjectTags = strings.Split(tags, ",")
	}

	if limitStr := q.Get("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		filter.Limit = l
	}

	if cursor := q.Get("cursor"); cursor != "" {
		ts, id, ok := parseSearchCursor(cursor)
		if !ok {
			writeError(w, http.StatusBadRequest,
				"cursor must be of form <RFC3339-timestamp>|<chapterId>")
			return
		}
		filter.CursorCreatedAt = &ts
		filter.CursorID = &id
	}

	// Build viewer's org list for visibility. Only include orgs where the
	// viewer has teacher or admin roles (students are denied org access per
	// plan 031).
	if !claims.IsPlatformAdmin {
		orgs, err := h.Orgs.GetUserMemberships(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		for _, m := range orgs {
			if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
				filter.ViewerOrgs = append(filter.ViewerOrgs, m.OrgID)
			}
		}
	}

	// Plan 045 picker mode: linkableForCourse=<courseId> dispatches to
	// the dedicated SearchChaptersForPicker SQL (LEFT JOIN topics + courses
	// for the linked-topic decoration with cross-org title redaction).
	if linkableCourseID := q.Get("linkableForCourse"); linkableCourseID != "" {
		h.searchUnitsForPicker(w, r, claims, filter, linkableCourseID)
		return
	}

	units, err := h.Units.SearchChapters(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Build nextCursor for non-FTS browse pagination.
	var nextCursor *string
	if filter.Query == "" && len(units) > 0 && filter.Limit > 0 && len(units) == filter.Limit {
		last := units[len(units)-1]
		cursor := last.UpdatedAt.Format("2006-01-02T15:04:05.000000Z07:00") + "|" + last.ID
		nextCursor = &cursor
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":      units,
		"nextCursor": nextCursor,
	})
}

// parseSearchCursor splits the SearchChapters cursor format
// `<RFC3339-timestamp>|<chapterId>` and returns the parsed timestamp + ID.
// Returns ok=false on any malformed input — the caller should map to 400.
func parseSearchCursor(c string) (time.Time, string, bool) {
	parts := strings.SplitN(c, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return time.Time{}, "", false
	}
	// The handler emits with format "2006-01-02T15:04:05.000000Z07:00"
	// (RFC3339 with microseconds); time.RFC3339Nano accepts both.
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", false
	}
	return ts, parts[1], true
}

// pickerItem is the per-row payload SearchChapters returns in picker mode.
// Adds the linked-topic decoration plus a server-computed canLink that
// the UI uses to disable rows the caller cannot actually attach.
type pickerItem struct {
	store.UnitWithLinkedTopic
	CanLink bool `json:"canLink"`
}

// searchUnitsForPicker runs the SearchChaptersForPicker store method,
// gates on course-edit access, computes canLink per row, and writes
// the response. Split out from SearchChapters for clarity.
func (h *ChapterHandler) searchUnitsForPicker(
	w http.ResponseWriter,
	r *http.Request,
	claims *auth.Claims,
	filter store.SearchChaptersFilter,
	courseID string,
) {
	if h.Courses == nil {
		writeError(w, http.StatusInternalServerError, "Course store unavailable")
		return
	}
	course, err := h.Courses.GetCourse(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if course == nil {
		writeError(w, http.StatusNotFound, "Course not found")
		return
	}
	// effectivePlatformAdmin captures plain platform admins AND admins
	// currently impersonating another user. canLinkUnitToCourse uses the
	// same combined check (topics.go::canLinkUnitToCourse), so picker
	// canLink + redaction must too — otherwise an impersonating admin
	// sees canLink=false on rows the actual link handler would allow.
	effectivePlatformAdmin := claims.IsPlatformAdmin || claims.ImpersonatedBy != ""

	if !effectivePlatformAdmin && course.CreatedBy != claims.UserID {
		writeError(w, http.StatusForbidden,
			"Only the course creator can browse linkable units for this course")
		return
	}

	// Filter draft platform Units out of picker results for non-admin
	// callers, mirroring the regular SearchChapters visibility gate
	// (only published platform-scope is visible to teachers). Without
	// this, the picker leaks draft titles/summaries that the regular
	// search endpoint would not surface — and they'd be canLink=false
	// noise anyway.
	rows, err := h.Units.SearchChaptersForPicker(
		r.Context(), filter, course.OrgID, effectivePlatformAdmin,
		!effectivePlatformAdmin, // restrictPlatformToPublished
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// canLink per row: replicate canLinkUnitToCourse's semantics. Since
	// the visibility scope already restricts to platform-scope OR
	// org-scope-with-matching-org-id, the only remaining checks are:
	//   - platform-scope: status must be in published-statuses (admin
	//     bypasses; impersonating-admin uses effectivePlatformAdmin).
	//   - org-scope: caller must be teacher/org_admin in course.OrgID
	//     (or platform admin / impersonating-admin).
	canLinkOrg := effectivePlatformAdmin
	if !canLinkOrg && h.Orgs != nil {
		// Reuse the ViewerOrgs already computed by SearchChapters caller.
		// SearchChapters populates filter.ViewerOrgs before dispatching.
		for _, oid := range filter.ViewerOrgs {
			if oid == course.OrgID {
				canLinkOrg = true
				break
			}
		}
	}

	items := make([]pickerItem, 0, len(rows))
	for i := range rows {
		row := rows[i]
		can := false
		switch row.Scope {
		case "platform":
			can = effectivePlatformAdmin || publishedPlatformChapterStatuses[row.Status]
		case "org":
			can = canLinkOrg
		}
		items = append(items, pickerItem{
			UnitWithLinkedTopic: row,
			CanLink:             can,
		})
	}

	var nextCursor *string
	if filter.Query == "" && len(items) > 0 && filter.Limit > 0 && len(items) == filter.Limit {
		last := items[len(items)-1]
		cursor := last.UpdatedAt.Format("2006-01-02T15:04:05.000000Z07:00") + "|" + last.ID
		nextCursor = &cursor
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"nextCursor": nextCursor,
	})
}

// CreateChapter — POST /api/chapters
func (h *ChapterHandler) CreateChapter(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		Scope            string   `json:"scope"`
		ScopeID          *string  `json:"scopeId"`
		Title            string   `json:"title"`
		Slug             *string  `json:"slug"`
		Summary          string   `json:"summary"`
		GradeLevel       *string  `json:"gradeLevel"`
		SubjectTags      []string `json:"subjectTags"`
		StandardsTags    []string `json:"standardsTags"`
		EstimatedMinutes *int     `json:"estimatedMinutes"`
		MaterialType     string   `json:"materialType"`
		Status           string   `json:"status"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if !validChapterScopes[body.Scope] {
		writeError(w, http.StatusBadRequest, "scope must be platform, org, or personal")
		return
	}
	if body.Title == "" || len(body.Title) > maxChapterTitleLen {
		writeError(w, http.StatusBadRequest, "title is required (max 255 chars)")
		return
	}
	if body.Status != "" && !validChapterStatuses[body.Status] {
		writeError(w, http.StatusBadRequest, "invalid status value")
		return
	}

	// Enforce scope / scopeId consistency.
	if body.Scope == "platform" && body.ScopeID != nil && *body.ScopeID != "" {
		writeError(w, http.StatusBadRequest, "platform-scope units must not have a scopeId")
		return
	}
	if body.Scope != "platform" && (body.ScopeID == nil || *body.ScopeID == "") {
		writeError(w, http.StatusBadRequest, "scopeId is required for org and personal scope")
		return
	}

	if !h.canEditChapter(r.Context(), claims, body.Scope, body.ScopeID) {
		writeError(w, http.StatusForbidden, "not authorized for scope")
		return
	}

	unit, err := h.Units.CreateChapter(r.Context(), store.CreateChapterInput{
		Scope:            body.Scope,
		ScopeID:          body.ScopeID,
		Title:            body.Title,
		Slug:             body.Slug,
		Summary:          body.Summary,
		GradeLevel:       body.GradeLevel,
		SubjectTags:      body.SubjectTags,
		StandardsTags:    body.StandardsTags,
		EstimatedMinutes: body.EstimatedMinutes,
		MaterialType:     body.MaterialType,
		Status:           body.Status,
		CreatedBy:        claims.UserID,
	})
	if err != nil {
		if isConstraintError(err) {
			writeError(w, http.StatusConflict, "constraint violation: check scope/scopeId combination")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to create chapter")
		return
	}
	writeJSON(w, http.StatusCreated, unit)
}

// GetChapter — GET /api/chapters/{id}
func (h *ChapterHandler) GetChapter(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	unit, err := h.Units.GetChapter(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found") // don't leak existence
		return
	}
	writeJSON(w, http.StatusOK, unit)
}

// UpdateChapter — PATCH /api/chapters/{id}
func (h *ChapterHandler) UpdateChapter(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	chapterID := chi.URLParam(r, "id")

	// Load the row first to determine scope for authz and to return 404 vs 403.
	unit, err := h.Units.GetChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	// If not found OR caller can't even view it → 404 (don't leak existence).
	if unit == nil || !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditChapter(r.Context(), claims, unit.Scope, unit.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to edit")
		return
	}

	var body store.UpdateChapterInput
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Title != nil && (*body.Title == "" || len(*body.Title) > maxChapterTitleLen) {
		writeError(w, http.StatusBadRequest, "title must be 1-255 chars")
		return
	}
	if body.Status != nil && *body.Status != "" && !validChapterStatuses[*body.Status] {
		writeError(w, http.StatusBadRequest, "invalid status value")
		return
	}

	updated, err := h.Units.UpdateChapter(r.Context(), chapterID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteChapter — DELETE /api/chapters/{id}
func (h *ChapterHandler) DeleteChapter(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	chapterID := chi.URLParam(r, "id")

	unit, err := h.Units.GetChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditChapter(r.Context(), claims, unit.Scope, unit.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to delete")
		return
	}

	deleted, err := h.Units.DeleteChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if deleted == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetDocument — GET /api/chapters/{id}/document
func (h *ChapterHandler) GetDocument(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	chapterID := chi.URLParam(r, "id")

	unit, err := h.Units.GetChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	doc, err := h.Units.GetDocument(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "Document not found")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// GetProjectedDocument — GET /api/chapters/{id}/projected?role=student&attemptStates=b03:submitted,b05:not_started
// Returns the unit document filtered through the projection pipeline.
// Teachers/admins can pass ?role=student to preview the student view.
// Students always receive the student projection regardless of query param.
func (h *ChapterHandler) GetProjectedDocument(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	chapterID := chi.URLParam(r, "id")

	unit, err := h.Units.GetChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	// Determine the caller's actual role.
	actualRole := h.resolveViewerRole(r.Context(), claims, unit)

	// Parse optional ?role query param. Students are locked to "student"
	// regardless of what they request.
	role := actualRole
	if qRole := r.URL.Query().Get("role"); qRole != "" {
		requested := projection.ViewerRole(qRole)
		switch requested {
		case projection.RoleStudent, projection.RoleTeacher, projection.RoleAdmin:
			// Only privileged users may request a different role (e.g., preview).
			// Students are always forced to "student".
			if actualRole == projection.RoleStudent {
				role = projection.RoleStudent
			} else {
				role = requested
			}
		default:
			writeError(w, http.StatusBadRequest, "invalid role value; must be student, teacher, or platform_admin")
			return
		}
	}

	// Parse optional ?attemptStates query param: comma-separated blockId:state pairs.
	attemptStates := map[string]projection.AttemptState{}
	if raw := r.URL.Query().Get("attemptStates"); raw != "" {
		pairs := strings.Split(raw, ",")
		for _, pair := range pairs {
			parts := strings.SplitN(pair, ":", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				writeError(w, http.StatusBadRequest, "attemptStates must be comma-separated blockId:state pairs")
				return
			}
			state := projection.AttemptState(parts[1])
			switch state {
			case projection.AttemptNotStarted, projection.AttemptInProgress,
				projection.AttemptSubmitted, projection.AttemptPassed, projection.AttemptFailed:
				attemptStates[parts[0]] = state
			default:
				writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid attempt state %q; must be not_started, in_progress, submitted, passed, or failed", parts[1]))
				return
			}
		}
	}

	// Plan 062 — Fetch the COMPOSED document (parent + overlay
	// merged), not the raw child blocks. For a unit with no overlay
	// row the store falls back to the unit's own blocks, so this is
	// a strict superset of the prior behavior. The original code
	// called GetDocument here, which for forked units returned only
	// the raw child blocks — students saw empty / stale content.
	composedBlocks, err := h.Units.GetComposedDocument(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if composedBlocks == nil {
		writeError(w, http.StatusNotFound, "Document not found")
		return
	}

	// Extract content array from the document envelope.
	var envelope struct {
		Type    string            `json:"type"`
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(composedBlocks, &envelope); err != nil {
		writeError(w, http.StatusInternalServerError, "Malformed document")
		return
	}

	// Run projection on the COMPOSED blocks. Compose-then-filter is
	// the right order: composition merges/replaces blocks first
	// (overlay semantics), THEN projection hides teacher-only and
	// student-gated blocks.
	filtered := projection.ProjectBlocks(envelope.Content, role, attemptStates)

	// Return reconstructed document.
	writeJSON(w, http.StatusOK, map[string]any{
		"type":    "doc",
		"content": filtered,
	})
}

// resolveViewerRole determines the caller's effective projection role based on
// their platform status, org membership, or personal ownership.
func (h *ChapterHandler) resolveViewerRole(ctx context.Context, c *auth.Claims, u *store.Chapter) projection.ViewerRole {
	if c.IsPlatformAdmin {
		return projection.RoleAdmin
	}

	switch u.Scope {
	case "personal":
		// The owner of a personal unit is treated as a teacher for projection.
		if u.ScopeID != nil && *u.ScopeID == c.UserID {
			return projection.RoleTeacher
		}
		return projection.RoleStudent

	case "org":
		if u.ScopeID == nil {
			return projection.RoleStudent
		}
		roles, _ := h.Orgs.GetUserRolesInOrg(ctx, *u.ScopeID, c.UserID)
		for _, m := range roles {
			if m.Status != "active" {
				continue
			}
			if m.Role == "org_admin" || m.Role == "teacher" {
				return projection.RoleTeacher
			}
		}
		return projection.RoleStudent

	case "platform":
		// Non-admin viewing a platform unit → student.
		return projection.RoleStudent
	}

	return projection.RoleStudent
}

// GetChapterByTopic — GET /api/chapters/by-topic/{topicId}
// Looks up the teaching unit linked to the given topic. Returns 404 if no unit
// is linked to that topic or if the caller cannot view the found unit.
func (h *ChapterHandler) GetChapterByTopic(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	topicID := chi.URLParam(r, "topicId")

	unit, err := h.Units.GetChapterByTopicID(r.Context(), topicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found") // don't leak existence
		return
	}
	writeJSON(w, http.StatusOK, unit)
}

// SaveDocument — PUT /api/chapters/{id}/document
// Accepts a raw JSON body that must satisfy the spec-012 block-document shape.
func (h *ChapterHandler) SaveDocument(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	chapterID := chi.URLParam(r, "id")

	unit, err := h.Units.GetChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditChapter(r.Context(), claims, unit.Scope, unit.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to edit document")
		return
	}

	// Read the raw body so we can validate it before forwarding to the store.
	var raw json.RawMessage
	if !decodeJSON(w, r, &raw) {
		return
	}
	if err := validateBlockDocument(raw); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	doc, err := h.Units.SaveDocument(r.Context(), chapterID, raw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// TransitionChapter — POST /api/chapters/{id}/transition
// Transitions the unit to a new status per the spec-012 state machine.
// Body: { "status": "<target>" }
// Maps ErrInvalidTransition → 409 Conflict.
func (h *ChapterHandler) TransitionChapter(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	chapterID := chi.URLParam(r, "id")

	unit, err := h.Units.GetChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditChapter(r.Context(), claims, unit.Scope, unit.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to transition")
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if !validChapterStatuses[body.Status] {
		writeError(w, http.StatusBadRequest, "invalid target status")
		return
	}

	updated, err := h.Units.SetUnitStatus(r.Context(), chapterID, body.Status, claims.UserID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "Not found")
		return
	case errors.Is(err, store.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "invalid status transition")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// ListRevisions — GET /api/chapters/{id}/revisions
func (h *ChapterHandler) ListRevisions(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	chapterID := chi.URLParam(r, "id")

	unit, err := h.Units.GetChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	revisions, err := h.Units.ListRevisions(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": revisions})
}

// GetRevision — GET /api/chapters/{id}/revisions/{revisionId}
func (h *ChapterHandler) GetRevision(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	chapterID := chi.URLParam(r, "id")

	// Verify access to the parent unit first.
	unit, err := h.Units.GetChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	revisionID := chi.URLParam(r, "revisionId")
	rev, err := h.Units.GetRevision(r.Context(), revisionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if rev == nil || rev.ChapterID != chapterID {
		writeError(w, http.StatusNotFound, "Revision not found")
		return
	}
	writeJSON(w, http.StatusOK, rev)
}

// ---------- Overlay / Fork handlers (plan 034) ----------

// ForkChapter — POST /api/chapters/{id}/fork
// Body: { scope?, scopeId?, title? }
// Requires: canViewChapter on source + authorized-for-scope on target.
// If scope is omitted, infer from caller's memberships (like problem fork).
func (h *ChapterHandler) ForkChapter(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	sourceID := chi.URLParam(r, "id")

	var body struct {
		Scope   string  `json:"scope"`
		ScopeID *string `json:"scopeId"`
		Title   *string `json:"title"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	// Default target inference (same pattern as problem fork).
	if body.Scope == "" {
		orgs, err := h.Orgs.GetUserMemberships(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		activeOrgs := unitOrgIDs(orgs)
		if len(activeOrgs) == 1 {
			body.Scope = "org"
			orgID := activeOrgs[0]
			body.ScopeID = &orgID
		} else {
			body.Scope = "personal"
			uid := claims.UserID
			body.ScopeID = &uid
		}
	}

	if !validChapterScopes[body.Scope] {
		writeError(w, http.StatusBadRequest, "scope must be platform, org, or personal")
		return
	}

	// Source must be visible to the caller.
	source, err := h.Units.GetChapter(r.Context(), sourceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if source == nil || !h.canViewChapter(r.Context(), claims, source) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	// Caller must be authorized for the target scope.
	if !h.canEditChapter(r.Context(), claims, body.Scope, body.ScopeID) {
		writeError(w, http.StatusForbidden, "not authorized for target scope")
		return
	}

	child, err := h.Units.ForkChapter(r.Context(), sourceID, store.ForkTarget{
		Scope:    body.Scope,
		ScopeID:  body.ScopeID,
		Title:    body.Title,
		CallerID: claims.UserID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fork unit")
		return
	}
	if child == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusCreated, child)
}

// unitOrgIDs extracts active org IDs from user memberships.
func unitOrgIDs(ms []store.UserMembershipWithOrg) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		if m.Status == "active" {
			out = append(out, m.OrgID)
		}
	}
	return out
}

// GetOverlay — GET /api/chapters/{id}/overlay
// Returns the overlay row or 404 if the unit has no overlay.
// Auth: canViewChapter on the child unit.
func (h *ChapterHandler) GetOverlay(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	chapterID := chi.URLParam(r, "id")

	unit, err := h.Units.GetChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	ov, err := h.Units.GetOverlay(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if ov == nil {
		writeError(w, http.StatusNotFound, "No overlay")
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

// PatchOverlay — PATCH /api/chapters/{id}/overlay
// Body: { parentRevisionId?, blockOverrides? }
// Auth: canEditChapter on the child.
func (h *ChapterHandler) PatchOverlay(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	chapterID := chi.URLParam(r, "id")

	unit, err := h.Units.GetChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditChapter(r.Context(), claims, unit.Scope, unit.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to edit overlay")
		return
	}

	// Verify the overlay exists before attempting to update.
	existing, err := h.Units.GetOverlay(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "No overlay")
		return
	}

	var body struct {
		ParentRevisionID *string         `json:"parentRevisionId"`
		BlockOverrides   json.RawMessage `json:"blockOverrides"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	updated, err := h.Units.UpdateOverlay(r.Context(), chapterID, store.UpdateOverlayInput{
		ParentRevisionID: body.ParentRevisionID,
		BlockOverrides:   body.BlockOverrides,
	})
	if err != nil {
		if isConstraintError(err) {
			writeError(w, http.StatusBadRequest, "invalid parentRevisionId")
			return
		}
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "No overlay")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// GetComposedDocument — GET /api/chapters/{id}/composed
// Returns the composed document (overlay-merged), ready for projection.
// Auth: canViewChapter.
func (h *ChapterHandler) GetComposedDocument(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	chapterID := chi.URLParam(r, "id")

	unit, err := h.Units.GetChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	composed, err := h.Units.GetComposedDocument(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if composed == nil {
		writeError(w, http.StatusNotFound, "Document not found")
		return
	}

	// The store already returns a full envelope {"type":"doc","content":[...]}.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(composed)
}

// GetLineage — GET /api/chapters/{id}/lineage
// Returns the overlay chain as a breadcrumb list (root-first).
// Auth: canViewChapter.
func (h *ChapterHandler) GetLineage(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	chapterID := chi.URLParam(r, "id")

	unit, err := h.Units.GetChapter(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewChapter(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	lineage, err := h.Units.GetLineage(r.Context(), chapterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": lineage})
}
