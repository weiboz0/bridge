# Session whiteboard review remediation

**Status:** Proposed

**Related plan:** `docs/plans/094-session-whiteboard.md`

## Goal

Resolve the open Plan 094 code-review findings without weakening the session lifecycle, authorization, or archive boundaries.

The session row in PostgreSQL remains authoritative for whether collaboration is live.

Hocuspocus may improve final whiteboard capture, but it must never prevent a teacher from ending a session.

Teachers receive an explicit warning when the end succeeds but the latest whiteboard state cannot be confirmed in the archive.

## Non-goals

- This remediation does not introduce a distributed transaction between Go, PostgreSQL, and Hocuspocus.
- This remediation does not add an intermediate `ending` session status or an asynchronous end-session job.
- This remediation does not guarantee capture of changes that exist only in an unavailable Hocuspocus process.
- This remediation does not persist Excalidraw binary files or images.
- This remediation does not grant platform administrators or impersonators a private-canvas bypass.
- This remediation does not add visibility tightening or per-canvas teacher moderation.
- This remediation does not cache mutation authorization in Hocuspocus because session status must remain authoritative for every accepted mutation.
- This remediation does not run Playwright against an unpinned or pre-existing service.

## Settled decisions

### Session status wins

The Go API asks Hocuspocus to freeze and flush the session's loaded canvases before updating the session row.

The freeze request has a strict two-second deadline.

The Go API ends the session even when Hocuspocus is unavailable, times out, rejects the request, or returns a malformed response.

A successful database transition is returned as a successful end-session response in all of those cases.

The response states whether every Yjs update accepted by the realtime server was confirmed in the archive.

The teacher sees `Session ended, but the latest whiteboard changes may not have been archived.` when completion was not confirmed.

If the database transition fails, the session remains live and the API releases its operation-owned freeze lease before asking Hocuspocus to unfreeze the matching operation.

### Least-privilege canvas access

Canvas creation is allowed only for the session teacher or a participant whose current status is `present`.

An authenticated outsider may read a `session`-visibility canvas while a public class-less session is live, but may not create a canvas.

Platform administrators and impersonators receive the same canvas authorization as the represented user.

They do not gain a private student-canvas oversight bypass.

### Deliberate shared Excalidraw state

The shared scene contains elements and a small allowlist of durable scene-level settings.

The first durable setting is `viewBackgroundColor`.

Viewport position, zoom, selection, active tool, collaborators, view mode, and other user-interface state remain local.

Image insertion, image paste, and file drop are disabled because Plan 094 does not persist Excalidraw files.

Remote scene application must not write the same scene back into Yjs.

Local scene writes are trailing-coalesced over 100 milliseconds and identical serialized scenes are skipped.

Read-only archive surfaces never write local scene changes.

### Teacher controls

The live teacher whiteboard panel exposes the session canvas floor.

The control uses a dedicated `PATCH /api/sessions/{sessionId}/canvas-settings` endpoint rather than the generic session settings route.

The floor remains limited to `private`, `host`, or `participants`.

Canvas owners must confirm irreversible raises to `participants` or `session` before the request is sent.

## End-session protocol

### Durable operation-owned freeze lease

Go creates a random freeze token and acquires an expiring lease on the session row before asking Hocuspocus to flush.

The session row stores `canvas_freeze_token` and `canvas_freeze_until`.

The lease is acquired under a session-row lock and only while the session is live.

An active lease makes a concurrent end request return the stable `session_end_in_progress` conflict rather than creating or clearing another operation's freeze.

An expired lease may be replaced by a new end request.

Canvas authentication and every mutation recheck treat an unexpired freeze lease as non-writable, so a Hocuspocus restart cannot reopen writes between freeze and database end.

The initial lease duration is 15 seconds, which bounds a live-room interruption if the Go request dies before end or cleanup.

The implementation must use a named constant and a controlled-clock test rather than scattering the duration.

Completing the database end or aborting the operation clears the lease only when its token matches.

Hocuspocus mirrors active operations in an in-memory map keyed by session ID with token and expiry, never a boolean set.

