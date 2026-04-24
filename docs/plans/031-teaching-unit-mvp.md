# 031 — Teaching Unit MVP: Data Model + Minimal Editor

**Goal:** Ship the core teaching-unit schema (3 tables), a Go CRUD surface, and a Tiptap editor that supports only the two block types needed to prove the model end-to-end: `prose` and `problem-ref`. No overlay, no AI, no realtime, no lifecycle transitions, no student-facing pages.

**Architecture:** Three new tables (`teaching_units`, `unit_documents`, `unit_revisions`); `unit_revisions` stays empty this plan — its creation is gated by publish transitions that land in plan 033. Go `TeachingUnitStore` + `TeachingUnitHandler` follow the scope + access pattern from plan 028's problem bank. Frontend is a teacher-only editor page (`/teacher/units/new` and `/teacher/units/{id}/edit`) plus a read-only view page. Blocks use Tiptap v2 with a custom `problem-ref` node that renders an existing problem's card inline.

**Tech Stack:** PostgreSQL 15 (SQL migration), Drizzle ORM (schema.ts), Go (Chi, `database/sql` via pgx, testify), Next.js 16, React 19, Tiptap v2 (`@tiptap/react`, `@tiptap/starter-kit`), Vitest.

**Spec:** `docs/specs/012-teaching-units.md`

**Branch:** `feat/031-teaching-unit-mvp`

**Depends On:** Plan 028 (problem bank) — provides the `problems` table and `GET /api/problems/{id}` that `problem-ref` blocks reference.

**Unblocks:** Plans 032 (migration from topics), 033 (full block palette + lifecycle + projection), 034 (overlay reuse).

---

## Scope boundaries

**In scope:**
- `teaching_units`, `unit_documents`, `unit_revisions` tables + indexes
- Scope-aware CRUD (`platform`/`org`/`personal`) with the §Access policy from spec 012
- Tiptap editor rendering `prose` (rich text) and `problem-ref` (embedded problem card)
- Teacher-only pages: new, edit, read-only view
- Drizzle schema types; Go store + handler; integration + unit tests
- `docs/api.md` entries for the new endpoints

**Out of scope (deferred to later plans):**
- `unit_overlays` table and overlay rendering (plan 034)
- `unit_collections` (plan 036)
- Additional block types: `teacher-note`, `live-cue`, `solution-ref`, `test-case-ref`, `assignment-variant`, `media-embed`, `code-snippet` (plan 033)
- Status transitions with revision-snapshot creation (plan 033) — units stay in their assigned `status` string as a stored field; no server endpoints to transition state
- Render projection pipeline (plan 033) — editor shows what's stored; no per-role transforms
- Markdown I/O, AI drafting, Yjs realtime (plan 035)
- Student-facing unit pages; session/assignment binding to units (plan 032 onward)
- Migration from existing `topic.lessonContent` (plan 032)

Keep plan 031 small. Anything not listed in "In scope" is a new plan.

---

## File Structure

| File | Responsibility |
|---|---|
| `drizzle/0016_teaching_units.sql` | Create `teaching_units`, `unit_documents`, `unit_revisions`; add CHECK constraints; indexes. |
| `src/lib/db/schema.ts` | Drizzle table exports: `teachingUnits`, `unitDocuments`, `unitRevisions`. |
| `tests/unit/schema.test.ts` | Assert new table shape. |
| `platform/internal/store/teaching_units.go` | **New.** `TeachingUnitStore` with CRUD on `teaching_units`, upsert on `unit_documents`, no writes to `unit_revisions` yet. |
| `platform/internal/store/teaching_units_test.go` | **New.** Store integration tests. |
| `platform/internal/handlers/teaching_units.go` | **New.** `TeachingUnitHandler` with scope-aware CRUD + access policy. |
| `platform/internal/handlers/teaching_units_integration_test.go` | **New.** Handler integration tests — happy path, access matrix, cross-org isolation. |
| `platform/internal/handlers/stores.go` | Add `TeachingUnits *store.TeachingUnitStore` field. |
| `platform/cmd/api/main.go` | Wire `TeachingUnitHandler` into the authenticated route group. |
| `next.config.ts` | Add `/api/units/:path*` to `GO_PROXY_ROUTES`. |
| `package.json` | Add `@tiptap/react`, `@tiptap/starter-kit`, `@tiptap/pm`, `@tiptap/extension-code-block-lowlight`, `lowlight`. |
| `src/components/editor/tiptap/extensions.ts` | **New.** Tiptap extension list used by the teaching-unit editor. Registers `StarterKit` + the custom `problem-ref` node. |
| `src/components/editor/tiptap/problem-ref-node.tsx` | **New.** Custom Tiptap `Node` for `problem-ref`. Renders an embedded problem card fetched via the existing Go API. |
| `src/components/editor/tiptap/teaching-unit-editor.tsx` | **New.** The `<TeachingUnitEditor />` React component — wraps Tiptap, manages document state, exposes `onSave`. |
| `src/components/editor/tiptap/teaching-unit-viewer.tsx` | **New.** Read-only rendering of a saved block document. Reuses the same extensions. |
| `src/app/(portal)/teacher/units/new/page.tsx` | **New.** Teacher-facing "create a unit" page. |
| `src/app/(portal)/teacher/units/[id]/edit/page.tsx` | **New.** Edit an existing unit. |
| `src/app/(portal)/teacher/units/[id]/page.tsx` | **New.** Read-only view. |
| `src/lib/teaching-units.ts` | **New.** Client-side helpers: `fetchUnit`, `saveUnit`, `createUnit`. |
| `tests/unit/teaching-unit-editor.test.tsx` | **New.** Vitest coverage for the editor component: save roundtrip, `problem-ref` insertion. |
| `docs/api.md` | New "Teaching Units" section. |
| `TODO.md` | Add follow-up items that surface during implementation. |

