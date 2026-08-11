# Session whiteboard review remediation

**Status:** Design review resumed under an uncapped consensus gate by user direction on 2026-08-11.

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

The response states whether the final snapshot present in the responding Hocuspocus process was confirmed in the archive under an active freeze fence.

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

Canvas authentication and every mutation recheck reject an unexpired freeze lease, so a Hocuspocus restart cannot reopen writes between freeze and database end.

PostgreSQL `clock_timestamp()` is the sole wall-clock authority for acquiring, comparing, replacing, and completing durable leases.

The initial lease duration is 15 seconds from the database clock, which bounds a live-room interruption if the Go request dies before end or cleanup.

The implementation must use a named constant and a controlled-clock test rather than scattering the duration.

The successful end is one `UPDATE` whose `WHERE` clause requires the session to be live, the token to match, and `canvas_freeze_until > clock_timestamp()`.

The success decision and the write are therefore one database statement, never a prior check inside the transaction.

Mutation authorization takes a transaction-scoped shared PostgreSQL advisory lock for the session before reading status and lease state.

Every end transition takes the matching exclusive advisory lock before its session update and holds it through commit.

The lock uses PostgreSQL's transaction-scoped two-`int4` functions only: `pg_advisory_xact_lock_shared` and `pg_advisory_xact_lock`.

Session-scoped advisory functions are forbidden.

The first key is the reserved Bridge session-lifecycle class ID hexadecimal `0x42524447`, decimal `1112687687`.

The second key is derived identically in Go and TypeScript from the canonical lowercase UUID string: remove hyphens, parse the first eight hexadecimal characters as an unsigned 32-bit value, then reinterpret its two's-complement bits as a signed `int4`.

The required fixture vectors are `00000000 → 0`, `12345678 → 305419896`, `7fffffff → 2147483647`, `80000000 → -2147483648`, and `ffffffff → -1`.

A collision in the second key may serialize concurrently active unrelated sessions but cannot collide with another Bridge-internal class that follows the same registry or weaken authorization.

External database clients do not share that registry, so an accidental external collision remains an availability-only risk and is documented with the lock constant.

Lease acquisition, lease replacement, successful or degraded end, abort cleanup, and ended-session residual cleanup take the exclusive session advisory lock.

Mutation authorization takes the shared session advisory lock.

Those transactions always acquire the advisory lock before any session row or related data access, and mutation authorization locks no second entity.

Canvas creation and floor transactions retain their existing session-row lock and never acquire this advisory-lock class, so they cannot invert the lifecycle lock order.

This avoids hot-row multixact churn while making an authorization query block behind an executing end and observe `ended` after commit.

When the token matches and the lease is still unexpired, the successful update must persist the recorded Hocuspocus freeze result.

When the token matches but the lease has expired, the transaction still ends the session because session status wins, but it must persist `whiteboard_server_archive_complete = false`.

When a different token owns an unexpired lease, the stale request returns `session_end_in_progress` and cannot end or unfreeze the session.

When a different token is present but expired, the stale request may still end the session because status wins, but it must persist `whiteboard_server_archive_complete = false` and must not reuse either operation's freeze result.

Aborting a still-live operation clears the lease only when its token matches.

An end that consumes any expired lease clears that expired token even when it differs from the request token.

An already-ended completion is idempotent: it returns the stored archive result, never rewrites it, and must clear a matching or expired residual lease as housekeeping.

Hocuspocus mirrors active operations in an in-memory map keyed by session ID with token, database expiry, and monotonic expiry, never a boolean set.

Every freeze and unfreeze operation for one session executes through the same per-session async serializer.

The serializer is acquired before database validation and held through every in-memory map mutation, final flush, connection close, and operation result.

An unfreeze that arrives while freeze validation or flush is in progress waits for that freeze to finish or cancel, then removes the matching entry as the final serialized action.

Unfreeze performs no database query and is never abandoned merely because its caller disconnects.

Its queue wait is bounded by the active freeze deadline plus cancellation settlement, after which token comparison and local removal are synchronous.

Duplicate freezes for the same token share the result only while that operation is unsettled; a request arriving after settlement starts a fresh database validation.

Duplicate matching unfreezes coalesce, and a different freeze or unfreeze token receives a retryable conflict without entering the queue while an operation is active.

Because every active operation has the client-side deadline above, a rightful replacement token cannot be starved by a stale operation and retries after that bounded conflict.

The per-session queue therefore contains at most one active operation and one coalesced pending matching unfreeze.

Serializer registry lookup-or-create, enqueue, last-dequeue eviction, and map-entry identity checks execute synchronously without an `await` between check and mutation.

Only the last dequeued serialized operation may evict its serializer when the queue is empty and no active freeze-map entry remains, including after session end and lease expiry.

Hocuspocus captures `performance.now()` before starting database lease validation.

The database returns the remaining lease milliseconds evaluated with `clock_timestamp()`.

The conservative monotonic deadline is the pre-request monotonic start plus that returned duration, so validation and transport time are subtracted rather than extending the lease.

The freeze operation deadline is the earlier of that conservative deadline and its two-second internal bound.

The operation rechecks the monotonic deadline, cancellation state, and exact database token before map installation and before every later stage.

When cancellation or the internal deadline wins, the operation is marked inactive and all started cancellable work receives an abort signal.

The serializer remains held until every started operation settles or acknowledges cancellation.