An unfreeze for one token cannot clear a newer or concurrent token.

Expired in-memory entries stop blocking mutations and are removed lazily or by a bounded timer.

Bridge currently supports one Hocuspocus process for realtime documents.

Running multiple Hocuspocus replicas is unsupported until a coordinated document fan-out and flush protocol exists, even though the durable lease prevents post-freeze writes across processes.

### Internal Hocuspocus API

Hocuspocus exposes two internal HTTP operations:

- `POST /internal/canvas-sessions/freeze`
- `POST /internal/canvas-sessions/unfreeze`

Both operations require `Authorization: Bearer <HOCUSPOCUS_CONTROL_SECRET>`.

Both operations accept a strict JSON body of `{ "sessionId": "<uuid>", "freezeToken": "<uuid>" }`.

Unknown fields, a missing session ID, and a malformed UUID are rejected.

The freeze success response is `{ "flushed": <number>, "closed": <number> }`.

The unfreeze success response is `{ "unfrozen": true }` when the matching operation was cleared or was already absent.

The Go client treats a non-2xx response, timeout, transport error, invalid JSON, missing field, negative count, or wrong field type as a freeze failure.

The Go client uses `HOCUSPOCUS_INTERNAL_URL` for the origin and injects its HTTP client for deterministic timeout tests.

`HOCUSPOCUS_INTERNAL_URL` defaults to `http://127.0.0.1:<HOCUSPOCUS_PORT>`, with the existing Hocuspocus default port used when `HOCUSPOCUS_PORT` is unset.

The control operations run on a separate internal HTTP listener and are never registered on the browser websocket listener.

The internal listener defaults to loopback and requires an explicit private-network binding when Go and Hocuspocus run on separate hosts or containers.

The operations use a distinct `HOCUSPOCUS_CONTROL_SECRET`, not the realtime JWT signing secret.

The bearer is compared in constant time after validating its expected encoding and length.

The internal URL, listener binding, port, and secret are server-only configuration and are documented separately from any browser websocket URL.

### Freeze behavior

Hocuspocus maintains the operation-owned in-memory freeze map described above.

The freeze operation validates that the supplied token owns an unexpired database lease, then installs the matching in-memory entry before awaiting document work.

For every currently loaded `canvas:{canvasId}` belonging to the session, the operation serializes the current Yjs document and persists it through a dedicated final-flush path.

The final-flush path is allowed while the PostgreSQL session row is still live with the matching lease and fails if the lease, canvas, or session no longer exists.

Each final flush runs through the same Hocuspocus document save mutex as ordinary and debounced persistence.

The mutex is held through encoding and database completion so an older in-flight store cannot commit after the final snapshot.

The freeze operation succeeds only after every loaded canvas has been persisted successfully.

After a successful flush, Hocuspocus closes all loaded canvas connections for the session and reports the flushed and closed counts.

If persistence fails, Hocuspocus returns a non-2xx response and leaves that operation frozen until Go ends the session, explicitly unfreezes the matching token, or the lease expires.

The unfreeze operation removes only an entry with the matching token and is idempotent.

A Hocuspocus restart may lose the in-memory map, but the durable unexpired lease rejects later authentication and mutation rechecks until Go ends the session or the lease expires.

### Mutation ordering

The canvas mutation guard checks the operation-owned freeze map before the authorization request and again after the authorization request returns.

A frame already awaiting authorization when freeze begins is rejected by the second frozen check.

A frame that has fully passed the second guard check must apply and relay synchronously before the freeze HTTP handler can run in the Node event loop.

No await, queued microtask, asynchronous logging, metric, or other yield is permitted between that check and completed Yjs apply and relay.

The subsequent flush therefore captures every mutation accepted before the freeze boundary.

No mutation accepted after the freeze boundary may apply, relay, or persist.

Mutation authorization remains uncached so a committed session end takes precedence over realtime availability and throughput.

Client-side 100-millisecond coalescing reduces mutation volume without weakening that server-side decision.

### Go end-session sequence

