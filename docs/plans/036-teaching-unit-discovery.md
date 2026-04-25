# 036 — Teaching Unit: Discovery + Libraries

**Goal:** Build teacher-facing library surfaces with search (Postgres FTS + structured filters), unit collections for curated sequences, and quality-signal schema for future ranking.

**Spec:** `docs/specs/012-teaching-units.md` — §Discovery layers, §Quality signals, §unit_collections

**Branch:** `feat/036-discovery`

**Depends On:** Plan 032 (migrated content), Plan 033a (lifecycle statuses)

**Status:** In progress

---

## Scope

**In scope:**
- Migration 0019: `unit_collections` + `unit_collection_items` tables, FTS index on teaching_units (title + summary), quality signal columns on teaching_units
- Go store: `SearchUnits` with FTS + structured filters + cursor pagination, collection CRUD
- Go handler: `GET /api/units/search`, collection endpoints
- Teacher library page: `/teacher/units` with tabs (My Units, Org Library, Platform Library), search bar, filters
- Unit collections: create/edit/view curated sequences

**Out of scope (infrastructure needed):**
- pgvector semantic search (requires `CREATE EXTENSION vector` + embedding generation pipeline)
- Quality signal capture pipeline (class_usages, completion rates, ratings) — schema only, no capture
- Community sharing surface (stub mention only)
- Composite ranking function (deferred to when signals + embeddings exist)

---

## Task 1: Migration — collections + FTS + quality signals

**Files:**
- Create: `drizzle/0019_discovery.sql`
- Modify: `src/lib/db/schema.ts`

```sql
BEGIN;

-- FTS index on teaching_units
ALTER TABLE teaching_units
  ADD COLUMN IF NOT EXISTS search_vector tsvector
    GENERATED ALWAYS AS (
      to_tsvector('english', coalesce(title, '') || ' ' || coalesce(summary, ''))
    ) STORED;

CREATE INDEX IF NOT EXISTS teaching_units_search_idx
  ON teaching_units USING GIN (search_vector);

-- Quality signal columns (schema only — no capture pipeline yet)
ALTER TABLE teaching_units
  ADD COLUMN IF NOT EXISTS usage_count int NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS avg_rating numeric(3,2);

-- Unit collections
CREATE TABLE IF NOT EXISTS unit_collections (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  scope       varchar(16) NOT NULL,
  scope_id    uuid,
  title       varchar(255) NOT NULL,
  description text NOT NULL DEFAULT '',
  created_by  uuid NOT NULL REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT unit_collections_scope_chk CHECK (
    (scope = 'platform' AND scope_id IS NULL) OR
    (scope IN ('org', 'personal') AND scope_id IS NOT NULL)
  )
);

CREATE INDEX IF NOT EXISTS unit_collections_scope_idx
  ON unit_collections(scope, scope_id);

CREATE TABLE IF NOT EXISTS unit_collection_items (
  collection_id uuid NOT NULL REFERENCES unit_collections(id) ON DELETE CASCADE,
  unit_id       uuid NOT NULL REFERENCES teaching_units(id) ON DELETE CASCADE,
  sort_order    int NOT NULL DEFAULT 0,
  PRIMARY KEY (collection_id, unit_id)
);

COMMIT;
```

**Commit:** `feat(036): migration 0019 — FTS index, collections, quality signal columns`

---

## Task 2: Go store — search + collections

**Files:**
- Modify: `platform/internal/store/teaching_units.go` — add `SearchUnits`
- Create: `platform/internal/store/unit_collections.go`
- Create: `platform/internal/store/unit_collections_test.go`
- Modify: `platform/internal/store/teaching_units_test.go`

**SearchUnits:**
```go
type SearchUnitsFilter struct {
    Query       string   // FTS query
    Scope       string   // platform | org | personal
    ScopeID     *string
    Status      string
    GradeLevel  string
    SubjectTags []string // AND semantics
    ViewerID    string
    ViewerOrgs  []string
    IsPlatformAdmin bool
    Limit       int
    CursorCreatedAt *time.Time
    CursorID    *string
}

func (s *TeachingUnitStore) SearchUnits(ctx, filter) ([]TeachingUnit, error)
```

When Query is non-empty, filter by `search_vector @@ plainto_tsquery('english', $query)` and order by `ts_rank(search_vector, query) DESC`. Otherwise order by `updated_at DESC`.

Visibility follows the same access rules as `canViewUnit` — filter in SQL:
- Platform published/ready → any auth
- Org → teachers/admins in that org (deny students per plan-031 narrowing)
- Personal → owner only
- Platform admin sees all