---

## Access policy reference

For every handler task below, rules come from **spec 012 §Access policy** + **§Access table**. Summary (plan 031 has no lifecycle transitions, so status is just a stored field that participates in the access table unchanged):

- **View:** scope-based per spec 012 §Access table. Draft in platform scope → platform admin only. Draft in org scope → teachers and org_admins of that org. Personal scope → owner only.
- **Edit / create / delete:** platform → platform admins; org → `org_admin` or `teacher` in that org (`org_memberships.role`); personal → `created_by = userId` only.
- **Fork, overlay, lifecycle transitions, archived-unit rules:** deferred to later plans.

Reuse the access helper pattern from `platform/internal/handlers/problem_access.go` (introduced in plan 028). Do not duplicate role-lookup logic — extract a shared helper if needed.

### Plan-031-specific access narrowing: org students

Spec 012 §Access says students see org-scope `classroom_ready` / `coach_ready` / `archived` units "via class ref" — i.e., because a session or assignment in their class binds the unit. **Plan 031 has not built the class-binding mechanism yet** (that lands in plan 032 onward), so there is no way for a student to legitimately reach a unit.

To avoid shipping a too-permissive MVP, plan 031 **denies all student access to `teaching_units`**. `canViewUnit` returns `false` for any org-scope viewer whose role is `student` regardless of status. Teachers, org_admins, and platform admins are unaffected. Plan 032 widens this back to the spec-intended shape once class-binding exists (student sees the unit iff their class references it).

Document this narrowing in `docs/api.md` for plan 031 so front-end and backend callers understand the temporary constraint.

---

### Task 1: Schema migration + Drizzle types

**Files:**
- Create: `drizzle/0016_teaching_units.sql`
- Modify: `src/lib/db/schema.ts`
- Modify: `tests/unit/schema.test.ts`

- [ ] **Step 1: Write the migration (`drizzle/0016_teaching_units.sql`)**

```sql
-- Plan 031 / spec 012: teaching units core schema.
-- Introduces teaching_units, unit_documents, unit_revisions.
-- No lifecycle transitions in this plan — status is a stored field
-- and no code writes to unit_revisions yet.
-- Idempotent: IF EXISTS / IF NOT EXISTS guards throughout so dev
-- DBs mid-state apply cleanly.

BEGIN;

CREATE TABLE IF NOT EXISTS teaching_units (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  scope               varchar(16) NOT NULL,
  scope_id            uuid,
  title               varchar(255) NOT NULL,
  slug                varchar(255),
  summary             text NOT NULL DEFAULT '',
  grade_level         varchar(8),
  subject_tags        text[] NOT NULL DEFAULT '{}',
  standards_tags      text[] NOT NULL DEFAULT '{}',
  estimated_minutes   int,
  status              varchar(24) NOT NULL DEFAULT 'draft',
  created_by          uuid NOT NULL REFERENCES users(id),
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT teaching_units_scope_scope_id_chk CHECK (
    (scope = 'platform' AND scope_id IS NULL) OR
    (scope IN ('org', 'personal') AND scope_id IS NOT NULL)
  ),
  CONSTRAINT teaching_units_status_chk CHECK (
    status IN ('draft', 'reviewed', 'classroom_ready', 'coach_ready', 'archived')
  )
);

CREATE INDEX IF NOT EXISTS teaching_units_scope_scope_id_status_idx
  ON teaching_units(scope, scope_id, status);
CREATE INDEX IF NOT EXISTS teaching_units_created_by_idx
  ON teaching_units(created_by);
CREATE UNIQUE INDEX IF NOT EXISTS teaching_units_scope_slug_uniq
  ON teaching_units(scope, COALESCE(scope_id::text, ''), slug) WHERE slug IS NOT NULL;
CREATE INDEX IF NOT EXISTS teaching_units_subject_tags_gin_idx
  ON teaching_units USING GIN (subject_tags);
CREATE INDEX IF NOT EXISTS teaching_units_standards_tags_gin_idx
  ON teaching_units USING GIN (standards_tags);

CREATE TABLE IF NOT EXISTS unit_documents (
  unit_id    uuid PRIMARY KEY REFERENCES teaching_units(id) ON DELETE CASCADE,
  blocks     jsonb NOT NULL DEFAULT '{"type":"doc","content":[]}'::jsonb,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS unit_revisions (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  unit_id     uuid NOT NULL REFERENCES teaching_units(id) ON DELETE CASCADE,
  blocks      jsonb NOT NULL,
  reason      varchar(255),
  created_by  uuid NOT NULL REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS unit_revisions_unit_created_idx
  ON unit_revisions(unit_id, created_at DESC);

COMMIT;
```