1. The existing handler authorizes the represented user as the session teacher under the existing tenancy rules.
2. Go acquires the operation-owned freeze lease or returns an idempotent ended response or stable in-progress conflict.
3. Go calls Hocuspocus freeze with the matching token and a two-second deadline, then records whether a valid success response was received.
4. Go executes the database end transition with `whiteboard_server_archive_complete` set from the freeze result, conditional on the matching lease token, regardless of the Hocuspocus result.
5. If the database transition succeeds, Go emits the existing session-ended event and schedules the existing completion work.
6. If the database transition fails, Go clears only its matching database lease, sends a best-effort token-matched unfreeze request, and returns the existing safe 500-class error.
7. A freeze or unfreeze failure is logged as an actionable operational event without tokens, document content, user content, or internal response bodies.

A freeze request that completes after Go's timeout may leave its Hocuspocus operation frozen only until token-matched cleanup or the lease expiry.

That state is correct when the database end succeeds because the session is ended.

If the database end subsequently fails, token-matched cleanup restores collaboration immediately when available and the lease expiry bounds the failure when cleanup is unavailable.

### Public end-session response

The response preserves the existing top-level session fields so current consumers do not need a structural migration.

It adds `whiteboardServerArchiveComplete: boolean`.

When the value is false, it also adds `warning: "whiteboard_server_archive_incomplete"`.

The field and warning refer only to updates accepted by the realtime server before the freeze boundary.

They do not claim that unsent browser state, including a scene still inside the 100-millisecond client coalescing window, was archived.

The warning code is stable and contains no internal failure detail.

The database end transition persists `whiteboard_server_archive_complete` on the session row so the incomplete result survives tabs, navigation failures, and later archive visits.

The teacher dashboard may store the warning code in `sessionStorage` immediately before navigating to `/sessions/{sessionId}/whiteboards` for prompt display.

The archive page consumes and removes that value once, then confirms the durable status through the teacher-authorized canvas-settings endpoint before rendering the approved warning message.

The warning is not placed in the URL.

If ending the session fails, the teacher remains on the live page and sees an actionable error rather than being redirected.

## Realtime connection lifetime

Canvas authentication places the verified JWT expiry and token read-only decision in the connection context.

Hocuspocus schedules a connection close at the JWT expiry and clears the timer when the connection closes earlier.

Expiry delays beyond Node's maximum timer duration are chained or clamped and rechecked, while already expired tokens close immediately.

The close applies to writable and read-only canvas connections.

A per-connection hook performs the current internal authorization check and applies its read-only decision before that connection is admitted to the document.

The design must not rely on `onLoadDocument` for this decision because Hocuspocus does not call it for every connection to an already loaded document.

A stale writable claim connecting after the session ends is therefore read-only immediately, rather than only after its first mutation.

The existing pre-apply mutation recheck and storage guard remain defense in depth.

## Canvas API corrections

The creation authorization check and session-row lock occur within the same transaction used to enforce the per-session cap.

The accepted creator matrix is:

| Requester | Live private/class session | Live public class-less session | Ended session |
|---|---:|---:|---:|
| Teacher | Allow | Allow | Deny |
| `present` participant | Allow | Allow | Deny |
| Invited or `left` participant | Deny | Deny | Deny |
| Authenticated outsider | Deny | Deny | Deny |
| Platform admin or impersonator | Apply represented-user row | Apply represented-user row | Deny |

Canvas listing removes its redundant session lookup while preserving the current live and archive authorization rules.

The owner foreign key keeps the existing data-retention behavior unless current schema inspection proves it conflicts with the repository's user-deletion contract.

The neutral session page redirects to the archive only when the API establishes an ended-session archive case.

A nonexistent session remains a 404.

## Schema cleanup

Before rewriting migration 0028, the remediation verifies from repository and remote history that it has not shipped to an environment governed by `main`.

Local test databases that already applied the feature-branch migration are disposable and must be recreated or reconciled only through the approved test-database workflow.

The unused `plain_text` column is then removed from migration 0028 rather than retained as dead schema.