Database validation and snapshot writes use both server-side statement timeouts and client-side pool-checkout, socket, and query deadlines bounded by the operation deadline.

Each freeze operation uses a short-lived, non-pipelined PostgreSQL control client with one connection, a connect timeout no greater than the remaining operation budget, and no reuse by another session.

Deadline handling calls the postgres.js query cancellation API and destroys that operation client with `end({ timeout: 0 })`; a timed-out connection is never returned to the ordinary Hocuspocus pool.

A zero or negative remaining budget fails before checkout or query start.

When a client deadline expires, the underlying connection is destroyed and cannot return to the pool; the wrapper settles without waiting for an operating-system TCP timeout.

The conditional snapshot statement remains the database-side authority if a write reached PostgreSQL immediately before client cancellation.

These client and server bounds ensure cancellation settlement cannot hold the serializer indefinitely.

No final-flush or connection-close stage may start after cancellation.

Connection close is synchronous and runs only after every final flush succeeds while the operation remains active.

Every asynchronous sub-operation is awaited and cancellation-guarded, and no detached continuation may write a snapshot, mutate the map, close a connection, or publish a result after the serializer is released.

An unfreeze for one token cannot clear a newer or concurrent token.

Hocuspocus stores the conservative monotonic deadline computed from the pre-validation start; it never starts a fresh full-duration timer when validation returns.

It does not compare a Go- or Node-generated wall-clock timestamp to the database expiry.

Every expiry timer and lazy or sweep cleanup reacquires the per-session serializer and removes the map entry only when both its expected token and entry identity still match.

A delayed timer for an old token cannot delete a replacement token.

The same serialized cleanup synchronously performs the serializer-eviction check after removing an expired entry.

Inside the serializer, a freeze installs or replaces an entry only after a fresh database query proves that its exact token owns the current unexpired lease.

That validation transaction takes the same shared session advisory lock as mutation authorization, so it observes any preceding exclusive abort cleanup before it can install an entry.

No expiry ordering or timestamp tie-break is used for token replacement because no two validations for the same session may mutate the map concurrently.

Bridge currently supports one Hocuspocus process for realtime documents.

Running multiple Hocuspocus replicas is unsupported until a coordinated document fan-out and flush protocol exists, even though the durable lease prevents post-freeze writes across processes.

### Internal Hocuspocus API

Hocuspocus exposes two internal HTTP operations:

- `POST /internal/canvas-sessions/freeze`
- `POST /internal/canvas-sessions/unfreeze`

Both operations require `Authorization: Bearer <HOCUSPOCUS_CONTROL_SECRET>`.

Both operations accept a strict JSON body of `{ "sessionId": "<uuid>", "freezeToken": "<uuid>" }`.

Unknown fields, a missing field, and a malformed session or freeze-token UUID are rejected.

The freeze success response is `{ "flushed": <number>, "closed": <number> }`.

The unfreeze success response is `{ "unfrozen": true }` when the matching operation was cleared or no operation is active.

The Go client treats a non-2xx response, timeout, transport error, invalid JSON, missing field, negative count, or wrong field type as a freeze failure.

The Go client uses `HOCUSPOCUS_INTERNAL_URL` for the origin and injects its HTTP client for deterministic timeout tests.

Hocuspocus uses a distinct `HOCUSPOCUS_CONTROL_PORT` with default `4001` for the internal listener.

`HOCUSPOCUS_INTERNAL_URL` defaults to `http://127.0.0.1:<HOCUSPOCUS_CONTROL_PORT>`.

The control operations run on a separate internal HTTP listener and are never registered on the browser websocket listener.

The internal listener defaults to `127.0.0.1` and requires an explicit private-network binding when Go and Hocuspocus run on separate hosts or containers.

Any non-loopback control listener requires HTTPS with normal certificate and hostname verification.

Plain HTTP is permitted only for a loopback listener.

The Go client validates `HOCUSPOCUS_INTERNAL_URL` at startup and permits plain HTTP only when its canonical parsed IP is in `127.0.0.0/8` or equals IPv6 loopback `::1`.

IPv4-mapped IPv6 addresses are rejected rather than normalized into the IPv4 allowlist.

DNS names, including `localhost`, are not accepted as proof of loopback for plain HTTP.

The control client never follows redirects.

The Hocuspocus process fails startup if the control listener cannot bind, if its secret is missing or invalid, or if non-loopback TLS configuration is missing or invalid.

The operations use a distinct `HOCUSPOCUS_CONTROL_SECRET`, not the realtime JWT signing secret.

The bearer is compared in constant time after validating its expected encoding and length.

The internal URL, listener binding, port, and secret are server-only configuration and are documented separately from any browser websocket URL.

### Freeze behavior

Hocuspocus maintains the operation-owned in-memory freeze map described above.

The freeze operation validates that the supplied token owns an unexpired database lease, then installs the matching in-memory entry before awaiting document work.

For every currently loaded `canvas:{canvasId}` belonging to the session, the operation serializes the current Yjs document and persists it through a dedicated final-flush path.

The final-flush database write itself is conditional on the session being live, the lease token matching, and `canvas_freeze_until > clock_timestamp()`.

Those predicates are part of the write statement or its locking transaction, never a check performed before the write.

The final-flush path fails if the conditional write affects no row because the lease, canvas, or live session no longer exists.

