package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	sessionLifecycleLockClass  int32 = 1112687687 // 0x42524447, Bridge session lifecycle.
	classReplacementLockClass  int32 = 1112687436 // 0x4252434c, Bridge class replacement.
	sessionFreezeLeaseDuration       = 15 * time.Second
)

var (
	ErrSessionEndInProgress         = errors.New("session end in progress")
	ErrSessionSnapshotCountMismatch = errors.New("session snapshot count mismatch")
)

type sessionFreezeLease struct {
	Token     string
	Remaining time.Duration
}

type freezeLeaseValidation struct {
	Allowed   bool
	Remaining time.Duration
}

type sessionEndResult struct {
	WhiteboardServerArchiveComplete *bool
	EndedAt                         *time.Time
}

// SessionEndPreparation is the short durable handoff between the Go lifecycle
// lease transaction and the Hocuspocus control request. It intentionally owns
// no database lock while the HTTP request is in flight.
type SessionEndPreparation struct {
	Token     string
	CanvasIDs []string
}

// SessionEndResult exposes only durable state needed by API producers.
type SessionEndResult struct {
	WhiteboardServerArchiveComplete *bool
	EndedAt                         *time.Time
}

// CanvasSnapshot is one persisted state from an already fenced canvas bundle.
type CanvasSnapshot struct {
	CanvasID string
	YjsState string
}

type ReplacedSession struct {
	ID                              string  `json:"id"`
	WhiteboardServerArchiveComplete bool    `json:"whiteboardServerArchiveComplete"`
	ClearedFreezeToken              *string `json:"-"`
}

func sessionLifecycleAdvisoryKey(id string) (int32, error) {
	canonical := strings.ToLower(id)
	parsed, err := uuid.Parse(canonical)
	if err != nil || parsed.String() != canonical {
		return 0, fmt.Errorf("invalid canonical UUID %q", id)
	}
	v, err := strconv.ParseUint(canonical[:8], 16, 32)
	if err != nil {
		return 0, fmt.Errorf("parse lifecycle UUID key: %w", err)
	}
	return int32(uint32(v)), nil
}

// sortedSessionLifecycleIDs provides a process-local mirror of the database
// ordering used by replacement: derived signed key first, then canonical UUID.
// The SQL ORDER BY remains authoritative across processes; this duplicate sort
// makes every lock acquisition order explicit and regression-testable.
func sortedSessionLifecycleIDs(ids []string) ([]string, error) {
	ordered := append([]string(nil), ids...)
	keys := make(map[string]int32, len(ordered))
	for _, id := range ordered {
		key, err := sessionLifecycleAdvisoryKey(id)
		if err != nil {
			return nil, err
		}
		keys[id] = key
	}
	sort.Slice(ordered, func(i, j int) bool {
		if keys[ordered[i]] != keys[ordered[j]] {
			return keys[ordered[i]] < keys[ordered[j]]
		}
		return ordered[i] < ordered[j]
	})
	return ordered, nil
}

func lockSessionLifecycle(ctx context.Context, tx *sql.Tx, sessionID string, shared bool) error {
	key, err := sessionLifecycleAdvisoryKey(sessionID)
	if err != nil {
		return err
	}
	fn := "pg_advisory_xact_lock"
	if shared {
		fn = "pg_advisory_xact_lock_shared"
	}
	_, err = tx.ExecContext(ctx, `SELECT `+fn+`($1, $2)`, sessionLifecycleLockClass, key)
	return err
}

