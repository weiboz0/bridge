package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Plan 064 — parent ↔ child relationship store. Replaces the
// implicit `class_memberships role="parent"` model.

type ParentLink struct {
	ID           string     `json:"id"`
	ParentUserID string     `json:"parentUserId"`
	ChildUserID  string     `json:"childUserId"`
	Status       string     `json:"status"`
	CreatedBy    string     `json:"createdBy"`
	CreatedAt    time.Time  `json:"createdAt"`
	RevokedAt    *time.Time `json:"revokedAt"`
}

type ParentLinkStore struct {
	db *sql.DB
}

func NewParentLinkStore(db *sql.DB) *ParentLinkStore {
	return &ParentLinkStore{db: db}
}

const parentLinkColumns = `id, parent_user_id, child_user_id, status, created_by, created_at, revoked_at`

func scanParentLink(row interface{ Scan(...any) error }) (*ParentLink, error) {
	var l ParentLink
	err := row.Scan(&l.ID, &l.ParentUserID, &l.ChildUserID, &l.Status, &l.CreatedBy, &l.CreatedAt, &l.RevokedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// ErrParentLinkExists is returned by CreateLink when an active link
// already exists for the (parent, child) pair. Maps to a 409 in the
// admin handler.
var ErrParentLinkExists = errors.New("active parent_link already exists for (parent, child)")

// IsParentOfAnyParticipant reports whether `parentID` has an ACTIVE
// parent_link to ANY user listed as a participant of `sessionID`.
// Used by GetSessionTopics' parent-aware gate (plan 064 Phase 5)
// so a parent can read the agenda for sessions their child is
// joined to, without needing a class membership themselves.
//
// Single SQL EXISTS, indexed on (parent_user_id, child_user_id) and
// (session_id, user_id).
func (s *ParentLinkStore) IsParentOfAnyParticipant(ctx context.Context, parentID, sessionID string) (bool, error) {
	if parentID == "" || sessionID == "" {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM parent_links pl
			INNER JOIN session_participants sp
				ON sp.user_id = pl.child_user_id
			WHERE pl.parent_user_id = $1
			  AND pl.status = 'active'
			  AND sp.session_id = $2
		)`,
		parentID, sessionID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// IsParentOf reports whether `parentID` has an ACTIVE parent_link
// to `childID`. Existence-only on the matching row with
// status='active'. The single SQL EXISTS is sub-millisecond against
// the partial unique index `parent_links_active_uniq`.
//
// Plan 053b will consume this in `authorizeSessionDoc` for the
// parent-viewer migration.
func (s *ParentLinkStore) IsParentOf(ctx context.Context, parentID, childID string) (bool, error) {
	if parentID == "" || childID == "" {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM parent_links
			WHERE parent_user_id = $1 AND child_user_id = $2 AND status = 'active'
		)`,
		parentID, childID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// CreateLinkWithMembership atomically inserts a new ACTIVE parent_link
// AND upserts an active org_memberships{role:'parent'} row for the
// parent in `linkOrgID`. Both writes run in one transaction; if the
// membership upsert fails (e.g., FK violation, concurrent org delete),
// the link is rolled back so we never produce an orphaned link with
// no membership — Codex post-impl Q7 of plan 070 phase 1.
//
// If an active link already exists for the (parent, child) pair,
// returns ErrParentLinkExists.
//
// The reactivation semantic is the same as `OrgStore.UpsertActiveMembership`:
// `ON CONFLICT (org_id, user_id, role) DO UPDATE SET status='active'`,
// which flips a previously-suspended membership back to active.
func (s *ParentLinkStore) CreateLinkWithMembership(
	ctx context.Context,
	parentID, childID, createdBy, linkOrgID, role string,
) (*ParentLink, error) {
	if parentID == "" || childID == "" || createdBy == "" || linkOrgID == "" || role == "" {
		return nil, errors.New("parentID, childID, createdBy, linkOrgID, and role are required")
	}
	if parentID == childID {
		return nil, errors.New("parent and child must be different users")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	id := uuid.New().String()
	link, err := scanParentLink(tx.QueryRowContext(ctx,
		`INSERT INTO parent_links (id, parent_user_id, child_user_id, status, created_by, created_at)
		 VALUES ($1, $2, $3, 'active', $4, $5)
		 RETURNING `+parentLinkColumns,
		id, parentID, childID, createdBy, time.Now(),
	))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrParentLinkExists
		}
		return nil, err
	}

	// Upsert membership inside the same tx. If this fails the link
	// rolls back via deferred Rollback.
	membershipID := uuid.New().String()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO org_memberships (id, org_id, user_id, role, status, created_at)
		 VALUES ($1, $2, $3, $4, 'active', $5)
		 ON CONFLICT (org_id, user_id, role)
		 DO UPDATE SET status = 'active'`,
		membershipID, linkOrgID, parentID, role, time.Now(),
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return link, nil
}

// CreateLink inserts a new ACTIVE parent_link. If an active link
// already exists for the pair, returns ErrParentLinkExists. Revoked
// rows for the same pair don't conflict (partial unique).
//
// Prefer CreateLinkWithMembership when called from an org-scoped
// context (plan 070 phase 1). This naked variant remains for the
// platform-admin handler which doesn't have an org context.
func (s *ParentLinkStore) CreateLink(ctx context.Context, parentID, childID, createdBy string) (*ParentLink, error) {
	if parentID == "" || childID == "" || createdBy == "" {
		return nil, errors.New("parentUserId, childUserId, and createdBy are required")
	}
	if parentID == childID {
		return nil, errors.New("parent and child must be different users")
	}
	id := uuid.New().String()
	link, err := scanParentLink(s.db.QueryRowContext(ctx,
		`INSERT INTO parent_links (id, parent_user_id, child_user_id, status, created_by, created_at)
		 VALUES ($1, $2, $3, 'active', $4, $5)
		 RETURNING `+parentLinkColumns,
		id, parentID, childID, createdBy, time.Now(),
	))
	if err != nil {
		// Unique violation on the partial index → active link
		// exists. Reuses the shared isUniqueViolation helper from
		// topic_problems.go (same package).
		if isUniqueViolation(err) {
			return nil, ErrParentLinkExists
		}
		return nil, err
	}
	return link, nil
}

// RevokeLink flips an active link to status='revoked' with
// revoked_at = now(). No-op if the link is already revoked. Returns
// the updated row, or nil if the id doesn't exist.
func (s *ParentLinkStore) RevokeLink(ctx context.Context, linkID string) (*ParentLink, error) {
	return scanParentLink(s.db.QueryRowContext(ctx,
		`UPDATE parent_links
		 SET status = 'revoked', revoked_at = COALESCE(revoked_at, $1)
		 WHERE id = $2
		 RETURNING `+parentLinkColumns,
		time.Now(), linkID,
	))
}

// GetLink returns a single link by ID, or nil if absent.
func (s *ParentLinkStore) GetLink(ctx context.Context, linkID string) (*ParentLink, error) {
	return scanParentLink(s.db.QueryRowContext(ctx,
		`SELECT `+parentLinkColumns+` FROM parent_links WHERE id = $1`,
		linkID,
	))
}

// ListByParent returns all links (active and revoked) for a parent,
// ordered by created_at DESC. Used by the admin "what links does
// this user have" view.
func (s *ParentLinkStore) ListByParent(ctx context.Context, parentID string) ([]ParentLink, error) {
	return s.list(ctx, `WHERE parent_user_id = $1 ORDER BY created_at DESC`, parentID)
}

// ListByChild returns all links for a child, ordered by created_at
// DESC.
func (s *ParentLinkStore) ListByChild(ctx context.Context, childID string) ([]ParentLink, error) {
	return s.list(ctx, `WHERE child_user_id = $1 ORDER BY created_at DESC`, childID)
}

// ListChildrenForParent returns just the active child IDs for a
// parent. TS-side getLinkedChildren analogue (without the user
// JOIN, which the caller does separately).
func (s *ParentLinkStore) ListChildrenForParent(ctx context.Context, parentID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT child_user_id FROM parent_links WHERE parent_user_id = $1 AND status = 'active'`,
		parentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *ParentLinkStore) list(ctx context.Context, where string, args ...any) ([]ParentLink, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+parentLinkColumns+` FROM parent_links `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ParentLink
	for rows.Next() {
		var l ParentLink
		if err := rows.Scan(&l.ID, &l.ParentUserID, &l.ChildUserID, &l.Status, &l.CreatedBy, &l.CreatedAt, &l.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if out == nil {
		out = []ParentLink{}
	}
	return out, rows.Err()
}

// ParentLinkRow is an enriched parent_link row used by the
// org-admin list view (plan 070). Adds parent email/name + child
// name + the class the child belongs to (one of, since a child may
// be in multiple classes — query picks the most recently joined
// active class). Pure read shape; the caller doesn't write back.
type ParentLinkRow struct {
	ParentLink
	ParentEmail string  `json:"parentEmail"`
	ParentName  string  `json:"parentName"`
	ChildEmail  string  `json:"childEmail"`
	ChildName   string  `json:"childName"`
	// ClassID/ClassName are the child's most-recently-joined active
	// class in the org. Nullable — child may be in zero classes (rare,
	// but possible if all class memberships were inactivated).
	ClassID   *string `json:"classId,omitempty"`
	ClassName *string `json:"className,omitempty"`
}

// ListByOrgFilters narrows the org list query. All fields optional:
// no filter → all active links for children in the org.
type ListByOrgFilters struct {
	// Status: "active" (default), "revoked", or "all".
	Status string
	// ParentEmail: case-insensitive prefix match (LOWER(...) LIKE ...).
	ParentEmail string
	// ChildUserID: exact match on the child user.
	ChildUserID string
	// ClassID: only links whose child has an active membership in this class.
	ClassID string
}

// ListByOrg returns parent_links scoped to children who are active
// students in any class belonging to `orgID`. Joins users + classes
// for the enriched display shape. Filters apply post-scope.
//
// Plan 070 Phase 1 — primary query for the org-admin /org/parent-links page.
func (s *ParentLinkStore) ListByOrg(ctx context.Context, orgID string, f ListByOrgFilters) ([]ParentLinkRow, error) {
	if orgID == "" {
		return nil, errors.New("orgID is required")
	}
	args := []any{orgID}
	idx := 2

	// Base scope: child must have an active student membership in
	// some active class of this org. EXISTS subquery keeps it cheap.
	statusClause := `pl.status = 'active'`
	switch f.Status {
	case "all":
		statusClause = `1=1`
	case "revoked":
		statusClause = `pl.status = 'revoked'`
	}

	parentFilter := ``
	if f.ParentEmail != "" {
		parentFilter = ` AND LOWER(pu.email) LIKE LOWER($` + sqlIdx(idx) + `)`
		args = append(args, f.ParentEmail+"%")
		idx++
	}

	childFilter := ``
	if f.ChildUserID != "" {
		childFilter = ` AND pl.child_user_id = $` + sqlIdx(idx)
		args = append(args, f.ChildUserID)
		idx++
	}

	classFilter := ``
	if f.ClassID != "" {
		// Scope the class filter to active classes in THIS org —
		// defense-in-depth so a stale/cross-org class id doesn't
		// surface rows. The displayed `classId/className` comes
		// from the lateral most-recent class, which may differ
		// from the filter id when a child is in multiple classes
		// (acceptable for v1 — operator is filtering, not
		// reporting).
		classFilter = ` AND EXISTS (
			SELECT 1 FROM class_memberships cm2
			JOIN classes c2 ON c2.id = cm2.class_id
			WHERE cm2.user_id = pl.child_user_id
			  AND cm2.class_id = $` + sqlIdx(idx) + `
			  AND cm2.role = 'student'
			  AND c2.org_id = $1
			  AND c2.status = 'active'
		)`
		args = append(args, f.ClassID)
		idx++
	}

	// Lateral join surfaces ONE class per row — the child's most
	// recently joined active student class in this org. Without
	// LATERAL we'd duplicate links once per class membership.
	// parent_links columns are aliased pl.* — without the prefix
	// the join with `users` produces an "ambiguous id" error.
	plCols := `pl.id, pl.parent_user_id, pl.child_user_id, pl.status, pl.created_by, pl.created_at, pl.revoked_at`
	q := `
		SELECT
			` + plCols + `,
			pu.email, pu.name,
			cu.email, cu.name,
			klass.id, klass.title
		FROM parent_links pl
		JOIN users pu ON pu.id = pl.parent_user_id
		JOIN users cu ON cu.id = pl.child_user_id
		LEFT JOIN LATERAL (
			SELECT c.id, c.title
			FROM class_memberships cm
			JOIN classes c ON c.id = cm.class_id
			WHERE cm.user_id = pl.child_user_id
			  AND cm.role = 'student'
			  AND c.org_id = $1
			  AND c.status = 'active'
			ORDER BY cm.joined_at DESC
			LIMIT 1
		) klass ON TRUE
		WHERE ` + statusClause + `
		  AND EXISTS (
			SELECT 1 FROM class_memberships cm
			JOIN classes c ON c.id = cm.class_id
			WHERE cm.user_id = pl.child_user_id
			  AND cm.role = 'student'
			  AND c.org_id = $1
			  AND c.status = 'active'
		  )` + parentFilter + childFilter + classFilter + `
		ORDER BY pl.created_at DESC`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ParentLinkRow{}
	for rows.Next() {
		var r ParentLinkRow
		if err := rows.Scan(
			&r.ID, &r.ParentUserID, &r.ChildUserID, &r.Status, &r.CreatedBy, &r.CreatedAt, &r.RevokedAt,
			&r.ParentEmail, &r.ParentName,
			&r.ChildEmail, &r.ChildName,
			&r.ClassID, &r.ClassName,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ChildBelongsToOrg reports whether `childID` is an active student
// in any active class of `orgID`. Used by the org-admin parent-link
// authorization gate to prevent cross-org linkage attempts.
func (s *ParentLinkStore) ChildBelongsToOrg(ctx context.Context, childID, orgID string) (bool, error) {
	if childID == "" || orgID == "" {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM class_memberships cm
			JOIN classes c ON c.id = cm.class_id
			WHERE cm.user_id = $1
			  AND cm.role = 'student'
			  AND c.org_id = $2
			  AND c.status = 'active'
		)`,
		childID, orgID,
	).Scan(&exists)
	return exists, err
}

// sqlIdx formats `$N` argument indexes — keeps the query string
// readable without scattered fmt.Sprintf calls.
func sqlIdx(i int) string {
	return strconv.Itoa(i)
}

// isUniqueViolation lives in topic_problems.go (same package).
