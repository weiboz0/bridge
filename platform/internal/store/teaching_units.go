package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	overlayPkg "github.com/weiboz0/bridge/platform/internal/overlay"
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
	MaterialType     string    `json:"materialType"`
	Status           string    `json:"status"`
	CreatedBy        string    `json:"createdBy"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	TopicID          *string   `json:"topicId"`
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
	MaterialType     string // "" → "notes"
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

// UnitRevision is a snapshot of a unit's blocks content captured when the unit
// transitions to classroom_ready or coach_ready. Reason records the target
// status that triggered the snapshot (e.g. "classroom_ready").
type UnitRevision struct {
	ID        string          `json:"id"`
	UnitID    string          `json:"unitId"`
	Blocks    json.RawMessage `json:"blocks"`
	Reason    *string         `json:"reason"`
	CreatedBy string          `json:"createdBy"`
	CreatedAt time.Time       `json:"createdAt"`
}

// TeachingUnitStore manages teaching_units and unit_documents rows.
type TeachingUnitStore struct{ db *sql.DB }

// NewTeachingUnitStore constructs a store backed by db.
func NewTeachingUnitStore(db *sql.DB) *TeachingUnitStore { return &TeachingUnitStore{db: db} }

const teachingUnitColumns = `id, scope, scope_id, title, slug, summary,
  grade_level, subject_tags, standards_tags, estimated_minutes, material_type,
  status, created_by, created_at, updated_at, topic_id`

// scanTeachingUnit reads a teaching_units row. Returns (nil, nil) on
// sql.ErrNoRows so callers can use a uniform "not found" check.
func scanTeachingUnit(row interface{ Scan(...any) error }) (*TeachingUnit, error) {
	var u TeachingUnit
	var scopeID, slug, gradeLevel, topicID sql.NullString
	var estimatedMinutes sql.NullInt32
	var subjectTags, standardsTags pq.StringArray

	err := row.Scan(
		&u.ID, &u.Scope, &scopeID, &u.Title, &slug, &u.Summary,
		&gradeLevel, &subjectTags, &standardsTags, &estimatedMinutes, &u.MaterialType,
		&u.Status, &u.CreatedBy, &u.CreatedAt, &u.UpdatedAt, &topicID,
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
	if topicID.Valid {
		v := topicID.String
		u.TopicID = &v
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
	if in.MaterialType == "" {
		in.MaterialType = "notes"
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
		  estimated_minutes, material_type, status, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING `+teachingUnitColumns,
		in.Scope, in.ScopeID, in.Title, in.Slug, in.Summary,
		in.GradeLevel, pq.Array(in.SubjectTags), pq.Array(in.StandardsTags),
		in.EstimatedMinutes, in.MaterialType, in.Status, in.CreatedBy,
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

