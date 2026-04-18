package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Problem struct {
	ID          string    `json:"id"`
	TopicID     string    `json:"topicId"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StarterCode *string   `json:"starterCode"`
	Language    string    `json:"language"`
	Order       int       `json:"order"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateProblemInput struct {
	TopicID     string
	CreatedBy   string
	Title       string
	Description string
	StarterCode string // empty string -> NULL
	Language    string
	Order       int
}

type UpdateProblemInput struct {
	Title       *string
	Description *string
	// StarterCode: nil = unchanged; non-nil + empty string = clear (NULL);
	// non-nil + non-empty = set.
	StarterCode *string
	Language    *string
	Order       *int
}

type ProblemStore struct {
	db *sql.DB
}

func NewProblemStore(db *sql.DB) *ProblemStore { return &ProblemStore{db: db} }

const problemColumns = `id, topic_id, title, description, starter_code, language, "order", created_by, created_at, updated_at`

func scanProblem(row interface{ Scan(...any) error }) (*Problem, error) {
	var p Problem
	var starter sql.NullString
	err := row.Scan(&p.ID, &p.TopicID, &p.Title, &p.Description, &starter,
		&p.Language, &p.Order, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if starter.Valid {
		p.StarterCode = &starter.String
	}
	return &p, nil
}

func (s *ProblemStore) CreateProblem(ctx context.Context, input CreateProblemInput) (*Problem, error) {
	var starter any
	if input.StarterCode != "" {
		starter = input.StarterCode
	}
	return scanProblem(s.db.QueryRowContext(ctx,
		`INSERT INTO problems (topic_id, title, description, starter_code, language, "order", created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+problemColumns,
		input.TopicID, input.Title, input.Description, starter, input.Language, input.Order, input.CreatedBy))
}

func (s *ProblemStore) GetProblem(ctx context.Context, id string) (*Problem, error) {
	return scanProblem(s.db.QueryRowContext(ctx,
		`SELECT `+problemColumns+` FROM problems WHERE id = $1`, id))
}

func (s *ProblemStore) ListProblemsByTopic(ctx context.Context, topicID string) ([]Problem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+problemColumns+` FROM problems
		 WHERE topic_id = $1
		 ORDER BY "order" ASC, created_at ASC`, topicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Problem
	for rows.Next() {
		p, err := scanProblem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if out == nil {
		out = []Problem{}
	}
	return out, rows.Err()
}

func (s *ProblemStore) UpdateProblem(ctx context.Context, id string, input UpdateProblemInput) (*Problem, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if input.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *input.Title)
		argIdx++
	}
	if input.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *input.Description)
		argIdx++
	}
	if input.StarterCode != nil {
		setClauses = append(setClauses, fmt.Sprintf("starter_code = $%d", argIdx))
		if *input.StarterCode == "" {
			args = append(args, nil)
		} else {
			args = append(args, *input.StarterCode)
		}
		argIdx++
	}
	if input.Language != nil {
		setClauses = append(setClauses, fmt.Sprintf("language = $%d", argIdx))
		args = append(args, *input.Language)
		argIdx++
	}
	if input.Order != nil {
		setClauses = append(setClauses, fmt.Sprintf(`"order" = $%d`, argIdx))
		args = append(args, *input.Order)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetProblem(ctx, id)
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE problems SET %s WHERE id = $%d RETURNING `+problemColumns,
		strings.Join(setClauses, ", "), argIdx,
	)
	return scanProblem(s.db.QueryRowContext(ctx, query, args...))
}

func (s *ProblemStore) DeleteProblem(ctx context.Context, id string) (*Problem, error) {
	return scanProblem(s.db.QueryRowContext(ctx,
		`DELETE FROM problems WHERE id = $1 RETURNING `+problemColumns, id))
}
