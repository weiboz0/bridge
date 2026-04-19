# 027 — Legacy Classroom Cleanup

**Goal:** Remove the pre-course-hierarchy `classrooms` + `classroom_members` tables and their entire call chain, and rename `new_classrooms` → `class_settings` so the taxonomy has one word per concept.

**Architecture:** Fully orphaned code gets deleted (no modern portal links to `/dashboard/classrooms/*`); the only live dependency is `new_classrooms` which keeps its rows, just changes its name and the struct+function names that touch it. Phases are ordered so each one leaves the tree compiling and the app working.

**Tech Stack:** Go (store + handlers) · Next.js 16 (page/API route deletion) · Drizzle (schema.ts) · SQL migration.

**Branch:** `feat/027-legacy-classroom-cleanup`

**Prereqs:** None. This is pure cleanup; no new features depend on it.

---

## What gets deleted

**Frontend (entire `/dashboard/*` tree — self-contained, nothing outside links in):**

```
src/app/dashboard/
├── layout.tsx
├── page.tsx                              (legacy homepage — lists classrooms)
└── classrooms/
    ├── page.tsx                          (redirects to /dashboard)
    ├── new/page.tsx
    ├── join/page.tsx
    └── [id]/
        ├── page.tsx
        ├── editor/page.tsx
        └── session/[sessionId]/
            ├── page.tsx
            └── dashboard/page.tsx
```

**Next.js API routes:**
```
src/app/api/classrooms/
├── route.ts
├── [id]/
│   ├── route.ts
│   └── members/route.ts
└── join/route.ts
```

**Legacy lib:**
- `src/lib/classrooms.ts`

**Go backend:**
- `platform/internal/handlers/classrooms.go` — `ClassroomHandler` + its registration
- `platform/internal/store/classrooms.go` — `ClassroomStore`
- `platform/internal/store/classrooms_test.go`
- Stores wiring in `platform/internal/handlers/stores.go`
- Main-router registration in `platform/cmd/api/main.go`

**Test files referencing legacy tables:**
- `tests/api/classrooms.test.ts`
- `tests/api/classrooms-join.test.ts`
- `tests/integration/classrooms-api.test.ts`

## What gets renamed

**`new_classrooms` → `class_settings`** (table + struct + function names; row contents unchanged).

Consumers to update:
- Migration: `ALTER TABLE new_classrooms RENAME TO class_settings;`
- `platform/internal/store/classes.go` — `NewClassroom` struct → `ClassSettings`; `GetClassroom` → `GetClassSettings`; INSERT in `CreateClass` uses new name.
- `platform/internal/handlers/ai.go:110` — caller.
- `src/lib/db/schema.ts` — drizzle definition.
- Various store tests that `DELETE FROM new_classrooms` for cleanup.
- `scripts/seed_*` seed files that insert into `new_classrooms`.

## What gets dropped (schema)

- `DROP TABLE classrooms CASCADE;`
- `DROP TABLE classroom_members CASCADE;`

---

## Tasks

### Task 1: Delete the legacy frontend tree

**Files:**
- Delete: `src/app/dashboard/` (entire subtree)
- Delete: `src/app/api/classrooms/` (entire subtree)
- Delete: `src/lib/classrooms.ts`
- Delete: `src/components/session/session-controls.tsx` (only consumer is dashboard/classrooms/[id]/page.tsx)
- Modify: `src/middleware.ts` — if it routes `/dashboard/*` explicitly, remove that branch

Safe to delete because:
- Audited: no file outside `/dashboard/*` links to `/dashboard/...` or imports `src/lib/classrooms`
- Modern portals (`/student`, `/teacher`, `/admin`, `/org`, `/parent`) have their own pages for every flow

- [ ] Delete all the listed paths.
- [ ] `node_modules/.bin/tsc --noEmit` — surface any broken imports; fix by deleting or re-pointing.
- [ ] `node_modules/.bin/vitest run` — legacy test files also go (Task 5 covers this).
- [ ] `bun run dev` — confirm the app still boots and `/teacher`, `/student`, `/admin` still render.
- [ ] Commit.

