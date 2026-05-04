package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// isValidUUID checks if a string is a valid UUID v4 format.
func isValidUUID(s string) bool {
	return uuidRegex.MatchString(s)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// writeFieldError writes a JSON error response with optional field
// metadata so clients can pin the error to a specific form input
// rather than surfacing it as a generic banner. The body shape is a
// strict superset of writeError's: `{"error": <msg>, "field": <field>}`.
// Plan 071 introduced this to map slug unique-violations to a 409 the
// problem-form can show inline next to the slug input.
func writeFieldError(w http.ResponseWriter, status int, message, field string) {
	writeJSON(w, status, map[string]string{"error": message, "field": field})
}

// decodeJSON decodes the request body into dst. Returns false and writes a 400
// error if decoding fails.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return false
	}
	return true
}

// decodeJSONStrict is decodeJSON but rejects unknown fields with 400.
// Plan 046 added this to mirror the TS-side `.strict()` zod contract on
// the topic create/update routes — stale clients sending the dropped
// lessonContent / starterCode get a clean error instead of a silent
// strip. Use this on endpoints where unknown fields signal a real bug.
func decodeJSONStrict(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return false
	}
	return true
}

// requireUUID validates that a URL param is a valid UUID, writing a 400 error if not.
// Returns the value and true if valid, or empty string and false if invalid.
func requireUUID(w http.ResponseWriter, r *http.Request, param string) (string, bool) {
	val := chi.URLParam(r, param)
	if val != "" && !isValidUUID(val) {
		writeError(w, http.StatusBadRequest, "Invalid UUID: "+param)
		return "", false
	}
	return val, true
}

// constraintCodes is the set of 23xxx integrity constraint violation codes.
var constraintCodes = map[string]bool{
	"23000": true, // integrity_constraint_violation
	"23001": true, // restrict_violation
	"23502": true, // not_null_violation
	"23503": true, // foreign_key_violation
	"23505": true, // unique_violation
	"23514": true, // check_violation
	"23P01": true, // exclusion_violation
}

// isConstraintError returns true if err is a PostgreSQL constraint violation
// (unique, check, foreign-key). Handles both lib/pq and pgx driver errors.
// Callers should map these to 409 Conflict or 400 Bad Request as appropriate.
func isConstraintError(err error) bool {
	// Check lib/pq errors (used by some code paths).
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		if constraintCodes[string(pqErr.Code)] {
			return true
		}
	}
	// Check pgx/pgconn errors (used by pgx stdlib adapter).
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if constraintCodes[pgErr.Code] {
			return true
		}
	}
	return false
}

// ValidateUUIDParam returns a middleware that validates a named URL param is a valid UUID.
func ValidateUUIDParam(param string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			val := chi.URLParam(r, param)
			if val != "" && !isValidUUID(val) {
				writeError(w, http.StatusBadRequest, "Invalid UUID: "+param)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
