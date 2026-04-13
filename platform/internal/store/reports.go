package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type ParentReport struct {
	ID          string    `json:"id"`
	StudentID   string    `json:"studentId"`
	GeneratedBy string    `json:"generatedBy"`
	PeriodStart time.Time `json:"periodStart"`
	PeriodEnd   time.Time `json:"periodEnd"`
	Content     string    `json:"content"`
	Summary     string    `json:"summary"` // JSONB as string
	CreatedAt   time.Time `json:"createdAt"`
}

type CreateReportInput struct {
	StudentID   string    `json:"studentId"`
	GeneratedBy string    `json:"generatedBy"`
	PeriodStart time.Time `json:"periodStart"`
	PeriodEnd   time.Time `json:"periodEnd"`
	Content     string    `json:"content"`
	Summary     string    `json:"summary"`
}

type ReportStore struct {
	db *sql.DB
}

func NewReportStore(db *sql.DB) *ReportStore {
	return &ReportStore{db: db}
}

const reportColumns = `id, student_id, generated_by, period_start, period_end, content, summary, created_at`

func scanReport(row interface{ Scan(...any) error }) (*ParentReport, error) {
	var r ParentReport
	err := row.Scan(&r.ID, &r.StudentID, &r.GeneratedBy, &r.PeriodStart, &r.PeriodEnd,
		&r.Content, &r.Summary, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *ReportStore) CreateReport(ctx context.Context, input CreateReportInput) (*ParentReport, error) {
	id := uuid.New().String()
	summary := input.Summary
	if summary == "" {
		summary = "{}"
	}
	return scanReport(s.db.QueryRowContext(ctx,
		`INSERT INTO parent_reports (id, student_id, generated_by, period_start, period_end, content, summary, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING `+reportColumns,
		id, input.StudentID, input.GeneratedBy, input.PeriodStart, input.PeriodEnd, input.Content, summary, time.Now(),
	))
}

func (s *ReportStore) GetReport(ctx context.Context, id string) (*ParentReport, error) {
	return scanReport(s.db.QueryRowContext(ctx,
		`SELECT `+reportColumns+` FROM parent_reports WHERE id = $1`, id))
}

func (s *ReportStore) ListReportsByStudent(ctx context.Context, studentID string) ([]ParentReport, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+reportColumns+` FROM parent_reports WHERE student_id = $1 ORDER BY created_at DESC`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []ParentReport
	for rows.Next() {
		var r ParentReport
		if err := rows.Scan(&r.ID, &r.StudentID, &r.GeneratedBy, &r.PeriodStart, &r.PeriodEnd,
			&r.Content, &r.Summary, &r.CreatedAt); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	if reports == nil {
		reports = []ParentReport{}
	}
	return reports, rows.Err()
}
