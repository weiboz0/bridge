package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrBookNotFound = errors.New("book not found")

type Book struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Scope       string    `json:"scope"`
	ScopeID     *string   `json:"scopeId"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateBookInput struct {
	Title       string
	Description string
	Scope       string
	ScopeID     *string
	CreatedBy   string
}

type UpdateBookInput struct {
	Title       *string
	Description *string
}

type BookFilter struct {
	Scope   string
	ScopeID *string
}

type BookStore struct{ db *sql.DB }

func NewBookStore(db *sql.DB) *BookStore { return &BookStore{db: db} }

const bookColumns = `id, title, description, scope, scope_id, created_by, created_at, updated_at`

func scanBook(row interface{ Scan(...any) error }) (*Book, error) {
	var b Book
	var scopeID sql.NullString
	err := row.Scan(&b.ID, &b.Title, &b.Description, &b.Scope, &scopeID, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if scopeID.Valid {
		v := scopeID.String
		b.ScopeID = &v
	}
	return &b, nil
}

func validateBookFields(title, scope string, scopeID *string) (string, error) {
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" || len(trimmedTitle) > 255 {
		return "", fmt.Errorf("title must be 1-255 characters")
	}
	switch scope {
	case "platform":
		if scopeID != nil {
			return "", fmt.Errorf("scopeId must be empty for platform books")
		}
	case "org":
		if scopeID == nil || *scopeID == "" {
			return "", fmt.Errorf("scopeId is required for org books")
		}
	default:
		return "", fmt.Errorf("scope must be platform or org")
	}
	return trimmedTitle, nil
}

func (s *BookStore) CreateBook(ctx context.Context, in CreateBookInput) (*Book, error) {
	title, err := validateBookFields(in.Title, in.Scope, in.ScopeID)
	if err != nil {
		return nil, err
	}
	return scanBook(s.db.QueryRowContext(ctx, `
		INSERT INTO books (title, description, scope, scope_id, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+bookColumns,
		title, in.Description, in.Scope, in.ScopeID, in.CreatedBy,
	))
}

func (s *BookStore) GetBook(ctx context.Context, id string) (*Book, error) {
	return scanBook(s.db.QueryRowContext(ctx, `SELECT `+bookColumns+` FROM books WHERE id = $1`, id))
}

func (s *BookStore) ListBooks(ctx context.Context, filter BookFilter) ([]Book, error) {
	where := []string{}
	args := []any{}
	idx := 1
	if filter.Scope != "" {
		where = append(where, fmt.Sprintf("scope = $%d", idx))
		args = append(args, filter.Scope)
		idx++
	}
	if filter.ScopeID != nil {
		where = append(where, fmt.Sprintf("scope_id = $%d", idx))
		args = append(args, *filter.ScopeID)
		idx++
	}
	q := `SELECT ` + bookColumns + ` FROM books`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	q += ` ORDER BY updated_at DESC, title ASC`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	books := []Book{}
	for rows.Next() {
		book, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, *book)
	}
	return books, rows.Err()
}

func (s *BookStore) UpdateBook(ctx context.Context, id string, in UpdateBookInput) (*Book, error) {
	setClauses := []string{}
	args := []any{}
	idx := 1
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" || len(title) > 255 {
			return nil, fmt.Errorf("title must be 1-255 characters")
		}
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", idx))
		args = append(args, title)
		idx++
	}
	if in.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", idx))
		args = append(args, *in.Description)
		idx++
	}
	if len(setClauses) == 0 {
		return s.GetBook(ctx, id)
	}
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", idx))
	args = append(args, time.Now())
	idx++
	args = append(args, id)
	return scanBook(s.db.QueryRowContext(ctx,
		fmt.Sprintf(`UPDATE books SET %s WHERE id = $%d RETURNING `+bookColumns, strings.Join(setClauses, ", "), idx),
		args...,
	))
}

func (s *BookStore) DeleteBook(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM books WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrBookNotFound
	}
	return nil
}
