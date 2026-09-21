package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	MaxSessionCanvases  = 50
	MaxCanvasTitleRunes = 255
)

var (
	ErrCanvasCapReached          = errors.New("session canvas cap reached")
	ErrCanvasTitleRequired       = errors.New("canvas title is required")
	ErrCanvasTitleTooLong        = errors.New("canvas title exceeds 255 characters")
	ErrCanvasVisibilityTighten   = errors.New("canvas visibility may only be loosened")
	ErrCanvasBelowFloor          = errors.New("canvas visibility is below the session floor")
	ErrCanvasFloorTooLoose       = errors.New("session canvas floor may not be session")
	ErrCanvasFloorUnauthorized   = errors.New("only the session host may set the canvas floor")
	ErrCanvasCreatorUnauthorized = errors.New("only the session teacher or present participant may create a canvas")
	ErrCanvasOwnerUnauthorized   = errors.New("only the canvas owner may mutate a canvas")
	ErrCanvasSessionMismatch     = errors.New("canvas does not belong to supplied session")
	ErrCanvasUserNotFound        = errors.New("canvas authorization user not found")
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

// CanvasSettings is deliberately smaller than LiveSession: this is the
// teacher-only whiteboard control surface, not a generic session settings API.
type CanvasSettings struct {
	CanvasFloor                     string
	WhiteboardServerArchiveComplete *bool
}

// CanvasDocumentAccess is a single shared-lifecycle-lock current-state
// decision. Keeping every canvas/session/participant/class query on this
// transaction prevents an end lease from interleaving halfway through auth.
type CanvasDocumentAccess struct {
	Canvas                   Canvas
	SessionStatus, TeacherID string
	ParticipantStatus        *string
	SessionAccess            bool
}

func (s *CanvasStore) AuthorizeCanvasDocument(ctx context.Context, canvasID, sessionID, userID string) (*CanvasDocumentAccess, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var result CanvasDocumentAccess
	if err := lockSessionLifecycle(ctx, tx, sessionID, true); err != nil {
		return nil, err
	}
	var userExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, userID).Scan(&userExists); err != nil {
		return nil, err
	}
	if !userExists {
		return nil, ErrCanvasUserNotFound
	}
	var classID *string
	var visibility string
	var freezing bool
	err = tx.QueryRowContext(ctx, `SELECT c.id,c.session_id,c.owner_id,c.title,c.visibility,c.created_at,c.updated_at,se.status,se.teacher_id,se.class_id,se.visibility,COALESCE(se.canvas_freeze_until>clock_timestamp(),false)
		FROM session_canvases c JOIN sessions se ON se.id=c.session_id WHERE c.id=$1 AND c.session_id=$2`, canvasID, sessionID).Scan(&result.Canvas.ID, &result.Canvas.SessionID, &result.Canvas.OwnerID, &result.Canvas.Title, &result.Canvas.Visibility, &result.Canvas.CreatedAt, &result.Canvas.UpdatedAt, &result.SessionStatus, &result.TeacherID, &classID, &visibility, &freezing)
	if err == sql.ErrNoRows {
		var canvasExists bool
		if existsErr := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM session_canvases WHERE id=$1)`, canvasID).Scan(&canvasExists); existsErr != nil {
			return nil, existsErr
		}
		if canvasExists {
			return nil, ErrCanvasSessionMismatch
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if freezing {
		return nil, ErrSessionEndInProgress
	}
	var participant sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status FROM session_participants WHERE session_id=$1 AND user_id=$2`, result.Canvas.SessionID, userID).Scan(&participant); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if participant.Valid {
		result.ParticipantStatus = &participant.String
	}
	state := &sessionAccessState{ID: sessionID, Status: result.SessionStatus, TeacherID: result.TeacherID, ClassID: classID, Visibility: visibility}
	result.SessionAccess, _, err = evaluateSessionAccess(ctx, tx, state, userID)
	if err != nil {
		return nil, err
	}
	if result.SessionStatus == "ended" {
		// Archive eligibility separately preserves the teacher's host-level
		// access after end; CanAccessSession intentionally denies ended sessions.
		result.SessionAccess = result.TeacherID == userID
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &result, nil
}

type CreateCanvasInput struct {
	SessionID  string `json:"sessionId"`
	OwnerID    string `json:"ownerId"`
	Title      string `json:"title"`
	Visibility string `json:"visibility"`
}

