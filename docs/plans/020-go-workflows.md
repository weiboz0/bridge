# Plan 020 — Go Workflow Engine & Bridge AI Agents

> **STATUS: DEFERRED** — Workflow capability is not needed for current scope. Will revisit when Bridge requires background automation, scheduled tasks, or multi-step AI agent pipelines.

**Spec:** `docs/specs/004-go-backend-migration.md` — Phase 4
**Source:** `~/workshop/magicburg-go/gobackend/internal/workflows/` — proven DAG engine (~850 lines)
**Branch:** `feat/020-go-workflows`

---

## Overview

Port the workflow engine from magicburg and build four Bridge-specific AI agents. The engine executes DAGs (topological sort, parallel layer execution, approval gates, artifact passing) and a cron scheduler creates runs on a recurring basis. The Bridge agents operate as workflow steps: student tutor, teacher assistant, self-pacer, and content creator.

**What ships:**
1. Workflow engine core (dag, cron, executor, store, prompts, emitter)
2. Bridge-specific workflow store adapted for PostgreSQL
3. `cmd/engine/main.go` standalone engine process
4. Four Bridge AI agents with domain-specific personas and tools
5. Background job patterns (post-session reports, weekly parent reports, content pipeline)
6. Cron scheduling for recurring workflows
7. DB migration for workflow tables

**Depends on (assumed already landed):**
- Go project scaffolding (`go.mod`, Chi router, config, DB pool) — Phase 1
- LLM package (`internal/llm/`) — Phase 1
- Tools package (`internal/tools/`) — Phase 1
- Events package (`internal/events/`) — Phase 1
- Bridge store queries (`internal/store/`) — Phase 2

---

## Task 1 — DB Migration: Workflow Tables

**File:** `gobackend/migrations/007_workflow_tables.up.sql`

```sql
-- Workflow definitions
CREATE TABLE IF NOT EXISTS workflows (
    workflow_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title        VARCHAR(255) NOT NULL,
    description  TEXT DEFAULT '',
    template     VARCHAR(255) DEFAULT '',
    status       VARCHAR(20) NOT NULL DEFAULT 'draft'
                 CHECK (status IN ('draft', 'ready', 'archived')),
    head_version INTEGER,
    user_context TEXT DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workflows_user ON workflows(user_id);
CREATE INDEX idx_workflows_status ON workflows(status);

-- DAG versions (immutable snapshots of workflow graph)
CREATE TABLE IF NOT EXISTS workflow_dags (
    workflow_id    UUID NOT NULL REFERENCES workflows(workflow_id) ON DELETE CASCADE,
    version        INTEGER NOT NULL,
    parent_version INTEGER DEFAULT 0,
    dag            JSONB NOT NULL,
    summary        TEXT DEFAULT '',
    session_id     VARCHAR(255) DEFAULT '',
    tag            VARCHAR(100) DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workflow_id, version)
);

CREATE INDEX idx_workflow_dags_tag ON workflow_dags(workflow_id, tag)
    WHERE tag != '';

-- Workflow runs (execution instances)
CREATE TABLE IF NOT EXISTS workflow_runs (
    run_id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id        VARCHAR(255) DEFAULT '',
    workflow_id        UUID NOT NULL REFERENCES workflows(workflow_id) ON DELETE CASCADE,
    dag_version        INTEGER NOT NULL,
    status             VARCHAR(20) NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'running', 'paused', 'completed', 'failed', 'cancelled')),
    trigger_source     VARCHAR(50) DEFAULT 'api',
    trigger_session_id VARCHAR(255) DEFAULT '',
    slot_time          TIMESTAMPTZ,
    started_at         TIMESTAMPTZ,
    completed_at       TIMESTAMPTZ,
    result_summary     TEXT DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workflow_runs_workflow ON workflow_runs(workflow_id);
CREATE INDEX idx_workflow_runs_status ON workflow_runs(status);
CREATE INDEX idx_workflow_runs_schedule ON workflow_runs(schedule_id)
    WHERE schedule_id != '';

-- Step-level results within a run
CREATE TABLE IF NOT EXISTS workflow_step_results (
    run_id       UUID NOT NULL REFERENCES workflow_runs(run_id) ON DELETE CASCADE,
    step_id      VARCHAR(255) NOT NULL,
    status       VARCHAR(30) NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'running', 'completed', 'failed', 'skipped', 'awaiting_approval')),
    artifact     TEXT DEFAULT '',
    error        TEXT DEFAULT '',
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (run_id, step_id)
);

-- Cron schedules
CREATE TABLE IF NOT EXISTS workflow_schedules (
    schedule_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id  UUID NOT NULL REFERENCES workflows(workflow_id) ON DELETE CASCADE,
    dag_version  INTEGER NOT NULL,
    dag_tag      VARCHAR(100) DEFAULT '',
    status       VARCHAR(20) NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'paused', 'completed')),
    mode         VARCHAR(20) NOT NULL DEFAULT 'scheduled'
                 CHECK (mode IN ('oneshot', 'scheduled')),
    schedule     VARCHAR(255) DEFAULT '',
    start_at     TIMESTAMPTZ,
    end_at       TIMESTAMPTZ,
    catch_up     BOOLEAN NOT NULL DEFAULT FALSE,
    last_run_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workflow_schedules_workflow ON workflow_schedules(workflow_id);
CREATE INDEX idx_workflow_schedules_status ON workflow_schedules(status);

-- Workflow templates (built-in + user-saved)
CREATE TABLE IF NOT EXISTS workflow_templates (
    template_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID REFERENCES users(id) ON DELETE CASCADE,
    name         VARCHAR(255) NOT NULL UNIQUE,
    display_name VARCHAR(255) DEFAULT '',
    description  TEXT DEFAULT '',
    dag          JSONB NOT NULL,
    source       VARCHAR(20) NOT NULL DEFAULT 'builtin'
                 CHECK (source IN ('builtin', 'user')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**File:** `gobackend/migrations/007_workflow_tables.down.sql`

```sql
DROP TABLE IF EXISTS workflow_templates;
DROP TABLE IF EXISTS workflow_schedules;
DROP TABLE IF EXISTS workflow_step_results;
DROP TABLE IF EXISTS workflow_runs;
DROP TABLE IF EXISTS workflow_dags;
DROP TABLE IF EXISTS workflows;
```

**Tests:** Migration tested via `db.Migrate()` in integration tests (Task 8).

---

## Task 2 — Copy & Adapt DAG Utilities (`dag.go`)

Copy from `magicburg-go/gobackend/internal/workflows/dag.go` and change the package import path. The DAG logic is domain-agnostic and copies verbatim.

**File:** `gobackend/internal/workflows/dag.go`

```go
// Package workflows provides DAG graph utilities, cron scheduling,
// workflow execution, and persistence for Bridge.
package workflows

import (
	"fmt"
	"sort"
)

// edge represents a directed dependency between two steps.
type edge struct {
	From string
	To   string
}

