# 024 — Problems / TestCases / Attempts: Schema + API

**Goal:** Land the data foundation from spec 006 — three tables, store layer, Go HTTP API, integration tests. No UI in this plan.

**Architecture:** New `problems`, `test_cases`, `attempts` tables with the access policy from spec 006 enforced in handlers (mirroring the course-access pattern from Plan 021 Batch 2b). All endpoints under the existing `RequireAuth` group. No changes to Yjs / Hocuspocus / Pyodide.

**Tech Stack:** PostgreSQL 15+ migration · Go (Chi router, `database/sql` via pgx) · existing `auth` middleware · `testify` for tests.

**Branch:** `feat/024-problems-foundation`

**Prereqs:** Spec 006 (`docs/specs/006-problems-and-attempts.md`) and the `UserHasAccessToCourse` helper added in Plan 021c.

---

## File Structure

| File | Responsibility |
|---|---|
| `drizzle/0008_problems.sql` | Migration: 3 tables + indexes |
| `platform/internal/store/problems.go` | `ProblemStore`: CRUD + access helper |
| `platform/internal/store/test_cases.go` | `TestCaseStore`: CRUD with canonical/private split |
| `platform/internal/store/attempts.go` | `AttemptStore`: CRUD + list-by-user-and-problem |
| `platform/internal/store/problems_test.go` | Store integration tests (real DB) |
| `platform/internal/store/test_cases_test.go` | ditto |
| `platform/internal/store/attempts_test.go` | ditto |
| `platform/internal/handlers/problems.go` | HTTP handlers + access middleware |
| `platform/internal/handlers/problems_test.go` | Handler unit tests (auth guards, validation) |
| `platform/cmd/api/main.go` | Wire `ProblemHandler`, `TestCaseHandler`, `AttemptHandler` into the router |

`test_cases` and `attempts` are simple enough that the handlers for them can live in `platform/internal/handlers/problems.go` alongside `ProblemHandler` (one handler struct per resource, single file). Same convention as `assignments.go`.

---

### Task 1: Schema migration

**Files:**
- Create: `drizzle/0008_problems.sql`

- [ ] **Step 1: Write the migration**

```sql
-- Migration: Problems, TestCases, Attempts (spec 006)

CREATE TABLE "problems" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "topic_id" uuid NOT NULL REFERENCES "topics"("id") ON DELETE CASCADE,
  "title" varchar(255) NOT NULL,
  "description" text NOT NULL DEFAULT '',
  "starter_code" text,
  "language" varchar(32) NOT NULL,
  "order" integer NOT NULL DEFAULT 0,
  "created_by" uuid NOT NULL REFERENCES "user"("id"),
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX "problems_topic_order_idx" ON "problems"("topic_id", "order");

CREATE TABLE "test_cases" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "problem_id" uuid NOT NULL REFERENCES "problems"("id") ON DELETE CASCADE,
  "owner_id" uuid REFERENCES "user"("id") ON DELETE CASCADE,
  "name" varchar(120) NOT NULL DEFAULT '',
  "stdin" text NOT NULL DEFAULT '',
  "expected_stdout" text,
  "is_example" boolean NOT NULL DEFAULT false,
  "order" integer NOT NULL DEFAULT 0,
  "created_at" timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX "test_cases_problem_owner_idx" ON "test_cases"("problem_id", "owner_id");

CREATE TABLE "attempts" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "problem_id" uuid NOT NULL REFERENCES "problems"("id") ON DELETE CASCADE,
  "user_id" uuid NOT NULL REFERENCES "user"("id") ON DELETE CASCADE,
  "title" varchar(120) NOT NULL DEFAULT 'Untitled',
  "language" varchar(32) NOT NULL,
  "plain_text" text NOT NULL DEFAULT '',
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX "attempts_problem_user_updated_idx"
  ON "attempts"("problem_id", "user_id", "updated_at" DESC);
```

- [ ] **Step 2: Apply to dev DB**

```bash
psql "$DATABASE_URL" -f drizzle/0008_problems.sql
```

Expected: three `CREATE TABLE` and three `CREATE INDEX` lines, no errors.

- [ ] **Step 3: Apply to test DB**

```bash
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test \
  psql "$DATABASE_URL" -f drizzle/0008_problems.sql
```

- [ ] **Step 4: Verify tables present**

```bash
psql "$DATABASE_URL" -c "\d problems" -c "\d test_cases" -c "\d attempts"
```

Expected: all three tables listed with the columns above and the three indexes.

- [ ] **Step 5: Commit**

```bash
git add drizzle/0008_problems.sql
git commit -m "feat(024): schema for problems, test_cases, attempts"
```

---

### Task 2: ProblemStore

**Files:**
- Create: `platform/internal/store/problems.go`
- Create: `platform/internal/store/problems_test.go`

- [ ] **Step 1: Write the failing test for CreateAndGet**

