# Bridge Platform Design Spec

## Overview

Bridge is a live-first, multi-user K-12 programming education platform with AI-powered teaching capabilities. It combines a browser-based coding environment, real-time classroom collaboration, and teacher-controlled AI assistance to help teachers run interactive coding classes and track student progress.

**Target audience**: K-12 students learning programming, and their teachers.

**Core thesis**: No existing platform combines a professional-grade coding environment, real-time teacher monitoring, teacher-controlled AI tutoring, and AI-powered teaching insights. Bridge fills that gap with a live-session-first approach.

## Technology Stack

| Layer | Choice | Rationale |
|---|---|---|
| Runtime | Bun | Native TypeScript, fast WebSocket, fast package management, drop-in Node.js compatible |
| Frontend | Next.js (React) | Largest ecosystem, SSR for dashboard, client components for editor |
| Editor | CodeMirror 6 | Small bundle (~150KB), mobile/Chromebook friendly, native Yjs collab binding |
| Block Editor | Blockly (Phase 2) | Google's open-source block-based editor, transpiles to JS, widely adopted |
| Code Execution | Pyodide (Python, WASM) + native JS engine | All in-browser, zero server cost, inherently safe, works offline |
| Real-time Sync | Yjs + Hocuspocus | CRDT-based document sync, handles teacher viewing + collaborative editing |
| AI | Claude API, server-mediated | Strong instruction-following for Socratic tutoring, streaming support |
| Database | PostgreSQL + Redis | Postgres for relational data with row-level security; Redis for sessions, presence, pub/sub |
| Auth | Google OAuth + Microsoft OAuth + email/password | Google is essential for schools; Microsoft for M365 districts; email as fallback |
| Hosting | Cloud SaaS | Simplest to build and adopt; on-premise can be added later if needed |

## Architecture

Monolith architecture — single Next.js application with Hocuspocus as a sidecar process for real-time document sync.

```
Browser (Student)                    Browser (Teacher)
├── Next.js UI                       ├── Next.js UI
├── CodeMirror 6 editor              ├── Live dashboard (all students' code)
├── Pyodide (Python execution)       ├── AI dispatch controls
├── Blockly (Phase 2)                ├── Code annotation tools
└── Yjs client (doc sync)            └── Yjs awareness (cursors, selections)
        ↕ WebSocket                          ↕ WebSocket
┌─────────────────────────────────────────────────────┐
│                   Bridge Server                      │
│  Next.js App (Bun runtime)                          │
│  ├── API routes (REST)                              │
│  │   ├── Auth (Google OAuth, email/password)        │
│  │   ├── Classrooms & sessions                      │
│  │   ├── Assignments & submissions                  │
│  │   └── AI proxy (Claude API, teacher-gated)       │
│  ├── Hocuspocus sidecar (Yjs document sync)         │
│  └── SSE (class events, notifications)              │
├─────────────────────────────────────────────────────┤
│  PostgreSQL          │          Redis                │
│  (users, classes,    │  (sessions, presence,         │
│   assignments, code) │   pub/sub, caching)           │
└─────────────────────────────────────────────────────┘
```

All code execution happens in the browser. The server handles auth, data persistence, Yjs document sync, AI proxy, and event broadcasting. No server-side code execution in MVP.

## Classroom Model

**Live-session-first with persistent assignments.**

- A **Classroom** is a persistent container — it has a roster, assignments, and student work history.
- A **LiveSession** is an ephemeral event within a classroom — teacher starts it, students join, real-time collaboration happens. When it ends, all work is persisted.
- Live sessions are the primary experience. Assignments and async work are supporting features.

## Data Model

### User
| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| name | string | Display name |
| email | string | Unique |
| role | enum | teacher, student, admin |
| avatarUrl | string | Nullable |
| passwordHash | string | Nullable (empty for OAuth-only) |
| schoolId | UUID | FK → School, nullable (null for admin or unaffiliated) |
| createdAt | timestamp | |
| updatedAt | timestamp | |

### AuthProvider
| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| userId | UUID | FK → User |
| provider | enum | google, microsoft, email |
| providerUserId | string | External provider's user ID |
| createdAt | timestamp | |

### School
| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key (tenant boundary) |
| name | string | |
| settings | JSONB | School-level configuration |

### Classroom
| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| schoolId | UUID | FK → School |
| teacherId | UUID | FK → User |
| name | string | |
| description | string | |
| gradeLevel | enum | K-5, 6-8, 9-12 |
| editorMode | enum | blockly, python, javascript |
| joinCode | string | Unique, for students to join |

### LiveSession
| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| classroomId | UUID | FK → Classroom |
| teacherId | UUID | FK → User |
| status | enum | active, ended |
| settings | JSONB | aiEnabled, editorMode, etc. |
| startedAt | timestamp | |
| endedAt | timestamp | Nullable |

### SessionParticipant
| Field | Type | Notes |
|---|---|---|
| sessionId | UUID | FK → LiveSession |
| studentId | UUID | FK → User |
| status | enum | active, idle, needs_help |
| joinedAt | timestamp | |
| leftAt | timestamp | Nullable |

