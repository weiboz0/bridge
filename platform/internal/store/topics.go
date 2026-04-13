package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Topic struct {
	ID            string    `json:"id"`
	CourseID      string    `json:"courseId"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	SortOrder     int       `json:"sortOrder"`
	LessonContent string    `json:"lessonContent"`
	StarterCode   *string   `json:"starterCode"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type CreateTopicInput struct {
	CourseID      string  `json:"courseId"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	LessonContent string  `json:"lessonContent"`
	StarterCode   *string `json:"starterCode"`
}

type UpdateTopicInput struct {
	Title         *string `json:"title,omitempty"`
	Description   *string `json:"description,omitempty"`
	LessonContent *string `json:"lessonContent,omitempty"`
	StarterCode   *string `json:"starterCode,omitempty"`
}

type TopicStore struct {
	db *sql.DB
}

func NewTopicStore(db *sql.DB) *TopicStore {
	return &TopicStore{db: db}
}

const topicColumns = `id, course_id, title, description, sort_order, lesson_content, starter_code, created_at, updated_at`

func scanTopic(row interface{ Scan(...any) error }) (*Topic, error) {
	var t Topic
	err := row.Scan(&t.ID, &t.CourseID, &t.Title, &t.Description, &t.SortOrder,
		&t.LessonContent, &t.StarterCode, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *TopicStore) CreateTopic(ctx context.Context, input CreateTopicInput) (*Topic, error) {
	id := uuid.New().String()
	now := time.Now()
	lessonContent := input.LessonContent
	if lessonContent == "" {
		lessonContent = "{}"
	}

	return scanTopic(s.db.QueryRowContext(ctx,
		`INSERT INTO topics (id, course_id, title, description, sort_order, lesson_content, starter_code, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, COALESCE((SELECT MAX(sort_order) + 1 FROM topics WHERE course_id = $5), 0), $6, $7, $8, $9)
		 RETURNING `+topicColumns,
		id, input.CourseID, input.Title, input.Description, input.CourseID, lessonContent, input.StarterCode, now, now,
	))
}

func (s *TopicStore) GetTopic(ctx context.Context, id string) (*Topic, error) {
	return scanTopic(s.db.QueryRowContext(ctx,
		`SELECT `+topicColumns+` FROM topics WHERE id = $1`, id))
}

func (s *TopicStore) ListTopicsByCourse(ctx context.Context, courseID string) ([]Topic, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+topicColumns+` FROM topics WHERE course_id = $1 ORDER BY sort_order ASC`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []Topic
	for rows.Next() {
		var t Topic
		if err := rows.Scan(&t.ID, &t.CourseID, &t.Title, &t.Description, &t.SortOrder,
			&t.LessonContent, &t.StarterCode, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		topics = append(topics, t)
	}
	if topics == nil {
		topics = []Topic{}
	}
	return topics, rows.Err()
}

func (s *TopicStore) UpdateTopic(ctx context.Context, id string, input UpdateTopicInput) (*Topic, error) {
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
	if input.LessonContent != nil {
		setClauses = append(setClauses, fmt.Sprintf("lesson_content = $%d", argIdx))
		args = append(args, *input.LessonContent)
		argIdx++
	}
	if input.StarterCode != nil {
		setClauses = append(setClauses, fmt.Sprintf("starter_code = $%d", argIdx))
		args = append(args, *input.StarterCode)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetTopic(ctx, id)
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE topics SET %s WHERE id = $%d RETURNING `+topicColumns,
		strings.Join(setClauses, ", "), argIdx,
	)

	return scanTopic(s.db.QueryRowContext(ctx, query, args...))
}

func (s *TopicStore) DeleteTopic(ctx context.Context, id string) (*Topic, error) {
	return scanTopic(s.db.QueryRowContext(ctx,
		`DELETE FROM topics WHERE id = $1 RETURNING `+topicColumns, id))
}

func (s *TopicStore) ReorderTopics(ctx context.Context, courseID string, topicIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, topicID := range topicIDs {
		_, err := tx.ExecContext(ctx,
			`UPDATE topics SET sort_order = $1, updated_at = $2 WHERE id = $3 AND course_id = $4`,
			i, time.Now(), topicID, courseID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
