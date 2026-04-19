# 010 — Session Model

## Problem

Today a live session is tightly bound to a class: `live_sessions.class_id` is `NOT NULL`, participants are implicitly any class member, and there is no way to:

- Run an ad-hoc coding session without first creating a class (tutoring, drop-in office hours, a one-off workshop for two kids).
- Invite a guest (parent, visiting teacher, student from another class) to a specific session.
- Directly enroll named users in a session before it starts — the teacher has no "roster" surface for sessions, only for classes.
- Close a shareable invite link after a deadline or once the session starts, so "late" joiners can't wander in.

This spec unifies live and past sessions behind one `sessions` table, makes class association optional, and introduces three orthogonal invite mechanisms — class membership, direct add, and time-bounded invite token — that compose cleanly for every real-world pattern.

## Design

### Data Model

#### `sessions` (renamed from `live_sessions`, schema evolved)

```
sessions
├── id                  uuid PK
├── class_id            uuid NULL    → FK classes(id); NULL = orphan session
├── teacher_id          uuid NOT NULL → FK users(id); session creator
├── title               varchar(255) NOT NULL
├── invite_token        varchar(24) NULL UNIQUE  → presence gates token-based join
├── invite_expires_at   timestamptz NULL   → lobby deadline (NULL = no timer)
├── status              enum {live, ended}   (derived from ended_at; kept for query speed)
├── settings            jsonb DEFAULT '{}'
├── scheduled_session_id uuid NULL     → FK scheduled_sessions(id); backref when started from a schedule
├── started_at          timestamptz NOT NULL DEFAULT now()
├── ended_at            timestamptz NULL
├── created_at          timestamptz NOT NULL DEFAULT now()
└── updated_at          timestamptz NOT NULL DEFAULT now()
```

Renames:
- Table `live_sessions` → `sessions`. "Live" is now a status value, not a table identity — past sessions live in the same table with `ended_at != NULL`.

New columns:
- `class_id` drops `NOT NULL`.
- `title`: required. Every session has a human label. For class-associated sessions, defaults to the class title; for orphan, teacher sets it.
- `invite_token`, `invite_expires_at`: see below.
- `scheduled_session_id`: if the session was started from a pre-schedule, link back.

Dropped:
- `live_sessions.classroom_id` (already renamed to `class_id` in Plan 022; now fully gone).

#### `session_participants` (extended)

```
session_participants
├── session_id     uuid NOT NULL
├── user_id        uuid NOT NULL   → renamed from student_id
├── status         enum {invited, present, left}   → adds "invited"
├── invited_by     uuid NULL   → NEW; FK users(id); who pre-added them
├── invited_at     timestamptz NULL   → NEW
├── joined_at      timestamptz NULL   → now set on first actual connect
├── left_at        timestamptz NULL
└── PRIMARY KEY (session_id, user_id)
```

Rename: `student_id` → `user_id`. Teachers, parents, guests can also appear here; the old name was misleading.

Status lifecycle:

```
(nothing)  ──[invite]──▶  invited
invited    ──[join]────▶  present
present    ──[leave]───▶  left
left       ──[return]──▶  present
(any)      ──[revoke]──▶  row deleted
```

### Three invite mechanisms (orthogonal, any one suffices)

```
User U can connect to session S iff:
  (1) class_memberships has a row (S.class_id, U), OR
  (2) session_participants has (S, U) with status ∈ {invited, present, left}, OR
  (3) request carries a valid invite_token whose invite_expires_at > NOW()
```

Additionally, in every branch: `S.ended_at IS NULL`. An ended session is read-only for everyone; connecting to the live editor is refused with `410 Gone`.

**1. Class membership** — automatic for class-associated sessions. Any `class_memberships` row in `S.class_id` lets the user connect. No extra rows needed.

**2. Direct add** — teacher / org admin / platform admin explicitly enrolls a user before or during the session. Writes a `session_participants` row with `status="invited"` and `invited_by=caller`. When the user connects, status flips to `present`.

**3. Invite token** — a shareable URL of the form `/s/{invite_token}` that anyone can click. The token is rotatable; the lobby can be closed on a timer or on demand. See below.

### Invite token semantics

**States the token can be in:**