- [ ] **Step 2: Apply to dev + test DBs**

```bash
psql postgresql://work@127.0.0.1:5432/bridge      -f drizzle/0016_teaching_units.sql
psql postgresql://work@127.0.0.1:5432/bridge_test -f drizzle/0016_teaching_units.sql
```

Run each twice. Second run should emit only "already exists, skipping" NOTICEs.

- [ ] **Step 3: Verify**

```bash
psql postgresql://work@127.0.0.1:5432/bridge -c "\\d teaching_units"
psql postgresql://work@127.0.0.1:5432/bridge -c "\\d unit_documents"
psql postgresql://work@127.0.0.1:5432/bridge -c "\\d unit_revisions"
```

Expected: three tables present with the CHECK constraints and indexes shown.

- [ ] **Step 4: Drizzle schema.ts — add table exports**

Add to `src/lib/db/schema.ts` (follow the pattern from `problems` added in plan 028 — varchars with `.$type<...>()` for TS narrowing, GIN indexes declared in the table options):

```ts
export const teachingUnits = pgTable(
  "teaching_units",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    scope: varchar("scope", { length: 16 }).$type<"platform" | "org" | "personal">().notNull(),
    scopeId: uuid("scope_id"),
    title: varchar("title", { length: 255 }).notNull(),
    slug: varchar("slug", { length: 255 }),
    summary: text("summary").notNull().default(""),
    gradeLevel: varchar("grade_level", { length: 8 }).$type<"K-5" | "6-8" | "9-12" | null>(),
    subjectTags: text("subject_tags").array().notNull().default([]),
    standardsTags: text("standards_tags").array().notNull().default([]),
    estimatedMinutes: integer("estimated_minutes"),
    status: varchar("status", { length: 24 })
      .$type<"draft" | "reviewed" | "classroom_ready" | "coach_ready" | "archived">()
      .notNull()
      .default("draft"),
    createdBy: uuid("created_by").notNull().references(() => users.id),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (t) => ({
    scopeStatusIdx: index("teaching_units_scope_scope_id_status_idx").on(t.scope, t.scopeId, t.status),
    createdByIdx: index("teaching_units_created_by_idx").on(t.createdBy),
  })
);

export const unitDocuments = pgTable(
  "unit_documents",
  {
    unitId: uuid("unit_id").primaryKey().references(() => teachingUnits.id, { onDelete: "cascade" }),
    blocks: jsonb("blocks").$type<Record<string, unknown>>().notNull().default({ type: "doc", content: [] }),
    updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
  }
);

export const unitRevisions = pgTable(
  "unit_revisions",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    unitId: uuid("unit_id").notNull().references(() => teachingUnits.id, { onDelete: "cascade" }),
    blocks: jsonb("blocks").$type<Record<string, unknown>>().notNull(),
    reason: varchar("reason", { length: 255 }),
    createdBy: uuid("created_by").notNull().references(() => users.id),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (t) => ({
    unitCreatedIdx: index("unit_revisions_unit_created_idx").on(t.unitId, t.createdAt),
  })
);
```

- [ ] **Step 5: Update `tests/unit/schema.test.ts`**

Add:

```ts
it("teaching_units + unit_documents + unit_revisions are defined", () => {
  expect(schema.teachingUnits.scope).toBeDefined();
  expect(schema.teachingUnits.status).toBeDefined();
  expect(schema.unitDocuments.blocks).toBeDefined();
  expect(schema.unitRevisions.unitId).toBeDefined();
});
```

- [ ] **Step 6: Run**

```bash
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test node_modules/.bin/vitest run tests/unit/schema.test.ts
```

- [ ] **Step 7: Commit**

```bash
git add drizzle/0016_teaching_units.sql src/lib/db/schema.ts tests/unit/schema.test.ts
git commit -m "feat(031): schema 0015 — teaching_units, unit_documents, unit_revisions"
```

