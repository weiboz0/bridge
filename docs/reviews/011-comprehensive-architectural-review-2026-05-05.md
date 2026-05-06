# Comprehensive Architectural Review — 2026-05-05

> **Status**: charter committed; reviewer findings pending.
> **Reviewers**: Self (Opus 4.7), Codex, DeepSeek V4 Pro, GLM 5.1 (via opencode).
> **Scope**: cross-cutting architectural assessment, not per-PR code review.
> **Deliverable**: this file, with each reviewer's findings appended in their own section.

## Why now

Bridge has shipped ~70 numbered plans. Recent cycles built load-bearing primitives (Go-owned auth, parent-link schema + UIs, org-admin write surface, plan-driven 4-way review gate, subagent-first coding policy). Before the next big cycle starts, we want a holistic critique that surfaces:

- **Cross-cutting drift** that per-PR reviews miss because each only sees its diff.
- **Pattern inconsistencies** between recently-shipped subsystems (e.g., parent-link modal vs. org-settings form vs. invite modal).
- **Foundational debt** that's quietly compounding — auth gates, error handling, data-fetching shape, type-system rigor.
- **Risks that the per-PR Codex/DeepSeek/GLM gate keeps missing** because the lens is too narrow.
- **Strategic decisions needing review** before they ossify (e.g., dual-tier coding agent policy, single-PR-per-plan, opencode model identifiers).

The intent is NOT a comprehensive list of bugs — those belong in PR review. It's an architectural read: where the codebase is solid, where it's fragile, what to invest in next.

## Codebase shape (snapshot for reviewer context)

