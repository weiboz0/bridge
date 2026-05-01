# Plan 051 — Teaching Unit Architecture Review (SPEC / DESIGN, NOT EXECUTION)

## Status

- **Date:** 2026-04-30
- **Branch:** TBD (no implementation yet)
- **Type:** **Comprehensive design review.** This plan does NOT ship code. It frames open architectural questions about the teaching-unit data model, identifies what needs to change before plan 049 (Python 101 curriculum) hits scale, and produces concrete options + trade-offs for product to decide on. Implementation lands in plan 052+ once the design lands.
- **Source:** User request 2026-04-30 — "Let's put a placeholder plan to clean up the teaching units and topics relation. We should also add a teaching unit group concept (like a book). And review how the overlay or versioning of a unit should work. We need a comprehensive review."

## Why this review now

Plan 049 (Python 101 curriculum) hit several architectural friction points the existing schema can't solve cleanly:

1. **`teaching_units.topic_id` is a 1:1 partial-unique constraint.** A unit can be linked to AT MOST one topic across the entire platform. Once Bridge HQ's "Loops" topic claims the platform-scope `loops` unit, no other course can attach the same unit. Plan 049 documented this as "first-come, first-served claim semantics" and accepted clone-and-author-your-own as the workaround for adopting orgs. That's a bad UX and worse content-engineering — every adopter's "Loops" lesson drifts independently.
2. **No grouping above a unit, below a course.** Bridge HQ Python 101 has 12 standalone units stitched together by the course's topics. There's no "Python 101 textbook" entity that bundles those 12 units, with shared metadata (a single grade band, a single author, a single license, a single "publish all 12 at once" verb). Adopting orgs can't say "give me the whole Python 101 book" — they have to clone a course and re-link 12 units (which is impossible per #1 anyway).
3. **`unit_overlays` exists but the workflow doesn't.** `drizzle/0018_unit_overlays.sql` adds the table; `platform/internal/store/teaching_units.go` has helpers. But there's no UI affordance for "override THIS paragraph in the Loops unit for my class," no clear policy for when an overlay vs a fork is the right answer, no versioning of the canonical unit when its content drifts.
4. **No unit versioning / revisions.** `unit_revisions` table exists (snapshots taken on lifecycle transitions per plan 033/034) but there's no mechanism for "Bridge HQ updated the Loops unit; what happens to a class mid-semester whose teacher overlaid the v1 content?" or "I want my course to pin Loops@v3 even after Bridge HQ ships v4."

Plan 049 will fail at scale (e.g., when 100 schools want to adopt Python 101 and tweak it differently) without resolving these. So we hold the line at "12 units claimed by Bridge HQ" for plan 049 and use plan 051 to figure out the right architecture.

## Three problems in detail

### Problem 1: 1:1 unit↔topic vs N:M reality

**Schema today** (`drizzle/0017_topic_to_units.sql:15-16`):
```sql
CREATE UNIQUE INDEX teaching_units_topic_id_uniq
  ON teaching_units(topic_id) WHERE topic_id IS NOT NULL;
```

The partial-unique index means:
- A topic links to 0 or 1 unit (the topic-side has no unit_id column; the FK is on the unit).
- A unit links to 0 or 1 topic (the unique constraint).
- Together: 1:1.

**What product actually wants** (in plan 049 framing):
- Bridge HQ's library has 12 canonical units.
- 100 different schools each want to USE those 12 units in their own courses.
- A school may swap one for a custom variant, reorder them, skip OOP, add a math-stats unit, etc.

The 1:1 makes this impossible without cloning. Cloning produces drift.

**Three alternative shapes:**

**(A) Many-to-many with a join table.**
- Drop `teaching_units.topic_id` and the partial-unique index.
- New table `topic_units(topic_id, unit_id, sort_order)` with PK on `(topic_id, unit_id)`.
- A unit can appear in many topics across many courses.
- Trade-off: every existing query that says "this unit's topic is..." needs to handle 0..N topics. The "presentation order within a topic" may need to be in the join row, not the unit.

**(B) Unit lineage (canonical + clones, with content sharing).**
- Keep 1:1.
- Adopting a unit creates a CLONE of the canonical unit.
- The clone has a `parent_unit_id` (or the existing `forked_from` column extended to teaching_units).
- The clone's `unit_documents.blocks` is read-through to the parent UNLESS the clone has overlay rows that override specific blocks.
- A clone can pin a parent version (`parent_revision_id`) so it doesn't auto-update.
- Trade-off: the picker becomes "search across canonicals + clones," and the rendering layer needs to resolve the overlay chain at read time.

**(C) Hybrid: M:N for "use this unit," 1:1 for "I edited this unit's content."**
- A unit can be referenced (M:N) by many topics — display-only.
- If a teacher wants to EDIT the content for their topic, they fork a clone (1:1 owned by their topic).
- The fork has a parent reference; until forked, the topic is just a viewer of the canonical.
- Trade-off: two-tier UX ("attach unit" vs "fork unit") that teachers may find confusing.

