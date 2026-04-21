package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// ErrInvalidTransition is returned by SetStatus when the requested status
// transition is not permitted by the state machine (draft -> published ->
// archived -> published). Handlers should map this to HTTP 409 Conflict.
var ErrInvalidTransition = errors.New("invalid status transition")

// Problem is the scoped problem-bank row. Scope is "platform" (global),
// "org" (shared within an org), or "personal" (owned by one user). ScopeID is
// NULL iff Scope == "platform"; otherwise it points at the owning org or user.
type Problem struct {
	ID            string            `json:"id"`
	Scope         string            `json:"scope"`       // platform | org | personal
	ScopeID       *string           `json:"scopeId"`     // NULL when scope=platform
	Title         string            `json:"title"`
	Slug          *string           `json:"slug"`
	Description   string            `json:"description"`
	StarterCode   map[string]string `json:"starterCode"` // { "python": "...", ... }
	Difficulty    string            `json:"difficulty"`  // easy | medium | hard
	GradeLevel    *string           `json:"gradeLevel"`  // K-5 | 6-8 | 9-12 | null
	Tags          []string          `json:"tags"`
	Status        string            `json:"status"` // draft | published | archived
	ForkedFrom    *string           `json:"forkedFrom"`
	TimeLimitMs   *int              `json:"timeLimitMs"`
	MemoryLimitMb *int              `json:"memoryLimitMb"`
	CreatedBy     string            `json:"createdBy"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// TopicProblem is a Problem that was fetched via a topic join; it carries the
// per-topic sort order so UIs can render the attachment ordering without a
// second query.
type TopicProblem struct {
	Problem
	SortOrder int `json:"sortOrder"`
}

type CreateProblemInput struct {
	Scope         string
	ScopeID       *string
	Title         string
	Slug          *string
	Description   string
	StarterCode   map[string]string
	Difficulty    string // "" defaults to "easy"
	GradeLevel    *string
	Tags          []string
	Status        string // "" defaults to "draft"
	TimeLimitMs   *int
	MemoryLimitMb *int
	ForkedFrom    *string
	CreatedBy     string
}

type UpdateProblemInput struct {
	Title       *string
	Slug        *string
	Description *string
	// StarterCode: nil = unchanged; empty map = clear to '{}'.
	StarterCode map[string]string
	Difficulty  *string
	// GradeLevel: nil = unchanged; pointer to "" = clear to NULL; pointer to
	// non-empty = set.
	GradeLevel *string
	// Tags: nil = unchanged; empty slice = set to '{}'.
	Tags          []string
	TimeLimitMs   *int
	MemoryLimitMb *int
	// status is NOT settable here — use SetStatus, which enforces legal
	// transitions atomically.
}

// ListProblemsFilter describes the filter / pagination for ListProblems. The
// "accessible" default (Scope == "") returns every row the viewer is allowed
// to see: platform published + own-org published + own personal + anything
// they authored, plus everything for platform admins.
type ListProblemsFilter struct {
	Scope           string // "" = accessible-to-caller
	ScopeID         *string
	ViewerID        string
	ViewerOrgs      []string
	IsPlatformAdmin bool
	Status          string // "" = any
	Difficulty      string
	GradeLevel      string
	Tags            []string // AND semantics
	Search          string   // ILIKE on title
	Limit           int      // default 20, max 100
	CursorCreatedAt *time.Time
	CursorID        *string
}

// ForkTarget describes where a forked problem should land. The Title override
// is optional (falls back to "<source title> (fork)").
type ForkTarget struct {
	Scope    string
	ScopeID  *string
	Title    *string
	CallerID string
}

type ProblemStore struct {
	db *sql.DB
}

func NewProblemStore(db *sql.DB) *ProblemStore { return &ProblemStore{db: db} }

const problemColumns = `id, scope, scope_id, title, slug, description, starter_code,
  difficulty, grade_level, tags, status, forked_from, time_limit_ms, memory_limit_mb,
  created_by, created_at, updated_at`

// problemSelectColsP returns the problem columns prefixed with p. (for JOINs).
const problemSelectColsP = `p.id, p.scope, p.scope_id, p.title, p.slug, p.description, p.starter_code,
  p.difficulty, p.grade_level, p.tags, p.status, p.forked_from, p.time_limit_ms, p.memory_limit_mb,
  p.created_by, p.created_at, p.updated_at`

func scanProblem(row interface{ Scan(...any) error }) (*Problem, error) {
	p, err := scanProblemRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// scanProblemRow reads the 17 problem columns into a Problem. Nullable columns
// (scope_id, slug, grade_level, forked_from, time_limit_ms, memory_limit_mb)
// are translated into *string / *int. starter_code is unmarshalled from JSONB,
// tags is read via pq.Array.
func scanProblemRow(row interface{ Scan(...any) error }, extras ...any) (*Problem, error) {
	var p Problem
	var scopeID, slug, gradeLevel, forkedFrom sql.NullString
	var timeLimit, memLimit sql.NullInt32
	var starter []byte
	var tags pq.StringArray

	dest := []any{
		&p.ID, &p.Scope, &scopeID, &p.Title, &slug, &p.Description, &starter,
		&p.Difficulty, &gradeLevel, &tags, &p.Status, &forkedFrom, &timeLimit, &memLimit,
		&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	}
	dest = append(dest, extras...)

	if err := row.Scan(dest...); err != nil {
		return nil, err
	}

	if scopeID.Valid {
		v := scopeID.String
		p.ScopeID = &v
	}
	if slug.Valid {
		v := slug.String
		p.Slug = &v
	}
	if gradeLevel.Valid {
		v := gradeLevel.String
		p.GradeLevel = &v
	}
	if forkedFrom.Valid {
		v := forkedFrom.String
		p.ForkedFrom = &v
	}
	if timeLimit.Valid {
		v := int(timeLimit.Int32)
		p.TimeLimitMs = &v
	}
	if memLimit.Valid {
		v := int(memLimit.Int32)
		p.MemoryLimitMb = &v
	}

	if len(starter) == 0 {
		p.StarterCode = map[string]string{}
	} else {
		if err := json.Unmarshal(starter, &p.StarterCode); err != nil {
			return nil, fmt.Errorf("decode starter_code: %w", err)
		}
		if p.StarterCode == nil {
			p.StarterCode = map[string]string{}
		}
	}
	if tags == nil {
		p.Tags = []string{}
	} else {
		p.Tags = []string(tags)
	}

	return &p, nil
}

// CreateProblem inserts a new problem. Empty Difficulty / Status default to
// "easy" / "draft". Nil Tags / StarterCode are coerced to empty collections so
// the row never contains SQL NULL for those columns.
func (s *ProblemStore) CreateProblem(ctx context.Context, in CreateProblemInput) (*Problem, error) {
	if in.Difficulty == "" {
		in.Difficulty = "easy"
	}
	if in.Status == "" {
		in.Status = "draft"
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	if in.StarterCode == nil {
		in.StarterCode = map[string]string{}
	}
	starter, err := json.Marshal(in.StarterCode)
	if err != nil {
		return nil, fmt.Errorf("encode starter_code: %w", err)
	}
	return scanProblem(s.db.QueryRowContext(ctx, `
        INSERT INTO problems (
          scope, scope_id, title, slug, description, starter_code,
          difficulty, grade_level, tags, status, forked_from,
          time_limit_ms, memory_limit_mb, created_by
        ) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11,$12,$13,$14)
        RETURNING `+problemColumns,
		in.Scope, in.ScopeID, in.Title, in.Slug, in.Description, starter,
		in.Difficulty, in.GradeLevel, pq.Array(in.Tags), in.Status, in.ForkedFrom,
		in.TimeLimitMs, in.MemoryLimitMb, in.CreatedBy,
	))
}

// GetProblem returns the problem with the given id, or nil if not found.
func (s *ProblemStore) GetProblem(ctx context.Context, id string) (*Problem, error) {
	return scanProblem(s.db.QueryRowContext(ctx,
		`SELECT `+problemColumns+` FROM problems WHERE id = $1`, id))
}

// ListProblems returns problems matching the filter, paginated by
// (created_at DESC, id DESC). When Scope is empty the query returns every row
// the viewer can access (platform published + own-org published + own
// personal + anything they authored + everything if platform admin); setting
// Scope explicitly narrows within that accessible set.
func (s *ProblemStore) ListProblems(ctx context.Context, f ListProblemsFilter) ([]Problem, error) {
	where := []string{}
	args := []any{}
	idx := 1

	// Accessibility gate. Always applied (including when Scope is set) so that
	// an explicit scope=platform request still can't surface draft platform
	// problems to non-admins.
	if f.IsPlatformAdmin {
		// no-op: platform admins see everything
	} else {
		clauses := []string{
			"(p.scope = 'platform' AND p.status = 'published')",
		}
		// own-org published (if the viewer belongs to any org)
		if len(f.ViewerOrgs) > 0 {
			clauses = append(clauses, fmt.Sprintf("(p.scope = 'org' AND p.scope_id = ANY($%d) AND p.status = 'published')", idx))
			args = append(args, pq.Array(f.ViewerOrgs))
			idx++
		}
		// own personal (draft visible to self)
		if f.ViewerID != "" {
			clauses = append(clauses, fmt.Sprintf("(p.scope = 'personal' AND p.scope_id = $%d)", idx))
			args = append(args, f.ViewerID)
			idx++
			// authored by viewer in any scope
			clauses = append(clauses, fmt.Sprintf("(p.created_by = $%d)", idx))
			args = append(args, f.ViewerID)
			idx++
		}
		where = append(where, "("+strings.Join(clauses, " OR ")+")")
	}

	if f.Scope != "" {
		where = append(where, fmt.Sprintf("p.scope = $%d", idx))
		args = append(args, f.Scope)
		idx++
	}
	if f.ScopeID != nil {
		where = append(where, fmt.Sprintf("p.scope_id = $%d", idx))
		args = append(args, *f.ScopeID)
		idx++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf("p.status = $%d", idx))
		args = append(args, f.Status)
		idx++
	}
	if f.Difficulty != "" {
		where = append(where, fmt.Sprintf("p.difficulty = $%d", idx))
		args = append(args, f.Difficulty)
		idx++
	}
	if f.GradeLevel != "" {
		where = append(where, fmt.Sprintf("p.grade_level = $%d", idx))
		args = append(args, f.GradeLevel)
		idx++
	}
	if len(f.Tags) > 0 {
		// AND semantics: every tag must be present (text[] @> text[]).
		where = append(where, fmt.Sprintf("p.tags @> $%d", idx))
		args = append(args, pq.Array(f.Tags))
		idx++
	}
	if f.Search != "" {
		where = append(where, fmt.Sprintf("p.title ILIKE $%d", idx))
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	if f.CursorCreatedAt != nil && f.CursorID != nil {
		// (created_at, id) < (cursor_created_at, cursor_id) under DESC order.
		where = append(where, fmt.Sprintf("(p.created_at, p.id) < ($%d, $%d)", idx, idx+1))
		args = append(args, *f.CursorCreatedAt, *f.CursorID)
		idx += 2
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	q := `SELECT ` + problemSelectColsP + ` FROM problems p`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	q += fmt.Sprintf(` ORDER BY p.created_at DESC, p.id DESC LIMIT %d`, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Problem{}
	for rows.Next() {
		p, err := scanProblemRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// ListProblemsByTopic returns the problems attached to a topic, ordered by the
// topic's sort_order (ascending) then by title as a tiebreaker.
func (s *ProblemStore) ListProblemsByTopic(ctx context.Context, topicID string) ([]TopicProblem, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT `+problemSelectColsP+`, tp.sort_order
        FROM problems p
        JOIN topic_problems tp ON tp.problem_id = p.id
        WHERE tp.topic_id = $1
        ORDER BY tp.sort_order ASC, p.title ASC`, topicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TopicProblem{}
	for rows.Next() {
		var sortOrder int
		p, err := scanProblemRow(rows, &sortOrder)
		if err != nil {
			return nil, err
		}
		out = append(out, TopicProblem{Problem: *p, SortOrder: sortOrder})
	}
	return out, rows.Err()
}