func lockClassReplacement(ctx context.Context, tx *sql.Tx, classID string) error {
	key, err := sessionLifecycleAdvisoryKey(classID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1, $2)`, classReplacementLockClass, key)
	return err
}

func acquireSessionFreezeLease(ctx context.Context, db *sql.DB, sessionID, token string) (sessionFreezeLease, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return sessionFreezeLease{}, err
	}
	defer tx.Rollback()
	if err := lockSessionLifecycle(ctx, tx, sessionID, false); err != nil {
		return sessionFreezeLease{}, err
	}
	var remaining float64
	err = tx.QueryRowContext(ctx, `
		UPDATE sessions
		SET canvas_freeze_token = $2::uuid,
		    canvas_freeze_until = clock_timestamp() + ($3::bigint * interval '1 second')
		WHERE id = $1 AND status = 'live'
		  AND (canvas_freeze_until IS NULL OR canvas_freeze_until <= clock_timestamp() OR canvas_freeze_token = $2::uuid)
		RETURNING EXTRACT(epoch FROM canvas_freeze_until - clock_timestamp())`, sessionID, token, int64(sessionFreezeLeaseDuration/time.Second)).Scan(&remaining)
	if err == sql.ErrNoRows {
		return sessionFreezeLease{}, ErrSessionEndInProgress
	}
	if err != nil {
		return sessionFreezeLease{}, err
	}
	if err := tx.Commit(); err != nil {
		return sessionFreezeLease{}, err
	}
	return sessionFreezeLease{Token: token, Remaining: time.Duration(remaining * float64(time.Second))}, nil
}

func validateSessionFreezeLease(ctx context.Context, db *sql.DB, sessionID, token string) (freezeLeaseValidation, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return freezeLeaseValidation{}, err
	}
	defer tx.Rollback()
	if err := lockSessionLifecycle(ctx, tx, sessionID, true); err != nil {
		return freezeLeaseValidation{}, err
	}
	var remaining sql.NullFloat64
	err = tx.QueryRowContext(ctx, `SELECT EXTRACT(epoch FROM canvas_freeze_until - clock_timestamp())
		FROM sessions WHERE id = $1 AND status = 'live' AND canvas_freeze_token = $2::uuid AND canvas_freeze_until > clock_timestamp()`, sessionID, token).Scan(&remaining)
	if err == sql.ErrNoRows {
		return freezeLeaseValidation{}, tx.Commit()
	}
	if err != nil {
		return freezeLeaseValidation{}, err
	}
	if err := tx.Commit(); err != nil {
		return freezeLeaseValidation{}, err
	}
	return freezeLeaseValidation{Allowed: true, Remaining: time.Duration(remaining.Float64 * float64(time.Second))}, nil
}

// ValidateSessionFreezeLease holds the shared lifecycle lock while checking
// the exact database-clock lease. It is used by the private Node callback.
func (s *SessionStore) ValidateSessionFreezeLease(ctx context.Context, sessionID, token string) (bool, time.Duration, error) {
	result, err := validateSessionFreezeLease(ctx, s.db, sessionID, token)
	return result.Allowed, result.Remaining, err
}

// CanvasLifecycleState is a current-state read protected by the shared
// advisory lock. Authorization uses this instead of racing a plain session
// lookup with an in-flight end lease.
type CanvasLifecycleState struct {
	Status       string
	FreezeActive bool
}

func (s *SessionStore) CanvasLifecycleState(ctx context.Context, sessionID string) (CanvasLifecycleState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CanvasLifecycleState{}, err
	}
	defer tx.Rollback()
	if err := lockSessionLifecycle(ctx, tx, sessionID, true); err != nil {
		return CanvasLifecycleState{}, err
	}
	var result CanvasLifecycleState
	err = tx.QueryRowContext(ctx, `SELECT status, COALESCE(canvas_freeze_until > clock_timestamp(), false)
		FROM sessions WHERE id = $1`, sessionID).Scan(&result.Status, &result.FreezeActive)
	if err == sql.ErrNoRows {
		return CanvasLifecycleState{}, sql.ErrNoRows
	}
	if err != nil {
		return CanvasLifecycleState{}, err
	}
	if err := tx.Commit(); err != nil {
		return CanvasLifecycleState{}, err
	}
	return result, nil
}

// PrepareSessionEnd obtains the exclusive lease and lists the authoritative
// canvas set in one transaction. The transaction commits before Hocuspocus is
// called, avoiding a self-deadlock with its shared-lock callback.
func (s *SessionStore) PrepareSessionEnd(ctx context.Context, sessionID string) (SessionEndPreparation, error) {
	token := uuid.NewString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionEndPreparation{}, err
	}
	defer tx.Rollback()
	if err := lockSessionLifecycle(ctx, tx, sessionID, false); err != nil {
		return SessionEndPreparation{}, err
	}
	var remaining float64
	err = tx.QueryRowContext(ctx, `UPDATE sessions SET canvas_freeze_token = $2::uuid,
		canvas_freeze_until = clock_timestamp() + ($3::bigint * interval '1 second')
		WHERE id = $1 AND status = 'live'
		  AND (canvas_freeze_until IS NULL OR canvas_freeze_until <= clock_timestamp() OR canvas_freeze_token = $2::uuid)
		RETURNING EXTRACT(epoch FROM canvas_freeze_until - clock_timestamp())`, sessionID, token, int64(sessionFreezeLeaseDuration/time.Second)).Scan(&remaining)
	if err == sql.ErrNoRows {
		return SessionEndPreparation{}, ErrSessionEndInProgress
	}
	if err != nil {
		return SessionEndPreparation{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM session_canvases WHERE session_id = $1 ORDER BY id`, sessionID)
	if err != nil {
		return SessionEndPreparation{}, err
	}
	defer rows.Close()
	prep := SessionEndPreparation{Token: token, CanvasIDs: make([]string, 0)}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return SessionEndPreparation{}, err
		}
		prep.CanvasIDs = append(prep.CanvasIDs, id)
	}
	if err := rows.Err(); err != nil {
		return SessionEndPreparation{}, err
	}
	if err := tx.Commit(); err != nil {
		return SessionEndPreparation{}, err
	}
	return prep, nil
}