Each option has a different blast radius on existing code:
- (A) requires migration, picker rewrite, GetSessionTopics rewrite, all topic→unit JOIN sites.
- (B) requires a clone-on-attach mechanism in `LinkUnitToTopic`, an overlay resolution layer at read time, version pinning UX.
- (C) requires (A)'s changes for the read path PLUS (B)'s changes for the fork path.

**Open question for product:** Is content reuse "I want to use the same content" (option A) or "I want to start from this content and customize" (option B)? The answer to that determines the schema shape.

### Problem 2: Teaching unit groups (the "book" concept)

**Status today:** there's no entity between `courses` (which is org-scoped, has topics, has classes) and individual `teaching_units`. A teacher's mental model "Python 101 is a book with 12 chapters" has no schema home.

**What a "book" / "unit group" / "course-template" entity should bring:**

- **Bundle metadata:** title ("Python 101"), description, grade band, author/publisher, license, edition number (`edition: "2026 Spring"`). One source of truth for "what is this multi-unit thing called?"
- **Bundle versioning:** "Python 101 edition 2 includes the new generators unit and rewrites the OOP unit." Adopting orgs decide which edition to track.
- **Bundle visibility:** publish/unpublish the whole group atomically. Today publishing 12 units one-by-one is error-prone.
- **Bundle adoption:** "Clone Python 101 into my org" produces a new course populated with the right topics + units in the right order. Today that's a manual 12-step process.

**Three shapes:**

**(α) `unit_collections`** (already exists in the schema, see `unit_collections` table — partial implementation per plan 030 or thereabouts). Extend with an `is_template: bool`, ordering, and a "publish as course template" affordance.

**(β) New `course_templates` table.** Distinct from `unit_collections`. A course_template has an ordered list of unit references and metadata. Adopting an org instantiates a `courses` row from the template.

**(γ) `courses` itself becomes the template.** A course with a special `is_template: true` flag, owned by Bridge HQ org, becomes the canonical template. Adopting orgs use `CloneCourse` (already exists). The template is just a course that's marked as such.

(γ) is the lowest-friction option but conflates two concepts (a runnable course vs. a curriculum template). (α) reuses an existing partial entity. (β) is the most explicit but adds a new table.

**Open question for product:** is a "book" a special kind of course, or its own entity? If product copy says "the Python 101 book" alongside "the 9th-grade Python 101 class at Springfield High," they probably want different entities — the book is durable, the class is ephemeral.

### Problem 3: Overlay & versioning

**Status today:**
- `unit_overlays` table (`drizzle/0018_unit_overlays.sql`) — stores overlay records keyed on `(parent_unit_id, child_unit_id)` with overlay JSON.
- `unit_revisions` table — snapshots taken on status transitions per plan 033/034.
- Some store helpers in `platform/internal/store/teaching_units.go`.
- **No UI**, no clear semantics for "when does an overlay vs a fork happen," no version pinning at the consumer side, no diff/merge UX.

**Use cases the architecture must answer:**

1. **Teacher-level customization.** "Bridge HQ's Loops unit is great but I want to change the example to use my school's mascot." Should this be a fork (full copy) or an overlay (reference + overrides)?
2. **Bridge HQ-level update propagation.** "We added a section about `enumerate()` to Loops." Do existing classes auto-pick it up, or do they pin to the version they started with?
3. **Mid-semester drift safety.** "A teacher's class is mid-Python 101 when Bridge HQ pushes a Loops update that breaks the test cases for one of the homework problems." What guarantees does the platform make about content stability during a class?
4. **Diff & review.** "Show me what changed between v3 and v4 of the Loops unit." Supported by `unit_revisions` snapshots, but no UI surfaces this today.
5. **Author overlays at the org level.** "All Springfield High classes should use OUR overlay of Bridge HQ Python 101, with our school's branding." Per-class overlays don't compose well; per-org overlays would.

**Three shapes:**

**(I) Overlays-as-deltas.** Each consumer (org / class) has zero-or-more `unit_overlay` rows that store JSON-patch-style diffs against the canonical. Read time = parent + apply all matching overlays in priority order.
- Trade-off: complex diff/merge UX. Conflict resolution semantics need design.

**(II) Versioning + pinning, no overlays.** Every consumer (course / topic) pins a specific `unit_revision_id`. Updates to the canonical don't auto-propagate; the consumer manually upgrades. Customization is via cloning.
- Trade-off: no shared partial customization (pure-clone diverges fully). But mid-semester drift is impossible by construction.

**(III) Both — overlays for partial customization, version pinning for stability.** A consumer is pinned to a specific revision AND can apply a per-org overlay on top.
- Trade-off: most complex. Most expressive. Requires both UIs.