---

### Task 2: Go TeachingUnitStore

**Files:**
- Create: `platform/internal/store/teaching_units.go`
- Create: `platform/internal/store/teaching_units_test.go`

- [ ] **Step 1: Types and store**

```go
package store

type TeachingUnit struct {
    ID               string    `json:"id"`
    Scope            string    `json:"scope"`
    ScopeID          *string   `json:"scopeId"`
    Title            string    `json:"title"`
    Slug             *string   `json:"slug"`
    Summary          string    `json:"summary"`
    GradeLevel       *string   `json:"gradeLevel"`
    SubjectTags      []string  `json:"subjectTags"`
    StandardsTags    []string  `json:"standardsTags"`
    EstimatedMinutes *int      `json:"estimatedMinutes"`
    Status           string    `json:"status"`
    CreatedBy        string    `json:"createdBy"`
    CreatedAt        time.Time `json:"createdAt"`
    UpdatedAt        time.Time `json:"updatedAt"`
}

type UnitDocument struct {
    UnitID    string          `json:"unitId"`
    Blocks    json.RawMessage `json:"blocks"`
    UpdatedAt time.Time       `json:"updatedAt"`
}

type CreateTeachingUnitInput struct {
    Scope            string
    ScopeID          *string
    Title            string
    Slug             *string
    Summary          string
    GradeLevel       *string
    SubjectTags      []string
    StandardsTags    []string
    EstimatedMinutes *int
    Status           string // empty → "draft"
    CreatedBy        string
}

type UpdateTeachingUnitInput struct {
    Title            *string
    Slug             *string
    Summary          *string
    GradeLevel       *string
    SubjectTags      []string // nil = unchanged; empty slice = clear
    StandardsTags    []string
    EstimatedMinutes *int
    Status           *string
}

type TeachingUnitStore struct{ db *sql.DB }
func NewTeachingUnitStore(db *sql.DB) *TeachingUnitStore { return &TeachingUnitStore{db: db} }
```

Methods:
- `CreateUnit(ctx, in CreateTeachingUnitInput) (*TeachingUnit, error)` — inserts into `teaching_units` AND seeds `unit_documents` with the empty `{type:"doc",content:[]}` default in the same transaction.
- `GetUnit(ctx, id string) (*TeachingUnit, error)` — `(nil, nil)` if not found.
- `GetDocument(ctx, unitID string) (*UnitDocument, error)` — fetches the blocks row.
- `UpdateUnit(ctx, id string, in UpdateTeachingUnitInput) (*TeachingUnit, error)` — dynamic setClauses, follows plan 028 pattern.
- `SaveDocument(ctx, unitID string, blocks json.RawMessage) (*UnitDocument, error)` — upserts into `unit_documents`; bumps `updated_at`. Also bumps `teaching_units.updated_at`.
- `DeleteUnit(ctx, id string) (*TeachingUnit, error)` — hard delete; cascades remove `unit_documents` and `unit_revisions`.
- `ListUnitsForScope(ctx, scope, scopeID string) ([]TeachingUnit, error)` — simple list without pagination; a future plan adds cursor paging for the discovery surface.

Follow patterns from `platform/internal/store/problems.go` (plan 028) exactly: `pq.Array` for text arrays, `encoding/json` for JSONB, `sql.NullString` for nullables, package-level sentinel errors if needed.

- [ ] **Step 2: Column list helper**

```go
const teachingUnitColumns = `id, scope, scope_id, title, slug, summary,
  grade_level, subject_tags, standards_tags, estimated_minutes, status,
  created_by, created_at, updated_at`
```

- [ ] **Step 3: Tests (`teaching_units_test.go`)**

```
TestTeachingUnitStore_Create_Platform         — scope=platform, scope_id=nil
TestTeachingUnitStore_Create_Org              — scope=org, scope_id set
TestTeachingUnitStore_Create_Personal         — scope=personal, scope_id=userId
TestTeachingUnitStore_Create_Seeds_Document   — CreateUnit also creates an empty unit_documents row
TestTeachingUnitStore_Create_CheckConstraint  — scope=platform + scope_id set → 23514 CHECK violation
TestTeachingUnitStore_GetDocument             — returns the seeded empty doc
TestTeachingUnitStore_SaveDocument_Upsert     — first save inserts, second save updates; updated_at moves forward
TestTeachingUnitStore_SaveDocument_BumpsUnitUpdatedAt
TestTeachingUnitStore_UpdateUnit_Partial      — only provided fields change
TestTeachingUnitStore_UpdateUnit_SubjectTags  — empty slice clears; nil leaves unchanged
TestTeachingUnitStore_DeleteUnit_Cascades     — delete unit → document row gone
```

Reuse test fixture helpers from `platform/internal/store/problems_test.go` (`createTestUser`, `createTestOrg`, etc.) — they are package-internal `_test.go` helpers accessible from sibling test files.

