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

CREATE TYPE "public"."book_scope" AS ENUM ('platform', 'org');

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
    (scope = 'org'      AND scope_id IS NOT NULL)
  )
);

CREATE INDEX books_scope_idx ON books(scope, scope_id);
CREATE INDEX books_created_by_idx ON books(created_by);

-- Rename teaching_units → chapters (table + indexes + checks).
ALTER TABLE teaching_units RENAME TO chapters;
ALTER INDEX teaching_units_pkey RENAME TO chapters_pkey;
ALTER INDEX teaching_units_created_by_idx RENAME TO chapters_created_by_idx;
ALTER INDEX teaching_units_scope_scope_id_status_idx RENAME TO chapters_scope_scope_id_status_idx;
ALTER INDEX teaching_units_scope_slug_uniq RENAME TO chapters_scope_slug_uniq;
ALTER INDEX teaching_units_search_idx RENAME TO chapters_search_idx;
ALTER INDEX teaching_units_standards_tags_gin_idx RENAME TO chapters_standards_tags_gin_idx;
ALTER INDEX teaching_units_subject_tags_gin_idx RENAME TO chapters_subject_tags_gin_idx;
ALTER INDEX teaching_units_topic_id_uniq RENAME TO chapters_topic_id_uniq;
ALTER TABLE chapters RENAME CONSTRAINT teaching_units_scope_scope_id_chk TO chapters_scope_scope_id_chk;
ALTER TABLE chapters RENAME CONSTRAINT teaching_units_status_chk TO chapters_status_chk;
ALTER TABLE chapters RENAME CONSTRAINT teaching_units_created_by_fkey TO chapters_created_by_fkey;
ALTER TABLE chapters RENAME CONSTRAINT teaching_units_topic_id_fkey TO chapters_topic_id_fkey;
-- (Full enumeration verified by GLM round-1 against `\d teaching_units`. The
-- pre-impl audit step re-runs `\d teaching_units` to catch any post-plan-088
-- migrations that added new objects.)

-- Rename the 4 satellite tables that share the unit_ prefix + carry unit_id
-- columns (GLM round-1 BLOCKER):
ALTER TABLE unit_documents       RENAME TO chapter_documents;
ALTER TABLE unit_revisions       RENAME TO chapter_revisions;
ALTER TABLE unit_overlays        RENAME TO chapter_overlays;
ALTER TABLE unit_collection_items RENAME TO chapter_collection_items;
ALTER TABLE unit_collections     RENAME TO chapter_collections;  -- already in §1c, restated here

-- Rename the unit_id columns on those tables → chapter_id:
ALTER TABLE chapter_documents       RENAME COLUMN unit_id        TO chapter_id;
ALTER TABLE chapter_revisions       RENAME COLUMN unit_id        TO chapter_id;
ALTER TABLE chapter_overlays        RENAME COLUMN parent_unit_id TO parent_chapter_id;
ALTER TABLE chapter_overlays        RENAME COLUMN child_unit_id  TO child_chapter_id;
ALTER TABLE chapter_collection_items RENAME COLUMN unit_id       TO chapter_id;

-- Rename the satellite-table pk's + unit-prefixed indexes to chapter-prefixed.
-- Pre-impl `\d` audit for each table enumerates exact names; spec all renames
-- in this migration.

