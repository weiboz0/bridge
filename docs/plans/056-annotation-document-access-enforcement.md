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

Two-layer fix:

1. **Resolve the target document.** Annotations are keyed on `documentId`, which is `session:<sessionId>:user:<userId>` for student session docs (per the legacy classroom format). The handler must resolve `documentId` → underlying session/class/attempt → apply membership rules.
2. **Apply role-aware access:**
   - Document owner (the student in the session) can always read/create/delete their own annotations.
   - Class instructor / TA / org_admin of the class's org / platform admin can read all annotations for documents in their class context AND create new ones.
   - Anyone else: 404.

Returning 404 (not 403) matches existing patterns where document existence shouldn't leak by ID.

## Files

- Modify: `platform/internal/handlers/annotations.go` — add `requireAnnotationAccess(ctx, claims, documentID, level)` helper. Wire into the four endpoints.
- Modify: `platform/internal/store/annotations.go` — accept the access decision; otherwise the store stays simple.
- Modify: `platform/internal/handlers/annotations_test.go` and `tests/integration/annotations-api.test.ts` — extend with the cross-user denial matrix (outsider/student-in-other-class/teacher-of-different-class/instructor/platform-admin).
- Verify: existing tests still pass.

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

(Filled in after Phase 0.)