- [ ] **Step 4: Run + commit**

```bash
cd platform && DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./internal/store/... -run 'TestTeachingUnitStore_' -count=1 -v

git add platform/internal/store/teaching_units.go platform/internal/store/teaching_units_test.go
git commit -m "feat(031): TeachingUnitStore — CRUD + document upsert + cascade"
```

---

### Task 3: Go TeachingUnitHandler

**Files:**
- Create: `platform/internal/handlers/teaching_units.go`
- Create: `platform/internal/handlers/teaching_units_integration_test.go`
- Modify: `platform/internal/handlers/stores.go`
- Modify: `platform/cmd/api/main.go`
- Modify: `next.config.ts`

- [ ] **Step 1: `stores.go` — add store pointer**

Add `TeachingUnits *store.TeachingUnitStore` to the struct and wire `NewTeachingUnitStore(db)` in the `NewStores(db)` builder.

- [ ] **Step 2: Handler routes**

```go
type TeachingUnitHandler struct {
    Units *store.TeachingUnitStore
    Orgs  *store.OrgStore
}

func (h *TeachingUnitHandler) Routes(r chi.Router) {
    r.Route("/api/units", func(r chi.Router) {
        r.Get("/",  h.ListUnits)
        r.Post("/", h.CreateUnit)
    })
    r.Route("/api/units/{id}", func(r chi.Router) {
        r.Use(ValidateUUIDParam("id"))
        r.Get("/",    h.GetUnit)
        r.Patch("/",  h.UpdateUnit)
        r.Delete("/", h.DeleteUnit)
        r.Get("/document",  h.GetDocument)
        r.Put("/document",  h.SaveDocument)
    })
}
```

Note: lifecycle transition routes (`/publish`, `/archive`, etc.) and overlay/fork routes are **explicitly excluded** — they land in plans 033 and 034.

- [ ] **Step 3: Access helpers**

Factor out a minimal shared access helper. If `platform/internal/handlers/problem_access.go` already has `authorizedForScope` / `canViewProblemRow`, **reuse the pattern but do not force-share the same function**: units have the same shape (scope, scope_id) but no attachment-grant path yet (attachment via sessions/assignments lands in plan 032). Write unit-specific helpers inline at the top of `teaching_units.go`:

```go
// canViewUnit applies the Access table from spec 012 §Access, with the
// plan-031-specific narrowing: org students are denied access entirely
// until class-binding lands in plan 032.
//
// Plan 031 has no lifecycle transitions, so status is just a stored field.
func (h *TeachingUnitHandler) canViewUnit(ctx context.Context, c *auth.Claims, u *store.TeachingUnit) bool {
    if c.IsPlatformAdmin { return true }
    switch u.Scope {
    case "platform":
        // Published/archived visible to any authenticated user; drafts/reviewed to platform admin only.
        return u.Status == "classroom_ready" || u.Status == "coach_ready" || u.Status == "archived"
    case "org":
        if u.ScopeID == nil { return false }
        roles, _ := h.Orgs.GetUserRolesInOrg(ctx, *u.ScopeID, c.UserID)
        for _, m := range roles {
            if m.Status != "active" { continue }
            // Plan 031 only grants teachers and org_admins access. Students
            // are denied until plan 032 wires class/session binding.
            if m.Role == "org_admin" || m.Role == "teacher" { return true }
        }
        return false
    case "personal":
        return u.ScopeID != nil && *u.ScopeID == c.UserID
    }
    return false
}

// canEditUnit is the same function used for edit + create + delete
// per spec 012 §Access policy.
func (h *TeachingUnitHandler) canEditUnit(ctx context.Context, c *auth.Claims, scope string, scopeID *string) bool {
    switch scope {
    case "platform":
        return c.IsPlatformAdmin
    case "org":
        if scopeID == nil { return false }
        roles, _ := h.Orgs.GetUserRolesInOrg(ctx, *scopeID, c.UserID)
        for _, m := range roles {
            if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") { return true }
        }
        return false
    case "personal":
        return scopeID != nil && *scopeID == c.UserID
    }
    return false
}
```

- [ ] **Step 4: Handler bodies**

Use plan 028's `ProblemHandler` as a template. Error model:
- 404 when caller cannot view (never leak existence).
- 403 when caller can view but cannot edit.
- 409 on CHECK constraint violation surfaced from the DB (e.g. invalid scope/scope_id combination).
- 400 on validation errors (empty title, invalid enum values).

`SaveDocument` accepts a raw JSON body and writes it to `unit_documents.blocks`. Server-side validation must enforce:

