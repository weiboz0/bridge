package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Assignment struct {
	ID          string     `json:"id"`
	TopicID     *string    `json:"topicId"`
	ClassID     string     `json:"classId"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	StarterCode *string    `json:"starterCode"`
	DueDate     *time.Time `json:"dueDate"`
	Rubric      string     `json:"rubric"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type CreateAssignmentInput struct {
	ClassID     string     `json:"classId"`
	TopicID     *string    `json:"topicId"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	StarterCode *string    `json:"starterCode"`
	DueDate     *time.Time `json:"dueDate"`
	Rubric      string     `json:"rubric"`
}

type UpdateAssignmentInput struct {
	Title       *string    `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	StarterCode *string    `json:"starterCode,omitempty"`
	DueDate     *time.Time `json:"dueDate,omitempty"`
	Rubric      *string    `json:"rubric,omitempty"`
}

type AssignmentStore struct {
	db *sql.DB
}

func NewAssignmentStore(db *sql.DB) *AssignmentStore {
	return &AssignmentStore{db: db}
}

const assignmentColumns = `id, topic_id, class_id, title, description, starter_code, due_date, rubric, created_at`

func scanAssignment(row interface{ Scan(...any) error }) (*Assignment, error) {
	var a Assignment
	err := row.Scan(&a.ID, &a.TopicID, &a.ClassID, &a.Title, &a.Description,
		&a.StarterCode, &a.DueDate, &a.Rubric, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *AssignmentStore) CreateAssignment(ctx context.Context, input CreateAssignmentInput) (*Assignment, error) {
	id := uuid.New().String()
	rubric := input.Rubric
	if rubric == "" {
		rubric = "{}"
	}
	return scanAssignment(s.db.QueryRowContext(ctx,
		`INSERT INTO assignments (id, class_id, topic_id, title, description, starter_code, due_date, rubric, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING `+assignmentColumns,
		id, input.ClassID, input.TopicID, input.Title, input.Description,
		input.StarterCode, input.DueDate, rubric, time.Now(),
	))
}

func (s *AssignmentStore) GetAssignment(ctx context.Context, id string) (*Assignment, error) {
	return scanAssignment(s.db.QueryRowContext(ctx,
		`SELECT `+assignmentColumns+` FROM assignments WHERE id = $1`, id))
}

func (s *AssignmentStore) ListAssignmentsByClass(ctx context.Context, classID string) ([]Assignment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+assignmentColumns+` FROM assignments WHERE class_id = $1 ORDER BY created_at DESC`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []Assignment
	for rows.Next() {
		var a Assignment
		if err := rows.Scan(&a.ID, &a.TopicID, &a.ClassID, &a.Title, &a.Description,
			&a.StarterCode, &a.DueDate, &a.Rubric, &a.CreatedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	if assignments == nil {
		assignments = []Assignment{}
	}
	return assignments, rows.Err()
}

func (s *AssignmentStore) UpdateAssignment(ctx context.Context, id string, input UpdateAssignmentInput) (*Assignment, error) {
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
		args = append(args, *input.StarterCode)
		argIdx++
	}
	if input.DueDate != nil {
		setClauses = append(setClauses, fmt.Sprintf("due_date = $%d", argIdx))
		args = append(args, *input.DueDate)
		argIdx++
	}
	if input.Rubric != nil {
		setClauses = append(setClauses, fmt.Sprintf("rubric = $%d", argIdx))
		args = append(args, *input.Rubric)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetAssignment(ctx, id)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE assignments SET %s WHERE id = $%d RETURNING `+assignmentColumns,
		strings.Join(setClauses, ", "), argIdx,
	)
	return scanAssignment(s.db.QueryRowContext(ctx, query, args...))
}

func (s *AssignmentStore) DeleteAssignment(ctx context.Context, id string) (*Assignment, error) {
	return scanAssignment(s.db.QueryRowContext(ctx,
		`DELETE FROM assignments WHERE id = $1 RETURNING `+assignmentColumns, id))
}

// --- Submissions ---

type Submission struct {
	ID           string     `json:"id"`
	AssignmentID string     `json:"assignmentId"`
	StudentID    string     `json:"studentId"`
	DocumentID   *string    `json:"documentId"`
	Grade        *float64   `json:"grade"`
	Feedback     *string    `json:"feedback"`
	SubmittedAt  time.Time  `json:"submittedAt"`
}

type SubmissionWithStudent struct {
	ID           string     `json:"id"`
	AssignmentID string     `json:"assignmentId"`
	StudentID    string     `json:"studentId"`
	DocumentID   *string    `json:"documentId"`
	Grade        *float64   `json:"grade"`
	Feedback     *string    `json:"feedback"`
	SubmittedAt  time.Time  `json:"submittedAt"`
	StudentName  string     `json:"studentName"`
	StudentEmail string     `json:"studentEmail"`
}

func (s *AssignmentStore) CreateSubmission(ctx context.Context, assignmentID, studentID string, documentID *string) (*Submission, error) {
	id := uuid.New().String()
	var sub Submission
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO submissions (id, assignment_id, student_id, document_id, submitted_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT DO NOTHING
		 RETURNING id, assignment_id, student_id, document_id, grade, feedback, submitted_at`,
		id, assignmentID, studentID, documentID, time.Now(),
	).Scan(&sub.ID, &sub.AssignmentID, &sub.StudentID, &sub.DocumentID, &sub.Grade, &sub.Feedback, &sub.SubmittedAt)
	if err == sql.ErrNoRows {
		return nil, nil // already submitted
	}
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *AssignmentStore) ListSubmissionsByAssignment(ctx context.Context, assignmentID string) ([]SubmissionWithStudent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT s.id, s.assignment_id, s.student_id, s.document_id, s.grade, s.feedback, s.submitted_at, u.name, u.email
		 FROM submissions s
		 INNER JOIN users u ON s.student_id = u.id
		 WHERE s.assignment_id = $1
		 ORDER BY s.submitted_at DESC`, assignmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []SubmissionWithStudent
	for rows.Next() {
		var sub SubmissionWithStudent
		if err := rows.Scan(&sub.ID, &sub.AssignmentID, &sub.StudentID, &sub.DocumentID,
			&sub.Grade, &sub.Feedback, &sub.SubmittedAt, &sub.StudentName, &sub.StudentEmail); err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	if subs == nil {
		subs = []SubmissionWithStudent{}
	}
	return subs, rows.Err()
}

func (s *AssignmentStore) GetSubmission(ctx context.Context, id string) (*Submission, error) {
	var sub Submission
	err := s.db.QueryRowContext(ctx,
		`SELECT id, assignment_id, student_id, document_id, grade, feedback, submitted_at
		 FROM submissions WHERE id = $1`, id,
	).Scan(&sub.ID, &sub.AssignmentID, &sub.StudentID, &sub.DocumentID, &sub.Grade, &sub.Feedback, &sub.SubmittedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *AssignmentStore) GradeSubmission(ctx context.Context, id string, grade float64, feedback *string) (*Submission, error) {
	var sub Submission
	err := s.db.QueryRowContext(ctx,
		`UPDATE submissions SET grade = $1, feedback = $2 WHERE id = $3
		 RETURNING id, assignment_id, student_id, document_id, grade, feedback, submitted_at`,
		grade, feedback, id,
	).Scan(&sub.ID, &sub.AssignmentID, &sub.StudentID, &sub.DocumentID, &sub.Grade, &sub.Feedback, &sub.SubmittedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sub, nil
}
