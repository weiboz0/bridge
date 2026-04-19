package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/weiboz0/bridge/platform/internal/sandbox"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// stubPiston implements PistonRunner without booting a real Piston container.
type stubPiston struct {
	respond func(language, code, stdin string) (*sandbox.PistonExecuteResponse, error)
	calls   int
}

func (s *stubPiston) ExecuteWithStdin(_ context.Context, language, code, stdin string) (*sandbox.PistonExecuteResponse, error) {
	s.calls++
	return s.respond(language, code, stdin)
}

func makeStubResp(stdout string) *sandbox.PistonExecuteResponse {
	return &sandbox.PistonExecuteResponse{
		Run: sandbox.PistonStage{Stdout: stdout, Code: 0},
	}
}

// caseFor builds a canonical (no owner) test case with the given stdin and
// optional expected stdout. Pass nil for expected to make it informational.
func caseFor(id, stdin string, expected *string, isExample bool) store.TestCase {
	return store.TestCase{
		ID:             id,
		Stdin:          stdin,
		ExpectedStdout: expected,
		IsExample:      isExample,
	}
}

func ptrStr(s string) *string { return &s }

// ---------- auth-guard tests ----------

func TestRunTest_NoClaims(t *testing.T) {
	h := &AttemptTestHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/attempts/abc/test", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.RunTest(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRunCaseDiff_NoClaims(t *testing.T) {
	h := &AttemptTestHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/attempts/abc/test/xyz/diff", nil)
	req = withChiParams(req, map[string]string{"id": "abc", "caseId": "xyz"})
	w := httptest.NewRecorder()
	h.RunCaseDiff(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------- executeCases tests (pure, no DB) ----------

func TestExecuteCases_AllPass(t *testing.T) {
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, stdin string) (*sandbox.PistonExecuteResponse, error) {
			return makeStubResp(stdin), nil // echo
		},
	}}
	cases := []store.TestCase{
		caseFor("c1", "hi", ptrStr("hi"), true),
		caseFor("c2", "bye", ptrStr("bye"), false),
	}
	s := h.executeCases(context.Background(), "python", "code", cases)
	assert.Equal(t, 2, s.Summary.Passed)
	assert.Equal(t, 0, s.Summary.Failed)
	assert.Equal(t, 2, s.Summary.Total)
}

func TestExecuteCases_OneFail_HiddenLeaksNoDetail(t *testing.T) {
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, stdin string) (*sandbox.PistonExecuteResponse, error) {
			if stdin == "wrong" {
				return makeStubResp("not what was expected"), nil
			}
			return makeStubResp(stdin), nil
		},
	}}
	cases := []store.TestCase{
		caseFor("ex", "hi", ptrStr("hi"), true),
		caseFor("hidden", "wrong", ptrStr("wrong"), false),
	}
	s := h.executeCases(context.Background(), "python", "code", cases)
	assert.Equal(t, 1, s.Summary.Passed)
	assert.Equal(t, 1, s.Summary.Failed)

	var failedHidden *caseResult
	for i := range s.Cases {
		if s.Cases[i].Status == "fail" {
			failedHidden = &s.Cases[i]
		}
	}
	if assert.NotNil(t, failedHidden) {
		assert.Equal(t, "wrong_output", failedHidden.Reason)
		assert.False(t, failedHidden.IsExample, "the failing case is the hidden one")
	}
}

func TestExecuteCases_NormalizesTrailingNewlineAndCRLF(t *testing.T) {
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, _ string) (*sandbox.PistonExecuteResponse, error) {
			return makeStubResp("hello\n"), nil // Piston-like trailing \n
		},
	}}
	// Expected has CRLF line ending.
	cases := []store.TestCase{caseFor("c1", "x", ptrStr("hello\r\n"), true)}
	s := h.executeCases(context.Background(), "python", "code", cases)
	assert.Equal(t, 1, s.Summary.Passed, "trailing newline + CRLF should normalize away")
}

func TestExecuteCases_NormalizesTrailingWhitespacePerLine(t *testing.T) {
	// A student prints `Hello, World! ` with an accidental trailing space.
	// Exact-match would fail; our normalizer trims it.
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, _ string) (*sandbox.PistonExecuteResponse, error) {
			return makeStubResp("Hello, World! \n"), nil
		},
	}}
	cases := []store.TestCase{caseFor("c1", "x", ptrStr("Hello, World!"), true)}
	s := h.executeCases(context.Background(), "python", "code", cases)
	assert.Equal(t, 1, s.Summary.Passed, "stray trailing space should not fail a correct program")

	// Multi-line with trailing spaces on the first line.
	h2 := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, _ string) (*sandbox.PistonExecuteResponse, error) {
			return makeStubResp("first   \nsecond\n"), nil
		},
	}}
	cases2 := []store.TestCase{caseFor("c1", "x", ptrStr("first\nsecond"), true)}
	s2 := h2.executeCases(context.Background(), "python", "code", cases2)
	assert.Equal(t, 1, s2.Summary.Passed, "per-line trailing whitespace trimmed")
}