1. The outer envelope is `{ "type": "doc", "content": [...] }`.
2. **Every top-level block has a non-empty `attrs.id` string.** Walk `content`; reject with 400 if any top-level node is missing `attrs.id` or has `attrs.id === ""`. This enforces the spec-012 block-ID invariant server-side — client-side hooks in the Tiptap editor are not the only line of defense.
3. (Light check only — deeper shape validation lives in the editor.) Every top-level block's `type` is one of the known block types for this plan: `prose`, `problem-ref`. Future plans expand this allowlist; keep the allowlist centralised as a Go constant the server reads so Task 3's tests can flip values.

Anything that fails these checks → 400 with a descriptive error (e.g., `"block at index 3 is missing attrs.id"`). This catches a future API caller (or a misconfigured client) that persists structurally-valid JSON but breaks the overlay/reuse invariant.

- [ ] **Step 5: Wire into `cmd/api/main.go`**

Inside the authenticated route group:

```go
unitH := &handlers.TeachingUnitHandler{
    Units: stores.TeachingUnits,
    Orgs:  stores.Orgs,
}
unitH.Routes(r)
```

- [ ] **Step 6: Add `next.config.ts` proxy entry**

Append `"/api/units/:path*"` to `GO_PROXY_ROUTES`.

- [ ] **Step 7: Integration tests (`teaching_units_integration_test.go`)**

Matrix to cover:

```
GET /api/units
  - Lists only units caller can view per scope matrix
  - Platform admin sees all
  - Org teacher sees own org's draft/reviewed; student sees only ready/archived

POST /api/units
  - Platform admin creates platform-scope unit → 201
  - Teacher creates org-scope unit in own org → 201
  - Teacher creates org-scope unit in another org → 403
  - Student creates org-scope unit → 403
  - Anyone creates personal-scope unit where scope_id=self → 201
  - Create with scope=platform + scope_id set → 400 (rejected client-side) or 409 (DB CHECK)

GET /api/units/{id}
  - View matrix: platform published → any auth 200; platform draft → non-admin 404
  - org classroom_ready/coach_ready → same-org teacher 200; same-org student 404 (plan-031 narrowing); other-org 404
  - org draft/reviewed → same-org teacher 200; same-org student 404
  - personal → owner 200; non-owner 404

PATCH /api/units/{id}
  - Teacher updates own-org unit → 200
  - Student updates org unit → 403
  - Non-member updates org unit → 404 (never leak existence)

DELETE /api/units/{id}
  - Teacher deletes own-org unit → 204
  - Cascade: unit_documents row is gone after delete

GET /api/units/{id}/document
  - Returns the seeded empty doc for a newly created unit
  - 404 if caller cannot view the unit

PUT /api/units/{id}/document
  - Valid blocks → 200; updated_at bumps
  - Non-editor → 403
  - Invalid envelope (missing "type":"doc") → 400
  - Top-level block missing attrs.id → 400 ("block at index N is missing attrs.id")
  - Top-level block with unknown type (not prose / problem-ref in plan 031) → 400
```

- [ ] **Step 8: Run + commit**

```bash
cd platform && DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./internal/handlers/... -run 'TeachingUnit' -count=1 -v

git add platform/internal/handlers/teaching_units.go \
        platform/internal/handlers/teaching_units_integration_test.go \
        platform/internal/handlers/stores.go \
        platform/cmd/api/main.go \
        next.config.ts
git commit -m "feat(031): TeachingUnitHandler — scope-aware CRUD + document save"
```

---

### Task 4: Tiptap editor + `problem-ref` custom node

**Files:**
- Modify: `package.json` — add Tiptap deps
- Create: `src/components/editor/tiptap/extensions.ts`
- Create: `src/components/editor/tiptap/problem-ref-node.tsx`
- Create: `src/components/editor/tiptap/teaching-unit-editor.tsx`
- Create: `src/components/editor/tiptap/teaching-unit-viewer.tsx`

- [ ] **Step 1: Add deps**

```bash
bun add @tiptap/react @tiptap/pm @tiptap/starter-kit
```

Optionally for prose code highlighting: `@tiptap/extension-code-block-lowlight lowlight`. Defer if the MVP doesn't need it.

- [ ] **Step 2: `extensions.ts`**

```ts
import StarterKit from "@tiptap/starter-kit";
import { ProblemRefNode } from "./problem-ref-node";

export function teachingUnitExtensions() {
  return [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
    }),
    ProblemRefNode,
  ];
}
```

- [ ] **Step 3: `problem-ref-node.tsx`**

Custom Tiptap `Node` with:
- `name: "problem-ref"`
- `group: "block"`
- `atom: true` (not editable inline; the whole node is the block)
- `attrs`: `id: string` (block id), `problemId: string`, `pinnedRevision: string | null`, `visibility: "always" | "when-unit-active"`, `overrideStarter: string | null`
- `parseHTML`: matches `[data-type="problem-ref"]`
- `renderHTML`: emits a `div` with data-attrs — the React view renders the card.
- `addNodeView`: `ReactNodeViewRenderer(ProblemRefNodeView)` — a React component that fetches `GET /api/problems/{problemId}` and displays `{title, difficulty, tags}` inside a card using shadcn/ui components. While loading, show a skeleton. If 404/403, show "Problem unavailable."

