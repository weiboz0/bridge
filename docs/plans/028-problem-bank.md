# 028 — Problem Bank

**Goal:** Reshape `problems` into a first-class bank per spec 009 — three scopes (platform / org / personal), many-to-many topic attachment via `topic_problems`, reference `problem_solutions`, headless (no UI). Leaves every current user-visible flow working after migration.

**Architecture:** One atomic Drizzle migration rewrites the `problems` table, backfills `topic_problems` from today's `problems.topic_id`, and creates `problem_solutions` empty. The Go store + handler layer gains scope-aware CRUD, an explicit fork action, and two nested resources (solutions, topic-attachments). The student problem page and teacher watcher re-derive language from `class_settings.editor_mode` instead of `problem.language` (removed). Bank browse UI is deferred to spec 011.

**Tech Stack:** PostgreSQL 15 migration (SQL) · Go (Chi, `database/sql` via pgx, testify) · Drizzle (`schema.ts`) · existing `auth` middleware and `RequireAuth` group.

**Branch:** `feat/009-problem-bank`

**Prereqs:** Spec 009 (`docs/specs/009-problem-bank.md`); plan 024 (existing `ProblemStore`, `TestCaseStore`, `AttemptStore`); plan 027 merged (classrooms gone, `class_settings.editor_mode` is the canonical class-side editor config).

---

## File Structure

| File | Responsibility |
|---|---|
| `drizzle/0013_problem_bank.sql` | Single-transaction migration: reshape `problems`, backfill + create `topic_problems`, create `problem_solutions`, add indexes + CHECK constraint. |
| `src/lib/db/schema.ts` | Drizzle table definitions for the reshaped `problems` + new `topicProblems` and `problemSolutions`; drop old `language`/`topicId`/`order` columns. |
| `platform/internal/store/problems.go` | Expanded `ProblemStore` — new fields, scope-filtered `ListProblems`, `ForkProblem`, `SetStatus`. |
| `platform/internal/store/problem_solutions.go` | **New** `ProblemSolutionStore` — CRUD + `SetPublished`. |
| `platform/internal/store/topic_problems.go` | **New** `TopicProblemStore` — attach / detach / reorder / list. |
| `platform/internal/store/test_cases.go` | Add `IsEditorFilter` helper on `ListTestCases` so handler can shell out hidden canonical rows for non-editors. |
| `platform/internal/store/problems_test.go` | Extend with scope filters, fork semantics, CHECK constraint, cascade. |
| `platform/internal/store/problem_solutions_test.go` | **New.** |
| `platform/internal/store/topic_problems_test.go` | **New.** |
| `platform/internal/handlers/problems.go` | Expand: scope-aware list + create, `/fork`, `/publish`, `/archive`, `/unarchive`, strict DELETE guard. |
| `platform/internal/handlers/problem_solutions.go` | **New.** Nested under `/api/problems/{id}/solutions`. |
| `platform/internal/handlers/topic_problems.go` | **New.** Nested under `/api/topics/{topicId}/problems`. |
| `platform/internal/handlers/problems_test.go` | New coverage: scope filters, fork, lifecycle, DELETE guard. |
| `platform/internal/handlers/problem_solutions_test.go` | **New.** |
| `platform/internal/handlers/topic_problems_test.go` | **New.** |
| `platform/cmd/api/main.go` | Wire `SolutionHandler` and `TopicProblemHandler`. |
| `scripts/seed_python_101.sql` | Rewrite: insert problems with `scope='org'`, attach via `topic_problems`, add a sample `problem_solution` per problem. |
| `src/app/(portal)/student/classes/[id]/problems/[problemId]/page.tsx` | Read `starter_code[editorMode]` from JSONB map; drop `problem.language` usage. |
| `src/app/(portal)/teacher/classes/[id]/students/[studentId]/problems/[problemId]/page.tsx` | Same adjustment. |
| `docs/api.md` | New Problems / Solutions / Topic-Problems sections describing endpoint shapes. |
| `TODO.md` | Tick off "Problem bank" item; add follow-ups surfaced during implementation. |

---

## Access policy reference

For every handler task below, the canonical access rules come from **spec 009 §Access policy**. Don't re-derive them — link back to:

- View: scope grant OR attachment grant OR draft carve-out OR platform admin bypass.
- Edit/create/delete: platform admins (platform), org_admin or teacher in org_memberships (org), `created_by = userId` (personal).
- Attach/detach: org_admin or teacher in target course's org AND can view AND `status='published'`.
- Attempt visibility (teacher): teaches class K, student is in K, problem attached to a topic in K's course.

---

### Task 1: Schema migration (`drizzle/0013_problem_bank.sql`)

**Files:**
- Create: `drizzle/0013_problem_bank.sql`

- [ ] **Step 1: Write the migration**

```sql
-- Plan 028 / spec 009: reshape `problems` into a scoped bank with many-to-many
-- topic attachment, and introduce `problem_solutions` for reference answers.
--
-- Existing rows (today: topic_bound, language-specific) are backfilled as
-- org-scoped problems attached to the same topic they came from, with the
-- starter_code migrated into a jsonb map keyed by the old language column.
-- Single transaction; idempotent where practical so dev DBs mid-state apply
-- cleanly.

BEGIN;

-- 1. Add new columns (nullable initially for backfill safety).
ALTER TABLE problems
  ADD COLUMN IF NOT EXISTS scope           varchar(16),
  ADD COLUMN IF NOT EXISTS scope_id        uuid,
  ADD COLUMN IF NOT EXISTS slug            varchar(255),
  ADD COLUMN IF NOT EXISTS starter_code_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS difficulty      varchar(16),
  ADD COLUMN IF NOT EXISTS grade_level     varchar(8),
  ADD COLUMN IF NOT EXISTS tags            text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS status          varchar(16),
  ADD COLUMN IF NOT EXISTS forked_from     uuid REFERENCES problems(id),
  ADD COLUMN IF NOT EXISTS time_limit_ms   int,
  ADD COLUMN IF NOT EXISTS memory_limit_mb int;

-- 2. Backfill scope / scope_id / status / difficulty / grade_level / starter_code_json.
UPDATE problems p
SET
  scope = 'org',
  scope_id = c.org_id,
  status = 'published',
  difficulty = COALESCE(p.difficulty, 'easy'),
  grade_level = c.grade_level,
  starter_code_json = CASE
    WHEN p.starter_code IS NULL OR p.starter_code = ''
      THEN '{}'::jsonb
    ELSE jsonb_build_object(p.language, p.starter_code)
  END
FROM topics t, courses c
WHERE p.topic_id = t.id AND t.course_id = c.id;

-- 3. Tighten constraints now that every row has values.
ALTER TABLE problems
  ALTER COLUMN scope      SET NOT NULL,
  ALTER COLUMN difficulty SET NOT NULL,
  ALTER COLUMN status     SET NOT NULL,
  ALTER COLUMN starter_code_json SET NOT NULL;

-- scope_id stays nullable (platform scope uses NULL scope_id).
ALTER TABLE problems
  ADD CONSTRAINT problems_scope_scope_id_chk CHECK (
    (scope = 'platform' AND scope_id IS NULL) OR
    (scope IN ('org', 'personal') AND scope_id IS NOT NULL)
  );

-- 4. Create topic_problems and backfill from the current problems.topic_id.
CREATE TABLE IF NOT EXISTS topic_problems (
  topic_id    uuid NOT NULL REFERENCES topics(id)   ON DELETE CASCADE,
  problem_id  uuid NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
  sort_order  int  NOT NULL DEFAULT 0,
  attached_by uuid NOT NULL REFERENCES users(id),
  attached_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (topic_id, problem_id)
);
CREATE INDEX IF NOT EXISTS topic_problems_problem_idx ON topic_problems(problem_id);

INSERT INTO topic_problems (topic_id, problem_id, sort_order, attached_by, attached_at)
SELECT p.topic_id, p.id, p."order", p.created_by, p.created_at
FROM problems p
ON CONFLICT DO NOTHING;  -- re-run safe

-- 5. Drop the now-redundant columns on problems.
ALTER TABLE problems
  DROP COLUMN IF EXISTS topic_id,
  DROP COLUMN IF EXISTS language,
  DROP COLUMN IF EXISTS "order",
  DROP COLUMN IF EXISTS starter_code;  -- was the old text column; JSONB replaces it

-- Rename the new jsonb column to its final name.
ALTER TABLE problems RENAME COLUMN starter_code_json TO starter_code;

-- 6. Create problem_solutions (empty at migration).
CREATE TABLE IF NOT EXISTS problem_solutions (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  problem_id     uuid NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
  language       varchar(32) NOT NULL,
  title          varchar(120),
  code           text NOT NULL,
  notes          text,
  approach_tags  text[] NOT NULL DEFAULT '{}',
  is_published   boolean NOT NULL DEFAULT false,
  created_by     uuid NOT NULL REFERENCES users(id),
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS problem_solutions_problem_language_idx
  ON problem_solutions(problem_id, language);

-- 7. Indexes on the reshaped problems table.
DROP INDEX IF EXISTS problems_topic_order_idx;  -- column gone
CREATE INDEX IF NOT EXISTS problems_scope_scope_id_status_idx
  ON problems(scope, scope_id, status);
CREATE INDEX IF NOT EXISTS problems_created_by_idx ON problems(created_by);
CREATE UNIQUE INDEX IF NOT EXISTS problems_scope_slug_uniq
  ON problems(scope, COALESCE(scope_id::text, ''), slug) WHERE slug IS NOT NULL;
CREATE INDEX IF NOT EXISTS problems_tags_gin_idx ON problems USING GIN (tags);

COMMIT;
```

