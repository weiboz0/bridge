# 007 — Problem Editor UI

## Problem

Spec 006 defined Problems, TestCases, and Attempts. This spec covers the UI that surfaces them: the student's three-pane Problem page, the teacher's live-watch page, and the routing + presence model that makes the two stay in sync during a live session.

## Design

### Routes

```
/student/classes/{classId}/problems/{problemId}
/student/classes/{classId}/problems/{problemId}/attempts/{attemptId}
```

Both resolve to the same page. The first opens the most recently updated Attempt (the "active" one). The second opens a specific Attempt — used when a teacher shares a link to a student's work, or when the student bookmarks a specific version. The only difference is which Attempt the editor loads first; in-page attempt switching does not change the URL.

```
/teacher/classes/{classId}/problems/{problemId}/students/{studentId}
```

Teacher's live-watch view. Out-of-session review and in-session observation use the same route; live-session context (if any) is inferred from the student's presence, not the URL.

### Student page layout

Three panes in a 32/42/26 split (min-widths: 360 / 0 / 320):

- **Left — Problem.** Markdown description, example test cases rendered inline as labeled Input / Output blocks, a count of hidden cases ("2 more hidden test cases run on Test"), and a collapsible "My Test Cases" section where the student authors private cases.
- **Center — Editor.** Monaco editor with a header row: current Attempt title + "attempt N of M" + switcher + "New attempt" + "Test" + "Run". Below the editor, a thin chip strip shows the last few Attempts with status dots for quick switching. An "autosaved · 2s ago" indicator sits at the far right.
- **Right — Inputs + Terminal.** Inputs panel on top (short), Terminal below (fills remaining height).

Visual direction matches the committed mock at `/design/problem-student`.

### Teacher page layout

Three panes in a 26/48/26 split:

- **Left — Brief Problem.** Description only, plus a stats strip (example count, hidden count, language, class median time-to-pass) and a card showing the student being watched with a live indicator.
- **Center — Attempts.** A row of up to 3 Attempt cards across the top (most recent first), each showing `#N`, title, preview of the first 3 lines of code, status, and relative timestamp. Below the cards: the active attempt's code in a read-only Monaco instance.
- **Right — Terminal.** Shows the student's last run output, followed by a summary card for the most recent Test run (pass/fail per example case and hidden count).

The teacher never sees: the student's private test cases, the student's Inputs panel, or the "Run" / "Test" buttons. The view is strictly read-only.

### Attempts UX

- **First edit creates the Attempt.** Opening a Problem with no prior work shows the problem's `starter_code` in the editor; the Attempt row is POSTed only on the first real keystroke. Browsing a problem does not pollute the attempt history.
- **Autosave is debounced.** 500ms after the last keystroke, the current Attempt's `plain_text` is PATCHed. `updated_at` advances every save; that timestamp is the source of truth for "active attempt" and for teacher-visible sorting.
- **"New Attempt" button.** Creates a new Attempt seeded with the *current* editor contents (not the starter code) so the student can branch off a working idea. The previous Attempt remains untouched. The new Attempt becomes active immediately.
- **Switcher.** A dropdown shows all of the student's Attempts for this Problem with title, `updated_at`, and pass/fail status from the last Test run. Selecting an Attempt loads it into the editor. The URL changes to the `.../attempts/{id}` form only if the student explicitly copies the link via a "share this attempt" affordance — default switching stays at the problem-level URL.
- **Titles.** New Attempts start as "Untitled"; an inline rename affordance in the header sets the title. Titles are for the student's own navigation; they are not visible to peers.

### Teacher live-follow (decision C from brainstorm)

- Default behavior: the teacher's view tracks whichever Problem + Attempt the student currently has open. If the student navigates to a different Problem or switches attempts, the teacher's view follows.
- Pin override: clicking a non-active Attempt card, or navigating via the teacher URL with an explicit attempt segment, freezes the view. A "follow live" button in the header resumes tracking.
- Presence is broadcast over the existing Hocuspocus awareness channel as `{ studentId, problemId, attemptId }`. No separate presence service needed.

### Yjs room keying

Each Attempt is its own Yjs document, keyed `attempt:{id}`. The student's editor connects read-write; the teacher connects read-only. Switching Attempts disconnects from the old room and connects to the new one. The in-page attempt chip strip preloads no rooms — only the currently displayed Attempt has a live connection.

Implication: the existing `documents` Yjs flow for scratch/class sessions continues unchanged. Problem work uses its own rooms.

### Test case UI details

- **Canonical examples** render inline in the problem description as two-column Input / Output blocks with a neutral "Example" tag. Students always see these.
- **Hidden cases** are not rendered — only a count. When a hidden case fails during Test, the terminal shows "Hidden case 3 failed · wrong output" with no case detail.
- **Private test cases** live in the collapsible "My Test Cases" section on the left pane. Each is editable (stdin + optional expected stdout). When selected in the Inputs panel, "Run" uses that case's stdin; expected output (if set) is compared in the terminal output.
- **Inputs chip strip** shows: the canonical examples (always), the student's private cases (if any), and a "Custom…" option that reveals a stdin textarea.

### Terminal behaviors (UI contract; spec 008 handles the execution)

- **Run** streams output character-by-character (or line-by-line for simpler backends). When the program calls `input()`, a blinking cursor appears on the current line; keystrokes flow back as stdin. Process exit shows `exited with code N · Xms`.
- **Test** runs all canonical cases (examples + hidden) headless with each case's stdin piped. The terminal renders a compact summary card per run with a row per case. Failed example cases can be expanded to show actual vs expected output; failed hidden cases show only the failure count.
- **Clear** resets the terminal. Output is not persisted across page reloads (the backend does not store run history for Run; it only stores the last Test summary per Attempt, used for the teacher view).
- **Running indicator** is the amber pulse used throughout the mock; done = no indicator.

### Empty states

- **No Problems under a Topic yet** — student class page shows the topic with "No problems yet" and no link.
- **Problem opened, no prior Attempt** — editor loads starter_code, Attempt switcher shows nothing, autosave creates the first Attempt on first keystroke.
- **Teacher opens watch page for a student with no Attempts** — attempt cards row renders a single placeholder "Not started yet · last seen on /problems list 2m ago" and the editor shows the problem's starter_code read-only.

### Access control (page-level)

- Student page: viewer must be the `studentId` in the URL, or the student is implicit (session user). Non-owner students get 404.
- Teacher watch page: viewer must share a class with the student (active class_membership where that class's course contains the topic containing the problem), or be a platform admin. Enforced server-side before render. Read-only at all times.

### Non-goals

- In-session chat or help-queue UI changes (existing flow unchanged).
- Multi-student comparison view for teachers (one student at a time).
- Submitting an Attempt to an Assignment — deferred to a follow-up spec once grading is in scope.
- Real-time pair programming (student ↔ student). Yjs rooms are single-writer on the student side.

## Rollout

Implementation plan will run in order:

1. Problem / TestCase / Attempt schema + Go store + API (from spec 006).
2. Student page + routes; Inputs/Terminal panel; attempts switcher; Monaco integration using existing Yjs plumbing, keyed on `attempt:{id}`.
3. Teacher watch page + live-follow presence over Hocuspocus awareness.
4. Interactive stdin — spec 008 defines the execution model; this spec only reserves the UI surface for it.
