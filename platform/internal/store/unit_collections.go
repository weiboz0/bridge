package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// UnitCollection represents a curated sequence of teaching units, scoped
// the same way as teaching_units (platform / org / personal).
type UnitCollection struct {
	ID          string    `json:"id"`
	Scope       string    `json:"scope"`
	ScopeID     *string   `json:"scopeId"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// UnitCollectionItem links a teaching unit to a collection at a specific
// sort position.
type UnitCollectionItem struct {
	CollectionID string `json:"collectionId"`
	UnitID       string `json:"unitId"`
	SortOrder    int    `json:"sortOrder"`
}

// CreateCollectionInput carries the fields required to create a collection.
type CreateCollectionInput struct {
	Scope       string
	ScopeID     *string
	Title       string
	Description string
	CreatedBy   string
}

// UpdateCollectionInput carries optional partial-update fields for a collection.
type UpdateCollectionInput struct {
	Title       *string
	Description *string
}

// UnitCollectionStore manages unit_collections and unit_collection_items rows.
type UnitCollectionStore struct{ db *sql.DB }

// NewUnitCollectionStore constructs a store backed by db.
func NewUnitCollectionStore(db *sql.DB) *UnitCollectionStore {
	return &UnitCollectionStore{db: db}
}

const collectionColumns = `id, scope, scope_id, title, description, created_by, created_at, updated_at`

// scanCollection reads a unit_collections row. Returns (nil, nil) on
// sql.ErrNoRows so callers can use a uniform "not found" check.
func scanCollection(row interface{ Scan(...any) error }) (*UnitCollection, error) {
	var c UnitCollection
	var scopeID sql.NullString

	err := row.Scan(&c.ID, &c.Scope, &scopeID, &c.Title, &c.Description,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if scopeID.Valid {
		v := scopeID.String
		c.ScopeID = &v
	}
	return &c, nil
}

// CreateCollection inserts a new collection row.
func (s *UnitCollectionStore) CreateCollection(ctx context.Context, in CreateCollectionInput) (*UnitCollection, error) {
	return scanCollection(s.db.QueryRowContext(ctx, `
		INSERT INTO unit_collections (scope, scope_id, title, description, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+collectionColumns,
		in.Scope, in.ScopeID, in.Title, in.Description, in.CreatedBy,
	))
}

// GetCollection returns the collection with the given id, or (nil, nil)
// if not found.
func (s *UnitCollectionStore) GetCollection(ctx context.Context, id string) (*UnitCollection, error) {
	return scanCollection(s.db.QueryRowContext(ctx,
		`SELECT `+collectionColumns+` FROM unit_collections WHERE id = $1`, id))
}

// UpdateCollection applies partial updates to the collection. Nil fields
// are left untouched.
func (s *UnitCollectionStore) UpdateCollection(ctx context.Context, id string, in UpdateCollectionInput) (*UnitCollection, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if in.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *in.Title)
		argIdx++
	}
	if in.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *in.Description)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetCollection(ctx, id)
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	args = append(args, id)
	q := fmt.Sprintf(
		`UPDATE unit_collections SET %s WHERE id = $%d RETURNING `+collectionColumns,
		strings.Join(setClauses, ", "), argIdx,
	)
	return scanCollection(s.db.QueryRowContext(ctx, q, args...))
}

// DeleteCollection hard-deletes the collection and returns the deleted row
// (or nil if not found). Cascades remove unit_collection_items.
func (s *UnitCollectionStore) DeleteCollection(ctx context.Context, id string) (*UnitCollection, error) {
	return scanCollection(s.db.QueryRowContext(ctx,
		`DELETE FROM unit_collections WHERE id = $1 RETURNING `+collectionColumns, id))
}

