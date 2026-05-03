package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// ParentHandler — parent-facing endpoints.
//
// Plan 064: re-enabled with the new parent_links auth gate.
// Pre-064 these endpoints returned 501 (plan 047 disabled them
// because no parent ↔ child link existed in the DB; review 006 P0).
type ParentHandler struct {
	Reports     *store.ReportStore
	ParentLinks *store.ParentLinkStore
}

func (h *ParentHandler) Routes(r chi.Router) {
	r.Route("/api/parent/children/{childId}/reports", func(r chi.Router) {
		r.Use(ValidateUUIDParam("childId"))
		r.Get("/", h.ListReports)
		r.Post("/", h.CreateReport)
	})
}

// requireParentOf checks that `claims.UserID` has an ACTIVE
// parent_link to `childID`. On miss, writes a 403 and returns
// false. Platform admins bypass.
func (h *ParentHandler) requireParentOf(w http.ResponseWriter, r *http.Request, claims *auth.Claims, childID string) bool {
	if claims.IsPlatformAdmin {
		return true
	}
	if h.ParentLinks == nil {
		writeError(w, http.StatusInternalServerError, "ParentLinks store unavailable")
		return false
	}
	ok, err := h.ParentLinks.IsParentOf(r.Context(), claims.UserID, childID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "Not authorized")
		return false
	}
	return true
}

// ListReports handles GET /api/parent/children/{childId}/reports.
//
// Returns reports for the given child if the caller has an active
// parent_link to that child (or is a platform admin).
func (h *ParentHandler) ListReports(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	childID := chi.URLParam(r, "childId")
	if !h.requireParentOf(w, r, claims, childID) {
		return
	}
	reports, err := h.Reports.ListReportsByStudent(r.Context(), childID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, reports)
}

// CreateReport handles POST /api/parent/children/{childId}/reports.
//
// Accepts a parent-authored report for the given child. The same
// auth gate as ListReports applies. The body shape mirrors
// CreateReportInput minus the fields the server fills in (id,
// generated_by, created_at).
func (h *ParentHandler) CreateReport(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	childID := chi.URLParam(r, "childId")
	if !h.requireParentOf(w, r, claims, childID) {
		return
	}

	var body struct {
		PeriodStart time.Time       `json:"periodStart"`
		PeriodEnd   time.Time       `json:"periodEnd"`
		Content     string          `json:"content"`
		Summary     json.RawMessage `json:"summary"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if body.PeriodStart.IsZero() || body.PeriodEnd.IsZero() {
		writeError(w, http.StatusBadRequest, "periodStart and periodEnd are required")
		return
	}
	if body.PeriodEnd.Before(body.PeriodStart) {
		writeError(w, http.StatusBadRequest, "periodEnd must not precede periodStart")
		return
	}

	summary := string(body.Summary)
	if summary == "" {
		summary = "{}"
	}
	report, err := h.Reports.CreateReport(r.Context(), store.CreateReportInput{
		StudentID:   childID,
		GeneratedBy: claims.UserID,
		PeriodStart: body.PeriodStart,
		PeriodEnd:   body.PeriodEnd,
		Content:     body.Content,
		Summary:     summary,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create report")
		return
	}
	writeJSON(w, http.StatusCreated, report)
}