- **Frontend**: Next.js 16 App Router, React 19, TypeScript, shadcn/ui, Monaco, Yjs. ~277 .ts/.tsx files in `src/`. Routes under `src/app/`: 5 portals (`admin`, `org`, `parent`, `student`, `teacher`) + `(auth)` group + `onboarding` + design previews.
- **Backend**: Go (chi router, pgx, Drizzle for migrations only). 12 internal packages: `auth, config, db, events, handlers, llm, overlay, projection, sandbox, skills, store, tools`. ~93 non-test .go files + ~100 `_test.go` files.
- **Realtime**: Hocuspocus server (`server/hocuspocus.ts`) for Yjs collaboration; tokens minted by Go (`/api/realtime/token`).
- **Data**: Postgres via pgx (Go) + Drizzle migrations (25 files). Application code is in `platform/internal/store/*.go` (no ORM); tests use a `bridge_test` database.
- **Testing**: Vitest unit/integration (~86 tests) + Go test suite (~100 test files) + Playwright e2e (`e2e/`).
- **Recent strategic shifts**:
  - Plan 065 — Go now owns auth verification (Next.js delegates).
  - Plan 067 — sectioned sidebar replaces role-switcher.
  - Plan 068 — runtime guards (BRIDGE_HOST_EXPOSURE, identity-drift banner, schema-probe).
  - Plans 069 + 070 — org-admin write surface + parent-link UIs.
  - Plan-driven workflow with mandatory 4-way review gate (CLAUDE.md, plan 070 phase 3 onward).
  - Subagent-first coding policy (PR #125): orchestrator on Opus, implementation on Sonnet subagents.
  - Single-PR-per-plan (PR #129): all phases of a plan land on one branch + one PR.

## Review charter — what to look at

Reviewers should focus on architectural-level concerns, not line-level bugs (those belong in PR review). For each section, return findings tagged with **severity** (blocker / important / nit) and **category** (drift / debt / risk / strategic).

### 1. Auth & authorization architecture

- **Plan 065 outcome**: Go owns auth verification, Next.js delegates via `/api/me/identity`. Is the integration clean? Are there remaining Next-side identity decisions that should move to Go?
- **Per-handler auth gates**: Bridge uses `RequireClassAuthority(ctx, classID, level)` for class-scoped reads (plan 052). The org-admin endpoints use ad-hoc `GetUserRolesInOrg` checks (e.g., `orgs.go:295`, repeated). Is the inconsistency a real risk, or acceptable given the different access models? Should there be a `RequireOrgAuthority(orgID, level)` parallel?
- **Self-action guards**: plan 069 phase 4 added `UpdateMember` / `RemoveMember` self-action checks. Are there other self-action paths still unguarded (e.g., parent-link revoke in plan 070, teacher self-removal from a class, etc.)?
- **Impersonation pathway** (`bridge-impersonate` cookie): used by plan 039+. Is the platform-admin-impersonating-user model documented enough that handlers don't accidentally trust the wrong identity?
- **Token surface**: Go-minted JWT (`BRIDGE_SESSION_SECRETS`) + Yjs realtime tokens (`/api/realtime/token`) + JWE Auth.js session cookie. Three token types — any redundancy, blind spots, or rotation drift?

### 2. Data layer & schema

- **Migrations workflow**: Drizzle for migration files; plan 068 phase 3 established the `to_regclass` schema-probe to validate the schema state at boot. Is the probe robust? Are there other invariants the boot check should enforce (e.g., enum values, FK presence)?
- **Store layer patterns**: `platform/internal/store/*.go` has no ORM. Hand-written SQL, sometimes with inline JOINs (plan 069 phase 2 added `GetClass` JOIN, plan 070 phase 2 has `ListByOrg` LATERAL join). Any patterns worth promoting to a shared helper, or duplicated logic that should DRY?
- **Soft-delete vs. hard-delete inconsistency**: `parent_links` uses `status='revoked'` soft-revoke; `class_memberships` has no status column (DELETE only); `org_memberships` has `pending/active/suspended`; `classes` has `active/archived`; `organizations` has `pending/active/suspended`; `users` is hard-delete. Is the inconsistency intentional or accumulated?
- **Cross-table integrity**: parent_links references users; org_memberships references users; class_memberships references both. Plan 070 introduced an org_memberships upsert tied to parent_link creation. Are there other cross-table invariants the schema doesn't enforce that should be in app code?

### 3. Frontend architecture

- **Modal/dialog pattern fragmentation**: there's no shared `<Dialog>` component. Hand-rolled modals appear in `create-parent-link-modal`, `invite-member-modal`, `update-status-modal` (member-row-actions), `remove-dialog`, plus `confirm()` calls in `archive-class-button`. Is the pattern divergence a real maintainability cost, or acceptable for v1?
- **Server-component pattern**: pages do server-side fetches in `page.tsx`, then pass to client components for interactivity. Some pages (`/org/parent-links/page.tsx`, `/org/teachers/page.tsx`) resolve `orgId` server-side via `resolveOrgIdServerSide`; others (`/org/courses/page.tsx`, etc.) still use the legacy `?orgId=` query fallback. Is the inconsistency a problem?
- **Form pattern**: no react-hook-form / formik / zod. Hand-rolled `useState` + `onSubmit` (decisions §2 in multiple plans). At ~12 forms now, is this still the right call?
- **Type discipline**: TypeScript baseline is 10 pre-existing errors that we treat as "noise" and don't fix. Are these errors load-bearing (i.e., do they hide real bugs)? Should there be a "tsc baseline must trend down" policy?
- **Test coverage shape**: 86 Vitest files; some load-bearing components have zero tests (Monaco wiring, Yjs provider, hocuspocus connection). Is the coverage gap concentrated in the right places?

### 4. Testing strategy

- **Three test surfaces**: Vitest, Go integration, Playwright e2e. Are the boundaries between them clear? Are integration concerns being tested at the wrong layer?
- **Test-vs-implementation tier split**: new policy says test code defaults to Sonnet. Is the test-quality bar holding? Are subagents producing tests that miss edge cases the orchestrator would catch?
- **Schema-probe + integration db**: Go integration tests use `bridge_test` (drizzle-migrated). Vitest integration tests share the same DB. Is contention a real risk on parallel runs?
- **Flaky-test surface**: plan 069 hit a test flake (collision-prone hardcoded email in `TestProblemStore_CreateProblem_SlugAllowedInDifferentScope`). Are there other tests with similar patterns waiting to bite?

### 5. Plan-driven workflow & review gate

- **4-way review gate**: every plan + every PR runs self/Codex/DeepSeek/GLM. Two real BLOCKERS were caught (clipboard guard in #128, archived-class filter in #124) that per-PR self-review missed. Is the gate's marginal value holding up, or are the external reviews mostly noise?
- **Subagent-first policy**: orchestrator stays on Opus, implementation on Sonnet subagents. Has the orchestrator's verify-the-actual-changes step been catching real divergence between subagent reports and reality?
- **Single-PR-per-plan policy** (PR #129): just adopted. Plan 069 phases 2-4 was the first multi-phase PR. Is the resulting PR size reviewable, or does the consolidated diff overload reviewers?
- **Plan file as audit trail**: the `## Plan Review` and `## Code Review` sections in plan files now carry verdicts + rejections. Is this audit trail the right durable surface, or should verdicts go elsewhere (e.g., a separate `docs/reviews/` per-plan file)?
- **Reviewer-disagreement protocol**: when GLM and Codex returned different verdicts on plan 069 phase 5 (DeepSeek needs-changes vs. Codex CONCUR vs. GLM approved-with-notes), the orchestrator made the call. Is the resolution heuristic clear enough to scale, or does it need explicit doc?

### 6. Deferred / acknowledged items

The following were deferred from recent plan reviews and would benefit from a strategic look:
- Header count divergence (active-only stats vs. all-status list counts) — plan 069 Phase 4
- Modal pattern unification (introduce shared `<Dialog>`) — plan 069 + 070
- `resolveOrgIdServerSide` discriminated-union return — plan 069 Phase 4
- Self-action backend hardening for parent-link write paths — parallel to plan 069's pattern
- Token-based parent-claim flow (plan 070b) — product fork
- Email-token org invites (plan 069b) — product fork
- Role-update endpoint for member quick-actions

Are these the right deferrals? Any of them quietly higher-priority than current work?

### 7. Strategic decisions to validate

- **Go owns auth verification (plan 065)**: still the right call after 7 months, or is the dual-stack maintenance burden growing?
- **Two-tier coding agent (Sonnet default + Opus for hard work)**: surprised by anything? Cases where the tier split obviously wrong?
- **Single-PR-per-plan**: does it scale to 10+ phase plans?
- **External-reviewer set (Codex + DeepSeek + GLM)**: is the diversity actually adding signal, or are we paying 3x for marginal benefit?
- **Plan-driven workflow**: ~70 plans shipped. Is the overhead of plan-write → 4-way-review → implement → 4-way-review still net-positive, or is it slowing critical path on small features?

## Findings format (for each reviewer)

Append a section under `## <reviewer> findings` with:

```
### Section X.Y — <one-line summary>
**Severity**: blocker | important | nit
**Category**: drift | debt | risk | strategic

<2-4 paragraph finding with file:line references where applicable>

<recommendation: what to do, how big, when>
```

End with a 200-word executive summary at the top of your section. Don't repeat the charter; assume the reader has it.

---

## Self-review (Opus 4.7) findings

**Executive summary** (200 words): Bridge is in solid shape architecturally. The plan-driven workflow + 4-way review gate is paying off (real BLOCKERS caught in PRs #128, #124, #130 that per-PR self-review would have missed). Recent strategic shifts (Go owns auth, sectioned sidebar, single-PR-per-plan, subagent-first coding) are coherent. Three areas where I'd push hardest:

1. **Auth-gate inconsistency between class-scoped and org-scoped paths** is real architectural debt — `RequireClassAuthority(ctx, level)` exists for classes; org endpoints repeat ad-hoc `GetUserRolesInOrg` checks across ~6 handler functions. This is the highest-leverage refactor on the table.

2. **Modal pattern fragmentation** is becoming a real cost. Three plans in a row (066/069/070) hand-rolled their own modal scaffold. A shared `<Dialog>` primitive is overdue.

3. **Plan-file-as-audit-trail is showing strain.** Plan files now mix design, decisions, code review verdicts, post-execution reports, and reviewer push-back into one document. Plan 069's review section is ~500 lines of running commentary. A `docs/reviews/NNN-plan-XXX-review.md` separation would scale better.

Lower-priority items: TS baseline drift (10 errors persisting), test-DB contention risk on parallel runs, deferred items list growing without explicit triage cadence.

### Section 1.1 — Auth-gate inconsistency between class- and org-scoped endpoints
**Severity**: important
**Category**: drift

`platform/internal/handlers/access.go` defines `RequireClassAuthority(ctx, classes, orgs, claims, classID, level)` with three levels (`AccessRead/Roster/Mutate`) — a clean, reusable primitive. Class-scoped handlers across `classes.go`, `sessions.go`, `assignments.go` all use it.

Org-scoped handlers, in contrast, ad-hoc the same gate: `orgs.go::UpdateMember`, `RemoveMember`, `UpdateOrg`, plus `OrgParentLinksHandler::requireOrgAdmin` (plan 070), all repeat the same `GetUserRolesInOrg` lookup + role-membership scan. Plan 069 phase 4 added self-action guards to `UpdateMember` and `RemoveMember` — but only those two. `UpdateOrg` doesn't have a self-action guard (org admin can rename their own org's name, which is fine, but the pattern is asymmetric).

A `RequireOrgAuthority(ctx, orgs, claims, orgID, level)` parallel — with `OrgRead / OrgManage / OrgWrite` — would centralize the gate, make adding a self-action guard a one-line change in the helper, and drop ~80 lines of duplicated code across `orgs.go` + `org_parent_links.go`.

**Recommendation**: file plan 072 — `RequireOrgAuthority` helper. ~1 day of work, single-PR. Should land before plan 069b/070b add more org-admin endpoints.

### Section 1.2 — Self-action guards: incomplete coverage
**Severity**: important
**Category**: risk

Plan 069 phase 4 added self-action guards to `UpdateMember` (suspend) and `RemoveMember`. Codex's post-impl review (#130) explicitly called these out as v1-blocking. But the same risk exists in:
- **Parent-link revoke** (`platform/internal/handlers/org_parent_links.go::RevokeLink`): an org_admin parent of a child in the org could revoke their own parent_link. Less severe (loses /parent dashboard, not org access), but the asymmetry with the guarded paths is striking.
- **Class membership** (`classes.go::RemoveMember`): a teacher with `instructor` role can remove themselves from their own class via the API (UI gate exists). Backend has no protection.
- **Org admin removing the LAST org_admin**: more nuanced — would orphan the org. Backend has no enforcement.

**Recommendation**: as part of the §1.1 `RequireOrgAuthority` refactor, add a `requireNotSelfMutation(membership, claims)` and a `requireNotLastAdmin(orgID, role)` helper. Tackle the same time as the helper; ~+0.5 day.

### Section 2.1 — Soft-delete inconsistency across the schema
**Severity**: nit (today) → important (when product asks for "deactivated user" support)
**Category**: debt

Bridge's deletion model is a tangle:
- `parent_links`: soft-revoke (`status='revoked'` + `revoked_at`)
- `org_memberships`: status state machine (`pending/active/suspended`)
- `class_memberships`: hard-delete (DELETE on the row; no status column)
- `classes`: state machine (`active/archived`)
- `organizations`: state machine (`pending/active/suspended`)
- `users`: hard-delete (with FK CASCADE)
- `parent_links → users`: ON DELETE CASCADE (from drizzle/0024)
- `class_memberships → users`: ON DELETE CASCADE
- `org_memberships → users`: ON DELETE CASCADE

The inconsistency is accumulated, not designed. Currently fine — each table's choice has independent justification. But the moment product asks for "audit trail of removed students" or "GDPR-erasure user without losing assignment history", the schema will need the `class_memberships` soft-delete that doesn't exist, and migrations will compound.

**Recommendation**: don't migrate proactively. Document the model in `docs/schema-deletion-model.md` so future plans can decide intentionally instead of carrying assumptions. ~2 hours.

### Section 3.1 — Modal pattern fragmentation
**Severity**: important
**Category**: debt

Three modals shipped in the last 3 plans, all hand-rolled:
- `src/components/org/create-parent-link-modal.tsx` (plan 070)
- `src/components/org/invite-member-modal.tsx` (plan 069 phase 1)
- `src/components/org/member-row-actions.tsx` (plan 069 phase 4 — TWO modals: status + remove)

Each duplicates: backdrop ref, `e.target === backdropRef.current` dismissal, Escape key listener, `role="dialog" + aria-modal`, autoFocus on first input. The `confirm()` call in `archive-class-button.tsx` skips the pattern entirely (different visual treatment).

A shared `<Dialog>` primitive (e.g., wrapping shadcn's underlying Radix `<Dialog.Root>` if it's available, or a small ~50-line component) would:
- Cut ~30 lines per modal site → ~120 lines saved
- Surface a single place to fix the focus-trap NIT (deferred from plan 070 phase 2 + 3 reviews)
- Make the next modal a 5-minute write rather than another hand-roll

**Recommendation**: plan 073 — `<Dialog>` primitive + migrate the 4 existing call sites. ~0.5 day.

### Section 3.2 — Server-component data-fetching pattern inconsistency
**Severity**: nit
**Category**: drift

Some org-portal pages resolve `orgId` server-side via `resolveOrgIdServerSide` (helper introduced in plan 069 phase 4): `/org/teachers`, `/org/students`, `/org/parent-links`. Others still use the legacy `?orgId=` query fallback (`/org/courses`, `/org/classes`, `/org`, `/org/units`, `/org/settings`).

The split is accidental — pages that needed path-style API URLs (which require a real, non-empty orgId) got the helper; pages that only needed the query-fallback `?orgId=` didn't. Eventually all org-admin pages will need write surfaces with path-style URLs, at which point the inconsistency becomes more visible.

**Recommendation**: low-priority. Fold into plan 072 (RequireOrgAuthority) — when adding `RequireOrgAuthority`, also migrate all org-portal pages to `resolveOrgIdServerSide` for consistency. Single follow-up commit.

### Section 4.1 — Test database contention on parallel runs
**Severity**: important
**Category**: risk

Both Vitest (`bun run test`) and Go integration tests share `bridge_test`. CI runs them in separate jobs today, but a developer running both locally in parallel hits intermittent FK conflicts when one suite seeds users while another deletes them.

The flake from plan 069 (`TestProblemStore_CreateProblem_SlugAllowedInDifferentScope` collision-prone email) is a symptom of this — test isolation relies on per-test cleanup, but per-test cleanup races with other tests' SETUPs.

**Recommendation**: move toward per-test isolated schemas (`SET search_path = test_$pid`) or per-test transactions with rollback. ~2 days. Don't block on this — the contention is real but rare today. File as plan 074, low-priority.

### Section 5.1 — Plan-file-as-audit-trail is showing strain
**Severity**: important
**Category**: drift

Plan 069's `## Plan Review` + `## Code Review` sections together are ~500 lines (longer than the original spec). They mix:
- Pre-impl Codex passes 1-3 (legacy single-reviewer)
- Pre-impl 4-way review (self/Codex/DeepSeek/GLM)
- Per-phase post-impl review verdicts
- Reviewer push-back rejections with technical evidence
- Cross-phase consolidated PR review verdicts
- Per-fix re-dispatch confirmations

The result: when revisiting the plan to understand "what did we ship and why", the audit trail is signal but the review history is noise. New collaborators are likely to skip it.

**Recommendation**: separate the surfaces. Keep `## Plan Review` (decisions that altered the plan, kept as load-bearing context) inside the plan file. Move `## Code Review` to `docs/reviews/plan-NNN-code-review.md` with one section per PR. ~2 hours plus a one-time migration of the existing reviews. File as a chore PR.

### Section 7.1 — External-reviewer set: net-positive but signal-to-noise warrants tracking
**Severity**: nit (informational)
**Category**: strategic

Across the recent plans, the external-reviewer hit-rate is roughly:
- **Codex**: caught 3 real BLOCKERS (clipboard guard, archived-class filter, self-action backend guard). High signal.
- **DeepSeek V4 Flash**: caught 2 real concerns (dead 403 branch, AbortController). Moderate signal.
- **GLM 5.1**: caught 0 real BLOCKERS. Real signal: 1 NIT (StatusBadge dup), 1 NIT (conditional aria-controls). Reported 3+ false-premise BLOCKERS that were rejected with technical evidence (`cm.status` filter, `LEFT JOIN` for cascade FK, ArchiveClassButton body shape).

GLM is currently the lowest-signal reviewer. But: the FOUR-way diversity is what makes the ensemble robust. Removing GLM would re-introduce the blind-spot risk single-reviewer + dual-reviewer designs had.

**Recommendation**: keep the four-way for now. After 5 more plans, re-evaluate with concrete numbers. Track in `docs/review-stats.md`.



## Codex findings

**Executive summary**: Bridge's architectural direction is sound: Go-owned auth, explicit store-layer SQL, and plan-driven review are coherent choices. The weak spots are mostly incomplete cutovers and accumulated transition code. Highest-risk gap: realtime auth — Go JWT mint/recheck path exists, but Hocuspocus still accepts legacy `userId:role` tokens by default, with known forged-token and per-unit authorization holes documented in code comments. Auth has two parallel transition debts: org authority is reimplemented per-handler instead of centralized, and shadow Next API routes still contain stale direct-DB write logic under paths now proxied to Go. Data-layer risk is concentrated in schema validation: the startup probe confirms one table exists, but not constraints, indexes, enums, or non-table migrations. Frontend drift is visible in org-context resolution and modal patterns; both have already produced review findings and local fixes. Testing has breadth, but the E2E suite can silently skip core live-session and realtime auth coverage based on seed data or missing secrets. Finally, the workflow docs conflict on when PR creation happens relative to the 4-way review, which matters now that reviews run once against consolidated plan diffs.

### Section 1.1 — Realtime auth cutover is incomplete and still defaults to legacy tokens
**Severity**: blocker
**Category**: risk

Go now mints scoped realtime JWTs and rechecks access through `/api/internal/realtime/auth` (`platform/internal/handlers/realtime_token.go:77`, `:132`). Hocuspocus verifies those JWTs (`server/hocuspocus.ts:49`) and rechecks Go on load (`:174`). That path is the right architecture.

The problem is that Hocuspocus still accepts legacy `userId:role` tokens unless `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1` is set (`server/hocuspocus.ts:16`, `:25`, `:79`). The comments document the exposure: a forged legacy token could open session documents where `docOwner` matches the forged user id (`:97`, `:104`), and `unit:*` legacy auth checks only `role === "teacher"` without unit/org validation (`:144`, `:147`).

**Recommendation**: finish the plan 053 cutover as a production hard requirement. Make signed tokens required by default in production, fail Hocuspocus startup if the secret or require flag is absent, and delete the legacy auth branches after one compatibility release. Small scope, high safety return.

### Section 1.2 — Shadow Next write routes contain stale authorization logic
**Severity**: important
**Category**: drift

`next.config.ts` proxies `/api/orgs/:path*` to Go, but the repo still carries overlapping Next API routes with direct Drizzle writes and their own auth decisions (`src/app/api/orgs/[id]/members/route.ts:18, 25, 53`). The shadow Next PATCH/DELETE routes have no self-action guard (`src/app/api/orgs/[id]/members/[memberId]/route.ts:37, 65`), unlike the Go path that plan 069 phase 4 hardened.

**Recommendation**: delete or quarantine migrated Next route files in one cleanup plan. Until deletion, add a test that fails when proxied Go routes still have executable Next handlers under the same path.

### Section 1.3 — Org authority needs a shared helper to match the class authority pattern
**Severity**: important
**Category**: debt

(Confirms Self-review 1.1.) Class-scoped access has `RequireClassAuthority` (`access.go:17, 67`). Org-scoped access repeats role scans in every handler: `UpdateOrg`, `AddMember`, `UpdateMember`, `RemoveMember` each call `GetUserRolesInOrg` (`orgs.go:185, 282, 360, 441`). Parent links introduced a local `requireOrgAdmin` helper scoped only to that file (`org_parent_links.go:76, 88`) — better locally but confirms the missing abstraction.

**Recommendation**: add `RequireOrgAuthority(ctx, orgs, claims, orgID, level)` with read/admin levels and platform-admin override semantics. Migrate org dashboard, membership, courses/classes, parent-link handlers in the next auth-consolidation plan.

### Section 2.1 — Schema probe validates table presence, not schema integrity
**Severity**: important
**Category**: risk

The boot check verifies only `to_regclass('public.' + ExpectedSchemaProbe)` (`schema_probe.go:34, 36`). The migration docs explicitly acknowledge this cannot distinguish fully-migrated from partial (`migrations.go:13, 19`). `parent_links` relies on a status check, no-self-link check, FK actions, and a partial unique index (`drizzle/0024_parent_links.sql:20, 22, 35`). The current probe passes if the table exists but those constraints/indexes are missing.

**Recommendation**: keep `to_regclass`, but add a multi-sentinel probe for critical columns, constraints, enum values, and indexes. Before the next schema-affecting plan, not after.

### Section 3.1 — Org pages resolve org context via four different patterns
**Severity**: important
**Category**: drift

(Extends Self-review 3.2.) Teachers/students use `resolveOrgIdServerSide` (`org-context.ts:55, 61`). Classes/courses/settings still parse the query param and rely on endpoint fallback (`org/classes/page.tsx:14, 18`; `org/courses/page.tsx:14, 18`; `org/settings/page.tsx:22, 27`). Parent-links page hand-rolls its own fallback instead of using the helper (`org/parent-links/page.tsx:57, 59`).

**Recommendation**: make all org portal pages resolve one `OrgContext` object server-side: `{orgId, orgName, error}`. Replace scattered `parseOrgIdFromSearchParams` calls and stop swallowing all resolver errors as `null` (`org-context.ts:69`).

### Section 3.2 — Modal divergence generating repeated accessibility rework
**Severity**: important
**Category**: debt

(Confirms Self-review 3.1.) Multiple hand-rolled dialog implementations: invite modal (`invite-member-modal.tsx:137`), parent-link modal (`create-parent-link-modal.tsx:119`), status modal and remove dialog (`member-row-actions.tsx:90, 232`), native confirms for revoke/archive (`parent-links-view.tsx:50`, `archive-class-button.tsx:18`). Recent comments show repeated fixes for backdrop clicks, stale ARIA references, and clipboard/native-dialog edge cases — the same lifecycle and accessibility logic is being rediscovered per component.

**Recommendation**: introduce one shared `<Dialog>`/`<Confirm>` primitive and migrate org-admin modals first. Small frontend-foundation plan; should precede more org-admin write surfaces.

### Section 4.1 — E2E suite can pass while silently skipping realtime auth and live-session coverage
**Severity**: important
**Category**: risk

The realtime ratchet skips WebSocket auth entirely when `HOCUSPOCUS_TOKEN_SECRET` is unset (`hocuspocus-auth.spec.ts:26, 168`). HTTP mint tests also skip on missing seed units or 503 token config (`:117, 131, 159`). Core live-session E2E is data-dependent: missing class, existing active session, missing enrollment, absent past sessions all become skips (`session-flow.spec.ts:45, 69, 94, 159`). With Playwright serialized to one worker (`playwright.config.ts:6, 9`), the suite is stable but not a reliable regression gate for the features most likely to break on auth changes.

**Recommendation**: create deterministic E2E fixture setup for one teacher, student, class, unit, realtime secret. Convert these conditional skips into hard failures in CI for the auth, realtime, and live-session projects.

### Section 5.1 — Workflow docs conflict on PR timing relative to 4-way code review
**Severity**: important
**Category**: strategic

`docs/development-workflow.md:80` says the 4-way code review fires at PR-open time against the consolidated branch diff. But Step 6 says push and create the PR after Step 5 review (`:91, 99`). That sequencing is impossible as written. Single-PR-per-plan makes the review artifact larger and later (`:45, 55`); the contradiction matters more now.

**Recommendation**: amend the workflow now — Step 5 should open a draft PR, run 4-way review, resolve findings, then Step 6 marks it ready and merges. Add an explicit disagreement-resolution rule while editing the same section.



## DeepSeek V4 Pro findings

(pending)

## GLM 5.1 findings

**INFRASTRUCTURE UNAVAILABLE (this pass)** — both attempts timed out at the opencode 300s SIGTERM hard limit. First call used the full-charter URL fetch + 1500-word target; second call used a condensed inline brief + 800-word target. Both hit the same timeout window with zero output.

The volcengine endpoint hosting `volcengine-plan/glm-5.1` appears to need more time than opencode allots for architectural-review-sized reasoning. Per-PR code reviews (smaller scope, quicker output) succeeded on this same model in prior plans (069/070), so the model itself is reachable — just not within the 300s envelope on this prompt class.

**Substitution / mitigation options** for future architectural passes:
1. Split the architectural review into per-section calls (one charter section per call) — gives each model a 300s window for a small slice instead of the whole charter.
2. Configure opencode timeout extension if the runtime supports it (out of scope for this pass — would need a config change in the buddy plugin).
3. Accept that volcengine models work for per-PR reviews but not architectural reviews; document the constraint in CLAUDE.md.

**For this review**: the 4-way protocol is reduced to 3-way (self/Codex/DeepSeek). Re-running GLM 5.1 against the published findings (when convergence is established) would be a useful sanity-check follow-up but is not blocking the action items.


