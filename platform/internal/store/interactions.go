package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type AIInteraction struct {
	ID                 string    `json:"id"`
	StudentID          string    `json:"studentId"`
	SessionID          string    `json:"sessionId"`
	EnabledByTeacherID string    `json:"enabledByTeacherId"`
	Messages           string    `json:"messages"` // JSONB as string
	CreatedAt          time.Time `json:"createdAt"`
}

type CreateInteractionInput struct {
	StudentID          string `json:"studentId"`
	SessionID          string `json:"sessionId"`
	EnabledByTeacherID string `json:"enabledByTeacherId"`
}

type InteractionStore struct {
	db *sql.DB
}

func NewInteractionStore(db *sql.DB) *InteractionStore {
	return &InteractionStore{db: db}
}

const interactionColumns = `id, student_id, session_id, enabled_by_teacher_id, messages, created_at`

func scanInteraction(row interface{ Scan(...any) error }) (*AIInteraction, error) {
	var i AIInteraction
	err := row.Scan(&i.ID, &i.StudentID, &i.SessionID, &i.EnabledByTeacherID, &i.Messages, &i.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (s *InteractionStore) CreateInteraction(ctx context.Context, input CreateInteractionInput) (*AIInteraction, error) {
	id := uuid.New().String()
	return scanInteraction(s.db.QueryRowContext(ctx,
		`INSERT INTO ai_interactions (id, student_id, session_id, enabled_by_teacher_id, messages, created_at)
		 VALUES ($1, $2, $3, $4, '[]', $5)
		 RETURNING `+interactionColumns,
		id, input.StudentID, input.SessionID, input.EnabledByTeacherID, time.Now(),
	))
}

func (s *InteractionStore) GetInteraction(ctx context.Context, id string) (*AIInteraction, error) {
	return scanInteraction(s.db.QueryRowContext(ctx,
		`SELECT `+interactionColumns+` FROM ai_interactions WHERE id = $1`, id))
}

func (s *InteractionStore) GetActiveInteraction(ctx context.Context, studentID, sessionID string) (*AIInteraction, error) {
	return scanInteraction(s.db.QueryRowContext(ctx,
		`SELECT `+interactionColumns+` FROM ai_interactions WHERE student_id = $1 AND session_id = $2`,
		studentID, sessionID))
}

func (s *InteractionStore) ListInteractionsBySession(ctx context.Context, sessionID string) ([]AIInteraction, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+interactionColumns+` FROM ai_interactions WHERE session_id = $1 ORDER BY created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var interactions []AIInteraction
	for rows.Next() {
		var i AIInteraction
		if err := rows.Scan(&i.ID, &i.StudentID, &i.SessionID, &i.EnabledByTeacherID, &i.Messages, &i.CreatedAt); err != nil {
			return nil, err
		}
		interactions = append(interactions, i)
	}
	if interactions == nil {
		interactions = []AIInteraction{}
	}
	return interactions, rows.Err()
}

type ChatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

func (s *InteractionStore) AppendMessage(ctx context.Context, interactionID string, msg ChatMessage) (*AIInteraction, error) {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return scanInteraction(s.db.QueryRowContext(ctx,
		`UPDATE ai_interactions SET messages = messages || jsonb_build_array($1::jsonb) WHERE id = $2 RETURNING `+interactionColumns,
		string(msgJSON), interactionID))
}