// extractStepIDs returns the list of step IDs from a DAG map.
func extractStepIDs(dag map[string]any) []string {
	stepsRaw, _ := dag["steps"].([]any)
	ids := make([]string, 0, len(stepsRaw))
	for _, s := range stepsRaw {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// extractEdges returns the list of edges from a DAG map.
func extractEdges(dag map[string]any) []edge {
	edgesRaw, _ := dag["edges"].([]any)
	edges := make([]edge, 0, len(edgesRaw))
	for _, e := range edgesRaw {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		from, _ := m["from"].(string)
		to, _ := m["to"].(string)
		if from != "" && to != "" {
			edges = append(edges, edge{From: from, To: to})
		}
	}
	return edges
}

// extractStepDeps returns a map of step ID -> sorted list of dependency step IDs.
// An edge {from: X, to: Y} means Y depends on X.
func extractStepDeps(dag map[string]any) map[string][]string {
	stepIDs := extractStepIDs(dag)
	edges := extractEdges(dag)

	deps := make(map[string][]string, len(stepIDs))
	for _, id := range stepIDs {
		deps[id] = []string{}
	}

	for _, e := range edges {
		deps[e.To] = append(deps[e.To], e.From)
	}

	for id := range deps {
		sort.Strings(deps[id])
	}
	return deps
}

// TopologicalSort returns the steps of a DAG grouped into parallel execution layers
// using Kahn's algorithm. Each layer contains steps whose dependencies are all
// satisfied by prior layers. Step IDs within each layer are sorted for determinism.
func TopologicalSort(dag map[string]any) [][]string {
	stepIDs := extractStepIDs(dag)
	if len(stepIDs) == 0 {
		return nil
	}

	inDegree := make(map[string]int, len(stepIDs))
	adj := make(map[string][]string, len(stepIDs))
	for _, id := range stepIDs {
		inDegree[id] = 0
		adj[id] = []string{}
	}

	for _, e := range extractEdges(dag) {
		if _, ok := inDegree[e.From]; !ok {
			continue
		}
		if _, ok := inDegree[e.To]; !ok {
			continue
		}
		inDegree[e.To]++
		adj[e.From] = append(adj[e.From], e.To)
	}

	var ready []string
	for _, id := range stepIDs {
		if inDegree[id] == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	var layers [][]string
	for len(ready) > 0 {
		layer := ready
		ready = nil

		for _, id := range layer {
			for _, next := range adj[id] {
				inDegree[next]--
				if inDegree[next] == 0 {
					ready = append(ready, next)
				}
			}
		}
		sort.Strings(ready)
		layers = append(layers, layer)
	}

	return layers
}

// ValidateDAG checks a DAG for structural correctness and returns a list of
// error messages. An empty slice means the DAG is valid.
//
// Checks performed:
//   - Duplicate step IDs
//   - Edges referencing nonexistent steps
//   - Cycles (detected when topological sort does not include all steps)
func ValidateDAG(dag map[string]any) []string {
	var errs []string

	stepsRaw, _ := dag["steps"].([]any)
	seen := make(map[string]int)
	for _, s := range stepsRaw {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		seen[id]++
	}
	for id, count := range seen {
		if count > 1 {
			errs = append(errs, fmt.Sprintf("duplicate step ID: %q (appears %d times)", id, count))
		}
	}

	stepSet := make(map[string]bool, len(seen))
	for id := range seen {
		if id != "" {
			stepSet[id] = true
		}
	}

	edges := extractEdges(dag)
	for _, e := range edges {
		if !stepSet[e.From] {
			errs = append(errs, fmt.Sprintf("edge references unknown step %q (from)", e.From))
		}
		if !stepSet[e.To] {
			errs = append(errs, fmt.Sprintf("edge references unknown step %q (to)", e.To))
		}
	}

	inDegree := make(map[string]int, len(stepSet))
	adj := make(map[string][]string, len(stepSet))
	for id := range stepSet {
		inDegree[id] = 0
		adj[id] = []string{}
	}
	for _, e := range edges {
		if !stepSet[e.From] || !stepSet[e.To] {
			continue
		}
		inDegree[e.To]++
		adj[e.From] = append(adj[e.From], e.To)
	}

	var queue []string
	for id := range stepSet {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	processed := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		processed++
		for _, next := range adj[cur] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if processed < len(stepSet) {
		var cycleSteps []string
		for id := range stepSet {
			if inDegree[id] > 0 {
				cycleSteps = append(cycleSteps, id)
			}
		}
		sort.Strings(cycleSteps)
		errs = append(errs, fmt.Sprintf("cycle detected among steps: %v", cycleSteps))
	}

	sort.Strings(errs)
	return errs
}
```

**File:** `gobackend/internal/workflows/dag_test.go`

Copy verbatim from `magicburg-go/gobackend/internal/workflows/dag_test.go`. The test file uses only the `workflows` package and `testing` — no external imports to change. All 8 test functions (`TestTopologicalSort_Linear`, `_Parallel`, `_NoEdges`, `_SingleStep`, `_Diamond`, `_WideFanOut`, `TestValidateDAG_Cycle`, `_DuplicateIDs`, `_NonexistentEdgeStep`, `_Valid`, `TestExtractStepDeps`) copy as-is.

---

## Task 3 — Copy & Adapt Cron Utilities (`cron.go`)

Copy verbatim from `magicburg-go/gobackend/internal/workflows/cron.go`. No import changes needed — only depends on `fmt`, `time`, and `github.com/robfig/cron/v3` (already in `go.mod` from Phase 1).

**File:** `gobackend/internal/workflows/cron.go`

```go
package workflows

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

const maxCatchUpTicks = 100

// ParseCron parses a standard 5-field cron expression.
func ParseCron(expr string) (cron.Schedule, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return sched, nil
}

// NextTick returns the next cron tick strictly after the given time.
func NextTick(expr string, after time.Time) (time.Time, error) {
	sched, err := ParseCron(expr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(after), nil
}

// TicksBetween returns all cron ticks in the half-open interval (from, to].
// Returns at most maxCatchUpTicks to prevent runaway backfill.
func TicksBetween(expr string, from, to time.Time) ([]time.Time, error) {
	sched, err := ParseCron(expr)
	if err != nil {
		return nil, err
	}
	var ticks []time.Time
	cursor := from
	for len(ticks) < maxCatchUpTicks {
		next := sched.Next(cursor)
		if next.After(to) {
			break
		}
		ticks = append(ticks, next)
		cursor = next
	}
	return ticks, nil
}
```

**File:** `gobackend/internal/workflows/cron_test.go`

Copy verbatim from `magicburg-go/gobackend/internal/workflows/cron_test.go`. Change import path from `github.com/MagicBurg/magicburg/gobackend` to the Bridge module path. All 7 test functions copy as-is.

---

## Task 4 — Workflow Store (`store.go`) — Adapted for PostgreSQL

The magicburg store uses `?` placeholders (SQLite). Bridge uses PostgreSQL exclusively, so we use `$1, $2, ...` placeholders and `TIMESTAMPTZ` types. We also remove the SQLite dialect handling and the `ON CONFLICT` syntax differences.

**File:** `gobackend/internal/workflows/store.go`

```go
package workflows

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	workflowStatuses = map[string]bool{"draft": true, "ready": true, "archived": true}
)

// WorkflowStore provides access to workflow data in PostgreSQL.
type WorkflowStore struct {
	pool *pgxpool.Pool
}

// NewWorkflowStore creates a new WorkflowStore.
func NewWorkflowStore(pool *pgxpool.Pool) *WorkflowStore {
	return &WorkflowStore{pool: pool}
}

// nowUTC returns the current UTC time as an ISO 8601 string.
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// --- Workflow CRUD ---

// Create creates a new workflow and returns its ID.
func (s *WorkflowStore) Create(ctx context.Context, userID, title, template string) (string, error) {
	workflowID := uuid.New().String()
	now := nowUTC()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO workflows (workflow_id, user_id, title, template, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 'draft', $5, $6)`,
		workflowID, userID, title, template, now, now,
	)
	if err != nil {
		return "", err
	}
	return workflowID, nil
}

// Get returns a workflow by ID, or nil if not found.
func (s *WorkflowStore) Get(ctx context.Context, workflowID string) (map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		"SELECT workflow_id, user_id, title, description, template, status, head_version, created_at, updated_at FROM workflows WHERE workflow_id = $1",
		workflowID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}

	var wf struct {
		WorkflowID  string
		UserID      string
		Title       string
		Description string
		Template    string
		Status      string
		HeadVersion *int
		CreatedAt   time.Time
		UpdatedAt   time.Time
	}
	if err := rows.Scan(&wf.WorkflowID, &wf.UserID, &wf.Title, &wf.Description, &wf.Template, &wf.Status, &wf.HeadVersion, &wf.CreatedAt, &wf.UpdatedAt); err != nil {
		return nil, err
	}

	result := map[string]any{
		"workflow_id": wf.WorkflowID,
		"user_id":     wf.UserID,
		"title":       wf.Title,
		"description": wf.Description,
		"template":    wf.Template,
		"status":      wf.Status,
		"created_at":  wf.CreatedAt.Format(time.RFC3339),
		"updated_at":  wf.UpdatedAt.Format(time.RFC3339),
	}
	if wf.HeadVersion != nil {
		result["head_version"] = *wf.HeadVersion
	}
	return result, nil
}

// Update updates the specified fields on a workflow.
func (s *WorkflowStore) Update(ctx context.Context, workflowID string, updates map[string]any) error {
	allowed := map[string]bool{"title": true, "description": true, "status": true, "user_context": true}
	filtered := make(map[string]any)
	for k, v := range updates {
		if allowed[k] {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if status, ok := filtered["status"]; ok {
		st, _ := status.(string)
		if !workflowStatuses[st] {
			return fmt.Errorf("invalid workflow status: %s", st)
		}
	}
	now := nowUTC()

	setClauses := make([]string, 0, len(filtered)+1)
	values := make([]any, 0, len(filtered)+2)
	i := 1
	for k, v := range filtered {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, i))
		values = append(values, v)
		i++
	}
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", i))
	values = append(values, now)
	i++
	values = append(values, workflowID)

	query := fmt.Sprintf("UPDATE workflows SET %s WHERE workflow_id = $%d", strings.Join(setClauses, ", "), i)
	_, err := s.pool.Exec(ctx, query, values...)
	return err
}

// SetHeadVersion updates the head_version for a workflow.
func (s *WorkflowStore) SetHeadVersion(ctx context.Context, workflowID string, version int) error {
	tag, err := s.pool.Exec(ctx,
		"UPDATE workflows SET head_version = $1, updated_at = $2 WHERE workflow_id = $3",
		version, nowUTC(), workflowID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("workflow %s not found", workflowID)
	}
	return nil
}

// --- DAG Versions ---

// GetDagContent returns the parsed DAG content (the "dag" field) for a version.
func (s *WorkflowStore) GetDagContent(ctx context.Context, workflowID string, version int) (map[string]any, error) {
	var dagJSON []byte
	err := s.pool.QueryRow(ctx,
		"SELECT dag FROM workflow_dags WHERE workflow_id = $1 AND version = $2",
		workflowID, version,
	).Scan(&dagJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	var dag map[string]any
	if err := json.Unmarshal(dagJSON, &dag); err != nil {
		return nil, fmt.Errorf("unmarshal dag: %w", err)
	}
	return dag, nil
}

// SaveDag saves a new DAG version and returns the version number.
func (s *WorkflowStore) SaveDag(ctx context.Context, workflowID string, dag map[string]any, summary, sessionID string, parentVersion int) (int, error) {
	now := nowUTC()
	dagJSON, err := json.Marshal(dag)
	if err != nil {
		return 0, fmt.Errorf("marshal dag: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var maxV int
	err = tx.QueryRow(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM workflow_dags WHERE workflow_id = $1",
		workflowID,
	).Scan(&maxV)
	if err != nil {
		return 0, err
	}
	version := maxV + 1

	if parentVersion == 0 && version > 1 {
		var headVersion *int
		err = tx.QueryRow(ctx, "SELECT head_version FROM workflows WHERE workflow_id = $1", workflowID).Scan(&headVersion)
		if err == nil && headVersion != nil {
			parentVersion = *headVersion
		}
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO workflow_dags (workflow_id, version, parent_version, dag, summary, session_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		workflowID, version, parentVersion, dagJSON, summary, sessionID, now,
	)
	if err != nil {
		return 0, err
	}

	_, err = tx.Exec(ctx,
		`UPDATE workflows SET head_version = $1,
		 status = CASE WHEN status = 'draft' THEN 'ready' ELSE status END,
		 updated_at = $2 WHERE workflow_id = $3`,
		version, now, workflowID,
	)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return version, nil
}

// --- Runs ---

// RunOption configures optional run parameters.
type RunOption func(*runConfig)

type runConfig struct {
	scheduleID       string
	triggerSource    string
	triggerSessionID string
	slotTime         string
}

// WithScheduleID sets the schedule ID on a run.
func WithScheduleID(id string) RunOption {
	return func(c *runConfig) { c.scheduleID = id }
}

// WithTriggerSource sets the trigger source on a run.
func WithTriggerSource(src string) RunOption {
	return func(c *runConfig) { c.triggerSource = src }
}

// WithTriggerSessionID sets the trigger session ID on a run.
func WithTriggerSessionID(id string) RunOption {
	return func(c *runConfig) { c.triggerSessionID = id }
}

// WithSlotTime sets the slot time on a run.
func WithSlotTime(t string) RunOption {
	return func(c *runConfig) { c.slotTime = t }
}

// CreateRun creates a new pending run.
func (s *WorkflowStore) CreateRun(ctx context.Context, workflowID string, dagVersion int, opts ...RunOption) (string, error) {
	cfg := &runConfig{triggerSource: "api"}
	for _, o := range opts {
		o(cfg)
	}

	runID := uuid.New().String()
	now := nowUTC()

	var slotTime *string
	if cfg.slotTime != "" {
		slotTime = &cfg.slotTime
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO workflow_runs (run_id, schedule_id, workflow_id, dag_version, status,
		 trigger_source, trigger_session_id, slot_time, created_at)
		 VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7, $8)`,
		runID, cfg.scheduleID, workflowID, dagVersion, cfg.triggerSource,
		cfg.triggerSessionID, slotTime, now,
	)
	if err != nil {
		return "", err
	}
	return runID, nil
}

// GetRun returns a run by ID as a map.
func (s *WorkflowStore) GetRun(ctx context.Context, runID string) (map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT run_id, schedule_id, workflow_id, dag_version, status,
		 trigger_source, trigger_session_id, slot_time, started_at, completed_at,
		 result_summary, created_at
		 FROM workflow_runs WHERE run_id = $1`, runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}

	var r struct {
		RunID            string
		ScheduleID       string
		WorkflowID       string
		DagVersion       int
		Status           string
		TriggerSource    string
		TriggerSessionID string
		SlotTime         *time.Time
		StartedAt        *time.Time
		CompletedAt      *time.Time
		ResultSummary    string
		CreatedAt        time.Time
	}
	if err := rows.Scan(&r.RunID, &r.ScheduleID, &r.WorkflowID, &r.DagVersion, &r.Status,
		&r.TriggerSource, &r.TriggerSessionID, &r.SlotTime, &r.StartedAt, &r.CompletedAt,
		&r.ResultSummary, &r.CreatedAt); err != nil {
		return nil, err
	}

	result := map[string]any{
		"run_id":             r.RunID,
		"schedule_id":        r.ScheduleID,
		"workflow_id":        r.WorkflowID,
		"dag_version":        r.DagVersion,
		"status":             r.Status,
		"trigger_source":     r.TriggerSource,
		"trigger_session_id": r.TriggerSessionID,
		"result_summary":     r.ResultSummary,
		"created_at":         r.CreatedAt.Format(time.RFC3339),
	}
	if r.SlotTime != nil {
		result["slot_time"] = r.SlotTime.Format(time.RFC3339)
	}
	if r.StartedAt != nil {
		result["started_at"] = r.StartedAt.Format(time.RFC3339)
	}
	if r.CompletedAt != nil {
		result["completed_at"] = r.CompletedAt.Format(time.RFC3339)
	}
	return result, nil
}

// UpdateRun updates run fields.
func (s *WorkflowStore) UpdateRun(ctx context.Context, runID string, updates map[string]any) error {
	allowed := map[string]bool{"status": true, "started_at": true, "completed_at": true, "result_summary": true}
	filtered := make(map[string]any)
	for k, v := range updates {
		if allowed[k] {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	setClauses := make([]string, 0, len(filtered))
	values := make([]any, 0, len(filtered)+1)
	i := 1
	for k, v := range filtered {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, i))
		values = append(values, v)
		i++
	}
	values = append(values, runID)
	query := fmt.Sprintf("UPDATE workflow_runs SET %s WHERE run_id = $%d", strings.Join(setClauses, ", "), i)
	_, err := s.pool.Exec(ctx, query, values...)
	return err
}

// ClaimPendingRuns atomically claims up to limit pending runs.
func (s *WorkflowStore) ClaimPendingRuns(ctx context.Context, limit int) ([]map[string]any, error) {
	// Use SELECT ... FOR UPDATE SKIP LOCKED for PostgreSQL-native row locking.
	rows, err := s.pool.Query(ctx,
		`UPDATE workflow_runs
		 SET status = 'running', started_at = NOW()
		 WHERE run_id IN (
		   SELECT run_id FROM workflow_runs
		   WHERE status = 'pending'
		   ORDER BY created_at
		   LIMIT $1
		   FOR UPDATE SKIP LOCKED
		 )
		 RETURNING run_id, schedule_id, workflow_id, dag_version, status,
		   trigger_source, trigger_session_id, slot_time, started_at, created_at`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var r struct {
			RunID            string
			ScheduleID       string
			WorkflowID       string
			DagVersion       int
			Status           string
			TriggerSource    string
			TriggerSessionID string
			SlotTime         *time.Time
			StartedAt        *time.Time
			CreatedAt        time.Time
		}
		if err := rows.Scan(&r.RunID, &r.ScheduleID, &r.WorkflowID, &r.DagVersion,
			&r.Status, &r.TriggerSource, &r.TriggerSessionID, &r.SlotTime,
			&r.StartedAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		m := map[string]any{
			"run_id":             r.RunID,
			"schedule_id":        r.ScheduleID,
			"workflow_id":        r.WorkflowID,
			"dag_version":        r.DagVersion,
			"status":             r.Status,
			"trigger_source":     r.TriggerSource,
			"trigger_session_id": r.TriggerSessionID,
			"created_at":         r.CreatedAt.Format(time.RFC3339),
		}
		result = append(result, m)
	}
	return result, nil
}

// GetStepResults returns step results for a run.
func (s *WorkflowStore) GetStepResults(ctx context.Context, runID string) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT run_id, step_id, status, artifact, error, started_at, completed_at
		 FROM workflow_step_results WHERE run_id = $1 ORDER BY step_id`, runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var r struct {
			RunID       string
			StepID      string
			Status      string
			Artifact    string
			Error       string
			StartedAt   *time.Time
			CompletedAt *time.Time
		}
		if err := rows.Scan(&r.RunID, &r.StepID, &r.Status, &r.Artifact, &r.Error,
			&r.StartedAt, &r.CompletedAt); err != nil {
			return nil, err
		}
		m := map[string]any{
			"run_id":  r.RunID,
			"step_id": r.StepID,
			"status":  r.Status,
			"artifact": r.Artifact,
			"error":   r.Error,
		}
		result = append(result, m)
	}
	return result, nil
}

// SetStepResult inserts or updates a step result.
func (s *WorkflowStore) SetStepResult(ctx context.Context, runID, stepID, status, artifact, errMsg string) error {
	now := nowUTC()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO workflow_step_results (run_id, step_id, status, artifact, error, started_at, completed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (run_id, step_id) DO UPDATE SET
		   status = EXCLUDED.status,
		   artifact = CASE WHEN EXCLUDED.artifact != '' THEN EXCLUDED.artifact ELSE workflow_step_results.artifact END,
		   error = EXCLUDED.error,
		   completed_at = EXCLUDED.completed_at`,
		runID, stepID, status, artifact, errMsg, now, now,
	)
	return err
}

// FindResumableRuns finds paused runs with no remaining approval gates.
func (s *WorkflowStore) FindResumableRuns(ctx context.Context, limit int) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT r.run_id, r.workflow_id, r.dag_version, r.status,
		        r.trigger_source, r.trigger_session_id, r.created_at
		 FROM workflow_runs r
		 WHERE r.status = 'paused'
		   AND NOT EXISTS (
		     SELECT 1 FROM workflow_step_results sr
		     WHERE sr.run_id = r.run_id AND sr.status = 'awaiting_approval'
		   )
		 ORDER BY r.created_at
		 LIMIT $1`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var r struct {
			RunID            string
			WorkflowID       string
			DagVersion       int
			Status           string
			TriggerSource    string
			TriggerSessionID string
			CreatedAt        time.Time
		}
		if err := rows.Scan(&r.RunID, &r.WorkflowID, &r.DagVersion, &r.Status,
			&r.TriggerSource, &r.TriggerSessionID, &r.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{
			"run_id":             r.RunID,
			"workflow_id":        r.WorkflowID,
			"dag_version":        r.DagVersion,
			"status":             r.Status,
			"trigger_source":     r.TriggerSource,
			"trigger_session_id": r.TriggerSessionID,
			"created_at":         r.CreatedAt.Format(time.RFC3339),
		})
	}
	return result, nil
}

// FindOrphanedRuns finds runs stuck in "running" status (crash recovery).
func (s *WorkflowStore) FindOrphanedRuns(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		"SELECT run_id, workflow_id, dag_version FROM workflow_runs WHERE status = 'running'",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var runID, workflowID string
		var dagVersion int
		if err := rows.Scan(&runID, &workflowID, &dagVersion); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{
			"run_id":      runID,
			"workflow_id": workflowID,
			"dag_version": dagVersion,
		})
	}
	return result, nil
}

