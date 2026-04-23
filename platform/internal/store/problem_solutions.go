package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// ErrAlreadyAttached is returned by TopicProblemStore.Attach when the
// (topic, problem) pair already exists. Handlers should map this to HTTP 409.
var ErrAlreadyAttached = errors.New("problem already attached to topic")

// ProblemSolution is a reference solution for a problem, optionally published
// to students. ApproachTags (e.g. ["brute-force", "dynamic-programming"]) let
// teachers annotate the algorithmic strategy used.
type ProblemSolution struct {
	ID           string    `json:"id"`
	ProblemID    string    `json:"problemId"`
	Language     string    `json:"language"`
	Title        *string   `json:"title"`
	Code         string    `json:"code"`
	Notes        *string   `json:"notes"`
	ApproachTags []string  `json:"approachTags"`
	IsPublished  bool      `json:"isPublished"`
	CreatedBy    string    `json:"createdBy"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// CreateSolutionInput carries the fields required to create a new solution.
type CreateSolutionInput struct {
	ProblemID    string
	Language     string
	Title        *string
	Code         string
	Notes        *string
	ApproachTags []string
	CreatedBy    string
}

// UpdateSolutionInput carries partial updates. Nil fields are left unchanged.
// ApproachTags nil = unchanged; empty slice = clear to '{}'.
type UpdateSolutionInput struct {
	Title        *string
	Code         *string
	Notes        *string
	ApproachTags []string // nil = unchanged
}

// ProblemSolutionStore manages the problem_solutions table.
type ProblemSolutionStore struct{ db *sql.DB }

// NewProblemSolutionStore constructs a ProblemSolutionStore.
func NewProblemSolutionStore(db *sql.DB) *ProblemSolutionStore { return &ProblemSolutionStore{db: db} }

const solutionColumns = `id, problem_id, language, title, code, notes, approach_tags,
  is_published, created_by, created_at, updated_at`

// scanSolution scans one problem_solutions row; returns nil on sql.ErrNoRows.
func scanSolution(row interface{ Scan(...any) error }) (*ProblemSolution, error) {
	var s ProblemSolution
	var title, notes sql.NullString
	var tags pq.StringArray

	if err := row.Scan(
		&s.ID, &s.ProblemID, &s.Language, &title, &s.Code, &notes, &tags,
		&s.IsPublished, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if title.Valid {
		v := title.String
		s.Title = &v
	}
	if notes.Valid {
		v := notes.String
		s.Notes = &v
	}
	if tags == nil {
		s.ApproachTags = []string{}
	} else {
		s.ApproachTags = []string(tags)
	}
	return &s, nil
}

// CreateSolution inserts a new solution row. ApproachTags nil is coerced to
// an empty slice so the column is never NULL.
func (s *ProblemSolutionStore) CreateSolution(ctx context.Context, in CreateSolutionInput) (*ProblemSolution, error) {
	if in.ApproachTags == nil {
		in.ApproachTags = []string{}
	}
	return scanSolution(s.db.QueryRowContext(ctx, `
        INSERT INTO problem_solutions (
          problem_id, language, title, code, notes, approach_tags, created_by
        ) VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING `+solutionColumns,
		in.ProblemID, in.Language, in.Title, in.Code, in.Notes,
		pq.Array(in.ApproachTags), in.CreatedBy,
	))
}

// GetSolution returns the solution with the given id, or nil if not found.
func (s *ProblemSolutionStore) GetSolution(ctx context.Context, id string) (*ProblemSolution, error) {
	return scanSolution(s.db.QueryRowContext(ctx,
		`SELECT `+solutionColumns+` FROM problem_solutions WHERE id = $1`, id))
}

// ListByProblem returns all solutions for a problem. When includeDrafts is
// false only is_published = true rows are returned.
func (s *ProblemSolutionStore) ListByProblem(ctx context.Context, problemID string, includeDrafts bool) ([]ProblemSolution, error) {
	q := `SELECT ` + solutionColumns + ` FROM problem_solutions WHERE problem_id = $1`
	if !includeDrafts {
		q += ` AND is_published = true`
	}
	q += ` ORDER BY created_at ASC`

	rows, err := s.db.QueryContext(ctx, q, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ProblemSolution{}
	for rows.Next() {
		sol, err := scanSolution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sol)
	}
	return out, rows.Err()
}

// UpdateSolution applies partial updates. Nil fields are left untouched.
// Pass an empty ApproachTags slice to clear to '{}'.
func (s *ProblemSolutionStore) UpdateSolution(ctx context.Context, id string, in UpdateSolutionInput) (*ProblemSolution, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if in.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		if *in.Title == "" {
			args = append(args, nil)
		} else {
			args = append(args, *in.Title)
		}
		argIdx++
	}
	if in.Code != nil {
		setClauses = append(setClauses, fmt.Sprintf("code = $%d", argIdx))
		args = append(args, *in.Code)
		argIdx++
	}
	if in.Notes != nil {
		setClauses = append(setClauses, fmt.Sprintf("notes = $%d", argIdx))
		if *in.Notes == "" {
			args = append(args, nil)
		} else {
			args = append(args, *in.Notes)
		}
		argIdx++
	}
	if in.ApproachTags != nil {
		setClauses = append(setClauses, fmt.Sprintf("approach_tags = $%d", argIdx))
		args = append(args, pq.Array(in.ApproachTags))
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetSolution(ctx, id)
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	args = append(args, id)
	q := fmt.Sprintf(
		`UPDATE problem_solutions SET %s WHERE id = $%d RETURNING `+solutionColumns,
		strings.Join(setClauses, ", "), argIdx,
	)
	return scanSolution(s.db.QueryRowContext(ctx, q, args...))
}

// SetPublished sets is_published on a solution. The call is idempotent —
// publishing an already-published solution (or unpublishing a draft) is
// accepted with no error.
func (s *ProblemSolutionStore) SetPublished(ctx context.Context, id string, published bool) (*ProblemSolution, error) {
	return scanSolution(s.db.QueryRowContext(ctx, `
        UPDATE problem_solutions
        SET is_published = $1, updated_at = now()
        WHERE id = $2
        RETURNING `+solutionColumns,
		published, id,
	))
}

// DeleteSolution removes the solution and returns the deleted row (or nil).
func (s *ProblemSolutionStore) DeleteSolution(ctx context.Context, id string) (*ProblemSolution, error) {
	return scanSolution(s.db.QueryRowContext(ctx,
		`DELETE FROM problem_solutions WHERE id = $1 RETURNING `+solutionColumns, id))
}
