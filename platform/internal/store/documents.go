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
	// Plan 048 phase 7: nav metadata resolved via LEFT JOIN sessions on
	// session_id. Both fields are nullable: a document with no session_id,
	// or a dangling session_id (session row hard-deleted; documents.session_id
	// has no FK constraint per drizzle/0005_code-persistence.sql), returns
	// null for both. The student/code page reads these to construct a
	// navigation target ("Open in class" / "Open live session" / non-clickable).
	ClassID       *string `json:"classId,omitempty"`
	SessionStatus *string `json:"sessionStatus,omitempty"`
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

// docColumnsWithNav adds the LEFT JOIN columns for ClassID and
// SessionStatus. Used by ListDocuments (Plan 048 phase 7) so the
// student My Work page can construct navigation targets without
// per-document N+1 lookups.
const docColumnsWithNav = `d.id, d.owner_id, d.session_id, d.topic_id, d.language,
	d.plain_text, d.created_at, d.updated_at, s.class_id, s.status`

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

	// Plan 048 phase 7: filters apply on the documents row, prefixed
	// with `d.` because the SELECT now joins `sessions s`.
	if filters.OwnerID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("d.owner_id = $%d", argIdx))
		args = append(args, filters.OwnerID)
		argIdx++
	}
	if filters.SessionID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("d.session_id = $%d", argIdx))
		args = append(args, filters.SessionID)
		argIdx++
	}

	if len(whereClauses) == 0 {
		return []Document{}, nil
	}

	// LEFT JOIN sessions to surface ClassID and SessionStatus. Dangling
	// session_id (session row hard-deleted) yields null in both columns
	// — the My Work UI handles that as "non-clickable card".
	query := `SELECT ` + docColumnsWithNav + `
		FROM documents d
		LEFT JOIN sessions s ON s.id = d.session_id
		WHERE ` + strings.Join(whereClauses, " AND ") + `
		ORDER BY d.created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var d Document
		var classID, sessionStatus sql.NullString
		if err := rows.Scan(&d.ID, &d.OwnerID, &d.SessionID, &d.TopicID,
			&d.Language, &d.PlainText, &d.CreatedAt, &d.UpdatedAt,
			&classID, &sessionStatus); err != nil {
			return nil, err
		}
		if classID.Valid {
			v := classID.String
			d.ClassID = &v
		}
		if sessionStatus.Valid {
			v := sessionStatus.String
			d.SessionStatus = &v
		}
		docs = append(docs, d)
	}
	if docs == nil {
		docs = []Document{}
	}
	return docs, rows.Err()
}