- [ ] **Step 2: Apply to dev DB**

```bash
psql postgresql://work@127.0.0.1:5432/bridge -f drizzle/0013_problem_bank.sql
```

Expected: no errors. Re-run once; must be idempotent (second run produces only "already exists" NOTICEs).

- [ ] **Step 3: Apply to test DB**

```bash
psql postgresql://work@127.0.0.1:5432/bridge_test -f drizzle/0013_problem_bank.sql
```

- [ ] **Step 4: Smoke-verify the migration**

```bash
psql postgresql://work@127.0.0.1:5432/bridge -c "\d problems"
psql postgresql://work@127.0.0.1:5432/bridge -c "\d topic_problems"
psql postgresql://work@127.0.0.1:5432/bridge -c "\d problem_solutions"
psql postgresql://work@127.0.0.1:5432/bridge -c "SELECT COUNT(*) FROM topic_problems;"
psql postgresql://work@127.0.0.1:5432/bridge -c "SELECT COUNT(*) FROM problems WHERE scope = 'org' AND scope_id IS NULL;"
```

Expected: `problems.topic_id` is GONE; `problems.scope` and `problems.starter_code` (jsonb) are present; `topic_problems` row count equals the prior `problems` row count; zero org-scoped problems with NULL scope_id (CHECK holds).

- [ ] **Step 5: Commit**

```bash
git add drizzle/0013_problem_bank.sql
git commit -m "feat(028): schema 0013 — problem bank scopes + topic_problems + problem_solutions"
```

---

### Task 2: Drizzle `schema.ts` update

**Files:**
- Modify: `src/lib/db/schema.ts` — reshape `problems`, drop the legacy columns; add `topicProblems` and `problemSolutions` table definitions.

- [ ] **Step 1: Locate current `problems` definition**

```bash
grep -n "^export const problems" src/lib/db/schema.ts
```

- [ ] **Step 2: Replace with reshaped definition + new tables**

Drop `topicId`, `language`, `order`, `starterCode` (string). Add the new shape:

```ts
// scope enum — mirrors the DB CHECK in drizzle/0013
export const problemScopeEnum = pgEnum("problem_scope", ["platform", "org", "personal"]);
export const problemDifficultyEnum = pgEnum("problem_difficulty", ["easy", "medium", "hard"]);
export const problemStatusEnum = pgEnum("problem_status", ["draft", "published", "archived"]);

// NOTE: 0013 creates these as varchar(16) CHECK'd columns, not true PG enums,
// to keep the migration additive. We expose Drizzle enums purely for TS
// type-narrowing — the DB column remains varchar. If the codebase later
// converts to true enums, keep the TS names aligned.

export const problems = pgTable(
  "problems",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    scope: varchar("scope", { length: 16 }).$type<"platform" | "org" | "personal">().notNull(),
    scopeId: uuid("scope_id"),
    title: varchar("title", { length: 255 }).notNull(),
    slug: varchar("slug", { length: 255 }),
    description: text("description").notNull(),
    starterCode: jsonb("starter_code").$type<Record<string, string>>().notNull().default({}),
    difficulty: varchar("difficulty", { length: 16 })
      .$type<"easy" | "medium" | "hard">().notNull().default("easy"),
    gradeLevel: varchar("grade_level", { length: 8 }).$type<"K-5" | "6-8" | "9-12" | null>(),
    tags: text("tags").array().notNull().default([]),
    status: varchar("status", { length: 16 })
      .$type<"draft" | "published" | "archived">().notNull().default("draft"),
    forkedFrom: uuid("forked_from"),
    timeLimitMs: integer("time_limit_ms"),
    memoryLimitMb: integer("memory_limit_mb"),
    createdBy: uuid("created_by").notNull().references(() => users.id),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (t) => ({
    scopeStatusIdx: index("problems_scope_scope_id_status_idx").on(t.scope, t.scopeId, t.status),
    createdByIdx: index("problems_created_by_idx").on(t.createdBy),
  })
);

export const topicProblems = pgTable(
  "topic_problems",
  {
    topicId: uuid("topic_id").notNull().references(() => topics.id, { onDelete: "cascade" }),
    problemId: uuid("problem_id").notNull().references(() => problems.id, { onDelete: "cascade" }),
    sortOrder: integer("sort_order").notNull().default(0),
    attachedBy: uuid("attached_by").notNull().references(() => users.id),
    attachedAt: timestamp("attached_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (t) => ({
    pk: primaryKey({ columns: [t.topicId, t.problemId] }),
    problemIdx: index("topic_problems_problem_idx").on(t.problemId),
  })
);

export const problemSolutions = pgTable(
  "problem_solutions",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    problemId: uuid("problem_id").notNull().references(() => problems.id, { onDelete: "cascade" }),
    language: varchar("language", { length: 32 }).notNull(),
    title: varchar("title", { length: 120 }),
    code: text("code").notNull(),
    notes: text("notes"),
    approachTags: text("approach_tags").array().notNull().default([]),
    isPublished: boolean("is_published").notNull().default(false),
    createdBy: uuid("created_by").notNull().references(() => users.id),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (t) => ({
    problemLangIdx: index("problem_solutions_problem_language_idx").on(t.problemId, t.language),
  })
);
```

- [ ] **Step 3: Type-check**

```bash
node_modules/.bin/tsc --noEmit 2>&1 | grep -i "problems\.\|topicId\|starterCode" | head -20
```

Fix any remaining TS errors where consumers still read `problem.language` or `problem.topicId`. Do not widen scope — record the list of consumers for Task 9; only fix the compile-breakers (if any — many callers go through the HTTP API, not Drizzle directly).

- [ ] **Step 4: Update `tests/unit/schema.test.ts`**

Remove any assertion referencing `problems.topicId`, `problems.language`, `problems.order`, or string `starterCode`. Add:

```ts
it("problems has scope fields + topic_problems + problem_solutions", () => {
  expect(schema.problems.scope).toBeDefined();
  expect(schema.problems.starterCode).toBeDefined();
  expect(schema.topicProblems.problemId).toBeDefined();
  expect(schema.problemSolutions.isPublished).toBeDefined();
});
```

- [ ] **Step 5: Run**

```bash
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test node_modules/.bin/vitest run tests/unit/schema.test.ts
```

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add src/lib/db/schema.ts tests/unit/schema.test.ts
git commit -m "feat(028): Drizzle schema — problem bank shape (scopes, topicProblems, problemSolutions)"
```

---

### Task 3: Expand `ProblemStore`

**Files:**
- Modify: `platform/internal/store/problems.go`
- Modify: `platform/internal/store/problems_test.go`

- [ ] **Step 1: Replace the `Problem` struct + inputs**

```go
type Problem struct {
    ID            string            `json:"id"`
    Scope         string            `json:"scope"`          // platform | org | personal
    ScopeID       *string           `json:"scopeId"`        // NULL when scope=platform
    Title         string            `json:"title"`
    Slug          *string           `json:"slug"`
    Description   string            `json:"description"`
    StarterCode   map[string]string `json:"starterCode"`    // { "python": "...", ... }
    Difficulty    string            `json:"difficulty"`     // easy | medium | hard
    GradeLevel    *string           `json:"gradeLevel"`     // K-5 | 6-8 | 9-12 | null
    Tags          []string          `json:"tags"`
    Status        string            `json:"status"`         // draft | published | archived
    ForkedFrom    *string           `json:"forkedFrom"`
    TimeLimitMs   *int              `json:"timeLimitMs"`
    MemoryLimitMb *int              `json:"memoryLimitMb"`
    CreatedBy     string            `json:"createdBy"`
    CreatedAt     time.Time         `json:"createdAt"`
    UpdatedAt     time.Time         `json:"updatedAt"`
}