Provide a slash-command (`/problem`) inserter via a separate `InputRule` or expose an insert button on the editor toolbar that prompts for a problem ID or opens a lightweight picker (stub the picker as a plain input in plan 031; a proper picker lands in plan 033).

Every new `problem-ref` block gets a freshly generated `attrs.id` (nanoid). The editor never reuses ids.

- [ ] **Step 4: `teaching-unit-editor.tsx`**

```tsx
export interface TeachingUnitEditorProps {
  initialDoc?: JSONContent;           // from unit_documents.blocks
  onSave: (doc: JSONContent) => Promise<void>;
  onDirty?: (dirty: boolean) => void;
}

export function TeachingUnitEditor({ initialDoc, onSave, onDirty }: TeachingUnitEditorProps) {
  const editor = useEditor({
    extensions: teachingUnitExtensions(),
    content: initialDoc ?? { type: "doc", content: [] },
    onUpdate: ({ editor }) => {
      onDirty?.(true);
    },
  });
  // ... render <EditorContent editor={editor} /> + Save button + "Insert problem" toolbar button
}
```

The save button serializes `editor.getJSON()` and calls `onSave`. The parent page wires this to `PUT /api/units/{id}/document`.

All prose blocks and the custom `problem-ref` node get `attrs.id` automatically — implement a small `onCreate` / `onUpdate` hook that walks new top-level nodes missing an id and assigns one (nanoid). This enforces the spec-012 invariant that every block has a stable id even before overlays land in plan 034.

- [ ] **Step 5: `teaching-unit-viewer.tsx`**

Same extensions, `editable: false`. Renders the saved doc without toolbar. Used by `/teacher/units/{id}/page.tsx`.

- [ ] **Step 6: Mid-task verification — deps + extensions build cleanly**

After Steps 1–3 (deps + `extensions.ts` + `problem-ref-node.tsx`), before writing the editor and viewer components:

```bash
node_modules/.bin/tsc --noEmit 2>&1 | grep -E "tiptap|problem-ref" | head
```

No errors in the files touched so far. Then commit **Sub-task 4a**:

```bash
git add package.json bun.lock src/components/editor/tiptap/extensions.ts src/components/editor/tiptap/problem-ref-node.tsx
git commit -m "feat(031): Tiptap deps + extensions + problem-ref custom node"
```

- [ ] **Step 7: Editor + viewer components**

Write `teaching-unit-editor.tsx` (Step 4 content) and `teaching-unit-viewer.tsx` (Step 5 content). Verify the TS compiles:

```bash
node_modules/.bin/tsc --noEmit 2>&1 | grep -E "teaching-unit" | head
```

No new errors.

- [ ] **Step 8: Commit — Sub-task 4b**

```bash
git add src/components/editor/tiptap/teaching-unit-editor.tsx src/components/editor/tiptap/teaching-unit-viewer.tsx
git commit -m "feat(031): TeachingUnitEditor + TeachingUnitViewer components"
```

---

### Task 5: Teacher-facing pages

**Files:**
- Create: `src/app/(portal)/teacher/units/new/page.tsx`
- Create: `src/app/(portal)/teacher/units/[id]/edit/page.tsx`
- Create: `src/app/(portal)/teacher/units/[id]/page.tsx`
- Create: `src/lib/teaching-units.ts`

- [ ] **Step 1: Client-side API helpers (`src/lib/teaching-units.ts`)**

```ts
export async function createUnit(input: CreateUnitInput): Promise<TeachingUnit> {
  const res = await fetch("/api/units", { method: "POST", body: JSON.stringify(input), headers: { "Content-Type": "application/json" } });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function fetchUnit(id: string): Promise<TeachingUnit> { /* GET /api/units/{id} */ }
export async function fetchDocument(id: string): Promise<{ blocks: JSONContent }> { /* GET /api/units/{id}/document */ }
export async function saveDocument(id: string, blocks: JSONContent): Promise<void> { /* PUT /api/units/{id}/document */ }
```

Define a TS `TeachingUnit` type that mirrors the Go struct.

- [ ] **Step 2: `/teacher/units/new/page.tsx`**

Server component: auth, check the caller is a teacher in at least one org (or platform admin). Render a client component with a form for title + scope picker (`personal` default; orgs the teacher belongs to; `platform` only if platform admin). On submit → `createUnit(...)` → `router.push(/teacher/units/${newId}/edit)`.

- [ ] **Step 3: `/teacher/units/[id]/edit/page.tsx`**

