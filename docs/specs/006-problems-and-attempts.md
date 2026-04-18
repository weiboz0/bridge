# 006 — Problems, Test Cases, and Attempts

## Problem

Today the coding editor is not connected to curriculum. A student opens a class and gets a blank `documents` row — no associated task, no canonical spec, no test harness. Teachers cannot author "solve this" exercises, and students cannot keep multiple working drafts of the same problem. Grading, where it exists, is tied to class-bound `assignments` with no structured input/output test data.

This spec introduces **Problems** as a reusable curriculum unit under a Topic, **TestCases** (canonical + user-owned), and **Attempts** (a student's iterative programs against a Problem). It is a foundation layer. Subsequent specs cover the editor UI redesign and the stdin execution model.

The platform will later pivot to competitive-programming hosting; the model here is designed LeetCode-style (hidden test cases for grading, example cases for students) so that pivot is a UI change, not a data migration.

## Design

### Data Model

```
problems
├── id (uuid, PK)
├── topic_id (uuid, FK → topics, ON DELETE CASCADE)
├── title (varchar(255), NOT NULL)
├── description (text, NOT NULL, markdown)
├── starter_code (text, NULL)
├── language (varchar, NOT NULL)   — overrides courses.language for this problem
├── order (int, default 0)         — display order within topic
├── created_by (uuid, FK → users)
├── created_at (timestamptz)
├── updated_at (timestamptz)
```

```
test_cases
├── id (uuid, PK)
├── problem_id (uuid, FK → problems, ON DELETE CASCADE)
├── owner_id (uuid, FK → users, NULL)   — NULL = canonical, non-NULL = private to that user
├── name (varchar(120))                  — display label ("Example 1", "Negative input")
├── stdin (text, default '')
├── expected_stdout (text, NULL)         — NULL = no grading, just runs the code
├── is_example (bool, default false)     — student sees canonical cases only when true
├── order (int, default 0)
├── created_at (timestamptz)
```

```
attempts
├── id (uuid, PK)
├── problem_id (uuid, FK → problems, ON DELETE CASCADE)
├── user_id (uuid, FK → users, ON DELETE CASCADE)
├── title (varchar(120), default 'Untitled')
├── language (varchar, NOT NULL)   — usually inherits from problem; may differ for polyglot exercises
├── plain_text (text, default '')  — the source code
├── created_at (timestamptz)
├── updated_at (timestamptz)
```

**Indexes:** `problems(topic_id, order)`, `test_cases(problem_id, owner_id)`, `attempts(problem_id, user_id, updated_at DESC)`.

### Semantics

- **Canonical vs private test cases.** A test case with `owner_id IS NULL` is authored by the problem's creator (teacher/admin). A test case with `owner_id = u` belongs to user `u` and is invisible to everyone else. Students author their own private cases while debugging; canonical cases drive grading.
- **Example visibility.** Students only see canonical cases where `is_example = true`. Hidden cases (`is_example = false`) run server-side during grading but never render in the student UI. Example cases render in the problem description so students can reason about expected behavior.
- **Attempts are append-new.** Editing code produces in-place updates to the current attempt, but "New Attempt" creates a new row — the student never loses prior work. No attempt is ever deleted automatically. Students may rename and may explicitly delete their own attempts.
- **"Active" attempt = most recently updated.** No `is_active` column. Teacher and student UIs query `ORDER BY updated_at DESC LIMIT 1` for the active view, and show the full list for history selection.
- **Language on Problem.** Overrides course language so a topic can host Python and JavaScript variants of the same concept.

### Relationship to existing entities

- **Assignment stays unchanged.** An Assignment remains a class-bound, due-dated task. In a follow-up spec, `assignments.problem_id` will be added so assignments can reference a Problem (not required for this spec's scope).
- **Submission stays unchanged.** Grading artifacts live on `submissions`. A follow-up spec will add `submissions.attempt_id` so a student can submit a specific Attempt to an Assignment. Practice-mode test runs are ephemeral and do not write `submissions` rows.
- **Documents (existing `documents` table).** Not used for Problem-driven editing. `attempts` supersedes it in this flow. Existing Document-based pages (`student/code`, scratch sessions) continue to work.

### Execution flow (preview for later specs)

- **Run** = interactive: executes the current code in the editor's worker (Pyodide / JS runner today) with `input()` prompts. Output streams to the terminal panel. No grading.
- **Test** = batch: runs the current code against every canonical test case (example + hidden) with each case's `stdin` piped, compares `stdout` to `expected_stdout`. Shows pass/fail per example case; hides detail for failed hidden cases (generic "Hidden case 3 failed"). No submission row written.
- **Submit** (when an Assignment is linked) = runs Test, writes a `submissions` row with the attempt_id and pass/fail summary.

### API sketch (for the next spec's plan to detail)

```
POST   /api/topics/{topicId}/problems          teacher: create problem
GET    /api/topics/{topicId}/problems          list problems in topic
GET    /api/problems/{id}                      viewer needs access via topic/course
PATCH  /api/problems/{id}                      teacher edits
DELETE /api/problems/{id}                      teacher deletes

POST   /api/problems/{id}/test-cases           teacher (canonical) OR any authed user (private)
GET    /api/problems/{id}/test-cases           returns canonical-examples + the caller's private cases
PATCH  /api/test-cases/{id}                    owner only
DELETE /api/test-cases/{id}                    owner only

GET    /api/problems/{id}/attempts             list the caller's own attempts
POST   /api/problems/{id}/attempts             create new attempt
PATCH  /api/attempts/{id}                      update code (autosave)
DELETE /api/attempts/{id}                      owner only

GET    /api/teacher/problems/{id}/students/{userId}/attempts
                                               teacher view of student attempts — requires class membership
POST   /api/attempts/{id}/test                 runs canonical cases, returns pass/fail summary (ephemeral)
```

### Access control

- Problem read: creator, platform admin, or user with a class membership whose class's course contains the topic. Mirrors the access policy used for Topics and Courses after Plan 021 Batch 2b.
- Problem write: creator or platform admin.
- Canonical test case write: creator of the problem or platform admin.
- Private test case: only the owner reads/writes.
- Attempt read/write: only the user (owner). Teachers gain read-only access to attempts of students enrolled in any of the teacher's classes whose course contains the topic.

### Non-goals for this spec

- Editor UI changes (spec 007).
- Interactive stdin plumbing (spec 008).
- Autograder safety/timeouts beyond what the current Piston/Pyodide sandbox provides.
- Assignment → Problem linkage and grade flow (follow-up spec once UI is stable).

## Migration

A single migration adds the three tables plus indexes. No data backfill: existing `documents` rows are not migrated into `attempts`. Students who were working in the old scratch editor keep those documents; new Problem-driven work creates attempts from scratch.

## Rollout sketch

- Phase 1 (next plan): schema + Go store + API handlers + unit/integration tests.
- Phase 2: editor UI redesign — three-pane problem view (spec 007 drives this).
- Phase 3: interactive stdin (spec 008).
- Phase 4: link Assignment → Problem; link Submission → Attempt; autograder for Submit.