```go
package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProblemStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	problems := NewProblemStore(db)
	courses := NewCourseStore(db)
	topics := NewTopicStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "P-Course", GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	topic, err := topics.CreateTopic(ctx, CreateTopicInput{
		CourseID: course.ID, Title: "Arrays",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic.ID) })

	problem, err := problems.CreateProblem(ctx, CreateProblemInput{
		TopicID: topic.ID, CreatedBy: user.ID,
		Title: "Two Sum", Description: "Find two numbers that sum to target.",
		Language: "python", StarterCode: "def solve(): pass",
	})
	require.NoError(t, err)
	require.NotNil(t, problem)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", problem.ID) })

	assert.Equal(t, "Two Sum", problem.Title)
	assert.Equal(t, "python", problem.Language)
	assert.Equal(t, 0, problem.Order)

	got, err := problems.GetProblem(ctx, problem.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, problem.ID, got.ID)
}
```

- [ ] **Step 2: Run to confirm it fails**

```bash
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test \
  go test ./internal/store/ -run TestProblemStore_CreateAndGet -count=1
```

Expected: FAIL with `undefined: NewProblemStore` (or similar).

- [ ] **Step 3: Implement ProblemStore (minimal — just CreateAndGet)**

```go
package store

import (
	"context"
	"database/sql"
	"time"
)

type Problem struct {
	ID          string    `json:"id"`
	TopicID     string    `json:"topicId"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StarterCode *string   `json:"starterCode"`
	Language    string    `json:"language"`
	Order       int       `json:"order"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateProblemInput struct {
	TopicID     string
	CreatedBy   string
	Title       string
	Description string
	StarterCode string  // empty string → NULL
	Language    string
	Order       int
}

type ProblemStore struct {
	db *sql.DB
}

func NewProblemStore(db *sql.DB) *ProblemStore { return &ProblemStore{db: db} }

const problemColumns = `id, topic_id, title, description, starter_code, language, "order", created_by, created_at, updated_at`

func scanProblem(row interface {
	Scan(dest ...any) error
}) (*Problem, error) {
	var p Problem
	var starter sql.NullString
	err := row.Scan(&p.ID, &p.TopicID, &p.Title, &p.Description, &starter,
		&p.Language, &p.Order, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if starter.Valid {
		p.StarterCode = &starter.String
	}
	return &p, nil
}

func (s *ProblemStore) CreateProblem(ctx context.Context, in CreateProblemInput) (*Problem, error) {
	var starter any
	if in.StarterCode != "" {
		starter = in.StarterCode
	}
	return scanProblem(s.db.QueryRowContext(ctx,
		`INSERT INTO problems (topic_id, title, description, starter_code, language, "order", created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+problemColumns,
		in.TopicID, in.Title, in.Description, starter, in.Language, in.Order, in.CreatedBy))
}

func (s *ProblemStore) GetProblem(ctx context.Context, id string) (*Problem, error) {
	return scanProblem(s.db.QueryRowContext(ctx,
		`SELECT `+problemColumns+` FROM problems WHERE id = $1`, id))
}
```

- [ ] **Step 4: Run test, confirm pass**

```bash
DATABASE_URL=... go test ./internal/store/ -run TestProblemStore_CreateAndGet -count=1
```

Expected: PASS.

- [ ] **Step 5: Add ListByTopic + UpdateProblem + DeleteProblem with their tests**

Test code (add to `problems_test.go`):

```go
func TestProblemStore_ListByTopic_OrderedByOrderField(t *testing.T) {
	db := testDB(t)
	problems := NewProblemStore(db)
	// ...standard setup, create topic and 3 problems with order = 2, 0, 1...
	list, err := problems.ListProblemsByTopic(ctx, topic.ID)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, 0, list[0].Order)
	assert.Equal(t, 1, list[1].Order)
	assert.Equal(t, 2, list[2].Order)
}

func TestProblemStore_UpdateProblem_PartialFields(t *testing.T) {
	// Create problem, call UpdateProblem with title only, assert other fields unchanged.
}