func TestExecuteCases_NilExpectedIsInformational(t *testing.T) {
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, _ string) (*sandbox.PistonExecuteResponse, error) {
			return makeStubResp("anything goes"), nil
		},
	}}
	cases := []store.TestCase{caseFor("c1", "in", nil, false)}
	s := h.executeCases(context.Background(), "python", "code", cases)
	assert.Equal(t, 1, s.Summary.Passed, "no expected_stdout = pass-on-execute")
}

func TestExecuteCases_TimeoutMarksTimeout(t *testing.T) {
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, _ string) (*sandbox.PistonExecuteResponse, error) {
			time.Sleep(perCaseTimeout + 100*time.Millisecond)
			return nil, context.DeadlineExceeded
		},
	}}
	cases := []store.TestCase{caseFor("c1", "x", ptrStr("y"), true)}
	s := h.executeCases(context.Background(), "python", "code", cases)
	assert.Equal(t, 0, s.Summary.Passed)
	assert.Equal(t, 1, s.Summary.Failed) // timeout counts as failed in summary
	assert.Equal(t, "timeout", s.Cases[0].Status)
}

// Total-budget exhaustion must mark in-flight stragglers 'skipped', not
// 'timeout'. Regression for the case-classification bug caught in code
// review of plan 026.
func TestExecuteCases_TotalBudgetExhaustion_MarksSkipped(t *testing.T) {
	// Temporarily shrink the total budget so this test doesn't have to wait
	// 12 seconds. (parallelism stays 4 — 5 cases each 2.5s on a 4s total
	// budget: 4 run, the 5th can't even start; some of the 4 may not finish.)
	origTotal := totalBudget
	origPerCase := perCaseTimeout
	defer func() {
		// NOTE: vars are const; can't restore. Guard at function level via
		// overrides inside the handler test isn't set up. Instead, we pick
		// timings relative to the existing constants so the test remains
		// valid if they change.
		_ = origTotal
		_ = origPerCase
	}()

	// We need: per-case > total / parallelism so the first batch can't
	// finish before total expires. perCaseTimeout=3s, totalBudget=12s,
	// parallelism=4 → 4 cases in parallel each sleeping ~2.8s finish just
	// before total expires. Add 1 more that gets stuck behind.
	// Use a stub that sleeps almost perCaseTimeout before returning.
	sleepDur := perCaseTimeout - 200*time.Millisecond // 2.8s
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, _ string) (*sandbox.PistonExecuteResponse, error) {
			time.Sleep(sleepDur)
			return makeStubResp("done"), nil
		},
	}}
	// 5 cases at parallelism=4 means one waits. With 4 running near-simultaneously
	// for ~2.8s each, the 5th starts after the first finishes at ~2.8s.
	// It needs another 2.8s → would finish at 5.6s. Total budget 12s, so
	// normally all finish. To force budget exhaustion, run 10 cases:
	// 10 * 2.8s / 4 parallelism = 7s (best case); with overhead, should finish
	// but we can't count on it. Better: override totalBudget via a shorter
	// parent context passed in.
	parent, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	cases := make([]store.TestCase, 8)
	for i := range cases {
		cases[i] = caseFor("c"+string(rune('0'+i)), "x", ptrStr("done"), i%2 == 0)
	}
	s := h.executeCases(parent, "python", "code", cases)

	// After 1s parent context expires: the first 4 cases still have 1.8s to
	// go so they get cancelled → skipped (since totalCtx derives from parent
	// AND wraps in its own totalBudget; parent expires first). The remaining
	// 4 never get a worker slot so they're skipped too.
	skipped := 0
	for _, r := range s.Cases {
		if r.Status == "skipped" {
			skipped++
		}
	}
	assert.GreaterOrEqual(t, skipped, 4, "at least 4 cases should be marked skipped when parent ctx expires")
	// And NOT marked timeout: timeout requires caseCtx.Err() && !totalCtx.Err()
	// which doesn't happen when the parent blew the lid off.
	for _, r := range s.Cases {
		if r.Status == "timeout" {
			t.Errorf("case %s marked timeout but parent ctx was exhausted → expected skipped", r.CaseID)
		}
	}
}

func TestExecuteCases_PersistsValidJSON(t *testing.T) {
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, stdin string) (*sandbox.PistonExecuteResponse, error) {
			return makeStubResp(stdin), nil
		},
	}}
	s := h.executeCases(context.Background(), "python", "code",
		[]store.TestCase{caseFor("c1", "hi", ptrStr("hi"), true)})
	blob, err := json.Marshal(s)
	assert.NoError(t, err)
	assert.Contains(t, string(blob), `"passed":1`)
	assert.Contains(t, string(blob), `"total":1`)
}
