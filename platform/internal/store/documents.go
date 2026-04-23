package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Document struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"ownerId"`
	SessionID *string   `json:"sessionId"`
	TopicID   *string   `json:"topicId"`
	Language  string    `json:"language"`
	YjsState  *string   `json:"yjsState,omitempty"`
	PlainText string    `json:"plainText"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type DocumentContent struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"ownerId"`
	Language  string    `json:"language"`
	PlainText string    `json:"plainText"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type DocumentFilters struct {
	OwnerID   string
	SessionID string
}

type DocumentStore struct {
	db *sql.DB
}

func NewDocumentStore(db *sql.DB) *DocumentStore {
	return &DocumentStore{db: db}
}

const docColumns = `id, owner_id, session_id, topic_id, language, plain_text, created_at, updated_at`

func (s *DocumentStore) GetDocument(ctx context.Context, id string) (*Document, error) {
	var d Document
	err := s.db.QueryRowContext(ctx,
		`SELECT `+docColumns+` FROM documents WHERE id = $1`, id,
	).Scan(&d.ID, &d.OwnerID, &d.SessionID, &d.TopicID,
		&d.Language, &d.PlainText, &d.CreatedAt, &d.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *DocumentStore) ListDocuments(ctx context.Context, filters DocumentFilters) ([]Document, error) {
	whereClauses := []string{}
	args := []any{}
	argIdx := 1

	if filters.OwnerID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("owner_id = $%d", argIdx))
		args = append(args, filters.OwnerID)
		argIdx++
	}
	if filters.SessionID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("session_id = $%d", argIdx))
		args = append(args, filters.SessionID)
		argIdx++
	}

	if len(whereClauses) == 0 {
		return []Document{}, nil
	}

	query := `SELECT ` + docColumns + ` FROM documents WHERE ` +
		strings.Join(whereClauses, " AND ") + ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.OwnerID, &d.SessionID, &d.TopicID,
			&d.Language, &d.PlainText, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	if docs == nil {
		docs = []Document{}
	}
	return docs, rows.Err()
}
