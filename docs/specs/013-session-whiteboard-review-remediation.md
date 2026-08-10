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

The response states whether the whiteboard archive was confirmed complete.

The teacher sees `Session ended, but the latest whiteboard changes may not have been archived.` when completion was not confirmed.

If the database transition fails, the session remains live and the API asks Hocuspocus to unfreeze it on a best-effort basis.

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

### Internal Hocuspocus API

Hocuspocus exposes two internal HTTP operations:

- `POST /internal/canvas-sessions/freeze`
- `POST /internal/canvas-sessions/unfreeze`

Both operations require `Authorization: Bearer <HOCUSPOCUS_TOKEN_SECRET>`.

Both operations accept a strict JSON body of `{ "sessionId": "<uuid>" }`.

Unknown fields, a missing session ID, and a malformed UUID are rejected.

The freeze success response is `{ "flushed": <number>, "closed": <number> }`.

The unfreeze success response is `{ "unfrozen": true }`.

The Go client treats a non-2xx response, timeout, transport error, invalid JSON, missing field, negative count, or wrong field type as a freeze failure.

The Go client uses `HOCUSPOCUS_INTERNAL_URL` for the origin and injects its HTTP client for deterministic timeout tests.

`HOCUSPOCUS_INTERNAL_URL` defaults to `http://127.0.0.1:<HOCUSPOCUS_PORT>`, with the existing Hocuspocus default port used when `HOCUSPOCUS_PORT` is unset.

The internal URL is server-only configuration and is documented separately from any browser websocket URL.

### Freeze behavior

Hocuspocus maintains an in-memory set of frozen session IDs.

The freeze operation adds the session ID to that set before awaiting database or document work.

For every currently loaded `canvas:{canvasId}` belonging to the session, the operation serializes the current Yjs document and persists it through a dedicated final-flush path.

The final-flush path is allowed while the PostgreSQL session row is still live and fails if the canvas or session no longer exists.

The freeze operation succeeds only after every loaded canvas has been persisted successfully.

After a successful flush, Hocuspocus closes all loaded canvas connections for the session and reports the flushed and closed counts.

If persistence fails, Hocuspocus returns a non-2xx response and leaves the session frozen until Go ends the session or explicitly unfreezes it after a database failure.

The unfreeze operation removes the session ID from the in-memory set and is idempotent.

A Hocuspocus restart may lose the in-memory set, but the authoritative ended session row rejects later authentication, loading, storage, and mutation rechecks.

### Mutation ordering

The canvas mutation guard checks the frozen-session set before the authorization request and again after the authorization request returns.

A frame already awaiting authorization when freeze begins is rejected by the second frozen check.

A frame that has fully passed the guard applies synchronously before the freeze HTTP handler can run in the Node event loop.

The subsequent flush therefore captures every mutation accepted before the freeze boundary.

No mutation accepted after the freeze boundary may apply, relay, or persist.

Mutation authorization remains uncached so a committed session end takes precedence over realtime availability and throughput.

Client-side 100-millisecond coalescing reduces mutation volume without weakening that server-side decision.

### Go end-session sequence

1. The existing handler authorizes the represented user as the session teacher under the existing tenancy rules.
2. Go calls Hocuspocus freeze with a two-second deadline and records whether a valid success response was received.
3. Go executes the existing database end transition regardless of the freeze result.
4. If the database transition succeeds, Go emits the existing session-ended event and schedules the existing completion work.
5. If the database transition fails, Go sends a best-effort unfreeze request and returns the existing safe 500-class error.
6. A freeze failure is logged without tokens, document content, user content, or internal response bodies.

A freeze request that completes after Go's timeout may leave the Hocuspocus session frozen.

That state is correct when the database end succeeds because the session is ended.

If the database end subsequently fails, the explicit unfreeze request restores collaboration on a best-effort basis.

### Public end-session response

The response preserves the existing top-level session fields so current consumers do not need a structural migration.

It adds `whiteboardArchiveComplete: boolean`.

