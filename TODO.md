# TODO

Outstanding tasks and technical debt. Check this file when planning new work.

## Technical Debt

- [ ] **Next.js middleware deprecation** — `middleware.ts` convention is deprecated in Next.js 16, should migrate to `proxy` pattern
- [ ] **Pyodide CDN dependency** — Web Worker loads Pyodide from CDN; should consider self-hosting or bundling for reliability
- [x] ~~**Hocuspocus auth** — Token format is simple `userId:role` string; should use JWT or signed tokens in production. **Tracked by plan 053.**~~ (closed by plan 072)
- [ ] **Database migrations** — Using direct SQL apply (`psql -f`); drizzle-kit migrate has issues, needs investigation. Only journal entries 0000-0002 exist; 0003+ are hand-applied.
- [ ] **/login redirect loop** — Auth.js v5 + Next 16 combo causes `/login` to 302 to `/login?callbackUrl=/login`, blocking Playwright auth.setup. Likely related to the middleware-deprecation entry above.

## Open security/correctness plans

Filed but pending implementation (each requires Phase 0 Codex review per CLAUDE.md):

- **Plan 050** — Auth correctness: refresh JWT claims on every `jwt()` call + `DEV_SKIP_AUTH` production guard.
- **Plan 052** — Resource access authorization audit: sweep all unauthorized-by-id Go handlers (class, topic, schedule, assignment, unit-collection) under one helper pattern. **P0**.
- **Plan 053** — Hocuspocus signed tokens + per-scope authorization. **P0**.
- **Plan 054** — Schema drift cleanup: drop `documents.classroom_id`, declare three missing `teaching_units` indexes, fix `createUnit` `materialType` payload.
- **Plan 055** — Delete shadow Next class-members API routes (depends on plan 052).
- **Plan 056** — Annotation document access enforcement.
- **Plan 057** — Python 101 post-ship validation tracking (browser smokes + cross-org reuse design).
- **Plan 059** — User store correctness: surface `intended_role` + transactional `RegisterUser`. **P0**.
- **Plan 060** — Remove `UpdateOrgStatus` auto-verify.
- **Plan 061** — Widen `canViewUnit` for students with verified class binding (unblocks Python 101 student flow).
- **Plan 062** — `GetProjectedDocument` use composed overlay content (forked-unit student render).
- **Plan 063** — Session SSE + help-queue authorization (HTTP/SSE counterpart to plan 053's WebSocket fix).

## Process / dev-experience tidy-ups (from review 009-2026-04-30)

Smaller items not worth their own plan; pick up when in the area:

- [ ] **OAuth port mismatch in setup.md** (P2-1) — `docs/setup.md:103` instructs `localhost:3000` but app runs on 3003.
- [ ] **`CodeEditor` calls `setupMonaco()` at module scope during SSR** (P2-2) — `src/components/editor/code-editor.tsx:11`. Guard with `typeof window !== "undefined"` or move into a `useEffect`.
- [ ] **`useYjsProvider` returns refs → first render with stale data** (P2-3) — `src/lib/yjs/use-yjs-provider.ts:27-41`. Switch to `useSyncExternalStore` or `useState`.
- [ ] **`useYjsProvider` provider-lifecycle race on document name change** (P2-4) — `src/lib/yjs/use-yjs-provider.ts:35-78`. Serialize provider creation, or use a key-based remount on the consuming component.
- [ ] **Vestigial AI interactions auth bypass** (P2-5) — `src/app/api/ai/interactions/route.ts:12` has `if (false /* TODO */)`. Either implement or delete; Go owns the proxy.
- [ ] **In-memory SSE event bus** (P2-6) — `src/lib/sse.ts:3-31` won't scale beyond one Next pod. Replace with Redis pub/sub or document the limitation.
- [ ] **Student session SSE uses `window.location.href`** (P2-7) — `src/components/session/student/student-session.tsx:86-88`. Use `router.push(returnPath)`.
- [ ] **Email format not validated in `AddMember`** (P2-8) — `platform/internal/handlers/classes.go:292-302`. Add a basic email check before the DB call.
- [ ] **`maskURL` may leak credentials** (P2-9) — `platform/cmd/api/main.go:308-312`. Use `net/url` to parse and redact the password.
- [ ] **No request body size limits on JSON endpoints** (P2-10) — `platform/internal/handlers/helpers.go:38-43`. Wrap with `http.MaxBytesReader`.
- [ ] **Broadcaster subscriber callback runs inline with no timeout** (P2-11) — `platform/internal/events/broadcaster.go:44-62`. A slow SSE subscriber blocks the emitting goroutine.
- [ ] **~30 permanently disabled `test.skip` calls in e2e** (P2-12). Convert to conditional gates or delete dead tests.
- [ ] **No React error boundary anywhere in the tree** (P2-13). A single render error white-screens the whole app.
- [ ] **`UpdateParticipantStatus` pseudo-status handling is confusing** (P2-14) — `platform/internal/store/sessions.go:449-471`. Extract `help_requested_at` to dedicated methods or document the convention.
- [ ] **Plan 028 missing post-execution report** (process P2-15).
- [ ] **Seven plans have stale "In progress" status labels while their post-execution reports say "Complete"** (process P2-16) — plans 032-038.
- [ ] **Plan 012 has duplicate numbering** (process P2-17) — `012-assignments.md` AND `012-monaco-editor-migration.md`.
- [ ] **Deleted `seed_python_101.sql` still referenced in 20+ docs** (process P2-18). Annotate as "Retired by plan 049."
- [ ] **Parent-linking deferrals reference reassigned plan numbers** (process P2-19) — `platform/internal/handlers/parent.go:24-41` and `src/app/onboarding/page.tsx:30-34`.
- [ ] **13 `[OPEN]` review items in 3 shipped plans never resolved** (process P2-21) — plans 017, 012, 031. Either convert to `[WONTFIX]` with plan citations or file follow-ups.

## Frontend

- [ ] **Editor theme** — CodeMirror uses default light theme; add dark mode support matching the app theme
- [ ] **Mobile responsive** — Dashboard and editor pages need mobile layout optimization (Chromebook/tablet)
- [ ] **Loading states** — Add skeleton loaders for dashboard, classroom detail, and session pages
- [ ] **Error boundaries** — Add React error boundaries around editor and session components

## Real-time

- [ ] **Hocuspocus persistence** — Currently in-memory only; add PostgreSQL persistence for Yjs documents
- [ ] **Reconnection handling** — Handle WebSocket disconnects gracefully with auto-reconnect and user notification
- [ ] **Student tile optimization** — Each StudentTile creates its own Hocuspocus provider; may not scale to 30+ students
- [x] ~~**Broadcast mode** — Teacher live code broadcast with Yjs broadcast document~~

## AI

- [x] ~~**AI toggle SSE integration** — Student receives real-time SSE notification when teacher toggles AI~~
- [ ] **AI rate limiting** — No per-student rate limit on AI interactions yet
- [x] ~~**Annotation UI** — Annotation form + list in teacher collaborative editing sidebar~~
- [x] ~~**AI activity feed** — Teacher dashboard sidebar shows AI interaction summaries~~

## Phase 2 (Post-MVP)

- [ ] **Block editor (Blockly)** — K-5 students, transpiles to JS
- [ ] **JavaScript/HTML/CSS execution** — iframe sandbox with preview pane
- [ ] **Assignment system** — creation, submission, grading
- [ ] **Code playback** — Yjs history replay
- [ ] **Output canvas** — HTML5 Canvas for graphical output (turtle graphics, games)
- [ ] **AI progress tracking** — Student skill maps and risk flags
- [ ] **AI teacher assistant** — Pre/during/post session insights
- [ ] **Microsoft OAuth** — For M365 school districts

## Phase 3

- [ ] **Starter curriculum library** — Pre-built lessons and exercises
- [ ] **Assessment and grading** — Rubrics, auto-grading
- [ ] **Block-to-text transition** — Guided path from Blockly to Python
- [ ] **Analytics dashboard** — Student progress, class performance
- [ ] **Snippet library** — Teacher-loaded code templates
- [ ] **LTI integration** — Canvas, Google Classroom, Schoology

## Documentation

- [ ] Add architecture decisions doc (`docs/architecture/decisions.md`)
- [ ] Document Hocuspocus setup and document naming conventions
- [ ] Document AI system prompt customization
