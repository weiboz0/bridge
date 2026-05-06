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

(pending)

## Codex findings

(pending)

## DeepSeek V4 Pro findings

(pending)

## GLM 5.1 findings

(pending)
