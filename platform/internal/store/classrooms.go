package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// Classroom represents the legacy classrooms table.
type Classroom struct {
	ID          string    `json:"id"`
	TeacherID   string    `json:"teacherId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	GradeLevel  string    `json:"gradeLevel"`
	EditorMode  string    `json:"editorMode"`
	JoinCode    string    `json:"joinCode"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ClassroomMember struct {
	ClassroomID string    `json:"classroomId"`
	UserID      string    `json:"userId"`
	JoinedAt    time.Time `json:"joinedAt"`
}

type ClassroomMemberWithUser struct {
	UserID   string    `json:"userId"`
	JoinedAt time.Time `json:"joinedAt"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
}

type CreateClassroomInput struct {
	TeacherID   string `json:"teacherId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	GradeLevel  string `json:"gradeLevel"`
	EditorMode  string `json:"editorMode"`
}

type ClassroomStore struct {
	db *sql.DB
}

func NewClassroomStore(db *sql.DB) *ClassroomStore {
	return &ClassroomStore{db: db}
}

const classroomColumns = `id, teacher_id, name, description, grade_level, editor_mode, join_code, created_at, updated_at`

func scanClassroom(row interface{ Scan(...any) error }) (*Classroom, error) {
	var c Classroom
	err := row.Scan(&c.ID, &c.TeacherID, &c.Name, &c.Description, &c.GradeLevel,
		&c.EditorMode, &c.JoinCode, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *ClassroomStore) CreateClassroom(ctx context.Context, input CreateClassroomInput) (*Classroom, error) {
	id := uuid.New().String()
	now := time.Now()
	joinCode := generateJoinCode() // reuse from classes.go

	return scanClassroom(s.db.QueryRowContext(ctx,
		`INSERT INTO classrooms (id, teacher_id, name, description, grade_level, editor_mode, join_code, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING `+classroomColumns,
		id, input.TeacherID, input.Name, input.Description, input.GradeLevel, input.EditorMode, joinCode, now, now,
	))
}

func (s *ClassroomStore) GetClassroom(ctx context.Context, id string) (*Classroom, error) {
	return scanClassroom(s.db.QueryRowContext(ctx,
		`SELECT `+classroomColumns+` FROM classrooms WHERE id = $1`, id))
}

func (s *ClassroomStore) GetClassroomByJoinCode(ctx context.Context, joinCode string) (*Classroom, error) {
	return scanClassroom(s.db.QueryRowContext(ctx,
		`SELECT `+classroomColumns+` FROM classrooms WHERE join_code = $1`, joinCode))
}

func (s *ClassroomStore) ListClassrooms(ctx context.Context, userID string) ([]Classroom, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+classroomColumns+` FROM classrooms WHERE teacher_id = $1
		 UNION
		 SELECT `+classroomColumns+` FROM classrooms WHERE id IN (
		   SELECT classroom_id FROM classroom_members WHERE user_id = $1
		 )
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var classrooms []Classroom
	for rows.Next() {
		var c Classroom
		if err := rows.Scan(&c.ID, &c.TeacherID, &c.Name, &c.Description, &c.GradeLevel,
			&c.EditorMode, &c.JoinCode, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		classrooms = append(classrooms, c)
	}
	if classrooms == nil {
		classrooms = []Classroom{}
	}
	return classrooms, rows.Err()
}

func (s *ClassroomStore) JoinClassroom(ctx context.Context, classroomID, userID string) (*ClassroomMember, error) {
	var m ClassroomMember
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO classroom_members (classroom_id, user_id, joined_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING
		 RETURNING classroom_id, user_id, joined_at`,
		classroomID, userID, time.Now(),
	).Scan(&m.ClassroomID, &m.UserID, &m.JoinedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *ClassroomStore) GetClassroomMembers(ctx context.Context, classroomID string) ([]ClassroomMemberWithUser, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT cm.user_id, cm.joined_at, u.name, u.email
		 FROM classroom_members cm
		 INNER JOIN users u ON cm.user_id = u.id
		 WHERE cm.classroom_id = $1
		 ORDER BY cm.joined_at`, classroomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []ClassroomMemberWithUser
	for rows.Next() {
		var m ClassroomMemberWithUser
		if err := rows.Scan(&m.UserID, &m.JoinedAt, &m.Name, &m.Email); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	if members == nil {
		members = []ClassroomMemberWithUser{}
	}
	return members, rows.Err()
}