Each final flush runs through the same Hocuspocus document save mutex as ordinary and debounced persistence.

The mutex is held through encoding and database completion so an older in-flight store cannot commit after the final snapshot.

The freeze operation succeeds only after every loaded canvas has been persisted successfully.

After a successful flush, Hocuspocus closes all loaded canvas connections for the session and reports the flushed and closed counts.

If any final flush errors before the deadline, the active operation records one shared failure result, starts no later flush or close stage, retains its token-owned barrier, and returns non-2xx.

Open connections remain temporarily fenced until Go ends the session, token-matched unfreeze runs after a database end failure, or the lease expires.

The failure result never reports a successful flush or close count.

The unfreeze operation removes only an entry with the matching token and is idempotent.

If a different active token owns the entry, unfreeze returns `409 freeze_token_mismatch` rather than reporting success.

A Hocuspocus restart may lose the in-memory map, but the durable unexpired lease rejects later authentication and mutation rechecks until Go ends the session or the lease expires.

### Mutation ordering

The canvas mutation guard checks the operation-owned freeze map before the authorization request and again after the authorization request returns.

Every mutation-bearing Yjs frame performs the existing uncached Go authorization request in a short transaction that takes the session's shared advisory lock.

That request evaluates the active durable lease in PostgreSQL using `clock_timestamp()` on every call; the in-memory map is an immediate local barrier for the freeze handler, not an authorization cache.

A frame already awaiting authorization when freeze begins is rejected by the second frozen check.

A frame that has fully passed the second guard check must apply and relay before another macrotask or event-loop callback can run the freeze HTTP handler.

The installed Hocuspocus promise continuation between `beforeHandleMessage` and `MessageReceiver.apply` is permitted because its microtask drains before the next HTTP event-loop task.

No timer, I/O callback, `setImmediate`, or other macrotask yield is permitted between the second check and completed Yjs apply and relay.

The subsequent flush therefore captures every mutation accepted before the freeze boundary.

On a confirmed freeze path, no mutation accepted after the freeze boundary may apply, relay, or persist.

Mutation authorization remains uncached so a committed session end takes precedence over realtime availability and throughput.

Client-side 100-millisecond coalescing reduces mutation volume without weakening that server-side decision.

### Go end-session sequence

1. The existing handler authorizes the represented user as the session teacher under the existing tenancy rules.
2. Go acquires the operation-owned freeze lease or returns an idempotent ended response or stable in-progress conflict.
3. Go calls Hocuspocus freeze with the matching token and a two-second deadline, then records whether a valid success response was received.
4. Go executes the database end transition under the row lock regardless of the Hocuspocus result, using the freeze result only when the token still matches and the lease is unexpired, and otherwise using the expiry and ownership rules above.
5. If the database transition succeeds, Go emits the existing session-ended event and schedules the existing completion work.
6. If the database transition fails, Go clears only its matching database lease, sends a best-effort token-matched unfreeze request, and returns the existing safe 500-class error.
7. A freeze or unfreeze failure is logged as an actionable operational event without tokens, document content, user content, or internal response bodies.

A freeze request that completes after Go's timeout may leave its Hocuspocus operation frozen only until token-matched cleanup or the lease expiry.

That state is correct when the database end succeeds because the session is ended.

If the database end subsequently fails, token-matched cleanup restores collaboration immediately when available and the lease expiry bounds the failure when cleanup is unavailable.

### Confirmed and degraded guarantees

A confirmed path requires a successful Hocuspocus freeze and final flush followed by the matching, unexpired, advisory-lock-protected end update.

That path guarantees that no post-fence mutation applies or relays and that the final snapshot present in the responding Hocuspocus process was stored.

A degraded path is any Hocuspocus failure, timeout, lease expiry, stale-token fallback, or loss of the realtime process fence.

The degraded path still commits `status = 'ended'` and persists `whiteboard_server_archive_complete = false` because session lifecycle outranks realtime availability.

Any finite set of frames whose independent authorization transactions completed before the degraded end acquires its exclusive advisory lock may apply and relay transiently after the database commit.

That unavoidable cross-process race is not represented as archived: the ended-session storage guard rejects it, subsequent mutation checks and reconnects observe `ended`, and the teacher receives the incomplete-server-archive warning.

The implementation and documentation must not claim apply-and-relay atomicity for a degraded path.

### Public end-session response

The response preserves the existing top-level session fields so current consumers do not need a structural migration.

For sessions ended by this protocol, it adds `whiteboardServerArchiveComplete: boolean`.

For an older ended session whose stored value is null, the field and warning are omitted and the archive renders no completeness claim or warning.

When the value is false, it also adds `warning: "whiteboard_server_archive_incomplete"`.

The field and warning refer only to the final snapshot present in the responding Hocuspocus process at the confirmed freeze boundary.

They do not claim that unsent browser state, including a scene still inside the 100-millisecond client coalescing window, or state lost in an earlier process failure was archived.

The warning code is stable and contains no internal failure detail.

The database end transition persists `whiteboard_server_archive_complete` on the session row so the incomplete result survives tabs, navigation failures, and later archive visits.

The teacher dashboard may store the warning code in `sessionStorage` immediately before navigating to `/sessions/{sessionId}/whiteboards` for prompt display.

The archive page first confirms the durable status through `GET /api/sessions/{sessionId}/canvas-settings`, then consumes and removes the one-shot browser value before rendering the approved warning message.

