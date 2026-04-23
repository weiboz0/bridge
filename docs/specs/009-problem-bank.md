# 009 — Problem Bank

## Problem

Today `problems.topic_id` is a mandatory FK — every problem lives inside exactly one topic, which is inside exactly one course, which is inside exactly one org. This kills three things we want:

- **Reuse.** A platform-curated "Two Sum" should appear in Python 101, AP CS A, and a coding-club course without three copies. Today it needs three.
- **Cross-class problem discovery.** Teachers can't browse a bank of problems independent of a specific topic; there's no notion of a bank at all.
- **Orphan-session problem attachment.** Spec 010 allows sessions without a class, but there's no answer for "which problems can this session reference?" because problems today require a class-reachable topic.

There is also no "reference solution" concept — teachers can't author canonical solutions alongside a problem, which is standard for any problem-bank tool (LeetCode solutions tab, Codeforces editorial).

This spec introduces a first-class **problem bank**: problems become standalone, scoped entities with many-to-many topic attachment, and gain a `problem_solutions` child for reference implementations.

## Design

### Ownership scopes

Each problem has exactly one ownership scope, expressed as `(scope, scope_id)`:

| `scope` | `scope_id` | Meaning |
|---|---|---|
| `platform` | `NULL` | Curated by Bridge team. Edits restricted to platform admins. Published versions visible to everyone. |
| `org` | `orgs.id` | Owned by an org. Any org_admin or teacher in that org can edit. Published versions visible to all org members. |
| `personal` | `users.id` | Owned by a user. Only the owner can edit or view directly. |

Ownership is distinct from **attachment**: a problem attached to a topic becomes visible to that topic's class members regardless of its ownership scope. Ownership controls editing and default read visibility; attachment controls classroom exposure.

Ownership is a one-time decision per problem. Changing scope means forking (copy) or a privileged admin "move" action — not in this spec's scope.

### Sharing model — link, not copy

Attaching a platform problem to a topic is a **reference**, not a copy. The `topic_problems` join stores only `(topic_id, problem_id, sort_order)`. One canonical "Two Sum" row is shared across all its attachments. Updates to the canonical problem are seen immediately in every class using it.

If a teacher needs isolation (e.g., wants to modify a platform problem), they **fork explicitly**: `POST /api/problems/{id}/fork` creates a new row in their org's scope with `forked_from = source.id` for attribution. After fork, the two are independent — updates to either don't propagate.

Trade-off accepted: "platform updated the problem mid-term" is possible. In practice platform-owned problems should be stable; the explicit fork is the escape valve. Problem revisions / "new revision available — upgrade?" is explicitly out of scope (see §Follow-ups).

### Data Model

#### `problems` (modified)

```
problems
├── id                uuid PK
├── scope             enum {platform, org, personal}  NOT NULL
├── scope_id          uuid NULL
│                       NULL      when scope='platform'
│                       = orgs.id when scope='org'
│                       = users.id when scope='personal'
├── title             varchar(255) NOT NULL
├── slug              varchar(255) NULL            -- unique within scope; reserved for shareable URLs
├── description       text NOT NULL                 -- markdown; language-agnostic statement
├── starter_code      jsonb NOT NULL DEFAULT '{}'::jsonb
│                       -- {"python": "...", "javascript": "...", "blockly": "..."}
├── difficulty        enum {easy, medium, hard} NOT NULL DEFAULT 'easy'
├── grade_level       enum {K-5, 6-8, 9-12} NULL      -- reuses existing grade_level enum
├── tags              text[] NOT NULL DEFAULT '{}'    -- free-form sub-area tags
├── status            enum {draft, published, archived} NOT NULL DEFAULT 'draft'
├── forked_from       uuid NULL → problems(id)        -- attribution only; no propagation
├── time_limit_ms     int NULL                        -- reserved for CP direction
├── memory_limit_mb   int NULL                        -- reserved for CP direction
├── created_by        uuid NOT NULL → users(id)
├── created_at        timestamptz NOT NULL DEFAULT now()
├── updated_at        timestamptz NOT NULL DEFAULT now()
│
├── DROP: topic_id, language, "order"
│
├── INDEX (scope, scope_id, status)
├── INDEX (created_by)
├── UNIQUE INDEX (scope, scope_id, slug) WHERE slug IS NOT NULL
├── GIN INDEX (tags)
└── CHECK (
      (scope='platform' AND scope_id IS NULL) OR
      (scope='org'      AND scope_id IS NOT NULL) OR
      (scope='personal' AND scope_id IS NOT NULL)
    )
```

