# Plan 071 — Problem-bank backend error shaping

## Status
Draft. Pending Codex pre-impl review.

Two small, surgical backend changes that close UX gaps Codex flagged during plan 066's post-impl reviews. Both already have `[OPEN]` deferrals recorded in plan 066's §Code Review:
- Phase 3 NIT-2 (slug 409 mapping)
- Phase 4 NIT (empty-stdin validation)

This plan ships them as a single tiny PR.

## Problem

### 1. Slug uniqueness collisions surface as opaque 500s

Both `CreateProblem` and `UpdateProblem` in `platform/internal/handlers/problems.go` return `http.StatusInternalServerError` ("Database error" or "Failed to create problem") on any store error. The `problems` table has a partial-unique index on `(scope, COALESCE(scope_id::text, ''), slug)` (`drizzle/0013_problem_bank.sql:171-172`, `problems_scope_slug_uniq`). When a teacher picks a slug that already exists in the same scope, the insert fails with SQLSTATE `23505` against that constraint — but the user sees a generic banner saying "Save failed" with no hint that the slug field is the culprit.

The Next-side form (`src/components/problem/problem-form.tsx`) already has the surfacing wired (`readError` in the same file), but it can't distinguish a slug collision from any other 500 because the response body and status are the same.

### 2. Empty stdin on test cases is accepted server-side

`CreateTestCase` and `UpdateTestCase` (`platform/internal/handlers/problems.go:643, 734`) don't validate that `stdin` is non-empty. The Next-side editor (`src/components/problem/test-case-editor.tsx`) does, but a direct API caller (curl, scripted importer, future client) can write empty-input rows that fail the executor downstream with no useful error.

## Approach

### 1. Slug 409 mapping

Mirror the existing pattern in `platform/internal/store/teaching_units.go:310-328` — its `isUniqueViolationOn(err, constraint)` helper handles both `lib/pq.Error` and `pgx/pgconn.PgError` shapes. Promote that helper into a shared package (`platform/internal/store/dberr.go`) so both `teaching_units.go` and `problems.go` can call it without duplication.

The `problems.go` store wraps the unique-violation case into a typed sentinel error `ErrSlugConflict` (mirroring how `teaching_units.go::CreateUnit` already wraps its `teaching_units_topic_id_uniq` collision). Handler `errors.As`-tests it and returns `409 Conflict` with body `{"error": "Slug already taken in this scope", "field": "slug"}`.

The Next form's `readError` gets a 409 special-case that pins the message to the slug field inline.

### 2. Empty-stdin validation

Add a `body.Stdin == ""` check in `CreateTestCase` (after `decodeJSON`) and `UpdateTestCase` (when `body.Stdin != nil`, gate `*body.Stdin == ""`). Return `400 Bad Request` with `{"error": "stdin is required"}`.

## Out of scope

- Slug FORMAT validation (regex, length cap). The plan-066 post-impl review confirmed the backend has no slug-format constraint today; if we add one, the error shape needs to grow a separate `slug-format-invalid` 400 distinct from the 409. That's a separate decision and arguably orthogonal to this PR.
- Stdin LENGTH cap. The store doesn't have one; adding one risks breaking existing canonical cases imported from the Python 101 set.
- Test-case `expectedStdout` length cap. Same.

## Decisions to lock in

1. **Promote `isUniqueViolationOn` to a shared file** (`platform/internal/store/dberr.go`). Two callers today, more in the future. The function is 18 lines including the dual-driver fallthrough; copy-pasting is worse than the import.

2. **Sentinel error type, not a string check** in the handler. `var ErrSlugConflict = errors.New(...)` so `errors.Is(err, ErrSlugConflict)` works cleanly. (Codex pre-impl review of plan 066 phase 3 NIT-2 explicitly recommended this shape.)

3. **Response body shape on 409**: `{"error": "Slug already taken in this scope", "field": "slug"}`. Mirrors the existing 4xx body convention in the codebase (`{"error": ...}` is the only field used today; we add `field` as additive metadata for the form to consume without breaking existing callers).

4. **Field name is `slug`**, not `request.slug` or `body.slug`. The form already names its inputs by the JSON key.

5. **Empty stdin returns 400, not 422**. The codebase uses 400 for all client validation rejections (see `validProblemScopes`, `validProblemDifficulties` paths). 422 would be a one-off divergence.

## Files

### Phase 1 — slug-409 mapping

