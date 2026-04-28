package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors for token-based session join and direct-add.
var (
	ErrTokenNotFound = errors.New("invite token not found")
	ErrTokenExpired  = errors.New("invite token expired")
	ErrSessionEnded  = errors.New("session has ended")
	ErrUserNotFound  = errors.New("user not found")
)

type LiveSession struct {
	ID              string     `json:"id"`
	ClassID         *string    `json:"classId"`
	TeacherID       string     `json:"teacherId"`
	Title           string     `json:"title"`
	Status          string     `json:"status"`
	Settings        string     `json:"settings"`
	InviteToken     *string    `json:"inviteToken,omitempty"`
	InviteExpiresAt *time.Time `json:"inviteExpiresAt,omitempty"`
	StartedAt       time.Time  `json:"startedAt"`
	EndedAt         *time.Time `json:"endedAt"`
}

type SessionParticipant struct {
	SessionID       string     `json:"sessionId"`
	StudentID       string     `json:"studentId"`
	Status          string     `json:"status"`
	InvitedBy       *string    `json:"invitedBy,omitempty"`
	InvitedAt       *time.Time `json:"invitedAt,omitempty"`
	JoinedAt        *time.Time `json:"joinedAt,omitempty"`
	LeftAt          *time.Time `json:"leftAt,omitempty"`
	HelpRequestedAt *time.Time `json:"helpRequestedAt,omitempty"`
}

type ParticipantWithUser struct {
	SessionID       string     `json:"sessionId"`
	StudentID       string     `json:"studentId"`
	Status          string     `json:"status"`
	InvitedBy       *string    `json:"invitedBy,omitempty"`
	InvitedAt       *time.Time `json:"invitedAt,omitempty"`
	JoinedAt        *time.Time `json:"joinedAt,omitempty"`
	LeftAt          *time.Time `json:"leftAt,omitempty"`
	HelpRequestedAt *time.Time `json:"helpRequestedAt,omitempty"`
	Name            string     `json:"name"`
	Email           string     `json:"email"`
}

type SessionTopic struct {
	SessionID string `json:"sessionId"`
	TopicID   string `json:"topicId"`
}

