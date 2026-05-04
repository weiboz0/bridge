package store

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// IsUniqueViolationOn reports whether err is a Postgres 23505
// unique-violation against the named constraint. Handles both the
// lib/pq.Error shape and the pgx/pgconn.PgError shape — Bridge uses
// pgx for the pool but lib/pq error types still appear in some
// paths (e.g., array binding via pq.Array on the bound parameters).
//
// Promoted from teaching_units.go::isUniqueViolationOn so problems.go
// (and any future store) can reuse the dual-driver check without
// duplicating it.
func IsUniqueViolationOn(err error, constraint string) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		if pqErr.Code == "23505" && pqErr.Constraint == constraint {
			return true
		}
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" && pgErr.ConstraintName == constraint {
			return true
		}
	}
	return false
}
