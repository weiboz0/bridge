# 005 — Session Scheduling

## Problem

Sessions are currently ad-hoc — teachers click "Start Live Session" with no planning. There's no way to:
- Plan sessions in advance with topics
- Set recurring schedules (e.g., "Tuesday/Thursday 2-3pm")
- Let students know when sessions will happen
- Track attendance against a planned schedule

## Design

### Data Model

```
scheduled_sessions
├── id (uuid, PK)
├── class_id (uuid, FK → classes)
├── teacher_id (uuid, FK → users)
├── title (varchar, optional — defaults to class title)
├── scheduled_start (timestamptz, NOT NULL)
├── scheduled_end (timestamptz, NOT NULL)
├── recurrence (jsonb, nullable) — { pattern: "weekly", days: [2,4], until: "2026-06-30" }
├── topic_ids (uuid[], nullable) — topics planned for this session
├── live_session_id (uuid, nullable, FK → live_sessions) — linked when started
├── status (enum: planned, in_progress, completed, cancelled)
├── created_at (timestamptz)
├── updated_at (timestamptz)
```

### Status Flow

```
planned → in_progress (teacher starts session)
planned → cancelled (teacher cancels)
in_progress → completed (session ends)
```

When a teacher starts a session from a schedule entry:
1. Create a `live_sessions` record linked to the `scheduled_sessions.live_session_id`
2. Auto-link the planned topics via `session_topics`
3. Update status to `in_progress`

When the live session ends:
1. Update `scheduled_sessions.status` to `completed`

### Recurrence

For recurring sessions, the `recurrence` JSONB field stores:
```json
{
  "pattern": "weekly",
  "days": [2, 4],       // 0=Sun, 1=Mon, ..., 6=Sat
  "until": "2026-06-30"
}
```

The backend generates individual `scheduled_sessions` rows from the recurrence pattern. This avoids complex recurrence calculation at query time — each occurrence is a concrete row.

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | /api/classes/{id}/schedule | Create scheduled session(s) |
| GET | /api/classes/{id}/schedule | List scheduled sessions for class |
| GET | /api/classes/{id}/schedule/upcoming | Next N upcoming sessions |
| PATCH | /api/schedule/{id} | Update a scheduled session |
| DELETE | /api/schedule/{id} | Cancel a scheduled session |
| POST | /api/schedule/{id}/start | Start a session from a schedule entry |

### UI

**Teacher class detail page:**
- "Schedule" tab alongside existing members/sessions view
- Calendar widget showing upcoming sessions
- "Quick Schedule" button for one-off sessions
- "Recurring" option for weekly patterns

**Student class detail page:**
- "Upcoming Sessions" section showing next scheduled sessions
- Date, time, planned topics
- "Live Now" indicator when a scheduled session is in progress

**Teacher dashboard:**
- "Today's Schedule" section showing sessions for today across all classes

### Timezone Handling

- Stored in UTC (`timestamptz`)
- Displayed in user's local timezone (browser `Intl.DateTimeFormat`)
- Teacher sets schedule in their local timezone, converted to UTC on save

### Migration from Ad-hoc

Existing ad-hoc sessions (no schedule entry) continue to work. The schedule is optional — teachers can still click "Start Live Session" without scheduling. When they do, no `scheduled_sessions` record is created.

## Phases

1. **Schema + API**: Create table, Go store + handlers
2. **Teacher UI**: Schedule creation form, calendar view, start-from-schedule
3. **Student UI**: Upcoming sessions display
4. **Recurrence**: Weekly pattern generation
5. **Dashboard**: Today's schedule widget

## Dependencies

- Plan 021 (frontend migration) — schedule pages use Go API
- Session history (done) — past sessions list on class page