### Document
| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| ownerId | UUID | FK → User |
| classroomId | UUID | FK → Classroom |
| sessionId | UUID | FK → LiveSession, nullable |
| title | string | |
| language | enum | python, javascript, blockly |
| yjsState | bytea | Yjs encoded state (source of truth) |
| plainText | text | Denormalized snapshot for search/grading |

### Assignment
| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| classroomId | UUID | FK → Classroom |
| teacherId | UUID | FK → User |
| title | string | |
| description | text | |
| starterCode | text | |
| language | enum | python, javascript, blockly |
| dueDate | timestamp | Nullable |
| rubric | JSONB | Grading criteria |

### Submission
| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| assignmentId | UUID | FK → Assignment |
| studentId | UUID | FK → User |
| documentId | UUID | FK → Document |
| grade | float | Nullable |
| feedback | text | Nullable |
| submittedAt | timestamp | |

### AIInteraction
| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| studentId | UUID | FK → User |
| sessionId | UUID | FK → LiveSession |
| documentId | UUID | FK → Document |
| enabledByTeacherId | UUID | FK → User |
| messages | JSONB | Conversation history array |
| createdAt | timestamp | |

### CodeAnnotation
| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| documentId | UUID | FK → Document |
| authorId | UUID | FK → User |
| authorType | enum | teacher, ai |
| lineStart | int | Start line of annotation range |
| lineEnd | int | End line of annotation range |
| content | text | Annotation text |
| createdAt | timestamp | |

### StudentProgress (Phase 2)
| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| studentId | UUID | FK → User |
| classroomId | UUID | FK → Classroom |
| skillMap | JSONB | concept → proficiency level |
| riskFlags | JSONB | Array of flagged issues |
| lastAnalyzedAt | timestamp | |
| updatedAt | timestamp | |

### TeacherInsight (Phase 2)
| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| classroomId | UUID | FK → Classroom |
| sessionId | UUID | FK → LiveSession, nullable |
| type | enum | pre_session, during_session, post_session |
| content | JSONB | Structured recommendations |
| acknowledged | boolean | |
| createdAt | timestamp | |

## Real-Time System

### Channel 1: Document Sync (Yjs + Hocuspocus)

Every open editor is a Yjs document synced via WebSocket through Hocuspocus.

- **Student opens editor** → Yjs client connects to Hocuspocus → document syncs
- **Student types** → Yjs propagates CRDT ops to Hocuspocus → Hocuspocus persists to Postgres and broadcasts to subscribers
- **Teacher opens live dashboard** → subscribes to all documents in the session → sees every student's code updating in real-time
- **Teacher clicks into a student's code** → full collaborative editing (both cursors visible)
- **Awareness protocol** → Yjs awareness tracks cursor positions, selections, and user presence

### Channel 2: Class Events (SSE)

Server-to-client broadcasts for non-document events:

- Student joined/left session
- Student submitted assignment
- Teacher enabled/disabled AI for a student
- AI interaction started/completed
- Student raised hand / needs help
- Session started/ended

SSE is used rather than WebSocket because these are one-directional server-to-client broadcasts.

### Teacher Live Dashboard

The teacher's primary view during a live session. Grid of student tiles, each showing a miniaturized read-only CodeMirror instance synced via Yjs. Each tile shows:

- Student name
- Live code preview (miniaturized editor)
- Status indicator: active, idle, needs help, AI active
- AI toggle control
- Click to expand into full collaborative editing view

## AI System

### Role 1: Student Tutor

Teacher-enabled per student or per session. When enabled, student sees an "Ask for help" button that opens a chat panel alongside the editor.

**Server-side pipeline:**
1. Student sends message
2. API validates AI is enabled for this student in this session
3. Build prompt: system prompt (Socratic tutor, grade-appropriate language, no solutions) + assignment context + student's current code (from Yjs state) + conversation history
4. Call Claude API with streaming
5. Stream response to student via WebSocket
6. Log full interaction to AIInteraction table
7. Push notification to teacher's AI activity feed via SSE

**Guardrails:**
- System prompt enforces pedagogy — ask guiding questions, point to where to look, never provide complete solutions
- Output filtering — server-side check rejects responses containing complete function implementations
- Rate limiting — configurable max interactions per student per session
- Full teacher visibility — every conversation visible in real-time, teacher can intervene or disable

**Grade-level adaptation:**
- K-5 (Blockly): Simple vocabulary, visual references, encouragement-heavy
- 6-8 (Python intro): Explain concepts, reference line numbers, use analogies
- 9-12 (Python/JS): Technical language, reference documentation, discuss trade-offs

### Role 2: Student Progress Tracker (Phase 2)

AI continuously analyzes each student's work patterns and builds a learning profile.

**Signals:**
- Code submissions: concepts used, struggle areas
- Error patterns: repeated syntax errors, logic mistakes
- Time on task: duration, where they get stuck
- AI interactions: questions asked, hints needed
- Completion rate: assignments finished vs abandoned