type CanvasStore struct {
	db        *sql.DB
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
	return &CanvasStore{db: db}
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
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, ErrCanvasTitleRequired
	}
	if utf8.RuneCountInString(title) > MaxCanvasTitleRunes {
		return nil, ErrCanvasTitleTooLong
	}
	if !validCanvasVisibility(input.Visibility) {
		return nil, fmt.Errorf("unsupported canvas visibility %q", input.Visibility)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := lockSessionLifecycle(ctx, tx, input.SessionID, true); err != nil {
		return nil, err
	}
	backendPID, err := s.beforeSessionLock(ctx, tx, canvasStoreOperationCreate)
	if err != nil {
		return nil, err
	}

	var floor, status, teacherID string
	var freezing bool
	if err := tx.QueryRowContext(ctx,
		`SELECT canvas_floor, status, teacher_id, COALESCE(canvas_freeze_until > clock_timestamp(), false) FROM sessions WHERE id = $1 FOR UPDATE`, input.SessionID,
	).Scan(&floor, &status, &teacherID, &freezing); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	s.afterSessionLock(canvasStoreOperationCreate, backendPID)
	if teacherID != input.OwnerID {
		var present bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM session_participants WHERE session_id=$1 AND user_id=$2 AND status='present')`, input.SessionID, input.OwnerID).Scan(&present); err != nil {
			return nil, err
		}
		if !present {
			return nil, ErrCanvasCreatorUnauthorized
		}
	}
	if status == "ended" {
		return nil, ErrSessionEnded
	}
	if freezing {
		return nil, ErrSessionEndInProgress
	}

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
		uuid.New().String(), input.SessionID, input.OwnerID, title, input.Visibility, floor,
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

// GetCanvasByID resolves a realtime document name to its canvas. Callers must
// still authorize against the returned session and owner.
func (s *CanvasStore) GetCanvasByID(ctx context.Context, canvasID string) (*Canvas, error) {
	return scanCanvas(s.db.QueryRowContext(ctx,
		`SELECT `+canvasColumns+` FROM session_canvases WHERE id = $1`, canvasID,
	))
}

func (s *CanvasStore) GetCanvasSettings(ctx context.Context, sessionID string) (*CanvasSettings, error) {
	var settings CanvasSettings
	var complete sql.NullBool
	err := s.db.QueryRowContext(ctx, `SELECT canvas_floor, whiteboard_server_archive_complete
		FROM sessions WHERE id = $1`, sessionID).Scan(&settings.CanvasFloor, &complete)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if complete.Valid {
		settings.WhiteboardServerArchiveComplete = new(bool)
		*settings.WhiteboardServerArchiveComplete = complete.Bool
	}
	return &settings, nil
}

// SetCanvasVisibility may only loosen a canvas and never cross below its
// session floor. The session-row lock serializes this check with floor raises.
func (s *CanvasStore) SetCanvasVisibility(ctx context.Context, sessionID, canvasID, ownerID, visibility string) (*Canvas, error) {
	return s.UpdateCanvas(ctx, sessionID, canvasID, ownerID, nil, &visibility)
}

// UpdateCanvas changes an owner canvas's title and/or visibility under the
// session-row lock. This makes the ended-session check and floor comparison
// atomic with the write, closing the handler-to-store TOCTOU window.
func (s *CanvasStore) UpdateCanvas(ctx context.Context, sessionID, canvasID, ownerID string, title *string, visibility *string) (*Canvas, error) {
	if title == nil && visibility == nil {
		return nil, errors.New("canvas update is required")
	}
	var normalizedTitle string
	if title != nil {
		normalizedTitle = strings.TrimSpace(*title)
		if normalizedTitle == "" {
			return nil, ErrCanvasTitleRequired
		}
		if utf8.RuneCountInString(normalizedTitle) > MaxCanvasTitleRunes {
			return nil, ErrCanvasTitleTooLong
		}
	}
	if visibility != nil && !validCanvasVisibility(*visibility) {
		return nil, fmt.Errorf("unsupported canvas visibility %q", *visibility)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := lockSessionLifecycle(ctx, tx, sessionID, true); err != nil {
		return nil, err
	}
	backendPID, err := s.beforeSessionLock(ctx, tx, canvasStoreOperationSetVisibility)
	if err != nil {
		return nil, err
	}

	var floor, status string
	var freezing bool
	if err := tx.QueryRowContext(ctx,
		`SELECT canvas_floor, status, COALESCE(canvas_freeze_until > clock_timestamp(), false) FROM sessions WHERE id = $1 FOR UPDATE`, sessionID,
	).Scan(&floor, &status, &freezing); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	s.afterSessionLock(canvasStoreOperationSetVisibility, backendPID)
	var canvasOwner, current string
	if err := tx.QueryRowContext(ctx,
		`SELECT owner_id, visibility FROM session_canvases WHERE id = $1 AND session_id = $2`,
		canvasID, sessionID,
	).Scan(&canvasOwner, &current); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if canvasOwner != ownerID {
		return nil, ErrCanvasOwnerUnauthorized
	}
	if status == "ended" {
		return nil, ErrSessionEnded
	}
	if freezing {
		return nil, ErrSessionEndInProgress
	}

	if visibility != nil {
		var tightens, belowFloor bool
		if err := tx.QueryRowContext(ctx,
			`SELECT $1::canvas_visibility <= $2::canvas_visibility,
			        $1::canvas_visibility < $3::canvas_visibility`,
			*visibility, current, floor,
		).Scan(&tightens, &belowFloor); err != nil {
			return nil, err
		}
		if tightens {
			return nil, ErrCanvasVisibilityTighten
		}
		if belowFloor {
			return nil, ErrCanvasBelowFloor
		}
	}

	var canvas *Canvas
	if title != nil && visibility != nil {
		canvas, err = scanCanvas(tx.QueryRowContext(ctx, `UPDATE session_canvases SET title = $1, visibility = $2, updated_at = now() WHERE id = $3 AND session_id = $4 AND owner_id = $5 RETURNING `+canvasColumns, normalizedTitle, *visibility, canvasID, sessionID, ownerID))
	} else if title != nil {
		canvas, err = scanCanvas(tx.QueryRowContext(ctx, `UPDATE session_canvases SET title = $1, updated_at = now() WHERE id = $2 AND session_id = $3 AND owner_id = $4 RETURNING `+canvasColumns, normalizedTitle, canvasID, sessionID, ownerID))
	} else {
		canvas, err = scanCanvas(tx.QueryRowContext(ctx, `UPDATE session_canvases SET visibility = $1, updated_at = now() WHERE id = $2 AND session_id = $3 AND owner_id = $4 RETURNING `+canvasColumns, *visibility, canvasID, sessionID, ownerID))
	}
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
	if err := lockSessionLifecycle(ctx, tx, sessionID, true); err != nil {
		return "", err
	}
	backendPID, err := s.beforeSessionLock(ctx, tx, canvasStoreOperationSetFloor)
	if err != nil {
		return "", err
	}

	var teacherID, current, status string
	var freezing bool
	if err := tx.QueryRowContext(ctx,
		`SELECT teacher_id, canvas_floor, status, COALESCE(canvas_freeze_until > clock_timestamp(), false) FROM sessions WHERE id = $1 FOR UPDATE`, sessionID,
	).Scan(&teacherID, &current, &status, &freezing); err == sql.ErrNoRows {
		return "", nil
	} else if err != nil {
		return "", err
	}
	s.afterSessionLock(canvasStoreOperationSetFloor, backendPID)
	if status == "ended" {
		return "", ErrSessionEnded
	}
	if freezing {
		return "", ErrSessionEndInProgress
	}
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
// metadata. Live session visibility uses the shared session-access core;
// ended sessions deliberately use archive-specific rules.
func (s *CanvasStore) ListVisibleCanvases(ctx context.Context, sessionID, userID string) ([]Canvas, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	state, err := loadSessionAccessState(ctx, tx, sessionID)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return []Canvas{}, nil
	}

	rows, err := tx.QueryContext(ctx,
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

	var participant sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status FROM session_participants WHERE session_id = $1 AND user_id = $2`, sessionID, userID).Scan(&participant); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	present := participant.Valid && participant.String == "present"
	formerParticipant := participant.Valid && (participant.String == "present" || participant.String == "left")

	visible := []Canvas{}
	if state.Status == "ended" {
		for _, canvas := range all {
			if canvas.OwnerID == userID ||
				(state.TeacherID == userID && canvas.Visibility != "private") ||
				(formerParticipant && (canvas.Visibility == "participants" || canvas.Visibility == "session")) {
				visible = append(visible, canvas)
			}
		}
		return visible, nil
	}

	sessionAllowed, _, err := evaluateSessionAccess(ctx, tx, state, userID)
	if err != nil {
		return nil, err
	}
	for _, canvas := range all {
		if canvas.OwnerID == userID ||
			(state.TeacherID == userID && canvas.Visibility != "private") ||
			(present && (canvas.Visibility == "participants" || canvas.Visibility == "session")) ||
			(sessionAllowed && canvas.Visibility == "session") {
			visible = append(visible, canvas)
		}
	}
	return visible, nil
}

// DeleteCanvas deletes the canvas row containing its persisted Yjs state.
func (s *CanvasStore) DeleteCanvas(ctx context.Context, sessionID, canvasID, ownerID string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if err := lockSessionLifecycle(ctx, tx, sessionID, true); err != nil {
		return false, err
	}
	var status string
	var freezing bool
	if err := tx.QueryRowContext(ctx, `SELECT status, COALESCE(canvas_freeze_until > clock_timestamp(), false) FROM sessions WHERE id = $1 FOR UPDATE`, sessionID).Scan(&status, &freezing); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var canvasOwner string
	if err := tx.QueryRowContext(ctx,
		`SELECT owner_id FROM session_canvases WHERE id = $1 AND session_id = $2`, canvasID, sessionID,
	).Scan(&canvasOwner); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if canvasOwner != ownerID {
		return false, ErrCanvasOwnerUnauthorized
	}
	if status == "ended" {
		return false, ErrSessionEnded
	}
	if freezing {
		return false, ErrSessionEndInProgress
	}
	result, err := tx.ExecContext(ctx,
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
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return rows == 1, nil
}
