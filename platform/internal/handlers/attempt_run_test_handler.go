package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/errgroup"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/sandbox"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// PistonRunner abstracts the bit of *sandbox.PistonClient we use so tests
// can stub it out without booting a real Piston container.
type PistonRunner interface {
	ExecuteWithStdin(ctx context.Context, language, code, stdin string) (*sandbox.PistonExecuteResponse, error)
}

// AttemptTestHandler runs canonical test cases for an attempt against Piston.
type AttemptTestHandler struct {
	Attempts  *store.AttemptStore
	Problems  *store.ProblemStore
	TestCases *store.TestCaseStore
	Piston    PistonRunner
}

func (h *AttemptTestHandler) Routes(r chi.Router) {
	r.Route("/api/attempts/{id}/test", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Post("/", h.RunTest)
		r.Route("/{caseId}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("caseId"))
			r.Post("/diff", h.RunCaseDiff)
		})
	})
}

// Run-budget knobs. Per spec 008.
const (
	perCaseTimeout = 3 * time.Second
	totalBudget    = 12 * time.Second
	parallelism    = 4
)

type caseResult struct {
	CaseID     string `json:"caseId"`
	IsExample  bool   `json:"isExample"`
	Status     string `json:"status"` // pass | fail | timeout | skipped
	DurationMs int64  `json:"durationMs"`
	Reason     string `json:"reason,omitempty"`
}

type runSummary struct {
	RanAt   time.Time `json:"ranAt"`
	Summary struct {
		Passed  int `json:"passed"`
		Failed  int `json:"failed"`
		Skipped int `json:"skipped"`
		Total   int `json:"total"`
	} `json:"summary"`
	Cases []caseResult `json:"cases"`
}

// RunTest handles POST /api/attempts/{id}/test.
func (h *AttemptTestHandler) RunTest(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	attemptID := chi.URLParam(r, "id")

	a, err := h.Attempts.GetAttempt(r.Context(), attemptID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if a == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if a.UserID != claims.UserID && !claims.IsPlatformAdmin {
		writeError(w, http.StatusNotFound, "Not found") // owner-only; don't leak existence
		return
	}

	cases, err := h.TestCases.ListCanonical(r.Context(), a.ProblemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	summary := h.executeCases(r.Context(), a.Language, a.PlainText, cases)

	// Persist (best-effort — a write failure shouldn't fail the response).
	if blob, err := json.Marshal(summary); err == nil {
		if err := h.Attempts.UpdateLastTestResult(r.Context(), attemptID, blob); err != nil {
			// Log via writeError is overkill for a non-fatal persistence miss.
			// Fall through and return the summary anyway.
			_ = err
		}
	}

	writeJSON(w, http.StatusOK, summary)
}

// RunCaseDiff handles POST /api/attempts/{id}/test/{caseId}/diff.
// Re-runs the single example case and returns actual + expected.
// Hidden cases are 403 — we never disclose their stdout to non-authors,
// and the owner of the attempt is the only caller of this endpoint.
func (h *AttemptTestHandler) RunCaseDiff(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	attemptID := chi.URLParam(r, "id")
	caseID := chi.URLParam(r, "caseId")

	a, err := h.Attempts.GetAttempt(r.Context(), attemptID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if a == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if a.UserID != claims.UserID && !claims.IsPlatformAdmin {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	tc, err := h.TestCases.GetTestCase(r.Context(), caseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if tc == nil || tc.ProblemID != a.ProblemID {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	// Diff is for example cases only — never reveal hidden case actual output.
	if !tc.IsExample {
		writeError(w, http.StatusForbidden, "Diff is only available for example cases")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), perCaseTimeout)
	defer cancel()
	resp, err := h.Piston.ExecuteWithStdin(ctx, a.Language, a.PlainText, tc.Stdin)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Execution failed")
		return
	}

	expected := ""
	if tc.ExpectedStdout != nil {
		expected = *tc.ExpectedStdout
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"actualStdout":   resp.Run.Stdout,
		"expectedStdout": expected,
		"exitCode":       resp.Run.Code,
	})
}

// executeCases runs every canonical case in parallel under the per-case + total
// budgets and returns a summary suitable for JSON encoding.
func (h *AttemptTestHandler) executeCases(parent context.Context, language, code string, cases []store.TestCase) runSummary {
	ctx, cancel := context.WithTimeout(parent, totalBudget)
	defer cancel()

	results := make([]caseResult, len(cases))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(parallelism)

	var mu sync.Mutex
	for i, tc := range cases {
		i, tc := i, tc
		g.Go(func() error {
			// Skipped if the parent budget already expired before we got a slot.
			if gctx.Err() != nil {
				mu.Lock()
				results[i] = caseResult{CaseID: tc.ID, IsExample: tc.IsExample, Status: "skipped"}
				mu.Unlock()
				return nil
			}
			caseCtx, caseCancel := context.WithTimeout(gctx, perCaseTimeout)
			defer caseCancel()

			start := time.Now()
			resp, err := h.Piston.ExecuteWithStdin(caseCtx, language, code, tc.Stdin)
			dur := time.Since(start).Milliseconds()

			r := caseResult{CaseID: tc.ID, IsExample: tc.IsExample, DurationMs: dur}
			if err != nil {
				if caseCtx.Err() == context.DeadlineExceeded {
					r.Status = "timeout"
				} else {
					r.Status = "fail"
					r.Reason = "execution_error"
				}
			} else if tc.ExpectedStdout == nil {
				// Informational case: no comparison.
				r.Status = "pass"
			} else if normalizeStdout(resp.Run.Stdout) == normalizeStdout(*tc.ExpectedStdout) {
				r.Status = "pass"
			} else {
				r.Status = "fail"
				r.Reason = "wrong_output"
			}
			mu.Lock()
			results[i] = r
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	summary := runSummary{
		RanAt: time.Now().UTC(),
		Cases: results,
	}
	summary.Summary.Total = len(results)
	for _, r := range results {
		switch r.Status {
		case "pass":
			summary.Summary.Passed++
		case "fail", "timeout":
			summary.Summary.Failed++
		case "skipped":
			summary.Summary.Skipped++
		}
	}
	return summary
}

// normalizeStdout right-trims and normalizes \r\n -> \n so trailing newline
// differences and Windows line endings don't fail correct programs.
func normalizeStdout(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n")
}
