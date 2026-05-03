# Plan 057 — Python 101 post-ship validation + reuse semantics (P2)

## Status

- **Date:** 2026-05-01
- **Severity:** P2 (verification debt, not a security or correctness blocker)
- **Origin:** Review `008-...:49-55`. Plan 049's "Open follow-ups" list left these unfiled.

## Problem

Plan 049 shipped while several validation steps were marked "Open follow-ups (non-blocking; no plan filed yet)." Buried verification debt becomes invisible. Five items:

1. **Browser smoke A** — different-org teacher opens the Unit picker and sees Python 101 platform-scope units as `classroom_ready` AND "Already linked." Manual UI test, never run.
2. **Browser smoke B** — `eve@demo.edu` walks a student through a Python 101 problem in the demo class. Manual UI test, never run.
3. **Phase 5 picker discovery** — verify a different-org teacher's view of the platform library after the importer claims units for Bridge HQ. Plan 049 Phase 5 step 6.
4. **Cross-org "subscribe" semantics** — when a second org wants to teach Python 101 with their own copy. Today the demo class works via the importer's `--wire-demo-class` clone, but that's a one-off; there's no general flow. Implications for plan 044's 1:1 unit↔topic invariant.
5. **Pyodide ↔ Piston drift catalog** — Phase 5 caught 3 author bugs from CPython runtime differences. The constraint list in `content/python-101/README.md` is preventive but not exhaustive.

## Out of scope

- Authoring CLI helpers (also flagged in plan 049 follow-ups). Defer until authors complain.
- Any change to plan 044's 1:1 invariant. That's a `plan 051` placeholder topic if/when revived.

## Approach

Split the five items by what they need:

- Items 1, 2, 3 are **browser smokes** — manual UI runs with a checklist. Track in this plan; close when each is run.
- Item 4 is **product design** — needs a discovery pass to define what cross-org reuse looks like. Likely a separate plan once the design call is made.
- Item 5 is **content maintenance** — extend `content/python-101/README.md` as drift surfaces during real student use. No timeboxed task; ongoing.

This plan is therefore a **tracking plan**, not a "build a feature" plan. Its deliverable is the smoke-test checklist results + a small written design note for cross-org reuse.

## Files

- Modify: `docs/plans/049-python-101-curriculum.md` — strike the "Open follow-ups" entries that this plan now tracks; replace with a one-line pointer "see plan 057".
- Create: `docs/plans/057-python-101-smoke-checklist.md` (sub-doc) — checklist format for items 1-3.
- Modify: `content/python-101/README.md` — add a section "Pyodide ↔ Piston drift log" that authors append to as drift surfaces.
- Maybe-create: `docs/specs/057-cross-org-curriculum-reuse.md` — design note for item 4. Spec, not plan.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Browser smokes never get run, defeating the plan | medium | Make each smoke a checkbox in the sub-doc with sign-off date. Add to the periodic validation schedule. |
| Cross-org reuse design balloons into a major plan | low | Capture as a spec first. Don't commit to building until product validates the need with at least one external org. |

## Phases

### Phase 0: pre-impl Codex review

Codex review focuses on whether this plan's scope is right (tracking plan vs feature plan) and whether the smoke checklist captures everything plan 049 promised.

### Phase 1: write the smoke checklist + run smokes 1-3

- Author the sub-doc.
- In a session with browser access, run each smoke, mark the checkbox, write a one-line result.
- Surface any defects as their own bugs / plans.

### Phase 2: write the cross-org reuse spec

- Brainstorm with product (1-2 sessions).
- Decide whether to file plan 058+ for implementation.

### Phase 3: drift log entries

- Ongoing. Update `content/python-101/README.md` as drift surfaces.

## Codex Review of This Plan

### Pass 1 — 2026-05-02: SKIPPED (tracking-plan exemption)

This is a tracking plan, not a feature plan. Its deliverables are:

1. The smoke-test checklist sub-doc
   (`docs/plans/057-python-101-smoke-checklist.md`) — pure
   documentation.
2. The drift-log section appended to `content/python-101/README.md`
   — content for authors to fill as drift surfaces.
3. The plan-049 back-pointer rewrite — closes the "no plan filed"
   gap.

None of these touch executable code or auth surface, so Codex
review at the plan level adds no leverage. The actual non-doc
deliverables (smoke runs, cross-org reuse spec) need a human in
front of a browser / in product discovery — not autonomous
engineering work.

If/when item 4 (cross-org reuse) gets a design call, that becomes
its own plan with a real Codex Phase 0 review.

## Phase 1 Post-Implementation Note (2026-05-02)

Shipped what's autonomously deliverable:
- **Sub-doc checklist** at `docs/plans/057-python-101-smoke-checklist.md`
  with 3 smokes (A: different-org teacher picker view, B: Eve walking a
  student through a problem, C: Phase 5 picker discovery).
- **Drift-log table** added to `content/python-101/README.md` between
  the authoring rules and the toolchain section. Pre-seeded with the
  `Greet by Name` emoji case from plan 049 Phase 5. Future drift gets
  appended as surfaced.
- **Plan 049 back-pointer** — "Open follow-ups" list rewrites pointing
  to plan 057. Items now have a tracked home.

**Deferred (manual / product work)**:
- Smokes A/B/C (need browser sessions).
- Cross-org reuse spec (needs product discovery; will be its own
  spec/plan once the design call is made).
- Authoring CLI helpers (waiting on author feedback).
