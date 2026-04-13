# Go Backend Migration Spec

## Overview

Migrate all backend logic from Next.js API routes to a Go HTTP service. Next.js becomes a pure frontend (React pages + Auth.js OAuth flow). Go handles all API routes, database access, LLM integration, agentic workflows, and code execution. Hocuspocus stays as a JS sidecar for Yjs document sync.

**Motivation:**
- Agentic workloads (30+ concurrent AI agents per classroom) need goroutine-level concurrency
- Server-side code execution (C++, Java, Rust) needs Go's subprocess/container management
- Workflow engine (AIGC, reports, self-pacing) needs DAG execution with worker pools
- Existing Go patterns from magicburg are proven and portable (~6,500 lines reusable)

**Migration strategy:** Incremental with proxy (Option B). Next.js proxies `/api/*` to Go for migrated routes. Contract tests compare Go vs Next.js responses before each route is flipped.

---

## Architecture

```
Next.js (port 3003) — Frontend Only
├── React pages (SSR via fetch to Go API)
├── Auth.js (Google OAuth flow, session cookie, JWT)
├── Client components (editor, sidebar, Yjs provider, etc.)
├── apiClient helper (forwards JWT to Go)
└── No /api/* routes, no DB connection, no business logic

Go Backend (port 8001) — All API + Agentic
├── Chi router with middleware (auth, CORS, logging, recovery)
├── /api/* routes (all 50+ routes)
├── LLM integration (6 providers, streaming, tool calling)
├── Agentic loop (multi-turn AI with tool execution)
├── Workflow engine (DAG execution, cron scheduling, worker pools)
├── Skills/tools (code runner, lesson generator, tutor, etc.)
├── Agents (student tutor, teacher assistant, self-pacer, content creator)
├── Code execution (Piston integration)
├── SSE streaming
└── PostgreSQL (pgx, single connection pool)

Hocuspocus (port 4000) — Real-time Only (stays JS)
├── Yjs document sync
├── Document persistence hooks
└── PostgreSQL connection (for Yjs state storage)

Piston (port 2000) — Code Execution (new)
├── Sandboxed containers
└── 60+ language support
```

---

## Go Project Structure

Shared pattern directories mirror magicburg's layout for future sync.

```
gobackend/
├── cmd/
│   ├── api/main.go                  # HTTP server
│   └── engine/main.go               # Workflow engine
├── internal/
│   ├── auth/                        # ← shared pattern
│   │   ├── jwt.go                   # JWT verification (HS256, validates NEXTAUTH_SECRET)
│   │   └── middleware.go            # Bearer token extraction, claims injection
│   ├── config/                      # ← shared pattern
│   │   └── config.go               # TOML + env, LLM config, API keys
│   ├── db/                          # ← shared pattern
│   │   ├── open.go                  # PostgreSQL connection pool (pgx)
│   │   ├── dialect.go              # SQL dialect helpers
│   │   └── migrate.go              # golang-migrate integration
│   ├── llm/                         # ← shared pattern
│   │   ├── backend.go              # Interface: Chat, StreamChat, ChatWithTools
│   │   ├── types.go                # Message, StreamChunk, ToolCall, LLMConfig
│   │   ├── factory.go              # Provider factory (8 backends)
│   │   ├── openai.go              # OpenAI-compatible
│   │   ├── anthropic.go           # Anthropic Messages API
│   │   ├── gemini.go              # Google Gemini
│   │   └── agent_loop.go          # Multi-turn agentic loop
│   ├── tools/                       # ← shared pattern
│   │   ├── contracts.go            # Tool interface
│   │   └── registry.go            # Registry + dispatcher
│   ├── workflows/                   # ← shared pattern
│   │   ├── dag.go                  # Topological sort, cycle detection
│   │   ├── cron.go                 # Cron parsing
│   │   ├── executor.go            # DAG run executor
│   │   └── store.go               # Workflow persistence
│   ├── events/                      # ← shared pattern
│   │   └── broadcaster.go         # Per-session SSE event bus
│   ├── sandbox/                     # ← shared pattern
│   │   └── piston.go              # Piston API client
│   ├── store/                       # Bridge-specific DB queries
│   │   ├── users.go
│   │   ├── orgs.go
│   │   ├── courses.go
│   │   ├── classes.go
│   │   ├── sessions.go
│   │   ├── documents.go
│   │   ├── assignments.go
│   │   ├── attendance.go
│   │   └── reports.go
│   ├── handlers/                    # Bridge-specific API handlers
│   │   ├── auth.go
│   │   ├── orgs.go
│   │   ├── courses.go
│   │   ├── classes.go
│   │   ├── sessions.go
│   │   ├── documents.go
│   │   ├── assignments.go
│   │   ├── ai.go
│   │   ├── annotations.go
│   │   ├── admin.go
│   │   ├── parent.go
│   │   └── sse.go
│   ├── skills/                      # Bridge-specific AI tools
│   │   ├── code_runner.go
│   │   ├── code_analyzer.go
│   │   ├── lesson_generator.go
│   │   ├── report_generator.go
│   │   └── tutor.go
│   └── agents/                      # Bridge-specific AI agents
│       ├── student_tutor.go
│       ├── teacher_assistant.go
│       ├── self_pacer.go
│       └── content_creator.go
├── migrations/
│   └── *.sql
├── tests/
│   ├── contract/                    # Go vs Next.js comparison tests
│   ├── integration/
│   └── unit/
├── config.toml
├── go.mod
└── go.sum
```