A 200 settings response is authoritative: false displays the durable warning, while true or omitted null state clears a stale browser warning without displaying it.

A network error or non-200 settings response does not consume the browser value.

The warning is not placed in the URL.

If ending the session fails, the teacher remains on the live page and sees an actionable error rather than being redirected.

## Realtime connection lifetime

Canvas authentication places the verified JWT expiry and token read-only decision in the connection context.

Hocuspocus schedules a connection close at the JWT expiry and clears the timer when the connection closes earlier.

Expiry delays beyond Node's maximum timer duration are chained or clamped and rechecked, while already expired tokens close immediately.

The close applies to writable and read-only canvas connections.

A per-connection hook performs the current internal authorization check before that connection is admitted to the document.

Permanent viewer or ended-session authorization applies `connectionConfig.readOnly = true`.

An active temporary freeze does not mutate `connectionConfig.readOnly` because that flag is irreversible for the lifetime of the connection under Hocuspocus 3.4.4.

Instead, the server rejects or closes a frozen writer with the retryable `session_freezing` outcome.

The client reconnects with a maximum individual delay of two seconds and continues for at least 20 seconds after the most recent `session_freezing` rejection, exceeding the 15-second lease duration.

Every new retryable freeze rejection resets that horizon, so consecutive end attempts cannot exhaust recovery before the last lease expires.

An uncategorized transport close while the session is believed live uses the two-second ceiling for the first 20 seconds, then grows with jitter toward a 30-second ceiling while continuing until reconnection succeeds, an ended decision arrives, JWT refresh fails, or the user leaves.

It never exhausts into a manual-reload state, but sustained outages avoid a synchronized two-second retry herd.

A failed or crashed end therefore restores write access after token-matched cleanup or lease expiry without a manual reload or Hocuspocus restart.

The design must not rely on `onLoadDocument` for this decision because Hocuspocus does not call it for every connection to an already loaded document.

A stale writable claim connecting after the session ends is therefore read-only immediately, rather than only after its first mutation.

A user promoted from permanent viewer to writer must reconnect before gaining write access because permanent connection read-only is intentionally not reversible.

The existing pre-apply mutation recheck and storage guard remain defense in depth.

## Canvas API corrections

`GET /api/sessions/{sessionId}/canvas-settings` returns `{ "canvasFloor": "<level>", "whiteboardServerArchiveComplete"?: <boolean> }`.

The GET route is available to the represented session teacher while the session is live or ended.

It returns 404 for a missing session and 403 for every other represented user, including a platform administrator who is not impersonating the teacher.

`PATCH /api/sessions/{sessionId}/canvas-settings` remains teacher-only, live-only, and returns `{ "canvasFloor": "<level>" }`.

Both methods use the dedicated route rather than the former generic `/settings` name.

The former feature-branch-only `PATCH /api/sessions/{sessionId}/settings` route is removed rather than aliased, and the live teacher panel adds the dedicated route in the same phase.

Plan 094 has not shipped and the current deployed teacher panel has no floor control or caller of the old route, so no deployed pre-remediation browser bundle can invoke it.

Pre-merge verification confirms the absence of an old-route caller in `main`; stale local feature-development tabs must reload and are not represented as a production compatibility guarantee.

The new client requires both a 2xx status and the exact expected JSON response schema before claiming the floor changed; any parse or schema failure is surfaced.

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
- A request that resumes after its lease expires still ends the session but must persist and return `whiteboardServerArchiveComplete: false`.
- A stale request cannot end or clear a session owned by a different unexpired lease token.
- A stale request encountering a different expired token may end status-first but must persist `whiteboardServerArchiveComplete: false` and must not reuse either freeze result.
- The atomic successful end update and shared advisory-lock mutation authorization prevent a post-expiry frame from being admitted between the success predicate and commit on a confirmed path.
- A degraded-path test pauses multiple independently authorized frames across several connections, commits the incomplete end, resumes the concurrent fan-in, and proves transient applies cannot persist while every later mutation and reconnect is denied.
- Session advisory locks are always acquired before database reads or row locks, and a busy-session test proves end completion under concurrent mutation authorization without deadlock or exceeding the approved bound.
- Lease acquire, replace, abort, and complete use the reserved exclusive advisory key, while mutation and Hocuspocus validation use its shared form; a paused validation observes a preceding abort commit.
- Go, TypeScript, and PostgreSQL fixture vectors produce the exact same signed second advisory key for all five boundary UUID prefixes.
- Lease acquisition, replacement, mutation authorization, final flush, and end completion use database `clock_timestamp()` semantics under controlled tests.
- Teacher end authorization and cross-user isolation remain covered.
- Canvas creation covers teacher, present participant, invited participant, left participant, public outsider, platform admin, impersonator, ended session, and cap races.
- Floor mutation covers teacher authorization, the three allowed values, rejection of `session`, and the existing row-lock invariant.
- The exact Plan 094 mint and ended-mutation test names are present rather than being represented only by broad table tests.

### Hocuspocus tests

