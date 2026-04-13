package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Class struct {
	ID        string    `json:"id"`
	CourseID  string    `json:"courseId"`
	OrgID     string    `json:"orgId"`
	Title     string    `json:"title"`
	Term      string    `json:"term"`
	JoinCode  string    `json:"joinCode"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type NewClassroom struct {
	ID         string    `json:"id"`
	ClassID    string    `json:"classId"`
	EditorMode string    `json:"editorMode"`
	Settings   string    `json:"settings"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ClassMembership struct {
	ID       string    `json:"id"`
	ClassID  string    `json:"classId"`
	UserID   string    `json:"userId"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joinedAt"`
}

type ClassMemberWithUser struct {
	ID       string    `json:"id"`
	ClassID  string    `json:"classId"`
	UserID   string    `json:"userId"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joinedAt"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
}

type CreateClassInput struct {
	CourseID  string `json:"courseId"`
	OrgID     string `json:"orgId"`
	Title     string `json:"title"`
	Term      string `json:"term"`
	CreatedBy string `json:"createdBy"`
}

type ClassStore struct {
	db *sql.DB
}

func NewClassStore(db *sql.DB) *ClassStore {
	return &ClassStore{db: db}
}

const classColumns = `id, course_id, org_id, title, term, join_code, status, created_at, updated_at`

func scanClass(row interface{ Scan(...any) error }) (*Class, error) {
	var c Class
	err := row.Scan(&c.ID, &c.CourseID, &c.OrgID, &c.Title, &c.Term,
		&c.JoinCode, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func generateJoinCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I, O, 0, 1
	b := make([]byte, 8)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return string(b)
}

// CreateClass creates a class, its associated new_classroom, and adds the creator as instructor.
func (s *ClassStore) CreateClass(ctx context.Context, input CreateClassInput) (*Class, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	classID := uuid.New().String()
	now := time.Now()
	joinCode := generateJoinCode()

	var class Class
	err = tx.QueryRowContext(ctx,
		`INSERT INTO classes (id, course_id, org_id, title, term, join_code, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, $8)
		 RETURNING `+classColumns,
		classID, input.CourseID, input.OrgID, input.Title, input.Term, joinCode, now, now,
	).Scan(&class.ID, &class.CourseID, &class.OrgID, &class.Title, &class.Term,
		&class.JoinCode, &class.Status, &class.CreatedAt, &class.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Determine editor mode from course language
	editorMode := "python"
	var lang string
	err = tx.QueryRowContext(ctx, `SELECT language FROM courses WHERE id = $1`, input.CourseID).Scan(&lang)
	if err == nil && lang != "" {
		editorMode = lang
	}

	// Create new_classroom (1:1)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO new_classrooms (id, class_id, editor_mode, settings, created_at)
		 VALUES ($1, $2, $3, '{}', $4)`,
		uuid.New().String(), classID, editorMode, now,
	)
	if err != nil {
		return nil, err
	}

	// Add creator as instructor
	_, err = tx.ExecContext(ctx,
		`INSERT INTO class_memberships (id, class_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, 'instructor', $4)`,
		uuid.New().String(), classID, input.CreatedBy, now,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &class, nil
}

func (s *ClassStore) GetClass(ctx context.Context, id string) (*Class, error) {
	return scanClass(s.db.QueryRowContext(ctx,
		`SELECT `+classColumns+` FROM classes WHERE id = $1`, id))
}

func (s *ClassStore) ListClassesByOrg(ctx context.Context, orgID string) ([]Class, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+classColumns+` FROM classes WHERE org_id = $1 AND status = 'active' ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var classes []Class
	for rows.Next() {
		var c Class
		if err := rows.Scan(&c.ID, &c.CourseID, &c.OrgID, &c.Title, &c.Term,
			&c.JoinCode, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		classes = append(classes, c)
	}
	if classes == nil {
		classes = []Class{}
	}
	return classes, rows.Err()
}

func (s *ClassStore) ArchiveClass(ctx context.Context, id string) (*Class, error) {
	return scanClass(s.db.QueryRowContext(ctx,
		`UPDATE classes SET status = 'archived', updated_at = $1 WHERE id = $2 RETURNING `+classColumns,
		time.Now(), id))
}

func (s *ClassStore) GetClassByJoinCode(ctx context.Context, joinCode string) (*Class, error) {
	return scanClass(s.db.QueryRowContext(ctx,
		`SELECT `+classColumns+` FROM classes WHERE join_code = $1`, strings.ToUpper(joinCode)))
}

func (s *ClassStore) GetClassroom(ctx context.Context, classID string) (*NewClassroom, error) {
	var nc NewClassroom
	err := s.db.QueryRowContext(ctx,
		`SELECT id, class_id, editor_mode, settings, created_at FROM new_classrooms WHERE class_id = $1`, classID,
	).Scan(&nc.ID, &nc.ClassID, &nc.EditorMode, &nc.Settings, &nc.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &nc, nil
}

// --- Class Memberships ---

type AddClassMemberInput struct {
	ClassID string `json:"classId"`
	UserID  string `json:"userId"`
	Role    string `json:"role"`
}

func (s *ClassStore) AddClassMember(ctx context.Context, input AddClassMemberInput) (*ClassMembership, error) {
	id := uuid.New().String()
	role := input.Role
	if role == "" {
		role = "student"
	}
	var m ClassMembership
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO class_memberships (id, class_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT DO NOTHING
		 RETURNING id, class_id, user_id, role, joined_at`,
		id, input.ClassID, input.UserID, role, time.Now(),
	).Scan(&m.ID, &m.ClassID, &m.UserID, &m.Role, &m.JoinedAt)
	if err == sql.ErrNoRows {
		return nil, nil // already exists
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *ClassStore) ListClassMembers(ctx context.Context, classID string) ([]ClassMemberWithUser, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT cm.id, cm.class_id, cm.user_id, cm.role, cm.joined_at, u.name, u.email
		 FROM class_memberships cm
		 INNER JOIN users u ON cm.user_id = u.id
		 WHERE cm.class_id = $1
		 ORDER BY cm.joined_at`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []ClassMemberWithUser
	for rows.Next() {
		var m ClassMemberWithUser
		if err := rows.Scan(&m.ID, &m.ClassID, &m.UserID, &m.Role, &m.JoinedAt, &m.Name, &m.Email); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	if members == nil {
		members = []ClassMemberWithUser{}
	}
	return members, rows.Err()
}

func (s *ClassStore) GetClassMembership(ctx context.Context, membershipID string) (*ClassMembership, error) {
	var m ClassMembership
	err := s.db.QueryRowContext(ctx,
		`SELECT id, class_id, user_id, role, joined_at FROM class_memberships WHERE id = $1`, membershipID,
	).Scan(&m.ID, &m.ClassID, &m.UserID, &m.Role, &m.JoinedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *ClassStore) UpdateClassMemberRole(ctx context.Context, membershipID, role string) (*ClassMembership, error) {
	var m ClassMembership
	err := s.db.QueryRowContext(ctx,
		`UPDATE class_memberships SET role = $1 WHERE id = $2
		 RETURNING id, class_id, user_id, role, joined_at`,
		role, membershipID,
	).Scan(&m.ID, &m.ClassID, &m.UserID, &m.Role, &m.JoinedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *ClassStore) RemoveClassMember(ctx context.Context, membershipID string) (*ClassMembership, error) {
	var m ClassMembership
	err := s.db.QueryRowContext(ctx,
		`DELETE FROM class_memberships WHERE id = $1
		 RETURNING id, class_id, user_id, role, joined_at`,
		membershipID,
	).Scan(&m.ID, &m.ClassID, &m.UserID, &m.Role, &m.JoinedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// JoinResult is returned by JoinClassByCode.
type JoinResult struct {
	Class      *Class           `json:"class"`
	Membership *ClassMembership `json:"membership"`
}

func (s *ClassStore) JoinClassByCode(ctx context.Context, joinCode, userID string) (*JoinResult, error) {
	class, err := s.GetClassByJoinCode(ctx, joinCode)
	if err != nil {
		return nil, err
	}
	if class == nil || class.Status != "active" {
		return nil, nil
	}

	membership, err := s.AddClassMember(ctx, AddClassMemberInput{
		ClassID: class.ID,
		UserID:  userID,
		Role:    "student",
	})
	if err != nil {
		return nil, err
	}

	return &JoinResult{Class: class, Membership: membership}, nil
}

var validClassMemberRoles = map[string]bool{
	"instructor": true, "ta": true, "student": true,
	"observer": true, "guest": true, "parent": true,
}

// IsValidClassMemberRole checks if the role is valid.
func IsValidClassMemberRole(role string) bool {
	return validClassMemberRoles[role]
}

func init() {
	_ = fmt.Sprintf // avoid unused import
}
