package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const MaxSessionCanvases = 50

var (
	ErrCanvasCapReached        = errors.New("session canvas cap reached")
	ErrCanvasVisibilityTighten = errors.New("canvas visibility may only be loosened")
	ErrCanvasBelowFloor        = errors.New("canvas visibility is below the session floor")
	ErrCanvasFloorTooLoose     = errors.New("session canvas floor may not be session")
	ErrCanvasFloorUnauthorized = errors.New("only the session host may set the canvas floor")
)

// Canvas is a persisted whiteboard owned by one user within a session.
type Canvas struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"sessionId"`
	OwnerID    string    `json:"ownerId"`
	Title      string    `json:"title"`
	Visibility string    `json:"visibility"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type CreateCanvasInput struct {
	SessionID  string `json:"sessionId"`
	OwnerID    string `json:"ownerId"`
	Title      string `json:"title"`
	Visibility string `json:"visibility"`
}

type CanvasStore struct {
	db        *sql.DB
	sessions  *SessionStore
	testHooks *canvasStoreTestHooks
}

type canvasStoreOperation string

const (
	canvasStoreOperationCreate        canvasStoreOperation = "create"
	canvasStoreOperationSetVisibility canvasStoreOperation = "set_visibility"
	canvasStoreOperationSetFloor      canvasStoreOperation = "set_floor"
)

// canvasStoreTestHooks is nil in production. It lets the store tests hold an
// operation immediately after its session-row lock has supplied current state.
type canvasStoreTestHooks struct {
	beforeSessionLock func(canvasStoreOperation, int)
	afterSessionLock  func(canvasStoreOperation, int)
}

func NewCanvasStore(db *sql.DB) *CanvasStore {
	return &CanvasStore{db: db, sessions: NewSessionStore(db)}
}

func (s *CanvasStore) beforeSessionLock(ctx context.Context, tx *sql.Tx, operation canvasStoreOperation) (int, error) {
	if s.testHooks == nil {
		return 0, nil
	}
	var backendPID int
	if err := tx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&backendPID); err != nil {
		return 0, err
	}
	if s.testHooks.beforeSessionLock != nil {
		s.testHooks.beforeSessionLock(operation, backendPID)
	}
	return backendPID, nil
}

func (s *CanvasStore) afterSessionLock(operation canvasStoreOperation, backendPID int) {
	if s.testHooks != nil && s.testHooks.afterSessionLock != nil {
		s.testHooks.afterSessionLock(operation, backendPID)
	}
}

const canvasColumns = `id, session_id, owner_id, title, visibility, created_at, updated_at`