- Freeze authenticates the bearer secret and validates strict input.
- A mutation racing a successful freeze proves that the last accepted scene is persisted before connections close.
- A mutation awaiting authorization when freeze begins is rejected before Yjs apply and relay.
- Two successive frames on one connection perform two Go rechecks, and a durable lease acquired between them is observed even when the local freeze map is empty.
- The installed promise microtask from the second freeze check through Yjs apply cannot be interleaved by the freeze HTTP macrotask.
- A final flush waits for an already in-flight ordinary store and remains the last committed snapshot through the shared save mutex.
- A final flush whose conditional live/token/unexpired predicate no longer matches writes nothing and reports failure.
- A final-flush failure returns failure and keeps the session frozen until unfreeze.
- A partial final-flush error starts no later flush or connection close, returns the same failure to active duplicates, and retains only the matching barrier until lifecycle cleanup.
- Unfreeze is token-scoped and idempotent, and stale unfreeze cannot clear a newer freeze.
- A different active unfreeze token returns `409 freeze_token_mismatch`.
- A paused freeze validation and queued unfreeze serialize so that unfreeze is the final map action and the late validation cannot reinstall the barrier.
- A freeze request that arrives after its matching unfreeze revalidates against the database, observes the cleared lease, and cannot install a barrier.
- A cancelled or timed-out validation continuation cannot mutate the map after the per-session serializer is released.
- A validation response delayed beyond database expiry cannot install a barrier because elapsed monotonic time is subtracted and checked before installation.
- Each post-install asynchronous final-flush stage is paused across timeout and queued unfreeze to prove the serializer and cleanup wait for cancellation settlement; after acknowledgment, no write, close, map, or result effect can occur after release.
- Duplicate operations coalesce, foreign tokens conflict without queueing, the queue stays bounded, and an idle serializer is evicted.
- Serializer lookup, enqueue, last-dequeue eviction, expiry sweep, and token-conditional timer cleanup interleave under controlled scheduling without creating two serializers or deleting a replacement token.
- Pool-checkout timeout, zero remaining budget, query timeout, and half-open socket each settle through client cancellation, destroy the connection, release cleanup, and permit no late effect.
- The maximum 50 loaded canvases complete the batched or parallelized final-flush path inside the two-second Go deadline under the approved representative test latency.
- Canvas connections close at JWT expiry under a controlled clock.
- A stale writable token joining an already loaded ended canvas is downgraded, while one joining a temporarily frozen canvas receives the retryable freeze outcome.
- A temporary freeze never permanently downgrades a writer, and the writer can write after cleanup or expiry without reload through a reconnect horizon longer than the lease.
- Missing or malformed internal authorization remains fail-closed.
- Configuration tests prove distinct default control and websocket ports, absence of control routes on the public listener, IP-literal loopback-only plaintext for IPv4 and IPv6, DNS-name and non-loopback HTTP rejection, redirect refusal, certificate and hostname verification, missing or invalid control-secret failure, non-loopback TLS failure, and fail-fast control-port bind errors.

### Frontend and binding tests

- Remote scene application produces no echo write.
- Identical local scenes are skipped.
- Bursts of local changes produce one trailing write after 100 milliseconds.
- Only elements and allowlisted durable state enter Yjs.
- Local viewport, zoom, selection, tool, collaborators, and view mode survive remote updates.
- Image insertion, clipboard images, and file drops are rejected with a visible explanation.
- The live teacher panel reads and updates the floor.
- Canvas-settings GET covers live teacher, ended teacher with true/false/null archive state, missing session, unrelated user, direct platform admin, and teacher impersonation.
- Owner visibility changes require confirmation at the irreversible levels.
- Owner and viewer controls differ correctly.
- Teacher end failure remains in place with an error.
- Incomplete server-archive status survives the redirect, remains durably discoverable, and displays exactly once per archive visit.
- A 200 durable status overrides and consumes browser state, while network and non-200 responses retain it.
- The new settings client accepts only a 2xx response with the exact schema, and repository/main-history checks prove no deployed bundle called the removed feature-branch route.
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
- A teacher is warned when storage of the fenced Hocuspocus final snapshot is not confirmed.
- A successful freeze persists every server-accepted loaded-canvas mutation before closing its connections.
- On a confirmed path, no canvas mutation accepted after the freeze boundary applies or relays.
- On a degraded path, any finite set of frames already authorized before the exclusive end may apply transiently; every later mutation-bearing frame rechecks and observes `ended` or fails closed, and every remaining canvas connection closes no later than JWT expiry.
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

Review verdicts, findings, responses, and resolutions are recorded in the spec's `## Design Review` section with `[sol]` and `[fable]` source tags and the exact reviewed commit SHA.

A finding remains `[OPEN]` while a response is pending reviewer confirmation; an author response never self-certifies `[FIXED]`.

Only approval of the exact substantive commit permits the orchestrator to mechanically change its answered findings to `[FIXED]` and append the verdict ledger.

That ledger-only status update is non-substantive and does not invalidate the approvals it records.

Both reviewers are re-dispatched after every substantive revision in an active design gate.

A material revision after approval invalidates both prior verdicts and requires both reviewers to approve the new commit.

Material revisions include behavior, interfaces, authorization, persistence, failure semantics, scope, dependencies, or acceptance criteria; spelling and formatting-only corrections do not invalidate verdicts.

The design gate has no review-round cap.

This is the proposed permanent rule and becomes globally effective only when the reviewed Plan 094 remediation updates the canonical governance files in its declared file scope.

Until that merge, existing repository governance remains authoritative for other designs.

