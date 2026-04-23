# 012 — Teaching Units: Authoring, Reuse, and Delivery

## Problem

Today a teacher assembling a lesson edits five unrelated tables: `topics` (prose), `problems` (statement + starter + tests), `problem_solutions` (reference answers), `sessions` (live class plan), and `assignments` (graded sets). Each table has its own API, its own UI, its own form. A single pedagogical unit — "while loops, 6th grade, 45 minutes" — fragments across them. Teachers reassemble it mentally every time they use it.

Two other gaps compound the fragmentation:

1. **No authoring primitive for the unit.** `problem.description` is markdown; `topic.lessonContent` is JSONB; there's no editor that treats a lesson-plus-exercises as one thing.
2. **No reuse primitive at the lesson level.** Problems can be attached to many topics (from plan 028), but the lesson narrative itself is inert — a teacher who wants to use another teacher's "Intro to While Loops" lesson has nothing to link or fork.

This spec defines the **teaching unit** — a schema-backed block document that composes prose and reusable atoms (problems, test cases, solutions) — and an overlay-based reuse model that lets teachers inherit from platform or peer units while overriding specific blocks.

## Design

The design rests on four load-bearing choices:

1. **Typed block document as the authoring primitive.** Not markdown, not structured forms.
2. **Overlay-based reuse.** Not binary link-vs-fork.
3. **Atom-versus-block separation.** Problems/solutions/test cases stay normalized; blocks reference them.
4. **Publishing / runtime projection layer.** Access control (who can see the unit at all) and render projection (which blocks appear in which viewer's output) are separate axes. Access is `(scope × status × viewer_role)`. Projection is `(block × viewer_role × attempt_state) → render_decision`. They compose but never merge.

### Core concepts

```
teaching_unit        = ordered block document with metadata + lifecycle state
block                = typed node in the document; carries a stable attrs.id
atom                 = first-class pedagogical entity (problem, solution, test-case)
unit_overlay         = single-source-of-truth inheritance row linking child → parent + block overrides
unit_revision        = immutable snapshot of a unit at a point in time; basis for pinning
```

**Key invariant:** a unit is "inherited" iff a `unit_overlays` row exists for its id as `child_unit_id`. The `teaching_units` table does NOT duplicate `parent_unit_id`/`parent_revision_id` fields — the overlay row is the only place inheritance is recorded.

### Block types

**All blocks carry `attrs.id`** (stable, append-only identifier). This is schema-enforced in Tiptap and survives markdown round-trips via HTML comments (`<!-- block:<id> -->`).

| Block type         | `attrs.id` | Referenceable atom? | Purpose |
|--------------------|:----------:|:-------------------:|---------|
| `prose`            | ✓          |         —           | Rich text: explanation, concepts, examples. Math + images. |
| `code-snippet`     | ✓          |         —           | Inline non-executable example code for demonstration. |
| `problem-ref`      | ✓          | Problem             | Embed a problem from the bank. Pins to a revision or floats. |
| `test-case-ref`    | ✓          | TestCase            | Attached to a `problem-ref`; overrides visibility (show/hide). |
| `solution-ref`     | ✓          | Solution            | Reveal-after-attempt or always-visible. |
| `teacher-note`     | ✓          |         —           | Setup instructions, anticipated misconceptions, timing. |
| `live-cue`         | ✓          |         —           | "Pause here. Ask: what happens when condition is false?" |
| `assignment-variant` | ✓        |         —           | Wrapper that turns child blocks into a gradable submission with timing + rubric hook. |
| `media-embed`      | ✓          |         —           | Image, video, PDF, diagram. |

Per-block visibility is described separately in §Render projection below — it is not a property of the block type alone but of `(block × viewer_role × attempt_state)`. Prose blocks carry rich text (bold/italic/code/math/lists/headings). Everything else is a typed node with structured attributes.

### Data model

```
teaching_units
├── id                 uuid PK
├── scope              enum {platform, org, personal}
├── scope_id           uuid NULL       (NULL for platform; orgId / userId otherwise)
├── title              varchar(255)
├── slug               varchar(255)    unique within scope
├── summary            text            short description for library cards
├── grade_level        enum {K-5, 6-8, 9-12}
├── subject_tags       text[]          e.g. ['loops','arrays','recursion']
├── standards_tags     text[]          e.g. ['CSTA.3A.AP.11']
├── estimated_minutes  int
├── status             enum {draft, reviewed, classroom_ready, coach_ready, archived}
├── created_by         uuid FK → users.id
├── created_at / updated_at
│
├── CHECK scope/scope_id consistency
├── INDEX (scope, scope_id, status)
├── GIN INDEX (subject_tags), GIN INDEX (standards_tags)
└── vector INDEX on summary + title (pgvector for semantic search)

(No `parent_unit_id` / `parent_revision_id` columns — inheritance lives exclusively in `unit_overlays`.)

unit_documents
├── unit_id            uuid PK → teaching_units.id (1:1)
├── blocks             jsonb   NOT NULL          — ordered ProseMirror/Tiptap doc JSON
├── updated_at
│
└── `blocks` shape: {"type":"doc","content":[ {"type":"prose", …}, {"type":"problem-ref","attrs":{"problemId":"…","pinnedRevision":null,"overrideVisibility":null}}, … ]}

unit_revisions
├── id                 uuid PK
├── unit_id            uuid FK → teaching_units.id  ON DELETE CASCADE
├── blocks             jsonb   NOT NULL          — frozen snapshot of the document
├── reason             varchar(255)              — "published", "revised after review"
├── created_by         uuid FK → users.id
├── created_at         timestamptz
│
└── Only `classroom_ready` or `coach_ready` transitions create a revision. Drafts don't.

unit_overlays
├── child_unit_id      uuid PK → teaching_units.id
├── parent_unit_id     uuid FK → teaching_units.id
├── parent_revision_id uuid NULL → unit_revisions.id (NULL = follow parent's latest classroom_ready/coach_ready revision)
├── block_overrides    jsonb   NOT NULL DEFAULT '{}'::jsonb
├── created_at / updated_at
│
└── One child can have at most one parent (single-inheritance). Multi-parent/merge is out of scope.

`block_overrides` shape:
```json
{
  "<parent_block_id>": { "action": "hide" },
  "<parent_block_id>": { "action": "replace", "block": { /* full child block, will be rendered instead of the parent block */ } }
}
```

Only two actions: `hide` and `replace`. There is no `patch`/deep-merge action — it was considered but rejected because ProseMirror/Tiptap nodes have arrays of marks and content that don't have unambiguous merge semantics. If a teacher wants to tweak one phrase of a parent prose block, they `replace` with the full new block. This is less clever but deterministic: one rule for every override.

unit_collections
├── id                 uuid PK
├── scope              enum {platform, org, personal}
├── scope_id           uuid NULL
├── title              varchar(255)
├── description        text
├── created_by         uuid FK → users.id
│
unit_collection_items
├── collection_id      uuid FK → unit_collections.id ON DELETE CASCADE
├── unit_id            uuid FK → teaching_units.id
├── sort_order         int
└── PK (collection_id, unit_id)
```

The existing atom tables (`problems`, `test_cases`, `problem_solutions`) are **not touched**. This spec layers on top.

### Block document schema

Block JSON follows the Tiptap/ProseMirror convention. **Every top-level block carries `attrs.id`** (required by the schema, not optional) and an optional `attrs.parentId` used by child documents to anchor a child-only block after a specific parent block.

```json
{
  "type": "doc",
  "content": [
    {
      "type": "prose",
      "attrs": { "id": "b01" },
      "content": [{ "type": "text", "text": "Loops let you repeat work." }]
    },
    {
      "type": "media-embed",
      "attrs": { "id": "b02", "url": "https://…/while-loop-diagram.png", "alt": "While loop" }
    },
    {
      "type": "problem-ref",
      "attrs": {
        "id": "b03",
        "problemId": "…",
        "pinnedRevision": null,
        "visibility": "when-unit-active",
        "overrideStarter": null
      }
    },
    {
      "type": "teacher-note",
      "attrs": { "id": "b04" },
      "content": [{ "type": "text", "text": "Most 6th-graders confuse `while` with `if`." }]
    },
    {
      "type": "live-cue",
      "attrs": { "id": "b05", "trigger": "before-problem", "problemRefId": "b03" },
      "content": [{ "type": "text", "text": "Ask: what's the termination condition here?" }]
    }
  ]
}
```

**Block ID stability rules:**
- IDs are append-only across a unit's lifetime: a deleted block's ID is never reused.
- A split (one prose block becomes two) allocates a new ID for the new half; the original block keeps its id.
- A merge (two prose blocks become one) keeps the id of the first block and discards the second's id.
- `remark/unified` round-trips preserve IDs via HTML comments: `<!-- block:b03 -->` before the block's markdown representation. The markdown reader parses these; if a comment is missing on import, a new id is generated (not a reuse).
- The **one-time migration** of `topic.lessonContent` into a teaching unit generates fresh IDs — it is a cut, not a rename. Units forked from the migrated unit thereafter have stable IDs.

In a child overlay document, any block that represents net-new content (not an override of a parent block) MUST carry `attrs.parentId` pointing at the parent block it's anchored *after* (rendered right after that parent block), or `attrs.parentId = null` to anchor at the document end. This gives overlay rendering a deterministic position for every child-only block even when the parent inserts new blocks around it.

### Overlay semantics (the reuse model)

When a unit has a `unit_overlays` row (it inherits from a parent), its **rendered document** is computed:

1. Load the parent's blocks from `unit_revisions` at `parent_revision_id`, or from `unit_documents` of the parent's latest `classroom_ready`/`coach_ready` revision if `parent_revision_id IS NULL` (floating).
2. Load the child's own `unit_documents.blocks`. Every child block carries `attrs.parentId` — either an id of a parent block (anchor) or `null` (append-to-end).
3. Build the output by walking the parent's block sequence, inserting child-anchored blocks after their anchor:
   ```
   for each parent_block P in parent.blocks:
       override = overlay.block_overrides[P.attrs.id]
       if override.action == "hide":   emit nothing
       elif override.action == "replace":   emit override.block
       else:                           emit P unchanged
       for each child_block C in child.blocks where C.attrs.parentId == P.attrs.id:
           emit C
   // finally:
   for each child_block C in child.blocks where C.attrs.parentId == null:
       emit C
   ```
4. If a child block's `attrs.parentId` refers to a parent block id that no longer exists (because the parent was revised and that block was deleted), the child block **falls through to the end of the document** (rendered at the tail). This is deterministic and visible to the teacher, who can reanchor it manually.

This gives teachers three granularities:
- **Float and consume.** No overrides, no child-only blocks. Stays in sync with parent; upstream fixes propagate.
- **Swap or hide blocks.** Use `hide` to omit a parent block; use `replace` to substitute a full block. Net-new insertions go in the child document with `parentId` anchoring.
- **Pin and freeze.** Set `parent_revision_id` — the teacher controls when to adopt new parent revisions.

**Lineage** (chain of overlays: parent.overlay.parent_unit_id → grandparent → …) is single-inheritance only. Display as breadcrumb: "Platform › Lincoln HS (forked 2026-03) › Your adaptation (forked 2026-04)".

### Lifecycle (status machine)

Every teaching unit has a `status`:

| Status | Meaning |
|--------|---------|
| `draft`           | Author is still working. Not in any library. |
| `reviewed`        | Peer-reviewed; not yet in classroom use. |
| `classroom_ready` | Cleared for K-12 classroom use. |
| `coach_ready`     | Cleared for competitive-programming / after-school use. |
| `archived`        | No longer recommended but kept for existing class references. |

Transitions allowed: `draft → reviewed → classroom_ready | coach_ready`; any state can go to `archived`; `archived → classroom_ready` (unarchive). Drafts cannot be archived. Only `classroom_ready` and `coach_ready` transitions create a `unit_revisions` snapshot.

### Access (who can *see* the unit)

Access is a pure function of `(viewer_role, scope, scope_id, status)`. It decides whether the viewer can load the unit at all. A caller who fails this check gets 404 (spec 009 pattern — never leak existence).

| Scope      | Viewer                                       | status=draft | status=reviewed | status=classroom_ready / coach_ready | status=archived |
|------------|----------------------------------------------|:------------:|:---------------:|:------------------------------------:|:---------------:|
| platform   | any auth                                      |      ✗       |        ✗        |                 ✓                    |  ✓ (view only)  |
| platform   | platform_admin                                |      ✓       |        ✓        |                 ✓                    |        ✓        |
| org        | student in org                                |      ✗       |        ✗        |           ✓ (via class ref)          | ✓ (via existing class ref) |
| org        | teacher / org_admin in org                    |      ✓       |        ✓        |                 ✓                    |        ✓        |
| personal   | owner                                         |      ✓       |        ✓        |                 ✓                    |        ✓        |
| personal   | non-owner                                     |      ✗       |        ✗        |                 ✗                    |        ✗        |

"via class ref" = a student sees the unit because a class they belong to has bound this unit into a session or an assignment. Without such a binding, students never directly browse the library.

### Render projection (which blocks appear in the output)

Once a viewer has access, the render pipeline **projects** the block tree for that viewer. Projection is a pure function of `(block, viewer_role, attempt_state)`. Inputs:

- `viewer_role`: one of `student | teacher | author | reviewer | platform_admin`.
- `attempt_state` (per `problem-ref` block): `not_started | in_progress | submitted | passed | failed`. Falls back to `not_started` outside a class-bound session.

Per-block projection rules:

| Block type           | student                                                        | teacher / author / reviewer / platform_admin |
|----------------------|---------------------------------------------------------------|----------------------------------------------|
| `prose`              | rendered                                                       | rendered                                     |
| `code-snippet`       | rendered                                                       | rendered                                     |
| `problem-ref`        | rendered iff `attrs.visibility ∈ {"always", "when-unit-active"}` (the latter requires a class-bound session). Student sees statement, canonical examples, own test cases; hidden cases redacted per spec 009. | always rendered with full body |
| `test-case-ref`      | rendered as shell (name only) if canonical hidden; full body otherwise | always full body |
| `solution-ref`       | rendered iff `attrs.reveal ∈ {"always", "after-submit"}` AND (if `after-submit`) `attempt_state ∈ {submitted, passed, failed}` | always rendered |
| `teacher-note`       | **omitted from output** | rendered |
| `live-cue`           | **omitted from output** | rendered (highlighted during live session when its `trigger` matches the session's current problem) |
| `assignment-variant` | rendered iff the viewer's class has this unit's `assignment-variant` wrapped in an active assignment; otherwise omitted | always rendered |
| `media-embed`        | rendered                                                       | rendered                                     |

The projection runs **after** overlay composition and **before** final HTML render. It is deterministic, cacheable per `(unit_id, parent_revision_id, viewer_role, attempt_state_fingerprint)`, and purely read-side — it never mutates persisted state.

### Authoring UX (the editor)

- **Default mode:** block editor with a slash-command insertion palette (`/problem`, `/teacher-note`, `/live-cue`). The editor renders blocks with their own UI — `problem-ref` shows the embedded problem with difficulty/tags, not raw JSON.
- **Power-user mode:** "View markdown" toggle renders a subset of the document as markdown (prose + problem-ref as a callout, teacher-notes stripped). Imports accept markdown and hydrate back to blocks using `remark/unified` as the boundary.
- **AI drafting:** a separate "Draft with AI" panel accepts intent ("6th grade, while loops, 45 min, 3 problems easy→medium"). Calls the Claude API with structured tool use (`create_unit`, `add_problem_draft`, `add_teacher_note`, `add_live_cue`). The response streams into the editor as a fresh draft the teacher reviews and publishes.
- **Realtime co-editing:** Yjs + `y-prosemirror` binding. Two teachers can author a unit together live. Uses the same Hocuspocus service as code collaboration.
- **Preview:** toggle to a student view projection without leaving the editor. Shows exactly what a class member will see when the unit is delivered.

### Tech stack

| Layer | Choice | Rationale |
|-------|--------|-----------|
| Block editor | **Tiptap** (direct, not via BlockNote) | Custom node types map naturally to typed blocks; full control over schema + markdown serialization. BlockNote is convenience at the cost of control. |
| Realtime | **Yjs + `y-prosemirror`** | Already in stack for code collaboration; binding is native for ProseMirror/Tiptap. |
| Document storage | **Postgres JSONB** on `unit_documents` | Editor JSON is the canonical shape; normalize only atoms with independent lifecycle. |
| Atom storage | Existing `problems`, `test_cases`, `problem_solutions` | Reuse what plan 028 built. |
| Semantic search | **pgvector** on unit title + summary + first N prose blocks, *paired with* Postgres full-text + structured filters | Embeddings alone miss grade-level/tag filters; FTS alone misses concept similarity. Use both. |
| Markdown I/O | **`remark` + `unified`** | Main boundary in both directions. `turndown` is a lossy HTML-to-markdown escape hatch; not the backbone. |
| AI drafting | **Anthropic API** with tool use | Structured tools produce block-shaped output that lands directly in the editor. |
| Server rendering | React Server Components; same components used by editor preview and student view | View projections happen in the render pipeline, not in the editor. |
| Versioning | `unit_revisions` rows on publish transitions + Yjs update log for live edit history | Cheap point-in-time snapshots; Yjs gives fine-grained edit history between snapshots. |

### Access policy (edit + create + fork)

**View access** is fully captured by the §Access table above. The remaining access decisions:

- **Edit** (update blocks, change metadata, change status): `platform` → platform admins. `org` → any `org_admin` or `teacher` in that org (`org_memberships.role`). `personal` → `created_by = userId` only.
- **Create** in a scope: same role check as edit.
- **Fork** (create a new unit with `unit_overlays.parent_unit_id = parent`): requires view access to the parent, plus create access in the target scope.
- **Status transitions** (`draft → reviewed → classroom_ready`/`coach_ready`, or unarchive): require edit access. A future plan may add a required-reviewer gate for platform scope.

### How existing materials flow in

This spec does not delete `topics`, `problems`, or `sessions`. Migration is additive:

1. Each existing `topic.lessonContent` becomes a `teaching_unit` with `scope='org'`, `scope_id=topic.course.org_id`, a single `prose` block containing the lesson, and an ordered sequence of `problem-ref` blocks for problems currently attached via `topic_problems`. `status='classroom_ready'` for existing topics assumed to be live.
2. `sessions` continue to reference `topics` via `session_topics`. A follow-up plan may introduce a `session_units` join so sessions can pin to specific unit revisions.
3. `assignments` become assignable via an `assignment-variant` block wrapping one or more `problem-ref` blocks; the existing `assignments` table stores the gradable record.

The migration is phased so existing flows keep working. See "Phased rollout" below.

### Discovery layers

Teachers see four distinct surfaces:

- **My drafts** — personal-scope units in `draft`/`reviewed`.
- **Org library** — org-scope units the viewer's org has, filterable by subject tags, grade, difficulty, language, duration.
- **Platform library** — `scope='platform'` curated content.
- **Community** — opt-in public sharing. Out of initial scope; stub the UI surface.

All four surfaces share a search input that combines pgvector semantic similarity with Postgres FTS and structured filters. Result ordering blends score + quality signals (usage count, completion rate, teacher ratings).

### Quality signals (future-gated)

Captured per unit over time:
- `class_usages` — how many classes have referenced this unit
- `student_completion_rate` — ratio of assigned students who completed it
- `student_success_rate` — ratio whose attempts passed
- `teacher_ratings` — explicit stars + optional review text

These feed discovery ranking and provide observational data for curriculum designers. The scoring function itself is out of scope for v1; capture the signals, derive ranking later.

## Phased rollout

This is a large surface. Recommended plan split:

1. **Plan 031 — Teaching unit data model + minimal editor.** Schema (`teaching_units`, `unit_documents`, `unit_revisions`), Tiptap editor with `prose` + `problem-ref` blocks only, no overlay, no AI, no realtime. Write + save + display.
2. **Plan 032 — Migration of existing topics and problem attachments.** Convert each `topic.lessonContent` + its `topic_problems` rows into a teaching unit (`scope='org'`, `status='classroom_ready'`). Sessions and assignments continue to resolve through a compatibility shim that also returns a `unit_id`. This lands early so plan 033 onward operates on a library with real content.
3. **Plan 033 — Full block palette + lifecycle + projection.** Add `teacher-note`, `live-cue`, `solution-ref`, `test-case-ref`, `assignment-variant`, `media-embed`. Status transitions + `unit_revisions` snapshot creation. Render projection pipeline with per-role filtering and attempt-state resolution.
4. **Plan 034 — Overlay reuse.** `unit_overlays` table, overlay render pipeline (the algorithm in §Overlay semantics), fork/inherit UI in the library, breadcrumb lineage display.
5. **Plan 035 — Realtime co-edit + AI drafting.** Yjs binding via `y-prosemirror`. Anthropic API tool use for `create_unit` / `add_*` block scaffolding. Preview of student projection inside the editor.
6. **Plan 036 — Discovery + libraries.** pgvector + Postgres FTS composite search, library surfaces (platform / org / personal), quality-signal capture, unit collections. Operates on the real migrated content from plan 032 and any content authored after.

Each plan ships independently and leaves the app working. Earlier plans don't depend on later ones.

## Non-goals (explicitly)

- **Rich WYSIWYG math** beyond KaTeX. Math research papers are not the target audience.
- **Video editing.** Media embeds reference external URLs; hosting and editing are out of scope.
- **Multi-parent inheritance** on overlays. Single parent only. Cross-pollination via AI-assisted assembly, not schema.
- **Live collaborative editing of running sessions.** Unit edits during a live class are deliberately disabled — the unit is snapshot-pinned when the session starts.
- **Grading rubrics inside units.** `assignment-variant` blocks trigger the existing assignment/submission flow; rubric design is a follow-up spec.
- **Public community sharing and moderation.** Surface stub only; governance is its own spec.

## Open questions

- **AI-authored units as `platform` scope vs. always starting personal/org.** Proposed: AI drafts always land in the caller's personal scope; promotion to org or platform is a deliberate human step.
- **Standards taxonomy source.** CSTA, state-specific, ISTE? Start with free-form `standards_tags text[]` — add a curated taxonomy table once a catalog emerges.
- **Unit templates vs. inheritance.** Proposed: a template is just a unit with `status='classroom_ready'` and a `template=true` flag surfaced in discovery. Forking it is the same overlay mechanism — no separate template concept in the schema.
- **Attempt-state fingerprint granularity for projection caching.** The projection cache key includes `attempt_state_fingerprint`. Per-student-per-unit is the natural granularity but may be too fine. Prototype in plan 033.

## Follow-ups referenced but out of scope here

- **Session → unit pinning** (replaces or augments `session_topics`) → follow-up after plans 031–033 land.
- **Assignment-variant grading flow** integration with the existing `submissions` pipeline → plan 032 or its own plan.
- **Moderated community library** → post-MVP spec.
- **Standards-based curriculum mapping** (CSTA / CSTA+) → post-MVP spec once content catalog matures.
- **Offline editing / local-first sync** → would layer on Yjs cleanly but is out of scope for v1.