**Add:**
- `platform/internal/store/dberr.go` — `IsUniqueViolationOn(err error, constraint string) bool`. Promoted from `teaching_units.go::isUniqueViolationOn` (rename to exported because it's now reused). Same dual-driver implementation.
- `platform/internal/store/problems.go` — define `ErrSlugConflict` sentinel near the top of the file. In `CreateProblem` and `UpdateProblem`, after the SQL exec/scan, wrap a unique-violation against `problems_scope_slug_uniq` into `ErrSlugConflict`.

**Modify:**
- `platform/internal/store/teaching_units.go` — replace local `isUniqueViolationOn` calls with the shared `IsUniqueViolationOn`. Delete the local copy.
- `platform/internal/handlers/problems.go` — in `CreateProblem` and `UpdateProblem`, after the store call, `errors.Is(err, store.ErrSlugConflict)` → return 409 with `{"error": "Slug already taken in this scope", "field": "slug"}`. Other errors keep the existing 500 mapping.
- `src/components/problem/problem-form.tsx` — in `readError`, special-case 409 with `body.field === "slug"`. Surface inline next to the slug input rather than the generic banner. Add a `slugError` state to the form for this purpose.

### Phase 2 — empty-stdin validation

**Modify:**
- `platform/internal/handlers/problems.go::CreateTestCase` — add `if body.Stdin == "" { writeError(w, 400, "stdin is required"); return }` after the existing decode + canonical-auth check.
- `platform/internal/handlers/problems.go::UpdateTestCase` — add `if body.Stdin != nil && *body.Stdin == "" { writeError(w, 400, "stdin is required"); return }` after `decodeJSON`. Note: `nil` (unchanged) is still allowed — the validation only fires if the caller is explicitly trying to set stdin to empty.

### Phase 3 — tests

**Add:**
- `platform/internal/store/problems_test.go` — `TestCreateProblem_SlugConflict` covering: insert problem A with `slug: "two-sum"`; insert problem B with same `slug` + same scope; verify the second returns `ErrSlugConflict`. Also `TestCreateProblem_SlugAllowedInDifferentScope` to confirm the partial-unique scope still allows the same slug across `personal` users.
- `platform/internal/store/problems_test.go` — `TestUpdateProblem_SlugConflict` covering: update an existing problem to a slug owned by another problem in the same scope.
- `platform/internal/handlers/problems_test.go` (or wherever the existing handler-level tests live — verify location) — `TestCreateProblem_Returns409OnSlugConflict` and `TestCreateTestCase_Returns400OnEmptyStdin`.
- `tests/unit/problem-form.test.tsx` (NEW — no existing form-level test) — covers the 409 → inline-slug-error mapping in `readError`.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| The exported `IsUniqueViolationOn` rename misses a call-site in `teaching_units.go` | low | grep-and-rename across the package; the linter catches the leftover. |
| 409 mapping accidentally fires for non-slug unique violations | low | Constraint name is checked literally (`problems_scope_slug_uniq`); other unique constraints fall through to 500 unchanged. |
| Form regression — slug error rendered twice (once inline, once in banner) | low | The 409 handler returns and clears the generic banner state in the same setState. Test in `problem-form.test.tsx` covers it. |
| Empty-stdin rejection breaks existing canonical-case imports | low | The Python 101 importer always sends non-empty stdin. Spot-check `content/python-101/` test_cases.yaml before merging Phase 2. |
| Existing unit test reads `Failed to create problem` literal in the response | low | Audit `tests/` and `e2e/` for the substring before changing the 500 → 409 wording. |

## Phases

### Phase 0: Pre-impl Codex review

Per CLAUDE.md plan-review gate. Dispatch `codex:codex-rescue` to review against:
- `platform/internal/handlers/problems.go` (the routes the changes consume)
- `platform/internal/store/teaching_units.go:280-328` (the existing unique-violation pattern this plan mirrors)
- `platform/internal/store/problems.go::CreateProblem` (~line 207), `::UpdateProblem` (~line 401)
- `drizzle/0013_problem_bank.sql:171-172` (the constraint name)

Specific questions:
1. Is there a reason the existing `isUniqueViolationOn` in `teaching_units.go` isn't already exported and shared? Was there a deliberate scoping decision I'd be breaking by promoting it?
2. The handler currently returns `Internal Server Error` (500) for ALL store errors including not-found. With the 409 mapping in place, does anything else change shape? (E.g., does `errors.Is(err, store.ErrNotFound)` exist that I'm missing — should I be wrapping multiple sentinels?)
3. Does `pgconn.PgError.ConstraintName` always populate when the unique constraint is a partial index? Or only on a regular unique constraint? The slug constraint is a partial index (`WHERE slug IS NOT NULL`) — verify the constraint name surfaces.
4. Is `{"field": "slug"}` an acceptable additive shape for the 409 body, or does the codebase have a different convention I'm missing?
5. Empty-stdin gate — anything else I should validate while in there (max length, ASCII-only, line-ending normalization)?
6. Do existing tests assert the literal "Database error" string for slug conflicts? If so, the 409 mapping breaks them; flag for rewriting.

### Phase 1: Slug 409 mapping (PR 1)

- Promote `IsUniqueViolationOn` to `dberr.go`.
- Define `ErrSlugConflict`.
- Wrap CreateProblem + UpdateProblem store paths.
- Map 23505 → 409 in the handler.
- Update `readError` in problem-form.tsx for inline slug errors.
- Codex post-impl review.
- PR + merge.

### Phase 2: Empty-stdin validation (PR 2)

- Add the 400 check in CreateTestCase + UpdateTestCase.
- Codex post-impl review.
- PR + merge.

### Phase 3: Tests (PR 3, optional separate-or-bundled)

- Store-level slug-conflict tests.
- Handler-level 409 tests.
- Form-level 409→inline test.
- Empty-stdin handler test.
- PR + merge.

(Phase 3 may be folded into Phase 1 + 2 PRs depending on PR size during execution; if both phases stay small, keeping tests in the same PR as the change is preferred.)

## Codex Review of This Plan

(pending)