func (s *SessionStore) CompleteSessionConfirmed(ctx context.Context, sessionID, token string, snapshots []CanvasSnapshot) error {
	return completeSessionConfirmed(ctx, s.db, sessionID, token, snapshots)
}
func (s *SessionStore) CompleteSessionConfirmedResult(ctx context.Context, sessionID, token string, snapshots []CanvasSnapshot) (SessionEndResult, error) {
	var endedAt time.Time
	if err := completeSessionConfirmedAt(ctx, s.db, sessionID, token, snapshots, &endedAt); err != nil {
		return SessionEndResult{}, err
	}
	complete := true
	return SessionEndResult{WhiteboardServerArchiveComplete: &complete, EndedAt: &endedAt}, nil
}

func (s *SessionStore) CompleteSessionDegraded(ctx context.Context, sessionID, token string) (SessionEndResult, error) {
	result, err := completeSessionDegradedResult(ctx, s.db, sessionID, token)
	return SessionEndResult{WhiteboardServerArchiveComplete: result.WhiteboardServerArchiveComplete, EndedAt: result.EndedAt}, err
}

func (s *SessionStore) AbortSessionFreezeLease(ctx context.Context, sessionID, token string) error {
	return abortSessionFreezeLease(ctx, s.db, sessionID, token)
}