// GetUnitByTopicID returns the unit linked to the given topic, or (nil, nil) if
// no unit has that topic_id. Each topic maps to at most one unit (enforced by a
// partial unique index on teaching_units.topic_id WHERE topic_id IS NOT NULL).
func (s *TeachingUnitStore) GetUnitByTopicID(ctx context.Context, topicID string) (*TeachingUnit, error) {
	return scanTeachingUnit(s.db.QueryRowContext(ctx,
		`SELECT `+teachingUnitColumns+` FROM teaching_units WHERE topic_id = $1`, topicID))
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

// validUnitTransitions encodes the spec-012 state machine. Keys are
// "currentStatus→targetStatus" pairs.
var validUnitTransitions = map[string]bool{
	"draft→reviewed":           true,
	"reviewed→classroom_ready": true,
	"reviewed→coach_ready":     true,
	// Any non-draft → archived.
	"reviewed→archived":        true,
	"classroom_ready→archived": true,
	"coach_ready→archived":     true,
	"archived→archived":        true, // idempotent archive
	// Unarchive.
	"archived→classroom_ready": true,
}

// snapshotStatuses are target statuses that trigger a unit_revisions snapshot.
var snapshotStatuses = map[string]bool{
	"classroom_ready": true,
	"coach_ready":     true,
}

// SetUnitStatus atomically transitions a teaching unit's status and, when the
// target is classroom_ready or coach_ready, snapshots the current
// unit_documents.blocks into a unit_revisions row. Returns the updated unit.
//
// Invalid transitions return ErrInvalidTransition (defined in problems.go).
// A non-existent unit returns sql.ErrNoRows (handler maps to 404).
func (s *TeachingUnitStore) SetUnitStatus(ctx context.Context, unitID, newStatus, callerID string) (*TeachingUnit, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var currentStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM teaching_units WHERE id = $1 FOR UPDATE`, unitID,
	).Scan(&currentStatus)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	key := currentStatus + "→" + newStatus
	if !validUnitTransitions[key] {
		return nil, ErrInvalidTransition
	}

	// Snapshot blocks if transitioning to a publish status.
	if snapshotStatuses[newStatus] {
		reason := newStatus
		_, err = tx.ExecContext(ctx, `
			INSERT INTO unit_revisions (unit_id, blocks, reason, created_by)
			SELECT $1, blocks, $2, $3
			FROM unit_documents
			WHERE unit_id = $1`,
			unitID, reason, callerID,
		)
		if err != nil {
			return nil, fmt.Errorf("set unit status: create revision: %w", err)
		}
	}

	// Update the status.
	now := time.Now()
	unit, err := scanTeachingUnit(tx.QueryRowContext(ctx,
		`UPDATE teaching_units SET status = $1, updated_at = $2
		 WHERE id = $3
		 RETURNING `+teachingUnitColumns,
		newStatus, now, unitID,
	))
	if err != nil {
		return nil, err
	}
	if unit == nil {
		return nil, fmt.Errorf("set unit status: update returned no row")
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return unit, nil
}

// scanUnitRevision reads a single unit_revisions row.
func scanUnitRevision(row interface{ Scan(...any) error }) (*UnitRevision, error) {
	var r UnitRevision
	var blocks []byte
	var reason sql.NullString

	err := row.Scan(&r.ID, &r.UnitID, &blocks, &reason, &r.CreatedBy, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Blocks = json.RawMessage(blocks)
	if reason.Valid {
		r.Reason = &reason.String
	}
	return &r, nil
}

// ListRevisions returns all revisions for the given unit, ordered by
// created_at DESC (newest first).
func (s *TeachingUnitStore) ListRevisions(ctx context.Context, unitID string) ([]UnitRevision, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, unit_id, blocks, reason, created_by, created_at
		FROM unit_revisions
		WHERE unit_id = $1
		ORDER BY created_at DESC`, unitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []UnitRevision{}
	for rows.Next() {
		r, err := scanUnitRevision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// GetRevision returns a single unit_revisions row by ID, or (nil, nil) if not
// found.
func (s *TeachingUnitStore) GetRevision(ctx context.Context, revisionID string) (*UnitRevision, error) {
	return scanUnitRevision(s.db.QueryRowContext(ctx, `
		SELECT id, unit_id, blocks, reason, created_by, created_at
		FROM unit_revisions
		WHERE id = $1`, revisionID))
}

// ── Search ──────────────────────────────────────────────────────────────────

// SearchUnitsFilter describes the filter / pagination for SearchUnits.
// When Query is non-empty, results are ranked by FTS relevance; otherwise
// they are ordered by updated_at DESC (recent-first browse).
type SearchUnitsFilter struct {
	Query           string     // FTS query text (plainto_tsquery)
	Scope           string     // "" = all visible; platform | org | personal
	ScopeID         *string
	Status          string     // "" = any visible
	GradeLevel      string
	SubjectTags     []string   // AND semantics (subject_tags @> $tags)
	ViewerID        string
	ViewerOrgs      []string   // org IDs where viewer has teacher/admin membership
	IsPlatformAdmin bool
	Limit           int        // default 20, max 100
	CursorCreatedAt *time.Time // keyset cursor for non-FTS browse
	CursorID        *string
}

// SearchUnits returns teaching units matching the filter. When Query is
// non-empty, results are filtered by FTS (plainto_tsquery) and ranked by
// ts_rank. Otherwise results are ordered by updated_at DESC with keyset
// cursor pagination.
func (s *TeachingUnitStore) SearchUnits(ctx context.Context, f SearchUnitsFilter) ([]TeachingUnit, error) {
	where := []string{}
	args := []any{}
	idx := 1

	// ── Visibility gate (mirrors canViewUnit in the handler) ──
	if f.IsPlatformAdmin {
		// platform admins see everything — no visibility filter
	} else {
		clauses := []string{
			// Platform scope: published statuses visible to any authenticated user.
			"(scope = 'platform' AND status IN ('classroom_ready','coach_ready','archived'))",
		}
		// Org scope: teachers/admins in the org can see all statuses.
		if len(f.ViewerOrgs) > 0 {
			clauses = append(clauses, fmt.Sprintf(
				"(scope = 'org' AND scope_id = ANY($%d))", idx))
			args = append(args, pq.Array(f.ViewerOrgs))
			idx++
		}
		// Personal scope: owner only.
		if f.ViewerID != "" {
			clauses = append(clauses, fmt.Sprintf(
				"(scope = 'personal' AND scope_id = $%d)", idx))
			args = append(args, f.ViewerID)
			idx++
			// Authors always see their own units regardless of scope.
			clauses = append(clauses, fmt.Sprintf(
				"(created_by = $%d)", idx))
			args = append(args, f.ViewerID)
			idx++
		}
		where = append(where, "("+strings.Join(clauses, " OR ")+")")
	}

	// ── Structured filters ──
	if f.Scope != "" {
		where = append(where, fmt.Sprintf("scope = $%d", idx))
		args = append(args, f.Scope)
		idx++
	}
	if f.ScopeID != nil {
		where = append(where, fmt.Sprintf("scope_id = $%d", idx))
		args = append(args, *f.ScopeID)
		idx++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", idx))
		args = append(args, f.Status)
		idx++
	}
	if f.GradeLevel != "" {
		where = append(where, fmt.Sprintf("grade_level = $%d", idx))
		args = append(args, f.GradeLevel)
		idx++
	}
	if len(f.SubjectTags) > 0 {
		where = append(where, fmt.Sprintf("subject_tags @> $%d", idx))
		args = append(args, pq.Array(f.SubjectTags))
		idx++
	}

	// ── FTS vs browse ──
	hasFTS := f.Query != ""
	var queryParamIdx int
	if hasFTS {
		where = append(where, fmt.Sprintf(
			"search_vector @@ plainto_tsquery('english', $%d)", idx))
		queryParamIdx = idx
		args = append(args, f.Query)
		idx++
	}

	// ── Cursor (browse mode only — FTS sorts by rank) ──
	if !hasFTS && f.CursorCreatedAt != nil && f.CursorID != nil {
		where = append(where, fmt.Sprintf(
			"(updated_at, id) < ($%d, $%d)", idx, idx+1))
		args = append(args, *f.CursorCreatedAt, *f.CursorID)
		idx += 2
	}

	// ── Limit ──
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// ── Build query ──
	q := `SELECT ` + teachingUnitColumns + ` FROM teaching_units`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	if hasFTS {
		q += fmt.Sprintf(
			` ORDER BY ts_rank(search_vector, plainto_tsquery('english', $%d)) DESC, id DESC`,
			queryParamIdx)
	} else {
		q += ` ORDER BY updated_at DESC, id DESC`
	}
	q += fmt.Sprintf(` LIMIT %d`, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
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

// ── Overlay / Fork ──────────────────────────────────────────────────────────

// UnitOverlay represents a row from unit_overlays.
type UnitOverlay struct {
	ChildUnitID      string          `json:"childUnitId"`
	ParentUnitID     string          `json:"parentUnitId"`
	ParentRevisionID *string         `json:"parentRevisionId"`
	BlockOverrides   json.RawMessage `json:"blockOverrides"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

// ForkTarget is defined in problems.go and reused here for unit forks.

// UpdateOverlayInput carries the optional partial-update fields for an overlay.
// A nil pointer means "leave unchanged". For ParentRevisionID, a pointer to ""
// means "set to NULL" (floating).
type UpdateOverlayInput struct {
	ParentRevisionID *string         // nil = unchanged; ptr to "" = set NULL (float)
	BlockOverrides   json.RawMessage // nil = unchanged
}

// LineageEntry is a single node in the overlay chain (root-first).
type LineageEntry struct {
	UnitID    string    `json:"unitId"`
	Title     string    `json:"title"`
	Scope     string    `json:"scope"`
	CreatedAt time.Time `json:"createdAt"`
}

// ForkUnit creates a new teaching unit derived from sourceID. A transaction
// inserts the child unit, seeds an empty unit_documents row, and creates a
// unit_overlays row linking child → source (parent_revision_id = NULL = floating).
// Returns the new child unit, or (nil, nil) if the source does not exist.
func (s *TeachingUnitStore) ForkUnit(ctx context.Context, sourceID string, target ForkTarget) (*TeachingUnit, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Load the source unit to copy its title.
	src, err := scanTeachingUnit(tx.QueryRowContext(ctx,
		`SELECT `+teachingUnitColumns+` FROM teaching_units WHERE id = $1 FOR UPDATE`, sourceID))
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, nil
	}

	title := src.Title + " (fork)"
	if target.Title != nil && *target.Title != "" {
		title = *target.Title
	}

	child, err := scanTeachingUnit(tx.QueryRowContext(ctx, `
		INSERT INTO teaching_units (
		  scope, scope_id, title, summary, status, created_by
		) VALUES ($1,$2,$3,$4,'draft',$5)
		RETURNING `+teachingUnitColumns,
		target.Scope, target.ScopeID, title, src.Summary, target.CallerID,
	))
	if err != nil {
		return nil, fmt.Errorf("fork unit: insert child: %w", err)
	}
	if child == nil {
		return nil, fmt.Errorf("fork unit: insert returned no row")
	}

	// Seed an empty document for the child.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO unit_documents (unit_id)
		VALUES ($1)
		ON CONFLICT (unit_id) DO NOTHING`, child.ID,
	); err != nil {
		return nil, fmt.Errorf("fork unit: seed document: %w", err)
	}

	// Create the overlay row linking child → parent (floating).
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO unit_overlays (child_unit_id, parent_unit_id, parent_revision_id, block_overrides)
		VALUES ($1, $2, NULL, '{}'::jsonb)`,
		child.ID, sourceID,
	); err != nil {
		return nil, fmt.Errorf("fork unit: insert overlay: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return child, nil
}

// GetOverlay returns the overlay row for the given child unit, or (nil, nil)
// if the unit has no overlay (i.e. it is not a fork).
func (s *TeachingUnitStore) GetOverlay(ctx context.Context, childUnitID string) (*UnitOverlay, error) {
	var o UnitOverlay
	var parentRevID sql.NullString
	var overrides []byte

	err := s.db.QueryRowContext(ctx, `
		SELECT child_unit_id, parent_unit_id, parent_revision_id,
		       block_overrides, created_at, updated_at
		FROM unit_overlays
		WHERE child_unit_id = $1`, childUnitID,
	).Scan(&o.ChildUnitID, &o.ParentUnitID, &parentRevID,
		&overrides, &o.CreatedAt, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if parentRevID.Valid {
		v := parentRevID.String
		o.ParentRevisionID = &v
	}
	o.BlockOverrides = json.RawMessage(overrides)
	return &o, nil
}

// UpdateOverlay applies partial updates to the overlay row. Nil fields are
// unchanged. For ParentRevisionID, a pointer to "" clears the column to NULL
// (switches to floating mode).
func (s *TeachingUnitStore) UpdateOverlay(ctx context.Context, childUnitID string, in UpdateOverlayInput) (*UnitOverlay, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if in.ParentRevisionID != nil {
		setClauses = append(setClauses, fmt.Sprintf("parent_revision_id = $%d", argIdx))
		if *in.ParentRevisionID == "" {
			args = append(args, nil)
		} else {
			args = append(args, *in.ParentRevisionID)
		}
		argIdx++
	}
	if in.BlockOverrides != nil {
		setClauses = append(setClauses, fmt.Sprintf("block_overrides = $%d::jsonb", argIdx))
		args = append(args, []byte(in.BlockOverrides))
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetOverlay(ctx, childUnitID)
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	args = append(args, childUnitID)
	q := fmt.Sprintf(
		`UPDATE unit_overlays SET %s WHERE child_unit_id = $%d
		 RETURNING child_unit_id, parent_unit_id, parent_revision_id,
		           block_overrides, created_at, updated_at`,
		strings.Join(setClauses, ", "), argIdx,
	)

	var o UnitOverlay
	var parentRevID sql.NullString
	var overrides []byte

	err := s.db.QueryRowContext(ctx, q, args...).Scan(
		&o.ChildUnitID, &o.ParentUnitID, &parentRevID,
		&overrides, &o.CreatedAt, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if parentRevID.Valid {
		v := parentRevID.String
		o.ParentRevisionID = &v
	}
	o.BlockOverrides = json.RawMessage(overrides)
	return &o, nil
}

// GetComposedDocument returns the composed (overlay-merged) document for a unit.
//   - If the unit has no overlay, it returns the unit's own document blocks.
//   - If the unit has an overlay, it loads the parent blocks (from a pinned
//     revision or the latest published revision), the child's own blocks, and the
//     overlay's block_overrides, then calls overlay.ComposeDocument.
//
// Returns (nil, nil) if the unit has no document row.
func (s *TeachingUnitStore) GetComposedDocument(ctx context.Context, unitID string) (json.RawMessage, error) {
	ov, err := s.GetOverlay(ctx, unitID)
	if err != nil {
		return nil, err
	}

	// No overlay — return the unit's own blocks directly.
	if ov == nil {
		doc, err := s.GetDocument(ctx, unitID)
		if err != nil {
			return nil, err
		}
		if doc == nil {
			return nil, nil
		}
		return doc.Blocks, nil
	}

	// Load parent blocks.
	var parentBlocks json.RawMessage
	if ov.ParentRevisionID != nil {
		// Pinned to a specific revision.
		rev, err := s.GetRevision(ctx, *ov.ParentRevisionID)
		if err != nil {
			return nil, fmt.Errorf("composed doc: load pinned revision: %w", err)
		}
		if rev == nil || rev.UnitID != ov.ParentUnitID {
			// Pinned revision was deleted or belongs to wrong unit —
			// fall through to latest published, same as floating.
			parentBlocks, err = s.latestPublishedBlocks(ctx, ov.ParentUnitID)
			if err != nil {
				return nil, err
			}
		} else {
			parentBlocks = rev.Blocks
		}
	} else {
		// Floating — use the parent's latest published revision.
		parentBlocks, err = s.latestPublishedBlocks(ctx, ov.ParentUnitID)
		if err != nil {
			return nil, err
		}
	}

	// If the parent has no published revision, fall back to the parent's
	// current document. This handles draft-to-draft forks where the parent
	// has never been published.
	if parentBlocks == nil {
		parentDoc, err := s.GetDocument(ctx, ov.ParentUnitID)
		if err != nil {
			return nil, fmt.Errorf("composed doc: load parent document: %w", err)
		}
		if parentDoc != nil {
			parentBlocks = parentDoc.Blocks
		}
	}

	// Load child's own document blocks.
	childDoc, err := s.GetDocument(ctx, unitID)
	if err != nil {
		return nil, fmt.Errorf("composed doc: load child document: %w", err)
	}

	// Parse blocks into slices.
	parentContent := extractContent(parentBlocks)
	childContent := extractContent(childDoc.Blocks)

	// Parse block overrides.
	overrides := map[string]overlayPkg.BlockOverride{}
	if len(ov.BlockOverrides) > 0 && string(ov.BlockOverrides) != "{}" {
		if err := json.Unmarshal(ov.BlockOverrides, &overrides); err != nil {
			return nil, fmt.Errorf("composed doc: parse block_overrides: %w", err)
		}
	}

	// Compose.
	composed := overlayPkg.ComposeDocument(parentContent, childContent, overrides)

	// Wrap in document envelope.
	result := map[string]any{
		"type":    "doc",
		"content": composed,
	}
	return json.Marshal(result)
}

// latestPublishedBlocks returns the blocks from the parent's latest published
// revision (reason IN ('classroom_ready', 'coach_ready'), newest first).
// Returns nil if the parent has no published revision.
func (s *TeachingUnitStore) latestPublishedBlocks(ctx context.Context, unitID string) (json.RawMessage, error) {
	var blocks []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT blocks FROM unit_revisions
		WHERE unit_id = $1 AND reason IN ('classroom_ready', 'coach_ready')
		ORDER BY created_at DESC
		LIMIT 1`, unitID,
	).Scan(&blocks)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(blocks), nil
}

// extractContent parses a doc-envelope JSON and returns the content array
// as individual raw messages. Returns nil for nil/empty input.
func extractContent(raw json.RawMessage) []json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var envelope struct {
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil
	}
	return envelope.Content
}

// GetLineage walks the overlay chain from the given unit up to its root,
// returning a root-first ordered list of LineageEntry. Max depth is 10 to
// prevent infinite loops.
func (s *TeachingUnitStore) GetLineage(ctx context.Context, unitID string) ([]LineageEntry, error) {
	const maxDepth = 10

	// Collect the chain bottom-up with cycle detection.
	chain := []LineageEntry{}
	visited := map[string]bool{}
	currentID := unitID

	for i := 0; i < maxDepth; i++ {
		var entry LineageEntry
		var scopeID sql.NullString
		err := s.db.QueryRowContext(ctx, `
			SELECT id, title, scope, created_at FROM teaching_units WHERE id = $1`,
			currentID,
		).Scan(&entry.UnitID, &entry.Title, &entry.Scope, &entry.CreatedAt)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return nil, err
		}
		_ = scopeID // not used in LineageEntry

		chain = append(chain, entry)
		visited[currentID] = true

		// Look for a parent overlay.
		var parentID string
		err = s.db.QueryRowContext(ctx, `
			SELECT parent_unit_id FROM unit_overlays WHERE child_unit_id = $1`,
			currentID,
		).Scan(&parentID)
		if err == sql.ErrNoRows {
			break // reached the root
		}
		if err != nil {
			return nil, err
		}
		if visited[parentID] {
			break // cycle detected — stop traversal
		}
		currentID = parentID
	}

	// Reverse to root-first order.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}

	return chain, nil
}