type CreateProblemInput struct {
    Scope         string // required
    ScopeID       *string
    Title         string
    Slug          *string
    Description   string
    StarterCode   map[string]string // may be empty
    Difficulty    string            // defaults to "easy" if empty
    GradeLevel    *string
    Tags          []string
    Status        string            // defaults to "draft" if empty
    TimeLimitMs   *int
    MemoryLimitMb *int
    ForkedFrom    *string
    CreatedBy     string
}

type UpdateProblemInput struct {
    Title         *string
    Slug          *string
    Description   *string
    StarterCode   map[string]string // nil = unchanged; empty map = clear to '{}'
    Difficulty    *string
    GradeLevel    *string            // point at "" to clear to NULL
    Tags          []string           // nil = unchanged; empty slice = set to '{}'
    TimeLimitMs   *int
    MemoryLimitMb *int
    // status moves via SetStatus, not UpdateProblem.
}

type ListProblemsFilter struct {
    Scope      string   // "" = any accessible-to-caller
    ScopeID    *string  // must match scope; platform uses nil
    ViewerID   string   // required for "accessible" semantics
    ViewerOrgs []string // org IDs the viewer is a member of
    IsPlatformAdmin bool
    Status     string   // "" = any
    Difficulty string   // ""
    GradeLevel string   // ""
    Tags       []string // AND semantics
    Search     string   // ILIKE on title
    Limit      int      // default 20, max 100
    CursorCreatedAt *time.Time
    CursorID        *string
}
```

Column list helper:

```go
const problemColumns = `id, scope, scope_id, title, slug, description, starter_code,
  difficulty, grade_level, tags, status, forked_from, time_limit_ms, memory_limit_mb,
  created_by, created_at, updated_at`
```

`scanProblem` reads JSONB into `map[string]string` via `json.Unmarshal` on a `[]byte`, and `pq.Array(&tags)` for the text array. Prior art: `platform/internal/store/topics.go` reads `lesson_content jsonb`. Use `github.com/lib/pq` — already vendored (grep confirms: `platform/go.sum`).

- [ ] **Step 2: Rewrite `CreateProblem`**

```go
func (s *ProblemStore) CreateProblem(ctx context.Context, in CreateProblemInput) (*Problem, error) {
    if in.Difficulty == "" { in.Difficulty = "easy" }
    if in.Status == ""     { in.Status = "draft" }
    if in.Tags == nil      { in.Tags = []string{} }
    if in.StarterCode == nil { in.StarterCode = map[string]string{} }
    starter, _ := json.Marshal(in.StarterCode)
    return scanProblem(s.db.QueryRowContext(ctx, `
        INSERT INTO problems (
          scope, scope_id, title, slug, description, starter_code,
          difficulty, grade_level, tags, status, forked_from,
          time_limit_ms, memory_limit_mb, created_by
        ) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11,$12,$13,$14)
        RETURNING `+problemColumns,
        in.Scope, in.ScopeID, in.Title, in.Slug, in.Description, starter,
        in.Difficulty, in.GradeLevel, pq.Array(in.Tags), in.Status, in.ForkedFrom,
        in.TimeLimitMs, in.MemoryLimitMb, in.CreatedBy,
    ))
}
```

- [ ] **Step 3: Rewrite `ListProblems` (replaces `ListProblemsByTopic`)**

`ListProblems(ctx, f ListProblemsFilter)` builds a WHERE dynamically:

```
base := `SELECT `+problemColumns+` FROM problems WHERE 1=1`
// "accessible" semantics when Scope is empty:
//   (scope='platform' AND status='published')
//   OR (scope='org' AND scope_id = ANY($orgs) AND status='published')
//   OR (scope='personal' AND scope_id = $viewer)
//   OR (created_by = $viewer)
//   OR ($viewer_is_platform_admin)
// Explicit scope filter just narrows the above.
```

Pagination uses the `(created_at DESC, id DESC)` cursor pattern from `store.ListOrganizations` (plan 020 reference). Always cap `LIMIT` at 100.

Add helper `ListProblemsByTopic(ctx, topicID) ([]Problem, error)` that joins `topic_problems`:

```go
rows, err := s.db.QueryContext(ctx, `
  SELECT `+strings.Join(prefixCols("p", problemColumnList), ", ")+`, tp.sort_order
  FROM problems p
  JOIN topic_problems tp ON tp.problem_id = p.id
  WHERE tp.topic_id = $1
  ORDER BY tp.sort_order ASC, p.title ASC`, topicID)
```

Return a `[]TopicProblem` type that includes `SortOrder` — the shape handlers use for the `/api/topics/{topicId}/problems` list. Define:

```go
type TopicProblem struct {
    Problem
    SortOrder int `json:"sortOrder"`
}
```

- [ ] **Step 4: `UpdateProblem` — add the new fields**

Follow the existing pattern (dynamic `setClauses`). For `StarterCode`:

```go
if in.StarterCode != nil {
    b, _ := json.Marshal(in.StarterCode)
    setClauses = append(setClauses, fmt.Sprintf("starter_code = $%d::jsonb", argIdx))
    args = append(args, b); argIdx++
}
if in.Tags != nil {
    setClauses = append(setClauses, fmt.Sprintf("tags = $%d", argIdx))
    args = append(args, pq.Array(in.Tags)); argIdx++
}
```

`status` is deliberately NOT settable here; callers must use `SetStatus`.

- [ ] **Step 5: Add `SetStatus` and `ForkProblem`**

```go
func (s *ProblemStore) SetStatus(ctx context.Context, id, newStatus string) (*Problem, error) {
    // Valid transitions: draft→published, published→archived, archived→published.
    // Invalid returns ErrInvalidTransition (sentinel var).
    // ...query uses a WHERE status = <expected> for atomic transition; a 0-row
    //    result means invalid transition (handler maps to 409).
}

var ErrInvalidTransition = errors.New("invalid status transition")

func (s *ProblemStore) ForkProblem(ctx context.Context, sourceID string, target ForkTarget) (*Problem, error) {
    // ForkTarget { Scope, ScopeID, Title *string, CallerID }
    // Open tx. SELECT source. INSERT new problems row with forked_from=source.id.
    // INSERT SELECT canonical test_cases (owner_id IS NULL) into the new problem.
    // INSERT SELECT problem_solutions (all), rewriting created_by = caller.
    // Commit. Return the new problem.
}

type ForkTarget struct {
    Scope    string
    ScopeID  *string
    Title    *string
    CallerID string
}
```

- [ ] **Step 6: Update tests `platform/internal/store/problems_test.go`**

Add tests for:
- `CreateProblem` with scope=platform/scope_id=nil; scope=org/scope_id=orgID; scope=personal/scope_id=userID.
- CHECK constraint: scope=platform + scope_id=<uuid> rejected with `pgerr="23514"`.
- `ListProblems` filtering by scope+scopeID+status+difficulty+tags; AND semantics on tags.
- `ListProblems` "accessible" default: a user sees platform published + own-org published + personal-own, not other-org rows.
- `UpdateProblem` JSONB merge: passing `{python: "..."}` replaces the whole map (documented behavior, not partial merge).
- `SetStatus`: valid transitions pass; invalid returns `ErrInvalidTransition`.
- `ForkProblem`: copies canonical test cases count, copies solutions with `created_by = caller`, does not copy private test cases or attempts, source row is unchanged, new row has `forked_from = source.id`.

Example test shape (single case — the engineer expands the matrix):

```go
func TestListProblems_AccessibleDefault(t *testing.T) {
    ctx := context.Background()
    s := NewProblemStore(testDB)
    orgA := createOrg(t); orgB := createOrg(t)
    viewer := createUser(t); addMembership(t, viewer, orgA, "student")

    pPlatform := mustCreate(t, s, "platform", nil,   "published")
    pOrgA     := mustCreate(t, s, "org",      &orgA, "published")
    pOrgB     := mustCreate(t, s, "org",      &orgB, "published")
    pPersonal := mustCreate(t, s, "personal", &viewer, "draft")

    list, err := s.ListProblems(ctx, ListProblemsFilter{
        ViewerID: viewer, ViewerOrgs: []string{orgA}, Limit: 10,
    })
    require.NoError(t, err)
    ids := idsOf(list)
    require.Contains(t, ids, pPlatform.ID)
    require.Contains(t, ids, pOrgA.ID)
    require.Contains(t, ids, pPersonal.ID)
    require.NotContains(t, ids, pOrgB.ID)
}
```

- [ ] **Step 7: Run tests**

```bash
cd platform && go test ./internal/store/... -run Problem -count=1
```

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add platform/internal/store/problems.go platform/internal/store/problems_test.go
git commit -m "feat(028): ProblemStore — scoped fields, filtered list, fork, status transitions"
```