func completeSessionConfirmed(ctx context.Context, db *sql.DB, sessionID, token string, snapshots []CanvasSnapshot) error {
	return completeSessionConfirmedAt(ctx, db, sessionID, token, snapshots, nil)
}
func completeSessionConfirmedAt(ctx context.Context, db *sql.DB, sessionID, token string, snapshots []CanvasSnapshot, endedOut *time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockSessionLifecycle(ctx, tx, sessionID, false); err != nil {
		return err
	}
	var endedAt time.Time
	err = tx.QueryRowContext(ctx, `UPDATE sessions SET status = 'ended', ended_at = clock_timestamp(),
		whiteboard_server_archive_complete = true, canvas_freeze_token = NULL, canvas_freeze_until = NULL
		WHERE id = $1 AND status = 'live' AND canvas_freeze_token = $2::uuid AND canvas_freeze_until > clock_timestamp() RETURNING ended_at`, sessionID, token).Scan(&endedAt)
	if err == sql.ErrNoRows {
		return ErrSessionEndInProgress
	}
	if err != nil {
		return err
	}
	if len(snapshots) > 0 {
		ids, states := make([]string, len(snapshots)), make([]string, len(snapshots))
		for i, snapshot := range snapshots {
			ids[i], states[i] = snapshot.CanvasID, snapshot.YjsState
		}
		result, err := tx.ExecContext(ctx, `UPDATE session_canvases AS c SET yjs_state = bundle.state, updated_at = clock_timestamp()
			FROM unnest($2::uuid[], $3::text[]) AS bundle(id, state)
			WHERE c.session_id = $1 AND c.id = bundle.id`, sessionID, pq.Array(ids), pq.Array(states))
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != int64(len(snapshots)) {
			return ErrSessionSnapshotCountMismatch
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if endedOut != nil {
		*endedOut = endedAt
	}
	return nil
}

// abortSessionFreezeLease only releases the still-live operation that owns
// token. A stale callback must never tear down another request's fence.
func abortSessionFreezeLease(ctx context.Context, db *sql.DB, sessionID, token string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockSessionLifecycle(ctx, tx, sessionID, false); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE sessions SET canvas_freeze_token = NULL, canvas_freeze_until = NULL
		WHERE id = $1 AND status = 'live' AND canvas_freeze_token = $2::uuid`, sessionID, token)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func completeSessionDegraded(ctx context.Context, db *sql.DB, sessionID, token string) error {
	_, err := completeSessionDegradedResult(ctx, db, sessionID, token)
	return err
}

func completeSessionDegradedResult(ctx context.Context, db *sql.DB, sessionID, token string) (sessionEndResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return sessionEndResult{}, err
	}
	defer tx.Rollback()
	if err := lockSessionLifecycle(ctx, tx, sessionID, false); err != nil {
		return sessionEndResult{}, err
	}
	var existing sql.NullString
	var active bool
	var status string
	var archiveComplete sql.NullBool
	var existingEndedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT status, ended_at, whiteboard_server_archive_complete, canvas_freeze_token,
		COALESCE(canvas_freeze_until > clock_timestamp(), false) FROM sessions WHERE id = $1`, sessionID).
		Scan(&status, &existingEndedAt, &archiveComplete, &existing, &active)
	if err == sql.ErrNoRows {
		return sessionEndResult{}, ErrSessionEndInProgress
	}
	if err != nil {
		return sessionEndResult{}, err
	}
	if status == "ended" {
		if existing.Valid && ((token != "" && existing.String == token) || !active) {
			if token != "" && existing.String == token {
				_, err = tx.ExecContext(ctx, `UPDATE sessions SET canvas_freeze_token = NULL, canvas_freeze_until = NULL
					WHERE id = $1 AND status = 'ended' AND canvas_freeze_token = $2::uuid`, sessionID, token)
			} else {
				_, err = tx.ExecContext(ctx, `UPDATE sessions SET canvas_freeze_token = NULL, canvas_freeze_until = NULL
					WHERE id = $1 AND status = 'ended' AND canvas_freeze_until <= clock_timestamp()`, sessionID)
			}
			if err != nil {
				return sessionEndResult{}, err
			}
		}
		if err := tx.Commit(); err != nil {
			return sessionEndResult{}, err
		}
		var archive *bool
		if archiveComplete.Valid {
			archive = new(bool)
			*archive = archiveComplete.Bool
		}
		var endedAt *time.Time
		if existingEndedAt.Valid {
			endedAt = &existingEndedAt.Time
		}
		return sessionEndResult{WhiteboardServerArchiveComplete: archive, EndedAt: endedAt}, nil
	}
	if active && (!existing.Valid || existing.String != token) {
		return sessionEndResult{}, ErrSessionEndInProgress
	}
	var endedAt time.Time
	err = tx.QueryRowContext(ctx, `UPDATE sessions SET status = 'ended', ended_at = clock_timestamp(),
		whiteboard_server_archive_complete = false, canvas_freeze_token = NULL, canvas_freeze_until = NULL
		WHERE id = $1 AND status = 'live' RETURNING ended_at`, sessionID).Scan(&endedAt)
	if err == sql.ErrNoRows {
		return sessionEndResult{}, ErrSessionEndInProgress
	}
	if err != nil {
		return sessionEndResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return sessionEndResult{}, err
	}
	archive := false
	return sessionEndResult{WhiteboardServerArchiveComplete: &archive, EndedAt: &endedAt}, nil
}

// replaceClassLiveSessions obeys the global class-guard then lifecycle-lock order.
func replaceClassLiveSessions(ctx context.Context, tx *sql.Tx, classID string) ([]ReplacedSession, error) {
	if err := lockClassReplacement(ctx, tx, classID); err != nil {
		return nil, err
	}
	return replaceLockedClassLiveSessions(ctx, tx, classID)
}

// replaceLockedClassLiveSessions discovers sessions only after its caller has
// acquired the class guard and revalidated any producer-specific row state.
func replaceLockedClassLiveSessions(ctx context.Context, tx *sql.Tx, classID string) ([]ReplacedSession, error) {
	return replaceLockedClassLiveSessionsWithHook(ctx, tx, classID, nil, nil)
}

func replaceLockedClassLiveSessionsWithHook(ctx context.Context, tx *sql.Tx, classID string, afterLocks func(), lockedIDs func([]string)) ([]ReplacedSession, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM sessions WHERE class_id = $1 AND status = 'live'
		ORDER BY (('x' || substr(replace(lower(id::text), '-', ''), 1, 8))::bit(32))::int4, id`, classID)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ids, err = sortedSessionLifecycleIDs(ids)
	if err != nil {
		return nil, err
	}
	replaced := make([]ReplacedSession, 0, len(ids))
	for _, id := range ids {
		if err := lockSessionLifecycle(ctx, tx, id, false); err != nil {
			return nil, err
		}
	}
	if lockedIDs != nil {
		lockedIDs(ids)
	}
	if afterLocks != nil {
		afterLocks()
	}
	for _, id := range ids {
		var token sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT canvas_freeze_token FROM sessions WHERE id = $1 AND status = 'live'`, id).Scan(&token)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE sessions SET status = 'ended', ended_at = clock_timestamp(),
			whiteboard_server_archive_complete = false, canvas_freeze_token = NULL, canvas_freeze_until = NULL
			WHERE id = $1 AND status = 'live'`, id)
		if err != nil {
			return nil, err
		}
		var cleared *string
		if token.Valid {
			cleared = &token.String
		}
		replaced = append(replaced, ReplacedSession{ID: id, WhiteboardServerArchiveComplete: false, ClearedFreezeToken: cleared})
	}
	return replaced, nil
}