```bash
git rm -r src/app/dashboard/
git rm -r src/app/api/classrooms/
git rm src/lib/classrooms.ts
git rm src/components/session/session-controls.tsx
git add src/middleware.ts  # if modified
git commit -m "chore(027): delete legacy /dashboard/classrooms UI + Next.js API"
```

---

### Task 2: Delete Go ClassroomHandler + ClassroomStore

**Files:**
- Delete: `platform/internal/handlers/classrooms.go`
- Delete: `platform/internal/store/classrooms.go`
- Delete: `platform/internal/store/classrooms_test.go`
- Modify: `platform/internal/handlers/stores.go` — drop `Classrooms *store.ClassroomStore` field + its `NewClassroomStore(db)` line.
- Modify: `platform/cmd/api/main.go` — drop `classroomH := &handlers.ClassroomHandler{...}` + `classroomH.Routes(r)`.

Any remaining references to `stores.Classrooms` or `ClassroomStore` in tests need to go too — grep surfaces them.

- [ ] Delete the files.
- [ ] `cd platform && go build ./...` — confirm build clean.
- [ ] `go test ./...` — confirm no test refs the deleted store.
- [ ] Commit.

---

### Task 3: Rename `new_classrooms` → `class_settings` (schema migration)

**Files:**
- Create: `drizzle/0011_class_settings.sql`

```sql
-- Rename new_classrooms → class_settings (same rows, clearer name).
-- Plan 027.

ALTER TABLE new_classrooms RENAME TO class_settings;

-- Indexes renamed automatically when the table renames in Postgres,
-- but the constraint/index NAMES don't. Rename for clarity:
ALTER INDEX new_classrooms_pkey RENAME TO class_settings_pkey;
ALTER INDEX new_classrooms_class_idx RENAME TO class_settings_class_idx;
-- The old unique constraint on class_id stays valid; rename:
-- (constraint name is auto-generated; check with \d new_classrooms before
--  the rename and fill in the actual name if it differs)
```

- [ ] Run `psql $DATABASE_URL -c "\d new_classrooms"` and capture the actual index names before the migration.
- [ ] Apply to dev DB.
- [ ] Apply to test DB.
- [ ] Confirm `\d class_settings` shows all indexes and FK references intact.

---

### Task 4: Update Go callers

**Files:**
- Modify: `platform/internal/store/classes.go`
  - Rename struct `NewClassroom` → `ClassSettings`.
  - Rename method `GetClassroom(ctx, classID) (*NewClassroom, error)` → `GetClassSettings(ctx, classID) (*ClassSettings, error)`.
  - Update the `CreateClass` INSERT to `INSERT INTO class_settings (...)`.
  - Update the SELECT in `GetClassSettings` to `FROM class_settings`.
- Modify: `platform/internal/handlers/ai.go:110` — `h.Classes.GetClassSettings(...)`.
- Modify: any store test files that `DELETE FROM new_classrooms` (cleanup queries) → `DELETE FROM class_settings`.
  - `platform/internal/store/classes_test.go`
  - `platform/internal/store/courses_test.go`
  - `platform/internal/store/schedule_test.go`
  - `platform/internal/store/sessions_test.go`
  - `platform/internal/store/assignments_test.go`

The rename is mechanical. Run Go-side:

```bash
grep -rn "new_classrooms\|NewClassroom\b\|GetClassroom\b" platform/
# confirm zero matches after the rename
```

- [ ] Rename struct + methods.
- [ ] Update cleanup SQL in test files.
- [ ] `cd platform && go build ./...` + `DATABASE_URL=... go test ./... -count=1` — all green.
- [ ] Commit.

---

### Task 5: Update frontend schema + seeds

**Files:**
- Modify: `src/lib/db/schema.ts` — rename Drizzle table `new_classrooms` → `class_settings`; drop `classrooms` + `classroom_members` definitions entirely.
- Modify: `scripts/seed_problem_demo.sql` — `INSERT INTO new_classrooms` → `INSERT INTO class_settings`.
- Modify: `scripts/seed_python_101.sql` — same.
- Delete: `tests/api/classrooms.test.ts`
- Delete: `tests/api/classrooms-join.test.ts`
- Delete: `tests/integration/classrooms-api.test.ts`
- Modify: `tests/unit/schema.test.ts` — remove any test that references the deleted tables.
- Modify: `tests/helpers.ts` — if it creates/deletes classroom rows, drop those bits.

