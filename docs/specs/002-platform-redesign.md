# Platform Redesign Spec

## Overview

A comprehensive platform redesign covering organization management, role system, content hierarchy, code persistence, portal-based navigation, live session UX, and parent portal. This replaces the flat classroom model from spec 001 with a structured course → class → classroom → session hierarchy, role-based portals, and production-grade UX.

**Target audience:** K-12 programming education — teachers, students, parents, org admins, guest speakers.

**Core changes from spec 001:**
- Organizations as tenant boundary (not schools directly)
- No role at registration — roles assigned by admins and teachers
- Course → Class → Classroom → Session hierarchy with topics/chapters
- Separate portal routes per role (`/admin/`, `/teacher/`, `/student/`, `/parent/`)
- Code persisted in PostgreSQL
- All-in-one teacher dashboard with presentation, grid, broadcast, AI assistant
- Student flexible layouts (side-by-side or stacked)
- Parent portal with live session viewing

---

## Sub-project 1: Data Model & Role Redesign

### Organizations

Organizations are the tenant boundary. Replace the current `schools` table.

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| name | string | Display name |
| slug | string | URL-friendly identifier, unique |
| type | enum | `school`, `tutoring_center`, `bootcamp`, `other` |
| status | enum | `pending`, `active`, `suspended` |
| contactEmail | string | Primary contact |
| contactName | string | Primary contact name |
| domain | string | Nullable — e.g., `lincoln-high.edu` for verification |
| settings | JSONB | Org-level configuration |
| verifiedAt | timestamp | Nullable — when platform admin verified as school |
| createdAt | timestamp | |
| updatedAt | timestamp | |

**Lifecycle:**
1. Self-service: user fills "Register Your Organization" form → org created as `pending`
2. Platform admin reviews and approves → status becomes `active`
3. For school type: AI can assist verification (check .edu domain, cross-reference public databases)
4. Platform admin can also manually create orgs (for enterprise deals)
5. `suspended` status disables all access for the org

### User & Role System

**Users** register with no role. Roles are assigned through org membership and class membership.

**User table** (modified from spec 001):

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| name | string | Display name |
| email | string | Unique |
| avatarUrl | string | Nullable |
| passwordHash | string | Nullable (empty for OAuth-only) |
| createdAt | timestamp | |
| updatedAt | timestamp | |

Note: `role` and `schoolId` removed from user table. Roles are now in OrgMembership and ClassMembership.

**OrgMembership** (what role a user has in an organization):

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| orgId | UUID | FK → Organization |
| userId | UUID | FK → User |
| role | enum | `org_admin`, `teacher`, `student`, `parent` |
| status | enum | `pending`, `active`, `suspended` |
| invitedBy | UUID | FK → User, nullable |
| createdAt | timestamp | |

A user can have memberships in multiple orgs. A user can have multiple roles in the same org (e.g., teacher + parent).

**Platform admin** is a special flag on the User table (`isPlatformAdmin: boolean`) — not an org-level role.

### Registration Flows

1. **Anyone** → registers with name + Google/email → no role, no org → sees empty dashboard with prompts ("Register Your Organization" or "Waiting to be added to a class")
2. **Org admin** → registers, fills "Register Your Organization" → org created as `pending` → platform admin approves → user becomes `org_admin` of that org
3. **Teacher** → registers, org admin adds them by email → OrgMembership created with role `teacher`, status `active`
4. **Student** → teacher shares class join code → student registers and joins class in one step → OrgMembership auto-created with role `student`
5. **Parent** → teacher adds parent email to student profile OR student shares parent link → parent registers, linked to child → OrgMembership with role `parent`
6. **Guest** → teacher adds any registered user by email to a class with `guest` membership role → no org membership needed

### Course → Class → Classroom Hierarchy

