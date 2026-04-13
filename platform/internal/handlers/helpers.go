package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
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

// decodeJSON decodes the request body into dst. Returns false and writes a 400
// error if decoding fails.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
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