func scanCanvas(row interface{ Scan(...any) error }) (*Canvas, error) {
	var canvas Canvas
	if err := row.Scan(&canvas.ID, &canvas.SessionID, &canvas.OwnerID, &canvas.Title,
		&canvas.Visibility, &canvas.CreatedAt, &canvas.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &canvas, nil
}

func validCanvasVisibility(visibility string) bool {
	switch visibility {
	case "private", "host", "participants", "session":
		return true
	default:
		return false
	}
}

// CreateCanvas serializes against floor changes with the owning session row.
func (s *CanvasStore) CreateCanvas(ctx context.Context, input CreateCanvasInput) (*Canvas, error) {
	if strings.TrimSpace(input.Title) == "" {
		return nil, errors.New("canvas title is required")
	}
	if !validCanvasVisibility(input.Visibility) {
		return nil, fmt.Errorf("unsupported canvas visibility %q", input.Visibility)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	backendPID, err := s.beforeSessionLock(ctx, tx, canvasStoreOperationCreate)
	if err != nil {
		return nil, err
	}

	var floor string
	if err := tx.QueryRowContext(ctx,
		`SELECT canvas_floor FROM sessions WHERE id = $1 FOR UPDATE`, input.SessionID,
	).Scan(&floor); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	s.afterSessionLock(canvasStoreOperationCreate, backendPID)

	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM session_canvases WHERE session_id = $1`, input.SessionID,
	).Scan(&count); err != nil {
		return nil, err
	}
	if count >= MaxSessionCanvases {
		return nil, ErrCanvasCapReached
	}

	canvas, err := scanCanvas(tx.QueryRowContext(ctx,
		`INSERT INTO session_canvases (id, session_id, owner_id, title, visibility, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, GREATEST($5::canvas_visibility, $6::canvas_visibility), now(), now())
		 RETURNING `+canvasColumns,
		uuid.New().String(), input.SessionID, input.OwnerID, strings.TrimSpace(input.Title), input.Visibility, floor,
	))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return canvas, nil
}

// GetCanvas fetches a canvas only when it belongs to the requested session.
func (s *CanvasStore) GetCanvas(ctx context.Context, sessionID, canvasID string) (*Canvas, error) {
	return scanCanvas(s.db.QueryRowContext(ctx,
		`SELECT `+canvasColumns+` FROM session_canvases WHERE id = $1 AND session_id = $2`,
		canvasID, sessionID,
	))
}

// SetCanvasVisibility may only loosen a canvas and never cross below its
// session floor. The session-row lock serializes this check with floor raises.
func (s *CanvasStore) SetCanvasVisibility(ctx context.Context, sessionID, canvasID, ownerID, visibility string) (*Canvas, error) {
	if !validCanvasVisibility(visibility) {
		return nil, fmt.Errorf("unsupported canvas visibility %q", visibility)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	backendPID, err := s.beforeSessionLock(ctx, tx, canvasStoreOperationSetVisibility)
	if err != nil {
		return nil, err
	}

	var floor string
	if err := tx.QueryRowContext(ctx,
		`SELECT canvas_floor FROM sessions WHERE id = $1 FOR UPDATE`, sessionID,
	).Scan(&floor); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	s.afterSessionLock(canvasStoreOperationSetVisibility, backendPID)

	var current string
	if err := tx.QueryRowContext(ctx,
		`SELECT visibility FROM session_canvases WHERE id = $1 AND session_id = $2 AND owner_id = $3`,
		canvasID, sessionID, ownerID,
	).Scan(&current); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var tightens, belowFloor bool
	if err := tx.QueryRowContext(ctx,
		`SELECT $1::canvas_visibility <= $2::canvas_visibility,
		        $1::canvas_visibility < $3::canvas_visibility`,
		visibility, current, floor,
	).Scan(&tightens, &belowFloor); err != nil {
		return nil, err
	}
	if tightens {
		return nil, ErrCanvasVisibilityTighten
	}
	if belowFloor {
		return nil, ErrCanvasBelowFloor
	}

	canvas, err := scanCanvas(tx.QueryRowContext(ctx,
		`UPDATE session_canvases SET visibility = $1, updated_at = now()
		 WHERE id = $2 AND session_id = $3 AND owner_id = $4
		 RETURNING `+canvasColumns,
		visibility, canvasID, sessionID, ownerID,
	))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return canvas, nil
}

// SetSessionCanvasFloor changes the host-controlled minimum visibility.
// Raising the floor widens every lower canvas in the same transaction;
// lowering it deliberately leaves existing canvases unchanged.
func (s *CanvasStore) SetSessionCanvasFloor(ctx context.Context, sessionID, hostID, floor string) (string, error) {
	if floor == "session" {
		return "", ErrCanvasFloorTooLoose
	}
	if floor != "private" && floor != "host" && floor != "participants" {
		return "", fmt.Errorf("unsupported canvas floor %q", floor)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	backendPID, err := s.beforeSessionLock(ctx, tx, canvasStoreOperationSetFloor)
	if err != nil {
		return "", err
	}

	var teacherID, current string
	if err := tx.QueryRowContext(ctx,
		`SELECT teacher_id, canvas_floor FROM sessions WHERE id = $1 FOR UPDATE`, sessionID,
	).Scan(&teacherID, &current); err == sql.ErrNoRows {
		return "", nil
	} else if err != nil {
		return "", err
	}
	s.afterSessionLock(canvasStoreOperationSetFloor, backendPID)
	if teacherID != hostID {
		return "", ErrCanvasFloorUnauthorized
	}

	var raises bool
	if err := tx.QueryRowContext(ctx,
		`SELECT $1::canvas_visibility > $2::canvas_visibility`, floor, current,
	).Scan(&raises); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE sessions SET canvas_floor = $1, updated_at = now() WHERE id = $2`, floor, sessionID,
	); err != nil {
		return "", err
	}
	if raises {
		if _, err := tx.ExecContext(ctx,
			`UPDATE session_canvases SET visibility = $1, updated_at = now()
			 WHERE session_id = $2 AND visibility < $1::canvas_visibility`, floor, sessionID,
		); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return floor, nil
}

// ListVisibleCanvases applies the session access rules before returning canvas
// metadata. Live session visibility delegates session-level access to
// SessionStore; ended sessions deliberately use archive-specific rules.
func (s *CanvasStore) ListVisibleCanvases(ctx context.Context, sessionID, userID string) ([]Canvas, error) {
	session, err := s.sessions.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return []Canvas{}, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+canvasColumns+` FROM session_canvases WHERE session_id = $1 ORDER BY created_at, id`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	all := []Canvas{}
	for rows.Next() {
		canvas, err := scanCanvas(rows)
		if err != nil {
			return nil, err
		}
		all = append(all, *canvas)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	participant, err := s.sessions.GetSessionParticipant(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	present := participant != nil && participant.Status == "present"
	formerParticipant := participant != nil && (participant.Status == "present" || participant.Status == "left")

	visible := []Canvas{}
	if session.Status == "ended" {
		for _, canvas := range all {
			if canvas.OwnerID == userID ||
				(session.TeacherID == userID && canvas.Visibility != "private") ||
				(formerParticipant && (canvas.Visibility == "participants" || canvas.Visibility == "session")) {
				visible = append(visible, canvas)
			}
		}
		return visible, nil
	}

	sessionAllowed, _, err := s.sessions.CanAccessSession(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	for _, canvas := range all {
		if canvas.OwnerID == userID ||
			(session.TeacherID == userID && canvas.Visibility != "private") ||
			(present && (canvas.Visibility == "participants" || canvas.Visibility == "session")) ||
			(sessionAllowed && canvas.Visibility == "session") {
			visible = append(visible, canvas)
		}
	}
	return visible, nil
}

// DeleteCanvas deletes the canvas row containing its persisted Yjs state.
func (s *CanvasStore) DeleteCanvas(ctx context.Context, sessionID, canvasID, ownerID string) (bool, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM session_canvases WHERE id = $1 AND session_id = $2 AND owner_id = $3`,
		canvasID, sessionID, ownerID,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}
