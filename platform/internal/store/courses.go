package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Course struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"orgId"`
	CreatedBy   string    `json:"createdBy"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	GradeLevel  string    `json:"gradeLevel"`
	Language    string    `json:"language"`
	IsPublished bool      `json:"isPublished"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateCourseInput struct {
	OrgID       string `json:"orgId"`
	CreatedBy   string `json:"createdBy"`
	Title       string `json:"title"`
	Description string `json:"description"`
	GradeLevel  string `json:"gradeLevel"`
	Language    string `json:"language"`
}

type UpdateCourseInput struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	GradeLevel  *string `json:"gradeLevel,omitempty"`
	Language    *string `json:"language,omitempty"`
	IsPublished *bool   `json:"isPublished,omitempty"`
}

type CourseStore struct {
	db *sql.DB
}

func NewCourseStore(db *sql.DB) *CourseStore {
	return &CourseStore{db: db}
}

func scanCourse(row interface{ Scan(...any) error }) (*Course, error) {
	var c Course
	err := row.Scan(&c.ID, &c.OrgID, &c.CreatedBy, &c.Title, &c.Description,
		&c.GradeLevel, &c.Language, &c.IsPublished, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

const courseColumns = `id, org_id, created_by, title, description, grade_level, language, is_published, created_at, updated_at`

func (s *CourseStore) CreateCourse(ctx context.Context, input CreateCourseInput) (*Course, error) {
	id := uuid.New().String()
	now := time.Now()
	desc := input.Description
	lang := input.Language
	if lang == "" {
		lang = "python"
	}

	return scanCourse(s.db.QueryRowContext(ctx,
		`INSERT INTO courses (id, org_id, created_by, title, description, grade_level, language, is_published, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, false, $8, $9)
		 RETURNING `+courseColumns,
		id, input.OrgID, input.CreatedBy, input.Title, desc, input.GradeLevel, lang, now, now,
	))
}

func (s *CourseStore) GetCourse(ctx context.Context, id string) (*Course, error) {
	return scanCourse(s.db.QueryRowContext(ctx,
		`SELECT `+courseColumns+` FROM courses WHERE id = $1`, id))
}

func (s *CourseStore) ListCoursesByOrg(ctx context.Context, orgID string) ([]Course, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+courseColumns+` FROM courses WHERE org_id = $1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCourses(rows)
}

func (s *CourseStore) ListCoursesByCreator(ctx context.Context, createdBy string) ([]Course, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+courseColumns+` FROM courses WHERE created_by = $1 ORDER BY created_at DESC`, createdBy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCourses(rows)
}

func (s *CourseStore) UpdateCourse(ctx context.Context, id string, input UpdateCourseInput) (*Course, error) {
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
	if input.GradeLevel != nil {
		setClauses = append(setClauses, fmt.Sprintf("grade_level = $%d", argIdx))
		args = append(args, *input.GradeLevel)
		argIdx++
	}
	if input.Language != nil {
		setClauses = append(setClauses, fmt.Sprintf("language = $%d", argIdx))
		args = append(args, *input.Language)
		argIdx++
	}
	if input.IsPublished != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_published = $%d", argIdx))
		args = append(args, *input.IsPublished)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetCourse(ctx, id)
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE courses SET %s WHERE id = $%d RETURNING `+courseColumns,
		strings.Join(setClauses, ", "), argIdx,
	)

	return scanCourse(s.db.QueryRowContext(ctx, query, args...))
}

func (s *CourseStore) DeleteCourse(ctx context.Context, id string) (*Course, error) {
	return scanCourse(s.db.QueryRowContext(ctx,
		`DELETE FROM courses WHERE id = $1 RETURNING `+courseColumns, id))
}

func (s *CourseStore) CloneCourse(ctx context.Context, courseID, newCreatedBy string) (*Course, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get original course
	var orig Course
	err = tx.QueryRowContext(ctx,
		`SELECT `+courseColumns+` FROM courses WHERE id = $1`, courseID,
	).Scan(&orig.ID, &orig.OrgID, &orig.CreatedBy, &orig.Title, &orig.Description,
		&orig.GradeLevel, &orig.Language, &orig.IsPublished, &orig.CreatedAt, &orig.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Insert cloned course
	newID := uuid.New().String()
	now := time.Now()
	var cloned Course
	err = tx.QueryRowContext(ctx,
		`INSERT INTO courses (id, org_id, created_by, title, description, grade_level, language, is_published, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, false, $8, $9)
		 RETURNING `+courseColumns,
		newID, orig.OrgID, newCreatedBy, orig.Title+" (Copy)", orig.Description,
		orig.GradeLevel, orig.Language, now, now,
	).Scan(&cloned.ID, &cloned.OrgID, &cloned.CreatedBy, &cloned.Title, &cloned.Description,
		&cloned.GradeLevel, &cloned.Language, &cloned.IsPublished, &cloned.CreatedAt, &cloned.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Clone topics
	_, err = tx.ExecContext(ctx,
		`INSERT INTO topics (id, course_id, title, description, sort_order, lesson_content, starter_code, created_at, updated_at)
		 SELECT gen_random_uuid(), $1, title, description, sort_order, lesson_content, starter_code, $2, $3
		 FROM topics WHERE course_id = $4 ORDER BY sort_order`,
		newID, now, now, courseID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &cloned, nil
}

func scanCourses(rows *sql.Rows) ([]Course, error) {
	var courses []Course
	for rows.Next() {
		var c Course
		if err := rows.Scan(&c.ID, &c.OrgID, &c.CreatedBy, &c.Title, &c.Description,
			&c.GradeLevel, &c.Language, &c.IsPublished, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		courses = append(courses, c)
	}
	if courses == nil {
		courses = []Course{}
	}
	return courses, rows.Err()
}