`docs/shared-patterns.md` tracks which directories mirror magicburg and their sync status.

---

## Auth Bridge

Auth.js continues to handle OAuth. Go validates the JWT.

**Login flow (unchanged):**
Browser → Auth.js (Next.js) → Google OAuth → session cookie

**API call flow (new):**
React component → `useSession()` gets token → `fetch(GO_API + "/api/...", { Authorization: Bearer })` → Go validates JWT → processes request

**Go JWT validation:**
- Reads `NEXTAUTH_SECRET` from environment
- Validates HS256 signature (same algorithm Auth.js uses)
- Extracts claims: `user.id`, `user.email`, `user.name`, `user.isPlatformAdmin`
- Impersonation: reads `bridge-impersonate` cookie, overrides identity

---

## Next.js Frontend Changes

**Remove:**
- All `src/lib/*.ts` backend modules (classrooms, courses, sessions, etc.)
- All `src/app/api/` route handlers
- All `src/lib/db/` database code (Drizzle, schema)
- Server actions (`"use server"` blocks in portal pages)

**Add:**
- `src/lib/api-client.ts` — shared fetch wrapper with auth token
- `GO_API_URL` environment variable

**Change:**
- Server components: `db.select()` → `api("/api/courses")`
- Server actions: inline `"use server"` → client-side `fetch()` + `POST`
- Portal pages: same React components, different data source

**Keep unchanged:**
- React components, portal layouts, sidebar, theme
- Auth.js (login/register pages, OAuth flow)
- Editor components (Monaco, Blockly, Pyodide, JS sandbox)
- Yjs/Hocuspocus integration (browser → Hocuspocus directly)
- Playwright E2E tests (they test the frontend, agnostic of backend)

---

## Contract Testing

For each migrated route, before flipping the proxy:

1. Send identical request to Next.js (port 3003)
2. Send identical request to Go (port 8001)
3. Compare: status codes, response body shape, key values
4. Pass → flip proxy; Fail → fix Go

**Compare:** HTTP status, JSON structure, data values, auth behavior (401/403)
**Skip:** Timestamps, new UUIDs, exact error wording

Structure: `gobackend/tests/contract/` — same pattern as magicburg.

---

## Migration Phases

### Phase 1: Go Foundation + Proxy (Week 1-2)
- Go project setup: Chi router, middleware, config, DB connection
- Copy shared patterns from magicburg (auth, llm, tools, events)
- Next.js proxy middleware for `/api/*`
- Contract test infrastructure
- Migrate: orgs, users, admin routes

### Phase 2: Core Routes (Week 2-3)
- Migrate: courses, topics, classes, class memberships
- Migrate: sessions, session topics, participants, SSE events
- Migrate: documents, assignments, submissions
- Migrate: annotations, AI toggle/interactions
- Each route: implement → contract test → flip proxy

### Phase 3: AI + Agentic (Week 3-4)
- Migrate: AI chat (streaming, guardrails, grade-level prompts)
- Port agentic loop with Bridge tools (tutor, code analyzer)
- Port parent report generation (LLM-based)
- Implement Piston integration for server-side code execution
- New languages: C++, Java, Rust via Piston

### Phase 4: Workflow Engine + Agents (Week 4-5)
- Port DAG executor from magicburg
- Build Bridge agents: teacher assistant, self-pacer, content creator
- Background jobs: AIGC content, batch reports, self-pacing recommendations
- Cron scheduling for recurring workflows

### Phase 5: Cleanup (Week 5-6)
- Remove all `/api/*` from Next.js
- Remove `src/lib/` backend modules
- Convert server components to Go API fetch
- Convert server actions to client fetch
- Update Playwright E2E tests
- Final verification pass

---

## Piston Integration

For server-side code execution (C++, Java, Rust, and enhanced Python/JS):

```
POST http://piston:2000/api/v2/execute
{
  "language": "cpp",
  "version": "10.2.0",
  "files": [{ "content": "#include <iostream>..." }],
  "compile_timeout": 10000,
  "run_timeout": 5000,
  "compile_memory_limit": 256000000,
  "run_memory_limit": 256000000
}
```

Go's `sandbox/piston.go` wraps this with:
- Timeout enforcement
- Memory limit configuration per language
- Output size limits
- Queue management for concurrent executions

Browser-side execution (Pyodide, JS iframe, Blockly) remains unchanged for instant feedback. Piston is used for:
- Compiled languages (C++, Java, Rust, C#)
- Full Python with pip packages
- Assignment auto-grading (run student code against test cases)
- AI-generated code validation

---

## What's Deferred

- Shared Go module extraction (wait until both projects stabilize)
- WebSocket migration (Hocuspocus stays JS)
- Auth.js replacement (JWT-only auth without Auth.js — future consideration)
- Kubernetes deployment (single-machine Docker Compose for now)