// ListCollections returns all collections for the given scope + scopeID,
// ordered by updated_at DESC. Pass an empty scopeID for scope="platform".
func (s *UnitCollectionStore) ListCollections(ctx context.Context, scope, scopeID string) ([]UnitCollection, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if scopeID == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+collectionColumns+`
			FROM unit_collections
			WHERE scope = $1 AND scope_id IS NULL
			ORDER BY updated_at DESC`, scope)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+collectionColumns+`
			FROM unit_collections
			WHERE scope = $1 AND scope_id = $2
			ORDER BY updated_at DESC`, scope, scopeID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []UnitCollection{}
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// ListCollectionsForViewer returns all collections visible to the viewer,
// filtered by optional scope. Used by the collection list endpoint.
func (s *UnitCollectionStore) ListCollectionsForViewer(ctx context.Context, viewerID string, viewerOrgs []string, isPlatformAdmin bool, scope string) ([]UnitCollection, error) {
	where := []string{}
	args := []any{}
	idx := 1

	if isPlatformAdmin {
		// Platform admins see all collections.
	} else {
		clauses := []string{
			"(scope = 'platform')",
		}
		if len(viewerOrgs) > 0 {
			clauses = append(clauses, fmt.Sprintf(
				"(scope = 'org' AND scope_id = ANY($%d))", idx))
			args = append(args, fmt.Sprintf("{%s}", strings.Join(viewerOrgs, ",")))
			idx++
		}
		if viewerID != "" {
			clauses = append(clauses, fmt.Sprintf(
				"(scope = 'personal' AND scope_id = $%d)", idx))
			args = append(args, viewerID)
			idx++
			clauses = append(clauses, fmt.Sprintf(
				"(created_by = $%d)", idx))
			args = append(args, viewerID)
			idx++
		}
		where = append(where, "("+strings.Join(clauses, " OR ")+")")
	}

	if scope != "" {
		where = append(where, fmt.Sprintf("scope = $%d", idx))
		args = append(args, scope)
		idx++
	}

	q := `SELECT ` + collectionColumns + ` FROM unit_collections`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	q += ` ORDER BY updated_at DESC LIMIT 100`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []UnitCollection{}
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// ── Collection items ────────────────────────────────────────────────────────

// AddItem inserts a unit into a collection at the given sort position.
// Returns the inserted item. If the item already exists, it is a no-op
// that returns the existing row.
func (s *UnitCollectionStore) AddItem(ctx context.Context, collectionID, unitID string, sortOrder int) (*UnitCollectionItem, error) {
	var item UnitCollectionItem
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO unit_collection_items (collection_id, unit_id, sort_order)
		VALUES ($1, $2, $3)
		ON CONFLICT (collection_id, unit_id) DO UPDATE SET sort_order = EXCLUDED.sort_order
		RETURNING collection_id, unit_id, sort_order`,
		collectionID, unitID, sortOrder,
	).Scan(&item.CollectionID, &item.UnitID, &item.SortOrder)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// RemoveItem deletes a unit from a collection. Returns true if the row
// existed and was deleted.
func (s *UnitCollectionStore) RemoveItem(ctx context.Context, collectionID, unitID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM unit_collection_items
		WHERE collection_id = $1 AND unit_id = $2`,
		collectionID, unitID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ReorderItem updates the sort_order of an existing item. Returns (nil, nil)
// if the item does not exist.
func (s *UnitCollectionStore) ReorderItem(ctx context.Context, collectionID, unitID string, sortOrder int) (*UnitCollectionItem, error) {
	var item UnitCollectionItem
	err := s.db.QueryRowContext(ctx, `
		UPDATE unit_collection_items SET sort_order = $3
		WHERE collection_id = $1 AND unit_id = $2
		RETURNING collection_id, unit_id, sort_order`,
		collectionID, unitID, sortOrder,
	).Scan(&item.CollectionID, &item.UnitID, &item.SortOrder)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// ListItems returns all items in a collection, ordered by sort_order ASC.
func (s *UnitCollectionStore) ListItems(ctx context.Context, collectionID string) ([]UnitCollectionItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT collection_id, unit_id, sort_order
		FROM unit_collection_items
		WHERE collection_id = $1
		ORDER BY sort_order ASC, unit_id ASC`,
		collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []UnitCollectionItem{}
	for rows.Next() {
		var item UnitCollectionItem
		if err := rows.Scan(&item.CollectionID, &item.UnitID, &item.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
