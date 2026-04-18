package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type TestCase struct {
	ID             string    `json:"id"`
	ProblemID      string    `json:"problemId"`
	OwnerID        *string   `json:"ownerId"` // nil = canonical
	Name           string    `json:"name"`
	Stdin          string    `json:"stdin"`
	ExpectedStdout *string   `json:"expectedStdout"`
	IsExample      bool      `json:"isExample"`
	Order          int       `json:"order"`
	CreatedAt      time.Time `json:"createdAt"`
}

type CreateTestCaseInput struct {
	ProblemID      string
	OwnerID        *string // nil = canonical
	Name           string
	Stdin          string
	ExpectedStdout *string
	IsExample      bool
	Order          int
}

type UpdateTestCaseInput struct {
	Name           *string
	Stdin          *string
	// ExpectedStdout: nil = unchanged; non-nil + empty = clear (NULL);
	// non-nil + non-empty = set.
	ExpectedStdout *string
	IsExample      *bool
	Order          *int
}

type TestCaseStore struct {
	db *sql.DB
}

func NewTestCaseStore(db *sql.DB) *TestCaseStore { return &TestCaseStore{db: db} }

const testCaseColumns = `id, problem_id, owner_id, name, stdin, expected_stdout, is_example, "order", created_at`

func scanTestCase(row interface{ Scan(...any) error }) (*TestCase, error) {
	var c TestCase
	var owner, expected sql.NullString
	err := row.Scan(&c.ID, &c.ProblemID, &owner, &c.Name, &c.Stdin, &expected,
		&c.IsExample, &c.Order, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if owner.Valid {
		c.OwnerID = &owner.String
	}
	if expected.Valid {
		c.ExpectedStdout = &expected.String
	}
	return &c, nil
}

func (s *TestCaseStore) CreateTestCase(ctx context.Context, input CreateTestCaseInput) (*TestCase, error) {
	return scanTestCase(s.db.QueryRowContext(ctx,
		`INSERT INTO test_cases (problem_id, owner_id, name, stdin, expected_stdout, is_example, "order")
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+testCaseColumns,
		input.ProblemID, input.OwnerID, input.Name, input.Stdin, input.ExpectedStdout, input.IsExample, input.Order))
}

func (s *TestCaseStore) GetTestCase(ctx context.Context, id string) (*TestCase, error) {
	return scanTestCase(s.db.QueryRowContext(ctx,
		`SELECT `+testCaseColumns+` FROM test_cases WHERE id = $1`, id))
}

// ListForViewer returns canonical cases (owner_id IS NULL) plus the viewer's
// own private cases. Hidden vs example is NOT filtered here — the handler
// layer decides what to expose based on whether the viewer is the problem's
// author.
func (s *TestCaseStore) ListForViewer(ctx context.Context, problemID, viewerID string) ([]TestCase, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+testCaseColumns+` FROM test_cases
		 WHERE problem_id = $1 AND (owner_id IS NULL OR owner_id = $2)
		 ORDER BY (owner_id IS NULL) DESC, "order" ASC, created_at ASC`,
		problemID, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TestCase
	for rows.Next() {
		c, err := scanTestCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	if out == nil {
		out = []TestCase{}
	}
	return out, rows.Err()
}

// ListCanonical returns every canonical case for a problem (including hidden).
// Used by the server-side Test runner.
func (s *TestCaseStore) ListCanonical(ctx context.Context, problemID string) ([]TestCase, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+testCaseColumns+` FROM test_cases
		 WHERE problem_id = $1 AND owner_id IS NULL
		 ORDER BY "order" ASC, created_at ASC`, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TestCase
	for rows.Next() {
		c, err := scanTestCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	if out == nil {
		out = []TestCase{}
	}
	return out, rows.Err()
}

func (s *TestCaseStore) UpdateTestCase(ctx context.Context, id string, input UpdateTestCaseInput) (*TestCase, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if input.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *input.Name)
		argIdx++
	}
	if input.Stdin != nil {
		setClauses = append(setClauses, fmt.Sprintf("stdin = $%d", argIdx))
		args = append(args, *input.Stdin)
		argIdx++
	}
	if input.ExpectedStdout != nil {
		setClauses = append(setClauses, fmt.Sprintf("expected_stdout = $%d", argIdx))
		if *input.ExpectedStdout == "" {
			args = append(args, nil)
		} else {
			args = append(args, *input.ExpectedStdout)
		}
		argIdx++
	}
	if input.IsExample != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_example = $%d", argIdx))
		args = append(args, *input.IsExample)
		argIdx++
	}
	if input.Order != nil {
		setClauses = append(setClauses, fmt.Sprintf(`"order" = $%d`, argIdx))
		args = append(args, *input.Order)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetTestCase(ctx, id)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE test_cases SET %s WHERE id = $%d RETURNING `+testCaseColumns,
		strings.Join(setClauses, ", "), argIdx,
	)
	return scanTestCase(s.db.QueryRowContext(ctx, query, args...))
}

func (s *TestCaseStore) DeleteTestCase(ctx context.Context, id string) (*TestCase, error) {
	return scanTestCase(s.db.QueryRowContext(ctx,
		`DELETE FROM test_cases WHERE id = $1 RETURNING `+testCaseColumns, id))
}