**Outputs per student:**
- Skill map: concept → proficiency level (e.g., "loops — strong, functions — struggling")
- Risk flags: "hasn't submitted in 3 sessions", "error rate increasing"
- Recommended next steps: "ready for recursion", "needs more practice on conditionals"

### Role 3: Teacher Assistant (Phase 2)

AI aggregates student data into actionable insights.

**Before class:**
- "5 students struggled with for loops last session — consider a review exercise"
- "3 students are ahead — here's an extension challenge"
- Lesson plan adjustment suggestions

**During live session:**
- "Alice has been idle for 4 minutes on line 7"
- "80% of students have the same bug on line 12 — consider addressing for the whole class"
- Priority queue ranking students by who needs attention most

**After class:**
- Session summary: coverage, participation, common struggles
- Per-student progress updates
- Suggested follow-up exercises

## Code Annotations

Teachers and AI can attach comments to specific lines or ranges of lines in a student's code.

- **Teacher annotations**: Teacher clicks a line in student's code, types a comment, student sees it in real-time via Yjs sync
- **AI annotations**: When AI explains an error or concept, it attaches the explanation to the relevant line(s) rather than only in the chat panel
- Annotations appear as inline markers (similar to GitHub code review comments)
- Persisted with the document — students can review later
- Work during live sessions and asynchronously on submissions

## Code Execution

All execution happens in the browser in MVP:

- **Python**: Pyodide (CPython compiled to WASM). ~50ms execution latency, inherently sandboxed, works offline. Supports standard library and select packages (numpy, etc.).
- **JavaScript/HTML/CSS** (Phase 2): Native browser JS engine. Sandboxed via iframe. HTML/CSS rendered in a preview pane.
- **Blockly** (Phase 2): Transpiles to JavaScript, executed via native JS engine.

**Output panel**: Text console for stdout/stderr. Visual canvas (HTML5 Canvas) for graphical output in Phase 2 (turtle graphics, simple games, animations).

No server-side code execution is planned for MVP or Phase 2. Server-side execution (via Piston or Judge0) would only be needed if supporting compiled languages (Java, C++) in Phase 3+.

## Authentication

- **Google OAuth**: Primary — most schools use Google Workspace for Education
- **Microsoft/Azure AD**: Phase 2 — for Microsoft 365 districts
- **Email/password**: Fallback for schools not on Google or Microsoft
- **Class join codes**: Teacher generates a code, student enters name and joins without creating an account (for quick demos/trials)
- **LTI integration** (Phase 3): Plug into Canvas, Google Classroom, Schoology

Users can link multiple auth providers to a single account via the AuthProvider table.

## Additional Features

### Student "Raise Hand" / Help Queue (MVP)
Student clicks a button when stuck → enters a visible queue on teacher's dashboard with context (current line, recent errors). Teacher works through the queue in priority order.

### Live Code Broadcast (MVP)
Teacher shares their editor to all students' screens for live demonstrations. Students see teacher's code updating in real-time. Inverse of the monitoring feature — reuses the same Yjs infrastructure.

### Code Playback (Phase 2)
Replay the full edit history of a student's code. Yjs already tracks the operation history. Teacher can watch how a student built their solution — useful for understanding thought process, catching plagiarism, and grading open-ended projects.

### Output Canvas (Phase 2)
HTML5 Canvas for graphical output beyond text console. Turtle graphics, simple games, animations. Critical for engagement in K-5 and middle school.

### Snippet / Template Library (Phase 3)
Teacher pre-loads code snippets or starter templates. Students can insert them to reduce boilerplate and stay focused on the concept being taught.

## Phasing

### MVP (Phase 1)
- Auth: Google OAuth + email/password, teacher/student roles
- Classroom creation, join codes, student roster
- Live sessions: teacher starts, students join
- Browser editor: CodeMirror 6 with Python (Pyodide) execution
- Real-time dashboard: teacher sees all student code tiles via Yjs
- Teacher ↔ student collaborative editing
- Teacher live code broadcast
- AI tutor: teacher enables per student, Socratic hints, no solutions
- Code annotations: teacher and AI can comment on specific lines
- Student "raise hand" help queue
- AI interaction logging and teacher visibility

### Phase 2
- Block editor (Blockly) for K-5 students
- JavaScript/HTML/CSS execution
- Assignment creation, submission, and grading
- Code playback (Yjs history replay)
- Output canvas (HTML5 Canvas for visual output)
- AI progress tracking + student skill maps
- AI teacher assistant (pre/during/post session insights)
- Microsoft OAuth

### Phase 3
- Starter curriculum / lesson library
- Assessment and grading tools with rubrics
- Block-to-text transition path
- Analytics dashboard (student progress, class performance)
- Snippet / template library
- LTI integration (Canvas, Google Classroom, Schoology)

## Compliance Considerations

- **COPPA**: Required for students under 13. Teacher/school creates accounts on behalf of students. AI interactions are logged and teacher-visible. No direct data collection from children without school consent.
- **FERPA**: Student education records are protected. Row-level security in Postgres ensures tenant isolation. Data export and deletion capabilities required.
- **Data retention**: Schools control their data. Deletion policies must be configurable per school.