type SessionTopicWithDetails struct {
	TopicID     string `json:"topicId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	SortOrder   int    `json:"sortOrder"`
	// Plan 044 phase 1: linked Unit identity surfaced. Null when no
	// Unit is linked OR when the Unit's scope_id doesn't match the
	// topic's course org (cross-org-leak guard in the JOIN).
	UnitID           *string `json:"unitId"`
	UnitTitle        *string `json:"unitTitle"`
	UnitMaterialType *string `json:"unitMaterialType"`
}

type CreateSessionInput struct {
	ClassID   *string `json:"classId"`
	TeacherID string  `json:"teacherId"`
	Title     string  `json:"title"`
	Settings  string  `json:"settings"`
}

type ListSessionsFilter struct {
	TeacherID       string
	ClassID         *string
	Status          string
	Limit           int
	CursorStartedAt *time.Time
	CursorID        *string
}

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

const sessionColumns = `id, class_id, teacher_id, title, status, settings, invite_token, invite_expires_at, started_at, ended_at`
const participantColumns = `session_id, user_id, status, invited_by, invited_at, joined_at, left_at, help_requested_at`

func scanSession(row interface{ Scan(...any) error }) (*LiveSession, error) {
	var s LiveSession
	err := row.Scan(&s.ID, &s.ClassID, &s.TeacherID, &s.Title, &s.Status, &s.Settings,
		&s.InviteToken, &s.InviteExpiresAt, &s.StartedAt, &s.EndedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func scanParticipant(row interface{ Scan(...any) error }) (*SessionParticipant, error) {
	var p SessionParticipant
	err := row.Scan(&p.SessionID, &p.StudentID, &p.Status, &p.InvitedBy, &p.InvitedAt,
		&p.JoinedAt, &p.LeftAt, &p.HelpRequestedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *SessionStore) CreateSession(ctx context.Context, input CreateSessionInput) (*LiveSession, error) {
	if strings.TrimSpace(input.Title) == "" {
		return nil, errors.New("title is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now()
	if input.ClassID != nil {
		// End any live session for this classroom.
		_, err = tx.ExecContext(ctx,
			`UPDATE sessions SET status = 'ended', ended_at = $1 WHERE class_id = $2 AND status = 'live'`,
			now, input.ClassID)
		if err != nil {
			return nil, err
		}
	}

	id := uuid.New().String()
	settings := input.Settings
	if settings == "" {
		settings = "{}"
	}

	var session LiveSession
	err = tx.QueryRowContext(ctx,
		`INSERT INTO sessions (id, class_id, teacher_id, title, status, settings, started_at)
		 VALUES ($1, $2, $3, $4, 'live', $5, $6)
		 RETURNING `+sessionColumns,
		id, input.ClassID, input.TeacherID, input.Title, settings, now,
	).Scan(&session.ID, &session.ClassID, &session.TeacherID, &session.Title, &session.Status, &session.Settings,
		&session.InviteToken, &session.InviteExpiresAt, &session.StartedAt, &session.EndedAt)
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

// ListSessions returns sessions matching the supplied filters, paginated by
// (started_at DESC, id DESC). An empty TeacherID/Status means "any".
func (s *SessionStore) ListSessions(ctx context.Context, f ListSessionsFilter) ([]LiveSession, bool, error) {
	where := []string{}
	args := []any{}
	idx := 1

	if f.TeacherID != "" {
		where = append(where, fmt.Sprintf("teacher_id = $%d", idx))
		args = append(args, f.TeacherID)
		idx++
	}
	if f.ClassID != nil {
		where = append(where, fmt.Sprintf("class_id = $%d", idx))
		args = append(args, *f.ClassID)
		idx++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", idx))
		args = append(args, f.Status)
		idx++
	}
	if f.CursorStartedAt != nil && f.CursorID != nil {
		where = append(where, fmt.Sprintf("(started_at, id) < ($%d, $%d)", idx, idx+1))
		args = append(args, *f.CursorStartedAt, *f.CursorID)
		idx += 2
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	q := `SELECT ` + sessionColumns + ` FROM sessions`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	q += fmt.Sprintf(` ORDER BY started_at DESC, id DESC LIMIT %d`, limit+1)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	out := []LiveSession{}
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, *session)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return out, hasMore, nil
}

// ListSessionsByClass returns all sessions for a class, most recent first.
func (s *SessionStore) ListSessionsByClass(ctx context.Context, classID string) ([]LiveSession, error) {
	filter := ListSessionsFilter{
		ClassID: &classID,
		Limit:   100,
	}

	all := []LiveSession{}
	for {
		page, hasMore, err := s.ListSessions(ctx, filter)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if !hasMore || len(page) == 0 {
			if all == nil {
				all = []LiveSession{}
			}
			return all, nil
		}

		last := page[len(page)-1]
		filter.CursorStartedAt = &last.StartedAt
		filter.CursorID = &last.ID
	}
}

// SessionWithParticipantCount is a session with the number of participants.
type SessionWithParticipantCount struct {
	LiveSession
	ParticipantCount int `json:"participantCount"`
}

// ListSessionsWithCounts returns sessions with participant counts.
func (s *SessionStore) ListSessionsWithCounts(ctx context.Context, classID string) ([]SessionWithParticipantCount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ls.id, ls.class_id, ls.teacher_id, ls.title, ls.status, ls.settings,
		        ls.invite_token, ls.invite_expires_at, ls.started_at, ls.ended_at,
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
		if err := rows.Scan(&s.ID, &s.ClassID, &s.TeacherID, &s.Title, &s.Status, &s.Settings,
			&s.InviteToken, &s.InviteExpiresAt, &s.StartedAt, &s.EndedAt, &s.ParticipantCount); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	if sessions == nil {
		sessions = []SessionWithParticipantCount{}
	}
	return sessions, rows.Err()
}

// UpdateSessionInput describes mutable session fields for a partial update.
type UpdateSessionInput struct {
	Title           *string    `json:"title"`
	Settings        *string    `json:"settings"`
	InviteExpiresAt *time.Time `json:"inviteExpiresAt"`
	// ClearInviteExpiry is true when the caller explicitly sets inviteExpiresAt to null.
	ClearInviteExpiry bool `json:"-"`
}

// UpdateSession performs a partial update on the mutable session fields
// (title, settings, invite_expires_at). Only non-nil fields are applied.
func (s *SessionStore) UpdateSession(ctx context.Context, id string, input UpdateSessionInput) (*LiveSession, error) {
	return scanSession(s.db.QueryRowContext(ctx,
		`UPDATE sessions SET
			title = COALESCE($1, title),
			settings = COALESCE($2, settings),
			invite_expires_at = CASE
				WHEN $4 THEN NULL
				WHEN $3::timestamptz IS NOT NULL THEN $3
				ELSE invite_expires_at
			END,
			updated_at = now()
		 WHERE id = $5
		 RETURNING `+sessionColumns,
		input.Title, input.Settings, input.InviteExpiresAt, input.ClearInviteExpiry, id))
}

func (s *SessionStore) EndSession(ctx context.Context, id string) (*LiveSession, error) {
	return scanSession(s.db.QueryRowContext(ctx,
		`UPDATE sessions SET status = 'ended', ended_at = $1 WHERE id = $2 RETURNING `+sessionColumns,
		time.Now(), id))
}

func (s *SessionStore) JoinSession(ctx context.Context, sessionID, studentID string) (*SessionParticipant, error) {
	// Plan 043 Codex post-impl review: a pre-invited row (status='invited',
	// joined_at=NULL) was previously left untouched because of
	// ON CONFLICT DO NOTHING. Now: invited → present and joined_at gets set
	// so the teacher's roster reflects the actual join. Already-present
	// rows are left alone (the user is just re-asserting). 'left' rows
	// remain rejected by the handler-level canJoinSession gate before we
	// reach this query — they need a fresh invite to come back.
	return scanParticipant(s.db.QueryRowContext(ctx,
		`INSERT INTO session_participants (session_id, user_id, status, joined_at)
		 VALUES ($1, $2, 'present', $3)
		 ON CONFLICT (session_id, user_id) DO UPDATE
		   SET status = 'present',
		       joined_at = COALESCE(session_participants.joined_at, EXCLUDED.joined_at)
		   WHERE session_participants.status = 'invited'
		 RETURNING `+participantColumns,
		sessionID, studentID, time.Now(),
	))
}

func (s *SessionStore) LeaveSession(ctx context.Context, sessionID, studentID string) (*SessionParticipant, error) {
	return scanParticipant(s.db.QueryRowContext(ctx,
		`UPDATE session_participants SET status = 'left', left_at = $1
		 WHERE session_id = $2 AND user_id = $3 AND left_at IS NULL
		 RETURNING `+participantColumns,
		time.Now(), sessionID, studentID,
	))
}

// GetSessionParticipant returns the single participant row for (sessionID, userID).
// Returns (nil, nil) when the user has no row for the session.
//
// Plan 043 Phase 1 P0: callers use this to check whether a non-class-member
// is pre-invited and may join the session. Status "left" still surfaces a
// row — callers must filter on status themselves to enforce "invited|present
// only" semantics (a user who left should not get re-entry without a fresh
// invite).
func (s *SessionStore) GetSessionParticipant(ctx context.Context, sessionID, userID string) (*SessionParticipant, error) {
	return scanParticipant(s.db.QueryRowContext(ctx,
		`SELECT `+participantColumns+`
		 FROM session_participants
		 WHERE session_id = $1 AND user_id = $2`,
		sessionID, userID,
	))
}

func (s *SessionStore) GetSessionParticipants(ctx context.Context, sessionID string) ([]ParticipantWithUser, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT sp.session_id, sp.user_id, sp.status, sp.invited_by, sp.invited_at,
		        sp.joined_at, sp.left_at, sp.help_requested_at, u.name, u.email
		 FROM session_participants sp
		 INNER JOIN users u ON sp.user_id = u.id
		 WHERE sp.session_id = $1
		 ORDER BY sp.joined_at NULLS LAST`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []ParticipantWithUser
	for rows.Next() {
		var p ParticipantWithUser
		if err := rows.Scan(&p.SessionID, &p.StudentID, &p.Status, &p.InvitedBy, &p.InvitedAt,
			&p.JoinedAt, &p.LeftAt, &p.HelpRequestedAt, &p.Name, &p.Email); err != nil {
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
	query := `UPDATE session_participants SET status = $1 WHERE session_id = $2 AND user_id = $3
		RETURNING ` + participantColumns
	args := []any{status, sessionID, studentID}
	switch status {
	case "needs_help":
		query = `UPDATE session_participants
			SET help_requested_at = $1
			WHERE session_id = $2 AND user_id = $3
			RETURNING ` + participantColumns
		args = []any{time.Now(), sessionID, studentID}
	case "active":
		query = `UPDATE session_participants
			SET help_requested_at = NULL
			WHERE session_id = $1 AND user_id = $2
			RETURNING ` + participantColumns
		args = []any{sessionID, studentID}
	case "invited", "present", "left":
	default:
		return nil, fmt.Errorf("unsupported participant status %q", status)
	}
	return scanParticipant(s.db.QueryRowContext(ctx, query, args...))
}

// --- Direct-Add Participant Methods ---

// AddParticipant inserts a participant with status 'invited'. If the user
// already has a row and their status is 'left', they are re-invited. If their
// status is 'invited' or 'present', the existing row is returned unchanged.
func (s *SessionStore) AddParticipant(ctx context.Context, sessionID, userID, invitedBy string) (*SessionParticipant, error) {
	now := time.Now()
	return scanParticipant(s.db.QueryRowContext(ctx,
		`INSERT INTO session_participants (session_id, user_id, status, invited_by, invited_at, joined_at)
		 VALUES ($1, $2, 'invited', $3, $4, NULL)
		 ON CONFLICT (session_id, user_id) DO UPDATE SET
			status = CASE
				WHEN session_participants.status = 'left' THEN 'invited'::participant_status
				ELSE session_participants.status
			END,
			invited_by = CASE
				WHEN session_participants.status = 'left' THEN EXCLUDED.invited_by
				ELSE session_participants.invited_by
			END,
			invited_at = CASE
				WHEN session_participants.status = 'left' THEN EXCLUDED.invited_at
				ELSE session_participants.invited_at
			END,
			left_at = CASE
				WHEN session_participants.status = 'left' THEN NULL
				ELSE session_participants.left_at
			END
		 RETURNING `+participantColumns,
		sessionID, userID, invitedBy, now,
	))
}

// AddParticipantByEmail looks up a user by email and calls AddParticipant.
// Returns ErrUserNotFound if the email does not match any user.
func (s *SessionStore) AddParticipantByEmail(ctx context.Context, sessionID, email, invitedBy string) (*SessionParticipant, error) {
	userStore := NewUserStore(s.db)
	user, err := userStore.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return s.AddParticipant(ctx, sessionID, user.ID, invitedBy)
}

// RemoveParticipant deletes the participant row entirely.
// Returns true if a row was deleted.
func (s *SessionStore) RemoveParticipant(ctx context.Context, sessionID, userID string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM session_participants WHERE session_id = $1 AND user_id = $2`,
		sessionID, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// PromoteToPresent sets an invited or left participant to 'present' and records
// joined_at. If the participant is already present, the existing row is returned.
func (s *SessionStore) PromoteToPresent(ctx context.Context, sessionID, userID string) (*SessionParticipant, error) {
	now := time.Now()
	// Try to promote — only if status is 'invited' or 'left'
	p, err := scanParticipant(s.db.QueryRowContext(ctx,
		`UPDATE session_participants
		 SET status = 'present', joined_at = $1
		 WHERE session_id = $2 AND user_id = $3 AND status IN ('invited', 'left')
		 RETURNING `+participantColumns,
		now, sessionID, userID,
	))
	if err != nil {
		return nil, err
	}
	if p != nil {
		return p, nil
	}
	// No rows updated — either already present or not found. Fetch existing.
	return scanParticipant(s.db.QueryRowContext(ctx,
		`SELECT `+participantColumns+` FROM session_participants
		 WHERE session_id = $1 AND user_id = $2`,
		sessionID, userID,
	))
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
	// Plan 044 phase 1: LEFT JOIN against teaching_units to surface the
	// linked Unit (1:1 via teaching_units.topic_id unique index). The
	// outer JOIN on courses + the scope/scope_id check is the cross-org
	// leak guard from Codex correction #3 — a teaching_unit's scope_id
	// must match the topic's course org_id (or be platform-scope).
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.title, t.description, t.sort_order,
		        u.id, u.title, u.material_type
		 FROM session_topics st
		 INNER JOIN topics t ON st.topic_id = t.id
		 INNER JOIN courses c ON c.id = t.course_id
		 LEFT JOIN teaching_units u
		   ON u.topic_id = t.id
		   AND (u.scope = 'platform' OR u.scope_id = c.org_id)
		 WHERE st.session_id = $1
		 ORDER BY t.sort_order ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []SessionTopicWithDetails
	for rows.Next() {
		var t SessionTopicWithDetails
		if err := rows.Scan(
			&t.TopicID, &t.Title, &t.Description, &t.SortOrder,
			&t.UnitID, &t.UnitTitle, &t.UnitMaterialType,
		); err != nil {
			return nil, err
		}
		topics = append(topics, t)
	}
	if topics == nil {
		topics = []SessionTopicWithDetails{}
	}
	return topics, rows.Err()
}

// --- Invite Token Methods ---

// base62Alphabet is used for generating URL-safe invite tokens.
const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// generateInviteToken generates a cryptographically secure random 24-character
// base62-encoded token suitable for use in URLs.
func generateInviteToken() (string, error) {
	b := make([]byte, 24)
	alphabetLen := big.NewInt(int64(len(base62Alphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			return "", fmt.Errorf("generating invite token: %w", err)
		}
		b[i] = base62Alphabet[n.Int64()]
	}
	return string(b), nil
}

// GetSessionByToken fetches a session by its invite_token.
// Returns (nil, nil) if no session has that token.
func (s *SessionStore) GetSessionByToken(ctx context.Context, token string) (*LiveSession, error) {
	return scanSession(s.db.QueryRowContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE invite_token = $1`, token))
}

