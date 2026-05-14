# Plan 088 — Books library + rename teaching_units → chapters

## Problem

Today's content model is a single tree: **courses → topics → teaching_units** with `teaching_units.topic_id` UNIQUE (1:1). That collapses the "static curriculum library" and the "per-class course delivery" into one structure — topics function as labels on units rather than chapter containers, and there's no place for a publisher/author to organize curriculum into book-like volumes that span courses.

User intent (from brainstorming exchange):
- **Library** is static, book-organized: a `book` (e.g. "Python for K-8 Beginners") contains `chapters` (e.g. "Lists & Loops", "Functions"). Independent of courses.
- **Courses** stay as the curated delivery layer — a course pulls chapters from one or more books and pins them to course topics for delivery.
- **Rename units → chapters** to match the library mental model. A chapter is what teaching_units is today, just renamed.

Two changes in one plan: introduce a new `books` library entity, and rename `teaching_units` → `chapters` across DB / Go / TS / routes / tests.

## Approach

Four phases. Single-shot migration; no zero-downtime requirement.

- **Phase 1 — Backend (Codex)**: migration (table rename + add `books` table + `chapters.book_id`), Go code rename (`TeachingUnit` → `Chapter`, `teaching_units.go` → `chapters.go`, etc.), route flip (`/api/units/*` → `/api/chapters/*`), books CRUD (store + handlers), tests renamed + new book tests.
- **Phase 2 — Frontend rename (Sonnet)**: rename page directories (`teacher/units/*` → `teacher/chapters/*`, same for admin/org/student), components, types, tests. Add Next.js redirects from `/units/*` → `/chapters/*` for user bookmarks.
- **Phase 3 — Books UI (Sonnet)**: admin + teacher book list / detail / edit pages, book picker, chapter-list filter by book.
- **Phase 4 — Verify + docs**: full suite, smoke test, docs update.

### Phase 1 — Backend (Codex)

#### 1a. Migration

```sql
-- drizzle/00XX_books_and_chapters.sql

CREATE TYPE "public"."book_scope" AS ENUM ('platform', 'org', 'personal');

CREATE TABLE "books" (
  "id"          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "title"       varchar(255) NOT NULL,
  "description" text NOT NULL DEFAULT '',
  "scope"       "book_scope" NOT NULL,
  "scope_id"    uuid NULL,
  "created_by"  uuid NOT NULL REFERENCES users(id),
  "created_at"  timestamptz NOT NULL DEFAULT now(),
  "updated_at"  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT books_scope_id_required CHECK (
    (scope = 'platform' AND scope_id IS NULL) OR
    (scope IN ('org', 'personal') AND scope_id IS NOT NULL)
  )
);

CREATE INDEX books_scope_idx ON books(scope, scope_id);
CREATE INDEX books_created_by_idx ON books(created_by);

-- Rename teaching_units → chapters (table + indexes + FK names + sequences).
ALTER TABLE teaching_units RENAME TO chapters;
ALTER INDEX teaching_units_pkey RENAME TO chapters_pkey;
-- Rename every other index that starts with `teaching_units_*` (audit pre-impl
-- via `psql -d bridge_test -c "\d chapters"` after the rename to inventory the
-- old names; spec the new names in the migration).
-- All FK constraints from other tables to teaching_units(id) keep their
-- existing names; Postgres FKs don't care about the target table's name in
-- the constraint name, just the OID. But the FK-target-side checks update
-- automatically (referencing the renamed table).

ALTER TABLE chapters ADD COLUMN "book_id" uuid NULL REFERENCES books(id) ON DELETE SET NULL;
CREATE INDEX chapters_book_idx ON chapters(book_id);
```