Problems become **language-agnostic**. The statement is prose; starter code is a per-language map; language choice is an attempt-level concern (attempts already carry `language`).

#### `topic_problems` (new — many-to-many)

```
topic_problems
├── topic_id          uuid → topics(id)   ON DELETE CASCADE
├── problem_id        uuid → problems(id) ON DELETE CASCADE
├── sort_order        int NOT NULL DEFAULT 0
├── attached_by       uuid NOT NULL → users(id)
├── attached_at       timestamptz NOT NULL DEFAULT now()
├── PRIMARY KEY (topic_id, problem_id)
└── INDEX (problem_id)                   -- reverse lookup
```

The same problem can appear in many topics. Each topic maintains its own ordering. Deleting a topic detaches; deleting a problem detaches from all topics.

#### `problem_solutions` (new — reference solutions)

```
problem_solutions
├── id                uuid PK
├── problem_id        uuid NOT NULL → problems(id) ON DELETE CASCADE
├── language          varchar NOT NULL             -- python / javascript / blockly
├── title             varchar(120) NULL            -- "Brute force O(n²)", "Optimal O(n)"
├── code              text NOT NULL
├── notes             text NULL                    -- markdown explanation of the approach
├── approach_tags     text[] NOT NULL DEFAULT '{}' -- e.g. ['brute_force', 'hash_map', 'dp']
├── is_published      bool NOT NULL DEFAULT false  -- draft vs. visible to students
├── created_by        uuid NOT NULL → users(id)
├── created_at / updated_at
└── INDEX (problem_id, language)
```

Multiple solutions per problem per language are allowed and expected — one `"Brute force"`, one `"Optimal"` is the canonical shape. `is_published=false` is the "draft / reveal later" state; teachers flip to `true` after a class session.

#### `test_cases` — unchanged