When the value is false, it also adds `warning: "whiteboard_archive_incomplete"`.

The warning code is stable and contains no internal failure detail.

The teacher dashboard stores the warning code in `sessionStorage` immediately before navigating to `/sessions/{sessionId}/whiteboards`.

The archive page consumes and removes that value once, then renders the approved warning message.

The warning is not placed in the URL.

If ending the session fails, the teacher remains on the live page and sees an actionable error rather than being redirected.

## Realtime connection lifetime

Canvas authentication places the verified JWT expiry and current read-only decision in the connection context.

Hocuspocus schedules a connection close at the JWT expiry and clears the timer when the connection closes earlier.

The close applies to writable and read-only canvas connections.

`onLoadDocument` applies the current internal authorization response to the connection's read-only configuration before returning the document.

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

Migration 0028 has not shipped outside this feature branch, so the unused `plain_text` column is removed from the migration rather than retained as dead schema.

The Drizzle schema, Go store, startup probe sentinels, parity tests, and documentation are updated together.

No migration is run against a non-test database.

## Test contract

### Go integration and unit tests

- Freeze success ends the session and returns `whiteboardArchiveComplete: true` without a warning.
- Freeze timeout, transport failure, non-2xx response, malformed response, and internal-auth failure each still end the session and return the stable warning.
- Database end failure leaves the session live, attempts unfreeze, returns an error, and emits no ended event.
- Teacher end authorization and cross-user isolation remain covered.
- Canvas creation covers teacher, present participant, invited participant, left participant, public outsider, platform admin, impersonator, ended session, and cap races.
- Floor mutation covers teacher authorization, the three allowed values, rejection of `session`, and the existing row-lock invariant.
- The exact Plan 094 mint and ended-mutation test names are present rather than being represented only by broad table tests.

### Hocuspocus tests

- Freeze authenticates the bearer secret and validates strict input.
- A mutation racing a successful freeze proves that the last accepted scene is persisted before connections close.
- A mutation awaiting authorization when freeze begins is rejected before Yjs apply and relay.
- A final-flush failure returns failure and keeps the session frozen until unfreeze.
- Unfreeze is idempotent.
- Canvas connections close at JWT expiry under a controlled clock.
- `onLoadDocument` applies the current read-only response.
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
- Incomplete archive status survives the redirect and displays exactly once.
- Missing sessions remain 404 instead of redirecting to the archive.

### Playwright contract

`e2e/session-whiteboard.spec.ts` covers teacher canvas creation, a visibility raise, an authorized viewer observing the scene, teacher session end, and the read-only archive.

The test also covers public-outside creation denial and the teacher-visible incomplete-archive warning through a controlled failure seam.

The spec may run only against a separately started Bridge stack with an explicit pinned `E2E_BASE_URL`.

It must not use the default port or any pre-existing service.

## Documentation corrections

The API documentation records the dedicated canvas-settings route, creator matrix, end response metadata, and best-effort archive semantics.

The architecture decisions document records that PostgreSQL session status outranks Hocuspocus availability and that no administrator bypass exists for private canvases.

The testing documentation uses the five explicit empty provider variables rather than the stale `/dev/null` shorthand.

The student-session effect dependencies, dead `plain_text` references, and other documentation drift identified in Plan-wide Review 1 are corrected in the same remediation plan.

## Acceptance criteria

- A teacher can end a session within the database request path even when Hocuspocus is unavailable.
- A teacher is warned when final whiteboard archive completeness is not confirmed.
- A successful freeze persists every accepted loaded-canvas mutation before closing its connections.
- No canvas mutation accepted after the freeze boundary applies or relays.
- Established canvas connections terminate when their JWT expires.
- Public outsiders cannot consume the canvas cap.
- Realtime scene updates do not echo, do not share local UI state, and do not imply unsupported image persistence.
- The teacher can control the session floor from the live interface.
- Every open Plan-wide Review 1 finding is fixed or explicitly resolved with evidence before the code-review gate passes.
- The exact implementation commit passes `bash scripts/ci-local.sh` before merge.
