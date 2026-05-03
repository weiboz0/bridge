package handlers

import (
	"net/http"
	"net/mail"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 065 Phase 1 — server-to-server endpoint that mints
// `bridge.session` JWTs.
//
// Auth.js completes sign-in (Google or credentials), then Next.js's
// Edge middleware lazily calls this endpoint to obtain a Bridge
// session token. The browser carries the resulting cookie alongside
// Auth.js's JWE; once Phase 3 ships, Go middleware verifies this
// token instead of decrypting the JWE.
//
// Bearer-protected by BRIDGE_INTERNAL_SECRET — this is NOT a
// user-facing endpoint and must never be reachable from the
// browser. Mounted OUTSIDE RequireAuth so the bearer check runs
// first (mirrors plan 053's /api/internal/realtime/auth pattern).

// InternalSessionsHandler exposes POST /api/internal/sessions.
type InternalSessionsHandler struct {
	Users *store.UserStore

	// PrimarySigningSecret is the first entry of BRIDGE_SESSION_SECRETS
	// — the one we mint with. Verify uses the full list (see
	// auth.VerifyBridgeSession), but mint always uses the primary.
	PrimarySigningSecret string

	// InternalBearer is the shared HMAC the Auth.js mint helper sends
	// in Authorization. Empty disables this endpoint (returns 503).
	InternalBearer string
}

// Routes mounts the handler. Caller MUST mount outside RequireAuth.
func (h *InternalSessionsHandler) Routes(r chi.Router) {
	r.Route("/api/internal/sessions", func(r chi.Router) {
		r.Post("/", h.Mint)
	})
}

// mintSessionRequest is what the Auth.js helper posts.
type mintSessionRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// mintSessionResponse mirrors plan 053's mint response shape so the
// client-side helper can refresh on `expiresAt - 24h`.
type mintSessionResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// bridgeSessionTTL matches Auth.js's default session lifetime so a
// browser without re-mint activity is treated the same way.
const bridgeSessionTTL = 7 * 24 * time.Hour

// Mint handles POST /api/internal/sessions.
//
// Order of checks is deliberate:
//  1. Endpoint configuration (503 if either secret is unset).
//  2. Bearer authentication (401 if missing/wrong) — runs BEFORE
//     body parsing so an unauthenticated caller can't probe payload
//     validation.
//  3. Body validation (400).
//  4. User lookup by email (404 if missing).
//  5. Sign + return.
func (h *InternalSessionsHandler) Mint(w http.ResponseWriter, r *http.Request) {
	if h.PrimarySigningSecret == "" || h.InternalBearer == "" {
		writeError(w, http.StatusServiceUnavailable, "Bridge sessions not configured")
		return
	}

	expected := "Bearer " + h.InternalBearer
	if r.Header.Get("Authorization") != expected {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body mintSessionRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if _, err := mail.ParseAddress(body.Email); err != nil {
		writeError(w, http.StatusBadRequest, "email is invalid")
		return
	}

	user, err := h.Users.GetUserByEmail(r.Context(), body.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if user == nil {
		// The email reached us via Auth.js, which only signs in
		// users that signIn() has approved (Google + signup-intent
		// flow inserts a row first). A missing row here means the
		// caller is forging requests OR there's a race between the
		// signIn callback and this mint call. Either way we don't
		// want to mint a token referencing a non-existent user.
		writeError(w, http.StatusNotFound, "User not found")
		return
	}

	displayName := body.Name
	if displayName == "" {
		displayName = user.Name
	}

	token, err := auth.SignBridgeSession(
		h.PrimarySigningSecret,
		user.ID,
		user.Email,
		displayName,
		user.IsPlatformAdmin,
		bridgeSessionTTL,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Token sign failed")
		return
	}

	writeJSON(w, http.StatusOK, mintSessionResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(bridgeSessionTTL),
	})
}