**UnitCollectionStore:** basic CRUD + add/remove/reorder items.

**Commit:** `feat(036): SearchUnits with FTS + UnitCollectionStore`

---

## Task 3: Go handler — search + collection endpoints

**Files:**
- Modify: `platform/internal/handlers/teaching_units.go` — add search route
- Create: `platform/internal/handlers/unit_collections.go`
- Create: `platform/internal/handlers/unit_collections_integration_test.go`
- Modify: `platform/cmd/api/main.go`
- Modify: `next.config.ts` — add `/api/collections/:path*`

```
GET  /api/units/search?q=loops&scope=org&gradeLevel=6-8&tags=loops,arrays
GET  /api/collections
POST /api/collections
GET  /api/collections/{id}
PATCH /api/collections/{id}
DELETE /api/collections/{id}
POST /api/collections/{id}/items    body: { unitId, sortOrder? }
DELETE /api/collections/{id}/items/{unitId}
```

**Commit:** `feat(036): search + collection endpoints`

---

## Task 4: Teacher library page

**Files:**
- Create: `src/app/(portal)/teacher/units/page.tsx` — library page with tabs
- Modify: `src/app/(portal)/teacher/page.tsx` — add "Unit Library" link
- Create: `src/lib/unit-search.ts` — search helper

Library page layout:
- Tabs: "My Units" (personal scope), "Org Library" (org scope), "Platform Library" (platform scope)
- Search bar (FTS query)
- Filter chips: grade level, subject tags
- Results: card grid showing title, summary, status badge, grade, tags, usage count
- Click → unit view page (`/teacher/units/{id}`)
- "Create Unit" button → `/teacher/units/new`

Keep it simple — a functional search + browse. No infinite scroll for MVP; just `limit=20` + "Load more" button.

**Commit:** `feat(036): teacher unit library page with search + filters`

---

## Task 5: Verify + docs + code review

**Verification:**
```bash
cd platform && go test ./... -count=1 -timeout 180s
node_modules/.bin/vitest run
node_modules/.bin/tsc --noEmit
```

Update `docs/api.md`. Write post-execution report. Run Codex code review.

**Commit:** `docs(036): API reference + post-execution report`

---

## Code Review

### Review 1

- **Date**: 2026-04-25
- **Reviewer**: Claude (Codex unavailable — model access issue)
- **Verdict**: Approved

No bugs found. Specific checks:
- FTS uses `plainto_tsquery('english', $N)` with parameterized query — no SQL injection risk
- Visibility gate in `SearchUnits` matches `canViewUnit`: platform published→all, org→teacher/admin members, personal→owner, author bypass, admin sees all
- Collection auth follows scope-based pattern (CHECK constraint enforced at DB level)
- `isConstraintError` fix handles both `lib/pq` and `pgx/pgconn` error types — correctly prevents duplicate collection items from returning 500
- FTS generated column (`GENERATED ALWAYS AS ... STORED`) auto-maintained by PostgreSQL — no manual sync needed
- Frontend debounce implementation uses `setTimeout` with cleanup — correct pattern

## Post-Execution Report

**Status:** Complete

**Implemented**

- Migration 0019: FTS tsvector generated column + GIN index on teaching_units, quality signal columns (usage_count, avg_rating — schema only, no capture), unit_collections + unit_collection_items tables.
- SearchUnits: FTS with `plainto_tsquery` + `ts_rank` ranking when query provided, `updated_at DESC` browse mode otherwise. Full visibility gate in SQL. Structured filters (scope, status, gradeLevel, subjectTags with `@>` containment). Cursor pagination. 7 store tests.
- UnitCollectionStore: full CRUD + item add/remove/reorder. 21 store tests.
- Search handler: `GET /api/units/search` with query params. 22 integration tests.
- Collection handler: 8 endpoints for collection CRUD + items. Integration tests cover auth, cross-org isolation, admin bypass.
- Teacher library page: tabs (My Units / Org / Platform), debounced FTS search, grade level + tags filters, card grid with status badges, "Load more" pagination. Link from teacher dashboard.

**Verification**

- Go: 14 packages green (handlers 80s, store 27s)
- tsc: no new errors
- Migration idempotent on both DBs

**pgvector follow-up**

Semantic search requires `CREATE EXTENSION vector` + an embedding generation pipeline. Schema is ready for it — add a `search_embedding vector(1536)` column and a cosine similarity index when the extension is available. The composite ranking function (blending FTS score + semantic score + quality signals) is deferred until both signals exist.