**Open question for product:** is content drift during a class a real concern (option II / III) or is auto-update the desired default (option I)? The answer drives the schema.

## What this plan PRODUCES

Not code. Not a migration. Not a YAML format. **A decision document.**

Specifically, after this plan completes, we have:

1. A user-confirmed answer to each open question above.
2. A chosen shape for each of the three problems.
3. A migration sketch (no SQL written yet) that gets us from today's schema to the chosen shapes.
4. A list of which existing code surfaces need touching (LinkUnitToTopic, picker dialog, GetSessionTopics, CloneCourse, the entire content-rendering layer).
5. A list of existing plans that need amending (044's 1:1 invariant, 045's picker semantics, 049's library framing, etc.).
6. A new execution plan number assigned to the schema rewrite (likely **plan 052**), with phased rollout.

## Workflow

This is **review-driven, not code-driven.**

### Phase 0: Survey existing partial implementations

Document what's already in the codebase but not exposed:
- `unit_overlays` table schema + any store helpers
- `unit_revisions` table schema + lifecycle transition snapshots
- `unit_collections` table — what it does today, who uses it
- `forked_from` column on `problems` — how problem lineage works today
- The picker auth gate's handling of "Already linked" state (plan 045)
- Any in-flight or partial work in `platform/internal/store/teaching_units.go` referencing fork/overlay

Output: `docs/plans/051-teaching-unit-architecture-review-survey.md` — pure inventory.

### Phase 1: Stakeholder interview

Single-author? Multiple-author? The user's product mental model on the three open questions:

- Reuse semantics (Problem 1): same content vs. start-from-this-content?
- Book entity (Problem 2): special course vs. distinct entity?
- Drift policy (Problem 3): auto-update vs. version-pin?

Could be done in this chat session, with the user responding to each open question. Output: a "Decisions" subsection appended to this plan.

### Phase 2: Migration sketch

For each problem, produce a concrete schema-change proposal:
- New / modified tables, with column lists
- Migration steps (data backfill, dual-write windows if needed)
- Read-path changes (which queries touch which tables)
- Rollback story

Output: `docs/plans/051-migration-sketch.md`.

### Phase 3: Existing-plan reconciliation

List every previously-written plan that makes assumptions invalidated by the new design:
- Plan 044: 1:1 unit↔topic — likely invalidated.
- Plan 045: picker auth widening + "Already linked" — needs new gate.
- Plan 048: session_topics snapshot — may need to snapshot units AND their pinned revisions.
- Plan 049: Python 101 as platform-scope library — re-frame as a "book" / unit-group.

For each: write a short note in the affected plan's post-execution report (or a new "Superseded by plan 052" header).

### Phase 4: Execution plan creation

Spin up the new execution plan (likely numbered 052) with phased migration. This plan 051 ends here; plan 052 takes over for code.

## Out of scope

- **Code.** This plan ships zero lines of code. Implementation is plan 052+.
- **Curriculum content.** Plan 049 ships content under TODAY's schema; plan 051's outcome may require re-importing under a new schema, but that's a plan 049b / 052 concern.
- **Problem schema.** This review focuses on teaching units. Problems have their own design tensions (e.g., `forked_from` vs. shared starter code) but not in scope here.
- **AI tutor integration.** The AI tutor consumes whatever the schema produces; reshaping the unit schema doesn't redesign the tutor.
- **Performance.** The chosen shape MUST not regress the existing read paths to a meaningful degree, but this plan doesn't benchmark — that's plan 052's verification work.

## Open questions for user (Phase 1 stakeholder interview)

These are the questions the user answers before this plan moves forward:

1. **Reuse semantics for unit content.** When 100 schools adopt Python 101's "Loops" unit, do they all see the SAME bytes (with overlays for their own customizations) or each get their OWN COPY they can edit independently?
2. **Book entity.** Is "Python 101" a curriculum book that's distinct from a runnable course, or are they the same thing?
3. **Drift policy.** When Bridge HQ updates a unit mid-semester, do existing classes pick it up automatically or stay pinned to the version they started?
4. **Authoring scale.** How many concurrent unit-authors are realistic? (Solo curriculum lead vs. distributed authoring across schools changes the locking story.)
5. **Multi-language.** Will the same unit ever exist in multiple natural languages (English / Spanish / Mandarin)? If yes, that interacts with versioning — translations are a kind of overlay.

## Status

**This plan is INTENTIONALLY a placeholder.** No implementation begins until:
- Phase 0 survey complete.
- Phase 1 user-confirmed answers to the open questions.
- Phase 2 migration sketch.
- Phase 3 existing-plan reconciliation.
- A new plan 052 (or whatever number) is created with the agreed schema rewrite as an execution plan.

## Codex Review

(Pending — when this plan moves from placeholder to actionable, dispatch Codex per CLAUDE.md gate.)