Regex fallout check:

```bash
grep -rn "classrooms\|classroom_members\|new_classrooms\|NewClassroom\|ClassroomStore\|ClassroomHandler" \
  src/ platform/ server/ tests/ scripts/
# should surface only the renamed class_settings references
```

- [ ] Apply edits.
- [ ] Re-apply both seed scripts to dev DB (idempotent — validates the table name).
- [ ] Run full Vitest; confirm pre-existing failures are unchanged but no NEW failures appear.
- [ ] Commit.

---

### Task 6: Drop legacy tables

**Files:**
- Create: `drizzle/0012_drop_legacy_classrooms.sql`

```sql
-- Drop the pre-course-hierarchy classrooms tables. The data model has
-- been superseded by classes + class_memberships + class_settings
-- (plan 004 + plan 027). No rows here are still wired to live code paths.
-- Plan 027.

DROP TABLE IF EXISTS classroom_members CASCADE;
DROP TABLE IF EXISTS classrooms CASCADE;
```

- [ ] Before applying: `psql $DATABASE_URL -c "SELECT COUNT(*) FROM classrooms; SELECT COUNT(*) FROM classroom_members;"` — note the row counts so we understand what's being discarded.
- [ ] Confirm nothing in Go or client code still references these tables (grep from Task 5).
- [ ] Apply to dev + test DBs.
- [ ] `cd platform && DATABASE_URL=... go test ./... -count=1` — green.
- [ ] `node_modules/.bin/vitest run` — no new failures.
- [ ] Commit.

---

### Task 7: Update taxonomy docs

**Files:**
- Modify (if it exists): any README / taxonomy diagram mentioning `classrooms` or `new_classrooms`. Search `docs/` for occurrences.
- Optional: commit a fresh `docs/taxonomy.md` reflecting the post-027 state.

- [ ] Commit.

---

### Task 8: Verify + review + PR

- [ ] `cd platform && DATABASE_URL=... go test ./... -count=1 -timeout 180s` — green.
- [ ] `node_modules/.bin/tsc --noEmit` — clean on new files; pre-existing errors unchanged (lesson-content, user-actions `asChild`).
- [ ] `node_modules/.bin/vitest run` — same pre-existing 40-ish failures, no NEW failures.
- [ ] `bun run test:e2e` — all 65 tests still pass.
- [ ] Manual smoke: teacher logs in, creates a session on a class, student joins, editor works. (Exercises the renamed `class_settings` table via `CreateClass` + `GetClassSettings`.)
- [ ] Code review pass, address findings.
- [ ] Post-execution report appended to this plan.
- [ ] PR open.

---

## Non-goals

- **Rename anything else.** `new_classrooms` → `class_settings` is the only rename. Other tables (classes, class_memberships, live_sessions) are fine as-is — spec 010 handles `live_sessions` → `sessions` in its own plan.
- **Migrate any remaining legacy-DB rows.** The 2 rows in `classrooms` + `classroom_members` are test artifacts from pre-portal days; they're being dropped, not migrated.
- **Touch the AI chat handler's session context logic** beyond the mechanical rename. Any deeper AI refactor is out of scope.
- **Cleanup of the `documents` table** (old Yjs persistence for session scratch code). The student/teacher session pages still use it. Its cleanup is a follow-up after spec 011 (portal UI redesign) lands.

## Risks

- **`classrooms` still appears in `live_sessions.classroom_id` column name? No — plan 022 renamed it to `class_id`.** Double-check with `\d sessions` (post-merge) that the column is `class_id`. If any row still has a lingering `classroom_id` reference somewhere we missed, the CASCADE drop would fail — hence the explicit grep gate in Task 6.
- **Drizzle migration order.** Drizzle reads migrations in filename order. The rename (0011) must run before the drop (0012). Filenames already enforce this.
- **Pre-existing E2E test `access-control.spec.ts`** tests `/dashboard/*` redirect? Unlikely — the modern tests target `/teacher`, `/student`, etc. Verify before Task 1 deletes the pages.
- **Other Claude Code sessions** may have outdated checkouts and touch deleted files. Usual multi-agent rule: pull before resuming work.
