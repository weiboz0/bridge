package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// TopicProblemAttachment represents a single row in topic_problems,
// describing which problem is attached to which topic and at what sort order.
type TopicProblemAttachment struct {
	TopicID    string    `json:"topicId"`
	ProblemID  string    `json:"problemId"`
	SortOrder  int       `json:"sortOrder"`
	AttachedBy string    `json:"attachedBy"`
	AttachedAt time.Time `json:"attachedAt"`
}

// TopicProblemStore manages the topic_problems join table.
type TopicProblemStore struct{ db *sql.DB }

// NewTopicProblemStore constructs a TopicProblemStore.
func NewTopicProblemStore(db *sql.DB) *TopicProblemStore { return &TopicProblemStore{db: db} }

const topicProblemColumns = `topic_id, problem_id, sort_order, attached_by, attached_at`

func scanTopicProblem(row interface{ Scan(...any) error }) (*TopicProblemAttachment, error) {
	var a TopicProblemAttachment
	if err := row.Scan(&a.TopicID, &a.ProblemID, &a.SortOrder, &a.AttachedBy, &a.AttachedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// Attach inserts a (topic, problem) pair with the given sort order and
// attached_by user. Returns ErrAlreadyAttached when the pair already exists.
func (s *TopicProblemStore) Attach(ctx context.Context, topicID, problemID string, sortOrder int, attachedBy string) (*TopicProblemAttachment, error) {
	row := s.db.QueryRowContext(ctx, `
        INSERT INTO topic_problems (topic_id, problem_id, sort_order, attached_by)
        VALUES ($1, $2, $3, $4)
        RETURNING `+topicProblemColumns,
		topicID, problemID, sortOrder, attachedBy,
	)
	a, err := scanTopicProblem(row)
	if err != nil {
		// pq unique_violation on the PRIMARY KEY (topic_id, problem_id).
		if isUniqueViolation(err) {
			return nil, ErrAlreadyAttached
		}
		return nil, err
	}
	return a, nil
}

// Detach removes the (topic, problem) pair. Returns true iff a row was deleted.
func (s *TopicProblemStore) Detach(ctx context.Context, topicID, problemID string) (bool, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM topic_problems WHERE topic_id = $1 AND problem_id = $2`,
		topicID, problemID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// SetSortOrder updates the sort_order for an existing attachment. Returns nil
// if the (topic, problem) pair does not exist.
func (s *TopicProblemStore) SetSortOrder(ctx context.Context, topicID, problemID string, sortOrder int) (*TopicProblemAttachment, error) {
	return scanTopicProblem(s.db.QueryRowContext(ctx, `
        UPDATE topic_problems SET sort_order = $1
        WHERE topic_id = $2 AND problem_id = $3
        RETURNING `+topicProblemColumns,
		sortOrder, topicID, problemID,
	))
}

// IsAttached reports whether the (topic, problem) pair exists.
func (s *TopicProblemStore) IsAttached(ctx context.Context, topicID, problemID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM topic_problems WHERE topic_id = $1 AND problem_id = $2)`,
		topicID, problemID).Scan(&exists)
	return exists, err
}

// ListTopicsByProblem returns the IDs of all topics a problem is attached to.
func (s *TopicProblemStore) ListTopicsByProblem(ctx context.Context, problemID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT topic_id FROM topic_problems WHERE problem_id = $1 ORDER BY attached_at ASC`,
		problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// isUniqueViolation returns true if err represents a PostgreSQL unique-
// constraint violation (SQLSTATE 23505). Works for both lib/pq and pgx-via-
// stdlib driver error strings, since both include the SQLSTATE code "23505"
// somewhere in the error text.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505")
}