Spec 013 alone continues beyond its former cap under the user's explicit 2026-08-11 direction recorded below; that exception does not silently amend another design's gate.

Round numbers are audit labels only, and review continues until both required reviewers approve the same substantive commit with no open blockers.

A runtime, transport, authentication, quota, or empty-output failure blocks the gate until the required reviewer returns a verdict but does not terminate the consensus process.

The gate pauses only for a hard safeguard, a genuine user decision, an unavailable required reviewer that cannot be recovered, or explicit user direction to stop.

After every three consecutive substantive rounds without consensus, the orchestrator records and surfaces a concise non-convergence checkpoint with cumulative open findings and reviewer status, then continues unless a pause condition applies.

If the same finding is reopened twice after a claimed resolution, or two consecutive checkpoints show no net reduction in open blockers, that non-convergence becomes a genuine user-decision pause rather than an autonomous spending loop.

After user direction, the uncapped consensus process resumes with round numbering preserved.

A passing design gate authorizes plan drafting or revision, not implementation.

The plan-review gate remains separately required before implementation.

The canonical rule is mirrored in `AGENTS.md`, `docs/reviewers.md`, `docs/development-workflow.md`, and `docs/coding-agent.md` in the Plan 094 remediation scope before those governance files are changed.

## Design Review

### Round 1 — 2026-08-10 — commit `b589dd7`

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

### Round 2 — 2026-08-10 — commit `d572d25`

- `[OPEN]` `[sol][fable]` An end request could outlive its 15-second lease and still commit a pre-expiry successful archive result after writes resumed.
  → Response in `aaf2f44` and `260ca47`: the successful end uses an atomic token-and-unexpired `UPDATE`, mutation authorization takes a shared row lock, and every expired-lease path records an incomplete server archive.
- `[OPEN]` `[fable]` Go, Node, and PostgreSQL clocks could disagree about lease expiry.
  → Response in `aaf2f44`: PostgreSQL `clock_timestamp()` is the wall-clock authority, and Node derives only a monotonic remaining duration from a fresh database validation.
- `[OPEN]` `[fable]` Final-flush liveness and lease validation could be a check-before-write race.
  → Response in `aaf2f44`: the live, token, and unexpired predicates execute in the snapshot write statement or its locking transaction.
- `[OPEN]` `[fable]` The durable per-mutation lease check was underspecified and could be weakened by caching.
  → Response in `aaf2f44` and `260ca47`: every mutation-bearing frame retains the uncached, shared-row-lock Go recheck, and the test contract now proves two consecutive frames observe an intervening durable lease.
- `[OPEN]` `[fable]` Older null archive state had no defined public representation.
  → Response in `aaf2f44`: null omits the optional boolean and warning and produces no completeness claim in the archive.
- `[OPEN]` `[fable]` A non-loopback internal listener could send its bearer and freeze token over plaintext.
  → Response in `aaf2f44` and `260ca47`: non-loopback traffic requires verified HTTPS, the Go client rejects non-loopback HTTP at startup, and the configuration test matrix enforces both sides.
- `[OPEN]` `[sol]` The internal URL default incorrectly reused the public websocket port.
  → Response in `aaf2f44`: a distinct `HOCUSPOCUS_CONTROL_PORT` defaults to 4001 and drives the loopback internal URL.
- `[OPEN]` `[sol]` Durable archive status had no ended-session-readable producer-to-consumer API.
  → Response in `aaf2f44`: the canvas-settings GET contract defines its response, represented-teacher authorization, null behavior, 403, 404, and integration matrix.
- `[OPEN]` `[sol]` The no-microtask invariant contradicted Hocuspocus 3.4.4's promise continuation.
  → Response in `aaf2f44`: the actual invariant permits the installed microtask and forbids an interleaving macrotask before apply and relay.
- `[OPEN]` `[sol]` Design approvals were not bound to a commit and could survive an unreviewed material edit.
  → Response in `aaf2f44`: both reviewers approve an exact commit and must be re-dispatched after every substantive revision.

**Round 2 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

### Round 3 — 2026-08-10 — commit `aaf2f44`

- `[OPEN]` `[fable]` The end transaction could check expiry before its write and admit a post-expiry mutation before committing a successful result.
  → Response in `260ca47`: success is a single conditional `UPDATE`, and mutation authorization takes a shared row lock that blocks behind the update and observes the committed ended status.
- `[OPEN]` `[fable]` A stale request encountering a different expired lease token had no defined state transition.
  → Response in `260ca47`: status may still end, but the stale request must record an incomplete archive and cannot reuse either operation's freeze result.
- `[OPEN]` `[sol]` A temporary freeze could irreversibly set an established writer's connection to read-only.
  → Response in `260ca47`: temporary freeze rejects or closes with retryable `session_freezing`; only permanent viewer or ended decisions set read-only, and reconnect restores writing after cleanup or expiry.
- `[OPEN]` `[sol][fable]` The control transport rules lacked client-side non-loopback HTTP rejection and an executable configuration matrix.
  → Response in `260ca47`: Go fails startup on non-loopback HTTP, Node fails on invalid secret, TLS, or bind configuration, and the named test matrix covers listener isolation and verified HTTPS.
- `[OPEN]` `[sol]` The uncached mutation rule lacked a same-connection proof.
  → Response in `260ca47`: two consecutive frames must perform distinct rechecks and observe a durable lease acquired between them with an empty local map.