The same migration adds nullable `sessions.canvas_freeze_token`, nullable `sessions.canvas_freeze_until`, and nullable `sessions.whiteboard_server_archive_complete` columns.

The archive-complete value is null until a session end executes the new protocol and is set atomically with `status = 'ended'`.

The Drizzle schema, Go store, startup probe sentinels, parity tests, and documentation are updated together.

No migration is run against a non-test database.

## Test contract

### Go integration and unit tests

- Freeze success ends the session and returns `whiteboardServerArchiveComplete: true` without a warning.
- Freeze timeout, transport failure, non-2xx response, malformed response, and internal-auth failure each still end the session and return the stable warning.
- Database end failure leaves the session live, clears only the matching lease, attempts token-matched unfreeze, returns an error, and emits no ended event.
- Overlapping end requests cannot clear each other's leases or turn a stale snapshot into a successful archive result.
- A timed-out freeze response arriving after cleanup cannot reinstall or prolong the cleared lease.
- A crashed end request stops blocking mutations after the controlled 15-second lease expiry.
- Teacher end authorization and cross-user isolation remain covered.
- Canvas creation covers teacher, present participant, invited participant, left participant, public outsider, platform admin, impersonator, ended session, and cap races.
- Floor mutation covers teacher authorization, the three allowed values, rejection of `session`, and the existing row-lock invariant.
- The exact Plan 094 mint and ended-mutation test names are present rather than being represented only by broad table tests.

### Hocuspocus tests

- Freeze authenticates the bearer secret and validates strict input.
- A mutation racing a successful freeze proves that the last accepted scene is persisted before connections close.
- A mutation awaiting authorization when freeze begins is rejected before Yjs apply and relay.
- No asynchronous boundary may be introduced between the second freeze check and Yjs apply and relay.
- A final flush waits for an already in-flight ordinary store and remains the last committed snapshot through the shared save mutex.
- A final-flush failure returns failure and keeps the session frozen until unfreeze.
- Unfreeze is token-scoped and idempotent, and stale unfreeze cannot clear a newer freeze.
- The maximum 50 loaded canvases complete the batched or parallelized final-flush path inside the two-second Go deadline under the approved representative test latency.
- Canvas connections close at JWT expiry under a controlled clock.
- A stale writable token joining an already loaded ended or frozen canvas is downgraded by the per-connection authorization hook.
- Missing or malformed internal authorization remains fail-closed.

### Frontend and binding tests

- Remote scene application produces no echo write.
- Identical local scenes are skipped.
- Bursts of local changes produce one trailing write after 100 milliseconds.
- Only elements and allowlisted durable state enter Yjs.
- Local viewport, zoom, selection, tool, collaborators, and view mode survive remote updates.
- Image insertion, clipboard images, and file drops are rejected with a visible explanation.
- The live teacher panel reads and updates the floor.
- Owner visibility changes require confirmation at the irreversible levels.
- Owner and viewer controls differ correctly.
- Teacher end failure remains in place with an error.
- Incomplete server-archive status survives the redirect, remains durably discoverable, and displays exactly once per archive visit.
- Missing sessions remain 404 instead of redirecting to the archive.

### Playwright contract

`e2e/session-whiteboard.spec.ts` covers teacher canvas creation, a visibility raise, an authorized viewer observing the scene, teacher session end, and the read-only archive.

The test also covers public-outside creation denial and the teacher-visible incomplete-archive warning through a controlled failure seam.

The spec may run only against a separately started Bridge stack with an explicit pinned `E2E_BASE_URL`.

It must not use the default port or any pre-existing service.

## Documentation corrections

The API documentation records the dedicated canvas-settings route, creator matrix, end response metadata, and the precise server-accepted-state archive semantics.

The architecture decisions document records that PostgreSQL session status outranks Hocuspocus availability and that no administrator bypass exists for private canvases.

The testing documentation uses the five explicit empty provider variables rather than the stale `/dev/null` shorthand.

The student-session effect dependencies, dead `plain_text` references, and other documentation drift identified in Plan-wide Review 1 are corrected in the same remediation plan.

