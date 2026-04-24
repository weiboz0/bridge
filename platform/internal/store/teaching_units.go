package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// TeachingUnit is the core row from teaching_units. Scope is "platform"
// (global), "org" (shared within an org), or "personal" (owned by one user).
// ScopeID is NULL iff Scope == "platform"; otherwise it points at the owning
// org or user.
type TeachingUnit struct {
	ID               string    `json:"id"`
	Scope            string    `json:"scope"`
	ScopeID          *string   `json:"scopeId"`
	Title            string    `json:"title"`
	Slug             *string   `json:"slug"`
	Summary          string    `json:"summary"`
	GradeLevel       *string   `json:"gradeLevel"`
	SubjectTags      []string  `json:"subjectTags"`
	StandardsTags    []string  `json:"standardsTags"`
	EstimatedMinutes *int      `json:"estimatedMinutes"`
	Status           string    `json:"status"`
	CreatedBy        string    `json:"createdBy"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// UnitDocument is the single document (block-based content) row for a unit.
type UnitDocument struct {
	UnitID    string          `json:"unitId"`
	Blocks    json.RawMessage `json:"blocks"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// CreateTeachingUnitInput carries the fields required to create a teaching unit.
// Status defaults to "draft" when empty.
type CreateTeachingUnitInput struct {
	Scope            string
	ScopeID          *string
	Title            string
	Slug             *string
	Summary          string
	GradeLevel       *string
	SubjectTags      []string
	StandardsTags    []string
	EstimatedMinutes *int
	Status           string // "" → "draft"
	CreatedBy        string
}

// UpdateTeachingUnitInput carries optional partial-update fields.
// Nil fields are left untouched. For SubjectTags/StandardsTags:
//   - nil = leave unchanged
//   - empty slice = clear to '{}'
//
// For Slug/GradeLevel: pointer to "" = clear to NULL.
type UpdateTeachingUnitInput struct {
	Title            *string
	Slug             *string
	Summary          *string
	GradeLevel       *string
	SubjectTags      []string // nil = unchanged; empty = clear
	StandardsTags    []string // nil = unchanged; empty = clear
	EstimatedMinutes *int
	Status           *string
}

// TeachingUnitStore manages teaching_units and unit_documents rows.
type TeachingUnitStore struct{ db *sql.DB }

// NewTeachingUnitStore constructs a store backed by db.
func NewTeachingUnitStore(db *sql.DB) *TeachingUnitStore { return &TeachingUnitStore{db: db} }

const teachingUnitColumns = `id, scope, scope_id, title, slug, summary,
  grade_level, subject_tags, standards_tags, estimated_minutes, status,
  created_by, created_at, updated_at`

// scanTeachingUnit reads a teaching_units row. Returns (nil, nil) on
// sql.ErrNoRows so callers can use a uniform "not found" check.
func scanTeachingUnit(row interface{ Scan(...any) error }) (*TeachingUnit, error) {
	var u TeachingUnit
	var scopeID, slug, gradeLevel sql.NullString
	var estimatedMinutes sql.NullInt32
	var subjectTags, standardsTags pq.StringArray

	err := row.Scan(
		&u.ID, &u.Scope, &scopeID, &u.Title, &slug, &u.Summary,
		&gradeLevel, &subjectTags, &standardsTags, &estimatedMinutes, &u.Status,
		&u.CreatedBy, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if scopeID.Valid {
		v := scopeID.String
		u.ScopeID = &v
	}
	if slug.Valid {
		v := slug.String
		u.Slug = &v
	}
	if gradeLevel.Valid {
		v := gradeLevel.String
		u.GradeLevel = &v
	}
	if estimatedMinutes.Valid {
		v := int(estimatedMinutes.Int32)
		u.EstimatedMinutes = &v
	}
	if subjectTags == nil {
		u.SubjectTags = []string{}
	} else {
		u.SubjectTags = []string(subjectTags)
	}
	if standardsTags == nil {
		u.StandardsTags = []string{}
	} else {
		u.StandardsTags = []string(standardsTags)
	}

	return &u, nil
}

// CreateUnit inserts a new teaching unit row and seeds an empty unit_documents
// row in a single transaction. Status defaults to "draft".
func (s *TeachingUnitStore) CreateUnit(ctx context.Context, in CreateTeachingUnitInput) (*TeachingUnit, error) {
	if in.Status == "" {
		in.Status = "draft"
	}
	if in.SubjectTags == nil {
		in.SubjectTags = []string{}
	}
	if in.StandardsTags == nil {
		in.StandardsTags = []string{}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	unit, err := scanTeachingUnit(tx.QueryRowContext(ctx, `
		INSERT INTO teaching_units (
		  scope, scope_id, title, slug, summary,
		  grade_level, subject_tags, standards_tags,
		  estimated_minutes, status, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING `+teachingUnitColumns,
		in.Scope, in.ScopeID, in.Title, in.Slug, in.Summary,
		in.GradeLevel, pq.Array(in.SubjectTags), pq.Array(in.StandardsTags),
		in.EstimatedMinutes, in.Status, in.CreatedBy,
	))
	if err != nil {
		return nil, err
	}
	if unit == nil {
		return nil, fmt.Errorf("create unit: insert returned no row")
	}

	// Seed the empty document. The DEFAULT in the schema handles the blocks
	// value; we explicitly insert so tests can assert the row exists immediately.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO unit_documents (unit_id)
		VALUES ($1)
		ON CONFLICT (unit_id) DO NOTHING`,
		unit.ID,
	); err != nil {
		return nil, fmt.Errorf("create unit: seed document: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return unit, nil
}

// GetUnit returns the unit with the given id, or (nil, nil) if not found.
func (s *TeachingUnitStore) GetUnit(ctx context.Context, id string) (*TeachingUnit, error) {
	return scanTeachingUnit(s.db.QueryRowContext(ctx,
		`SELECT `+teachingUnitColumns+` FROM teaching_units WHERE id = $1`, id))
}

// GetDocument returns the unit_documents row for unitID.
func (s *TeachingUnitStore) GetDocument(ctx context.Context, unitID string) (*UnitDocument, error) {
	var doc UnitDocument
	var blocks []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT unit_id, blocks, updated_at FROM unit_documents WHERE unit_id = $1`, unitID,
	).Scan(&doc.UnitID, &blocks, &doc.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	doc.Blocks = json.RawMessage(blocks)
	return &doc, nil
}

// UpdateUnit applies partial updates to the teaching unit. Nil fields are
// unchanged. For Slug/GradeLevel, a pointer to "" clears the column to NULL.
// For SubjectTags/StandardsTags, nil leaves unchanged; an empty slice clears.
func (s *TeachingUnitStore) UpdateUnit(ctx context.Context, id string, in UpdateTeachingUnitInput) (*TeachingUnit, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if in.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *in.Title)
		argIdx++
	}
	if in.Slug != nil {
		setClauses = append(setClauses, fmt.Sprintf("slug = $%d", argIdx))
		if *in.Slug == "" {
			args = append(args, nil)
		} else {
			args = append(args, *in.Slug)
		}
		argIdx++
	}
	if in.Summary != nil {
		setClauses = append(setClauses, fmt.Sprintf("summary = $%d", argIdx))
		args = append(args, *in.Summary)
		argIdx++
	}
	if in.GradeLevel != nil {
		setClauses = append(setClauses, fmt.Sprintf("grade_level = $%d", argIdx))
		if *in.GradeLevel == "" {
			args = append(args, nil)
		} else {
			args = append(args, *in.GradeLevel)
		}
		argIdx++
	}
	if in.SubjectTags != nil {
		setClauses = append(setClauses, fmt.Sprintf("subject_tags = $%d", argIdx))
		args = append(args, pq.Array(in.SubjectTags))
		argIdx++
	}
	if in.StandardsTags != nil {
		setClauses = append(setClauses, fmt.Sprintf("standards_tags = $%d", argIdx))
		args = append(args, pq.Array(in.StandardsTags))
		argIdx++
	}
	if in.EstimatedMinutes != nil {
		setClauses = append(setClauses, fmt.Sprintf("estimated_minutes = $%d", argIdx))
		args = append(args, *in.EstimatedMinutes)
		argIdx++
	}
	if in.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *in.Status)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetUnit(ctx, id)
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	args = append(args, id)
	q := fmt.Sprintf(
		`UPDATE teaching_units SET %s WHERE id = $%d RETURNING `+teachingUnitColumns,
		strings.Join(setClauses, ", "), argIdx,
	)
	return scanTeachingUnit(s.db.QueryRowContext(ctx, q, args...))
}