func TestProblemStore_DeleteProblem(t *testing.T) {
	// Create, delete, assert GetProblem returns nil.
}
```

Implementation (append to `problems.go`):

```go
func (s *ProblemStore) ListProblemsByTopic(ctx context.Context, topicID string) ([]Problem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+problemColumns+` FROM problems
		 WHERE topic_id = $1 ORDER BY "order" ASC, created_at ASC`, topicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Problem
	for rows.Next() {
		p, err := scanProblem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if out == nil {
		out = []Problem{}
	}
	return out, rows.Err()
}

type UpdateProblemInput struct {
	Title       *string
	Description *string
	StarterCode *string  // pointer to ""  means clear (set NULL)
	Language    *string
	Order       *int
}

func (s *ProblemStore) UpdateProblem(ctx context.Context, id string, in UpdateProblemInput) (*Problem, error) {
	// Use the same coalescing pattern as UpdateCourse: build SET clause dynamically.
	// (Reference: platform/internal/store/courses.go UpdateCourse for the pattern.)
	sets := []string{`updated_at = now()`}
	args := []any{}
	pos := 1
	add := func(col string, v any) {
		sets = append(sets, col+" = $"+itoa(pos))
		args = append(args, v)
		pos++
	}
	if in.Title != nil { add("title", *in.Title) }
	if in.Description != nil { add("description", *in.Description) }
	if in.StarterCode != nil {
		if *in.StarterCode == "" { add("starter_code", nil) } else { add("starter_code", *in.StarterCode) }
	}
	if in.Language != nil { add("language", *in.Language) }
	if in.Order != nil { add(`"order"`, *in.Order) }

	args = append(args, id)
	query := `UPDATE problems SET ` + strings.Join(sets, ", ") + ` WHERE id = $` + itoa(pos) + ` RETURNING ` + problemColumns
	return scanProblem(s.db.QueryRowContext(ctx, query, args...))
}

func (s *ProblemStore) DeleteProblem(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM problems WHERE id = $1`, id)
	return err
}
```

(`itoa` and `strings` are imported as needed; the existing `courses.go` uses the same idiom — copy from there if a helper already exists.)

- [ ] **Step 6: Run all problem store tests**

```bash
DATABASE_URL=... go test ./internal/store/ -run TestProblemStore -v -count=1
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add platform/internal/store/problems.go platform/internal/store/problems_test.go
git commit -m "feat(024): ProblemStore — CRUD + list-by-topic"
```

---

### Task 3: TestCaseStore

**Files:**
- Create: `platform/internal/store/test_cases.go`
- Create: `platform/internal/store/test_cases_test.go`

- [ ] **Step 1: Write the failing test for `ListForViewer` (the access-aware list)**

The trickiest method here. A canonical case (owner_id IS NULL) is visible to everyone with course access; a private case is visible only to its owner. The student-facing list = `(canonical cases) ∪ (the caller's own cases)`. Hidden canonical cases are returned too — the handler decides what to expose.

```go
func TestTestCaseStore_ListForViewer_MixesCanonicalAndOwnerPrivate(t *testing.T) {
	db := testDB(t)
	tc := NewTestCaseStore(db)
	// ...setup: problem `p`, users `alice` and `bob`...
	canonicalExample := mustCreate(tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, OwnerID: nil, Stdin: "1 2", ExpectedStdout: ptr("3"),
		IsExample: true,
	}))
	canonicalHidden := mustCreate(tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, OwnerID: nil, Stdin: "x y", ExpectedStdout: ptr("z"),
		IsExample: false,
	}))
	alicePriv := mustCreate(tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, OwnerID: &alice.ID, Stdin: "alice-only",
	}))
	_ = mustCreate(tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, OwnerID: &bob.ID, Stdin: "bob-only",
	}))

	got, err := tc.ListForViewer(ctx, p.ID, alice.ID)
	require.NoError(t, err)
	ids := idSet(got)
	assert.True(t, ids.Has(canonicalExample.ID), "canonical example visible")
	assert.True(t, ids.Has(canonicalHidden.ID), "canonical hidden returned to caller")
	assert.True(t, ids.Has(alicePriv.ID), "alice's private case visible to alice")
	assert.False(t, ids.HasOther(bob.ID), "bob's private case NOT visible to alice")
	assert.Equal(t, 3, len(got))
}
```

(`mustCreate`, `ptr`, `idSet` are tiny test helpers. Inline them in the test file or in `helpers_test.go` if one exists.)

- [ ] **Step 2: Run, confirm fail**

```bash
DATABASE_URL=... go test ./internal/store/ -run TestTestCaseStore_ListForViewer -count=1
```

Expected: FAIL with undefined symbols.

- [ ] **Step 3: Implement TestCaseStore**

```go
package store

import (
	"context"
	"database/sql"
	"time"
)

type TestCase struct {
	ID             string    `json:"id"`
	ProblemID      string    `json:"problemId"`
	OwnerID        *string   `json:"ownerId"`        // nil = canonical
	Name           string    `json:"name"`
	Stdin          string    `json:"stdin"`
	ExpectedStdout *string   `json:"expectedStdout"`
	IsExample      bool      `json:"isExample"`
	Order          int       `json:"order"`
	CreatedAt      time.Time `json:"createdAt"`
}

type CreateTestCaseInput struct {
	ProblemID      string
	OwnerID        *string  // nil = canonical
	Name           string
	Stdin          string
	ExpectedStdout *string
	IsExample      bool
	Order          int
}

type TestCaseStore struct{ db *sql.DB }

func NewTestCaseStore(db *sql.DB) *TestCaseStore { return &TestCaseStore{db: db} }

const testCaseColumns = `id, problem_id, owner_id, name, stdin, expected_stdout, is_example, "order", created_at`

func scanTestCase(row interface{ Scan(dest ...any) error }) (*TestCase, error) {
	var c TestCase
	var owner, expected sql.NullString
	err := row.Scan(&c.ID, &c.ProblemID, &owner, &c.Name, &c.Stdin, &expected, &c.IsExample, &c.Order, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if owner.Valid { c.OwnerID = &owner.String }
	if expected.Valid { c.ExpectedStdout = &expected.String }
	return &c, nil
}

func (s *TestCaseStore) CreateTestCase(ctx context.Context, in CreateTestCaseInput) (*TestCase, error) {
	return scanTestCase(s.db.QueryRowContext(ctx,
		`INSERT INTO test_cases (problem_id, owner_id, name, stdin, expected_stdout, is_example, "order")
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+testCaseColumns,
		in.ProblemID, in.OwnerID, in.Name, in.Stdin, in.ExpectedStdout, in.IsExample, in.Order))
}