## Acceptance criteria

- A teacher can end a session within the database request path even when Hocuspocus is unavailable.
- A teacher is warned when complete archival of server-accepted whiteboard state is not confirmed.
- A successful freeze persists every server-accepted loaded-canvas mutation before closing its connections.
- No canvas mutation accepted after the freeze boundary applies or relays.
- Established canvas connections terminate when their JWT expires.
- Public outsiders cannot consume the canvas cap.
- Realtime scene updates do not echo, do not share local UI state, and do not imply unsupported image persistence.
- The teacher can control the session floor from the live interface.
- Every open Plan-wide Review 1 finding is fixed or explicitly resolved with evidence before the code-review gate passes.
- The exact implementation commit passes `bash scripts/ci-local.sh` before merge.

## Permanent design-review gate

Every committed design spec under `docs/specs/**` must pass a design-review gate before its implementation plan is drafted or revised from it.

The two required reviewers are Codex `gpt-5.6-sol` at high reasoning effort and Claude Code `claude-fable-5`.

Both reviewers receive a read-only prompt and review the committed spec against current source, architecture decisions, and repository safeguards.

Both must return `APPROVE` or `APPROVE WITH NITS` with no open blockers for the gate to pass.

Review verdicts, findings, responses, and resolutions are recorded in the spec's `## Design Review` section with `[sol]` and `[fable]` source tags.

After a revision, only reviewers that requested changes must be re-dispatched, although either reviewer may be re-run when a revision materially changes an area they approved.

The gate allows at most five substantive verdict rounds.

A runtime, transport, authentication, quota, or empty-output failure does not consume a substantive round, but it blocks the gate until the required reviewer returns a verdict.

Unresolved findings after Round 5 trigger the existing hard-safeguard pause and cannot be overridden by autopilot.

A passing design gate authorizes plan drafting or revision, not implementation.

The plan-review gate remains separately required before implementation.

The canonical rule is mirrored in `AGENTS.md`, `docs/reviewers.md`, `docs/development-workflow.md`, and `docs/coding-agent.md` in the Plan 094 remediation scope before those governance files are changed.

## Design Review

### Round 1 — 2026-08-10

- `[OPEN]` `[sol][fable]` A global boolean freeze can be cleared by an overlapping end attempt, can reappear after a delayed request, and can strand a live session when best-effort unfreeze fails.
  The revision replaces it with a token-owned PostgreSQL lease, matching in-memory operation records, conditional cleanup, single-flight conflict behavior, and a 15-second expiry.
- `[OPEN]` `[fable]` A Hocuspocus restart after successful freeze but before database end can admit new writes while Go still reports archive success.
  The durable lease is now checked by connection authorization and every mutation recheck, so process restart cannot erase the freeze boundary.
- `[OPEN]` `[sol]` A final flush can be overwritten by an older ordinary store completing later.
  The final path now shares the document save mutex and holds it through the database write.
- `[OPEN]` `[sol][fable]` The prior archive-complete field overstated what could be known while clients coalesce unsent scenes.
  The contract is narrowed and renamed to `whiteboardServerArchiveComplete`, which covers only Yjs updates accepted by the server.
- `[OPEN]` `[sol]` `onLoadDocument` does not run for every connection to an already loaded document.
  Current read-only authorization now occurs in a per-connection hook.
- `[OPEN]` `[fable]` Archive incompleteness was transient browser state.
  The result is now persisted atomically on the session row and exposed to the teacher archive.
- `[OPEN]` `[fable]` The control API reused the signing secret and had no separate listener requirement.
  The revision specifies a distinct control secret, constant-time comparison, and an internal-only listener.
- `[OPEN]` `[fable]` The no-yield ordering assumption, fixed deadline load, timer overflow, multi-process assumption, and migration-shipping check were underspecified.
  The revision makes the no-yield and single-process invariants explicit, adds timer and migration checks, and retains the teacher-selected two-second bound with maximum-cap test evidence required before approval.

**Round 1 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.
