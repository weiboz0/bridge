# 023 — E2E Test Coverage Expansion

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Expand Playwright E2E tests to cover all 5 roles and newly added features — session flow, join class, scheduling, admin management, and parent portal.

**Architecture:** Build on existing E2E infrastructure (auth.setup.ts, helpers.ts). Add admin auth setup. Test through the browser against running Next.js + Go + Hocuspocus services.

**Tech Stack:** Playwright, TypeScript

**Branch:** `feat/023-e2e-coverage`

**Prerequisites:** All three services running — Next.js (port 3003), Go API (port 8002), Hocuspocus (port 4000)

---

## Current State

- 9 spec files, ~48 test cases
- 4 roles with auth setup (teacher, student, org_admin, parent)
- Admin role has NO auth setup or tests
- Core session flow (start → join → collaborate) has NO E2E tests
- Join class by code has NO E2E tests
- Session scheduling has NO E2E tests
- Admin user management is SKIPPED

---

### Task 1: Add Admin Auth Setup

**Files:**
- Modify: `e2e/auth.setup.ts`
- Modify: `e2e/helpers.ts`

Add admin (platform admin) account to auth setup. The admin account is `m2chrischou@gmail.com` — need a credentials-based admin for E2E. Create one if needed via the Go API, or use the existing admin with Google OAuth (which won't work in E2E).

**Approach:** Create a test admin user with credentials in the test DB, or use the existing admin impersonation flow.

- [ ] Add `admin` to ACCOUNTS in helpers.ts (use an existing platform admin with password, or create one)
- [ ] Add admin auth setup to auth.setup.ts
- [ ] Save to `e2e/.auth/admin.json`
- [ ] Commit

---

### Task 2: Session Flow E2E — Teacher Starts, Student Joins

**Files:**
- Create: `e2e/session-flow.spec.ts`

This is the most critical missing test. Tests the core platform flow:

- [ ] **Test: teacher can start a live session from class detail page**
  - Login as teacher, navigate to `/teacher/classes/{id}`
  - Click "Start Live Session"
  - Verify redirect to session dashboard
  - Verify "Live Session Active" indicator

- [ ] **Test: student sees active session on class page**
  - Login as student (enrolled in the same class)
  - Navigate to `/student/classes/{id}`
  - Verify "Live Session — Join Now" card is visible
  - Click to join session

- [ ] **Test: teacher can end a session**
  - Login as teacher
  - Navigate to session dashboard
  - End the session
  - Verify session status changes to ended

- [ ] **Test: teacher sees past sessions on class page**
  - Navigate to `/teacher/classes/{id}`
  - Verify past sessions list shows the ended session with duration and student count

- [ ] Commit

---

### Task 3: Join Class by Code E2E

**Files:**
- Create: `e2e/join-class.spec.ts`

- [ ] **Test: student can join a class with join code**
  - Login as teacher, get join code from class detail page
  - Login as student2 (who is not enrolled)
  - Navigate to `/student`
  - Click "Join a Class" button
  - Enter the join code
  - Click "Join"
  - Verify class appears in student's class list

- [ ] **Test: invalid join code shows error**
  - Login as student
  - Click "Join a Class"
  - Enter "INVALID1"
  - Verify error message

- [ ] Commit

---

### Task 4: Admin Portal E2E

**Files:**
- Create: `e2e/admin.spec.ts`

- [ ] **Test: admin can see platform stats**
  - Login as admin
  - Verify dashboard shows pending orgs, active orgs, total users counts

- [ ] **Test: admin can view org list**
  - Navigate to `/admin/orgs`
  - Verify table with organizations

- [ ] **Test: admin can view user list with dropdown actions**
  - Navigate to `/admin/users`
  - Verify user table loads
  - Verify action dropdown (MoreHorizontal button) is visible for non-self users

- [ ] **Test: admin can approve a pending org** (if test data has one)
  - Navigate to `/admin/orgs?status=pending`
  - Click approve action
  - Verify status changes to active

- [ ] Commit

---

### Task 5: Session Scheduling E2E

**Files:**
- Create: `e2e/scheduling.spec.ts`

- [ ] **Test: teacher can create a scheduled session via API**
  - POST to `/api/classes/{id}/schedule` with future date/time
  - Verify 201 response

- [ ] **Test: teacher can list upcoming sessions for a class via API**
  - GET `/api/classes/{id}/schedule/upcoming`
  - Verify response includes the created schedule

- [ ] **Test: teacher can start a session from a schedule entry via API**
  - POST to `/api/schedule/{id}/start`
  - Verify 201 response with live session

- [ ] **Test: teacher can cancel a scheduled session via API**
  - DELETE `/api/schedule/{id}`
  - Verify status changes to cancelled

- [ ] Commit

---

### Task 6: Parent Portal Deep Tests

**Files:**
- Modify: `e2e/help-queue.spec.ts` or create `e2e/parent.spec.ts`

- [ ] **Test: parent dashboard shows linked children**
  - Login as parent
  - Verify children list is visible

- [ ] **Test: parent can view child detail page**
  - Click on child name
  - Verify child profile page loads

- [ ] **Test: parent can view reports page**
  - Navigate to reports page
  - Verify page loads (may be empty)

- [ ] Commit

---

### Task 7: Course & Class CRUD Completion

**Files:**
- Modify: `e2e/courses.spec.ts`

- [ ] **Test: teacher can edit a topic** (if UI supports it)
- [ ] **Test: teacher can delete a topic** (if UI supports it)
- [ ] **Test: teacher can see class join code on class detail page**
  - Navigate to class detail
  - Verify join code is displayed (8-char code)

- [ ] Commit

---

### Task 8: Cross-Role Access Control

**Files:**
- Create: `e2e/access-control.spec.ts`

- [ ] **Test: student cannot access teacher portal**
  - Login as student, navigate to `/teacher`
  - Verify redirect to `/` or access denied

- [ ] **Test: teacher cannot access admin portal**
  - Login as teacher, navigate to `/admin`
  - Verify redirect to `/` or access denied

- [ ] **Test: unauthenticated user redirected to login**
  - Clear cookies, navigate to `/teacher`
  - Verify redirect to `/login`

- [ ] Commit

---

### Task 9: Final Verification

- [ ] Run full E2E suite: `bun run test:e2e`
- [ ] Verify all tests pass
- [ ] Count total test cases
- [ ] Push and create PR