// --- Schedules ---

// CreateSchedule creates a new schedule.
func (s *WorkflowStore) CreateSchedule(ctx context.Context, workflowID string, dagVersion int, mode, scheduleExpr, tag, startAt, endAt string, catchUp bool) (string, error) {
	scheduleID := uuid.New().String()
	now := nowUTC()
	var startAtVal, endAtVal *string
	if startAt != "" {
		startAtVal = &startAt
	}
	if endAt != "" {
		endAtVal = &endAt
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO workflow_schedules (schedule_id, workflow_id, dag_version, dag_tag, status, mode, schedule, start_at, end_at, catch_up, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7, $8, $9, $10, $11)`,
		scheduleID, workflowID, dagVersion, tag, mode, scheduleExpr, startAtVal, endAtVal, catchUp, now, now,
	)
	if err != nil {
		return "", err
	}
	return scheduleID, nil
}

// ListActiveSchedules returns all active schedules across all workflows.
func (s *WorkflowStore) ListActiveSchedules(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT schedule_id, workflow_id, dag_version, dag_tag, status, mode,
		        schedule, start_at, end_at, catch_up, last_run_at
		 FROM workflow_schedules WHERE status = 'pending'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var sched struct {
			ScheduleID string
			WorkflowID string
			DagVersion int
			DagTag     string
			Status     string
			Mode       string
			Schedule   string
			StartAt    *time.Time
			EndAt      *time.Time
			CatchUp    bool
			LastRunAt  *time.Time
		}
		if err := rows.Scan(&sched.ScheduleID, &sched.WorkflowID, &sched.DagVersion,
			&sched.DagTag, &sched.Status, &sched.Mode, &sched.Schedule,
			&sched.StartAt, &sched.EndAt, &sched.CatchUp, &sched.LastRunAt); err != nil {
			return nil, err
		}
		m := map[string]any{
			"schedule_id": sched.ScheduleID,
			"workflow_id": sched.WorkflowID,
			"dag_version": sched.DagVersion,
			"mode":        sched.Mode,
			"schedule":    sched.Schedule,
			"catch_up":    sched.CatchUp,
		}
		if sched.StartAt != nil {
			m["start_at"] = sched.StartAt.Format(time.RFC3339)
		}
		if sched.EndAt != nil {
			m["end_at"] = sched.EndAt.Format(time.RFC3339)
		}
		if sched.LastRunAt != nil {
			m["last_run_at"] = sched.LastRunAt.Format(time.RFC3339)
		}
		result = append(result, m)
	}
	return result, nil
}

// UpdateScheduleLastRun records the last run time for a schedule.
func (s *WorkflowStore) UpdateScheduleLastRun(ctx context.Context, scheduleID, lastRunAt string, newStatus ...string) error {
	now := nowUTC()
	if len(newStatus) > 0 && newStatus[0] != "" {
		_, err := s.pool.Exec(ctx,
			"UPDATE workflow_schedules SET last_run_at = $1, status = $2, updated_at = $3 WHERE schedule_id = $4",
			lastRunAt, newStatus[0], now, scheduleID,
		)
		return err
	}
	_, err := s.pool.Exec(ctx,
		"UPDATE workflow_schedules SET last_run_at = $1, updated_at = $2 WHERE schedule_id = $3",
		lastRunAt, now, scheduleID,
	)
	return err
}

// UpdateScheduleStatus updates the status of a schedule.
func (s *WorkflowStore) UpdateScheduleStatus(ctx context.Context, scheduleID, status string) error {
	validStatuses := map[string]bool{"pending": true, "paused": true, "completed": true}
	if !validStatuses[status] {
		return fmt.Errorf("invalid schedule status: %s", status)
	}
	now := nowUTC()
	_, err := s.pool.Exec(ctx,
		"UPDATE workflow_schedules SET status = $1, updated_at = $2 WHERE schedule_id = $3",
		status, now, scheduleID,
	)
	return err
}
```

**Key differences from magicburg:**
- Uses `pgxpool.Pool` instead of `*dbpkg.DB` (wrapping `database/sql`)
- All queries use `$1, $2, ...` placeholders
- `ClaimPendingRuns` uses `FOR UPDATE SKIP LOCKED` (PostgreSQL-native, much better than magicburg's SQLite approach)
- Scan into typed structs instead of `map[string]any` via generic scanner
- `FindResumableRuns` uses a single query with `NOT EXISTS` subquery instead of two queries

---

## Task 5 — Prompts & Emitter (`prompts.go`, `emitter.go`)

### prompts.go

Copy `BuildStepPrompt` verbatim from magicburg. It has no external dependencies.

**File:** `gobackend/internal/workflows/prompts.go`

```go
package workflows

import (
	"fmt"
	"sort"
	"strings"
)

const defaultAgentTemplate = "You are an execution agent. Complete the assigned task precisely and output only the result."

// BuildStepPrompt builds the LLM prompt for executing a single workflow step.
func BuildStepPrompt(workflowTitle, stepName, stepInstruction string, artifacts map[string]string, actorPersona, userID, slotTime string) string {
	parts := []string{}

	if actorPersona != "" {
		parts = append(parts, actorPersona)
	}

	parts = append(parts, defaultAgentTemplate)
	parts = append(parts, fmt.Sprintf("You are executing step %q of the workflow %q.", stepName, workflowTitle))

	if slotTime != "" {
		parts = append(parts, fmt.Sprintf("Slot time: %s (the scheduled time this run represents)", slotTime))
	}

	parts = append(parts, fmt.Sprintf("Instruction: %s", stepInstruction))

	if len(artifacts) > 0 {
		keys := make([]string, 0, len(artifacts))
		for k := range artifacts {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var artifactParts []string
		for _, k := range keys {
			v := artifacts[k]
			if v != "" {
				artifactParts = append(artifactParts, fmt.Sprintf("## %s\n%s", k, v))
			}
		}
		if len(artifactParts) > 0 {
			context := "Context from completed steps:\n---\n" + strings.Join(artifactParts, "\n---\n") + "\n---"
			parts = append(parts, context)
		}
	}

	return strings.Join(parts, "\n\n")
}
```

**File:** `gobackend/internal/workflows/prompts_test.go`

Copy verbatim from magicburg. Tests are pure unit tests with no external dependencies beyond `stretchr/testify`.

### emitter.go

Bridge does not use JSONL file-based chat history like magicburg. Bridge emits to the SSE event bus (`internal/events`). The emitter interface remains the same, but the implementation uses `SessionBroadcaster`.

**File:** `gobackend/internal/workflows/emitter.go`

```go
package workflows

import (
	"fmt"
	"log/slog"

	"github.com/bridge-edu/bridge/gobackend/internal/events"
)

// StatusEmitter emits workflow status messages to live sessions.
type StatusEmitter interface {
	EmitStepStatus(workflowID, runID, stepID, stepName, status, triggerSessionID, userID string) error
	EmitRunCompletion(workflowID, runID, status, summary string, artifacts map[string]string, triggerSessionID, userID string) error
}

// BroadcastEmitter implements StatusEmitter via the SSE event bus.
type BroadcastEmitter struct {
	broadcaster *events.SessionBroadcaster
}

// NewBroadcastEmitter creates a BroadcastEmitter backed by the given broadcaster.
func NewBroadcastEmitter(broadcaster *events.SessionBroadcaster) *BroadcastEmitter {
	return &BroadcastEmitter{broadcaster: broadcaster}
}

func (e *BroadcastEmitter) EmitStepStatus(workflowID, runID, stepID, stepName, status, triggerSessionID, userID string) error {
	if triggerSessionID == "" {
		return nil
	}
	shortRun := runID
	if len(shortRun) > 8 {
		shortRun = shortRun[:8]
	}

	e.broadcaster.Broadcast(triggerSessionID, events.Event{
		Type: "workflow_step",
		Data: map[string]any{
			"workflow_id": workflowID,
			"run_id":      runID,
			"step_id":     stepID,
			"step_name":   stepName,
			"status":      status,
			"message":     fmt.Sprintf("[Run %s] Step '%s' %s", shortRun, stepName, status),
		},
	})
	return nil
}

func (e *BroadcastEmitter) EmitRunCompletion(workflowID, runID, status, summary string, artifacts map[string]string, triggerSessionID, userID string) error {
	if triggerSessionID == "" {
		return nil
	}
	shortRun := runID
	if len(shortRun) > 8 {
		shortRun = shortRun[:8]
	}

	artifactPreview := make(map[string]string)
	for stepID, artifact := range artifacts {
		preview := artifact
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		artifactPreview[stepID] = preview
	}

	e.broadcaster.Broadcast(triggerSessionID, events.Event{
		Type: "workflow_complete",
		Data: map[string]any{
			"workflow_id": workflowID,
			"run_id":      runID,
			"status":      status,
			"summary":     summary,
			"artifacts":   artifactPreview,
			"message":     fmt.Sprintf("[Run %s] Workflow run %s: %s", shortRun, status, summary),
		},
	})
	return nil
}

// NoopEmitter is a silent emitter for testing and background jobs.
type NoopEmitter struct{}

func (e *NoopEmitter) EmitStepStatus(_, _, _, _, _, _, _ string) error { return nil }
func (e *NoopEmitter) EmitRunCompletion(_, _, _, _ string, _ map[string]string, _, _ string) error {
	return nil
}
```

**File:** `gobackend/internal/workflows/emitter_test.go`

```go
package workflows

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopEmitter(t *testing.T) {
	e := &NoopEmitter{}
	assert.NoError(t, e.EmitStepStatus("wf", "run", "step", "name", "completed", "sess", "user"))
	assert.NoError(t, e.EmitRunCompletion("wf", "run", "completed", "done", nil, "sess", "user"))
}

func TestBroadcastEmitter_EmptySession(t *testing.T) {
	e := &BroadcastEmitter{broadcaster: nil}
	// Empty trigger session should be a no-op (does not touch broadcaster)
	err := e.EmitStepStatus("wf", "run", "step", "name", "completed", "", "user")
	assert.NoError(t, err)

	err = e.EmitRunCompletion("wf", "run", "completed", "done", nil, "", "user")
	assert.NoError(t, err)
}
```

---

## Task 6 — DAG Run Executor (`executor.go`)

Copy from magicburg and adapt the `WorkflowStore` method signatures to include `context.Context` (Bridge pattern). The core logic is identical: topological sort into layers, parallel goroutine execution within layers, artifact chain, approval gates, failure propagation.

**File:** `gobackend/internal/workflows/executor.go`

```go
package workflows

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/bridge-edu/bridge/gobackend/internal/llm"
)

// LLMRunner executes an LLM agent call and returns the text output.
type LLMRunner interface {
	RunAgent(ctx context.Context, prompt, persona, sessionID, userID string) (string, error)
	RunAgentWithTools(ctx context.Context, task, persona, sessionID, userID string, tools []llm.ToolSpec, executor llm.ToolExecutor) (string, int, error)
}

// RunExecutor manages the execution of a single workflow run.
type RunExecutor struct {
	store   *WorkflowStore
	llm     LLMRunner
	emitter StatusEmitter
}

// NewRunExecutor creates a new RunExecutor.
func NewRunExecutor(store *WorkflowStore, llm LLMRunner, emitter StatusEmitter) *RunExecutor {
	return &RunExecutor{store: store, llm: llm, emitter: emitter}
}

// Execute runs a workflow DAG to completion (or until paused/failed).
func (e *RunExecutor) Execute(ctx context.Context, run map[string]any) error {
	runID, _ := run["run_id"].(string)
	workflowID, _ := run["workflow_id"].(string)
	dagVersion, _ := run["dag_version"].(int)

	logger := slog.With("run_id", runID, "workflow_id", workflowID, "dag_version", dagVersion)
	logger.Info("starting workflow run execution")

	// Mark run as running
	now := nowUTC()
	if err := e.store.UpdateRun(ctx, runID, map[string]any{
		"status":     "running",
		"started_at": now,
	}); err != nil {
		return fmt.Errorf("mark run running: %w", err)
	}

	// Load DAG content
	dag, err := e.store.GetDagContent(ctx, workflowID, dagVersion)
	if err != nil {
		e.failRun(ctx, runID, fmt.Sprintf("failed to load DAG: %v", err))
		return nil
	}
	if dag == nil {
		e.failRun(ctx, runID, "DAG not found")
		return nil
	}

	// Validate DAG
	validationErrors := ValidateDAG(dag)
	if len(validationErrors) > 0 {
		e.failRun(ctx, runID, fmt.Sprintf("DAG validation failed: %s", strings.Join(validationErrors, "; ")))
		return nil
	}

	// Topological sort
	layers := TopologicalSort(dag)

	// Build helper maps
	stepMap := buildStepMap(dag)
	actorMap := buildActorMap(dag)
	depsMap := extractStepDeps(dag)

	// Load existing results (for resume support)
	artifacts := make(map[string]string)
	failedSteps := make(map[string]bool)
	existingResults, err := e.store.GetStepResults(ctx, runID)
	if err != nil {
		e.failRun(ctx, runID, fmt.Sprintf("failed to load step results: %v", err))
		return nil
	}
	for _, r := range existingResults {
		sid, _ := r["step_id"].(string)
		status, _ := r["status"].(string)
		artifact, _ := r["artifact"].(string)
		if status == "completed" {
			artifacts[sid] = artifact
		} else if status == "failed" {
			failedSteps[sid] = true
		}
	}

	// Get workflow title
	workflowTitle := ""
	wf, err := e.store.Get(ctx, workflowID)
	if err == nil && wf != nil {
		workflowTitle, _ = wf["title"].(string)
	}

	// Get trigger info
	triggerSessionID, _ := run["trigger_session_id"].(string)
	slotTime, _ := run["slot_time"].(string)
	userID := ""
	if wf != nil {
		userID, _ = wf["user_id"].(string)
	}

	// Execute layers
	for layerIdx, layer := range layers {
		logger.Info("executing layer", "layer", layerIdx, "steps", layer)

		if err := ctx.Err(); err != nil {
			e.failRun(ctx, runID, fmt.Sprintf("execution cancelled: %v", err))
			return nil
		}

		var (
			mu          sync.Mutex
			wg          sync.WaitGroup
			layerFailed bool
			layerPaused bool
		)

		for _, stepID := range layer {
			stepID := stepID

			wg.Add(1)
			go func() {
				defer wg.Done()

				// Skip if already completed
				mu.Lock()
				if _, done := artifacts[stepID]; done {
					mu.Unlock()
					logger.Info("skipping already-completed step", "step_id", stepID)
					return
				}
				mu.Unlock()

				// Check if any dependency failed
				deps := depsMap[stepID]
				mu.Lock()
				depFailed := false
				for _, depID := range deps {
					if failedSteps[depID] {
						depFailed = true
						break
					}
				}
				mu.Unlock()

				if depFailed {
					_ = e.store.SetStepResult(ctx, runID, stepID, "skipped", "", "dependency failed")
					mu.Lock()
					failedSteps[stepID] = true
					layerFailed = true
					mu.Unlock()
					return
				}

				// Collect upstream artifacts
				mu.Lock()
				upstreamArtifacts := make(map[string]string)
				for _, depID := range deps {
					if art, ok := artifacts[depID]; ok {
						upstreamArtifacts[depID] = art
					}
				}
				mu.Unlock()

				// Get step config
				step := stepMap[stepID]
				stepName, _ := step["name"].(string)
				stepInstruction, _ := step["instruction"].(string)
				actorPersona := ""
				if actorID, ok := step["actor"].(string); ok && actorID != "" {
					if actor, ok := actorMap[actorID]; ok {
						actorPersona, _ = actor["persona"].(string)
					}
				}

				// Check approval requirement
				needsApproval := false
				if cfg, ok := step["config"].(map[string]any); ok {
					if appr, ok := cfg["approval"].(bool); ok && appr {
						needsApproval = true
					}
				}

				// Mark step as running
				_ = e.store.SetStepResult(ctx, runID, stepID, "running", "", "")

				// Build prompt and call LLM
				prompt := BuildStepPrompt(workflowTitle, stepName, stepInstruction, upstreamArtifacts, actorPersona, userID, slotTime)
				result, err := e.llm.RunAgent(ctx, prompt, actorPersona, runID, userID)

				if err != nil {
					logger.Error("step failed", "step_id", stepID, "error", err)
					_ = e.store.SetStepResult(ctx, runID, stepID, "failed", "", err.Error())
					mu.Lock()
					failedSteps[stepID] = true
					layerFailed = true
					mu.Unlock()
					_ = e.emitter.EmitStepStatus(workflowID, runID, stepID, stepName, "failed", triggerSessionID, userID)
					return
				}

				if needsApproval {
					_ = e.store.SetStepResult(ctx, runID, stepID, "awaiting_approval", result, "")
					mu.Lock()
					artifacts[stepID] = result
					layerPaused = true
					mu.Unlock()
					_ = e.emitter.EmitStepStatus(workflowID, runID, stepID, stepName, "awaiting_approval", triggerSessionID, userID)
				} else {
					_ = e.store.SetStepResult(ctx, runID, stepID, "completed", result, "")
					mu.Lock()
					artifacts[stepID] = result
					mu.Unlock()
					_ = e.emitter.EmitStepStatus(workflowID, runID, stepID, stepName, "completed", triggerSessionID, userID)
				}
			}()
		}

		wg.Wait()

		if layerPaused {
			logger.Info("run paused for approval", "layer", layerIdx)
			_ = e.store.UpdateRun(ctx, runID, map[string]any{"status": "paused"})
			return nil
		}

		if layerFailed {
			logger.Info("run failed due to step failure", "layer", layerIdx)
			e.skipRemainingSteps(ctx, runID, layers, layerIdx+1, stepMap, failedSteps)
			now := nowUTC()
			_ = e.store.UpdateRun(ctx, runID, map[string]any{
				"status":         "failed",
				"completed_at":   now,
				"result_summary": "Run failed: one or more steps encountered errors",
			})
			_ = e.emitter.EmitRunCompletion(workflowID, runID, "failed", "Run failed", artifacts, triggerSessionID, userID)
			return nil
		}
	}

	// All layers completed successfully
	logger.Info("workflow run completed successfully")
	now = nowUTC()
	summary := buildRunSummary(layers, stepMap, artifacts)
	_ = e.store.UpdateRun(ctx, runID, map[string]any{
		"status":         "completed",
		"completed_at":   now,
		"result_summary": summary,
	})
	_ = e.emitter.EmitRunCompletion(workflowID, runID, "completed", summary, artifacts, triggerSessionID, userID)
	return nil
}

// failRun marks a run as failed with the given summary.
func (e *RunExecutor) failRun(ctx context.Context, runID, summary string) {
	now := nowUTC()
	_ = e.store.UpdateRun(ctx, runID, map[string]any{
		"status":         "failed",
		"completed_at":   now,
		"result_summary": summary,
	})
}

// skipRemainingSteps marks all steps in layers after fromLayer as skipped.
func (e *RunExecutor) skipRemainingSteps(ctx context.Context, runID string, layers [][]string, fromLayer int, stepMap map[string]map[string]any, alreadyHandled map[string]bool) {
	for i := fromLayer; i < len(layers); i++ {
		for _, stepID := range layers[i] {
			if alreadyHandled[stepID] {
				continue
			}
			_ = e.store.SetStepResult(ctx, runID, stepID, "skipped", "", "upstream step failed")
		}
	}
}

// buildStepMap converts the DAG "steps" array into a map keyed by step ID.
func buildStepMap(dag map[string]any) map[string]map[string]any {
	stepsRaw, _ := dag["steps"].([]any)
	m := make(map[string]map[string]any, len(stepsRaw))
	for _, s := range stepsRaw {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		id, _ := step["id"].(string)
		if id != "" {
			m[id] = step
		}
	}
	return m
}

// buildActorMap converts the DAG "actors" array into a map keyed by actor ID.
func buildActorMap(dag map[string]any) map[string]map[string]any {
	actorsRaw, _ := dag["actors"].([]any)
	m := make(map[string]map[string]any, len(actorsRaw))
	for _, a := range actorsRaw {
		actor, ok := a.(map[string]any)
		if !ok {
			continue
		}
		id, _ := actor["id"].(string)
		if id != "" {
			m[id] = actor
		}
	}
	return m
}

// buildRunSummary creates a human-readable summary of the completed run.
func buildRunSummary(layers [][]string, stepMap map[string]map[string]any, artifacts map[string]string) string {
	totalSteps := 0
	for _, layer := range layers {
		totalSteps += len(layer)
	}
	if totalSteps == 0 {
		return "Workflow completed (no steps)"
	}

	completedCount := 0
	var stepNames []string
	for _, layer := range layers {
		for _, stepID := range layer {
			if _, ok := artifacts[stepID]; ok {
				completedCount++
			}
			if step, ok := stepMap[stepID]; ok {
				name, _ := step["name"].(string)
				if name != "" {
					stepNames = append(stepNames, name)
				}
			}
		}
	}

	return fmt.Sprintf("Completed %d/%d steps: %s",
		completedCount, totalSteps,
		strings.Join(stepNames, " -> "))
}
```

**File:** `gobackend/internal/workflows/executor_test.go`

The executor tests need the most adaptation since they depend on the store (which now uses pgxpool). We will use a test PostgreSQL database.

```go
package workflows

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bridge-edu/bridge/gobackend/internal/llm"
)

// --- Mocks ---

type mockLLM struct {
	mu        sync.Mutex
	responses map[string]string
	err       error
	calls     []string
}

func (m *mockLLM) RunAgent(ctx context.Context, prompt, persona, sessionID, userID string) (string, error) {
	m.mu.Lock()
	m.calls = append(m.calls, prompt)
	m.mu.Unlock()
	if m.err != nil {
		return "", m.err
	}
	for key, resp := range m.responses {
		if strings.Contains(prompt, key) {
			return resp, nil
		}
	}
	return "mock output", nil
}

func (m *mockLLM) RunAgentWithTools(_ context.Context, task, persona, sessionID, userID string, _ []llm.ToolSpec, _ llm.ToolExecutor) (string, int, error) {
	result, err := m.RunAgent(context.Background(), task, persona, sessionID, userID)
	return result, 0, err
}

type failingLLM struct {
	mu        sync.Mutex
	responses map[string]string
	failOn    string
	failErr   error
	calls     []string
}

func (m *failingLLM) RunAgent(ctx context.Context, prompt, persona, sessionID, userID string) (string, error) {
	m.mu.Lock()
	m.calls = append(m.calls, prompt)
	m.mu.Unlock()
	if m.failOn != "" && strings.Contains(prompt, m.failOn) {
		return "", m.failErr
	}
	for key, resp := range m.responses {
		if strings.Contains(prompt, key) {
			return resp, nil
		}
	}
	return "mock output", nil
}

func (m *failingLLM) RunAgentWithTools(_ context.Context, task, persona, sessionID, userID string, _ []llm.ToolSpec, _ llm.ToolExecutor) (string, int, error) {
	result, err := m.RunAgent(context.Background(), task, persona, sessionID, userID)
	return result, 0, err
}

type mockEmitter struct {
	mu         sync.Mutex
	stepEvents []string
	runEvents  []string
}

func (m *mockEmitter) EmitStepStatus(workflowID, runID, stepID, stepName, status, triggerSessionID, userID string) error {
	m.mu.Lock()
	m.stepEvents = append(m.stepEvents, fmt.Sprintf("%s:%s", stepID, status))
	m.mu.Unlock()
	return nil
}

func (m *mockEmitter) EmitRunCompletion(workflowID, runID, status, summary string, artifacts map[string]string, triggerSessionID, userID string) error {
	m.mu.Lock()
	m.runEvents = append(m.runEvents, fmt.Sprintf("%s:%s", runID, status))
	m.mu.Unlock()
	return nil
}

// --- Helpers ---
// setupExecutorTest requires TEST_DATABASE_URL env var pointing to a test PostgreSQL database.
// Tests create a fresh schema per test using a unique search_path.

func setupExecutorTest(t *testing.T) (*WorkflowStore, *mockLLM, *mockEmitter, *RunExecutor) {
	t.Helper()
	pool := testPool(t) // defined in store_test.go — creates pgxpool, runs migration
	store := NewWorkflowStore(pool)
	llmMock := &mockLLM{responses: make(map[string]string)}
	emitter := &mockEmitter{}
	executor := NewRunExecutor(store, llmMock, emitter)
	return store, llmMock, emitter, executor
}

func createTestWorkflowAndRun(t *testing.T, store *WorkflowStore, dag map[string]any) (string, string, map[string]any) {
	t.Helper()
	ctx := context.Background()
	userID := "test-user"
	wfID, err := store.Create(ctx, userID, "Test Workflow", "")
	require.NoError(t, err)

	_, err = store.SaveDag(ctx, wfID, dag, "test dag", "", 0)
	require.NoError(t, err)

	runID, err := store.CreateRun(ctx, wfID, 1, WithTriggerSource("test"))
	require.NoError(t, err)

	run, err := store.GetRun(ctx, runID)
	require.NoError(t, err)
	require.NotNil(t, run)

	return wfID, runID, run
}

// Test functions mirror magicburg's executor_test.go:
// - TestExecuteLinearDAG
// - TestExecuteParallelDAG
// - TestExecuteWithArtifactPassing
// - TestExecuteApprovalGate
// - TestExecuteStepFailure
// - TestExecuteResumeAfterApproval
// - TestExecuteDAGValidationFailure
// - TestExecuteWithActorPersona
// - TestExecuteEmptyDAG
// - TestExecuteContextCancellation
// - TestExecuteFullWorkflowLifecycle
// - TestExecuteWithFailureAndSkip
//
// Each test body is identical to magicburg except:
// - store methods take ctx as first argument
// - assertions remain unchanged
```

---

## Task 7 — Bridge AI Agents

### 7a. Student Tutor Agent (`internal/agents/student_tutor.go`)

The student tutor agent provides per-student AI tutoring during live sessions. It is invoked as a workflow step or called directly from the AI chat handler. It respects grade-level-appropriate language, never gives full answers, and references the student's code.

**File:** `gobackend/internal/agents/student_tutor.go`

```go
package agents

import (
	"context"
	"fmt"

	"github.com/bridge-edu/bridge/gobackend/internal/llm"
)

// GradeLevel represents a K-12 grade band.
type GradeLevel string

const (
	GradeK5  GradeLevel = "K-5"
	Grade68  GradeLevel = "6-8"
	Grade912 GradeLevel = "9-12"
)

const baseRules = `You are a patient coding tutor helping a student learn to program.

RULES:
- Ask guiding questions to help the student think through the problem
- Point to where the issue might be (e.g., "look at line 5"), but don't give the answer
- Never provide complete function implementations or full solutions
- If the student asks you to write the code for them, redirect them to think about the approach
- Celebrate small wins and encourage persistence
- Keep responses concise (2-4 sentences unless explaining a concept)`

var gradePrompts = map[GradeLevel]string{
	GradeK5: baseRules + `

GRADE LEVEL: Elementary (K-5)
- Use simple vocabulary and short sentences
- Use analogies from everyday life (building blocks, recipes, treasure maps)
- Be extra encouraging and patient
- Focus on visual thinking: "What do you see happening when you run this?"
- Reference block concepts if using Blockly: "Which purple block did you use?"`,

	Grade68: baseRules + `

GRADE LEVEL: Middle School (6-8)
- Explain concepts clearly but don't over-simplify
- Reference specific line numbers: "Take a look at line 7 — what value does x have there?"
- Use analogies when helpful but can be more technical
- Encourage reading error messages: "What does the error message tell you?"
- Help build debugging habits: "What did you expect to happen vs what actually happened?"`,

	Grade912: baseRules + `

GRADE LEVEL: High School (9-12)
- Use proper technical terminology
- Reference documentation and best practices
- Discuss trade-offs when relevant: "This works, but what happens if the list is empty?"
- Encourage independent problem-solving: "How would you test that this works?"
- Help develop computational thinking and code organization skills`,
}

// TutorRequest contains the inputs for a tutoring interaction.
type TutorRequest struct {
	StudentName string
	GradeLevel  GradeLevel
	Language    string // programming language
	Code        string // student's current code
	Question    string // student's question
	TopicTitle  string // current lesson topic
	ErrorOutput string // if the student ran the code and got an error
}

// StudentTutor generates grade-appropriate tutoring responses.
type StudentTutor struct {
	backend llm.Backend
}

// NewStudentTutor creates a new StudentTutor.
func NewStudentTutor(backend llm.Backend) *StudentTutor {
	return &StudentTutor{backend: backend}
}

// SystemPrompt returns the grade-appropriate system prompt.
func SystemPrompt(grade GradeLevel) string {
	prompt, ok := gradePrompts[grade]
	if !ok {
		return gradePrompts[Grade68]
	}
	return prompt
}

// Respond generates a tutoring response for the given request.
func (t *StudentTutor) Respond(ctx context.Context, req TutorRequest) (string, error) {
	system := SystemPrompt(req.GradeLevel)

	userPrompt := fmt.Sprintf("Student: %s\nLanguage: %s\nTopic: %s\n\n", req.StudentName, req.Language, req.TopicTitle)
	if req.Code != "" {
		userPrompt += fmt.Sprintf("Current code:\n```%s\n%s\n```\n\n", req.Language, req.Code)
	}
	if req.ErrorOutput != "" {
		userPrompt += fmt.Sprintf("Error output:\n```\n%s\n```\n\n", req.ErrorOutput)
	}
	userPrompt += fmt.Sprintf("Question: %s", req.Question)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: system},
		{Role: llm.RoleUser, Content: userPrompt},
	}

	resp, err := t.backend.Chat(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("tutor chat: %w", err)
	}
	return resp.Content, nil
}

// RespondWithHistory generates a tutoring response with conversation history.
func (t *StudentTutor) RespondWithHistory(ctx context.Context, req TutorRequest, history []llm.Message) (string, error) {
	system := SystemPrompt(req.GradeLevel)

	messages := make([]llm.Message, 0, len(history)+2)
	messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: system})
	messages = append(messages, history...)

	userPrompt := req.Question
	if req.Code != "" {
		userPrompt = fmt.Sprintf("My current code:\n```%s\n%s\n```\n\n%s", req.Language, req.Code, req.Question)
	}
	if req.ErrorOutput != "" {
		userPrompt += fmt.Sprintf("\n\nError output:\n```\n%s\n```", req.ErrorOutput)
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: userPrompt})

	resp, err := t.backend.Chat(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("tutor chat with history: %w", err)
	}
	return resp.Content, nil
}
```

**File:** `gobackend/internal/agents/student_tutor_test.go`

```go
package agents

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemPrompt(t *testing.T) {
	tests := []struct {
		grade    GradeLevel
		contains string
	}{
		{GradeK5, "Elementary (K-5)"},
		{Grade68, "Middle School (6-8)"},
		{Grade912, "High School (9-12)"},
		{"invalid", "Middle School (6-8)"}, // defaults to 6-8
	}
	for _, tt := range tests {
		t.Run(string(tt.grade), func(t *testing.T) {
			prompt := SystemPrompt(tt.grade)
			assert.Contains(t, prompt, tt.contains)
			assert.Contains(t, prompt, "Never provide complete function implementations")
		})
	}
}

func TestStudentTutor_Respond(t *testing.T) {
	mock := &mockBackend{response: "Great question! Look at line 3."}
	tutor := NewStudentTutor(mock)

	resp, err := tutor.Respond(context.Background(), TutorRequest{
		StudentName: "Alice",
		GradeLevel:  Grade68,
		Language:    "python",
		Code:        "x = 5\nprint(x + y)",
		Question:    "Why do I get a NameError?",
		TopicTitle:  "Variables",
	})
	require.NoError(t, err)
	assert.Equal(t, "Great question! Look at line 3.", resp)

	// Verify system prompt was grade-appropriate
	require.Len(t, mock.calls, 1)
	assert.Equal(t, "system", string(mock.calls[0][0].Role))
	assert.Contains(t, mock.calls[0][0].Text(), "Middle School (6-8)")

	// Verify user prompt contains code and question
	userMsg := mock.calls[0][1].Text()
	assert.Contains(t, userMsg, "x = 5")
	assert.Contains(t, userMsg, "NameError")
}

func TestStudentTutor_Respond_WithError(t *testing.T) {
	mock := &mockBackend{response: "Check your variable name."}
	tutor := NewStudentTutor(mock)

	resp, err := tutor.Respond(context.Background(), TutorRequest{
		StudentName: "Bob",
		GradeLevel:  GradeK5,
		Language:    "blockly",
		Question:    "My blocks are not connecting",
		TopicTitle:  "Loops",
		ErrorOutput: "BlockError: invalid connection",
	})
	require.NoError(t, err)
	assert.Equal(t, "Check your variable name.", resp)

	userMsg := mock.calls[0][1].Text()
	assert.Contains(t, userMsg, "BlockError: invalid connection")
}

func TestStudentTutor_RespondWithHistory(t *testing.T) {
	mock := &mockBackend{response: "Yes, exactly right!"}
	tutor := NewStudentTutor(mock)

	history := []llm.Message{
		{Role: llm.RoleUser, Content: "What is a variable?"},
		{Role: llm.RoleAssistant, Content: "A variable is like a labeled box."},
	}

	resp, err := tutor.RespondWithHistory(context.Background(), TutorRequest{
		StudentName: "Carol",
		GradeLevel:  Grade912,
		Language:    "javascript",
		Question:    "So let and const are different boxes?",
	}, history)
	require.NoError(t, err)
	assert.Equal(t, "Yes, exactly right!", resp)

	// Verify history was included
	require.Len(t, mock.calls, 1)
	assert.Len(t, mock.calls[0], 4) // system + 2 history + user
}
```

### 7b. Teacher Assistant Agent (`internal/agents/teacher_assistant.go`)

The teacher assistant monitors live session data and generates real-time recommendations for the teacher.

**File:** `gobackend/internal/agents/teacher_assistant.go`

```go
package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/bridge-edu/bridge/gobackend/internal/llm"
)

const teacherAssistantPersona = `You are a real-time teaching assistant for a coding classroom.
Your role is to help the teacher by analyzing student progress data and providing actionable insights.

GUIDELINES:
- Be concise: teachers are busy during live sessions
- Lead with the most urgent issue
- Use specific numbers: "3 of 12 students" not "some students"
- Suggest concrete actions: "Consider showing an example of..." not "maybe review..."
- Group similar issues: "5 students have the same indentation error on line 3"
- Flag students who may need individual help
- Never suggest answers to give students — suggest teaching strategies`

// SessionSnapshot contains aggregated data from a live session at a point in time.
type SessionSnapshot struct {
	SessionID      string
	TopicTitle     string
	Language       string
	TotalStudents  int
	ActiveStudents int
	NeedHelp       int // students who clicked "need help"
	StudentStates  []StudentState
}

// StudentState represents one student's current state in a session.
type StudentState struct {
	StudentName  string
	Code         string
	ErrorOutput  string
	IsStuck      bool   // no code changes in >2 minutes
	HelpRequests int
	LineCount    int
	Status       string // "active", "idle", "needs_help"
}

// TeacherInsight is a recommendation for the teacher.
type TeacherInsight struct {
	Summary        string
	Recommendations []string
	UrgentStudents []string // students needing immediate attention
}

// TeacherAssistant generates real-time recommendations for teachers.
type TeacherAssistant struct {
	backend llm.Backend
}

// NewTeacherAssistant creates a new TeacherAssistant.
func NewTeacherAssistant(backend llm.Backend) *TeacherAssistant {
	return &TeacherAssistant{backend: backend}
}

// Analyze generates insights from a session snapshot.
func (ta *TeacherAssistant) Analyze(ctx context.Context, snapshot SessionSnapshot) (string, error) {
	prompt := buildTeacherPrompt(snapshot)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: teacherAssistantPersona},
		{Role: llm.RoleUser, Content: prompt},
	}

	resp, err := ta.backend.Chat(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("teacher assistant: %w", err)
	}
	return resp.Content, nil
}

func buildTeacherPrompt(snap SessionSnapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Session: %s\nTopic: %s\nLanguage: %s\n", snap.SessionID, snap.TopicTitle, snap.Language)
	fmt.Fprintf(&b, "Students: %d active / %d total, %d requesting help\n\n", snap.ActiveStudents, snap.TotalStudents, snap.NeedHelp)

	// Summarize common patterns
	stuckCount := 0
	errorStudents := 0
	var commonErrors []string

	for _, s := range snap.StudentStates {
		if s.IsStuck {
			stuckCount++
		}
		if s.ErrorOutput != "" {
			errorStudents++
			// First line of error for pattern detection
			firstLine := s.ErrorOutput
			if idx := strings.Index(firstLine, "\n"); idx > 0 {
				firstLine = firstLine[:idx]
			}
			commonErrors = append(commonErrors, fmt.Sprintf("  - %s: %s", s.StudentName, firstLine))
		}
	}

	fmt.Fprintf(&b, "Stuck students (no changes >2min): %d\n", stuckCount)
	fmt.Fprintf(&b, "Students with errors: %d\n", errorStudents)

	if len(commonErrors) > 0 {
		b.WriteString("\nError details:\n")
		for _, e := range commonErrors {
			b.WriteString(e + "\n")
		}
	}

	b.WriteString("\nProvide a brief analysis and 2-3 actionable recommendations for the teacher.")
	return b.String()
}
```

**File:** `gobackend/internal/agents/teacher_assistant_test.go`

```go
package agents

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeacherAssistant_Analyze(t *testing.T) {
	mock := &mockBackend{response: "3 of 5 students have an IndentationError on line 3. Consider a quick demo."}
	ta := NewTeacherAssistant(mock)

	resp, err := ta.Analyze(context.Background(), SessionSnapshot{
		SessionID:      "sess-1",
		TopicTitle:     "For Loops",
		Language:       "python",
		TotalStudents:  5,
		ActiveStudents: 5,
		NeedHelp:       2,
		StudentStates: []StudentState{
			{StudentName: "Alice", ErrorOutput: "IndentationError: unexpected indent", IsStuck: true},
			{StudentName: "Bob", ErrorOutput: "IndentationError: unexpected indent", IsStuck: true},
			{StudentName: "Carol", ErrorOutput: "IndentationError: unexpected indent"},
			{StudentName: "Dave", Code: "for i in range(5):\n  print(i)", Status: "active"},
			{StudentName: "Eve", Code: "for i in range(5):\n  print(i)", Status: "active"},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, resp, "IndentationError")

	// Verify system prompt is the teacher persona
	require.Len(t, mock.calls, 1)
	assert.Contains(t, mock.calls[0][0].Text(), "real-time teaching assistant")

	// Verify the user prompt includes student data
	userMsg := mock.calls[0][1].Text()
	assert.Contains(t, userMsg, "For Loops")
	assert.Contains(t, userMsg, "Stuck students")
	assert.Contains(t, userMsg, "Students with errors: 3")
}

func TestTeacherAssistant_EmptySession(t *testing.T) {
	mock := &mockBackend{response: "No issues detected."}
	ta := NewTeacherAssistant(mock)

	resp, err := ta.Analyze(context.Background(), SessionSnapshot{
		SessionID:      "sess-2",
		TopicTitle:     "Variables",
		Language:       "python",
		TotalStudents:  0,
		ActiveStudents: 0,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp)
}

func TestBuildTeacherPrompt(t *testing.T) {
	prompt := buildTeacherPrompt(SessionSnapshot{
		SessionID:      "sess-1",
		TopicTitle:     "Loops",
		Language:       "python",
		TotalStudents:  10,
		ActiveStudents: 8,
		NeedHelp:       3,
		StudentStates: []StudentState{
			{StudentName: "Alice", IsStuck: true, ErrorOutput: "SyntaxError: invalid syntax\n  line 5"},
			{StudentName: "Bob", IsStuck: false, Status: "active"},
		},
	})
	assert.Contains(t, prompt, "8 active / 10 total, 3 requesting help")
	assert.Contains(t, prompt, "Stuck students (no changes >2min): 1")
	assert.Contains(t, prompt, "Students with errors: 1")
	assert.Contains(t, prompt, "Alice: SyntaxError: invalid syntax")
}
```

### 7c. Self-Pacer Agent (`internal/agents/self_pacer.go`)

The self-pacer agent analyzes a student's performance history and recommends the next topic or a review.

**File:** `gobackend/internal/agents/self_pacer.go`

```go
package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/bridge-edu/bridge/gobackend/internal/llm"
)

const selfPacerPersona = `You are a personalized learning advisor for K-12 coding students.
Your role is to recommend what the student should learn next based on their performance history.

GUIDELINES:
- If the student struggled with a topic, suggest review or prerequisite topics
- If the student excelled, suggest advancing to the next topic
- Consider both assignment grades and engagement (AI interactions, session attendance)
- Be encouraging but honest about gaps
- Output a structured recommendation with: recommended_action (advance/review/practice), topic_id, reasoning

Respond in JSON format:
{
  "recommended_action": "advance" | "review" | "practice",
  "topic_id": "<topic UUID>",
  "topic_title": "<topic name>",
  "reasoning": "<1-2 sentence explanation>",
  "confidence": 0.0-1.0
}`

// StudentPerformance contains a student's historical performance data.
type StudentPerformance struct {
	StudentID       string
	StudentName     string
	GradeLevel      GradeLevel
	CompletedTopics []TopicPerformance
	CurrentTopicID  string
	AvailableTopics []AvailableTopic
}

// TopicPerformance tracks how a student performed on a completed topic.
type TopicPerformance struct {
	TopicID           string
	TopicTitle        string
	AssignmentGrade   *float64 // nil = not graded
	AIInteractions    int
	SessionsAttended  int
	SessionsAvailable int
}

// AvailableTopic is a topic the student hasn't completed yet.
type AvailableTopic struct {
	TopicID    string
	TopicTitle string
	SortOrder  int
}

// SelfPacer generates personalized learning path recommendations.
type SelfPacer struct {
	backend llm.Backend
}

// NewSelfPacer creates a new SelfPacer.
func NewSelfPacer(backend llm.Backend) *SelfPacer {
	return &SelfPacer{backend: backend}
}

// Recommend generates a learning path recommendation.
func (sp *SelfPacer) Recommend(ctx context.Context, perf StudentPerformance) (string, error) {
	prompt := buildSelfPacerPrompt(perf)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: selfPacerPersona},
		{Role: llm.RoleUser, Content: prompt},
	}

	resp, err := sp.backend.Chat(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("self-pacer: %w", err)
	}
	return resp.Content, nil
}

func buildSelfPacerPrompt(perf StudentPerformance) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Student: %s (Grade Level: %s)\n\n", perf.StudentName, perf.GradeLevel)

	b.WriteString("Completed Topics:\n")
	if len(perf.CompletedTopics) == 0 {
		b.WriteString("  (none yet)\n")
	}
	for _, tp := range perf.CompletedTopics {
		grade := "not graded"
		if tp.AssignmentGrade != nil {
			grade = fmt.Sprintf("%.0f/100", *tp.AssignmentGrade)
		}
		fmt.Fprintf(&b, "  - %s: grade=%s, AI interactions=%d, attendance=%d/%d\n",
			tp.TopicTitle, grade, tp.AIInteractions, tp.SessionsAttended, tp.SessionsAvailable)
	}

	b.WriteString("\nAvailable Topics (in order):\n")
	for _, at := range perf.AvailableTopics {
		fmt.Fprintf(&b, "  - %s (id: %s)\n", at.TopicTitle, at.TopicID)
	}

	b.WriteString("\nWhat should this student do next?")
	return b.String()
}
```

**File:** `gobackend/internal/agents/self_pacer_test.go`

```go
package agents

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelfPacer_Recommend(t *testing.T) {
	mock := &mockBackend{response: `{"recommended_action":"advance","topic_id":"topic-2","topic_title":"For Loops","reasoning":"Student scored 95% on Variables.","confidence":0.9}`}
	sp := NewSelfPacer(mock)

	grade := float64(95)
	resp, err := sp.Recommend(context.Background(), StudentPerformance{
		StudentID:   "student-1",
		StudentName: "Alice",
		GradeLevel:  Grade68,
		CompletedTopics: []TopicPerformance{
			{TopicID: "topic-1", TopicTitle: "Variables", AssignmentGrade: &grade, AIInteractions: 3, SessionsAttended: 2, SessionsAvailable: 2},
		},
		AvailableTopics: []AvailableTopic{
			{TopicID: "topic-2", TopicTitle: "For Loops", SortOrder: 2},
			{TopicID: "topic-3", TopicTitle: "Functions", SortOrder: 3},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, resp, "advance")
	assert.Contains(t, resp, "topic-2")
}

func TestSelfPacer_RecommendReview(t *testing.T) {
	mock := &mockBackend{response: `{"recommended_action":"review","topic_id":"topic-1","topic_title":"Variables","reasoning":"Student scored 45%.","confidence":0.85}`}
	sp := NewSelfPacer(mock)

	grade := float64(45)
	resp, err := sp.Recommend(context.Background(), StudentPerformance{
		StudentID:   "student-2",
		StudentName: "Bob",
		GradeLevel:  GradeK5,
		CompletedTopics: []TopicPerformance{
			{TopicID: "topic-1", TopicTitle: "Variables", AssignmentGrade: &grade, AIInteractions: 10, SessionsAttended: 1, SessionsAvailable: 3},
		},
		AvailableTopics: []AvailableTopic{
			{TopicID: "topic-2", TopicTitle: "For Loops", SortOrder: 2},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, resp, "review")
}

func TestBuildSelfPacerPrompt(t *testing.T) {
	grade := float64(80)
	prompt := buildSelfPacerPrompt(StudentPerformance{
		StudentName: "Carol",
		GradeLevel:  Grade912,
		CompletedTopics: []TopicPerformance{
			{TopicTitle: "Variables", AssignmentGrade: &grade, AIInteractions: 5, SessionsAttended: 3, SessionsAvailable: 3},
		},
		AvailableTopics: []AvailableTopic{
			{TopicID: "t-2", TopicTitle: "Loops"},
		},
	})
	assert.Contains(t, prompt, "Carol (Grade Level: 9-12)")
	assert.Contains(t, prompt, "Variables: grade=80/100")
	assert.Contains(t, prompt, "Loops (id: t-2)")
}

func TestBuildSelfPacerPrompt_NoCompletedTopics(t *testing.T) {
	prompt := buildSelfPacerPrompt(StudentPerformance{
		StudentName: "Dave",
		GradeLevel:  GradeK5,
		AvailableTopics: []AvailableTopic{
			{TopicID: "t-1", TopicTitle: "Hello World"},
		},
	})
	assert.Contains(t, prompt, "(none yet)")
	assert.Contains(t, prompt, "Hello World")
}
```

### 7d. Content Creator Agent (`internal/agents/content_creator.go`)

The content creator agent generates lesson content (explanations, exercises, starter code) for a topic. This is used as a workflow step in the AIGC content pipeline.

**File:** `gobackend/internal/agents/content_creator.go`

```go
package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/bridge-edu/bridge/gobackend/internal/llm"
)

const contentCreatorPersona = `You are an expert K-12 computer science curriculum designer.
Your role is to create engaging, age-appropriate lesson content for coding education.

GUIDELINES:
- Match vocabulary and complexity to the grade level
- Include clear explanations with relatable examples
- Create exercises that build progressively
- Provide starter code that scaffolds learning (partial implementation for students to complete)
- Include comments in starter code that guide the student
- For K-5: use visual/block-based concepts, keep code very short
- For 6-8: introduce debugging, use real-world examples
- For 9-12: include best practices, edge cases, computational thinking

Respond in JSON format:
{
  "explanation": "<lesson explanation in markdown>",
  "exercises": [
    {
      "title": "<exercise title>",
      "description": "<what the student should do>",
      "starter_code": "<partial code for student to complete>",
      "difficulty": "easy" | "medium" | "hard"
    }
  ],
  "key_concepts": ["<concept1>", "<concept2>"],
  "teacher_notes": "<optional notes for the teacher>"
}`

// ContentRequest specifies what lesson content to generate.
type ContentRequest struct {
	TopicTitle       string
	TopicDescription string
	GradeLevel       GradeLevel
	Language         string // programming language
	CourseContext     string // what the course is about
	PriorTopics      []string
}

// ContentCreator generates lesson content for topics.
type ContentCreator struct {
	backend llm.Backend
}

// NewContentCreator creates a new ContentCreator.
func NewContentCreator(backend llm.Backend) *ContentCreator {
	return &ContentCreator{backend: backend}
}

// Generate creates lesson content for a topic.
func (cc *ContentCreator) Generate(ctx context.Context, req ContentRequest) (string, error) {
	prompt := buildContentPrompt(req)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: contentCreatorPersona},
		{Role: llm.RoleUser, Content: prompt},
	}

	resp, err := cc.backend.Chat(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("content creator: %w", err)
	}
	return resp.Content, nil
}

func buildContentPrompt(req ContentRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Create lesson content for the following topic:\n\n")
	fmt.Fprintf(&b, "Topic: %s\n", req.TopicTitle)
	if req.TopicDescription != "" {
		fmt.Fprintf(&b, "Description: %s\n", req.TopicDescription)
	}
	fmt.Fprintf(&b, "Grade Level: %s\n", req.GradeLevel)
	fmt.Fprintf(&b, "Programming Language: %s\n", req.Language)
	if req.CourseContext != "" {
		fmt.Fprintf(&b, "Course Context: %s\n", req.CourseContext)
	}

	if len(req.PriorTopics) > 0 {
		fmt.Fprintf(&b, "\nStudents have already completed these topics:\n")
		for _, t := range req.PriorTopics {
			fmt.Fprintf(&b, "  - %s\n", t)
		}
	}

	b.WriteString("\nGenerate the lesson content including explanation, exercises with starter code, and key concepts.")
	return b.String()
}
```

**File:** `gobackend/internal/agents/content_creator_test.go`

```go
package agents

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentCreator_Generate(t *testing.T) {
	mock := &mockBackend{response: `{"explanation":"# For Loops\nFor loops repeat...","exercises":[{"title":"Count to 10"}],"key_concepts":["iteration","range"]}`}
	cc := NewContentCreator(mock)

	resp, err := cc.Generate(context.Background(), ContentRequest{
		TopicTitle:   "For Loops",
		GradeLevel:   Grade68,
		Language:     "python",
		PriorTopics:  []string{"Variables", "Conditionals"},
	})
	require.NoError(t, err)
	assert.Contains(t, resp, "For Loops")
	assert.Contains(t, resp, "iteration")

	// Verify system prompt
	require.Len(t, mock.calls, 1)
	assert.Contains(t, mock.calls[0][0].Text(), "curriculum designer")
}

func TestBuildContentPrompt(t *testing.T) {
	prompt := buildContentPrompt(ContentRequest{
		TopicTitle:       "Functions",
		TopicDescription: "Learn to define and call functions",
		GradeLevel:       Grade912,
		Language:         "javascript",
		CourseContext:     "Intro to Web Development",
		PriorTopics:      []string{"Variables", "Loops"},
	})
	assert.Contains(t, prompt, "Functions")
	assert.Contains(t, prompt, "javascript")
	assert.Contains(t, prompt, "9-12")
	assert.Contains(t, prompt, "Web Development")
	assert.Contains(t, prompt, "Variables")
	assert.Contains(t, prompt, "Loops")
}

func TestBuildContentPrompt_Minimal(t *testing.T) {
	prompt := buildContentPrompt(ContentRequest{
		TopicTitle: "Hello World",
		GradeLevel: GradeK5,
		Language:   "blockly",
	})
	assert.Contains(t, prompt, "Hello World")
	assert.Contains(t, prompt, "K-5")
	assert.NotContains(t, prompt, "already completed")
}
```

### 7e. Test Helpers for Agents (`internal/agents/mock_test.go`)

**File:** `gobackend/internal/agents/mock_test.go`

```go
package agents

import (
	"context"

	"github.com/bridge-edu/bridge/gobackend/internal/llm"
)

// mockBackend is a test double for llm.Backend that returns canned responses.
type mockBackend struct {
	response string
	calls    [][]llm.Message // record all calls
}

func (m *mockBackend) Name() string { return "mock" }

func (m *mockBackend) Chat(_ context.Context, messages []llm.Message, _ ...llm.ChatOption) (*llm.LLMResponse, error) {
	m.calls = append(m.calls, messages)
	return &llm.LLMResponse{Content: m.response}, nil
}

func (m *mockBackend) StreamChat(_ context.Context, _ []llm.Message, _ ...llm.ChatOption) (<-chan llm.StreamChunk, error) {
	return nil, nil
}

func (m *mockBackend) ChatWithTools(_ context.Context, messages []llm.Message, _ []llm.ToolSpec, _ ...llm.ChatOption) (*llm.LLMResponse, error) {
	return m.Chat(context.Background(), messages)
}

func (m *mockBackend) SupportsTools() bool { return false }

func (m *mockBackend) ListModels(_ context.Context) ([]string, error) { return nil, nil }
```

---

## Task 8 — Engine Entry Point (`cmd/engine/main.go`)

The standalone engine process polls for pending runs, claims them atomically, executes DAGs, handles crash recovery, and evaluates cron schedules. Adapted from magicburg's `cmd/engine/main.go`.

**File:** `gobackend/cmd/engine/main.go`

```go
// Command engine runs the Bridge workflow engine service.
//
// The engine polls for pending workflow runs, claims them atomically,
// executes DAG steps, handles crash recovery, and evaluates cron schedules.
//
// Run with:
//
//	go run ./cmd/engine
//	go run ./cmd/engine --poll-interval 10 --max-concurrent 5
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bridge-edu/bridge/gobackend/internal/config"
	"github.com/bridge-edu/bridge/gobackend/internal/events"
	"github.com/bridge-edu/bridge/gobackend/internal/llm"
	"github.com/bridge-edu/bridge/gobackend/internal/workflows"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	pollInterval := flag.Int("poll-interval", 30, "seconds between polls")
	maxConcurrent := flag.Int("max-concurrent", 3, "max concurrent runs")
	flag.Parse()

	slog.Info("Starting Bridge Engine",
		"poll_interval", *pollInterval,
		"max_concurrent", *maxConcurrent,
	)

	// Load config
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize database pool
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		slog.Error("Failed to create database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := workflows.NewWorkflowStore(pool)

	// Create LLM runner and status emitter
	llmRunner := &EngineLLMRunner{}
	broadcaster := events.NewBroadcaster(nil) // engine does not serve SSE; nil DB is safe for broadcast-only
	emitter := workflows.NewBroadcastEmitter(broadcaster)

	worker := &EngineWorker{
		store:         store,
		llm:           llmRunner,
		emitter:       emitter,
		pollInterval:  time.Duration(*pollInterval) * time.Second,
		maxConcurrent: *maxConcurrent,
		running:       make(map[string]context.CancelFunc),
	}

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-done
		slog.Info("Shutdown signal received")
		cancel()
	}()

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		slog.Error("Engine worker failed", "error", err)
		os.Exit(1)
	}
	slog.Info("Engine stopped")
}

// EngineWorker polls for pending workflow runs and executes them.
type EngineWorker struct {
	store         *workflows.WorkflowStore
	llm           workflows.LLMRunner
	emitter       workflows.StatusEmitter
	pollInterval  time.Duration
	maxConcurrent int
	running       map[string]context.CancelFunc
	mu            sync.Mutex
}

// Run is the main loop.
func (w *EngineWorker) Run(ctx context.Context) error {
	slog.Info("Engine worker starting",
		"poll_interval", w.pollInterval,
		"max_concurrent", w.maxConcurrent,
	)

	if err := w.recoverOrphanedRuns(ctx); err != nil {
		slog.Error("Error recovering orphaned runs", "error", err)
	}

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	if err := w.pollCycle(ctx); err != nil {
		slog.Error("Error in poll cycle", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("Waiting for running tasks to finish...")
			w.waitForRunning()
			return nil
		case <-ticker.C:
			if err := w.pollCycle(ctx); err != nil {
				slog.Error("Error in poll cycle", "error", err)
			}
		}
	}
}

// recoverOrphanedRuns resets runs stuck in "running" after a crash.
func (w *EngineWorker) recoverOrphanedRuns(ctx context.Context) error {
	orphaned, err := w.store.FindOrphanedRuns(ctx)
	if err != nil {
		return fmt.Errorf("find orphaned runs: %w", err)
	}

	for _, run := range orphaned {
		runID, _ := run["run_id"].(string)
		slog.Warn("Recovering orphaned run", "run_id", runID)
		if err := w.store.UpdateRun(ctx, runID, map[string]any{"status": "pending"}); err != nil {
			slog.Error("Failed to recover orphaned run", "run_id", runID, "error", err)
		}
	}

	if len(orphaned) > 0 {
		slog.Info("Recovered orphaned runs", "count", len(orphaned))
	}
	return nil
}

// pollCycle performs one poll iteration.
func (w *EngineWorker) pollCycle(ctx context.Context) error {
	if err := w.checkSchedules(ctx); err != nil {
		slog.Error("Error checking schedules", "error", err)
	}

	w.mu.Lock()
	available := w.maxConcurrent - len(w.running)
	w.mu.Unlock()

	if available <= 0 {
		return nil
	}

	pending, err := w.store.ClaimPendingRuns(ctx, available)
	if err != nil {
		return fmt.Errorf("claim pending runs: %w", err)
	}

	for _, run := range pending {
		runID, _ := run["run_id"].(string)
		workflowID, _ := run["workflow_id"].(string)
		dagVersion, _ := run["dag_version"]

		slog.Info("Claimed run",
			"run_id", runID,
			"workflow_id", workflowID,
			"dag_version", dagVersion,
		)

		runCtx, cancelFn := context.WithCancel(ctx)
		w.mu.Lock()
		w.running[runID] = cancelFn
		w.mu.Unlock()

		go func(rID string) {
			defer func() {
				w.mu.Lock()
				delete(w.running, rID)
				w.mu.Unlock()
			}()
			if err := w.executeRun(runCtx, run); err != nil {
				slog.Error("Run failed", "run_id", rID, "error", err)
			}
		}(runID)

		available--
		if available <= 0 {
			break
		}
	}

	// Check for resumable runs
	if available > 0 {
		resumable, err := w.store.FindResumableRuns(ctx, available)
		if err != nil {
			slog.Error("Error finding resumable runs", "error", err)
		} else {
			for _, run := range resumable {
				runID, _ := run["run_id"].(string)

				w.mu.Lock()
				_, alreadyRunning := w.running[runID]
				w.mu.Unlock()

				if alreadyRunning {
					continue
				}

				slog.Info("Resuming run", "run_id", runID)

				runCtx, cancelFn := context.WithCancel(ctx)
				w.mu.Lock()
				w.running[runID] = cancelFn
				w.mu.Unlock()

				go func(rID string) {
					defer func() {
						w.mu.Lock()
						delete(w.running, rID)
						w.mu.Unlock()
					}()
					if err := w.executeRun(runCtx, run); err != nil {
						slog.Error("Run failed", "run_id", rID, "error", err)
					}
				}(runID)
			}
		}
	}

	return nil
}

// executeRun executes a single workflow run.
func (w *EngineWorker) executeRun(ctx context.Context, run map[string]any) error {
	executor := workflows.NewRunExecutor(w.store, w.llm, w.emitter)
	return executor.Execute(ctx, run)
}

// EngineLLMRunner implements workflows.LLMRunner.
type EngineLLMRunner struct {
	once    sync.Once
	backend llm.Backend
	err     error
}

func (r *EngineLLMRunner) RunAgent(ctx context.Context, task, persona, sessionID, userID string) (string, error) {
	r.once.Do(func() {
		cfg := config.GetLLMConfig(config.Chat)
		r.backend, r.err = llm.CreateBackend(llm.LLMConfig{
			Backend:     cfg.Backend,
			Model:       cfg.Model,
			APIKey:      cfg.APIKey,
			BaseURL:     cfg.BaseURL,
			Temperature: cfg.Temperature,
			MaxTokens:   cfg.MaxTokens,
		})
	})
	if r.err != nil {
		return "", fmt.Errorf("create llm backend: %w", r.err)
	}
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: persona},
		{Role: llm.RoleUser, Content: task},
	}
	resp, err := r.backend.Chat(ctx, messages)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (r *EngineLLMRunner) RunAgentWithTools(ctx context.Context, task, persona, sessionID, userID string, tools []llm.ToolSpec, executor llm.ToolExecutor) (string, int, error) {
	r.once.Do(func() {
		cfg := config.GetLLMConfig(config.Chat)
		r.backend, r.err = llm.CreateBackend(llm.LLMConfig{
			Backend:     cfg.Backend,
			Model:       cfg.Model,
			APIKey:      cfg.APIKey,
			BaseURL:     cfg.BaseURL,
			Temperature: cfg.Temperature,
			MaxTokens:   cfg.MaxTokens,
		})
	})
	if r.err != nil {
		return "", 0, fmt.Errorf("create llm backend: %w", r.err)
	}
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: persona},
		{Role: llm.RoleUser, Content: task},
	}
	return llm.RunAgenticLoop(ctx, llm.AgenticLoopConfig{
		MaxIterations: 15,
		Backend:       r.backend,
		Tools:         tools,
		ToolExecutor:  executor,
	}, messages)
}

// checkSchedules evaluates cron schedules and creates runs for due ones.
func (w *EngineWorker) checkSchedules(ctx context.Context) error {
	schedules, err := w.store.ListActiveSchedules(ctx)
	if err != nil {
		return fmt.Errorf("list active schedules: %w", err)
	}

	now := time.Now().UTC()
	nowISO := now.Format(time.RFC3339)

	for _, sched := range schedules {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		scheduleID, _ := sched["schedule_id"].(string)
		mode, _ := sched["mode"].(string)
		workflowID, _ := sched["workflow_id"].(string)
		lastRunAt, _ := sched["last_run_at"].(string)
		schedExpr, _ := sched["schedule"].(string)
		startAt, _ := sched["start_at"].(string)
		endAt, _ := sched["end_at"].(string)
		dagVersion, _ := sched["dag_version"].(int)
		catchUp, _ := sched["catch_up"].(bool)

		// Check end_at: if past, mark completed
		if endAt != "" {
			endTime, err := time.Parse(time.RFC3339, endAt)
			if err == nil && now.After(endTime) {
				_ = w.store.UpdateScheduleStatus(ctx, scheduleID, "completed")
				slog.Info("Schedule ended", "schedule_id", scheduleID)
				continue
			}
		}

		switch mode {
		case "oneshot":
			if lastRunAt == "" {
				runID, err := w.store.CreateRun(ctx, workflowID, dagVersion,
					workflows.WithScheduleID(scheduleID),
					workflows.WithTriggerSource("schedule"),
					workflows.WithSlotTime(nowISO),
				)
				if err != nil {
					slog.Error("Failed to create oneshot run", "schedule_id", scheduleID, "error", err)
					continue
				}
				_ = w.store.UpdateScheduleLastRun(ctx, scheduleID, nowISO, "completed")
				slog.Info("Oneshot schedule fired", "schedule_id", scheduleID, "run_id", runID)
			}

		case "scheduled":
			if schedExpr == "" {
				continue
			}

			var reference time.Time
			if lastRunAt != "" {
				reference, _ = time.Parse(time.RFC3339, lastRunAt)
			} else if startAt != "" {
				reference, _ = time.Parse(time.RFC3339, startAt)
			} else {
				reference = now.Add(-w.pollInterval)
			}

			end := now
			if endAt != "" {
				endTime, _ := time.Parse(time.RFC3339, endAt)
				if endTime.Before(end) {
					end = endTime
				}
			}

			if catchUp {
				ticks, err := workflows.TicksBetween(schedExpr, reference, end)
				if err != nil {
					slog.Error("Invalid cron expression", "schedule_id", scheduleID, "error", err)
					continue
				}
				for _, tick := range ticks {
					tickISO := tick.Format(time.RFC3339)
					runID, err := w.store.CreateRun(ctx, workflowID, dagVersion,
						workflows.WithScheduleID(scheduleID),
						workflows.WithTriggerSource("schedule"),
						workflows.WithSlotTime(tickISO),
					)
					if err != nil {
						slog.Error("Failed to create catch-up run", "schedule_id", scheduleID, "error", err)
						continue
					}
					_ = w.store.UpdateScheduleLastRun(ctx, scheduleID, tickISO)
					slog.Info("Catch-up run created", "schedule_id", scheduleID, "run_id", runID)
				}
			} else {
				nextTick, err := workflows.NextTick(schedExpr, reference)
				if err != nil {
					slog.Error("Invalid cron expression", "schedule_id", scheduleID, "error", err)
					continue
				}
				if !nextTick.After(end) {
					tickISO := nextTick.Format(time.RFC3339)
					runID, err := w.store.CreateRun(ctx, workflowID, dagVersion,
						workflows.WithScheduleID(scheduleID),
						workflows.WithTriggerSource("schedule"),
						workflows.WithSlotTime(tickISO),
					)
					if err != nil {
						slog.Error("Failed to create scheduled run", "schedule_id", scheduleID, "error", err)
						continue
					}
					_ = w.store.UpdateScheduleLastRun(ctx, scheduleID, tickISO)
					slog.Info("Scheduled run created", "schedule_id", scheduleID, "run_id", runID)
				}
			}
		}
	}
	return nil
}

// waitForRunning waits for all currently running tasks to complete.
func (w *EngineWorker) waitForRunning() {
	w.mu.Lock()
	count := len(w.running)
	for _, cancelFn := range w.running {
		cancelFn()
	}
	w.mu.Unlock()

	if count == 0 {
		return
	}

	slog.Info("Waiting for running tasks", "count", count)

	deadline := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			slog.Warn("Timed out waiting for running tasks")
			return
		case <-ticker.C:
			w.mu.Lock()
			remaining := len(w.running)
			w.mu.Unlock()
			if remaining == 0 {
				slog.Info("All tasks finished")
				return
			}
		}
	}
}
```

---

## Task 9 — Background Job Workflows (DAG Definitions)

These are pre-built DAG templates for common Bridge background jobs. They are registered as workflow templates in the DB seeder or created via the API.

### 9a. Post-Session Report Generation

**File:** `gobackend/internal/agents/workflows.go`

```go
package agents

// PostSessionReportDAG returns a DAG for generating a post-session summary.
// Triggered when a teacher ends a live session.
func PostSessionReportDAG(sessionID, classID string) map[string]any {
	return map[string]any{
		"steps": []any{
			map[string]any{
				"id":          "gather_data",
				"name":        "Gather Session Data",
				"instruction": "Collect session participation data, student code snapshots, AI interaction counts, help requests, and error patterns for session " + sessionID,
				"actor":       "data_collector",
			},
			map[string]any{
				"id":          "analyze_patterns",
				"name":        "Analyze Learning Patterns",
				"instruction": "Analyze the session data to identify common errors, stuck points, engagement patterns, and learning outcomes.",
				"actor":       "analyst",
			},
			map[string]any{
				"id":          "generate_report",
				"name":        "Generate Teacher Report",
				"instruction": "Create a concise post-session report for the teacher covering: participation summary, common issues, standout moments, and suggestions for follow-up.",
				"actor":       "reporter",
			},
		},
		"edges": []any{
			map[string]any{"from": "gather_data", "to": "analyze_patterns"},
			map[string]any{"from": "analyze_patterns", "to": "generate_report"},
		},
		"actors": []any{
			map[string]any{"id": "data_collector", "persona": "You are a data collection agent that extracts structured information from educational session logs."},
			map[string]any{"id": "analyst", "persona": "You are a learning analytics specialist who identifies patterns in student behavior."},
			map[string]any{"id": "reporter", "persona": "You are a concise report writer for educators. Keep reports under 500 words."},
		},
	}
}

// WeeklyParentReportDAG returns a DAG for generating weekly parent reports.
// Scheduled as a cron job (Sundays at 6 PM).
func WeeklyParentReportDAG(studentID, classID string) map[string]any {
	return map[string]any{
		"steps": []any{
			map[string]any{
				"id":          "collect_progress",
				"name":        "Collect Weekly Progress",
				"instruction": "Gather this week's data for student " + studentID + " in class " + classID + ": sessions attended, topics covered, assignment grades, AI interactions, teacher annotations.",
			},
			map[string]any{
				"id":          "generate_report",
				"name":        "Generate Parent Report",
				"instruction": "Write a warm, encouraging parent-friendly progress report. Include: summary, attendance, progress, areas for growth, and teacher notes. Keep it under 300 words. Use the student's first name.",
				"actor":       "parent_reporter",
			},
		},
		"edges": []any{
			map[string]any{"from": "collect_progress", "to": "generate_report"},
		},
		"actors": []any{
			map[string]any{"id": "parent_reporter", "persona": "You are a helpful education assistant generating a weekly progress report for a parent. Write in a warm, encouraging, parent-friendly tone. Use simple language."},
		},
	}
}

// ContentGenerationDAG returns a DAG for the AIGC content pipeline.
// Used to generate lesson content for a new topic.
func ContentGenerationDAG(topicTitle, gradeLevel, language string) map[string]any {
	return map[string]any{
		"steps": []any{
			map[string]any{
				"id":          "research",
				"name":        "Research Topic",
				"instruction": "Research best practices for teaching '" + topicTitle + "' to " + gradeLevel + " students using " + language + ". Identify key concepts, common misconceptions, and effective teaching strategies.",
				"actor":       "researcher",
			},
			map[string]any{
				"id":          "create_explanation",
				"name":        "Create Explanation",
				"instruction": "Write a clear, age-appropriate explanation of the topic. Include relatable examples and analogies.",
				"actor":       "writer",
			},
			map[string]any{
				"id":          "create_exercises",
				"name":        "Create Exercises",
				"instruction": "Design 3 exercises of increasing difficulty (easy, medium, hard). Each exercise should include starter code with guiding comments. The starter code should compile/run but be incomplete.",
				"actor":       "writer",
			},
			map[string]any{
				"id":          "review",
				"name":        "Review Content",
				"instruction": "Review the explanation and exercises for accuracy, age-appropriateness, and pedagogical soundness. Suggest improvements.",
				"actor":       "reviewer",
				"config":      map[string]any{"approval": true},
			},
		},
		"edges": []any{
			map[string]any{"from": "research", "to": "create_explanation"},
			map[string]any{"from": "research", "to": "create_exercises"},
			map[string]any{"from": "create_explanation", "to": "review"},
			map[string]any{"from": "create_exercises", "to": "review"},
		},
		"actors": []any{
			map[string]any{"id": "researcher", "persona": "You are a computer science education researcher specializing in K-12 pedagogy."},
			map[string]any{"id": "writer", "persona": "You are an expert curriculum content writer for K-12 coding education. Match vocabulary to the grade level."},
			map[string]any{"id": "reviewer", "persona": "You are a senior curriculum reviewer. Check for accuracy, age-appropriateness, and alignment with CS education standards."},
		},
	}
}
```

**File:** `gobackend/internal/agents/workflows_test.go`

```go
package agents

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bridge-edu/bridge/gobackend/internal/workflows"
)

func TestPostSessionReportDAG(t *testing.T) {
	dag := PostSessionReportDAG("sess-123", "class-456")

	// Valid DAG
	errs := workflows.ValidateDAG(dag)
	require.Empty(t, errs, "DAG should be valid: %v", errs)

	// Should have 3 steps and 2 edges
	steps, _ := dag["steps"].([]any)
	edges, _ := dag["edges"].([]any)
	assert.Len(t, steps, 3)
	assert.Len(t, edges, 2)

	// Topological sort should produce 3 layers (linear)
	layers := workflows.TopologicalSort(dag)
	assert.Len(t, layers, 3)
}

func TestWeeklyParentReportDAG(t *testing.T) {
	dag := WeeklyParentReportDAG("student-1", "class-1")

	errs := workflows.ValidateDAG(dag)
	require.Empty(t, errs)

	steps, _ := dag["steps"].([]any)
	assert.Len(t, steps, 2)

	layers := workflows.TopologicalSort(dag)
	assert.Len(t, layers, 2)
}

func TestContentGenerationDAG(t *testing.T) {
	dag := ContentGenerationDAG("For Loops", "6-8", "python")

	errs := workflows.ValidateDAG(dag)
	require.Empty(t, errs)

	steps, _ := dag["steps"].([]any)
	assert.Len(t, steps, 4)

	// Parallel structure: research -> (explanation, exercises) -> review
	layers := workflows.TopologicalSort(dag)
	assert.Len(t, layers, 3)
	assert.Len(t, layers[0], 1) // research
	assert.Len(t, layers[1], 2) // explanation + exercises (parallel)
	assert.Len(t, layers[2], 1) // review

	// Review step should have approval gate
	stepMap := make(map[string]map[string]any)
	for _, s := range steps {
		step := s.(map[string]any)
		id := step["id"].(string)
		stepMap[id] = step
	}
	reviewConfig, _ := stepMap["review"]["config"].(map[string]any)
	assert.Equal(t, true, reviewConfig["approval"])
}
```

---

## Task 10 — Cron Schedule Seed Data

**File:** `gobackend/internal/agents/schedules.go`

```go
package agents

// PredefinedSchedules returns the cron expressions for Bridge's recurring workflows.
// These are registered when a class is created or when the admin enables them.

const (
	// WeeklyParentReportCron runs every Sunday at 6 PM UTC.
	WeeklyParentReportCron = "0 18 * * 0"

	// DailySelfPacerCron runs daily at 7 AM UTC to check if students need new recommendations.
	DailySelfPacerCron = "0 7 * * *"
)
```

---

## File Summary

| File | Action | Lines (approx) |
|------|--------|-----------------|
| `gobackend/migrations/007_workflow_tables.up.sql` | Create | 90 |
| `gobackend/migrations/007_workflow_tables.down.sql` | Create | 6 |
| `gobackend/internal/workflows/dag.go` | Create (copy) | 185 |
| `gobackend/internal/workflows/dag_test.go` | Create (copy) | 226 |
| `gobackend/internal/workflows/cron.go` | Create (copy) | 49 |
| `gobackend/internal/workflows/cron_test.go` | Create (copy) | 97 |
| `gobackend/internal/workflows/prompts.go` | Create (copy) | 64 |
| `gobackend/internal/workflows/prompts_test.go` | Create (copy) | 152 |
| `gobackend/internal/workflows/emitter.go` | Create (adapt) | 85 |
| `gobackend/internal/workflows/emitter_test.go` | Create | 25 |
| `gobackend/internal/workflows/store.go` | Create (adapt) | 400 |
| `gobackend/internal/workflows/executor.go` | Create (adapt) | 275 |
| `gobackend/internal/workflows/executor_test.go` | Create (adapt) | 200 |
| `gobackend/internal/agents/student_tutor.go` | Create | 110 |
| `gobackend/internal/agents/student_tutor_test.go` | Create | 85 |
| `gobackend/internal/agents/teacher_assistant.go` | Create | 115 |
| `gobackend/internal/agents/teacher_assistant_test.go` | Create | 65 |
| `gobackend/internal/agents/self_pacer.go` | Create | 100 |
| `gobackend/internal/agents/self_pacer_test.go` | Create | 70 |
| `gobackend/internal/agents/content_creator.go` | Create | 85 |
| `gobackend/internal/agents/content_creator_test.go` | Create | 55 |
| `gobackend/internal/agents/mock_test.go` | Create | 35 |
| `gobackend/internal/agents/workflows.go` | Create | 110 |
| `gobackend/internal/agents/workflows_test.go` | Create | 55 |
| `gobackend/internal/agents/schedules.go` | Create | 12 |
| `gobackend/cmd/engine/main.go` | Create (adapt) | 350 |
| **Total** | | **~3,150** |

---

## Execution Order

1. **Task 1** — DB migration (workflow tables)
2. **Task 2** — `dag.go` + tests (no dependencies, pure algorithms)
3. **Task 3** — `cron.go` + tests (no dependencies)
4. **Task 5** — `prompts.go` + `emitter.go` + tests
5. **Task 4** — `store.go` (depends on Task 1 migration)
6. **Task 6** — `executor.go` + tests (depends on Tasks 2-5)
7. **Task 7** — All four agents + tests (independent of each other, depend on LLM package)
8. **Task 9** — Background job DAG definitions + tests (depends on Tasks 2, 7)
9. **Task 10** — Schedule constants
10. **Task 8** — Engine entry point (depends on everything above)

Tasks 2, 3, 5, and 7a-7d can be parallelized.

---

## Verification

After all tasks are complete:

```bash
cd gobackend

# Run all workflow tests
go test ./internal/workflows/... -v -count=1

# Run all agent tests
go test ./internal/agents/... -v -count=1

# Verify engine compiles
go build ./cmd/engine

# Run migration on test database
go run ./cmd/api --migrate-only
```