| Token state | Effect on `/s/{token}` |
|---|---|
| `invite_token = NULL` | Route doesn't exist for this session (no token issued). |
| token present, `invite_expires_at = NULL` | Works until session ends. |
| token present, `invite_expires_at > NOW()` | Works; still in lobby window. |
| token present, `invite_expires_at <= NOW()` | `410 Gone`. |
| session `ended_at IS NOT NULL` | `410 Gone`. Token never works after session end. |
| token rotated (new one issued) | Old token → `404`; new token begins life with caller-provided `invite_expires_at`. |

**Common teacher intents, one knob:**

| Intent | Set | Behavior |
|---|---|---|
| "Closed the moment we start" | `invite_expires_at = scheduled_start` | Hard cutoff at session start. |
| "10-min grace window" | `invite_expires_at = scheduled_start + 10m` | Late joiners welcome 10 min. |
| "Open all session" | `invite_expires_at = NULL` | Works until `ended_at` is set. |
| "Close lobby now" | teacher sets `invite_expires_at = NOW()` | Immediate close; existing participants unaffected. |

**Status codes on failed join:**

| Situation | Response |
|---|---|
| Token unknown (never issued, or rotated away) | `404 Not Found` |
| Token valid, session ended | `410 Gone` |
| Token valid, `invite_expires_at` past | `410 Gone` |

404 for unknown, 410 for "was once valid, now isn't" — distinguishes accidental-typo from "you're too late."

### Access summary

| Action | Required authority |
|---|---|
| Create class-associated session | instructor in the class, or class's org_admin, or platform admin |
| Create orphan session | any authenticated user (teacher role); creator becomes `teacher_id` |
| Add a participant directly | session creator; or class instructor (if class-associated); or class's org_admin; or platform admin |
| Revoke a participant | same as add |
| Rotate invite token | same as add |
| Close lobby (`invite_expires_at = NOW()`) | same as add |
| End session | session creator; or class instructor (if class-associated); or platform admin |
| Connect to live editor | any of the three join mechanisms |
| Read past session (transcript, code, test results) | session creator; class members (if class-associated); platform admin |

### API sketch

```
POST   /api/sessions                           create a new session
       body: { title, classId?, scheduledSessionId?, settings?, inviteToken?: "generate", inviteExpiresAt? }
       returns: Session + optional invite_url

PATCH  /api/sessions/{id}                       update title/settings/invite_expires_at
POST   /api/sessions/{id}/end                   → set ended_at = NOW()
POST   /api/sessions/{id}/rotate-invite         → new token, optional fresh invite_expires_at
DELETE /api/sessions/{id}/invite                → clears invite_token + invite_expires_at

GET    /api/sessions/{id}                       details (auth: any of the 3 mechanisms)
GET    /api/sessions?classId=...                list for class
GET    /api/sessions?teacherId=me               list for me (as creator)
GET    /api/sessions?status=ended&classId=...   past-session history

POST   /api/sessions/{id}/participants          direct-add
       body: { userId } or { email }            (email resolves to user_id; 404 if unknown)
       effect: session_participants { status:"invited", invited_by: caller }

DELETE /api/sessions/{id}/participants/{userId} revoke (row deleted)

POST   /api/s/{invite_token}/join               token-based join
       returns 404 | 410 | 200 per state table above
       effect: session_participants upsert { status:"present" }

GET    /api/sessions/{id}/participants          roster (auth: creator/instructor/admin)
       returns: [{ user, status, invitedBy, invitedAt, joinedAt, leftAt }]
```

Hocuspocus door stays the same: connecting to `session:{id}:user:{uid}` is gated by one of the three mechanisms; the Go auth endpoint tells Hocuspocus "allow / deny / read-only."

### Scheduled sessions

`scheduled_sessions` (already exists, plan 022) is unchanged on schema. Integration touch-ups:

- Starting a schedule now creates a `sessions` row with `scheduled_session_id = schedule.id`. Cleaner lineage than today's `live_session_id` backref on the schedule.
- Canceling a schedule doesn't cascade to ended sessions — if a session was started from a cancelled schedule, it keeps running.

### Non-goals

- **Email delivery for invitations to non-users.** Phase 2. For now `POST /participants { email }` returns 404 when the email doesn't resolve; the teacher needs to get the user signed up out of band first.
- **Per-participant permissions.** Today everyone in a session has the same capabilities based on their app role (teacher vs student). No "read-only participant" or "observer" distinction. Add later if a real need appears.
- **Recording / playback.** Out of scope.
- **Waiting room** (participant must be admitted by the teacher). Different product decision — orthogonal to invite tokens and not in this spec.