// SaveDocument upserts the blocks content for the given unit. It bumps
// unit_documents.updated_at and teaching_units.updated_at in the same
// transaction so cache-busting consumers always see a consistent pair.
func (s *TeachingUnitStore) SaveDocument(ctx context.Context, unitID string, blocks json.RawMessage) (*UnitDocument, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now()

	var doc UnitDocument
	var rawBlocks []byte
	err = tx.QueryRowContext(ctx, `
		INSERT INTO unit_documents (unit_id, blocks, updated_at)
		VALUES ($1, $2::jsonb, $3)
		ON CONFLICT (unit_id) DO UPDATE
		  SET blocks = EXCLUDED.blocks,
		      updated_at = EXCLUDED.updated_at
		RETURNING unit_id, blocks, updated_at`,
		unitID, []byte(blocks), now,
	).Scan(&doc.UnitID, &rawBlocks, &doc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	doc.Blocks = json.RawMessage(rawBlocks)

	// Bump the parent unit's updated_at so API consumers can detect
	// that the unit content has changed without fetching the document.
	if _, err := tx.ExecContext(ctx,
		`UPDATE teaching_units SET updated_at = $1 WHERE id = $2`, now, unitID,
	); err != nil {
		return nil, fmt.Errorf("save document: bump unit updated_at: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &doc, nil
}

// DeleteUnit hard-deletes the unit and returns the deleted row (or nil if not
// found). Cascades in the schema remove unit_documents and unit_revisions.
func (s *TeachingUnitStore) DeleteUnit(ctx context.Context, id string) (*TeachingUnit, error) {
	return scanTeachingUnit(s.db.QueryRowContext(ctx,
		`DELETE FROM teaching_units WHERE id = $1 RETURNING `+teachingUnitColumns, id))
}

// ListUnitsForScope returns all units for the given scope and scopeID, ordered
// by updated_at DESC. Pass an empty scopeID for scope="platform".
func (s *TeachingUnitStore) ListUnitsForScope(ctx context.Context, scope, scopeID string) ([]TeachingUnit, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if scopeID == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+teachingUnitColumns+`
			FROM teaching_units
			WHERE scope = $1 AND scope_id IS NULL
			ORDER BY updated_at DESC`, scope)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+teachingUnitColumns+`
			FROM teaching_units
			WHERE scope = $1 AND scope_id = $2
			ORDER BY updated_at DESC`, scope, scopeID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TeachingUnit{}
	for rows.Next() {
		u, err := scanTeachingUnit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}
