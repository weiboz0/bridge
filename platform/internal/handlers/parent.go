package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type ParentHandler struct {
	Reports *store.ReportStore
}

func (h *ParentHandler) Routes(r chi.Router) {
	r.Route("/api/parent/children/{childId}/reports", func(r chi.Router) {
		r.Use(ValidateUUIDParam("childId"))
		r.Get("/", h.ListReports)
		r.Post("/", h.CreateReport)
	})
}

// ListReports handles GET /api/parent/children/{childId}/reports
func (h *ParentHandler) ListReports(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	childID := chi.URLParam(r, "childId")
	reports, err := h.Reports.ListReportsByStudent(r.Context(), childID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, reports)
}

// CreateReport handles POST /api/parent/children/{childId}/reports
// This generates a report using the LLM (placeholder for now — Phase C will add the skill)
func (h *ParentHandler) CreateReport(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	childID := chi.URLParam(r, "childId")

	var body struct {
		PeriodStart string `json:"periodStart"`
		PeriodEnd   string `json:"periodEnd"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.PeriodStart == "" || body.PeriodEnd == "" {
		writeError(w, http.StatusBadRequest, "periodStart and periodEnd are required")
		return
	}

	periodStart, err := time.Parse(time.RFC3339, body.PeriodStart)
	if err != nil {
		writeError(w, http.StatusBadRequest, "periodStart must be ISO 8601 format")
		return
	}
	periodEnd, err := time.Parse(time.RFC3339, body.PeriodEnd)
	if err != nil {
		writeError(w, http.StatusBadRequest, "periodEnd must be ISO 8601 format")
		return
	}

	// TODO: Phase C will add LLM-based report generation via the report_generator skill.
	// For now, create a placeholder report.
	report, err := h.Reports.CreateReport(r.Context(), store.CreateReportInput{
		StudentID:   childID,
		GeneratedBy: claims.UserID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Content:     "Report generation will be implemented in Phase C (AI Skills).",
		Summary:     "{}",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create report")
		return
	}
	writeJSON(w, http.StatusCreated, report)
}