**Course** (reusable curriculum template):

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| orgId | UUID | FK → Organization |
| createdBy | UUID | FK → User (teacher) |
| title | string | e.g., "Intro to Python" |
| description | text | |
| gradeLevel | enum | `K-5`, `6-8`, `9-12` |
| language | enum | `python`, `javascript`, `blockly` |
| isPublished | boolean | Whether visible to other teachers in org |
| createdAt | timestamp | |
| updatedAt | timestamp | |

**Topic** (a chapter/unit within a course):

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| courseId | UUID | FK → Course |
| title | string | e.g., "Variables and Data Types" |
| description | text | |
| sortOrder | int | Position in the course |
| lessonContent | JSONB | Instructions, code examples, diagrams (structured content) |
| starterCode | text | Nullable — pre-filled code for this topic |
| createdAt | timestamp | |
| updatedAt | timestamp | |

**Class** (a specific offering of a course):

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| courseId | UUID | FK → Course |
| orgId | UUID | FK → Organization |
| title | string | e.g., "Intro to Python - Fall 2026 Period 3" |
| term | string | e.g., "Fall 2026" |
| joinCode | string | 8-char code for students to join |
| status | enum | `active`, `archived` |
| createdAt | timestamp | |
| updatedAt | timestamp | |

**ClassMembership** (who's in a class and what role):

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| classId | UUID | FK → Class |
| userId | UUID | FK → User |
| role | enum | `instructor`, `ta`, `student`, `observer`, `guest`, `parent` |
| joinedAt | timestamp | |

**Classroom** (virtual room, 1:1 with class — auto-created):

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| classId | UUID | FK → Class, unique |
| editorMode | enum | `python`, `javascript`, `blockly` |
| settings | JSONB | Room-level config |
| createdAt | timestamp | |

**Session** (a live event in a classroom):

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| classroomId | UUID | FK → Classroom |
| createdBy | UUID | FK → User (teacher who started it) |
| status | enum | `active`, `ended` |
| settings | JSONB | aiEnabled, etc. |
| startedAt | timestamp | |
| endedAt | timestamp | Nullable |

**SessionTopic** (N:N linking — what topics are covered in a session):

| Field | Type | Notes |
|---|---|---|
| sessionId | UUID | FK → Session |
| topicId | UUID | FK → Topic |

**SessionParticipant** (unchanged from spec 001):

| Field | Type | Notes |
|---|---|---|
| sessionId | UUID | FK → Session |
| userId | UUID | FK → User |
| status | enum | `active`, `idle`, `needs_help` |
| joinedAt | timestamp | |
| leftAt | timestamp | Nullable |

### Code Persistence

**Document** (persisted code):

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| ownerId | UUID | FK → User |
| classroomId | UUID | FK → Classroom |
| sessionId | UUID | FK → Session, nullable (null for standalone editor work) |
| topicId | UUID | FK → Topic, nullable |
| language | enum | `python`, `javascript`, `blockly` |
| yjsState | bytea | Yjs encoded state (source of truth while live) |
| plainText | text | Snapshot for search/display/parent viewing |
| updatedAt | timestamp | |
| createdAt | timestamp | |

**Persistence mechanism:**
- Hocuspocus `onStoreDocument` hook saves Yjs binary state to PostgreSQL every 30s and on disconnect
- On session end, a final plain-text snapshot is saved
- Documents keyed by `session:{sessionId}:user:{userId}` (existing convention)
- `onLoadDocument` hook restores Yjs state when reconnecting

### Existing Tables Kept (with modifications)

- **AuthProvider** — unchanged
- **AIInteraction** — `sessionId` FK stays, add `topicId` FK
- **CodeAnnotation** — unchanged, `documentId` string links to Document

---

## Sub-project 2: Portal & Navigation Overhaul

### Portal Routes

| Portal | Route prefix | Who sees it |
|---|---|---|
| Platform Admin | `/admin/` | Users with `isPlatformAdmin` flag |
| Org Admin | `/org/` | Users with `org_admin` OrgMembership |
| Teacher | `/teacher/` | Users with `teacher` OrgMembership |
| Student | `/student/` | Users with `student` OrgMembership or ClassMembership |
| Parent | `/parent/` | Users with `parent` OrgMembership or linked children |

Unaffiliated users (no roles) see `/` with prompts to register an org or wait for an invitation.

Users with multiple roles see a **role switcher** in the sidebar to navigate between portals.

### Sidebar Navigation

Adopts the magicburg pattern: fixed left sidebar, collapsible, icon-driven.

**Per-portal nav items:**

| Portal | Navigation items |
|---|---|
| Admin | Organizations, Users, System Settings |
| Org Admin | Dashboard, Teachers, Students, Courses, Classes, Settings |
| Teacher | Dashboard, My Courses, My Classes, Schedule, Reports |
| Student | Dashboard, My Classes, My Code, Help |
| Parent | Dashboard, My Children, Reports |

### Theming

- HSL-based CSS variables (from magicburg)
- Light/dark toggle in sidebar footer, persisted in localStorage
- Geist font (already in use)
- Consistent color palette across all portals

### Landing Page

The current landing page (`/`) becomes:
- If not logged in: marketing/login page
- If logged in with roles: redirect to primary portal
- If logged in without roles: onboarding prompt ("Register Your Organization" or "You'll be added to a class by your teacher")

---

## Sub-project 3: Course & Content Management

### Teacher Course Workflow

1. Teacher creates a **Course** (title, description, grade level, language)
2. Adds **Topics** in order (each with title, lesson content, starter code)
3. **Lesson content** per topic is structured JSONB:
   ```json
   {
     "blocks": [
       { "type": "markdown", "content": "# Variables\n\nA variable stores a value..." },
       { "type": "code", "language": "python", "content": "x = 5\nprint(x)" },
       { "type": "image", "url": "/uploads/diagram.png", "alt": "Variable diagram" }
     ]
   }
   ```
4. Publishes the course (visible to other teachers in org for cloning)
5. Creates a **Class** from the course (assigns term, generates join code)
6. Class auto-creates a **Classroom**

### Assignment System

Assignments are tied to topics:

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| topicId | UUID | FK → Topic |
| classId | UUID | FK → Class |
| title | string | |
| description | text | |
| starterCode | text | Nullable |
| dueDate | timestamp | Nullable |
| rubric | JSONB | Grading criteria |
| createdAt | timestamp | |

**Submission:**

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| assignmentId | UUID | FK → Assignment |
| studentId | UUID | FK → User |
| documentId | UUID | FK → Document |
| grade | float | Nullable |
| feedback | text | Nullable |
| submittedAt | timestamp | |

### Additional Language Support

- **Blockly** (K-5): Google Blockly editor, transpiles to JS, runs in browser
- **JavaScript/HTML/CSS**: native browser execution, iframe sandbox, HTML preview pane
- Editor mode set per course/classroom, determines which editor loads

---

## Sub-project 4: Live Session Redesign

### Teacher Dashboard (All-in-One)

Single page at `/teacher/classes/[classId]/session/[sessionId]/dashboard`

**3 resizable/collapsible panels:**

```
┌──────────────────────────────────────────────────────────┐
│  Header: topic name, timer, student count, [End Session] │
├────────────┬──────────────────────────┬──────────────────┤
│            │                          │                  │
│  Student   │   Main Area             │  AI Assistant    │
│  List      │   (mode switching):     │  Panel           │
│  (collaps.)│                          │  (collapsible)   │
│            │   [Presentation]         │                  │
│  - name    │   [Student Grid]        │  - recommendations│
│  - status  │   [Collaborate]         │  - help queue    │
│  - AI ●    │   [Broadcast]           │  - activity feed │
│  - ✋      │                          │  - annotations   │
│            │                          │                  │
├────────────┴──────────────────────────┴──────────────────┤
│  Toolbar: mode tabs + session controls                    │
└──────────────────────────────────────────────────────────┘
```

**Main area modes:**
1. **Presentation** — renders lesson content from the selected topic(s)
2. **Student Grid** — miniaturized code tiles per student (existing, improved)
3. **Collaborative Edit** — full editor synced with a selected student, annotation sidebar
4. **Broadcast** — teacher's editor mirrored to all students

**Panel behaviors:**
- Each panel can be collapsed to give more space
- Layout preference persisted per teacher
- Student list always shows hand-raised students at top

**AI Assistant panel shows:**
- Real-time recommendations: "80% of students have the same error on line 5"
- Help queue (students who raised hand, sorted by wait time)
- AI interaction feed (which students are chatting with AI, message counts)
- Suggested next actions: "3 students are ahead — consider an extension challenge"

### Student Session View

Page at `/student/classes/[classId]/session/[sessionId]`

**Layout options (student selects):**
- **Side-by-side** — lesson content left, code editor right (default for wide screens)
- **Stacked** — lesson content top, code editor bottom (default for narrow screens)
- Layout toggle in toolbar, preference persisted in localStorage

**Side panel (toggleable, minimizable):**
- AI Chat
- Annotations
- Minimized state shows notification badges (unread messages, new annotations)

**Output panel:** collapsible below the editor in both layouts

**When teacher broadcasts:**
- Broadcast editor appears prominently (top of main area or overlay)
- Student's own editor remains accessible below

### Standalone Editor

Available at `/teacher/editor` and `/student/editor`
- Same layout options as student session view (side-by-side or stacked)
- No Yjs — local only, with save to PostgreSQL
- For teachers: lesson content authoring, demo preparation
- For students: practice outside of sessions

---

## Sub-project 5: Parent Portal

### Route: `/parent/...`

### Dashboard (`/parent`)
- Cards for each linked child showing: name, org, classes enrolled, last activity, overall progress
- "Live Now" indicator when a child is in an active session

### Child Detail (`/parent/children/[id]`)
- **Attendance** — sessions attended vs total, per class
- **Progress** — topics covered, completion status, grades on assignments
- **Recent Activity** — timeline of sessions with topics covered

### Code Viewing (`/parent/children/[id]/code`)
- Browse by class → topic → session
- Read-only plain-text view with syntax highlighting (no editor, no Yjs)
- Teacher annotations visible

### Live Session Viewing (`/parent/children/[id]/live`)
- Read-only view when child is in active session
- Shows: lesson content, child's code (Yjs read-only subscription), output panel
- Can see teacher annotations appearing in real-time
- Cannot interact — no chat, no editing, no raise hand
- Parent connects as `observer` to child's Yjs document

### AI Progress Reports (`/parent/children/[id]/reports`)
- AI-generated weekly summaries in parent-friendly language
- Generated from: attendance, AI interaction logs, teacher annotations, code submissions, grades
- Example: "Alice attended 4 of 5 sessions this week. She completed the 'Loops' topic and started 'Functions'. Her teacher noted good debugging skills. She asked the AI tutor 6 questions, mostly about loop syntax."

### Linking Parent to Student
- Teacher adds parent email to a student's class membership
- Student shares a parent invite link
- Parent registers and sees their linked children automatically

---

## Migration Strategy

This redesign modifies existing tables significantly. Migration approach:

1. **Sub-project 1** creates new tables (Organization, OrgMembership, Course, Topic, Class, ClassMembership, Classroom, SessionTopic, Document) and migrates data from old tables (schools → organizations, classrooms → classes + classrooms, users.role → OrgMembership)
2. **Sub-project 2** adds new portal routes alongside existing ones, then removes old routes
3. **Sub-projects 3-5** are additive — no destructive migrations

### Backward Compatibility

During sub-project 1, maintain backward compat:
- Keep old API routes working during transition
- Migrate existing classrooms to new class + classroom structure
- Migrate existing user roles to OrgMembership records
- Remove old routes/tables after new ones are verified

---

## What's Deferred (Not in This Spec)

- Block-to-text transition path (Blockly → Python guided transition)
- LTI integration (Canvas, Google Classroom, Schoology)
- Mobile native app
- Video/audio in sessions (screen share, voice chat)
- Billing/subscription system
- Multi-language i18n