- `[OPEN]` `[fable]` A newly valid token had no rule when an older in-memory token's monotonic timer remained active.
  → Response in `260ca47`: a freshly database-validated token replaces the stale in-memory entry because PostgreSQL is authoritative.
- `[OPEN]` `[fable]` The review ledger attributed resolution text to the older reviewed commit.
  → Response in this ledger revision: every response names the later commit that contains it, while headings retain the exact reviewed SHA.
- `[OPEN]` `[fable]` The old settings route, normative freeze-result write, control-port bind failure, and browser-warning consumption order were underspecified.
  → Response in `260ca47`: the old route is removed with same-phase client migration, matched unexpired end must persist the result, bind failure aborts startup, and durable confirmation precedes one-shot consumption.

**Round 3 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

### Round 4 — 2026-08-10 — commit `7b2e644`

- `[OPEN]` `[sol]` A degraded end could commit after an authorization transaction released its lock but before the allowed frame applied and relayed.
  → Response in `18e9216`: the design no longer claims impossible cross-process apply-and-relay atomicity when Hocuspocus cannot retain a fence; it defines the confirmed guarantee separately and proves that a degraded transient frame cannot persist, continue, or survive reconnect while the teacher receives the incomplete warning.
- `[OPEN]` `[fable]` The ledger self-certified findings as fixed even though the exact response commit had not been approved.
  → Response in `18e9216`: every finding is restored to `[OPEN]`, responses never change status, and only exact-commit reviewer approval permits a mechanical ledger-only transition to `[FIXED]`.
- `[OPEN]` `[fable]` An older asynchronous database-validation response could overwrite a newer in-memory freeze token.
  → Response in `18e9216`: the map stores database expiry and accepts a different token only when its database expiry is later than the current entry.
- `[OPEN]` `[fable]` Foreign expired and already-ended lease cleanup states were undefined.
  → Response in `18e9216`: any consumed expired lease is cleared, an already-ended completion never rewrites the archive result, and matching or expired residue may be cleaned safely.
- `[OPEN]` `[fable]` A row-level shared lock on every mutation risked multixact churn and lacked a universal lock order.
  → Response in `18e9216`: mutation and end use transaction-scoped shared/exclusive session advisory locks, always acquired before database access, and the busy-session test covers deadlock and latency.
- `[OPEN]` `[fable]` Plain-HTTP loopback and redirect behavior was ambiguous.
  → Response in `18e9216`: only IP-literal `127.0.0.0/8` and `::1` are accepted for HTTP, DNS names are rejected, and redirects are disabled.
- `[OPEN]` `[fable]` The reconnect horizon could expire before the 15-second freeze lease.
  → Response in `18e9216`: reconnect continues for at least 20 seconds with no individual delay above two seconds, and the crash test requires recovery without reload.
- `[OPEN]` `[fable]` Durable/browser warning disagreement, permanent viewer promotion, and stale old-route clients were underspecified.
  → Response in `18e9216`: a 200 durable result is authoritative, failed requests retain browser state, promotion requires reconnect, and stale-route 404 is surfaced as an error.

**Round 4 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

### Round 5 — 2026-08-10 — commit `5e8b4ab`

- `[OPEN]` `[sol][fable]` A database validation started before cleanup can return after token-matched unfreeze removed an empty map entry and reinstall the stale freeze barrier.
  The later-expiry comparison has no state to compare when the map is empty, so the accepted design still needs per-session freeze/unfreeze serialization or a bounded tombstone or generation retained after removal.
- `[OPEN]` `[fable]` The acceptance criterion still states unconditional post-freeze apply-and-relay atomicity even though the body intentionally limits that guarantee to confirmed paths.
- `[OPEN]` `[fable]` The reconnect horizon must reset on every retryable freeze rejection to survive consecutive end attempts.
- `[OPEN]` `[fable]` Advisory-lock participation needs to cover lease acquisition and replacement explicitly, use a reserved two-part application keyspace, and define equal-expiry token ordering without timestamp truncation ambiguity.
- `[OPEN]` `[fable]` Already-ended residual-lease cleanup must be deterministic, stale-route errors must tolerate an unparseable non-2xx body, and loopback checks must use canonical parsed addresses while rejecting IPv4-mapped IPv6 forms.

**Round 5 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

**Historical gate result:** BLOCKED under the former five-round rule.
The five-round maximum is exhausted with unresolved `[OPEN]` findings, so the hard safeguard requires user direction before any further design revision, Plan 094 re-plan, governance-file edit, or implementation.

### User direction — 2026-08-11

The user approved reopening the design with per-session serialized freeze and unfreeze operations and removed the design-review round cap.

The historical Round 5 block is therefore resolved as a process decision, while its technical findings remain `[OPEN]` until Sol and Fable approve the same revised commit.

### Round 6 — 2026-08-11 — commit `fd80fde`

- `[OPEN]` `[sol]` Validation delay was not subtracted from the database remaining duration, so an already-expired token could receive a new local interval.
  → Response in `816256a`: Node anchors the conservative deadline before the database request, subtracts all validation and transport delay, and rechecks before every stage.
- `[OPEN]` `[sol]` Cancellation tests did not cover final-flush and connection-close continuations after timeout and queued unfreeze.
  → Response in `816256a`: the serializer remains held until every started stage settles or acknowledges cancellation, no new stage starts after cancellation, connection close is synchronous and last, and each stage gets a paused-continuation regression.
