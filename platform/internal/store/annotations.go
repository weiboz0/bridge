package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Annotation struct {
	ID         string     `json:"id"`
	DocumentID string     `json:"documentId"`
	AuthorID   string     `json:"authorId"`
	AuthorType string     `json:"authorType"`
	LineStart  string     `json:"lineStart"`
	LineEnd    string     `json:"lineEnd"`
	Content    string     `json:"content"`
	Resolved   *time.Time `json:"resolved"`
	CreatedAt  time.Time  `json:"createdAt"`
}

type CreateAnnotationInput struct {
	DocumentID string `json:"documentId"`
	AuthorID   string `json:"authorId"`
	AuthorType string `json:"authorType"`
	LineStart  string `json:"lineStart"`
	LineEnd    string `json:"lineEnd"`
	Content    string `json:"content"`
}

type AnnotationStore struct {
	db *sql.DB
}

func NewAnnotationStore(db *sql.DB) *AnnotationStore {
	return &AnnotationStore{db: db}
}

const annotationColumns = `id, document_id, author_id, author_type, line_start, line_end, content, resolved_at, created_at`

func scanAnnotation(row interface{ Scan(...any) error }) (*Annotation, error) {
	var a Annotation
	err := row.Scan(&a.ID, &a.DocumentID, &a.AuthorID, &a.AuthorType,
		&a.LineStart, &a.LineEnd, &a.Content, &a.Resolved, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// GetAnnotation looks up a single annotation by ID. Plan 056: used
// by Delete/Resolve to fetch the annotation's documentID before
// authorizing the mutation.
func (s *AnnotationStore) GetAnnotation(ctx context.Context, id string) (*Annotation, error) {
	return scanAnnotation(s.db.QueryRowContext(ctx,
		`SELECT `+annotationColumns+` FROM code_annotations WHERE id = $1`, id))
}

func (s *AnnotationStore) CreateAnnotation(ctx context.Context, input CreateAnnotationInput) (*Annotation, error) {
	id := uuid.New().String()
	return scanAnnotation(s.db.QueryRowContext(ctx,
		`INSERT INTO code_annotations (id, document_id, author_id, author_type, line_start, line_end, content, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING `+annotationColumns,
		id, input.DocumentID, input.AuthorID, input.AuthorType,
		input.LineStart, input.LineEnd, input.Content, time.Now(),
	))
}

func (s *AnnotationStore) ListAnnotations(ctx context.Context, documentID string) ([]Annotation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+annotationColumns+` FROM code_annotations WHERE document_id = $1 ORDER BY created_at`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var annotations []Annotation
	for rows.Next() {
		var a Annotation
		if err := rows.Scan(&a.ID, &a.DocumentID, &a.AuthorID, &a.AuthorType,
			&a.LineStart, &a.LineEnd, &a.Content, &a.Resolved, &a.CreatedAt); err != nil {
			return nil, err
		}
		annotations = append(annotations, a)
	}
	if annotations == nil {
		annotations = []Annotation{}
	}
	return annotations, rows.Err()
}

func (s *AnnotationStore) DeleteAnnotation(ctx context.Context, id string) (*Annotation, error) {
	return scanAnnotation(s.db.QueryRowContext(ctx,
		`DELETE FROM code_annotations WHERE id = $1 RETURNING `+annotationColumns, id))
}

func (s *AnnotationStore) ResolveAnnotation(ctx context.Context, id string) (*Annotation, error) {
	return scanAnnotation(s.db.QueryRowContext(ctx,
		`UPDATE code_annotations SET resolved_at = $1 WHERE id = $2 RETURNING `+annotationColumns,
		time.Now(), id))
}