Schema is unchanged from plan 024. Visibility rules (see §Access policy) refine how hidden canonical cases are served to non-editors. Owner semantics hold: `owner_id IS NULL` = canonical (inherits problem's editor set), `owner_id = user_id` = private practice case.

#### `attempts` — unchanged

Remains per-(user, problem). No `class_id`. The teacher attempt-viewability rule below derives class context via the student's class memberships and the problem's topic attachments — no schema change needed.

### Access policy

#### Problem view

User `U` can view problem `P` iff any of:

1. **Scope grants read.**
   - `P.scope='platform'` AND `P.status='published'` → any authenticated user.
   - `P.scope='org'` AND `P.status='published'` AND `U` is a member of `P.scope_id` → any role.
   - `P.scope='personal'` AND `P.scope_id = U.id`.
2. **Attachment grants read.** `P` is attached via `topic_problems` to a topic in a course `C`, and `U` is a member of some class `K` with `K.course_id = C.id`. Works regardless of `P.scope` — this is how cross-org teaching and personal-problem-in-a-class work.
3. **Draft carve-out.** `P.status='draft'` hides `P` from everyone except editors of `P` (see below) and platform admins. Attachment does not override draft.
4. **Platform admin bypass.** `U.is_platform_admin` → always.

`archived` problems behave like `published` for viewing (existing attachments keep working) but are hidden from browse/search and cannot be attached to new topics.

#### Problem edit, create, and delete

Same role check governs create (in scope), patch, delete, and lifecycle transitions (`publish` / `archive` / `unarchive`):

- `platform` → platform admins only.
- `org` → `org_admin` or `teacher` role in the target org's `org_memberships`.
- `personal` → `P.created_by = U.id` only. No org-admin override for personal scope — personal is truly private.
- `forked_from` has no effect on edit after fork.

`DELETE /api/problems/{id}` additionally requires no `topic_problems` rows and no `attempts` on the problem (otherwise `409`). Archive is the escape valve for problems that have been used.

#### Attach / detach to a topic

`U` can attach `P` to `topic T` iff:
- `U` is an `org_admin` or `teacher` in `T.course.org_id`'s org, AND
- `U` can view `P`, AND
- `P.status = 'published'`.

Detach / reorder requires the first condition only (viewability may have changed; detach is always allowed by course staff).

#### Test cases

- Canonical (`owner_id IS NULL`): editable by anyone who can edit `P`.
  - `is_example=true`: visible to all `P` viewers.
  - `is_example=false` (hidden grader): full body visible only to `P` editors; students see `{id, name, order, is_example: false}` shells when listing and pass/fail + name after running.
- Private (`owner_id = U.id`): visible/editable to owner only. Never exposed to teachers, even in class context.

#### Solutions

- `is_published=false` → visible to `P` editors only (plus platform admins).
- `is_published=true` → visible to any `P` viewer.
- Publishing a solution is the "reveal the answer" teachable-moment lever; teachers gate it to after-class.

#### Attempt visibility (teacher view)

Teacher `T` can view attempts by `(user U, problem P)` iff:
- `U` is a member of some class `K` that `T` teaches (role `instructor` or `ta`), AND
- `P` is attached to at least one topic in `K.course`.

This preserves cross-class / cross-org privacy without adding `class_id` to `attempts`.

### Forking

`POST /api/problems/{id}/fork  { target_scope, target_scope_id? }` creates a new `problems` row:

- `target_scope` defaults to `'org'` when the caller is in exactly one org context, `'personal'` otherwise.
- Copies: title (suffixed `" (fork)"` unless caller overrides), description, starter_code, difficulty, grade_level, tags, time_limit_ms, memory_limit_mb, status (`draft` by default on the new copy).
- Copies **canonical** test cases (`owner_id IS NULL`).
- Copies **all** `problem_solutions` rows with `created_by = caller`, preserving `is_published`.
- Does **not** copy private test cases or attempts — those are per-user and stay on the source problem.
- Sets `forked_from = source.id` and `created_by = caller`.
- Caller must be able to view the source problem and must pass the §Problem edit/create/delete role check for the target scope.

Forking platform problems into org bank is the primary use case; forking across orgs is allowed as long as the caller is an editor of the target org.

### API surface

All routes served by the Go platform service. Nested resources follow the `/api/topics/.../problems` pattern from plan 024.

**Problems — CRUD & lifecycle**
```
GET    /api/problems                  list + filter (scope, difficulty, gradeLevel, tags, status, q, limit, cursor)
GET    /api/problems/{id}             detail (incl. scope, test case counts, solution counts, user's attempts count)
POST   /api/problems                  create (body: scope, scope_id?, title, description, ...)
PATCH  /api/problems/{id}             partial update
DELETE /api/problems/{id}             hard-delete; 409 if attached to any topic or any attempts exist
POST   /api/problems/{id}/publish     draft → published; 409 if not draft
POST   /api/problems/{id}/archive     published → archived; 409 if not published
POST   /api/problems/{id}/unarchive   archived → published
POST   /api/problems/{id}/fork        body: { target_scope, target_scope_id?, title? } → returns new Problem
```

**Topic attachment** (symmetric)
```
GET    /api/topics/{topicId}/problems
POST   /api/topics/{topicId}/problems                  body: { problem_id, sort_order? }
DELETE /api/topics/{topicId}/problems/{problemId}
PATCH  /api/topics/{topicId}/problems/{problemId}      body: { sort_order }
```

**Solutions**
```
GET    /api/problems/{id}/solutions                    filtered by visibility
POST   /api/problems/{id}/solutions                    draft by default
PATCH  /api/problems/{id}/solutions/{solutionId}
DELETE /api/problems/{id}/solutions/{solutionId}
POST   /api/problems/{id}/solutions/{solutionId}/publish
POST   /api/problems/{id}/solutions/{solutionId}/unpublish
```

**Test cases** — plan 024 endpoints unchanged structurally. Handlers re-check the refined visibility rules on read, returning shell objects for hidden canonical cases when the caller is not an editor.

**Attempts** — plan 024 endpoints unchanged.

**List-filter contract for `GET /api/problems`:**

Default `?scope=accessible` (platform published ∪ my orgs' published ∪ my personal). Explicit `?scope=platform|org|personal` narrows. `?gradeLevel=`, `?difficulty=`, `?tags=loops,strings` (AND semantics), `?q=` on title. Pagination via `(created_at, id)` cursor, same pattern as plan 020 org listings.

**Error model:**
- `404` when caller cannot view (never leak existence).
- `403` when caller can view but cannot edit / attach / publish.
- `409` on invalid state transitions or on delete of a referenced problem.
- `400` on schema / enum violations (rely on DB CHECK constraints as last line).

### Migration

One atomic Drizzle migration `drizzle/0013_problem_bank.sql` (next number after 0012):

1. `ALTER TABLE problems ADD COLUMN` the new columns (`scope`, `scope_id`, `slug`, `starter_code jsonb`, `difficulty`, `grade_level`, `tags`, `status`, `forked_from`, `time_limit_ms`, `memory_limit_mb`). Allow NULLs initially for the non-default ones.
2. Backfill:
   - `scope = 'org'`, `scope_id = courses.org_id` via join through `topics → courses`.
   - `starter_code = jsonb_build_object(language, starter_code)` using the old columns.
   - `difficulty = 'easy'` (curators adjust later), `grade_level = courses.grade_level`, `status = 'published'` (existing rows are live), `tags = '{}'`.
3. Flip `scope`, `difficulty`, `status` to `NOT NULL`; leave `scope_id` nullable (platform scope has `scope_id IS NULL`). Add CHECK constraint so only the platform row is allowed to be null.
4. `CREATE TABLE topic_problems`. Backfill `INSERT INTO topic_problems (topic_id, problem_id, sort_order, attached_by, attached_at)` from `(problems.topic_id, problems.id, problems.order, problems.created_by, problems.created_at)`.
5. `ALTER TABLE problems DROP COLUMN topic_id, language, "order"`.
6. `CREATE TABLE problem_solutions` (empty at migration time).
7. Add the `(scope, scope_id, status)`, `(created_by)`, slug unique, and GIN tag indexes.

All in one `BEGIN; ... COMMIT;`. `IF EXISTS` guards on column drops so dev DBs mid-state still apply cleanly. Migration 0012's pattern (idempotent re-runs) is the template.

**Rollback caveat:** once new problems are attached to multiple topics, `problems.topic_id` cannot be reconstructed cleanly. Apply dev → test → prod with verification at each step; the rollback window is tight by design. Production dry-run against a copy of the Python 101 seed is a plan 029 (implementation) gate.

**Consumer fan-out (order):**

1. Migration + Drizzle `schema.ts`.
2. Go store / handler updates + new endpoints (`platform/internal/store/problems.go` expansion; split `problem_solutions.go` if file pushes past ~400 LOC).
3. `scripts/seed_python_101.sql` rewritten to use the new model.
4. Existing student problem page + teacher watcher page — read-path adjustments only (no `topic_id` lookup on problem).
5. Bank browse UI — **deferred to spec 011** (portal UI redesign). This spec ships headless.

### Testing

- **Go handler integration tests** per endpoint, each covering: happy path, auth failure (401), authorization failure (403), cross-user / cross-org isolation (404), visibility matrix (platform × org × personal) × (draft × published × archived) × (creator / same-org teacher / other-org teacher / student / unauthenticated), fork semantics (copy shape and `forked_from`), attach/detach state transitions, `409` on invalid lifecycle transitions.
- **Go store unit tests**: `CHECK (scope, scope_id)` constraint enforcement, cascade deletes (`ON DELETE CASCADE` on topic → topic_problems; on problem → topic_problems, solutions, test_cases, attempts).
- **Drizzle / TS**: `tests/unit/schema.test.ts` asserts the new table shape.
- **Migration dry-run**: apply `0013` against a copy of the existing Python 101 dev data; assert row counts, `starter_code` jsonb correctness, `topic_problems` count equals the old problem count, `scope/scope_id` are set on every row.
- **E2E**: no new flows this spec; regression pass on plans 025/025b/026 flows after migration lands (the student problem page and teacher watcher still render attached problems).

## Follow-ups referenced but out of scope here

- **Bank browse UI** (filter / search / attach from the portal) → **spec 011** (portal UI redesign).
- **Problem revisions** (versioned statements with "new revision available — upgrade?" on linked attachments) → **spec 009b**, only if "platform updated mid-term" becomes a real operational pain.
- **Canonical solution validation** (run each `problem_solutions.code` against the canonical test cases on save, reject if fails) → implementation plan follow-up, tied to the Piston runner in plan 026.
- **Curated tag taxonomy** (formal `tags` table + admin UI) → once free-form `tags` stabilize and show up in analytics.
- **Student-authored personal problems** (contest practice, portfolio) → creation today is restricted to teachers / org_admins / platform admins; students as authors is a future toggle.
- **Contest mode** — timed, frozen statement, hidden-cases-only view → CP-direction follow-up spec.
- **Shareable problem URL `/p/{slug}`** → schema supports it (`slug` + unique index within scope); UI routing deferred.
- **Session → problem reference** (spec 010 integration) — `sessions` table doesn't need a `problem_id` column; sessions reference topics already, and any problem attached to a referenced topic is reachable via the §Access policy "attachment grants read" rule. Orphan sessions reach problems via the session creator's scope + any problems explicitly attached via a future `session_problems` join (deferred to spec 010's plan, not this one).
