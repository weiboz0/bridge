package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type LiveSession struct {
	ID          string     `json:"id"`
	ClassID string     `json:"classId"`
	TeacherID   string     `json:"teacherId"`
	Status      string     `json:"status"`
	Settings    string     `json:"settings"`
	StartedAt   time.Time  `json:"startedAt"`
	EndedAt     *time.Time `json:"endedAt"`
}

type SessionParticipant struct {
	SessionID       string     `json:"sessionId"`
	StudentID       string     `json:"studentId"`
	Status          string     `json:"status"`
	JoinedAt        time.Time  `json:"joinedAt"`
	LeftAt          *time.Time `json:"leftAt"`
	HelpRequestedAt *time.Time `json:"helpRequestedAt,omitempty"`
}

type ParticipantWithUser struct {
	SessionID       string     `json:"sessionId"`
	StudentID       string     `json:"studentId"`
	Status          string     `json:"status"`
	JoinedAt        time.Time  `json:"joinedAt"`
	LeftAt          *time.Time `json:"leftAt"`
	HelpRequestedAt *time.Time `json:"helpRequestedAt,omitempty"`
	Name            string     `json:"name"`
	Email           string     `json:"email"`
}

type SessionTopic struct {
	SessionID string `json:"sessionId"`
	TopicID   string `json:"topicId"`
}

type SessionTopicWithDetails struct {
	TopicID       string  `json:"topicId"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	SortOrder     int     `json:"sortOrder"`
	LessonContent string  `json:"lessonContent"`
	StarterCode   *string `json:"starterCode"`
}

type CreateSessionInput struct {
	ClassID string `json:"classId"`
	TeacherID   string `json:"teacherId"`
	Settings    string `json:"settings"`
}

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

const sessionColumns = `id, class_id, teacher_id, status, settings, started_at, ended_at`

func scanSession(row interface{ Scan(...any) error }) (*LiveSession, error) {
	var s LiveSession
	err := row.Scan(&s.ID, &s.ClassID, &s.TeacherID, &s.Status, &s.Settings, &s.StartedAt, &s.EndedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *SessionStore) CreateSession(ctx context.Context, input CreateSessionInput) (*LiveSession, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// End any live session for this classroom
	now := time.Now()
	_, err = tx.ExecContext(ctx,
		`UPDATE sessions SET status = 'ended', ended_at = $1 WHERE class_id = $2 AND status = 'live'`,
		now, input.ClassID)
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
	settings := input.Settings
	if settings == "" {
		settings = "{}"
	}

	var session LiveSession
	err = tx.QueryRowContext(ctx,
		`INSERT INTO sessions (id, class_id, teacher_id, title, status, settings, started_at)
		 VALUES ($1, $2, $3, COALESCE((SELECT title FROM classes WHERE id = $2), 'Untitled session'), 'live', $4, $5)
		 RETURNING `+sessionColumns,
		id, input.ClassID, input.TeacherID, settings, now,
	).Scan(&session.ID, &session.ClassID, &session.TeacherID, &session.Status, &session.Settings, &session.StartedAt, &session.EndedAt)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *SessionStore) GetSession(ctx context.Context, id string) (*LiveSession, error) {
	return scanSession(s.db.QueryRowContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE id = $1`, id))
}