// RotateInviteToken generates a new invite token for the session, invalidating
// any previous token immediately. Returns the updated session.
func (s *SessionStore) RotateInviteToken(ctx context.Context, sessionID string) (*LiveSession, error) {
	token, err := generateInviteToken()
	if err != nil {
		return nil, err
	}
	return scanSession(s.db.QueryRowContext(ctx,
		`UPDATE sessions SET invite_token = $1, updated_at = now()
		 WHERE id = $2
		 RETURNING `+sessionColumns,
		token, sessionID))
}

// SetInviteExpiry sets or clears the invite_expires_at timestamp.
// Pass nil to remove the expiry (open lobby).
func (s *SessionStore) SetInviteExpiry(ctx context.Context, sessionID string, expiresAt *time.Time) (*LiveSession, error) {
	return scanSession(s.db.QueryRowContext(ctx,
		`UPDATE sessions SET invite_expires_at = $1, updated_at = now()
		 WHERE id = $2
		 RETURNING `+sessionColumns,
		expiresAt, sessionID))
}

// RevokeInviteToken clears both invite_token and invite_expires_at,
// making any existing invite link dead.
func (s *SessionStore) RevokeInviteToken(ctx context.Context, sessionID string) (*LiveSession, error) {
	return scanSession(s.db.QueryRowContext(ctx,
		`UPDATE sessions SET invite_token = NULL, invite_expires_at = NULL, updated_at = now()
		 WHERE id = $1
		 RETURNING `+sessionColumns,
		sessionID))
}

