# Bridge

A live-first K-12 coding education platform with AI-powered teaching capabilities.

Bridge combines a browser-based coding environment, real-time classroom collaboration, and teacher-controlled AI assistance to help teachers run interactive coding classes and track student progress.

## Features

### MVP (Current)
- **Authentication** — Google OAuth and email/password sign-in
- **Classroom management** — Create classrooms, generate join codes, manage student rosters
- **Role-based access** — Teacher and student roles with appropriate permissions

### Planned
- **Browser-based code editor** — CodeMirror 6 with Python execution via Pyodide (all in-browser)
- **Real-time collaboration** — Teachers see all student code live via Yjs + Hocuspocus
- **AI tutor** — Teacher-controlled, Socratic AI assistant (Claude API) that hints, never solves
- **Code annotations** — Teachers and AI can comment on specific lines of student code
- **Block-based editor** — Blockly for K-5 students, with transition path to text-based coding
- **Student progress tracking** — AI-powered skill maps and risk flags
- **Teacher assistant** — Pre/during/post session insights and recommendations

## Tech Stack

| Layer | Technology |
|---|---|
| Runtime | [Bun](https://bun.sh) |
| Framework | [Next.js 15](https://nextjs.org) (App Router) |
| Language | TypeScript |
| Database | PostgreSQL + [Drizzle ORM](https://orm.drizzle.team) |
| Auth | [Auth.js v5](https://authjs.dev) |
| UI | [shadcn/ui](https://ui.shadcn.com) + Tailwind CSS |
| Testing | [Vitest](https://vitest.dev) |
| Editor | CodeMirror 6 (planned) |
| Code Execution | Pyodide / WASM (planned) |
| Real-time | Yjs + Hocuspocus (planned) |
| AI | Claude API (planned) |

## Getting Started

### Prerequisites

- [Bun](https://bun.sh) >= 1.0
- PostgreSQL >= 15
- Node.js >= 20 (used by Next.js internally)

### Setup

```bash
# Clone the repo
git clone https://github.com/weiboz0/bridge.git
cd bridge

# Install dependencies
bun install

# Set up environment
cp .env.example .env
# Edit .env with your database credentials and auth secrets

# Create databases
createdb bridge
createdb bridge_test

# Run migrations
bun run db:generate
bun run db:migrate

# Start development server (two terminals needed)
bun run dev          # Terminal 1: Next.js app
bun run hocuspocus   # Terminal 2: Yjs WebSocket server for real-time collaboration
```

The app will be available at `http://localhost:3000`. The Hocuspocus WebSocket server runs on port 4000.

### Environment Variables

| Variable | Description |
|---|---|
| `DATABASE_URL` | PostgreSQL connection string |
| `NEXTAUTH_URL` | App URL (http://localhost:3000 for dev) |
| `NEXTAUTH_SECRET` | Auth.js secret (`openssl rand -base64 32`) |
| `GOOGLE_CLIENT_ID` | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret |
| `REDIS_URL` | Redis connection string |
| `LLM_BACKEND` | LLM provider: `anthropic` (default), `openai`, `aliyun`, `ark`, `nvidia`, `openrouter` |
| `LLM_MODEL` | Model override (default: provider's default) |
| `LLM_BASE_URL` | API endpoint override (for proxies or private deployments) |
| `ANTHROPIC_API_KEY` | Anthropic API key (when using `anthropic` backend) |
| `DASHSCOPE_API_KEY` | DashScope API key (when using `aliyun` backend) |

### Running Tests

```bash
# Run all tests (unit + integration)
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run test

# Run tests in watch mode
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run test:watch

# Run only unit tests
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run test tests/unit/

# Run only integration tests
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run test tests/integration/

# Run a specific test file
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run test tests/integration/sessions-api.test.ts
```

**Test structure:**
- `tests/unit/` — Unit tests (schema, utilities, guardrails, component rendering)
- `tests/api/` — Database integration tests (business logic with real Postgres)
- `tests/integration/` — API integration tests (route handlers with mocked auth + real DB)

### Available Scripts

| Command | Description |
|---|---|
| `bun run dev` | Start Next.js development server |
| `bun run hocuspocus` | Start Yjs WebSocket server (required for live sessions) |
| `bun run build` | Build for production |
| `bun run start` | Start production server |
| `bun run test` | Run test suite |
| `bun run test:watch` | Run tests in watch mode |
| `bun run lint` | Run ESLint |
| `bun run db:generate` | Generate database migrations |
| `bun run db:migrate` | Apply database migrations |
| `bun run db:studio` | Open Drizzle Studio (DB browser) |

## Project Structure

```
bridge/
├── src/
│   ├── app/                    # Next.js App Router pages and API routes
│   │   ├── (auth)/             # Login and registration pages
│   │   ├── dashboard/          # Authenticated dashboard and classroom pages
│   │   └── api/                # REST API endpoints
│   ├── components/             # React components (shadcn/ui + custom)
│   ├── lib/                    # Business logic and utilities
│   │   ├── db/                 # Drizzle schema and client
│   │   ├── auth.ts             # Auth.js configuration
│   │   ├── classrooms.ts       # Classroom operations
│   │   └── utils.ts            # Shared utilities
│   └── types/                  # TypeScript type augmentations
├── tests/                      # Vitest test suites
│   ├── unit/                   # Unit tests (schema, utils, guardrails, components)
│   ├── api/                    # Database integration tests (business logic)
│   └── integration/            # API integration tests (route handlers end-to-end)
├── server/                     # Hocuspocus WebSocket server (sidecar)
├── drizzle/                    # Database migrations
└── docs/                       # Design specs, plans, and workflow docs
```

## Architecture

Bridge uses a monolith architecture — a single Next.js application handling both the frontend and API, with Hocuspocus as a sidecar for real-time document sync (planned).

All code execution happens in the browser via Pyodide (Python) and native JS — no server-side code execution required.

See [Design Spec](docs/superpowers/specs/2026-04-10-bridge-platform-design.md) for the full architecture document.

## License

Private — All rights reserved.
