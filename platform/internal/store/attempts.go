package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Attempt struct {
	ID        string    `json:"id"`
	ProblemID string    `json:"problemId"`
	UserID    string    `json:"userId"`
	Title     string    `json:"title"`
	Language  string    `json:"language"`
	PlainText string    `json:"plainText"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// LastTestResult is the JSONB summary of the most recent Test run, or nil
	// if Test has never been invoked. The store keeps it as raw JSON so the
	// shape of the summary belongs to the runner (handler layer).
	LastTestResult *json.RawMessage `json:"lastTestResult,omitempty"`
}

type CreateAttemptInput struct {
	ProblemID string
	UserID    string
	Title     string // empty -> "Untitled"
	Language  string
	PlainText string
}

type UpdateAttemptInput struct {
	Title     *string
	PlainText *string
}

type AttemptStore struct {
	db *sql.DB
}

func NewAttemptStore(db *sql.DB) *AttemptStore { return &AttemptStore{db: db} }

const attemptColumns = `id, problem_id, user_id, title, language, plain_text, created_at, updated_at, last_test_result`

func scanAttempt(row interface{ Scan(...any) error }) (*Attempt, error) {
	var a Attempt
	var lastTest sql.NullString
	err := row.Scan(&a.ID, &a.ProblemID, &a.UserID, &a.Title, &a.Language,
		&a.PlainText, &a.CreatedAt, &a.UpdatedAt, &lastTest)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastTest.Valid {
		raw := json.RawMessage(lastTest.String)
		a.LastTestResult = &raw
	}
	return &a, nil
}

func (s *AttemptStore) CreateAttempt(ctx context.Context, input CreateAttemptInput) (*Attempt, error) {
	title := input.Title
	if title == "" {
		title = "Untitled"
	}
	return scanAttempt(s.db.QueryRowContext(ctx,
		`INSERT INTO attempts (problem_id, user_id, title, language, plain_text)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+attemptColumns,
		input.ProblemID, input.UserID, title, input.Language, input.PlainText))
}

func (s *AttemptStore) GetAttempt(ctx context.Context, id string) (*Attempt, error) {
	return scanAttempt(s.db.QueryRowContext(ctx,
		`SELECT `+attemptColumns+` FROM attempts WHERE id = $1`, id))
}

// ListByUserAndProblem returns a user's attempts for a problem, most recently
// updated first — the order the UI uses to decide which attempt is "active".
func (s *AttemptStore) ListByUserAndProblem(ctx context.Context, problemID, userID string) ([]Attempt, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+attemptColumns+` FROM attempts
		 WHERE problem_id = $1 AND user_id = $2
		 ORDER BY updated_at DESC`, problemID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Attempt
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	if out == nil {
		out = []Attempt{}
	}
	return out, rows.Err()
}

func (s *AttemptStore) UpdateAttempt(ctx context.Context, id string, input UpdateAttemptInput) (*Attempt, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if input.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *input.Title)
		argIdx++
	}
	if input.PlainText != nil {
		setClauses = append(setClauses, fmt.Sprintf("plain_text = $%d", argIdx))
		args = append(args, *input.PlainText)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetAttempt(ctx, id)
	}

	// Always bump updated_at so the client's "most recently edited" sort is stable.
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE attempts SET %s WHERE id = $%d RETURNING `+attemptColumns,
		strings.Join(setClauses, ", "), argIdx,
	)
	return scanAttempt(s.db.QueryRowContext(ctx, query, args...))
}

func (s *AttemptStore) DeleteAttempt(ctx context.Context, id string) (*Attempt, error) {
	return scanAttempt(s.db.QueryRowContext(ctx,
		`DELETE FROM attempts WHERE id = $1 RETURNING `+attemptColumns, id))
}

// UpdateLastTestResult persists a test run summary to the attempt without
// touching plain_text or updated_at — Test runs are read-only at the editor
// level, so they should not bump "most recently edited" sort order.
func (s *AttemptStore) UpdateLastTestResult(ctx context.Context, attemptID string, summary json.RawMessage) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE attempts SET last_test_result = $1 WHERE id = $2`,
		[]byte(summary), attemptID,
	)
	return err
}

// CountAttemptsByProblem returns the number of attempts that reference a
// problem. Used by DeleteProblem to enforce the "problem has attempts" guard
// (409 Conflict instead of silently cascading and destroying student work).
func (s *AttemptStore) CountAttemptsByProblem(ctx context.Context, problemID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM attempts WHERE problem_id = $1`, problemID).Scan(&n)
	return n, err
}

// IsTeacherOfAttempt — plan 053b Phase 1.
//
// Reports whether `teacherID` has an instructor-or-TA
// class_membership in ANY class where (a) the class's course owns
// a topic linking to the attempt's problem AND (b) the attempt's
// owner is also a class_membership in the SAME class.
//
// Both constraints in one SQL EXISTS — critical: a teacher of
// Class A must NOT get tokens for a student in Class B even if
// both classes use the same problem (popular-problem leak Codex
// caught at Phase 0).
func (s *AttemptStore) IsTeacherOfAttempt(ctx context.Context, teacherID, attemptID string) (bool, error) {
	if teacherID == "" || attemptID == "" {
		return false, nil
	}
	var ok bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM attempts a
			INNER JOIN topic_problems tp ON tp.problem_id = a.problem_id
			INNER JOIN topics t          ON t.id = tp.topic_id
			INNER JOIN courses co        ON co.id = t.course_id
			INNER JOIN classes c         ON c.course_id = co.id
			INNER JOIN class_memberships cm_t
				ON cm_t.class_id = c.id
				AND cm_t.user_id = $1
				AND cm_t.role IN ('instructor', 'ta')
			INNER JOIN class_memberships cm_s
				ON cm_s.class_id = c.id
				AND cm_s.user_id = a.user_id
			WHERE a.id = $2
		)`, teacherID, attemptID).Scan(&ok)
	if err != nil {
		return false, err
	}
	return ok, nil
}

// IsOrgAdminOfAttempt — plan 053b Phase 1.
//
// Reports whether `userID` is an active org_admin for the org that
// owns the course of the attempt's problem AND the attempt owner
// is enrolled in SOME class for that course. The attempt-owner
// constraint preserves the same privacy boundary as
// IsTeacherOfAttempt — an org_admin can't peek at attempts of
// students who happen to be in their org but not in any class for
// the relevant course.
func (s *AttemptStore) IsOrgAdminOfAttempt(ctx context.Context, userID, attemptID string) (bool, error) {
	if userID == "" || attemptID == "" {
		return false, nil
	}
	var ok bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM attempts a
			INNER JOIN topic_problems tp ON tp.problem_id = a.problem_id
			INNER JOIN topics t          ON t.id = tp.topic_id
			INNER JOIN courses co        ON co.id = t.course_id
			INNER JOIN org_memberships om
				ON om.org_id = co.org_id
				AND om.user_id = $1
				AND om.role = 'org_admin'
				AND om.status = 'active'
			INNER JOIN class_memberships cm_s
				ON cm_s.class_id IN (SELECT id FROM classes WHERE course_id = co.id)
				AND cm_s.user_id = a.user_id
			WHERE a.id = $2
		)`, userID, attemptID).Scan(&ok)
	if err != nil {
		return false, err
	}
	return ok, nil
}