// CanAccessSession checks whether a user may access a session.
// Returns (allowed, reason, err) where reason is one of:
// "teacher", "class_member", "participant", "not_found", "ended", "no_access".
func (s *SessionStore) CanAccessSession(ctx context.Context, sessionID, userID string) (bool, string, error) {
	var status, teacherID string
	var classID *string
	err := s.db.QueryRowContext(ctx,
		`SELECT status, teacher_id, class_id FROM sessions WHERE id = $1`, sessionID,
	).Scan(&status, &teacherID, &classID)
	if err == sql.ErrNoRows {
		return false, "not_found", nil
	}
	if err != nil {
		return false, "", err
	}

	if status == "ended" {
		return false, "ended", nil
	}

	if teacherID == userID {
		return true, "teacher", nil
	}

	// Check class membership if session belongs to a class
	if classID != nil {
		var exists bool
		err = s.db.QueryRowContext(ctx,
			`SELECT EXISTS(
				SELECT 1 FROM class_memberships
				WHERE class_id = $1 AND user_id = $2
			)`, *classID, userID,
		).Scan(&exists)
		if err != nil {
			return false, "", err
		}
		if exists {
			return true, "class_member", nil
		}
	}

	// Check participant row (invited or present)
	var participantExists bool
	err = s.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM session_participants
			WHERE session_id = $1 AND user_id = $2 AND status IN ('invited', 'present')
		)`, sessionID, userID,
	).Scan(&participantExists)
	if err != nil {
		return false, "", err
	}
	if participantExists {
		return true, "participant", nil
	}

	return false, "no_access", nil
}

// JoinSessionByToken validates the invite token and adds the user as a
// participant with status 'present'. It returns sentinel errors for
// invalid/expired tokens and ended sessions. If the user is already a
// participant, the existing row is returned.
func (s *SessionStore) JoinSessionByToken(ctx context.Context, sessionID, userID, token string) (*SessionParticipant, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Fetch session and validate token within the transaction
	var sessionStatus string
	var inviteToken *string
	var inviteExpiresAt *time.Time
	err = tx.QueryRowContext(ctx,
		`SELECT status, invite_token, invite_expires_at FROM sessions WHERE id = $1 FOR UPDATE`,
		sessionID,
	).Scan(&sessionStatus, &inviteToken, &inviteExpiresAt)
	if err == sql.ErrNoRows {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, err
	}

	if sessionStatus == "ended" {
		return nil, ErrSessionEnded
	}

	if inviteToken == nil || *inviteToken != token {
		return nil, ErrTokenNotFound
	}

	if inviteExpiresAt != nil && inviteExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	// Insert participant (ON CONFLICT DO NOTHING for idempotency)
	var p SessionParticipant
	err = tx.QueryRowContext(ctx,
		`INSERT INTO session_participants (session_id, user_id, status, joined_at)
		 VALUES ($1, $2, 'present', $3)
		 ON CONFLICT (session_id, user_id) DO NOTHING
		 RETURNING `+participantColumns,
		sessionID, userID, time.Now(),
	).Scan(&p.SessionID, &p.StudentID, &p.Status, &p.InvitedBy, &p.InvitedAt,
		&p.JoinedAt, &p.LeftAt, &p.HelpRequestedAt)
	if err == sql.ErrNoRows {
		// Already a participant — fetch existing row
		err = tx.QueryRowContext(ctx,
			`SELECT `+participantColumns+`
			 FROM session_participants WHERE session_id = $1 AND user_id = $2`,
			sessionID, userID,
		).Scan(&p.SessionID, &p.StudentID, &p.Status, &p.InvitedBy, &p.InvitedAt,
			&p.JoinedAt, &p.LeftAt, &p.HelpRequestedAt)
		if err != nil {
			return nil, err
		}
		// No need to commit — read-only at this point
		return &p, tx.Commit()
	}
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &p, nil
}