## Migration

**New columns on `sessions`** (was `live_sessions`):

```sql
-- Rename table first
ALTER TABLE live_sessions RENAME TO sessions;

-- Relax class_id
ALTER TABLE sessions ALTER COLUMN class_id DROP NOT NULL;

-- Add invite fields
ALTER TABLE sessions
  ADD COLUMN title varchar(255) NOT NULL DEFAULT 'Untitled session',
  ADD COLUMN invite_token varchar(24) UNIQUE,
  ADD COLUMN invite_expires_at timestamptz,
  ADD COLUMN scheduled_session_id uuid REFERENCES scheduled_sessions(id) ON DELETE SET NULL;

-- Backfill title from class (best-effort)
UPDATE sessions s
SET title = COALESCE(c.title, 'Untitled session')
FROM classes c WHERE c.id = s.class_id AND s.title = 'Untitled session';

-- Drop the DEFAULT after backfill
ALTER TABLE sessions ALTER COLUMN title DROP DEFAULT;
```

**Participants rename + extend:**

```sql
ALTER TABLE session_participants RENAME COLUMN student_id TO user_id;

-- Add "invited" to the enum
ALTER TYPE session_participant_status ADD VALUE 'invited';

-- Invitation audit columns
ALTER TABLE session_participants
  ADD COLUMN invited_by uuid REFERENCES users(id) ON DELETE SET NULL,
  ADD COLUMN invited_at timestamptz;

-- joined_at already exists; now nullable so invited-but-never-joined rows don't lie
ALTER TABLE session_participants ALTER COLUMN joined_at DROP NOT NULL;
```

**Scheduled sessions backref flip:**

```sql
-- If scheduled_sessions had live_session_id pointing at live_sessions(id),
-- that FK still works (same rows, renamed table). No-op.
-- New canonical backref is sessions.scheduled_session_id; keep old one until
-- callers migrate, then drop in a later plan.
```

## Rollout

Phased so each phase is a mergeable PR and no phase breaks running sessions.

**Phase 1 — Schema (breaking-safe).** Rename `live_sessions → sessions`, add new columns as nullable, rename `student_id → user_id` on participants. Hocuspocus / Go stores recompile against the renamed table. No behavior change. Tag: Plan 030a.

**Phase 2 — Invite tokens.** Implement token issuance, rotation, expiry check. `POST /s/{token}/join`. Token generation defaults to NULL (no token) on session create — existing create paths unaffected. Tag: Plan 030b.

**Phase 3 — Direct-add.** `session_participants` status adds `invited`. New POST/DELETE on `/participants`. Hocuspocus join gate consults the participants table as a third mechanism. Tag: Plan 030c.

**Phase 4 — Orphan sessions.** Create-session API accepts `classId = null`. UI exposes "New session" button on teacher dashboard (not just on a class). Tag: Plan 030d.

**Phase 5 — Scheduled-session backref flip.** Use `sessions.scheduled_session_id` as canonical. Drop old backref once no callers remain. Tag: Plan 030e (can be delayed indefinitely; not user-visible).

Each phase is ~200–400 lines. The order matters: phases 1–3 refit existing behavior without introducing orphans; phase 4 is the only one that exposes new product capability.

## Open questions

- **Invite token length/charset.** 24 chars of `[a-zA-Z0-9]` → ~143 bits entropy. Sufficient to make guessing impractical. Alternative: shorter, friendly-coded (like class join codes) with a rate limiter on the join endpoint. Pick in implementation review.
- **Session-level org binding.** An orphan session has no `class_id`, so no implicit `org_id`. Do we want `session.org_id NULL` for orphan-ish bookkeeping (quotas, admin filters)? Probably yes in Phase 4; cheap to add.
- **Participant duplication across attempts.** A user joining multiple sessions produces multiple `session_participants` rows (one per session). Not a duplication issue — intended. Just worth noting.

## Follow-ups referenced but out of scope here

- Rich topic content with AI authoring → **spec 012**.
- Problem bank ownership scopes → **spec 009** (informs what a session can reference beyond class-linked topics).
- Portal UI redesign that surfaces the new session capabilities per persona → **spec 011**.
