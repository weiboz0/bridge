# Bridge

A live-first K-12 coding education platform with AI-powered tutoring.

Bridge combines a browser-based coding environment, real-time classroom collaboration, and teacher-controlled AI assistance to help teachers run interactive coding classes and track student progress.

## Features

- **Live coding sessions** — Teachers start sessions, students join and code together in real-time
- **Multi-language editor** — Monaco Editor with Python (Pyodide), JavaScript (iframe sandbox), and Blockly (K-5)
- **Real-time collaboration** — Teachers see all student code live via Yjs + Hocuspocus
- **AI tutor** — Socratic AI assistant that hints, never solves. Teacher-controlled per student. Supports 7 LLM providers (Anthropic, OpenAI, DashScope, Gemini, OpenRouter, Ark, Ollama)
- **Course & class management** — Courses → Topics → Classes with join codes
- **Organization system** — Schools, tutoring centers, bootcamps with role-based access (admin, org_admin, teacher, student, parent)
- **Code annotations** — Teachers and AI comment on specific lines of student code
- **Session scheduling** — Plan sessions in advance with topic selection
- **Parent portal** — Parents view child progress and AI-generated reports
- **Admin impersonation** — Platform admins can test any user's experience

## Tech Stack

| Layer | Technology |
|---|---|
| Frontend | [Next.js 16](https://nextjs.org) (App Router), React, TypeScript |
| Backend API | [Go](https://go.dev) (Chi router, pgx, JWT auth) |
| Database | PostgreSQL |
| Auth | [Auth.js v5](https://authjs.dev) (Google OAuth + credentials) |
| UI | [shadcn/ui](https://ui.shadcn.com) + Tailwind CSS v4 |
| Editor | [Monaco Editor](https://microsoft.github.io/monaco-editor/) |
| Code Execution | Pyodide (Python WASM), iframe sandbox (JS), Piston (compiled languages) |
| Real-time | Yjs + [Hocuspocus](https://tiptap.dev/hocuspocus) |
| AI | 7 LLM providers via unified Go abstraction layer |
| Testing | Vitest (frontend), Go test (backend), Playwright (E2E) |

## Getting Started

### Prerequisites

- [Bun](https://bun.sh) >= 1.0
- [Go](https://go.dev) >= 1.23
- PostgreSQL >= 15

### Setup

```bash
# Clone the repo
git clone https://github.com/weiboz0/bridge.git
cd bridge

# Install frontend dependencies
bun install

# Set up environment
cp .env.example .env
# Edit .env with your database credentials and auth secrets

# Create databases
createdb bridge
createdb bridge_test

# Run migrations
bun run db:migrate

# Start all services (three terminals)
bun run dev                    # Terminal 1: Next.js (port 3003)
bun run hocuspocus             # Terminal 2: Yjs WebSocket (port 4000)
cd platform && make dev        # Terminal 3: Go API (port 8002)
```

### Environment Variables

| Variable | Description |
|---|---|
| `DATABASE_URL` | PostgreSQL connection string (e.g., `postgresql://work@127.0.0.1:5432/bridge`) |
| `NEXTAUTH_SECRET` | Auth.js secret — `openssl rand -base64 32` |
| `GOOGLE_CLIENT_ID` | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret |
| `LLM_BACKEND` | LLM provider: `anthropic`, `openai`, `aliyun`, `gemini`, `openrouter`, `ark`, `ollama` |
| `LLM_MODEL` | Model override (default: provider's default) |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `DASHSCOPE_API_KEY` | DashScope/Aliyun API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `GEMINI_API_KEY` | Google Gemini API key |
| `OPENROUTER_API_KEY` | OpenRouter API key |

### Running Tests

```bash
# Frontend tests (Vitest)
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run test

# Go backend tests
cd platform
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test make test-integration

# E2E tests (Playwright — requires all services running)
bun run test:e2e

# Go contract tests (requires both Next.js and Go servers running)
cd platform && make test-contract
```

## Project Structure

```
bridge/
├── src/                        # Next.js frontend
│   ├── app/                    # App Router pages
│   │   ├── (portal)/           # Role-based portals (teacher, student, admin, parent, org)
│   │   └── api/                # API routes (proxied to Go)
│   ├── components/             # React components
│   └── lib/                    # Shared utilities, API client, auth
├── platform/                   # Go backend
│   ├── cmd/api/                # Server entry point
│   └── internal/
│       ├── auth/               # JWT + JWE verification, middleware
│       ├── config/             # TOML + env config
│       ├── db/                 # PostgreSQL connection
│       ├── events/             # SSE broadcaster
│       ├── handlers/           # HTTP handlers (~60 routes)
│       ├── llm/                # Multi-provider LLM abstraction + agentic loop
│       ├── sandbox/            # Piston, Deno, bwrap sandboxes
│       ├── skills/             # AI tools (tutor, code analyzer, generators)
│       ├── store/              # SQL data access layer
│       └── tools/              # Tool registry for agentic loop
├── server/                     # Hocuspocus WebSocket server
├── e2e/                        # Playwright E2E tests
├── tests/                      # Vitest test suites
└── docs/                       # Specs, plans, and workflow docs
    ├── specs/                  # Design specifications
    └── plans/                  # Implementation plans
```

## Architecture

Bridge uses a **dual-service architecture**:

- **Next.js** handles the frontend (React server components + client components), Auth.js OAuth flow, and proxies API requests to Go
- **Go platform** handles all API logic, database access, LLM integration, and code execution
- **Hocuspocus** runs as a sidecar for Yjs real-time document synchronization

All client-side API requests go through Next.js `beforeFiles` rewrites which proxy to the Go backend. Server components use a direct API client with Auth.js JWE token forwarding.

## License

[MIT](LICENSE)