Server component: fetch unit metadata + document. If caller cannot view/edit → `notFound()`. Render `<TeachingUnitEditor initialDoc={doc.blocks} onSave={(doc) => saveDocument(id, doc)} />` in a client boundary.

Debounced autosave is NICE to have but not required in plan 031 — a manual "Save" button is fine. Autosave lands in plan 035 alongside realtime.

- [ ] **Step 4: `/teacher/units/[id]/page.tsx`**

Read-only: fetch metadata + doc, render `<TeachingUnitViewer doc={doc.blocks} />` plus an Edit button linking to `/edit`.

- [ ] **Step 5: Manual smoke test**

```bash
PORT=3003 bun run dev
# terminal 2:
cd platform && air
# browse to /teacher/units/new, create a unit, insert a problem-ref via the toolbar,
# save, reopen at /teacher/units/{id}/edit, reload — the doc round-trips cleanly.
```

Confirm:
- Create form produces a valid unit in the DB.
- Editor opens with the seeded empty doc.
- Prose blocks get ids automatically.
- `problem-ref` blocks fetch the referenced problem and render a card.
- Save roundtrips through the API.
- Viewer renders the saved doc read-only.

- [ ] **Step 6: Commit**

```bash
git add "src/app/(portal)/teacher/units/" src/lib/teaching-units.ts
git commit -m "feat(031): teacher unit editor + viewer pages"
```

---

### Task 6: Vitest coverage for the editor

**Files:**
- Create: `tests/unit/teaching-unit-editor.test.tsx`

- [ ] **Step 1: Tests**

Scope: unit-level behavior of `<TeachingUnitEditor />`. Not e2e.

```
renders with initial doc
assigns attrs.id to new prose blocks
inserts a problem-ref via the toolbar → node appears in editor.getJSON()
calls onSave with editor.getJSON() when Save clicked
onDirty fires on first input
```

Use `@testing-library/react`. Mock `fetch` for any problem-ref lookups.

- [ ] **Step 2: Run + commit**

```bash
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test node_modules/.bin/vitest run tests/unit/teaching-unit-editor.test.tsx

git add tests/unit/teaching-unit-editor.test.tsx
git commit -m "feat(031): vitest coverage for TeachingUnitEditor"
```

---

### Task 7: Verify, docs, post-execution

**Files:**
- Modify: `docs/api.md`
- Modify: `TODO.md`
- Modify: `docs/plans/031-teaching-unit-mvp.md` — append post-execution report

- [ ] **Step 1: Full Go suite**

```bash
cd platform && DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./... -count=1 -timeout 180s
```

Green required.

- [ ] **Step 2: Full Vitest suite**

```bash
cd /home/chris/workshop/Bridge && DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test node_modules/.bin/vitest run
```

Green required.

- [ ] **Step 3: Type-check**

```bash
node_modules/.bin/tsc --noEmit
```

No new errors for files touched in this plan. Pre-existing errors (stale `.next` cache, unrelated files) are acceptable but noted in the report.

- [ ] **Step 4: `docs/api.md` additions**

Add a "Teaching Units" section documenting:
- `GET /api/units` — list units the caller can view
- `POST /api/units` — create; request body, response shape, status codes
- `GET /api/units/{id}` — get metadata
- `PATCH /api/units/{id}` — update metadata
- `DELETE /api/units/{id}` — hard delete (409 if something references the unit — though plan 031 has no references)
- `GET /api/units/{id}/document` — fetch the blocks
- `PUT /api/units/{id}/document` — save the blocks

Keep it concise — spec 012 covers the semantics.

- [ ] **Step 5: Update `TODO.md`**

Add follow-up items surfaced during implementation (e.g., "proper problem picker for problem-ref insertion", "autosave in plan 035", "unit listing/discovery in plan 036").

- [ ] **Step 6: Post-execution report**

Append a `## Post-Execution Report` section to this plan with:
- Files created/modified
- Test coverage delta
- Known limitations (no lifecycle, no overlay, no projection, no student-facing surfaces — explicitly deferred per scope boundaries above)
- Verification output (final Go + Vitest tail)

- [ ] **Step 7: Push + PR**

```bash
git add docs/api.md TODO.md docs/plans/031-teaching-unit-mvp.md
git commit -m "docs(031): API reference, TODO, post-execution report"

git push -u origin feat/031-teaching-unit-mvp

GH_TOKEN=$(cat .gh-token) gh pr create \
  --title "feat: teaching unit MVP — data model + minimal Tiptap editor (plan 031 / spec 012)" \
  --body-file docs/plans/031-teaching-unit-mvp.md
```

---

## Code Review

Reviewers append findings here following `docs/code-review.md` format. Author responds inline with `→ Response:` and status `[FIXED]` / `[WONTFIX]`.