// UpdateProblem applies partial updates. Nil-valued fields are left untouched.
// For StarterCode, pass nil to leave unchanged or an empty map to clear to
// '{}'::jsonb. For Tags, pass nil to leave unchanged or an empty slice to
// clear to '{}'. For GradeLevel, pass a pointer to "" to clear to NULL. Status
// is intentionally not writable here — use SetStatus.
func (s *ProblemStore) UpdateProblem(ctx context.Context, id string, in UpdateProblemInput) (*Problem, error) {
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
	if in.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *in.Description)
		argIdx++
	}
	if in.StarterCode != nil {
		b, err := json.Marshal(in.StarterCode)
		if err != nil {
			return nil, fmt.Errorf("encode starter_code: %w", err)
		}
		setClauses = append(setClauses, fmt.Sprintf("starter_code = $%d::jsonb", argIdx))
		args = append(args, b)
		argIdx++
	}
	if in.Difficulty != nil {
		setClauses = append(setClauses, fmt.Sprintf("difficulty = $%d", argIdx))
		args = append(args, *in.Difficulty)
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
	if in.Tags != nil {
		setClauses = append(setClauses, fmt.Sprintf("tags = $%d", argIdx))
		args = append(args, pq.Array(in.Tags))
		argIdx++
	}
	if in.TimeLimitMs != nil {
		setClauses = append(setClauses, fmt.Sprintf("time_limit_ms = $%d", argIdx))
		args = append(args, *in.TimeLimitMs)
		argIdx++
	}
	if in.MemoryLimitMb != nil {
		setClauses = append(setClauses, fmt.Sprintf("memory_limit_mb = $%d", argIdx))
		args = append(args, *in.MemoryLimitMb)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetProblem(ctx, id)
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	args = append(args, id)
	q := fmt.Sprintf(
		`UPDATE problems SET %s WHERE id = $%d RETURNING `+problemColumns,
		strings.Join(setClauses, ", "), argIdx,
	)
	return scanProblem(s.db.QueryRowContext(ctx, q, args...))
}

// DeleteProblem removes the problem and returns the deleted row (or nil).
func (s *ProblemStore) DeleteProblem(ctx context.Context, id string) (*Problem, error) {
	return scanProblem(s.db.QueryRowContext(ctx,
		`DELETE FROM problems WHERE id = $1 RETURNING `+problemColumns, id))
}

// SetStatus atomically transitions a problem's status. Permitted transitions:
// draft -> published, published -> archived, archived -> published. Any other
// attempted transition (including a no-op draft -> draft) returns
// ErrInvalidTransition. A missing row returns (nil, nil).
func (s *ProblemStore) SetStatus(ctx context.Context, id, newStatus string) (*Problem, error) {
	var expected string
	switch newStatus {
	case "published":
		// allowed from either draft or archived — use ANY.
		row := s.db.QueryRowContext(ctx,
			`UPDATE problems SET status = $1, updated_at = now()
             WHERE id = $2 AND status IN ('draft','archived')
             RETURNING `+problemColumns,
			newStatus, id)
		p, err := scanProblemRow(row)
		if err == sql.ErrNoRows {
			return s.statusTransitionFailure(ctx, id)
		}
		return p, err
	case "archived":
		expected = "published"
	case "draft":
		// draft is only the initial state; not reachable via SetStatus.
		return nil, ErrInvalidTransition
	default:
		return nil, ErrInvalidTransition
	}

	row := s.db.QueryRowContext(ctx,
		`UPDATE problems SET status = $1, updated_at = now()
         WHERE id = $2 AND status = $3
         RETURNING `+problemColumns,
		newStatus, id, expected)
	p, err := scanProblemRow(row)
	if err == sql.ErrNoRows {
		return s.statusTransitionFailure(ctx, id)
	}
	return p, err
}

// statusTransitionFailure disambiguates "row missing" (return nil, nil) from
// "row present but current status disallows the transition" (return
// ErrInvalidTransition). Called only after a zero-row UPDATE.
func (s *ProblemStore) statusTransitionFailure(ctx context.Context, id string) (*Problem, error) {
	cur, err := s.GetProblem(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, nil
	}
	return nil, ErrInvalidTransition
}

// ForkProblem creates a new problem copied from sourceID under target.Scope /
// target.ScopeID, authored by target.CallerID. Status is forced to "draft";
// forked_from points at the source. Canonical test cases (owner_id IS NULL)
// and every problem_solutions row are copied over, with created_by rewritten
// to the caller on solutions. Private/hidden test cases and attempts are not
// copied. The whole operation runs in a single transaction.
func (s *ProblemStore) ForkProblem(ctx context.Context, sourceID string, target ForkTarget) (*Problem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	src, err := scanProblem(tx.QueryRowContext(ctx,
		`SELECT `+problemColumns+` FROM problems WHERE id = $1 FOR UPDATE`, sourceID))
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

	starter, err := json.Marshal(src.StarterCode)
	if err != nil {
		return nil, fmt.Errorf("encode starter_code: %w", err)
	}

	newRow, err := scanProblem(tx.QueryRowContext(ctx, `
        INSERT INTO problems (
          scope, scope_id, title, description, starter_code,
          difficulty, grade_level, tags, status, forked_from,
          time_limit_ms, memory_limit_mb, created_by
        ) VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8,'draft',$9,$10,$11,$12)
        RETURNING `+problemColumns,
		target.Scope, target.ScopeID, title, src.Description, starter,
		src.Difficulty, src.GradeLevel, pq.Array(src.Tags), src.ID,
		src.TimeLimitMs, src.MemoryLimitMb, target.CallerID,
	))
	if err != nil {
		return nil, err
	}
	if newRow == nil {
		return nil, fmt.Errorf("fork: insert returned no row")
	}

	// Canonical test cases (owner_id IS NULL) — re-parent to the new problem.
	// A new UUID is generated per row via DEFAULT gen_random_uuid().
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO test_cases (problem_id, owner_id, name, stdin, expected_stdout, is_example, "order")
        SELECT $1, NULL, name, stdin, expected_stdout, is_example, "order"
        FROM test_cases
        WHERE problem_id = $2 AND owner_id IS NULL`,
		newRow.ID, src.ID,
	); err != nil {
		return nil, fmt.Errorf("fork: copy canonical test_cases: %w", err)
	}

	// problem_solutions — copy every solution, rewriting created_by to the
	// caller but preserving is_published.
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO problem_solutions (
          problem_id, language, title, code, notes, approach_tags, is_published, created_by
        )
        SELECT $1, language, title, code, notes, approach_tags, is_published, $2
        FROM problem_solutions
        WHERE problem_id = $3`,
		newRow.ID, target.CallerID, src.ID,
	); err != nil {
		return nil, fmt.Errorf("fork: copy problem_solutions: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return newRow, nil
}
