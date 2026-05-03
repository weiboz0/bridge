package store

import (
	"context"
	"database/sql"
	"errors"
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

// CreateLink inserts a new ACTIVE parent_link. If an active link
// already exists for the pair, returns ErrParentLinkExists. Revoked
// rows for the same pair don't conflict (partial unique).
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

// isUniqueViolation lives in topic_problems.go (same package).
