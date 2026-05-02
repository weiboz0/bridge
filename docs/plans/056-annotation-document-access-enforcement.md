# Plan 056 — Annotation document access enforcement (P1)

## Status

- **Date:** 2026-05-01
- **Severity:** P1 (annotation tampering + teacher feedback leak)
- **Origin:** Review `009-...:33-39`.

## Problem

The annotation handler at `platform/internal/handlers/annotations.go:28-119` and the store at `platform/internal/store/annotations.go:66` perform direct `code_annotations` operations with no authorization beyond `claims != nil`. Anyone authenticated can:

| Endpoint | File:Line | Failure |
|---|---|---|
| `POST /api/annotations` | `annotations.go:28` | Create against any `documentId` |
| `GET /api/annotations?documentId=...` | `annotations.go:57` | Read teacher feedback for any document |
| `DELETE /api/annotations/{id}` | `annotations.go:85` | Delete anyone's annotation |
| `PATCH /api/annotations/{id}` | `annotations.go:100` (resolve) | Tamper with resolution state |

The store layer `CreateAnnotation` and `ListAnnotations` (`store/annotations.go:66`) takes `documentId` directly with no JOIN to verify document ownership or class context.

## Out of scope

- The annotation data model itself (no schema change).
- AI-generated annotations (separate flow; not exposed by these endpoints today).

## Approach

Three-layer fix:

1. **Restrict accepted documentId prefixes.** Annotations today only target `session:<sessionId>:user:<userId>`. Other realtime doc shapes (`attempt:*`, `unit:*`, `broadcast:*`) are not annotation surfaces. Reject anything else with 400 — explicitly, with a test.
2. **Resolve the session.** Parse `session:<sid>:user:<uid>` → load the session → derive class/owner.
3. **Apply role-aware access:**
   - Read (List): doc owner (the student) OR session teacher OR class staff (instructor/TA/org_admin) OR platform admin. Anyone else → 404.
   - Create / Delete / Resolve: **teacher only** — session teacher OR class staff (instructor/TA/org_admin) OR platform admin. Students do NOT create or modify annotations: the existing UI confirms (`AnnotationForm` is rendered only in the teacher dashboard at `teacher-dashboard.tsx:371`; no student-side annotation form exists). The store also hardcodes `AuthorType: "teacher"`.

**Deny-shape rule** (matches `classes.go:193-195`, `sessions.go:423-426`, `problems.go:359-360, 378-384, 433-439`):

- **404** — when the caller lacks READ access to the annotation's document. Don't leak existence.
- **403** — when the caller CAN read (i.e., they're the doc owner or have class-roster authority) but lacks the teacher-tier authority needed for the mutation. Example: a student CAN read their own doc's annotations (list = 200), but they can't create/delete/resolve them (= 403).

Concretely the matrix shape is:

| Actor | List | Create / Delete / Resolve |
|---|---|---|
| Doc owner (student on own doc) | 200 | 403 |
| Other student in same class | 404 | 404 |
| Student in different class / outsider | 404 | 404 |
| Session teacher / class instructor / TA / org_admin / platform admin | 200 | 200 |
| Non-`session:` documentId | 400 | 400 |

## Files

- Modify: `platform/internal/handlers/annotations.go`
  - Add `requireAnnotationAccess(ctx, claims, documentID, level)` helper. Returns the resolved session + role assignment, or an `authDecision`-shaped error (status + message).
  - Helper uses `RequireClassAuthority(ctx, h.Classes, h.Orgs, claims, *session.ClassID, AccessRoster)` for the class-staff path — same pattern plan 052/053 established.
  - Add `Sessions`, `Classes`, `Orgs` fields to `AnnotationHandler`. Wire from `main.go`.
  - Wire the helper into Create/List/Delete/Resolve.
- Modify: `platform/internal/store/annotations.go`
  - Add `GetAnnotation(ctx, id) (*Annotation, error)` so Delete/Resolve can fetch the annotation, derive its `documentID`, then authorize.
- Modify: `platform/cmd/api/main.go` — pass `stores.Sessions`, `stores.Classes`, `stores.Orgs` into `AnnotationHandler`.
- Modify: `platform/internal/handlers/annotations_test.go` — replace synthetic `d1` documentIds with real session-shaped IDs, add cross-user denial matrix:
  - student in same class on **own** doc — list 200, create/delete/resolve 403 (CAN read, can't write).
  - student in same class on another student's doc — list 404, create/delete/resolve 404.
  - student in different class — 404 across the board.
  - teacher of session — list/create/delete/resolve 200.
  - class instructor (not session teacher) — list/create/delete/resolve 200.
  - class TA — list/create/delete/resolve 200 (AccessRoster covers TAs).
  - org_admin of class's org — list/create/delete/resolve 200.
  - platform admin — list/create/delete/resolve 200.
  - outsider (no membership) — 404 across the board.
  - documentId with non-`session:` prefix → 400.
- Modify: `tests/integration/annotations-api.test.ts` — same matrix at the API layer.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Document-ID parsing is brittle | medium | The format is `session:<id>:user:<id>` — tested in plan 028 (annotations spec). Extract the parser into a single helper and reuse. |
| Performance: every annotation call now does a class-membership lookup | low | One indexed JOIN per request. Caching at the helper level if needed. |
| Existing teacher tooling that lists annotations across multiple students breaks | low | Teachers retain the read path through their class membership; the matrix tests confirm. |

## Phases

### Phase 0: pre-impl Codex review

### Phase 1: helper + handler wiring + matrix tests + smoke

## Codex Review of This Plan

### Pass 1 — 2026-05-02: BLOCKED → fixes folded in

Codex found 5 blockers; plan revised inline:

1. **Student create/delete is wrong.** No student-side AnnotationForm
   exists; store hardcodes `AuthorType: "teacher"`. Plan now says
   create/delete/resolve are teacher-only (session teacher / class
   staff / org_admin / platform admin). Students retain READ access
   to their own doc's annotations.

2. **DocumentId scope under-specified.** Today annotations only target
   `session:*` docs. Plan now explicitly rejects other prefixes
   (`attempt:*`, `unit:*`, `broadcast:*`) with 400 + a test.

3. **`GetAnnotation(ctx, id)` is missing.** Delete/Resolve need it
   to look up the annotation's documentID before authorizing. Plan
   now adds the store method.

4. **`AnnotationHandler` needs `Sessions`/`Classes`/`Orgs` stores.**
   Plan now wires them from main.go and uses the existing
   `RequireClassAuthority(...AccessRoster)` helper from plan 052.

5. **Tests use synthetic `d1` documentIds and have no cross-user
   denial coverage.** Plan now specifies a 9-case matrix at both
   handler and integration level using session-shaped IDs from a
   real fixture.

### Pass 2 — 2026-05-02: 1 blocker, fixed inline

Codex caught a self-contradiction: the Approach said "Create/delete/
resolve return 403" globally, but the matrix had same-class
other-student mutation as 404. Resolution: the helper distinguishes
read-deny (404) from write-deny (403) based on whether the caller has
read-level access at all. A student on their OWN doc can read but
not write → 403 on mutations. A student on another student's doc has
no read access → 404 across the board. Matrix table now spells this
out explicitly. AccessRoster confirmed correct (allows TAs, matches
existing grading/teacher patterns).

### Pass 3 — 2026-05-02: **CONCUR**

Plan is clear to proceed to implementation.