---

### Task 4: `ProblemSolutionStore` + `TopicProblemStore`

**Files:**
- Create: `platform/internal/store/problem_solutions.go`
- Create: `platform/internal/store/problem_solutions_test.go`
- Create: `platform/internal/store/topic_problems.go`
- Create: `platform/internal/store/topic_problems_test.go`

- [ ] **Step 1: `ProblemSolutionStore`**

```go
package store

import (
    "context"
    "database/sql"
    "errors"
    "time"

    "github.com/lib/pq"
)

type ProblemSolution struct {
    ID            string    `json:"id"`
    ProblemID     string    `json:"problemId"`
    Language      string    `json:"language"`
    Title         *string   `json:"title"`
    Code          string    `json:"code"`
    Notes         *string   `json:"notes"`
    ApproachTags  []string  `json:"approachTags"`
    IsPublished   bool      `json:"isPublished"`
    CreatedBy     string    `json:"createdBy"`
    CreatedAt     time.Time `json:"createdAt"`
    UpdatedAt     time.Time `json:"updatedAt"`
}

type CreateSolutionInput struct {
    ProblemID    string
    Language     string
    Title        *string
    Code         string
    Notes        *string
    ApproachTags []string
    CreatedBy    string
}

type UpdateSolutionInput struct {
    Title        *string
    Code         *string
    Notes        *string
    ApproachTags []string // nil = unchanged
}

type ProblemSolutionStore struct{ db *sql.DB }
func NewProblemSolutionStore(db *sql.DB) *ProblemSolutionStore { return &ProblemSolutionStore{db: db} }

const solutionColumns = `id, problem_id, language, title, code, notes, approach_tags,
  is_published, created_by, created_at, updated_at`

func (s *ProblemSolutionStore) CreateSolution(ctx context.Context, in CreateSolutionInput) (*ProblemSolution, error)
func (s *ProblemSolutionStore) GetSolution(ctx context.Context, id string) (*ProblemSolution, error)
func (s *ProblemSolutionStore) ListByProblem(ctx context.Context, problemID string, includeDrafts bool) ([]ProblemSolution, error)
func (s *ProblemSolutionStore) UpdateSolution(ctx context.Context, id string, in UpdateSolutionInput) (*ProblemSolution, error)
func (s *ProblemSolutionStore) SetPublished(ctx context.Context, id string, published bool) (*ProblemSolution, error)
func (s *ProblemSolutionStore) DeleteSolution(ctx context.Context, id string) (*ProblemSolution, error)
```