func (s *TestCaseStore) GetTestCase(ctx context.Context, id string) (*TestCase, error) {
	return scanTestCase(s.db.QueryRowContext(ctx,
		`SELECT `+testCaseColumns+` FROM test_cases WHERE id = $1`, id))
}

// ListForViewer returns canonical cases + the viewer's own private cases.
// Used by the student-facing problem page (handler decides which fields to redact).
func (s *TestCaseStore) ListForViewer(ctx context.Context, problemID, viewerID string) ([]TestCase, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+testCaseColumns+` FROM test_cases
		 WHERE problem_id = $1 AND (owner_id IS NULL OR owner_id = $2)
		 ORDER BY (owner_id IS NULL) DESC, "order" ASC, created_at ASC`,
		problemID, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TestCase
	for rows.Next() {
		c, err := scanTestCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	if out == nil { out = []TestCase{} }
	return out, rows.Err()
}

// ListCanonical returns all canonical cases for a problem (used by Test endpoint).
func (s *TestCaseStore) ListCanonical(ctx context.Context, problemID string) ([]TestCase, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+testCaseColumns+` FROM test_cases
		 WHERE problem_id = $1 AND owner_id IS NULL
		 ORDER BY "order" ASC, created_at ASC`, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TestCase
	for rows.Next() {
		c, err := scanTestCase(rows)
		if err != nil { return nil, err }
		out = append(out, *c)
	}
	if out == nil { out = []TestCase{} }
	return out, rows.Err()
}

type UpdateTestCaseInput struct {
	Name           *string
	Stdin          *string
	ExpectedStdout *string  // pointer to "" clears (NULL); nil leaves unchanged
	IsExample      *bool
	Order          *int
}

func (s *TestCaseStore) UpdateTestCase(ctx context.Context, id string, in UpdateTestCaseInput) (*TestCase, error) {
	// Same coalescing pattern as UpdateProblem.
	// ...
}

func (s *TestCaseStore) DeleteTestCase(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM test_cases WHERE id = $1`, id)
	return err
}
```

- [ ] **Step 4: Run, confirm `TestTestCaseStore_ListForViewer` passes**

```bash
DATABASE_URL=... go test ./internal/store/ -run TestTestCaseStore -v -count=1
```

Expected: PASS.

- [ ] **Step 5: Add tests for Create/Get/Update/Delete and `ListCanonical`**

```go
func TestTestCaseStore_ListCanonical_OnlyCanonical(t *testing.T) { ... }
func TestTestCaseStore_UpdateTestCase_ClearExpected(t *testing.T) { ... }
func TestTestCaseStore_DeleteTestCase(t *testing.T) { ... }
```

- [ ] **Step 6: Run all test_case store tests, confirm pass**

```bash
DATABASE_URL=... go test ./internal/store/ -run TestTestCaseStore -v -count=1
```

- [ ] **Step 7: Commit**

```bash
git add platform/internal/store/test_cases.go platform/internal/store/test_cases_test.go
git commit -m "feat(024): TestCaseStore — canonical + private split"
```

---

### Task 4: AttemptStore

**Files:**
- Create: `platform/internal/store/attempts.go`
- Create: `platform/internal/store/attempts_test.go`

- [ ] **Step 1: Write the failing test for `ListByUserAndProblem` (sorted by updated_at DESC)**

```go
func TestAttemptStore_ListByUserAndProblem_OrderedByUpdatedDesc(t *testing.T) {
	db := testDB(t)
	attempts := NewAttemptStore(db)
	// setup: problem `p`, user `alice`. Create 3 attempts, then UPDATE the middle one.
	a1 := mustCreate(attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: alice.ID, Language: "python", PlainText: "v1",
	}))
	a2 := mustCreate(attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: alice.ID, Language: "python", PlainText: "v2",
	}))
	a3 := mustCreate(attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: alice.ID, Language: "python", PlainText: "v3",
	}))
	time.Sleep(10 * time.Millisecond)
	_, err := attempts.UpdateAttempt(ctx, a2.ID, UpdateAttemptInput{
		PlainText: ptr("v2 edited"),
	})
	require.NoError(t, err)

	list, err := attempts.ListByUserAndProblem(ctx, p.ID, alice.ID)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, a2.ID, list[0].ID, "edited attempt is most recent")
	assert.Equal(t, a3.ID, list[1].ID)
	assert.Equal(t, a1.ID, list[2].ID)
}
```

- [ ] **Step 2: Run, confirm fail**

- [ ] **Step 3: Implement AttemptStore**

```go
package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type Attempt struct {
	ID         string    `json:"id"`
	ProblemID  string    `json:"problemId"`
	UserID     string    `json:"userId"`
	Title      string    `json:"title"`
	Language   string    `json:"language"`
	PlainText  string    `json:"plainText"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type CreateAttemptInput struct {
	ProblemID string
	UserID    string
	Title     string  // empty → defaults to "Untitled"
	Language  string
	PlainText string
}

type UpdateAttemptInput struct {
	Title     *string
	PlainText *string
}

type AttemptStore struct{ db *sql.DB }

func NewAttemptStore(db *sql.DB) *AttemptStore { return &AttemptStore{db: db} }

const attemptColumns = `id, problem_id, user_id, title, language, plain_text, created_at, updated_at`

func scanAttempt(row interface{ Scan(dest ...any) error }) (*Attempt, error) {
	var a Attempt
	err := row.Scan(&a.ID, &a.ProblemID, &a.UserID, &a.Title, &a.Language, &a.PlainText, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return nil, err }
	return &a, nil
}

func (s *AttemptStore) CreateAttempt(ctx context.Context, in CreateAttemptInput) (*Attempt, error) {
	title := in.Title
	if title == "" { title = "Untitled" }
	return scanAttempt(s.db.QueryRowContext(ctx,
		`INSERT INTO attempts (problem_id, user_id, title, language, plain_text)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+attemptColumns,
		in.ProblemID, in.UserID, title, in.Language, in.PlainText))
}

func (s *AttemptStore) GetAttempt(ctx context.Context, id string) (*Attempt, error) {
	return scanAttempt(s.db.QueryRowContext(ctx,
		`SELECT `+attemptColumns+` FROM attempts WHERE id = $1`, id))
}

func (s *AttemptStore) ListByUserAndProblem(ctx context.Context, problemID, userID string) ([]Attempt, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+attemptColumns+` FROM attempts
		 WHERE problem_id = $1 AND user_id = $2
		 ORDER BY updated_at DESC`, problemID, userID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []Attempt
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil { return nil, err }
		out = append(out, *a)
	}
	if out == nil { out = []Attempt{} }
	return out, rows.Err()
}

func (s *AttemptStore) UpdateAttempt(ctx context.Context, id string, in UpdateAttemptInput) (*Attempt, error) {
	sets := []string{`updated_at = now()`}
	args := []any{}
	pos := 1
	add := func(col string, v any) {
		sets = append(sets, col+" = $"+itoa(pos))
		args = append(args, v)
		pos++
	}
	if in.Title != nil { add("title", *in.Title) }
	if in.PlainText != nil { add("plain_text", *in.PlainText) }

	args = append(args, id)
	query := `UPDATE attempts SET ` + strings.Join(sets, ", ") + ` WHERE id = $` + itoa(pos) + ` RETURNING ` + attemptColumns
	return scanAttempt(s.db.QueryRowContext(ctx, query, args...))
}

func (s *AttemptStore) DeleteAttempt(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM attempts WHERE id = $1`, id)
	return err
}
```

- [ ] **Step 4: Run, confirm pass**

- [ ] **Step 5: Add tests for Create/Get/Update/Delete + cross-user isolation**

```go
func TestAttemptStore_ListByUserAndProblem_FiltersByUser(t *testing.T) {
	// alice + bob each have an attempt; alice's list should not include bob's.
}
func TestAttemptStore_UpdateAttempt_TouchesUpdatedAt(t *testing.T) { ... }
func TestAttemptStore_DeleteAttempt(t *testing.T) { ... }
```

- [ ] **Step 6: Run all attempt store tests, confirm pass**

```bash
DATABASE_URL=... go test ./internal/store/ -run TestAttemptStore -v -count=1
```

- [ ] **Step 7: Commit**

```bash
git add platform/internal/store/attempts.go platform/internal/store/attempts_test.go
git commit -m "feat(024): AttemptStore — append-new with updated_at sort"
```

---

### Task 5: HTTP handlers — Problem CRUD

**Files:**
- Create: `platform/internal/handlers/problems.go`
- Create: `platform/internal/handlers/problems_test.go`

This handler file holds three resource handlers (`ProblemHandler`, `TestCaseHandler`, `AttemptHandler`) sharing the same access-control helper.

- [ ] **Step 1: Write a no-claims test for `ListProblems`**

```go
package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListProblems_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/topics/00000000-0000-0000-0000-000000000000/problems", nil)
	req = withChiParams(req, map[string]string{"topicId": "00000000-0000-0000-0000-000000000000"})
	w := httptest.NewRecorder()
	h.ListProblems(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/handlers/ -run TestListProblems_NoClaims -count=1
```

Expected: FAIL with undefined `ProblemHandler`.

- [ ] **Step 3: Implement ProblemHandler skeleton + ListProblems**

```go
package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type ProblemHandler struct {
	Problems  *store.ProblemStore
	TestCases *store.TestCaseStore
	Attempts  *store.AttemptStore
	Topics    *store.TopicStore
	Courses   *store.CourseStore
}

func (h *ProblemHandler) Routes(r chi.Router) {
	r.Route("/api/topics/{topicId}/problems", func(r chi.Router) {
		r.Use(ValidateUUIDParam("topicId"))
		r.Get("/", h.ListProblems)
		r.Post("/", h.CreateProblem)
	})
	r.Route("/api/problems/{id}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Get("/", h.GetProblem)
		r.Patch("/", h.UpdateProblem)
		r.Delete("/", h.DeleteProblem)

		r.Get("/test-cases", h.ListTestCases)
		r.Post("/test-cases", h.CreateTestCase)
		r.Get("/attempts", h.ListAttempts)
		r.Post("/attempts", h.CreateAttempt)
	})
	r.Route("/api/test-cases/{id}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Patch("/", h.UpdateTestCase)
		r.Delete("/", h.DeleteTestCase)
	})
	r.Route("/api/attempts/{id}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Get("/", h.GetAttempt)
		r.Patch("/", h.UpdateAttempt)
		r.Delete("/", h.DeleteAttempt)
	})
}

// userCanViewProblem mirrors the GetCourse access policy: creator,
// platform admin, or any user with an active class membership in a
// class belonging to the problem's course (via topic).
func (h *ProblemHandler) userCanViewProblem(r *http.Request, p *store.Problem, claims *auth.Claims) (bool, error) {
	if claims.IsPlatformAdmin || p.CreatedBy == claims.UserID {
		return true, nil
	}
	topic, err := h.Topics.GetTopic(r.Context(), p.TopicID)
	if err != nil { return false, err }
	if topic == nil { return false, nil }
	return h.Courses.UserHasAccessToCourse(r.Context(), topic.CourseID, claims.UserID)
}

func (h *ProblemHandler) ListProblems(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	topicID := chi.URLParam(r, "topicId")

	if !claims.IsPlatformAdmin {
		topic, err := h.Topics.GetTopic(r.Context(), topicID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if topic == nil {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		hasAccess, err := h.Courses.UserHasAccessToCourse(r.Context(), topic.CourseID, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if !hasAccess {
			// Topics belong to courses the caller doesn't see → 403.
			// (Note: course-creator path also gets here — handle by checking creator on the topic's course.)
			course, err := h.Courses.GetCourse(r.Context(), topic.CourseID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Database error")
				return
			}
			if course == nil || course.CreatedBy != claims.UserID {
				writeError(w, http.StatusForbidden, "Access denied")
				return
			}
		}
	}

	list, err := h.Problems.ListProblemsByTopic(r.Context(), topicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, list)
}
```

- [ ] **Step 4: Run, confirm `TestListProblems_NoClaims` passes**

```bash
go test ./internal/handlers/ -run TestListProblems_NoClaims -count=1
```

Expected: PASS.

- [ ] **Step 5: Implement Create/Get/Update/Delete for Problem with tests**

```go
// CreateProblem: only creator-of-the-topic's-course or platform admin.
// (Topic doesn't have a created_by; reuse the course creator check.)
//
// GetProblem: viewer access via userCanViewProblem.
// UpdateProblem: only the problem's creator or platform admin.
// DeleteProblem: only the problem's creator or platform admin.
```

Tests to add in `problems_test.go`:

```go
func TestGetProblem_NoClaims(t *testing.T) { ... }
func TestCreateProblem_NoClaims(t *testing.T) { ... }
func TestUpdateProblem_NoClaims(t *testing.T) { ... }
func TestDeleteProblem_NoClaims(t *testing.T) { ... }
```

These follow the same pattern as `TestUpdateCourse_NoClaims` etc. — minimal handler test that exercises the auth guard.

- [ ] **Step 6: Run all problem handler tests**

```bash
go test ./internal/handlers/ -run TestProblem -v -count=1
```

- [ ] **Step 7: Commit**

```bash
git add platform/internal/handlers/problems.go platform/internal/handlers/problems_test.go
git commit -m "feat(024): ProblemHandler — CRUD + access checks"
```

---

### Task 6: HTTP handlers — TestCase CRUD

Same file (`platform/internal/handlers/problems.go`); methods on `ProblemHandler`.

- [ ] **Step 1: Add `ListTestCases` test (no-claims)**

```go
func TestListTestCases_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/problems/abc/test-cases", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.ListTestCases(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
```

- [ ] **Step 2: Implement ListTestCases — student view (canonical-examples + caller's private)**

```go
// ListTestCases returns:
//  - if caller can view the problem AND is the creator/admin → all canonical (example + hidden) + caller's private
//  - if caller can view the problem otherwise → only canonical-where-is_example=true + caller's private
//
// Hidden canonical cases must NEVER appear in a non-creator response.
func (h *ProblemHandler) ListTestCases(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	problemID := chi.URLParam(r, "id")
	problem, err := h.Problems.GetProblem(r.Context(), problemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if problem == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	canView, err := h.userCanViewProblem(r, problem, claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !canView {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	all, err := h.TestCases.ListForViewer(r.Context(), problemID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	isAuthor := claims.IsPlatformAdmin || problem.CreatedBy == claims.UserID
	if isAuthor {
		writeJSON(w, http.StatusOK, all)
		return
	}

	// Non-author: redact hidden canonical cases.
	out := make([]store.TestCase, 0, len(all))
	for _, c := range all {
		if c.OwnerID == nil && !c.IsExample {
			continue // hidden canonical: redact
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}
```

- [ ] **Step 3: Add an integration-style test that verifies redaction (uses real DB)**

This goes in a new file `platform/internal/handlers/problems_redact_test.go` so it can use the store fixtures:

```go
// +build integration  ← (no, just gate on DATABASE_URL like other store tests do)

func TestListTestCases_RedactsHiddenForNonAuthor(t *testing.T) {
	db := testDB(t)
	// Compose handler from real stores; create a problem owned by `teacher`
	// with one canonical example, one canonical hidden, and one private of `student`.
	// Call ListTestCases with `student` claims; assert hidden canonical absent.
	// Call again with `teacher` claims; assert hidden canonical present.
}
```

- [ ] **Step 4: Implement Create/Update/Delete TestCase handlers**

Access rules:
- **Create canonical** (`owner_id` omitted in body) — must be problem creator or admin.
- **Create private** (`owner_id` not in body, server sets to `claims.UserID`) — any viewer of the problem.
- **Update / Delete** — owner of the case (for private) OR problem creator/admin (for canonical).

The HTTP body never accepts `ownerId` from the client; server decides based on a `kind` field or the route. Simpler: one POST endpoint with an `isPrivate` boolean — defaults true for non-authors, requires explicit false (and authoring rights) to create a canonical.

Concrete shape:

```json
POST /api/problems/{id}/test-cases
{
  "name": "Negatives",
  "stdin": "-1 -2",
  "expectedStdout": "-3",
  "isExample": false,
  "isCanonical": false   // default; only authors may pass true
}
```

Handler logic checks `isCanonical` against author rights. Tests:

```go
func TestCreateTestCase_NoClaims(t *testing.T) { ... }
func TestUpdateTestCase_NoClaims(t *testing.T) { ... }
func TestDeleteTestCase_NoClaims(t *testing.T) { ... }
func TestCreateTestCase_NonAuthorCannotCreateCanonical(t *testing.T) {
	// Integration: setup problem owned by teacher; student calls Create with
	// isCanonical=true → expect 403.
}
```

- [ ] **Step 5: Run all test case handler tests**

```bash
go test ./internal/handlers/ -run TestListTestCases\|TestCreateTestCase\|TestUpdateTestCase\|TestDeleteTestCase -v -count=1
```

- [ ] **Step 6: Commit**

```bash
git add platform/internal/handlers/problems.go platform/internal/handlers/problems_test.go platform/internal/handlers/problems_redact_test.go
git commit -m "feat(024): TestCase handlers — canonical/private split with hidden redaction"
```

---

### Task 7: HTTP handlers — Attempt CRUD

- [ ] **Step 1: Add no-claims tests for all four endpoints**

```go
func TestListAttempts_NoClaims(t *testing.T) { ... }
func TestCreateAttempt_NoClaims(t *testing.T) { ... }
func TestGetAttempt_NoClaims(t *testing.T) { ... }
func TestUpdateAttempt_NoClaims(t *testing.T) { ... }
func TestDeleteAttempt_NoClaims(t *testing.T) { ... }
```

- [ ] **Step 2: Implement the four endpoints**

Access rules:
- **List** — caller sees only their own attempts for the problem.
- **Create** — caller creates an attempt for themselves on a problem they can view; server sets `user_id` from claims.
- **Get** — owner only (404 for non-owner — don't leak existence).
- **Update** — owner only.
- **Delete** — owner only.

Teacher read-access is **NOT** in this plan — that endpoint (`GET /api/teacher/problems/{id}/students/{userId}/attempts`) is reserved for spec 007's plan. Without it, the basic API is closed-system per user; teacher visibility comes later.

Sketch:

```go
func (h *ProblemHandler) CreateAttempt(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	problemID := chi.URLParam(r, "id")

	problem, err := h.Problems.GetProblem(r.Context(), problemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if problem == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	canView, err := h.userCanViewProblem(r, problem, claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !canView {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var body struct {
		Title     string `json:"title"`
		Language  string `json:"language"`
		PlainText string `json:"plainText"`
	}
	if !decodeJSON(w, r, &body) { return }
	if body.Language == "" { body.Language = problem.Language }

	a, err := h.Attempts.CreateAttempt(r.Context(), store.CreateAttemptInput{
		ProblemID: problemID, UserID: claims.UserID,
		Title: body.Title, Language: body.Language, PlainText: body.PlainText,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create attempt")
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// GetAttempt / UpdateAttempt / DeleteAttempt: load attempt, verify
// attempt.UserID == claims.UserID (or platform admin), reject otherwise.
```

- [ ] **Step 3: Add integration tests for cross-user isolation**

```go
func TestGetAttempt_OtherUser_Returns404(t *testing.T) {
	// alice's attempt; bob calls Get → 404 (not 403, to avoid leaking existence).
}
func TestUpdateAttempt_OtherUser_Returns404(t *testing.T) { ... }
```

- [ ] **Step 4: Run all attempt handler tests**

```bash
go test ./internal/handlers/ -run TestAttempt -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add platform/internal/handlers/problems.go platform/internal/handlers/problems_test.go
git commit -m "feat(024): Attempt handlers — owner-only access, cross-user 404"
```

---

### Task 8: Wire into main router

**Files:**
- Modify: `platform/cmd/api/main.go`

- [ ] **Step 1: Construct stores and handler in `main.go` next to existing wiring**

Reference the existing pattern around `topicH := &handlers.TopicHandler{...}` (search for `topicH` in `main.go`).

```go
problemStore := store.NewProblemStore(database)
testCaseStore := store.NewTestCaseStore(database)
attemptStore := store.NewAttemptStore(database)

problemH := &handlers.ProblemHandler{
	Problems:  problemStore,
	TestCases: testCaseStore,
	Attempts:  attemptStore,
	Topics:    topicStore,
	Courses:   courseStore,
}
```

- [ ] **Step 2: Register routes inside the `RequireAuth` group**

```go
problemH.Routes(r)
```

Place it next to `topicH.Routes(r)` to keep curriculum routes grouped.

- [ ] **Step 3: Update `next.config.ts` proxy list**

Add to `GO_PROXY_ROUTES`:

```typescript
"/api/topics/:path*",     // already proxied via courses? — verify; if not, add
"/api/problems/:path*",
"/api/test-cases/:path*",
"/api/attempts/:path*",
```

(Some of these routes already exist in the proxy list. Don't duplicate — just add the missing ones.)

- [ ] **Step 4: Build + restart**

```bash
cd platform && go build ./... && cd ..
# then restart the Go service so the new routes are live
```

- [ ] **Step 5: Smoke-test with curl**

Acquire a session cookie from a logged-in browser tab (or use an existing E2E auth state), then:

```bash
COOKIE='authjs.session-token=...'
curl -sf -H "Cookie: $COOKIE" http://localhost:3003/api/topics/{some-topic-id}/problems | jq .
# Expected: [] (empty list, since no problems created yet)
```

- [ ] **Step 6: Commit**

```bash
git add platform/cmd/api/main.go next.config.ts
git commit -m "feat(024): wire problem/test-case/attempt routes into main + proxy"
```

---

### Task 9: Final verification

- [ ] **Run the full Go suite with DB**

```bash
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test \
  go test ./... -count=1 -timeout 180s
```

Expected: all packages PASS.

- [ ] **Run the existing E2E suite as a regression check** (no new E2E in this plan)

```bash
bun run test:e2e
```

Expected: all 65 tests still pass — none of the new code is wired into the frontend yet.

- [ ] **Self-check: types match spec 006**

Open `docs/specs/006-problems-and-attempts.md` and walk down the schema definitions. Confirm every column listed there appears in `drizzle/0008_problems.sql` with the right type and nullability. Confirm every API endpoint listed in spec 006's API sketch is wired in `Routes()` (except `POST /api/attempts/{id}/test` and `GET /api/teacher/...` which are reserved for plan 025+).

- [ ] **Append post-execution report to this plan file**

Following the format used in plans 022 and 023.

- [ ] **Code review** — dispatch a reviewer agent on the diff range, fix Critical + Important findings, log results in this file's `## Code Review` section.

- [ ] **Push and create PR**

```bash
git push -u origin feat/024-problems-foundation
gh pr create --title "feat: problems / test_cases / attempts foundation (Plan 024)" --body "..."
```

---

## Out of scope for this plan

- The `POST /api/attempts/{id}/test` endpoint (Piston runner) — reserved for plan 026 (spec 008 implementation).
- The teacher `GET /api/teacher/problems/{id}/students/{userId}/attempts` endpoint — reserved for plan 025 (spec 007 UI), since it only exists to feed the teacher watch page.
- `attempts.last_test_result` JSONB column — added in plan 026 alongside the test runner.
- Any UI: student page, teacher page, attempt switcher, terminal redesign.
- Migration of existing `documents` rows into `attempts` — explicitly skipped per spec 006.