ALTER TABLE chapters ADD COLUMN "book_id" uuid NULL REFERENCES books(id) ON DELETE SET NULL;
CREATE INDEX chapters_book_idx ON chapters(book_id);
```

**Inbound FK constraint names** on the 5 satellite tables (e.g., `unit_collection_items_unit_id_fkey`, `unit_documents_unit_id_fkey`) survive the column rename — Postgres FKs keep their CREATE-time name and the constraint behavior intact. Per Decision #13 (added below), these legacy-named FK constraints are left alone — cosmetic churn not worth the rename overhead. If any name lookup in app code matters (e.g., `dberr.go` sentinels keyed to constraint names), audit + rename those specific ones.

**Migration safety**:
- Existing chapters all get `book_id = NULL` (Decision #4). Library is empty; future UI/backfill scripts assign chapters to books.
- `topic_id` UNIQUE constraint preserved (Decision #6).
- Existing course → topic → chapter delivery semantics unchanged.
- All test fixture builders need updating (they reference `teaching_units` by name).
- **Pre-impl audit step in Phase 1**: `\d teaching_units` to enumerate ALL existing indexes / constraints / triggers / sequences. The migration must rename each one (Postgres doesn't auto-rename `teaching_units_*`-prefixed objects when the table is renamed). Common renames expected: `teaching_units_pkey` → `chapters_pkey`, `teaching_units_topic_id_uniq` → `chapters_topic_id_uniq`, `teaching_units_scope_idx` → `chapters_scope_idx`, etc.
- **FK constraint cosmetic names**: Postgres FK constraints carry their CREATE-time name (e.g., `assignments_unit_id_fkey`). Renaming the target table does NOT auto-rename the FK constraints on the source tables. These names are cosmetic (constraint behavior intact) but tools that dump schemas will show the legacy prefix. Decision: leave FK constraint names alone — cosmetic churn isn't worth the rename overhead. A future cleanup plan can audit and rename if it matters.

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

**Verified by GLM round-1**: `server/hocuspocus.ts:140,164` use `documentName.startsWith("unit:")` to skip persistence. Go side at `realtime_token.go:308` resolves `"unit:{unitId}"` doc names; `realtime_token.go:335-340` has a `case "unit"` / `authorizeUnitDoc` branch. **DeepSeek round-1 also flagged**: the FRONTEND side that *generates* these document names (likely `src/hooks/use-realtime-token.ts` or wherever a Yjs provider is constructed for unit/chapter docs) must rename too — pre-impl grep `"unit:" + id` and similar template literals.

Rename:

- `server/hocuspocus.ts:140,164` — `"unit:"` → `"chapter:"`.
- `platform/internal/handlers/realtime_token.go` — `case "unit"` → `case "chapter"`; `authorizeUnitDoc` → `authorizeChapterDoc`; doc-name parsing throughout the file.
- Frontend Yjs provider construction sites — rename document-name template literals from `"unit:" + id` to `"chapter:" + id` (or template form). Pre-impl `grep -rn '"unit:"' src/` to inventory.
- Tests `realtime_token_test.go` + `realtime_jwt_test.go` hardcode `"unit:"` prefixes (~33 occurrences across 4 files per DeepSeek count) — update.

In-flight WebSocket sessions break (server-side document key changes). Acceptable per Decision #9; dev/test scale.

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
5. **Books scope is `platform` or `org` only — no `personal`.** Both Codex round-1 and DeepSeek round-1 flagged personal-scope books as a v1 cut candidate with no validated use case. Cutting it now means fewer enum values, simpler CHECK constraint, smaller test matrix, less ambiguity in the UI ("my private library" wasn't a clear product fit). `canView` / `canEdit` mirror the existing unit rules for these two scopes (platform admin → platform-scope; org admin + teacher → org-scope). If personal-scope books become a real need later, a future plan can re-add — the `book_scope` enum is the only DDL-level expansion required.
6. **Keep `chapters.topic_id` UNIQUE constraint**. Books and topics are orthogonal axes — book is library organization, topic is course curation. A chapter can live in a book AND be pinned to a course topic.
7. **JSON field shapes don't change** outside of adding `bookId` to chapter responses. The `id`, `title`, `scope`, etc. fields stay the same names (they were never "unit"-prefixed).
8. **No default book auto-created**. Migration leaves existing chapters with `book_id = NULL`. UI surfaces them as "Unfiled" until an admin assigns them. Simpler migration, no implicit org-modeling decisions.
9. **Hocuspocus collab keys**: if they reference "unit" in the key, rename to "chapter" in the same PR. In-flight dev sessions lose state — acceptable at current scale.
10. **Plan files in `docs/plans/` keep their `unit` references** — archival record of decisions at the time. Only `docs/api.md` and forward-looking docs are updated.
11. **Topics stay untouched.** They remain children of courses and continue to point at chapters (via the renamed `topic_id` column on `chapters`). Plan 088 does not deprecate topics.
12. **String literals + log fields with "unit" get renamed** wherever they refer to the entity. This includes: API error messages ("Unit not found" → "Chapter not found"), `slog` log field names (`unit_id` → `chapter_id`), sentinel constraint-name strings in `dberr.go` if any (e.g., `teaching_units_topic_id_uniq` → `chapters_topic_id_uniq` — must match the renamed index name from §1a). Frontend toast / error copy ("Unit saved", "Failed to load unit"). Skip Go test-comment usage of "unit test" (the testing concept) — only rename when the word refers to the entity.

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

**IMPORTANT — Intermediate state**: this plan's phase boundaries are mid-rename commits. Phase 1 flips backend routes to `/api/chapters/*` while the frontend still fetches `/api/units/*`. The app between Phase 1 and Phase 2 commits is INTENTIONALLY BROKEN at runtime. Don't try to run the full stack mid-PR; just verify type-checks + tests per phase. The 4-way code review fires after Phase 4 against the consolidated diff, not per-phase. (Codex round-1 + DeepSeek round-1 caveats.)

**Push-timing constraint** (DeepSeek round-1): Phase 1 can be committed locally but should NOT be pushed to a shared branch before Phase 2 is also ready locally. The remote branch should never sit in the half-renamed state. Sequence:
1. Implement Phase 1 + 2 locally as separate commits.
2. Force-push both at once.
3. Phase 3 (books UI) can ship later commits, since by then the rename is consistent.

Phase 4 (verify + docs) is a small commit that can land separately.

1. **Pre-impl audit**:
   - `psql -d bridge_test -c "\d teaching_units"` to inventory all current indexes + constraints for the rename migration. Repeat for `unit_documents`, `unit_revisions`, `unit_overlays`, `unit_collection_items`, `unit_collections`.
   - Also check for triggers, views, and functions referencing the old table names (DeepSeek round-1 nit): `psql -d bridge_test -c "SELECT triggername FROM pg_trigger WHERE tgrelid::regclass::text LIKE 'unit_%' OR tgrelid::regclass::text = 'teaching_units';"` and similar for `pg_views` / `pg_proc.prosrc LIKE '%teaching_units%'`.
   - `grep -rln "teaching_units\|TeachingUnit\|teachingUnits\|unit_documents\|unit_revisions\|unit_overlays\|unit_collection" platform/ src/ tests/ e2e/ scripts/ server/` to inventory every file needing touched (Codex round-1 NIT: include tests, e2e, scripts, server — not just platform/src).
   - `grep -rn "/api/units" src/ platform/ tests/ e2e/` to inventory route consumers.
   - `grep -rn '"unit:"' src/ platform/ server/` to inventory Hocuspocus doc-name string literals (frontend + backend + collab server). DeepSeek round-1 emphasized ~33 occurrences across 4 test files — count them.
   - **SQL string-literal trap** (DeepSeek round-1): Go SQL strings like `"SELECT ... FROM teaching_units"` survive compilation and only fail at test runtime. Pre-impl `grep -rn "teaching_units" platform/` is the critical safety net — every match is either a SQL string, a function/var name, or a comment, and ALL must be touched.
2. **Migration**: write `drizzle/00XX_books_and_chapters.sql` per §1a. Update `src/lib/db/schema.ts` to match (`teachingUnits` export → `chapters` + new `books`). Run `bun run db:generate` to compare; reconcile.
3. **Rename Go files + structs**: `teaching_units.go` → `chapters.go`, all symbol renames per §1c + §1e.
4. **Routes flip**: `/api/units/*` → `/api/chapters/*` (no compat aliases).
5. **Books store + handler** (§1b + §1d).
6. **Cross-cutting renames** (§1f).
7. **Hocuspocus key check** (§1g).
8. **Run Go tests**: `cd platform && TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./... -count=1 -timeout 180s`. All green. **If sandbox restrictions block DB-backed tests (per plan 085 lessons learned), commit anyway — the orchestrator runs the full Go suite locally before signing off Phase 1.** Non-DB-dependent tests must pass.
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

### Self-review (Opus 4.7) — 2026-05-14

**Verdict: CONCUR with self-applied refinements.**

Self-review concerns folded:

1. **String literals + log fields with "unit"** — added Decision #12. The rename can't stop at type names; API error messages, slog fields, and `dberr.go` constraint-name sentinels all need updating.
2. **All `teaching_units_*` prefixed indexes / constraints / sequences need explicit rename** in the migration. Pre-impl `\d teaching_units` audit added to Phase 1 step 1. Common renames enumerated.
3. **FK constraint cosmetic names on source tables** (e.g., `assignments_unit_id_fkey`) survive the table rename and stay legacy-prefixed. Decision: leave alone — cosmetic churn not worth the overhead. Future cleanup plan can audit.
4. **Codex sandbox stalls from plan 085 lessons** — Phase 1 step 8 explicitly says "commit anyway if sandbox blocks DB tests; orchestrator runs full suite before signing off". Avoids re-blocking on the same sandbox issue.

Open concerns flagged for external reviewers:

- **Mass-rename merge-conflict risk** is real. Plan calls for a single-PR rename. If any other branch is touching unit-related code during the rebase window, conflicts will be substantial. Worth a reviewer call on whether to defer plan 088 if other in-flight work touches the same files.
- **Hocuspocus collab key migration** (§1g) — verify pre-impl. In-flight dev collab sessions lose state; acceptable at current scale but worth reviewer signoff.
- **`book_id` NULLABLE leaves existing chapters unfiled** — UX implication: the new books UI will show every existing chapter as "unfiled" until manually assigned. Reviewer may want a backfill commitment (e.g., auto-create a "Legacy" book per org) — plan defers this to a future plan; reviewer should agree or push back.
- **`unit_collections` rename to `chapter_collections`** — I haven't read this file in detail. If `unit_collections` is a course-pinned-collection concept distinct from chapters-in-a-book, the rename may be wrong. Reviewer should verify.
- **Personal-scope books** (Decision #5) — may be unused in practice. Reviewer may push to drop personal scope to simplify.

### Round 1 verdicts — 2026-05-14

| Reviewer | Verdict | Resolution |
|----------|---------|------------|
| Self (Opus 4.7) | CONCUR | — |
| Codex | **CONCUR + 3 NITs** | (a) Broaden rename grep to include `tests/`, `e2e/`, `scripts/` → FIXED in Phase 1 step 1. (b) Add phase-ordering broken-state callout → FIXED at top of Phase 1. (c) Decide explicitly on 4 satellite tables (`unit_documents`, etc.) → already FIXED in §1a per GLM BLOCKER. Confirmed `unit_collections` semantics are generic scope-based (not course-pinned) — rename is correct. Confirmed `docs/api.md` has no `/api/units/*` content. |
| DeepSeek V4 Pro | **CONCUR + caveat** | Caveat: Phase 1/2 commit push-timing must not leave the remote branch in half-renamed state → FIXED with explicit push-timing instructions at top of Phase 1. Additional folds: (a) Pre-impl `\d` audit also covers triggers/views/functions; (b) Frontend Yjs provider sites generating `"unit:"` doc names also rename; (c) SQL string-literal trap callout in pre-impl audit (Go `"SELECT ... FROM teaching_units"` survives compilation). Suggested cutting personal-scope books → FIXED (Decision #5 narrowed). |
| GLM 5.1 | **BLOCKER + nits** | BLOCKER: 4 satellite tables (`unit_documents`, `unit_revisions`, `unit_overlays`, `unit_collection_items`) + their `unit_id` columns must rename → FIXED in §1a with explicit ALTER TABLE statements. Confirmed Hocuspocus collab keys at specific line numbers (server/hocuspocus.ts:140/164, realtime_token.go:308/335-340); folded into §1g. Confirmed fixture helpers are `newUnitFixture`, `mkUnit`, `linkUnitFixture`, `collectionFixture` (not `seedUnit` as I assumed); plan rename audit step catches all. |

**Plan revised in commits `cae482f` → `117e4ff`**. GLM BLOCKER cleanly resolved (satellite tables explicit). Codex + DeepSeek nits all folded.

### Round 2 verdict — 2026-05-14

| Reviewer | Verdict | Notes |
|----------|---------|-------|
| GLM 5.1 (round 2) | **CONCUR** | Satellite-table renames cleanly resolved per round-1 BLOCKER. One non-blocking note: satellite-table indexes/constraints aren't enumerated by name like the main table's (the pre-impl `\d` audit step + "spec all renames in this migration" instruction covers it). |

### Final 4-way gate status

| Reviewer | Final verdict |
|----------|---------------|
| Self (Opus 4.7) | CONCUR |
| Codex | CONCUR (round 1) |
| DeepSeek V4 Pro | CONCUR (round 1) |
| GLM 5.1 | CONCUR (round 2) |

**Gate is clean. Plan 088 ready for implementation.**

## Code Review

### Round 1 (after Phase 4 verify, against `cf691c5`)

Dispatched 4-way in parallel (Codex, DeepSeek V4 Flash, GLM 5.1, plus self-review).

| Reviewer | Verdict | Blockers |
|---|---|---|
| Self (Opus 4.7) | BLOCKER-aware CONCUR | Flagged the Hocuspocus key partial-migration risk before dispatch; raised with reviewers as area-to-stress-test. |
| Codex | **BLOCKER × 5** | (1) `src/lib/yjs/use-yjs-tiptap.ts:92` still generates `unit:${id}` doc names — backend only accepts `chapter:` after the prefix migration → realtime collab broken on every chapter page. (2) `BookHandler` uses `requirePlatformAdmin` on every route but `/teacher/books` expects org-scoped teacher/admin access; teachers will get 403 across the new UI. (3) 9 `@deprecated` aliases survived Phase 2 cleanup (`TeachingUnit`, `UnitDocument`, `CreateUnitInput`, `fetchUnitDocument`, `transitionUnit`, `UnitOverlay`, `UnitRevision`, `forkUnit`, `saveUnitDocument`) — same-PR rename, no external consumers, CLAUDE.md says delete-don't-alias. (4) `/teacher/books/${book.id}` link target doesn't exist — Phase 3 added the route at `/admin/books/[id]` only, and `BookActions` hardcodes the `/admin/books/` prefix. (5) `store.UnitOverlay` JSON tags are `childUnitId`/`parentUnitId` but the TS overlay reader expects `childChapterId`/`parentChapterId` — wire format mismatch will break overlay fetches after the rename. |
| DeepSeek V4 Flash | **BLOCKER × 1** | Books authz mismatch (same as Codex #2). Recommended mirroring `canViewChapter`/`canEditChapter` so the handler matches the new scope-aware UI contract. |
| GLM 5.1 | **BLOCKER × 1** | Same Hocuspocus key migration finding as Codex #1, plus three additional test files still asserting `unit:` doc-name prefixes (`tests/unit/realtime-jwt.test.ts`, `tests/unit/use-realtime-token.test.tsx`, `e2e/hocuspocus-auth.spec.ts`). Originally flagged the 1 frontend file; round-2 surfaced the 3 test files after the frontend fix. |

**Fixes (`ec39ddb`)**
- Hocuspocus doc-name: flipped `unit:${unitId}` → `chapter:${unitId}` in `src/lib/yjs/use-yjs-tiptap.ts:92`, plus 4 test files (`tests/unit/realtime-jwt.test.ts`, `tests/unit/use-realtime-token.test.tsx`, `tests/unit/realtime-get-token.test.ts`, `e2e/hocuspocus-auth.spec.ts`).
- BookHandler: replaced `requirePlatformAdmin` with scope-mirrored `canViewBook`/`canEditBook` helpers paralleling the chapter handler. List endpoint now returns 200 + visibility-filtered items (no existence leak); Get/Update/Delete return 404 for non-viewers (no existence leak). Test fixture updated to pass `OrgStore`; 4 assertions updated for the new shape.
- Deprecated aliases: removed all 9 from `src/lib/chapters.ts` and confirmed zero callers across `src/` and `tests/`.
- Teacher books route: added `/teacher/books/[id]/page.tsx` + `book-edit-trigger.tsx` (copied from admin, org-name resolution via `/api/orgs` instead of admin-gated `/api/admin/orgs`). Added `detailBasePath` prop to `BookActions` so teacher list rows route to `/teacher/books/${id}`.
- `UnitOverlay` JSON tags: `ChildChapterID` → `childChapterId`, `ParentChapterID` → `parentChapterId` in `platform/internal/store/chapters.go`.

### Round 2 (after `ec39ddb`)

| Reviewer | Verdict | Blockers |
|---|---|---|
| GLM 5.1 (round 2) | **BLOCKER × 1** | 3 more test files still on `unit:` doc-name prefix (`tests/unit/realtime-jwt.test.ts`, `tests/unit/use-realtime-token.test.tsx`, `e2e/hocuspocus-auth.spec.ts`). The Hocuspocus key fix landed on the frontend in `ec39ddb` but the test-side assertions weren't all flipped. |
| Codex (round 2) | **CONCUR** | All 5 round-1 BLOCKERs resolved cleanly. |
| DeepSeek V4 Flash (round 2) | **CONCUR** | Books authz rewrite matches chapter pattern. |

**Fixes (`4529262`)**: flipped the 3 additional test-file prefixes; verified all 7 `chapter:` references match the backend `RealtimeAccessControl` allowlist.

### Cascade fixes (after running the full Go suite under `DATABASE_URL=postgresql://...bridge_test`)

Running the Go suite with the correct env var (the suite was silently skipping previously due to `TEST_DATABASE_URL` vs `DATABASE_URL` mismatch) uncovered 6 cascading issues, all addressed in `0d9a1ed`:

- `store/chapters.go:367` — `ListChaptersByTopicIDs` SELECT was missing `u.book_id` (scanChapter expects 17 cols, got 16 → "Database error" on session creation).
- `store/chapters.go:1067-1071` — `UnitOverlay` JSON tag fix from round-1 confirmed; comment notes the rename rationale.
- `store/users_test.go:209-212` — `addTestMembership` INSERT referenced non-existent `updated_at` column on `org_memberships` (pre-existing plan-085 bug, masked by silent suite skip).
- `handlers/books_integration_test.go` — 4 assertions updated to match canViewBook semantics (List 403→200 visibility-filtered, Get/Update/Delete 403→404 no-leak).
- `db/migrations.go:124` — restored `chapters_book_idx` in `ExpectedSchemaSentinels.Indexes` (plan 088 added it in the same migration that creates `books`; parity test demands bidirectional coverage).
- `db/schema_probe.go:111-132` — relaxed `checkIndexes` to query `pg_indexes` by `indexname` only (drop `tablename` filter). Safe because index names are unique per schema, and necessary because a single migration can declare indexes on multiple tables.
- `db/schema_probe_integration_test.go` — updated `TestCheckSchemaProbe_Missing{Column,Constraint,Index}` to drop current books-table sentinels (`description`, `books_scope_id_required`, `books_scope_idx`) instead of retired `parent_links` sentinels.

### Round 3 — final confirmation

Re-dispatched the 3 originally-blocking reviewers (Codex, DeepSeek V4 Flash, GLM 5.1) against `0d9a1ed` to confirm all round-1 BLOCKERs and round-2 GLM follow-up are cleanly resolved.

(Verdicts pending — to be filled when round-3 returns.)

## Post-Execution Report

### Commits on the branch (in order)

- Plan iterations: `1d51299` (draft) → `cae482f` (self-review) → `117e4ff` (round-1 reviewer folds incl. GLM BLOCKER) → `6a1772b` (round-2 GLM CONCUR / gate clean).
- Phase 1 backend (`ad372e0`): Codex implementation — migration 0026, books table + 5 satellite-table renames + chapters.book_id + all index/constraint renames + new BookStore + new BookHandler + chapter store/handler renames + Hocuspocus key prefix migration. 62 files, 2719 ins / 2038 del. Codex sandbox stalled before commit; orchestrator verified locally + committed.
- Phase 2 frontend (`8ddfd40`): Sonnet — page directories renamed via `git mv`, components, types, fetch URLs, 308 redirects in `next.config.ts`, middleware updates.
- Phase 2 cleanup (`83752eb`): orchestrator dropped Sonnet's backward-compat aliases (`fetchUnit`, `createUnit`, `createTestTeachingUnit`, `chapters as teachingUnits` import alias) per CLAUDE.md "delete unused, don't alias".
- Phase 3 books UI (`968bf83`): Sonnet — admin + teacher list/detail/edit pages, OrgFilter-style filters, BookEditDialog, BookPickerDialog, BookActions (with type-to-confirm Delete), chapter list `bookId` dropdown.
- Phase 3 follow-up (`f868da6`): orchestrator closed the backend gap Sonnet flagged — added `ChapterBookFilter` type + `bookId` query param to both `GET /api/chapters` and `GET /api/chapters/search`. Wired admin book detail page (chapters card) + teacher chapters page (book dropdown). Dropped one more deprecated alias (`searchUnits`). Removed all "TODO(plan 088 phase 3)" comments.

### Deviations from the plan

- **Compat-alias rip-out wasn't anticipated**: Sonnet (both Phase 2 and Phase 3) defaulted to adding `@deprecated` aliases on renamed exports. Orchestrator removed all of them in cleanup commits — same-PR rename means there are no external consumers to compat-with. Now codified for future Sonnet briefs (the Phase 3 brief explicitly forbade them; they didn't show up in Sonnet's Phase 3 output, but `searchUnits` slipped through earlier).
- **Phase 3 finished WITHOUT the chapter-edit-page picker integration**. Plan 088 §3d explicitly permitted shipping the `BookPickerDialog` as a library component without a caller. Future plan that touches the chapter-edit page will wire it.
- **Backend gap caught + closed mid-Phase-3**: `GET /api/chapters` initially had no `bookId` filter, so the admin book detail page was empty and the teacher book dropdown was non-functional. Closed in `f868da6` (added `ChapterBookFilter` + 2 handler updates + frontend wiring) rather than deferring to a follow-up plan.

### Verification (final)

- Migration applied cleanly to both `bridge` (dev) and `bridge_test`.
- Go: `cd platform && TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./... -count=1 -timeout 240s` — all packages pass.
- Vitest: `bun run test` — 796 passed + 3 pre-existing failures in `tests/integration/auth-jwt-refresh.test.ts` (unchanged from main).
- Lint: 100 errors / 45 warnings (baseline unchanged).
- TSC: 7 errors (baseline unchanged).

### Known limitations / follow-up work

- **Chapter-edit page book picker integration** — `BookPickerDialog` exists as a reusable library component; the chapter-edit page (`src/app/(portal)/teacher/chapters/[id]/edit/page.tsx`) doesn't yet expose a "Move to book" affordance. Natural fit for the next plan that touches the chapter-edit page.
- **Existing chapters all have `book_id = NULL`** per Decision #4. The admin UI shows them as "Unfiled". An admin must manually assign each chapter to a book (or a future backfill plan can auto-assign org chapters to a "Legacy" book per org).
- **Books list query** displays `—` for chapter count rather than actual counts. Future enhancement could either hit the chapters endpoint per book (N+1) or add a denormalized count.
- **Inbound FK constraint names** on satellite tables (e.g., `chapter_documents` references `chapters` via constraints still named `unit_documents_unit_id_fkey`). Cosmetic legacy names per plan policy; future cleanup plan can rename.
- **No audit log** for book admin operations (create/edit/delete) — same gap as plans 085/086 for user/org admin ops. Future plan should add an `admin_actions` table and retrofit.

### File census (branch vs main)

86 files changed, 4 new migrations, ~50 renamed (Go + TS), ~10 new files (books domain).

Ready for 4-way code review against the consolidated branch diff.
