# Plan 057 — Python 101 smoke-test checklist (sub-doc)

> Sub-document of `docs/plans/057-python-101-post-ship-validation.md`.
> Manual browser smokes deferred from plan 049's "Open follow-ups."
> Each row gets ticked when run, with a one-line result and date.

## Smoke A — Different-org teacher's view of Python 101 in the picker

**Setup**

1. Run `bun run db:setup` (clean DB) → seed Bridge HQ + Python 101 importer.
2. Create a second org (e.g., "Test School #2") with one teacher (`teacher2@example.com`).
3. Sign in as `teacher2`.

**Steps + expected**

- [ ] Open the Unit picker (e.g., from a topic edit page). Filter for "Python 101"
  units. Each platform-scope unit must show:
  - status badge: `classroom_ready`
  - link state: `Already linked` (because plan 044's 1:1 invariant claims them
    for Bridge HQ topics).
- [ ] No platform-scope unit is browseable AS the source for forking unless via
  the explicit "Browse library" path.
- [ ] "Linkable for course" filter excludes platform-scope units already linked.

**Result**: _(fill in date + outcome)_

---

## Smoke B — Eve walks a student through a Python 101 problem

**Setup**

1. Same seeded state as Smoke A.
2. Sign in as `eve@demo.edu` in one browser; `alice@demo.edu` (student) in another.
3. Confirm `Python 101 · Period 3` is wired (per `--wire-demo-class` flag).

**Steps + expected**

- [ ] Eve opens `Python 101 · Period 3` and starts a session.
- [ ] Alice joins the session. Alice can see the unit content (post-plan-061 +
  plan-062 fix).
- [ ] Eve assigns a Python problem. Alice opens the problem editor, runs code,
  and submits. Output appears in the terminal panel.
- [ ] Eve sees Alice's attempt in real time via the teacher dashboard.
- [ ] Help-queue: Alice raises hand. Eve sees the entry.

**Result**: _(fill in date + outcome)_

---

## Smoke C — Phase 5 picker discovery (different-org teacher view)

Verifies plan 049 Phase 5 step 6 — the importer's claim of platform-scope
units doesn't accidentally surface partial / unowned units to other orgs.

**Setup**

1. Same seeded state as Smoke A.
2. Sign in as `teacher2` (different org from Bridge HQ).

**Steps + expected**

- [ ] Open the platform library browser. Python 101 units are visible as
  reference content (status `classroom_ready`).
- [ ] Each unit's "linked topic" badge points to a Bridge HQ topic, not
  teacher2's org topic.
- [ ] Attempting to link a platform-scope unit to teacher2's topic fails with
  "already linked" (plan 044 invariant). Forking is the supported flow.

**Result**: _(fill in date + outcome)_

---

## Outcomes

When a smoke surfaces a bug:

1. File the bug as its own plan (number > 057).
2. Reference it from this checklist row.
3. Re-run the smoke after the fix lands.