func (s *SessionStore) GetActiveSession(ctx context.Context, classID string) (*LiveSession, error) {
	return scanSession(s.db.QueryRowContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE class_id = $1 AND status = 'live'`, classID))
}

// ListSessionsByClass returns all sessions for a class, most recent first.
func (s *SessionStore) ListSessionsByClass(ctx context.Context, classID string) ([]LiveSession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE class_id = $1 ORDER BY started_at DESC`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []LiveSession
	for rows.Next() {
		var ls LiveSession
		if err := rows.Scan(&ls.ID, &ls.ClassID, &ls.TeacherID, &ls.Status, &ls.Settings, &ls.StartedAt, &ls.EndedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, ls)
	}
	if sessions == nil {
		sessions = []LiveSession{}
	}
	return sessions, rows.Err()
}

// SessionWithParticipantCount is a session with the number of participants.
type SessionWithParticipantCount struct {
	LiveSession
	ParticipantCount int `json:"participantCount"`
}

// ListSessionsWithCounts returns sessions with participant counts.
func (s *SessionStore) ListSessionsWithCounts(ctx context.Context, classID string) ([]SessionWithParticipantCount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ls.id, ls.class_id, ls.teacher_id, ls.status, ls.settings, ls.started_at, ls.ended_at,
		        COALESCE((SELECT count(*) FROM session_participants sp WHERE sp.session_id = ls.id), 0)
		 FROM sessions ls
		 WHERE ls.class_id = $1
		 ORDER BY ls.started_at DESC`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []SessionWithParticipantCount
	for rows.Next() {
		var s SessionWithParticipantCount
		if err := rows.Scan(&s.ID, &s.ClassID, &s.TeacherID, &s.Status, &s.Settings,
			&s.StartedAt, &s.EndedAt, &s.ParticipantCount); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	if sessions == nil {
		sessions = []SessionWithParticipantCount{}
	}
	return sessions, rows.Err()
}

func (s *SessionStore) EndSession(ctx context.Context, id string) (*LiveSession, error) {
	return scanSession(s.db.QueryRowContext(ctx,
		`UPDATE sessions SET status = 'ended', ended_at = $1 WHERE id = $2 RETURNING `+sessionColumns,
		time.Now(), id))
}

func (s *SessionStore) JoinSession(ctx context.Context, sessionID, studentID string) (*SessionParticipant, error) {
	var p SessionParticipant
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO session_participants (session_id, user_id, status, joined_at)
		 VALUES ($1, $2, 'present', $3)
		 ON CONFLICT DO NOTHING
		 RETURNING session_id, user_id, status, joined_at, left_at, help_requested_at`,
		sessionID, studentID, time.Now(),
	).Scan(&p.SessionID, &p.StudentID, &p.Status, &p.JoinedAt, &p.LeftAt, &p.HelpRequestedAt)
	if err == sql.ErrNoRows {
		return nil, nil // already joined
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *SessionStore) LeaveSession(ctx context.Context, sessionID, studentID string) (*SessionParticipant, error) {
	var p SessionParticipant
	err := s.db.QueryRowContext(ctx,
		`UPDATE session_participants SET left_at = $1
		 WHERE session_id = $2 AND user_id = $3 AND left_at IS NULL
		 RETURNING session_id, user_id, status, joined_at, left_at, help_requested_at`,
		time.Now(), sessionID, studentID,
	).Scan(&p.SessionID, &p.StudentID, &p.Status, &p.JoinedAt, &p.LeftAt, &p.HelpRequestedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *SessionStore) GetSessionParticipants(ctx context.Context, sessionID string) ([]ParticipantWithUser, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT sp.session_id, sp.user_id, sp.status, sp.joined_at, sp.left_at, sp.help_requested_at, u.name, u.email
		 FROM session_participants sp
		 INNER JOIN users u ON sp.user_id = u.id
		 WHERE sp.session_id = $1
		 ORDER BY sp.joined_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []ParticipantWithUser
	for rows.Next() {
		var p ParticipantWithUser
		if err := rows.Scan(&p.SessionID, &p.StudentID, &p.Status, &p.JoinedAt, &p.LeftAt, &p.HelpRequestedAt, &p.Name, &p.Email); err != nil {
			return nil, err
		}
		participants = append(participants, p)
	}
	if participants == nil {
		participants = []ParticipantWithUser{}
	}
	return participants, rows.Err()
}

func (s *SessionStore) UpdateParticipantStatus(ctx context.Context, sessionID, studentID, status string) (*SessionParticipant, error) {
	var p SessionParticipant
	query := `UPDATE session_participants SET status = $1 WHERE session_id = $2 AND user_id = $3
		RETURNING session_id, user_id, status, joined_at, left_at, help_requested_at`
	args := []any{status, sessionID, studentID}
	switch status {
	case "needs_help":
		query = `UPDATE session_participants
			SET help_requested_at = $1
			WHERE session_id = $2 AND user_id = $3
			RETURNING session_id, user_id, status, joined_at, left_at, help_requested_at`
		args = []any{time.Now(), sessionID, studentID}
	case "active":
		query = `UPDATE session_participants
			SET help_requested_at = NULL
			WHERE session_id = $1 AND user_id = $2
			RETURNING session_id, user_id, status, joined_at, left_at, help_requested_at`
		args = []any{sessionID, studentID}
	}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&p.SessionID, &p.StudentID, &p.Status, &p.JoinedAt, &p.LeftAt, &p.HelpRequestedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// --- Session Topics ---

func (s *SessionStore) LinkSessionTopic(ctx context.Context, sessionID, topicID string) (*SessionTopic, error) {
	var st SessionTopic
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO session_topics (session_id, topic_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING
		 RETURNING session_id, topic_id`,
		sessionID, topicID,
	).Scan(&st.SessionID, &st.TopicID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *SessionStore) UnlinkSessionTopic(ctx context.Context, sessionID, topicID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM session_topics WHERE session_id = $1 AND topic_id = $2`,
		sessionID, topicID)
	return err
}

func (s *SessionStore) GetSessionTopics(ctx context.Context, sessionID string) ([]SessionTopicWithDetails, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.title, t.description, t.sort_order, t.lesson_content, t.starter_code
		 FROM session_topics st
		 INNER JOIN topics t ON st.topic_id = t.id
		 WHERE st.session_id = $1
		 ORDER BY t.sort_order ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []SessionTopicWithDetails
	for rows.Next() {
		var t SessionTopicWithDetails
		if err := rows.Scan(&t.TopicID, &t.Title, &t.Description, &t.SortOrder, &t.LessonContent, &t.StarterCode); err != nil {
			return nil, err
		}
		topics = append(topics, t)
	}
	if topics == nil {
		topics = []SessionTopicWithDetails{}
	}
	return topics, rows.Err()
}
