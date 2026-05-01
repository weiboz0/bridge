# Plan 054 — Drop stale `documents.classroom_id` from Drizzle schema (P1)

## Status

- **Date:** 2026-05-01
- **Severity:** P1 (schema drift; bombs Drizzle-generated migrations)
- **Origin:** Reviews `008-...:25-31` and `009-...:49-55`.

## Problem

Migration `drizzle/0012_drop_legacy_classrooms.sql:47-49` dropped the `documents.classroom_id` column and its index. The Drizzle schema at `src/lib/db/schema.ts:405-427` still declares them:

```ts
classroomId: uuid("classroom_id"),
...
index("documents_classroom_idx").on(table.classroomId),
```

Plans 027 (the legacy classroom cleanup) and 048 (session agenda single-source) both rely on resolving navigation through a `LEFT JOIN sessions` instead, and `platform/internal/store/documents.go:98-101` already uses that join. The Drizzle schema is the only artifact that still believes the column exists.

Failure modes:
1. `bun run db:generate` will produce a migration that re-adds the dropped column on the next change to `documents`.
2. Any Drizzle query like `db.select({ classroomId: documents.classroomId })` against a migrated DB returns a column-doesn't-exist error.
3. Type-narrowing on the schema model gives consumers a phantom column in their type intellisense.

## Out of scope

- Other potential schema drift (audit separately if needed).
- The `@deprecated` comment that was added on the field — this plan removes the field entirely.

## Approach

Three steps:
1. Delete `classroomId` and `documents_classroom_idx` from the Drizzle schema.
2. Search for any remaining TS/JS references to `documents.classroomId` or `classroom_id` in document context. Update or delete.
3. Add a tiny test that asserts the column is NOT in the live DB (drizzle-kit introspect against a fresh migration apply, comparing column list).

## Files

- Modify: `src/lib/db/schema.ts:405-427` — remove field + index. Update `@deprecated` JSDoc comment on the table.
- Search: `grep -rn "classroomId\|classroom_id" src/ tests/` — update each hit.
- Add: `tests/integration/schema-documents-shape.test.ts` — query `information_schema.columns WHERE table_name = 'documents'` and assert `classroom_id` is NOT in the result. Add a similar assertion for any column the schema CLAIMS that the live DB DOESN'T have.
- Verify: `bun run db:generate` produces no migration for `documents` after this change (the schema now matches the applied migration chain).

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| A test or fixture still imports `documents.classroomId` | medium | The grep step finds them; update or delete. |
| `documents.test.ts` (if any) uses the legacy column | low | Same fix. |
| A Yjs-related path needs the column | very low | Plan 027 already audited and removed all callers. |

## Phases

### Phase 0: pre-impl Codex review

Dispatch on this plan + the search results from `grep -rn "classroomId\|classroom_id" src/ tests/`. Capture verdict.

### Phase 1: delete + audit + test

Single-commit change.

## Codex Review of This Plan

(Filled in after Phase 0.)