Semantics: `ListByProblem(..., includeDrafts=false)` filters to `is_published = true`. `SetPublished` is a dedicated method so the handler can map invalid repeats to 409 if desired (or just idempotent update — pick idempotent here since there's no transition graph).

- [ ] **Step 2: Tests for `ProblemSolutionStore`**

Happy-path CRUD, draft vs published filter, cascade delete (delete parent problem → solutions gone).

```go
func TestSolutions_CascadeDeleteWithProblem(t *testing.T) {
    s := NewProblemSolutionStore(testDB); ps := NewProblemStore(testDB)
    p := mustCreateProblem(t, ps, "org", orgID, "published")
    sol, err := s.CreateSolution(context.Background(), CreateSolutionInput{
        ProblemID: p.ID, Language: "python", Code: "print('hi')", CreatedBy: userID,
    })
    require.NoError(t, err)
    _, err = ps.DeleteProblem(context.Background(), p.ID)
    require.NoError(t, err)
    got, _ := s.GetSolution(context.Background(), sol.ID)
    require.Nil(t, got)
}
```

- [ ] **Step 3: `TopicProblemStore`**

```go
type TopicProblemStore struct{ db *sql.DB }
func NewTopicProblemStore(db *sql.DB) *TopicProblemStore { return &TopicProblemStore{db: db} }

type TopicProblemAttachment struct {
    TopicID    string    `json:"topicId"`
    ProblemID  string    `json:"problemId"`
    SortOrder  int       `json:"sortOrder"`
    AttachedBy string    `json:"attachedBy"`
    AttachedAt time.Time `json:"attachedAt"`
}

// Attach: idempotent insert; returns ErrAlreadyAttached if (topic, problem) exists.
var ErrAlreadyAttached = errors.New("problem already attached to topic")

func (s *TopicProblemStore) Attach(ctx context.Context, topicID, problemID string, sortOrder int, attachedBy string) (*TopicProblemAttachment, error)
func (s *TopicProblemStore) Detach(ctx context.Context, topicID, problemID string) (bool, error)  // true if a row was removed
func (s *TopicProblemStore) SetSortOrder(ctx context.Context, topicID, problemID string, sortOrder int) (*TopicProblemAttachment, error)
func (s *TopicProblemStore) IsAttached(ctx context.Context, topicID, problemID string) (bool, error)
// ListTopicsByProblem for the reverse lookup used in attempt visibility checks.
func (s *TopicProblemStore) ListTopicsByProblem(ctx context.Context, problemID string) ([]string, error)
```

- [ ] **Step 4: Tests for `TopicProblemStore`**

```go
func TestTopicProblems_AttachDetachReorder(t *testing.T)
func TestTopicProblems_DuplicateAttachReturnsAlreadyAttached(t *testing.T)
func TestTopicProblems_CascadeOnTopicDelete(t *testing.T)
func TestTopicProblems_CascadeOnProblemDelete(t *testing.T)
```

- [ ] **Step 5: Run**

```bash
cd platform && go test ./internal/store/... -run "Solution|TopicProblem" -count=1
```

- [ ] **Step 6: Commit**

```bash
git add platform/internal/store/problem_solutions.go \
        platform/internal/store/problem_solutions_test.go \
        platform/internal/store/topic_problems.go \
        platform/internal/store/topic_problems_test.go
git commit -m "feat(028): stores for problem_solutions and topic_problems"
```

---

### Task 5: Expand `ProblemHandler`

**Files:**
- Modify: `platform/internal/store/attempts.go` — add `CountAttemptsByProblem`.
- Modify: `platform/internal/store/attempts_test.go` — test the new method.
- Modify: `platform/internal/handlers/problems.go`
- Modify: `platform/internal/handlers/problems_test.go`
- Modify: `platform/cmd/api/main.go` — pass `Solutions` and `TopicProblems` stores into the handler struct when wiring.
- Modify: `platform/internal/handlers/stores.go` — add `Solutions *store.ProblemSolutionStore` and `TopicProblems *store.TopicProblemStore`.

- [ ] **Step 0: Add `AttemptStore.CountAttemptsByProblem`**

```go
// platform/internal/store/attempts.go
func (s *AttemptStore) CountAttemptsByProblem(ctx context.Context, problemID string) (int, error) {
    var n int
    err := s.db.QueryRowContext(ctx,
        `SELECT COUNT(*) FROM attempts WHERE problem_id = $1`, problemID).Scan(&n)
    return n, err
}
```

One test: create 2 attempts, call Count → 2; delete problem via cascade → Count returns 0.

- [ ] **Step 1: Extend the handler struct**

```go
type ProblemHandler struct {
    Problems      *store.ProblemStore
    TestCases     *store.TestCaseStore
    Attempts      *store.AttemptStore
    Solutions     *store.ProblemSolutionStore    // new
    TopicProblems *store.TopicProblemStore       // new
    Topics        *store.TopicStore
    Courses       *store.CourseStore
    Orgs           *store.OrgStore                // for GetUserRolesInOrg / membership lookup
}
```

- [ ] **Step 2: Rework the route table**

```go
func (h *ProblemHandler) Routes(r chi.Router) {
    r.Route("/api/problems", func(r chi.Router) {
        r.Get("/", h.ListProblems)          // scope-aware filter
        r.Post("/", h.CreateProblem)
    })
    r.Route("/api/problems/{id}", func(r chi.Router) {
        r.Use(ValidateUUIDParam("id"))
        r.Get("/", h.GetProblem)
        r.Patch("/", h.UpdateProblem)
        r.Delete("/", h.DeleteProblem)
        r.Post("/publish",   h.PublishProblem)
        r.Post("/archive",   h.ArchiveProblem)
        r.Post("/unarchive", h.UnarchiveProblem)
        r.Post("/fork",      h.ForkProblem)

        // Test cases and attempts stay on the same handler for now.
        r.Get("/test-cases",  h.ListTestCases)
        r.Post("/test-cases", h.CreateTestCase)
        r.Get("/attempts",    h.ListAttempts)
        r.Post("/attempts",   h.CreateAttempt)
    })
    // Topic-scoped list kept for "what's in this topic?" queries used by
    // the student problem page and teacher dashboard.
    r.Route("/api/topics/{topicId}/problems", func(r chi.Router) {
        r.Use(ValidateUUIDParam("topicId"))
        r.Get("/", h.ListProblemsByTopic)
    })
}
```

`POST /api/topics/{topicId}/problems` (attach) lives in the **new** `TopicProblemHandler` (Task 7). Keep the read there too if you prefer — but consolidating GET with the existing chi.Route block is fine as long as handler methods don't leak into the attach/detach handler.

- [ ] **Step 3: `ListProblems` — scope filter + accessible default**

```go
func (h *ProblemHandler) ListProblems(w http.ResponseWriter, r *http.Request) {
    claims, _ := auth.ClaimsFromContext(r.Context())
    f := parseListFilterFromQuery(r)              // parses scope, difficulty, tags, q, cursor
    f.ViewerID = claims.UserID
    f.IsPlatformAdmin = claims.IsPlatformAdmin
    orgs, err := h.Orgs.ListOrgsForUser(r.Context(), claims.UserID)
    if err != nil { internalErr(w, err); return }
    f.ViewerOrgs = ids(orgs) // helper that maps []OrgMembership -> []string of OrgIDs

    list, err := h.Problems.ListProblems(r.Context(), f)
    if err != nil { internalErr(w, err); return }
    writeJSON(w, http.StatusOK, list)
}
```

- [ ] **Step 4: `CreateProblem` — role check by scope**

```go
func (h *ProblemHandler) CreateProblem(w http.ResponseWriter, r *http.Request) {
    claims, _ := auth.ClaimsFromContext(r.Context())
    var body struct {
        Scope         string             `json:"scope"`
        ScopeID       *string            `json:"scopeId"`
        Title         string             `json:"title"`
        Description   string             `json:"description"`
        StarterCode   map[string]string  `json:"starterCode"`
        Difficulty    string             `json:"difficulty"`
        GradeLevel    *string            `json:"gradeLevel"`
        Tags          []string           `json:"tags"`
        TimeLimitMs   *int               `json:"timeLimitMs"`
        MemoryLimitMb *int               `json:"memoryLimitMb"`
        Slug          *string            `json:"slug"`
    }
    if err := decodeJSON(r, &body); err != nil { badReq(w, err); return }

    if !h.authorizedForScope(r.Context(), claims, body.Scope, body.ScopeID) {
        http.Error(w, "forbidden", http.StatusForbidden); return
    }
    // ... validate enums, title length, tag length ...

    p, err := h.Problems.CreateProblem(r.Context(), store.CreateProblemInput{
        Scope: body.Scope, ScopeID: body.ScopeID, Title: body.Title,
        Description: body.Description, StarterCode: body.StarterCode,
        Difficulty: body.Difficulty, GradeLevel: body.GradeLevel,
        Tags: body.Tags, Status: "draft",
        TimeLimitMs: body.TimeLimitMs, MemoryLimitMb: body.MemoryLimitMb,
        Slug: body.Slug, CreatedBy: claims.UserID,
    })
    if err != nil { internalErr(w, err); return }
    writeJSON(w, http.StatusCreated, p)
}
```

`authorizedForScope(ctx, claims, scope, scopeID)`:

```go
func (h *ProblemHandler) authorizedForScope(ctx context.Context, c *auth.Claims, scope string, scopeID *string) bool {
    switch scope {
    case "platform":
        return c.IsPlatformAdmin
    case "org":
        if scopeID == nil { return false }
        roles, err := h.Orgs.GetUserRolesInOrg(ctx, *scopeID, c.UserID)
        if err != nil || len(roles) == 0 { return false }
        for _, m := range roles {
            if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
                return true
            }
        }
        return false
    case "personal":
        return scopeID != nil && *scopeID == c.UserID
    default:
        return false
    }
}
```

- [ ] **Step 5: Lifecycle handlers**

```go
func (h *ProblemHandler) PublishProblem(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id"); claims, _ := auth.ClaimsFromContext(r.Context())
    if !h.authorizedForProblemEdit(r.Context(), claims, id) { http.Error(w, "forbidden", 403); return }
    p, err := h.Problems.SetStatus(r.Context(), id, "published")
    switch {
    case errors.Is(err, store.ErrInvalidTransition): http.Error(w, "already published", 409)
    case err != nil: internalErr(w, err)
    default: writeJSON(w, 200, p)
    }
}
// ArchiveProblem / UnarchiveProblem identical shape — different target status.
```

`authorizedForProblemEdit`:

```go
func (h *ProblemHandler) authorizedForProblemEdit(ctx context.Context, c *auth.Claims, problemID string) bool {
    p, err := h.Problems.GetProblem(ctx, problemID)
    if err != nil || p == nil { return false }
    return h.authorizedForScope(ctx, c, p.Scope, p.ScopeID) ||
           (p.Scope == "personal" && p.CreatedBy == c.UserID)
}
```

- [ ] **Step 6: `ForkProblem`**

```go
func (h *ProblemHandler) ForkProblem(w http.ResponseWriter, r *http.Request) {
    sourceID := chi.URLParam(r, "id"); claims, _ := auth.ClaimsFromContext(r.Context())
    var body struct {
        TargetScope   string  `json:"targetScope"`
        TargetScopeID *string `json:"targetScopeId"`
        Title         *string `json:"title"`
    }
    if err := decodeJSON(r, &body); err != nil { badReq(w, err); return }

    // Default: org (exactly one org) else personal.
    if body.TargetScope == "" {
        orgs, _ := h.Orgs.ListOrgsForUser(r.Context(), claims.UserID)
        if len(orgs) == 1 {
            body.TargetScope = "org"; id := orgs[0].OrgID; body.TargetScopeID = &id
        } else {
            body.TargetScope = "personal"; uid := claims.UserID; body.TargetScopeID = &uid
        }
    }
    // View-source check.
    if !h.canViewProblem(r.Context(), claims, sourceID) { http.Error(w, "not found", 404); return }
    // Authorize target-scope create.
    if !h.authorizedForScope(r.Context(), claims, body.TargetScope, body.TargetScopeID) {
        http.Error(w, "forbidden", 403); return
    }
    p, err := h.Problems.ForkProblem(r.Context(), sourceID, store.ForkTarget{
        Scope: body.TargetScope, ScopeID: body.TargetScopeID,
        Title: body.Title, CallerID: claims.UserID,
    })
    if err != nil { internalErr(w, err); return }
    writeJSON(w, 201, p)
}
```

- [ ] **Step 7: `DeleteProblem` — guard with 409**

```go
func (h *ProblemHandler) DeleteProblem(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id"); claims, _ := auth.ClaimsFromContext(r.Context())
    if !h.authorizedForProblemEdit(r.Context(), claims, id) { http.Error(w, "forbidden", 403); return }
    // 409 if attached to any topic OR any attempts exist.
    topics, _ := h.TopicProblems.ListTopicsByProblem(r.Context(), id)
    if len(topics) > 0 { http.Error(w, "problem is attached to topics", 409); return }
    // Uses AttemptStore.CountAttemptsByProblem added in Step 0 below.
    n, _ := h.Attempts.CountAttemptsByProblem(r.Context(), id)
    if n > 0 { http.Error(w, "problem has attempts", 409); return }
    _, err := h.Problems.DeleteProblem(r.Context(), id)
    if err != nil { internalErr(w, err); return }
    w.WriteHeader(204)
}
```

(`AttemptStore.ListAttemptsByProblem` exists in plan 024 with a `userID` arg; pass `""` for any-user, or add `CountAttemptsByProblem` if that existing method requires non-empty user — check the prior signature before writing.)

- [ ] **Step 8: Update `canViewProblem`**

```go
// Spec 009 §Access policy — scope grant OR attachment grant OR draft carve-out OR platform admin.
func (h *ProblemHandler) canViewProblem(ctx context.Context, c *auth.Claims, id string) bool {
    if c.IsPlatformAdmin { return true }
    p, err := h.Problems.GetProblem(ctx, id)
    if err != nil || p == nil { return false }

    // Draft carve-out: only editors + platform admins.
    if p.Status == "draft" {
        return h.authorizedForProblemEdit(ctx, c, id)
    }
    // Scope grant.
    switch p.Scope {
    case "platform":
        return p.Status == "published" || p.Status == "archived"
    case "org":
        if p.ScopeID == nil { return false }
        roles, _ := h.Orgs.GetUserRolesInOrg(ctx, *p.ScopeID, c.UserID)
        for _, m := range roles {
            if m.Status == "active" { return p.Status != "draft" }
        }
    case "personal":
        if p.ScopeID != nil && *p.ScopeID == c.UserID { return true }
    }
    // Attachment grant.
    topics, _ := h.TopicProblems.ListTopicsByProblem(ctx, id)
    for _, topicID := range topics {
        ok, _, _ := h.canViewTopic(&http.Request{}, topicID, c)
        if ok { return true }
    }
    return false
}
```

(Refactor `canViewTopic` to accept `ctx` instead of `*http.Request` if the current signature forces you to fake a request — keep handler functions from doing that. Check the current signature first and adjust the helper cleanly.)

- [ ] **Step 9: Tests**

Handler test file expansion. The matrix to cover (one Go subtest per cell; use `t.Run` blocks):

```
scope       status       viewer                   expected_http
platform    published    anyone                   200
platform    draft        non-admin                404
platform    draft        platform_admin           200
org         published    same-org member          200
org         published    other-org member         404
org         draft        same-org teacher         200
org         draft        same-org student         404
personal    any          owner                    200
personal    any          non-owner non-admin      404
attached    any          class member             200  (attachment grant)
```

Plus:
- `POST /api/problems` with each scope + correct/incorrect role returns 201/403.
- `POST /api/problems/{id}/publish` from draft → 200; repeat → 409.
- `POST /api/problems/{id}/fork` from caller in one org → default target=org; caller in zero → target=personal; caller can view source (404 if not).
- `DELETE /api/problems/{id}` with attempts → 409; without → 204.

- [ ] **Step 10: Run**

```bash
cd platform && go test ./internal/handlers/... -run Problem -count=1
```

- [ ] **Step 11: Commit**

```bash
git add platform/internal/store/stores.go \
        platform/internal/handlers/problems.go \
        platform/internal/handlers/problems_test.go \
        platform/internal/handlers/stores.go \
        platform/cmd/api/main.go
git commit -m "feat(028): ProblemHandler — scope, fork, lifecycle, strict delete"
```

---

### Task 6: `SolutionHandler`

**Files:**
- Create: `platform/internal/handlers/problem_solutions.go`
- Create: `platform/internal/handlers/problem_solutions_test.go`
- Modify: `platform/cmd/api/main.go` — register the handler.

- [ ] **Step 1: Handler + routes**

```go
type SolutionHandler struct {
    Problems       *store.ProblemStore
    Solutions      *store.ProblemSolutionStore
    Orgs           *store.OrgStore
    // Re-uses the access helpers; accept a pointer to ProblemHandler's view helper,
    // OR duplicate the small helper here — pick duplicate to avoid circular coupling.
}

func (h *SolutionHandler) Routes(r chi.Router) {
    r.Route("/api/problems/{id}/solutions", func(r chi.Router) {
        r.Use(ValidateUUIDParam("id"))
        r.Get("/",  h.ListSolutions)
        r.Post("/", h.CreateSolution)
    })
    r.Route("/api/problems/{id}/solutions/{solutionId}", func(r chi.Router) {
        r.Use(ValidateUUIDParam("id"), ValidateUUIDParam("solutionId"))
        r.Patch("/",       h.UpdateSolution)
        r.Delete("/",      h.DeleteSolution)
        r.Post("/publish",   h.PublishSolution)
        r.Post("/unpublish", h.UnpublishSolution)
    })
}
```

- [ ] **Step 2: Visibility rule on `ListSolutions`**

```go
func (h *SolutionHandler) ListSolutions(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    claims, _ := auth.ClaimsFromContext(r.Context())
    if !h.canViewProblem(r.Context(), claims, id) { http.Error(w, "not found", 404); return }
    includeDrafts := h.canEditProblem(r.Context(), claims, id)
    list, err := h.Solutions.ListByProblem(r.Context(), id, includeDrafts)
    if err != nil { internalErr(w, err); return }
    writeJSON(w, 200, list)
}
```

- [ ] **Step 3: Create / Update / Delete / Publish**

All require `canEditProblem(problem)` (i.e., an editor of the owning problem's scope). `Publish` calls `SetPublished(id, true)`; `Unpublish` is `SetPublished(id, false)`.

- [ ] **Step 4: Tests**

- Student viewing a problem sees `is_published=true` solutions only (drafts omitted).
- Same-org teacher viewing the same problem sees drafts + published.
- Create / Patch / Publish flipping `is_published` true/false; Unpublish returns 200 after first call and 200 again (idempotent, no 409).
- Delete cascade confirmed in store tests (Task 4); here just 204.

- [ ] **Step 5: Run + Commit**

```bash
cd platform && go test ./internal/handlers/... -run Solution -count=1
git add platform/internal/handlers/problem_solutions.go \
        platform/internal/handlers/problem_solutions_test.go \
        platform/cmd/api/main.go
git commit -m "feat(028): SolutionHandler — nested CRUD + publish/unpublish"
```

---

### Task 7: Topic attachment handler + TestCase visibility refinement

**Files:**
- Create: `platform/internal/handlers/problem_access.go` — shared `canViewProblem` + `authorizedForProblemEdit` + `authorizedForScope` helpers (extracted from Task 5 so `TopicProblemHandler` can reuse them without cross-handler coupling).
- Create: `platform/internal/handlers/topic_problems.go`
- Create: `platform/internal/handlers/topic_problems_test.go`
- Modify: `platform/internal/handlers/problems.go` — replace the private methods with calls to the shared helpers in `problem_access.go`. Also update `ListTestCases` — shell out hidden canonical rows when caller is not an editor.
- Modify: `platform/cmd/api/main.go` — register `TopicProblemHandler`.

- [ ] **Step 0: Extract shared access helpers to `problem_access.go`**

Move the three helpers out of `ProblemHandler` into package-level functions that take explicit store dependencies:

```go
// platform/internal/handlers/problem_access.go
package handlers

import (
    "context"

    "github.com/weiboz0/bridge/platform/internal/auth"
    "github.com/weiboz0/bridge/platform/internal/store"
)

type problemAccessDeps struct {
    Problems       *store.ProblemStore
    TopicProblems  *store.TopicProblemStore
    Topics         *store.TopicStore
    Courses        *store.CourseStore
    Orgs           *store.OrgStore
    ClassMembers   *store.ClassMembershipStore
}

func canViewProblem(ctx context.Context, d problemAccessDeps, c *auth.Claims, id string) bool { /* … */ }
func canViewProblemRow(ctx context.Context, d problemAccessDeps, c *auth.Claims, p *store.Problem) bool { /* same logic but skips the initial fetch */ }
func authorizedForScope(ctx context.Context, d problemAccessDeps, c *auth.Claims, scope string, scopeID *string) bool { /* … */ }
func authorizedForProblemEdit(ctx context.Context, d problemAccessDeps, c *auth.Claims, p *store.Problem) bool { /* … */ }
```

`ProblemHandler` and `TopicProblemHandler` each construct a `problemAccessDeps` from their own fields and call the shared helpers. No reciprocal handler-to-handler pointer.

- [ ] **Step 1: `TopicProblemHandler` + routes**

```go
type TopicProblemHandler struct {
    Problems       *store.ProblemStore
    TopicProblems  *store.TopicProblemStore
    Topics         *store.TopicStore
    Courses        *store.CourseStore
    Orgs           *store.OrgStore
    ClassMembers   *store.ClassMembershipStore
}

func (h *TopicProblemHandler) accessDeps() problemAccessDeps {
    return problemAccessDeps{
        Problems: h.Problems, TopicProblems: h.TopicProblems,
        Topics: h.Topics, Courses: h.Courses, Orgs: h.Orgs, ClassMembers: h.ClassMembers,
    }
}

func (h *TopicProblemHandler) Routes(r chi.Router) {
    r.Route("/api/topics/{topicId}/problems", func(r chi.Router) {
        r.Use(ValidateUUIDParam("topicId"))
        r.Post("/", h.AttachProblem)
    })
    r.Route("/api/topics/{topicId}/problems/{problemId}", func(r chi.Router) {
        r.Use(ValidateUUIDParam("topicId"), ValidateUUIDParam("problemId"))
        r.Delete("/", h.DetachProblem)
        r.Patch("/",  h.ReorderProblem)
    })
}
```

GET `/api/topics/{topicId}/problems` lives on `ProblemHandler.ListProblemsByTopic` (Task 5 route) — reuse to avoid a duplicate handler.

- [ ] **Step 2: `AttachProblem`**

```go
func (h *TopicProblemHandler) AttachProblem(w http.ResponseWriter, r *http.Request) {
    topicID := chi.URLParam(r, "topicId")
    claims, _ := auth.ClaimsFromContext(r.Context())

    // 1. caller must be teacher/admin in the target course's org.
    topic, err := h.Topics.GetTopic(r.Context(), topicID)
    if err != nil || topic == nil { http.Error(w, "not found", 404); return }
    course, _ := h.Courses.GetCourse(r.Context(), topic.CourseID)
    if course == nil { http.Error(w, "not found", 404); return }
    roles, _ := h.Orgs.GetUserRolesInOrg(r.Context(), course.OrgID, claims.UserID)
    authorized := claims.IsPlatformAdmin
    for _, m := range roles {
        if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
            authorized = true; break
        }
    }
    if !authorized { http.Error(w, "forbidden", 403); return }

    var body struct {
        ProblemID string `json:"problemId"`
        SortOrder *int   `json:"sortOrder"`
    }
    if err := decodeJSON(r, &body); err != nil { badReq(w, err); return }

    // 2. caller must be able to view the problem, and problem must be published.
    p, err := h.Problems.GetProblem(r.Context(), body.ProblemID)
    if err != nil || p == nil { http.Error(w, "not found", 404); return }
    if p.Status != "published" { http.Error(w, "problem is not published", 409); return }
    if !canViewProblemRow(r.Context(), h.accessDeps(), claims, p) { http.Error(w, "not found", 404); return }

    order := 0
    if body.SortOrder != nil { order = *body.SortOrder }
    att, err := h.TopicProblems.Attach(r.Context(), topicID, body.ProblemID, order, claims.UserID)
    switch {
    case errors.Is(err, store.ErrAlreadyAttached): http.Error(w, "already attached", 409)
    case err != nil: internalErr(w, err)
    default: writeJSON(w, 201, att)
    }
}
```

- [ ] **Step 3: `DetachProblem` + `ReorderProblem`**

`Detach` only needs the teacher/admin-in-org check. `Reorder` also.

- [ ] **Step 4: `ListTestCases` visibility refinement**

In `platform/internal/handlers/problems.go`, find `ListTestCases`. Today it returns the full canonical + private test cases the caller has access to. Split into editor vs viewer path:

```go
func (h *ProblemHandler) ListTestCases(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    claims, _ := auth.ClaimsFromContext(r.Context())
    if !h.canViewProblem(r.Context(), claims, id) { http.Error(w, "not found", 404); return }
    isEditor := h.authorizedForProblemEdit(r.Context(), claims, id)

    list, err := h.TestCases.ListTestCases(r.Context(), id, claims.UserID)
    if err != nil { internalErr(w, err); return }

    if !isEditor {
        for i := range list {
            if list[i].OwnerID == nil && !list[i].IsExample {
                // hidden canonical case — return shell only
                list[i].Stdin = ""; list[i].ExpectedStdout = nil
            }
        }
    }
    writeJSON(w, 200, list)
}
```

Confirm `TestCase.Stdin` / `ExpectedStdout` shape with the current store struct.

- [ ] **Step 5: Tests**

```
- Student attaching a problem → 403.
- Teacher attaching a draft problem → 409.
- Teacher attaching a platform published problem to their org's topic → 201; appears in GET.
- Duplicate attach → 409.
- Detach + re-attach works.
- Student listing test_cases: `is_example=false` canonical cases come back with Stdin="" and ExpectedStdout=nil.
- Teacher (editor) sees full bodies for hidden cases.
```

- [ ] **Step 6: Run + Commit**

```bash
cd platform && go test ./internal/handlers/... -run "Attach|Detach|TestCase" -count=1
git add platform/internal/handlers/topic_problems.go \
        platform/internal/handlers/topic_problems_test.go \
        platform/internal/handlers/problems.go \
        platform/cmd/api/main.go
git commit -m "feat(028): topic attach/detach + hidden test-case shells for non-editors"
```

---

### Task 8: Seed rewrite (`scripts/seed_python_101.sql`)

**Files:**
- Modify: `scripts/seed_python_101.sql`

- [ ] **Step 1: Rewrite the problems + test_cases blocks**

Current state: each `INSERT INTO problems (...)` writes `topic_id, language, starter_code, "order"` directly. New shape:

```sql
-- problems: scope='org', scope_id=<org>, no topic_id, starter_code is jsonb.
INSERT INTO problems (id, scope, scope_id, title, description, starter_code,
                      difficulty, grade_level, tags, status, created_by)
VALUES
  ('00000000-0000-0000-0000-0000000aa001',
   'org', '<org_id>',
   'Sum of Two Numbers',
   'Read two integers, print their sum.',
   jsonb_build_object('python', 'a = int(input())\nb = int(input())\nprint(...)\n'),
   'easy', '6-8', ARRAY['io','arithmetic'], 'published',
   '<teacher_user_id>'),
  ...;

-- attach each problem to its topic.
INSERT INTO topic_problems (topic_id, problem_id, sort_order, attached_by)
VALUES
  ('<topic_io>',        '00000000-...-0aa001', 0, '<teacher_user_id>'),
  ...;

-- one sample canonical reference solution per problem, is_published=true so
-- students can see it after attempting. (Optional but demonstrates the
-- problem_solutions surface.)
INSERT INTO problem_solutions (problem_id, language, title, code, is_published, created_by)
VALUES
  ('00000000-...-0aa001', 'python', 'Direct',
   'a = int(input())\nb = int(input())\nprint(a + b)\n', true, '<teacher_user_id>'),
  ...;
```

Preserve all existing UUIDs so links the user has bookmarked still resolve. The topic-attachment rows take on `sort_order = <what used to be problems."order">`.

- [ ] **Step 2: Apply and smoke-check**

```bash
psql postgresql://work@127.0.0.1:5432/bridge -f scripts/seed_python_101.sql
psql postgresql://work@127.0.0.1:5432/bridge -c \
  "SELECT p.title, tp.sort_order FROM problems p
    JOIN topic_problems tp ON tp.problem_id = p.id
    WHERE p.scope = 'org' ORDER BY tp.sort_order;"
```

Expected: every Python 101 problem appears with its `sort_order`.

- [ ] **Step 3: Re-run twice — ensure the seed is idempotent**

The existing seed uses `ON CONFLICT (id) DO NOTHING`; preserve that for the new tables too. `INSERT ... ON CONFLICT DO NOTHING` on `topic_problems` and `problem_solutions`.

- [ ] **Step 4: Boot the stack + manual click-through**

```bash
# terminal 1
PORT=3003 bun run dev
# terminal 2
cd platform && air
# terminal 3
bun run hocuspocus
```

Open as a student (see CLAUDE.md for the E2E account list), navigate to a Python 101 class → topic → problem. Expected: problem loads, starter code appears (from `starter_code['python']`), sample test cases render, a submission runs end-to-end.

- [ ] **Step 5: Commit**

```bash
git add scripts/seed_python_101.sql
git commit -m "feat(028): reshape Python 101 seed for problem bank (scope + topic_problems + solutions)"
```

---

### Task 9: Consumer read-path fixes

**Files:**
- Modify: `src/app/(portal)/student/classes/[id]/problems/[problemId]/page.tsx`
- Modify: `src/app/(portal)/teacher/classes/[id]/students/[studentId]/problems/[problemId]/page.tsx`
- Modify: any component that calls `problem.language` or `problem.starterCode` (grep the tree).

- [ ] **Step 1: Grep the fanout**

```bash
grep -rn "\.language\b\|problem\.starterCode\b\|problem\.topicId\b" \
  src/app src/components src/lib
```

Every hit is a consumer that breaks when the server-side shape flips.

- [ ] **Step 2: Student problem page**

The page already loads `classSettings` (via `getClassSettings` from plan 027 rename). Read the editor mode:

```ts
const editorMode = (classSettings?.editorMode ?? "python") as "python" | "javascript" | "blockly";
```

Replace `problem.language` / `problem.starterCode` references:

```ts
// before:
//   const language = problem.language
//   const starter = problem.starterCode ?? ""
// after:
const language = editorMode;
const starter = (problem.starterCode as Record<string, string> | undefined)?.[editorMode] ?? "";
```

Pass `language` / `starter` through to the Monaco editor and Pyodide runner as today.

- [ ] **Step 3: Teacher watcher page**

Same treatment. Teacher watcher shows the student's active code via Yjs — the starter snapshot is only used as an empty-state placeholder. Use the same derivation.

- [ ] **Step 4: Type-check**

```bash
node_modules/.bin/tsc --noEmit 2>&1 | grep -E "starterCode|language" | head -20
```

Expected: none of our files flagged. If a pre-existing error unrelated to this change surfaces, leave it and note it in the post-execution report.

- [ ] **Step 5: Boot the stack + repeat the click-through from Task 8**

Confirm problem renders, Monaco loads with the Python starter, test runner works.

- [ ] **Step 6: Commit**

```bash
git add "src/app/(portal)/student/classes/[id]/problems/[problemId]/page.tsx" \
        "src/app/(portal)/teacher/classes/[id]/students/[studentId]/problems/[problemId]/page.tsx"
# plus any other modified consumers the grep surfaced.
git commit -m "feat(028): student + teacher pages read language from class_settings, starter from JSONB"
```

---

### Task 10: Verify, docs, post-execution report

**Files:**
- Modify: `docs/api.md` — add sections for problems, solutions, topic-problems.
- Modify: `TODO.md` — remove "Problem bank" item; add anything surfaced.
- Modify: `docs/plans/028-problem-bank.md` — post-execution report appended.

- [ ] **Step 1: Full Go suite**

```bash
cd platform && go test ./... -count=1 -timeout 180s
```

Must be green. If any test from outside this plan regresses, fix it — the reshape is expected to affect plan 024 / plan 026 tests that reference the old schema (plan 024's `ListProblemsByTopic` is replaced; plan 026 runner tests pass through `attempts` + `test_cases` which are unchanged).

- [ ] **Step 2: Full Vitest suite**

```bash
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test node_modules/.bin/vitest run
```

Must be green.

- [ ] **Step 3: Type-check**

```bash
node_modules/.bin/tsc --noEmit
```

Should be clean for our changes; pre-existing errors (if any) noted in the post-execution report.

- [ ] **Step 4: E2E regression (optional but recommended)**

```bash
bun run test:e2e
```

Plan 025 / 026 E2E flows hit the student problem page — they should still pass. If they regress due to the read-path changes in Task 9, fix before shipping.

- [ ] **Step 5: Update `docs/api.md`**

Add a "Problems" section describing the shape of:
- `GET /api/problems` with `?scope=`, `?difficulty=`, `?gradeLevel=`, `?tags=`, `?q=`, pagination.
- `GET /api/problems/{id}` response shape (include `scope`, `scopeId`, `starterCode` JSONB map, `status`, `forkedFrom`).
- Lifecycle endpoints (`/publish`, `/archive`, `/unarchive`).
- `POST /api/problems/{id}/fork` body + response.
- Solutions nested under `/api/problems/{id}/solutions`.
- Topic attachment under `/api/topics/{topicId}/problems`.

Keep it concise — a reader who has read spec 009 should only need the exact endpoint URIs + request/response bodies here.

- [ ] **Step 6: Update `TODO.md`**

Remove / tick off the "Problem bank" entry. Add any follow-up items surfaced during implementation (e.g., "revisit curated tag taxonomy once production tag usage is known", "wire problem-bank browse UI in spec 011").

- [ ] **Step 7: Append post-execution report to this plan**

Append a `## Post-Execution Report` section with:
- Files created / modified (counts).
- Migration notes — any deviation from the 0013 draft?
- Test coverage delta — new Go tests added, new Vitest assertions.
- Known limitations — e.g., tag taxonomy is free-form; `status` is a varchar CHECK rather than a true PG enum.
- Follow-ups for TODO.md.
- Verification output (the final `go test ./...` and `vitest run` tail).

- [ ] **Step 8: Final commit + push + PR**

```bash
git add docs/api.md TODO.md docs/plans/028-problem-bank.md
git commit -m "docs(028): API reference, TODO update, post-execution report"
GH_TOKEN=$(cat .gh-token) gh pr create \
  --title "feat: problem bank — scopes + topic attachment + solutions (plan 028 / spec 009)" \
  --body-file docs/plans/028-problem-bank.md
```

---

## Code Review

Reviewers append findings here following `docs/code-review.md` format. Author responds inline with `→ Response:` and status `[FIXED]` / `[WONTFIX]`.

### Review 1

- **Date**: 2026-04-21
- **Reviewer**: Codex
- **PR**: #48 — feat: problem bank — scopes + topic attachment + solutions (plan 028 / spec 009)
- **Verdict**: Approved

**Must Fix**

1. `[FIXED]` `GET /api/problems` did not implement the attachment-grant branch of the spec, so a problem attached to a viewer's class topic was reachable via detail reads but still absent from browse/search. The missing visibility logic lived in [platform/internal/store/problems.go:243](/home/chris/workshop/Bridge/platform/internal/store/problems.go:243) while the attachment-grant behavior was only enforced in row-level handlers.
   → Response: Reworked `ProblemStore.ListProblems` to include an attachment-grant `EXISTS` clause keyed by `topic_problems -> topics -> classes -> class_memberships`, and added regressions in [platform/internal/store/problems_test.go:403](/home/chris/workshop/Bridge/platform/internal/store/problems_test.go:403) and [platform/internal/handlers/problems_integration_test.go:568](/home/chris/workshop/Bridge/platform/internal/handlers/problems_integration_test.go:568).

2. `[FIXED]` Browse/search still surfaced archived rows by default for owners/authors, which violated the archive contract ("viewable by direct link, hidden from browse/search"). The default-accessibility branch in [platform/internal/store/problems.go:243](/home/chris/workshop/Bridge/platform/internal/store/problems.go:243) treated personal/authored rows as always list-visible.
   → Response: Tightened the default accessibility gate so archived rows stay out of the implicit browse/search set unless `status` is requested explicitly, with coverage in [platform/internal/store/problems_test.go:461](/home/chris/workshop/Bridge/platform/internal/store/problems_test.go:461) and [platform/internal/handlers/problems_integration_test.go:590](/home/chris/workshop/Bridge/platform/internal/handlers/problems_integration_test.go:590).

3. `[FIXED]` Cursor pagination was only half implemented: the endpoint accepted `cursor` but returned a bare array, so clients had no legal way to request page 2. The dead-end contract was in [platform/internal/handlers/problems.go:119](/home/chris/workshop/Bridge/platform/internal/handlers/problems.go:119) and [platform/internal/handlers/problems.go:196](/home/chris/workshop/Bridge/platform/internal/handlers/problems.go:196).
   → Response: Changed `GET /api/problems` to return `{ items, nextCursor }`, added overfetch-based `hasMore` support in the store, and covered the round-trip in [platform/internal/store/problems_test.go:491](/home/chris/workshop/Bridge/platform/internal/store/problems_test.go:491) and [platform/internal/handlers/problems_integration_test.go:608](/home/chris/workshop/Bridge/platform/internal/handlers/problems_integration_test.go:608).

The fixes are isolated to the list path: detail reads, topic listing, and nested resources were left unchanged. Focused Go tests for `internal/store` and `internal/handlers` pass after the patch. DB-backed integration assertions remain in place for environments where `DATABASE_URL` is set.