- `[OPEN]` `[fable]` The proposed uncapped gate contradicted canonical capped governance before those files were changed.
  → Response in `816256a`: the future rule becomes globally effective only through the reviewed governance edit; current repository rules remain authoritative elsewhere, while Spec 013 continues solely under explicit user direction.
- `[OPEN]` `[fable]` Hocuspocus lease validation did not participate in the advisory-lock order and could race Go abort cleanup.
  → Response in `816256a`: validation takes the shared session advisory lock and therefore observes preceding exclusive cleanup.
- `[OPEN]` `[fable]` Unfreeze and serializer queues lacked execution bounds and eviction.
  → Response in `816256a`: unfreeze is local and database-free, waits behind bounded cancellation settlement, duplicate operations coalesce, foreign tokens do not queue, the queue has two bounded positions, and idle serializers are evicted.
- `[OPEN]` `[fable]` Stale clients could accept a schema-invalid 2xx response from the removed route.
  → Response in `816256a`: success requires both 2xx and exact response schema; every parse or schema failure is surfaced.
- `[OPEN]` `[fable]` The degraded-path acceptance contract did not state a bound on later mutations and the consensus loop had no non-convergence checkpoint.
  → Response in `816256a`: only an already-authorized frame may be transient, all later frames recheck or fail closed, connections close by JWT expiry, and every three non-converged rounds produces a non-blocking user checkpoint.
- `[OPEN]` `[fable]` Advisory-lock namespace, loopback parsing, redirect, and uncategorized reconnect behavior needed tighter bounds.
  → Response in `816256a`: the two-key Bridge class is scoped honestly, canonical IP parsing rejects mapped forms, redirects remain disabled, and unexpected live disconnects continue bounded retries without manual reload.

**Round 6 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

### Round 7 — 2026-08-11 — commit `7bf6269`

- `[OPEN]` `[fable]` Server-side statement timeout could not bound pool checkout or a half-open client socket, leaving cancellation settlement and the serializer unbounded.
  → Response in `de1dbae`: each freeze uses a short-lived one-connection control client with connect/query/socket bounds, postgres.js cancellation, forced `end({ timeout: 0 })`, and zero-budget rejection.
- `[OPEN]` `[fable]` The spec claimed it could retrofit exact-schema handling into an already loaded pre-deploy browser bundle.
  → Response in `de1dbae`: the route and floor UI are explicitly unshipped feature-branch work, `main` has no old caller, and only the new client carries the exact-schema contract.
- `[OPEN]` `[sol]` The cross-language second advisory key did not define its hash algorithm or signed mapping.
  → Response in `de1dbae`: the key is the signed reinterpretation of the canonical UUID's first eight hex digits, with exact boundary vectors for Go, TypeScript, and PostgreSQL.
- `[OPEN]` `[sol]` A delayed expiry timer could remove a newer replacement entry without serialized token and identity checks.
  → Response in `de1dbae`: timer, lazy, and sweep cleanup reacquire the serializer, compare expected token and entry identity, and perform atomic eviction checks.
- `[OPEN]` `[sol]` The degraded path incorrectly bounded transient fan-in to one frame even though Hocuspocus authorizations run independently.
  → Response in `de1dbae`: the guarantee and regression cover any finite set already authorized across multiple connections, while all later frames recheck or fail closed.
- `[OPEN]` `[fable]` Transaction-scoped advisory functions, flush-error behavior, atomic serializer eviction, duplicate-result lifetime, and rightful replacement retry were underspecified.
  → Response in `de1dbae`: only `pg_advisory_xact_lock*` is allowed, failure retains a bounded token barrier without closing, registry changes are synchronous, result sharing ends at settlement, and foreign-token conflicts are bounded by the active deadline.
- `[OPEN]` `[fable]` Sustained reconnects, repeated reviewer non-convergence, and sweep-driven serializer cleanup lacked operational bounds.
  → Response in `de1dbae`: reconnect grows to a jittered 30-second tail after the fast window, repeated non-convergence becomes a user decision, and serialized sweep cleanup performs eviction.

**Round 7 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

### Round 8 — 2026-08-11 — commit `40cb07c`

- `[OPEN]` `[sol][fable]` Pinned postgres.js 3.4.9 does not provide owned, awaited cancellation and physical socket destruction through `query.cancel()` and `end({ timeout: 0 })`, so the proposed control client can leave detached query and cancel sockets.
- `[OPEN]` `[fable]` A one-connection non-pipelined control client contradicts the parallel or batched 50-canvas final-flush deadline, while using the ordinary pool would violate isolation and risk pool poisoning.
- `[OPEN]` `[fable]` A rightful replacement token had no actual Go retry loop within the freeze budget, so retryable conflict could degrade immediately despite the no-starvation claim.
- `[OPEN]` `[fable]` The permanent test contract incorrectly retained a pre-merge `main` history fact, the PostgreSQL form of the advisory key remained unstated, and non-convergence metrics needed exact definitions.
- `[OPEN]` `[fable]` Post-settlement duplicate freeze behavior and categorized-to-uncategorized reconnect backoff transitions remained ambiguous.

**Round 8 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

**User-decision pause:** final snapshot persistence must either move into Go's owned pgx transaction boundary or use a new Node database primitive with genuinely owned and awaited socket abort.