**Migration safety**:
- Existing chapters all get `book_id = NULL` (Decision #4). Library is empty; future UI/backfill scripts assign chapters to books.
- `topic_id` UNIQUE constraint preserved (Decision #6).
- Existing course → topic → chapter delivery semantics unchanged.
- All test fixture builders need updating (they reference `teaching_units` by name).

#### 1b. Books store (NEW)

`platform/internal/store/books.go`:

```go
type Book struct {
    ID          string    `json:"id"`
    Title       string    `json:"title"`
    Description string    `json:"description"`
    Scope       string    `json:"scope"`     // 'platform' | 'org' | 'personal'
    ScopeID     *string   `json:"scopeId"`   // NULL for platform; orgID / userID otherwise
    CreatedBy   string    `json:"createdBy"`
    CreatedAt   time.Time `json:"createdAt"`
    UpdatedAt   time.Time `json:"updatedAt"`
}

type BookStore struct { db *sql.DB }

func (s *BookStore) CreateBook(ctx, CreateBookInput) (*Book, error)
func (s *BookStore) GetBook(ctx, bookID) (*Book, error)
func (s *BookStore) ListBooks(ctx, BookFilter) ([]Book, error) // filter by scope + scopeID
func (s *BookStore) UpdateBook(ctx, bookID, UpdateBookInput) (*Book, error)
func (s *BookStore) DeleteBook(ctx, bookID) error  // chapters' book_id set to NULL via ON DELETE SET NULL
```

`CreateBookInput` validates: title 1-255 chars; scope in {platform, org, personal}; scopeID required when not platform.

#### 1c. Chapters store rename

`platform/internal/store/teaching_units.go` → `platform/internal/store/chapters.go`. Rename:

- File: `teaching_units.go` → `chapters.go`. Test file: `teaching_units_test.go` → `chapters_test.go`.
- Struct: `TeachingUnit` → `Chapter`. `CreateTeachingUnitInput` → `CreateChapterInput`. `UpdateTeachingUnitInput` → `UpdateChapterInput`.
- Method names: `CreateUnit` → `CreateChapter`, `GetUnit` → `GetChapter`, `ListUnits` → `ListChapters`, `UpdateUnit` → `UpdateChapter`, `DeleteUnit` → `DeleteChapter`, etc.
- Add `BookID *string` field on the struct + the SELECT/Scan in all queries.
- All variable names: `u Unit` → `c Chapter`, `unitID` → `chapterID`.
- Comments: replace "unit" → "chapter" wherever the word refers to a unit-the-entity (avoid touching "unit test" usages).

Same rename applied to `unit_collections.go` → `chapter_collections.go` (and the test file).

#### 1d. Books handler (NEW)

`platform/internal/handlers/books.go`:

Routes:
- `POST /api/books` — create.
- `GET /api/books` — list with `?scope=&scopeId=` filter.
- `GET /api/books/{id}` — get.
- `PATCH /api/books/{id}` — update (title + description).
- `DELETE /api/books/{id}` — delete.

Auth: same `canViewUnit` / `canEditUnit` style helpers, applied per-scope. Wrap in a `RequireAuth` chain.

#### 1e. Chapters handler rename

`platform/internal/handlers/teaching_units.go` → `platform/internal/handlers/chapters.go`. Rename:

- File rename.
- Handler struct: `TeachingUnitHandler` → `ChapterHandler`.
- Method names: `CreateUnit` → `CreateChapter`, etc. (mirror store renames).
- Route paths: `/api/units/*` → `/api/chapters/*`. **No compat aliases at API level** (frontend is sole consumer; same-PR rename keeps them in sync). Specifically:
  - `POST /api/chapters`
  - `GET /api/chapters/search`
  - `GET /api/chapters/by-topic/{topicId}`
  - `GET /api/chapters/{id}`
  - `PATCH /api/chapters/{id}`
  - `DELETE /api/chapters/{id}`
  - `GET /api/chapters/{id}/document`
  - `PUT /api/chapters/{id}/document`
  - `GET /api/chapters/{id}/projected`
  - `POST /api/chapters/{id}/transition`
  - `GET /api/chapters/{id}/revisions`
  - `GET /api/chapters/{id}/revisions/{revisionId}`
  - `POST /api/chapters/{id}/fork`
  - `GET /api/chapters/{id}/overlay`
  - `PATCH /api/chapters/{id}/overlay`
  - `GET /api/chapters/{id}/composed`
  - `GET /api/chapters/{id}/lineage`
  - `POST /api/chapters/{id}/draft-with-ai` (`unit_ai.go` → `chapter_ai.go`)
  - `POST /api/chapters/{id}/ai-transform`
  - `GET /api/topics/{topicId}/problems` → unchanged (topic-side path)
- `validUnitStatuses` → `validChapterStatuses`. Helper functions throughout.

Add `book_id` to `Chapter` JSON shape; not required in `Create` / `Update` inputs (nullable).

#### 1f. Cross-cutting backend renames

- `platform/cmd/api/main.go` — handler wiring: `UnitHandler` → `ChapterHandler`. `unitStore` → `chapterStore`. New `bookHandler` + `bookStore`.
- `platform/internal/handlers/stores.go` — same.
- `platform/internal/handlers/access.go` — `canViewUnit` / `canEditUnit` → `canViewChapter` / `canEditChapter`. `CanViewUnit` (free helper) → `CanViewChapter`.
- `platform/internal/handlers/realtime_token.go` — unit-collab token references rename. Check if collab session keys use the word "unit" anywhere; rename to "chapter" if so.
- `platform/internal/handlers/sessions.go` — any chapter references rename.
- `platform/internal/handlers/topics.go` — `LinkUnitToTopic` → `LinkChapterToTopic`. Route path: `POST /api/courses/{courseId}/topics/{topicId}/link-unit` (if exists) → `link-chapter`.
- `platform/internal/handlers/topic_problems.go` — any unit references rename.
- All Go test files: rename + update fixture helpers.

#### 1g. Hocuspocus / collab document keys

`server/hocuspocus.ts` may reference unit IDs as document keys. Verify pre-impl: if keys are like `unit:{id}` or use a `unit-` prefix, those become `chapter:{id}` / `chapter-`. Existing in-flight collab sessions would lose their server-side document on key rename. Acceptable for a one-time migration since this is dev/test data only at current scale.

#### 1h. Backend tests

- Rename every `_test.go` file that matches `teaching_units_*` / `unit_*` per the store/handler renames.
- Update fixture helpers: `seedUnit` → `seedChapter`, etc.
- Add new books tests: `books_test.go` (store) + `books_integration_test.go` (handler).
- All existing tests for the renamed handlers should pass without behavioral changes — the rename is name-only.

### Phase 2 — Frontend rename (Sonnet)

#### 2a. Drizzle schema

`src/lib/db/schema.ts`: `teachingUnits` export → `chapters`. Add `books` export. Update all importers.

#### 2b. Page directory rename

```
src/app/(portal)/teacher/units/    → src/app/(portal)/teacher/chapters/
src/app/(portal)/admin/units/      → src/app/(portal)/admin/chapters/
src/app/(portal)/org/units/        → src/app/(portal)/org/chapters/
src/app/(portal)/student/units/    → src/app/(portal)/student/chapters/
```

Inside each: rename `[id]` route handlers' content. Update internal links between pages.

#### 2c. Component rename

| Old | New |
|-----|-----|
| `src/components/teacher/unit-picker-dialog.tsx` | `chapter-picker-dialog.tsx` |
| `src/components/teacher/unit-editor.tsx` (if exists) | `chapter-editor.tsx` |
| `UnitItem` TS type | `ChapterItem` |
| `Unit` interface usages | `Chapter` |

Audit: `grep -rln "Unit\b\|unit\b" src/components` (case-sensitive, word boundary) and rename references where they refer to chapters-the-entity, not generic "unit test" or "unit of measure" usages.

#### 2d. Fetch URLs

Every `fetch("/api/units/...")` in frontend code → `/api/chapters/...`. Audit:

```bash
grep -rn "/api/units" src/
```

#### 2e. Redirects for bookmarks

`next.config.ts` add redirect rules:

```ts
redirects: async () => [
  { source: "/teacher/units/:path*", destination: "/teacher/chapters/:path*", permanent: true },
  { source: "/admin/units/:path*",   destination: "/admin/chapters/:path*",   permanent: true },
  { source: "/org/units/:path*",     destination: "/org/chapters/:path*",     permanent: true },
  { source: "/student/units/:path*", destination: "/student/chapters/:path*", permanent: true },
]
```

`permanent: true` issues 308 (Next's preferred over 301 for typed methods).

#### 2f. Middleware

`src/middleware.ts` — check for unit-related route matchers; rename to chapter.

#### 2g. Tests

Rename `tests/unit/*-unit-*.test.tsx` (where they refer to the entity, not "unit test"). Update fetch mocks for the new paths.

### Phase 3 — Books UI (Sonnet)

#### 3a. Admin books list

`src/app/(portal)/admin/books/page.tsx`:
- List all platform books + optional org-filter for org-scope books.
- Columns: Title, Scope, Owner-org-or-platform, Chapter count, Updated, Actions.
- "+ New book" button → opens create dialog.

#### 3b. Admin book detail

`src/app/(portal)/admin/books/[id]/page.tsx`:
- Book metadata Card + chapter list (chapters where `book_id = {id}`).
- Edit button (opens `BookEditDialog`).
- "+ Add chapter" inline button (creates a new chapter pre-assigned to this book).

#### 3c. Org/teacher book list

`src/app/(portal)/teacher/books/page.tsx`:
- Filter: My personal books + my org's books + platform books.
- Same column shape as admin.

#### 3d. Components

- `src/components/books/book-edit-dialog.tsx` — Create/Edit form (title + description).
- `src/components/books/book-picker-dialog.tsx` — used by the chapter-edit page to assign a chapter to a book.
- `src/components/books/book-actions.tsx` — dropdown for delete + similar.

#### 3e. Chapter list filter

`src/app/(portal)/teacher/chapters/page.tsx` (renamed from `units/page.tsx`): add a "Book" filter dropdown (alongside existing filters). Server-side query param `?bookId=`.

#### 3f. Tests

- `tests/unit/admin-books-page.test.tsx`
- `tests/unit/admin-book-detail-page.test.tsx`
- `tests/unit/book-edit-dialog.test.tsx`
- `tests/unit/book-picker-dialog.test.tsx`
- `tests/unit/book-actions.test.tsx`

### Phase 4 — Verify + docs

- Full Vitest + Go test suite.
- Smoke-test in dev: create a book, create a chapter assigned to that book, see it in the chapter list filtered by book.
- `docs/api.md`: add `/api/books/*` section + rename `/api/units/*` references to `/api/chapters/*`.
- Update `docs/project-structure.md` if any directory references changed (it shouldn't — the doc is high-level).

## Files

**Migration (1)**:
- `drizzle/00XX_books_and_chapters.sql` — table rename + books table + index + FK.

**Backend Go — Rename (10+)**:
- `platform/internal/store/teaching_units.go` → `chapters.go` (+ test file)
- `platform/internal/store/unit_collections.go` → `chapter_collections.go` (+ test file)
- `platform/internal/handlers/teaching_units.go` → `chapters.go` (+ integration test)
- `platform/internal/handlers/unit_ai.go` → `chapter_ai.go` (+ test file)
- `platform/internal/handlers/unit_collections.go` → `chapter_collections.go` (+ test file)
- `platform/internal/handlers/topics.go` — `LinkUnitToTopic` rename
- `platform/internal/handlers/topic_problems.go` — references rename
- `platform/internal/handlers/access.go` — `canViewUnit` / `canEditUnit` rename
- `platform/internal/handlers/stores.go` — store wiring rename
- `platform/internal/handlers/realtime_token.go` — collab-key rename
- `platform/cmd/api/main.go` — handler wiring + books handler addition

**Backend Go — New (3)**:
- `platform/internal/store/books.go` + test file
- `platform/internal/handlers/books.go` + integration test file
- `platform/internal/store/dberr.go` — possibly new sentinel error for book ops

**Frontend TS — Rename (5 page dirs + ~10 components/types)**:
- `src/app/(portal)/{teacher,admin,org,student}/units/` → `.../chapters/`
- `src/components/teacher/unit-picker-dialog.tsx` → `chapter-picker-dialog.tsx`
- `src/lib/db/schema.ts` — `teachingUnits` → `chapters`
- `src/middleware.ts` — route matchers if any
- `next.config.ts` — add 308 redirects
- All TS test files referencing the rename

**Frontend TS — New (5 pages + 3 components + 5 tests)**:
- `src/app/(portal)/admin/books/page.tsx` + `[id]/page.tsx`
- `src/app/(portal)/teacher/books/page.tsx` + `[id]/page.tsx`
- `src/components/books/book-edit-dialog.tsx`
- `src/components/books/book-picker-dialog.tsx`
- `src/components/books/book-actions.tsx`
- 5 new test files (per §3f)

**Docs (2)**:
- `docs/api.md` — books endpoints + unit→chapter path renames
- Historical plan files in `docs/plans/` — DO NOT rename; archival record.

## Decisions to lock in

1. **Single-shot migration**. No zero-downtime requirement. The migration renames `teaching_units` → `chapters` and adds `books` + `chapters.book_id` in one transaction.
2. **API paths flip cleanly** (`/api/units/*` → `/api/chapters/*`); no API-level compat aliases. Frontend is the sole consumer; same-PR keeps them in sync.
3. **Frontend page paths get 308 redirects** (Next.js `permanent: true`) from `/units/*` → `/chapters/*` so user bookmarks don't 404.
4. **`chapters.book_id` is NULLABLE.** Migration leaves all existing chapters unfiled. Future plan can backfill (e.g., auto-create a default book per org and assign all that org's existing chapters to it). Tightening to NOT NULL is a future cleanup once UI surfaces all chapters into books.
5. **Books scope mirrors chapter scope**: `platform` / `org` / `personal`. Same `canView` / `canEdit` rules apply (platform admin → platform-scope; org admin + teacher → org-scope; owner → personal-scope).
6. **Keep `chapters.topic_id` UNIQUE constraint**. Books and topics are orthogonal axes — book is library organization, topic is course curation. A chapter can live in a book AND be pinned to a course topic.
7. **JSON field shapes don't change** outside of adding `bookId` to chapter responses. The `id`, `title`, `scope`, etc. fields stay the same names (they were never "unit"-prefixed).
8. **No default book auto-created**. Migration leaves existing chapters with `book_id = NULL`. UI surfaces them as "Unfiled" until an admin assigns them. Simpler migration, no implicit org-modeling decisions.
9. **Hocuspocus collab keys**: if they reference "unit" in the key, rename to "chapter" in the same PR. In-flight dev sessions lose state — acceptable at current scale.
10. **Plan files in `docs/plans/` keep their `unit` references** — archival record of decisions at the time. Only `docs/api.md` and forward-looking docs are updated.
11. **Topics stay untouched.** They remain children of courses and continue to point at chapters (via the renamed `topic_id` column on `chapters`). Plan 088 does not deprecate topics.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Mass rename touches many files; merge conflicts likely if other branches are active | high | Land plan 088 in a single PR; minimize concurrent unit-related work during the rebase window. Communicate. |
| Hocuspocus document keys may reference "unit" | medium | §1g — verify pre-impl; rename if needed. In-flight collab data loss acceptable (dev/test scale). |
| Existing migrations reference `teaching_units` | low | Past migrations stay as-is (they create/alter the old name; the new rename migration is the most recent + authoritative). Don't try to rewrite history. |
| Frontend redirect rules don't catch sub-paths if format mismatches | low | Tests after the rename: hit `/teacher/units` and `/teacher/units/abc` in dev; assert 308 → `/chapters/...`. |
| Existing test files use fixture helpers like `seedUnit` that will need rename | high | Phase 1 explicitly renames fixture helpers (`seedUnit` → `seedChapter`) and updates all callers in one pass. |
| FK constraints from other tables (assignments, scheduled_sessions, etc.) may break on table rename | low | Postgres FKs are by OID; renaming the target table leaves FKs intact. The constraint names on the target side may keep `teaching_units_*` prefix — cosmetic; can rename in a future cleanup. |
| `teaching_units_topic_id_uniq` index name awkward post-rename | low | Rename to `chapters_topic_id_uniq` in the migration. |
| Personal-scope books with no clear use case | medium | Decision #5 keeps the parallel structure but personal books may be unused. Acceptable — costs nothing, mirrors units. Future plan can deprecate if data shows no usage. |
| Chapter `book_id` UI: no way to assign a chapter to a book in Phase 1 | low | Phase 3 adds the picker. Phase 1 chapters all stay `book_id = NULL`. |
| Books table doesn't have a `status` column (active/archived) like orgs do | low | v1 doesn't need archival semantics. Future plan can add when needed. |
| Search / "find a chapter" code paths still reference `teaching_units` in SQL string literals | medium | Phase 1 audit + rename. Will catch via failing tests. |
| Course-side topics that already 1:1 to a chapter break if anyone tries to assign a second chapter to the same topic | low | Existing UNIQUE constraint preserved; behavior unchanged. |
| Drizzle migration generator produces a different SQL than the hand-written file | low | After editing `schema.ts`, run `bun run db:generate`. If different, prefer the generated version + adjust the plan. |
| Bookmarks pointing at deep unit edit routes (`/teacher/units/abc/edit`) | low | 308 redirects with `:path*` cover all sub-paths. |
| Multi-org chapter visibility in a book | low (out of scope) | Per Decision #5, a book is scoped. Cross-org book sharing is out of scope; would need a separate sharing/publishing model. |

## Phases

### Phase 1 — Backend (Codex)

1. **Pre-impl audit**:
   - `psql -d bridge_test -c "\d teaching_units"` to inventory all current indexes + constraints for the rename migration.
   - `grep -rln "teaching_units\|TeachingUnit\|teachingUnits" platform/` to inventory every file needing touched.
   - `grep -rn "/api/units" src/ platform/` to inventory route consumers.
   - Verify Hocuspocus collab key pattern in `server/hocuspocus.ts`.
2. **Migration**: write `drizzle/00XX_books_and_chapters.sql` per §1a. Update `src/lib/db/schema.ts` to match (`teachingUnits` export → `chapters` + new `books`). Run `bun run db:generate` to compare; reconcile.
3. **Rename Go files + structs**: `teaching_units.go` → `chapters.go`, all symbol renames per §1c + §1e.
4. **Routes flip**: `/api/units/*` → `/api/chapters/*` (no compat aliases).
5. **Books store + handler** (§1b + §1d).
6. **Cross-cutting renames** (§1f).
7. **Hocuspocus key check** (§1g).
8. **Run Go tests**: `cd platform && TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./... -count=1 -timeout 180s`. All green.
9. **Self-review on Opus**.
10. **Commit + push** as `plan 088 phase 1 (backend)`.

### Phase 2 — Frontend rename (Sonnet)

1. **Pre-impl grep**: every `Unit`/`unit` reference under `src/` that refers to the entity (skip "unit test" usages in test files).
2. **Drizzle schema rename** (§2a).
3. **Page directory renames** (§2b): use `git mv` so history is preserved. Update all internal links between pages (e.g., `/teacher/units/...` → `/teacher/chapters/...`).
4. **Component + type renames** (§2c).
5. **Fetch URL renames** (§2d).
6. **Add 308 redirects** in `next.config.ts` (§2e).
7. **Middleware updates** (§2f) if any unit-named matchers exist.
8. **Test renames** (§2g).
9. **Run** `bun run lint` (no new errors vs baseline), `bunx tsc --noEmit` (no new errors), `bun run test`.
10. **Self-review on Opus**.
11. **Commit + push** as `plan 088 phase 2 (frontend rename)`.

### Phase 3 — Books UI (Sonnet)

1. **Admin pages** (§3a, §3b).
2. **Teacher pages** (§3c).
3. **Components** (§3d).
4. **Chapter list filter** (§3e).
5. **Tests** (§3f).
6. **Run** `bun run test`, `bun run lint`, `bunx tsc --noEmit`. No regressions.
7. **Self-review on Opus**.
8. **Commit + push** as `plan 088 phase 3 (books UI)`.

### Phase 4 — Verify + docs

1. **Full test suite** — Vitest + Go.
2. **Smoke-test in dev**: open the new `/admin/books`; create a book; create a chapter assigned to that book; verify the chapter list shows the assignment; verify `/teacher/units/abc` redirects to `/teacher/chapters/abc`.
3. **Update `docs/api.md`** with the books section + chapter path renames.
4. **Self-review** the combined branch diff. Cross-phase consistency check.
5. **Commit + push** as `plan 088 phase 4 (verify + docs)`.
6. **Trigger 4-way code review** against the consolidated branch diff.

## Testing plan

| Layer | Test file | Cases |
|-------|-----------|-------|
| Go store | `platform/internal/store/chapters_test.go` (RENAMED from `teaching_units_test.go`) | Existing tests pass with renamed types; add 1 case asserting `BookID` field round-trips correctly. |
| Go store | `platform/internal/store/books_test.go` (NEW) | Create / Get / List (filtered by scope+scopeID) / Update / Delete; not-found nil; scope+scopeID validation; ON DELETE SET NULL cascades to chapter rows. |
| Go handler | `platform/internal/handlers/chapters_integration_test.go` (RENAMED) | All existing cases pass; routes now `/api/chapters/*`. |
| Go handler | `platform/internal/handlers/books_integration_test.go` (NEW) | 5 endpoints × happy + 400 + 401 + 403 + 404 + cross-scope isolation. |
| Go cross-cutting | `platform/internal/handlers/access_test.go` (existing) | `canViewChapter` / `canEditChapter` rename; behavior unchanged. |
| TS list page | `tests/unit/admin-chapters-page.test.tsx` (RENAMED from `admin-units-page` if existed) | Existing assertions hold post-rename. |
| TS books pages | `tests/unit/admin-books-page.test.tsx`, `admin-book-detail-page.test.tsx` (NEW) | Render metadata + chapter list + Edit button; 404 / 403 panels. |
| TS components | `tests/unit/book-edit-dialog.test.tsx`, `book-picker-dialog.test.tsx`, `book-actions.test.tsx` (NEW) | Standard form / dialog / actions patterns. |
| Redirect | `tests/integration/units-redirect.test.ts` (NEW, optional) | Hit `/teacher/units/abc/edit`, assert 308 + Location: `/teacher/chapters/abc/edit`. |

## Verification steps

After each phase: lint + type-check + relevant tests pass. No new failures.

Before opening the PR:
- Full Vitest + Go suites.
- Manual smoke in dev (per Phase 4 step 2).
- Visual check: `/admin/books` renders; `/teacher/chapters` renders; `/teacher/units/abc` 308s correctly.

Lint baseline: 100 errors / 45 warnings on `main` (post plan 087). Must not regress.
TSC baseline: 7 errors on `main` (`tests/unit/identity-assert.test.ts`). Must not regress.
Vitest baseline: 3 pre-existing failures in `tests/integration/auth-jwt-refresh.test.ts`. Must not regress.

## Plan Review

(Placeholder — to be filled by 4-way plan review before implementation.)

## Code Review

(Placeholder — to be filled after Phase 4.)

## Post-Execution Report

(Placeholder — to be filled before opening the PR.)
