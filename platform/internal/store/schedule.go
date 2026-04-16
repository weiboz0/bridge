package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ScheduledSession struct {
	ID             string     `json:"id"`
	ClassID        string     `json:"classId"`
	TeacherID      string     `json:"teacherId"`
	Title          *string    `json:"title"`
	ScheduledStart time.Time  `json:"scheduledStart"`
	ScheduledEnd   time.Time  `json:"scheduledEnd"`
	Recurrence     *string    `json:"recurrence"`   // JSONB as string
	TopicIDs       []string   `json:"topicIds"`
	LiveSessionID  *string    `json:"liveSessionId"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type CreateScheduleInput struct {
	ClassID        string    `json:"classId"`
	TeacherID      string    `json:"teacherId"`
	Title          *string   `json:"title"`
	ScheduledStart time.Time `json:"scheduledStart"`
	ScheduledEnd   time.Time `json:"scheduledEnd"`
	Recurrence     *string   `json:"recurrence"`
	TopicIDs       []string  `json:"topicIds"`
}

type UpdateScheduleInput struct {
	Title          *string    `json:"title,omitempty"`
	ScheduledStart *time.Time `json:"scheduledStart,omitempty"`
	ScheduledEnd   *time.Time `json:"scheduledEnd,omitempty"`
	TopicIDs       []string   `json:"topicIds,omitempty"`
}

type ScheduleStore struct {
	db *sql.DB
}

func NewScheduleStore(db *sql.DB) *ScheduleStore {
	return &ScheduleStore{db: db}
}

const scheduleColumns = `id, class_id, teacher_id, title, scheduled_start, scheduled_end, recurrence, topic_ids, live_session_id, status, created_at, updated_at`

func scanSchedule(row interface{ Scan(...any) error }) (*ScheduledSession, error) {
	var s ScheduledSession
	var topicIDs []byte // PostgreSQL UUID[] comes as text
	err := row.Scan(&s.ID, &s.ClassID, &s.TeacherID, &s.Title,
		&s.ScheduledStart, &s.ScheduledEnd, &s.Recurrence, &topicIDs,
		&s.LiveSessionID, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.TopicIDs = parseUUIDArray(topicIDs)
	return &s, nil
}

// parseUUIDArray parses PostgreSQL UUID[] text representation.
func parseUUIDArray(b []byte) []string {
	if b == nil || len(b) == 0 || string(b) == "{}" {
		return []string{}
	}
	str := strings.Trim(string(b), "{}")
	if str == "" {
		return []string{}
	}
	return strings.Split(str, ",")
}

// formatUUIDArray formats a string slice as PostgreSQL UUID[] literal.
func formatUUIDArray(ids []string) string {
	if len(ids) == 0 {
		return "{}"
	}
	return "{" + strings.Join(ids, ",") + "}"
}

func (s *ScheduleStore) CreateSchedule(ctx context.Context, input CreateScheduleInput) (*ScheduledSession, error) {
	id := uuid.New().String()
	now := time.Now()
	topicArr := formatUUIDArray(input.TopicIDs)

	return scanSchedule(s.db.QueryRowContext(ctx,
		`INSERT INTO scheduled_sessions (id, class_id, teacher_id, title, scheduled_start, scheduled_end, recurrence, topic_ids, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::uuid[], 'planned', $9, $10)
		 RETURNING `+scheduleColumns,
		id, input.ClassID, input.TeacherID, input.Title,
		input.ScheduledStart, input.ScheduledEnd, input.Recurrence,
		topicArr, now, now,
	))
}

func (s *ScheduleStore) GetSchedule(ctx context.Context, id string) (*ScheduledSession, error) {
	return scanSchedule(s.db.QueryRowContext(ctx,
		`SELECT `+scheduleColumns+` FROM scheduled_sessions WHERE id = $1`, id))
}

func (s *ScheduleStore) ListByClass(ctx context.Context, classID string) ([]ScheduledSession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+scheduleColumns+` FROM scheduled_sessions WHERE class_id = $1 ORDER BY scheduled_start ASC`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSchedules(rows)
}

func (s *ScheduleStore) ListUpcoming(ctx context.Context, classID string, limit int) ([]ScheduledSession, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+scheduleColumns+` FROM scheduled_sessions
		 WHERE class_id = $1 AND status = 'planned' AND scheduled_start > now()
		 ORDER BY scheduled_start ASC LIMIT $2`, classID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSchedules(rows)
}

func scanSchedules(rows *sql.Rows) ([]ScheduledSession, error) {
	var sessions []ScheduledSession
	for rows.Next() {
		var s ScheduledSession
		var topicIDs []byte
		if err := rows.Scan(&s.ID, &s.ClassID, &s.TeacherID, &s.Title,
			&s.ScheduledStart, &s.ScheduledEnd, &s.Recurrence, &topicIDs,
			&s.LiveSessionID, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.TopicIDs = parseUUIDArray(topicIDs)
		sessions = append(sessions, s)
	}
	if sessions == nil {
		sessions = []ScheduledSession{}
	}
	return sessions, rows.Err()
}

func (s *ScheduleStore) UpdateSchedule(ctx context.Context, id string, input UpdateScheduleInput) (*ScheduledSession, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if input.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *input.Title)
		argIdx++
	}
	if input.ScheduledStart != nil {
		setClauses = append(setClauses, fmt.Sprintf("scheduled_start = $%d", argIdx))
		args = append(args, *input.ScheduledStart)
		argIdx++
	}
	if input.ScheduledEnd != nil {
		setClauses = append(setClauses, fmt.Sprintf("scheduled_end = $%d", argIdx))
		args = append(args, *input.ScheduledEnd)
		argIdx++
	}
	if input.TopicIDs != nil {
		setClauses = append(setClauses, fmt.Sprintf("topic_ids = $%d::uuid[]", argIdx))
		args = append(args, formatUUIDArray(input.TopicIDs))
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetSchedule(ctx, id)
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE scheduled_sessions SET %s WHERE id = $%d AND status = 'planned' RETURNING `+scheduleColumns,
		strings.Join(setClauses, ", "), argIdx,
	)
	return scanSchedule(s.db.QueryRowContext(ctx, query, args...))
}

