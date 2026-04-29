package handlers

import (
	"net/http"

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

// notImplementedBody is the canonical 501 payload for parent report
// endpoints. Plan 047 disabled both endpoints because the auth model
// requires a `parent_links` table that doesn't yet exist (review 006
// P0: any authenticated user could read any student's reports). Plan
// 049 will build parent-child linking, schema and all, then re-enable
// these endpoints with the proper auth gate.
//
// 501 Not Implemented is the right semantic: the request is valid,
// the feature is intentionally not implemented yet. The Next-side
// fetch branches on `res.status === 501` AND can read the `code`
// field for structured logging.
var notImplementedBody = map[string]any{
	"error": "Parent reports require parent-child linking, scheduled for plan 049",
	"code":  "not_implemented",
}

// ListReports handles GET /api/parent/children/{childId}/reports.
// Disabled in plan 047; re-enabled in plan 049 with parent_links auth.
func (h *ParentHandler) ListReports(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	writeJSON(w, http.StatusNotImplemented, notImplementedBody)
}

// CreateReport handles POST /api/parent/children/{childId}/reports.
// Disabled in plan 047; re-enabled in plan 049 with parent_links auth.
func (h *ParentHandler) CreateReport(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	writeJSON(w, http.StatusNotImplemented, notImplementedBody)
}
