# 012 — Teaching Units: Authoring, Reuse, and Delivery

## Problem

Today a teacher assembling a lesson edits five unrelated tables: `topics` (prose), `problems` (statement + starter + tests), `problem_solutions` (reference answers), `sessions` (live class plan), and `assignments` (graded sets). Each table has its own API, its own UI, its own form. A single pedagogical unit — "while loops, 6th grade, 45 minutes" — fragments across them. Teachers reassemble it mentally every time they use it.

Two other gaps compound the fragmentation:

1. **No authoring primitive for the unit.** `problem.description` is markdown; `topic.lessonContent` is JSONB; there's no editor that treats a lesson-plus-exercises as one thing.
2. **No reuse primitive at the lesson level.** Problems can be attached to many topics (from plan 028), but the lesson narrative itself is inert — a teacher who wants to use another teacher's "Intro to While Loops" lesson has nothing to link or fork.

This spec defines the **teaching unit** — a schema-backed block document that composes prose and reusable atoms (problems, test cases, solutions) — and an overlay-based reuse model that lets teachers inherit from platform or peer units while overriding specific blocks.

## Design

The design rests on three load-bearing choices:

1. **Typed block document as the authoring primitive.** Not markdown, not structured forms.
2. **Overlay-based reuse.** Not binary link-vs-fork.
3. **Atom-versus-block separation.** Problems/solutions/test cases stay normalized; blocks reference them.

### Core concepts

```
teaching_unit        = ordered block document with metadata + lifecycle state
block                = typed node in the document (prose, problem-ref, note, cue, …)
atom                 = first-class pedagogical entity (problem, solution, test-case)
unit_overlay         = inheritance record: {parent_unit_id, overrides: [block_patch]}
unit_revision        = immutable snapshot of a unit at a point in time; basis for pinning
```

### Block types

| Block type         | Student sees | Teacher sees | Referenceable atom? | Purpose |
|--------------------|:------------:|:------------:|:-------------------:|---------|
| `prose`            | ✓            | ✓            |         —           | Rich text: explanation, concepts, examples. Math + images. |
| `code-snippet`     | ✓            | ✓            |         —           | Inline non-executable example code for demonstration. |
| `problem-ref`      | ✓            | ✓            | Problem             | Embed a problem from the bank. Pins to a revision or floats. |
| `test-case-ref`    | only meta    | ✓            | TestCase            | Attached to a `problem-ref`; overrides visibility (show/hide). |
| `solution-ref`     | opt-in       | ✓            | Solution            | Reveal-after-attempt or always-visible. |
| `teacher-note`     | ✗            | ✓            |         —           | Setup instructions, anticipated misconceptions, timing. |
| `live-cue`         | ✗            | ✓ (highlighted during live session) | — | "Pause here. Ask: what happens when condition is false?" |
| `assignment-variant` | when graded | ✓          |         —           | Wrapper that turns child blocks into a gradable submission with timing + rubric hook. |
| `media-embed`      | ✓            | ✓            |         —           | Image, video, PDF, diagram. |

Prose blocks carry rich text (bold/italic/code/math/lists/headings). Everything else is a typed node with structured attributes.

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
├── parent_unit_id     uuid NULL → teaching_units.id   (overlay inheritance)
├── parent_revision_id uuid NULL → unit_revisions.id   (pinned revision; NULL = float)
├── created_by         uuid FK → users.id
├── created_at / updated_at
│
├── CHECK scope/scope_id consistency
├── INDEX (scope, scope_id, status)
├── GIN INDEX (subject_tags), GIN INDEX (standards_tags)
└── vector INDEX on summary + title (pgvector for semantic search)

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
├── parent_revision_id uuid NULL → unit_revisions.id (NULL = follow latest of parent)
├── block_overrides    jsonb        — map: source_block_id → {replace|hide|patch}
├── created_at / updated_at
│
└── One child can have at most one parent (single-inheritance). Multi-parent/merge is out of scope.

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

Block JSON follows the Tiptap/ProseMirror convention. Each embed has an `attrs` object:

```json
{
  "type": "doc",
  "content": [
    { "type": "prose", "content": [{ "type": "text", "text": "Loops let you repeat work." }] },
    { "type": "media-embed", "attrs": { "url": "https://…/while-loop-diagram.png", "alt": "While loop" } },
    {
      "type": "problem-ref",
      "attrs": {
        "id": "b12-01",
        "problemId": "…",
        "pinnedRevision": null,
        "visibility": "when-unit-active",
        "overrideStarter": null
      }
    },
    { "type": "teacher-note", "content": [{ "type": "text", "text": "Most 6th-graders confuse `while` with `if`." }] },
    {
      "type": "live-cue",
      "attrs": { "id": "b12-02", "trigger": "before-problem", "problemRefId": "b12-01" },
      "content": [{ "type": "text", "text": "Ask: what's the termination condition here?" }]
    }
  ]
}
```

Each block has a stable `attrs.id` so overlays can reference it.

### Overlay semantics (the reuse model)

When a teacher **inherits** a parent unit (`parent_unit_id` set), the rendered document at read time is:

1. Load the parent at `parent_revision_id` (or parent's latest `classroom_ready` revision if floating).
2. Walk each block in the parent. For each block with `attrs.id = X`:
   - If `block_overrides[X] = {action: "hide"}` → skip.
   - If `block_overrides[X] = {action: "replace", block: {...}}` → render the override block.
   - If `block_overrides[X] = {action: "patch", patch: {...}}` → deep-merge the patch onto the parent block.
   - Otherwise → render the parent block as-is.
3. Append any child-only blocks (blocks in the child document that have no `parent_block_id` reference).

This gives teachers three granularities:
- **Float and consume.** No overrides. Stays in sync with parent; upstream fixes propagate.
- **Swap a few blocks.** Override 1–3 blocks (e.g., replace the third problem with a harder one). Everything else stays synced.
- **Pin and freeze.** Set `parent_revision_id` — the teacher controls when to adopt new parent revisions.

**Lineage** (`parent_unit_id → parent_unit_id → …`) is a chain, not a graph. Display as breadcrumb: "Platform › Lincoln HS (forked 2026-03) › Your adaptation (forked 2026-04)".

### Lifecycle & view projections

Every teaching unit has a `status`:

| Status | Meaning | Student visibility | Teacher visibility (own org) | Listable in library |
|--------|---------|-------------------|------------------------------|---------------------|
| `draft`           | Author is still working | ✗ | Author only                     | ✗ |
| `reviewed`        | Peer-reviewed; not yet in classroom use | ✗ | Author + org_admin + reviewer | ✗ |
| `classroom_ready` | Cleared for K-12 classroom use | ✓ when unit is referenced by an active class | All teachers in scope | ✓ (platform/org) |
| `coach_ready`     | Cleared for competitive-programming / after-school use | ✓ when referenced | All teachers in scope | ✓ (platform/org) |
| `archived`        | No longer recommended but kept for existing classes | Preserved for existing refs; hidden from new | All teachers in scope | ✗ |

**View projections:** the render layer accepts a `viewer_role` ∈ `{student, teacher, author, reviewer}`. Blocks have per-type visibility rules (see the block-types table). The projection filters and transforms the block tree before rendering — students never receive `teacher-note` or `live-cue` blocks; solutions are projected based on the attempt state.

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

### Access policy

Aligns with spec 009 (problem bank) scopes:

- **View:** `scope='platform'` + `status ∈ {classroom_ready, coach_ready}` → any authenticated user. `scope='org'` → members of the org see `classroom_ready`/`coach_ready`/`archived`; teachers also see `draft`/`reviewed`. `scope='personal'` → author only. Archived units remain visible to any class currently referencing them.
- **Edit:** `platform` → platform admins. `org` → any `org_admin` or `teacher` in that org. `personal` → `created_by = userId` only.
- **Fork (create overlay child):** any viewer of the parent. The child lives in the forker's chosen scope.
- **Publish transition** (`draft → reviewed → classroom_ready`/`coach_ready`): any editor can transition within edit scope. A future plan may add a required-reviewer gate for platform scope.

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
2. **Plan 032 — Full block palette + lifecycle.** Add `teacher-note`, `live-cue`, `solution-ref`, `test-case-ref`, `assignment-variant`, `media-embed`. Status transitions (`draft → classroom_ready`). View projections per role.
3. **Plan 033 — Overlay reuse.** `unit_overlays`, `parent_unit_id` on units, overlay render pipeline. Fork/inherit UI in the library.
4. **Plan 034 — Realtime co-edit + AI drafting.** Yjs binding via `y-prosemirror`. Claude API tool use for `create_unit` / `add_*` scaffolding.
5. **Plan 035 — Discovery + libraries.** pgvector + FTS composite search, library surfaces, quality-signal capture. Unit collections.
6. **Plan 036 — Migration.** Convert existing `topic.lessonContent` and `topic_problems` into teaching units; wire sessions to units; deprecate direct topic/problem-list editing in the teacher UI.

Each plan ships independently and leaves the app working. Earlier plans don't depend on later ones.

## Non-goals (explicitly)

- **Rich WYSIWYG math** beyond KaTeX. Math research papers are not the target audience.
- **Video editing.** Media embeds reference external URLs; hosting and editing are out of scope.
- **Multi-parent inheritance** on overlays. Single parent only. Cross-pollination via AI-assisted assembly, not schema.
- **Live collaborative editing of running sessions.** Unit edits during a live class are deliberately disabled — the unit is snapshot-pinned when the session starts.
- **Grading rubrics inside units.** `assignment-variant` blocks trigger the existing assignment/submission flow; rubric design is a follow-up spec.
- **Public community sharing and moderation.** Surface stub only; governance is its own spec.

## Open questions

- **Block ID stability across overlay overrides when the parent revises and inserts new blocks.** Proposed: append-only block IDs (never reused); inserts get new IDs; overlays reference IDs not positions. But this requires careful editor plumbing. Prototype in plan 031 before committing.
- **AI-authored units as `platform` scope vs. always starting personal/org.** Proposed: AI drafts always land in the caller's personal scope; promotion to org or platform is a deliberate human step.
- **Standards taxonomy source.** CSTA, state-specific, ISTE? Start with free-form `standards_tags text[]` — add a curated taxonomy table once a catalog emerges.
- **Unit templates vs. inheritance.** Proposed: a template is just a unit with `status='classroom_ready'` and a `template=true` flag surfaced in discovery. Forking it is the same overlay mechanism — no separate template concept in the schema.

## Follow-ups referenced but out of scope here

- **Session → unit pinning** (replaces or augments `session_topics`) → follow-up after plans 031–033 land.
- **Assignment-variant grading flow** integration with the existing `submissions` pipeline → plan 032 or its own plan.
- **Moderated community library** → post-MVP spec.
- **Standards-based curriculum mapping** (CSTA / CSTA+) → post-MVP spec once content catalog matures.
- **Offline editing / local-first sync** → would layer on Yjs cleanly but is out of scope for v1.