func (s *ScheduleStore) CancelSchedule(ctx context.Context, id string) (*ScheduledSession, error) {
	return scanSchedule(s.db.QueryRowContext(ctx,
		`UPDATE scheduled_sessions SET status = 'cancelled', updated_at = $1
		 WHERE id = $2 AND status = 'planned'
		 RETURNING `+scheduleColumns,
		time.Now(), id))
}

func (s *ScheduleStore) StartScheduledSession(ctx context.Context, scheduleID, teacherID string, sessionStore *SessionStore) (*LiveSession, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get the schedule entry
	var sched ScheduledSession
	var topicIDs []byte
	err = tx.QueryRowContext(ctx,
		`SELECT `+scheduleColumns+` FROM scheduled_sessions WHERE id = $1 AND status = 'planned' FOR UPDATE`, scheduleID,
	).Scan(&sched.ID, &sched.ClassID, &sched.TeacherID, &sched.Title,
		&sched.ScheduledStart, &sched.ScheduledEnd, &sched.Recurrence, &topicIDs,
		&sched.LiveSessionID, &sched.Status, &sched.CreatedAt, &sched.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("schedule not found or not in planned status")
	}
	if err != nil {
		return nil, err
	}
	sched.TopicIDs = parseUUIDArray(topicIDs)

	// End any active session for this class
	now := time.Now()
	_, err = tx.ExecContext(ctx,
		`UPDATE live_sessions SET status = 'ended', ended_at = $1 WHERE class_id = $2 AND status = 'active'`,
		now, sched.ClassID)
	if err != nil {
		return nil, err
	}

	// Create live session
	sessionID := uuid.New().String()
	var session LiveSession
	err = tx.QueryRowContext(ctx,
		`INSERT INTO live_sessions (id, class_id, teacher_id, status, settings, started_at)
		 VALUES ($1, $2, $3, 'active', '{}', $4)
		 RETURNING id, class_id, teacher_id, status, settings, started_at, ended_at`,
		sessionID, sched.ClassID, teacherID, now,
	).Scan(&session.ID, &session.ClassID, &session.TeacherID, &session.Status,
		&session.Settings, &session.StartedAt, &session.EndedAt)
	if err != nil {
		return nil, err
	}

	// Link planned topics to session
	for _, topicID := range sched.TopicIDs {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO session_topics (session_id, topic_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			sessionID, topicID)
		if err != nil {
			return nil, err
		}
	}

	// Update schedule entry
	_, err = tx.ExecContext(ctx,
		`UPDATE scheduled_sessions SET status = 'in_progress', live_session_id = $1, updated_at = $2 WHERE id = $3`,
		sessionID, now, scheduleID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &session, nil
}

// CompleteScheduledSession updates the schedule status when a live session ends.
func (s *ScheduleStore) CompleteScheduledSession(ctx context.Context, liveSessionID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE scheduled_sessions SET status = 'completed', updated_at = $1
		 WHERE live_session_id = $2 AND status = 'in_progress'`,
		time.Now(), liveSessionID)
	return err
}
