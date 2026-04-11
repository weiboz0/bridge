# TODO

Outstanding tasks and technical debt. Check this file when planning new work.

## Technical Debt

- [ ] **Next.js middleware deprecation** — `middleware.ts` convention is deprecated in Next.js 16, should migrate to `proxy` pattern
- [ ] **Pyodide CDN dependency** — Web Worker loads Pyodide from CDN; should consider self-hosting or bundling for reliability
- [ ] **Hocuspocus auth** — Token format is simple `userId:role` string; should use JWT or signed tokens in production
- [ ] **Database migrations** — Using direct SQL apply (`psql -f`); drizzle-kit migrate has issues, needs investigation

## Frontend

- [ ] **Editor theme** — CodeMirror uses default light theme; add dark mode support matching the app theme
- [ ] **Mobile responsive** — Dashboard and editor pages need mobile layout optimization (Chromebook/tablet)
- [ ] **Loading states** — Add skeleton loaders for dashboard, classroom detail, and session pages
- [ ] **Error boundaries** — Add React error boundaries around editor and session components

## Real-time

- [ ] **Hocuspocus persistence** — Currently in-memory only; add PostgreSQL persistence for Yjs documents
- [ ] **Reconnection handling** — Handle WebSocket disconnects gracefully with auto-reconnect and user notification
- [ ] **Student tile optimization** — Each StudentTile creates its own Hocuspocus provider; may not scale to 30+ students
- [ ] **Broadcast mode** — Teacher live code broadcast UI not yet implemented (infrastructure ready)

## AI

- [ ] **AI toggle SSE integration** — Student doesn't receive real-time notification when teacher toggles AI; currently requires page refresh
- [ ] **AI rate limiting** — No per-student rate limit on AI interactions yet
- [ ] **Annotation UI** — CodeMirror gutter markers and annotation popover not yet implemented (API ready)
- [ ] **AI activity feed** — Teacher view of all AI interactions not yet implemented (API ready)

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
