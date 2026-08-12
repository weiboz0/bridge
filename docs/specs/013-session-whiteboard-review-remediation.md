# Session whiteboard review remediation

**Status:** Design approved by Sol and Fable 5 at exact commit `aa34784e1e51596bf1b6779176a86187fe3306ab` on 2026-08-11.

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

PostgreSQL derives the second key as `(('x' || substr(replace(lower($1::text), '-', ''), 1, 8))::bit(32))::int4`; Go and TypeScript must match those exact bits.

A collision in the second key may serialize concurrently active unrelated sessions but cannot collide with another Bridge-internal class that follows the same registry or weaken authorization.

External database clients do not share that registry, so an accidental external collision remains an availability-only risk and is documented with the lock constant.

The existing one-argument `pg_advisory_xact_lock(hashtext(...))` uses PostgreSQL's disjoint `int8` advisory-lock keyspace.

The class-replacement guard defined below precedes sorted session-lifecycle locks, which precede the legacy one-`int8` lock if a future transaction needs multiple classes; the implementation audit and deadlock test include the current legacy caller.

Lease acquisition, lease replacement, successful or degraded end, abort cleanup, and ended-session residual cleanup take the exclusive session advisory lock.

Mutation authorization takes the shared session advisory lock.

Those transactions always acquire the advisory lock before any session row or related data access, and mutation authorization locks no second entity.

Canvas create, visibility, delete, and floor transactions join the same order by taking the shared advisory lock before their existing session-row lock.

This avoids hot-row multixact churn while making an authorization query block behind an executing end and observe `ended` after commit.

When the token matches and the lease is still unexpired, the successful update must persist the recorded Hocuspocus freeze result.

When the token matches but the lease has expired, the transaction still ends the session because session status wins, but it must persist `whiteboard_server_archive_complete = false`.

When a different token owns an unexpired lease, the stale request returns `session_end_in_progress` and cannot end or unfreeze the session.

When a different token is present but expired, the stale request may still end the session because status wins, but it must persist `whiteboard_server_archive_complete = false` and must not reuse either operation's freeze result.

Aborting a still-live operation clears the lease only when its token matches.

An end that consumes any expired lease clears that expired token even when it differs from the request token.

An already-ended completion is idempotent: it returns the stored archive result, never rewrites it, and must clear a matching or expired residual lease as housekeeping.

Hocuspocus mirrors active operations in an in-memory map keyed by session ID with token, database expiry, and monotonic expiry, never a boolean set.

Every freeze, unfreeze, and complete operation for one session executes through the same per-session async serializer.

The serializer is acquired before lease validation and held through every in-memory map mutation, document snapshot capture, connection close, and immutable cache publication, but response writers use tracked reader references outside it.

Lease validation is an abortable HTTP callback to Go, not a Node database query.

The callback takes the session ID and freeze token, acquires the shared session advisory lock in Go, and returns `{ "allowed": true, "remainingMs": <positive integer> }` only when the exact token owns the current live unexpired lease.

The freeze request also carries the authoritative canvas-ID list read by Go after lease acquisition.

An unfreeze or complete that arrives while validation, capture, or response writing is in progress becomes the one coalesced pending terminal cleanup for that token.

Complete takes precedence if matching unfreeze and complete requests race because the database has ended by the time Go sends complete.

The terminal cleanup marks the active operation inactive, aborts every matching response writer, waits for the validation, capture, and writer continuations to settle, then removes only the matching entry and releases its accounting as the final serialized action.

Terminal cleanup is never abandoned merely because its unfreeze or complete caller disconnects.

Its queue wait is bounded by the active freeze deadline plus abort settlement, after which token comparison and local removal are synchronous.

Duplicate freezes for the same token share the in-flight result and, after success, receive the identical cached snapshot bundle until matching unfreeze or lease expiry.

They never recapture an already-closed zero-document state as a new successful result.

Duplicate matching terminal cleanups coalesce, and a different freeze, unfreeze, or complete token receives a retryable conflict without entering the queue while an operation is active.

Go retries a retryable token conflict with bounded jitter while time remains inside the one overall two-second freeze budget; it does not reset the budget.

If no retry succeeds, the end proceeds on the degraded path with the warning.

The per-session queue therefore contains at most one active operation and one coalesced pending matching terminal cleanup, whose mode may be promoted from unfreeze to complete but never duplicated.

The shared Hocuspocus listener preserves the pinned `ws` global `maxPayload` of 100 MiB because it also carries existing attempt, chapter, broadcast, and session documents whose compatibility contract is not reduced by this remediation.

After Hocuspocus parses the document name and message envelope, the canvas admission hook rejects a decoded canvas mutation update above exactly 1,048,576 bytes before shadow or authoritative Yjs apply.

This canvas-only bound cannot prevent the shared `ws` layer from reassembling an authenticated oversized message up to its existing global cap, and the design makes no such pre-buffer claim.

Before applying persisted state, Hocuspocus rejects a decoded update above 4 MiB and reserves the same 16 MiB scratch slot from a process-wide 128 MiB resident-document admission ledger; successful authoritative-and-shadow load shrinks it to twice the encoded current-state size and any failure releases it.

Each loaded canvas owns a shadow `Y.Doc`, a per-document admission turnstile, and exact resident-ledger accounting for the authoritative document plus shadow.

For every canvas mutation entrypoint, `beforeHandleMessage` first performs the cheap local-fence check, awaits the document turnstile with an owned deadline and abort signal, rechecks the local fence after that possible yield, performs the uncached Go authorization with a separate owned `AbortController` and 500-millisecond deadline, and rechecks the local fence again.

Before allocating a waiter, timer, abort controller, or close listener, one synchronous per-document counter admits at most eight mutation admissions total, including the active holder and queued waiters.

The ninth concurrent mutation admission is never queued; its connection closes with retryable code 1013 and the existing bounded reconnect policy.

Every admitted path decrements the exact generation counter only after its waiter, authorization, handoff, and cancellation settlement complete.

Connection close, document unload, operation cancellation, or deadline aborts the owned authorization request and awaits its settlement before turnstile release.

If a cancelled turnstile waiter is later granted, its continuation observes the cancelled generation and releases the grant synchronously without touching shadow, accounting, or document state.

The freeze operation installs its fence first, then awaits each loaded document's admission turnstile with the freeze deadline and the same cancelled-grant release rule before acquiring its save mutex and capturing, so any frame already inside the handoff finishes or rolls back before capture.

Only after the final authorization and fence check does the hook reserve a 16 MiB mutation-scratch slot, apply the at-most-1-MiB decoded Yjs update to the shadow, reject any result with pinned-Yjs `store.pendingStructs` or `store.pendingDs`, encode the shadow's resulting current state, and reject before authoritative apply if syntax, the 4 MiB current-state ceiling, or the process ledger fails.

Every authorization denial, timeout, cancellation, frozen recheck, shadow rejection, or reservation failure before handoff synchronously rolls the shadow back to the authoritative state, restores accounting, and releases the turnstile before the hook rejects.

The hook schedules a `setImmediate` failure fallback before resolving.

`onLoadDocument` reserves load scratch, applies persisted state to a temporary Yjs document inside its own `try`/`catch`, and releases immediately on decode or apply failure.

Before returning the validated temporary document, it registers a pending-load reservation and a `setImmediate` watchdog keyed by document name plus load generation.

After Hocuspocus synchronously applies the returned state but before it registers the authoritative document, Bridge's `afterLoadDocument` hook verifies the matching pending reservation and performs shadow construction, accounting shrink, direct synchronous Yjs `document.on("update")` listener installation, and exact `document.on("destroy")` cleanup installation inside one `try`/`catch`.

If any after-load initialization fails, its catch removes any partially installed listener, releases pending and committed accounting by generation, and rethrows while the watchdog remains armed.

Successful after-load initialization marks the pending generation initialized but does not cancel or release its watchdog.

At the next `setImmediate`, the watchdog finalizes the load only if Hocuspocus's document registry maps the name to that exact initialized instance; otherwise it removes any installed listeners and releases all reservation and resident accounting for the failed unregistered generation.

This ordering relies on Bridge's hook being the final `afterLoadDocument` extension and on no macrotask yield between the pinned load hooks and registry insertion; startup configuration assertions and the load tests enforce that invariant before a future extension can change it.

`beforeUnloadDocument` marks the exact document generation unloading, aborts and awaits any authorization, cancels and settles its pending admission or awaits its current handoff, then acquires the turnstile and rechecks the Hocuspocus registry still maps the name to the same instance.

It releases the turnstile without removing listeners or accounting because pinned Hocuspocus may still abort unload if a connection arrived while the hook awaited.

The transient unloading flag is cleared immediately before the hook returns; actual destruction is the point of no return, and a reconnect to a surviving instance may admit new work normally.

Only the synchronous Yjs `destroy` listener performs irreversible listener removal and exact instance-accounting release, so a post-hook aborted unload leaves the live document fully instrumented.

A later load uses a distinct generation and cannot be released by the old destroy listener.

Because the turnstile permits only one pending authoritative mutation, the listener correlates by pending admission identity, transaction-origin connection, and document instance rather than byte digest.

During the subsequent `MessageReceiver.apply`, it encodes the actual authoritative state, commits that state and exact accounting, cancels the fallback, and releases the turnstile before the apply call returns.

If `MessageReceiver.apply` throws or emits no matching document update, the fallback encodes the actual authoritative document without assuming it is unchanged, rebuilds the shadow from that actual state, reconciles accounting, and releases the turnstile before another mutation may validate.

Awareness, stateless, query, and sync-step-one messages never acquire the mutation turnstile or reserve mutation state.

After success the scratch slot shrinks to twice the encoded current-state size, accounting for the authoritative and shadow serialized-state baselines; that accounting is released only when those exact instances unload.

These ledgers bound admitted encoded inputs, scratch concurrency, encoded states, capture buffers, and cached responses, not the Yjs implementation's unobservable object overhead; the design makes no exact JavaScript-heap bound claim.

An update whose shadow result exceeds the ceiling fails closed with a visible size-limit error instead of changing the authoritative document.

Transient process-ledger exhaustion closes with retryable code 1013 and the existing bounded reconnect policy; deterministic document-size rejection uses a distinct visible non-retryable reason.

The sum of those counters for the authoritative canvases in one freeze request must not exceed 16 MiB.

Before the first snapshot encode of a nonempty authoritative list, the operation synchronously reserves 64 MiB from one process-wide 256 MiB capture-and-cache ledger.

Failure to reserve returns 503 before any connection close and therefore produces the degraded warning.

The transport, shadow-result, resident-document, per-session, and capture-reservation limits are hard admission bounds that run before authoritative Yjs apply or snapshot encode, while the 8 MiB per-snapshot and 32 MiB aggregate limits independently validate actual encoded output.

After capture, the reservation shrinks atomically to the actual cached entry and response byte accounting, never exceeding the existing 48 MiB response limit.

Matching complete, unfreeze, or token-conditional expiry releases that exact capture accounting; document unload releases the separate resident-document accounting by instance identity.

Serializer registry lookup-or-create, enqueue, last-dequeue eviction, and map-entry identity checks execute synchronously without an `await` between check and mutation.

Only the last dequeued serialized operation may evict its serializer when the queue is empty and no active freeze-map entry remains, including after session end and lease expiry.

Hocuspocus captures `performance.now()` before starting the Go validation callback.

The conservative monotonic deadline is the pre-request monotonic start plus the returned `remainingMs`, so callback latency is subtracted rather than extending the database lease.

The freeze operation deadline is the earlier of that conservative deadline and its two-second internal bound.

The operation rechecks the monotonic deadline, cancellation state, and expected token before map installation and before every later stage.

The callback uses `fetch` with an owned `AbortController` and a deadline no greater than the remaining operation budget.

When cancellation or the internal deadline wins, the operation is marked inactive, the callback is aborted, and no new snapshot or connection-close stage may start.

One document's snapshot encoding, digest, and entry construction are synchronous Bun runtime operations performed only while the serializer is held and the operation remains active.

Hocuspocus yields with `setImmediate` between documents so other sessions can process websocket and awareness traffic.

Only after every document capture succeeds and the immutable bundle is cached does the handler emit a 200 response incrementally by entry with stream backpressure; it never constructs one 48 MiB `JSON.stringify` value.

Each response writer takes a reference on that cached token entry, streams outside the per-session serializer, owns an `AbortController`, has a 250-millisecond no-progress timeout reset by successful writes, and has an absolute deadline at the earlier of two seconds from that writer's start or the cached token's remaining monotonic lease.

Abort forcibly destroys the HTTP response and the writer's settled promise releases its reader reference by token and entry identity.

Cached entries are re-streamed through a new writer over the same immutable cached entry for an idempotent retry, so a stalled first writer cannot block serializer access by the retry.

Complete aborts all matching writers inside the serializer, awaits their bounded settlement, and releases cached bytes only after the last reader reference reaches zero.

Every save-mutex acquisition is awaited with the deadline before synchronous encoding begins.

The serializer remains held until the callback and every started save-mutex wait settle or acknowledge abort, but not while an immutable cached response is streamed under a reader reference.

No postgres.js query, pool checkout, cancel connection, or snapshot database write exists in the Hocuspocus freeze path.

No detached continuation may encode a new snapshot, mutate the map, close a connection, or publish a new cache result after the serializer is released; every immutable response writer is registered, deadline-owned, reference-counted, and settled before its entry is freed.

An unfreeze for one token cannot clear a newer or concurrent token.

Hocuspocus stores the conservative monotonic deadline computed from the pre-callback start; it never starts a fresh full-duration timer when validation returns.

It does not compare a Go- or Node-generated wall-clock timestamp to the database expiry.

Every expiry timer and lazy or sweep cleanup reacquires the per-session serializer, aborts and settles matching response writers, and removes the map entry only when both its expected token and entry identity still match.

A delayed timer for an old token cannot delete a replacement token.

The same serialized cleanup synchronously performs the serializer-eviction check after removing an expired entry.

Inside the serializer, a freeze installs or replaces an entry only after the fresh Go callback proves that its exact token owns the current unexpired lease.

The Go validation transaction takes the same shared session advisory lock as mutation authorization, so it observes any preceding exclusive abort cleanup before Hocuspocus can install an entry.

No expiry ordering or timestamp tie-break is used for token replacement because no two validations for the same session may mutate the map concurrently.

Bridge currently supports one Hocuspocus process for realtime documents.

Running multiple Hocuspocus replicas is unsupported until a coordinated document fan-out and flush protocol exists, even though the durable lease prevents post-freeze writes across processes.

### Internal Hocuspocus API

Hocuspocus exposes three internal HTTP operations:

- `POST /internal/canvas-sessions/freeze`
- `POST /internal/canvas-sessions/unfreeze`
- `POST /internal/canvas-sessions/complete`

All three operations require `Authorization: Bearer <HOCUSPOCUS_CONTROL_SECRET>`.

Freeze accepts a strict JSON body of `{ "sessionId": "<uuid>", "freezeToken": "<uuid>", "canvasIds": ["<uuid>"] }`.

The canvas list is sorted, unique, and limited to the existing 50-canvas session cap.

An empty `canvasIds` list is valid and returns `{ "snapshots": [], "closed": 0 }` without document lookup or capture allocation after token validation and fence installation.

Unfreeze and complete accept `{ "sessionId": "<uuid>", "freezeToken": "<uuid>" }`.

Unknown fields, a missing field, and a malformed session or freeze-token UUID are rejected.

The freeze success response is `{ "snapshots": [{ "canvasId": "<uuid>", "stateBase64": "<base64>", "sha256": "<lowercase hex>" }], "closed": <number> }`.

Snapshots are sorted by canvas ID and include exactly the supplied canvases currently loaded in the responding Hocuspocus process.

An empty snapshot list is a valid confirmed result when none of the authoritative canvases is loaded.

The unfreeze success response is `{ "unfrozen": true }` when the matching operation was cleared or no operation is active.

The complete success response is `{ "released": true }` only after the matching active operation and response writers have been aborted and settled and the completed bundle, fence entry, and capture reservation were released or were already absent.

Complete never reopens an ended connection and a foreign active token returns `409 freeze_token_mismatch`.

A matching complete during validation, capture, or response writing becomes the final serialized terminal cleanup and does not report success until that bounded cleanup settles.

The Go client treats a non-2xx response, timeout, transport error, invalid JSON, duplicate or unexpected canvas ID, invalid base64 or digest, missing field, negative count, or wrong field type as a freeze failure.

Each decoded snapshot is limited to 8 MiB, total decoded snapshots are limited to 32 MiB, and the complete JSON response is limited to 48 MiB.

Hocuspocus checks decoded limits before base64 encoding, and Go enforces the response limit while reading plus the decoded limits independently.

Any limit violation returns or becomes a degraded freeze failure; no partial bundle is persisted.

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

### Go lease-validation callback

Hocuspocus calls `POST /api/internal/canvas-sessions/freeze-auth` with the same control bearer and strict `{ "sessionId": "<uuid>", "freezeToken": "<uuid>" }` body.

The Go handler takes the shared transaction-scoped session advisory lock, then returns `{ "allowed": true, "remainingMs": <positive integer> }` only for the exact current live unexpired lease.

Denied, expired, missing, malformed, and internal-failure responses are fail-closed and expose no session or user data.

The callback uses the existing server-to-server Go origin and is protected by the same loopback-or-verified-HTTPS rules as the reverse control call.

### Freeze behavior

Hocuspocus maintains the operation-owned in-memory freeze map described above.

The freeze operation validates the supplied token through Go, then installs the matching in-memory entry before awaiting document work.

For every supplied canvas ID whose `canvas:{canvasId}` document is currently loaded, the operation acquires the document's existing save mutex, waits for any ordinary store already in flight, and synchronously encodes the current Yjs state.

Hocuspocus computes SHA-256 over the decoded update bytes, applies the per-snapshot and aggregate limits, and adds the base64 state plus digest to the response bundle.

The digest detects framing or transport corruption only; it is not an authenticity or authorization boundary because the mutually authenticated control channel and Go's authoritative membership validation provide trust.

Hocuspocus performs no database write in the freeze path.

The freeze operation fails as a whole if any mutex wait, encode, digest, or limit check fails.

The save mutex is held through encoding so an older in-flight ordinary store has completed before capture.

No canvas mutation can pass the installed fence while capture proceeds.

After every loaded authoritative canvas has been captured successfully, Hocuspocus synchronously closes its loaded canvas connections and returns the bundle.

Canvases absent from the response were not loaded in this Hocuspocus process and retain their already-persisted database state.

An ordinary store already in flight completes before capture because of the save mutex.

An ordinary store scheduled after capture can only encode the same fenced document state; it either commits before Go's bundle transaction and is overwritten by the identical or authoritative bundle state, or its existing live-session predicate observes the committed ended row and writes nothing.

If any capture errors before the deadline, the active operation records one shared failure result, starts no later capture or close stage, retains its token-owned barrier, and returns non-2xx.

Open connections remain temporarily fenced until Go ends the session, token-matched unfreeze runs after a database end failure, or the lease expires.

The failure result never reports a successful snapshot bundle or close count.

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

No timer, I/O callback, `setImmediate`, or other macrotask yield is permitted between the second check and completed Yjs apply and relay except the already-scheduled admission-failure fallback, which cannot run until the synchronous apply/onChange chain has either committed the reservation or failed.

The subsequent snapshot capture therefore includes every mutation accepted before the freeze boundary.

On a confirmed freeze path, no mutation accepted after the freeze boundary may apply, relay, or persist.

Mutation authorization remains uncached so a committed session end takes precedence over realtime availability and throughput.

Client-side 100-millisecond coalescing reduces mutation volume without weakening that server-side decision.

### Implicit replacement-session ends

Creating a new class session and starting a scheduled class session currently end any existing live session for that class inside `SessionStore.CreateSession` and `ScheduleStore.StartScheduledSession`.

Those status producers must not bypass lifecycle locking or silently imply a complete archive.

Session status still outranks Hocuspocus availability: replacement start does not wait for a freeze request and never fails merely because realtime is unavailable.

Both store transactions acquire a reserved two-`int4` class-replacement advisory guard using class ID hexadecimal `0x4252434C`, decimal `1112687436`, and the same signed UUID mapping applied to the class ID; they then read live session IDs sorted by the derived signed lifecycle advisory key and full UUID tie-break, acquire each distinct session lock in that order, and recheck `status = 'live'` before changing it.

Scheduled start first reads the planned row's class ID without a row lock, then begins the write transaction, takes the class-replacement guard, and re-reads the same still-planned schedule row `FOR UPDATE` before any session discovery, so it never holds the schedule row while waiting for the guard.

The global order is class-replacement guard, then sorted session-lifecycle locks, then the legacy one-`int8` session-create lock if a future transaction ever needs all three; mutation authorization takes only one session lock, so it cannot invert the order.

For every row still live after locking, the replacement transaction sets `status = 'ended'`, `ended_at`, `whiteboard_server_archive_complete = false`, clears any freeze lease, and returns the ended session ID plus the cleared token if present.

It never overwrites a true or false result on a row that an explicit end completed first.

After commit, the handler runs the same scheduled-session completion work as explicit end for every replaced session, emits the ordinary ended event, and best-effort calls token-matched complete for every returned token; missing Hocuspocus cleanup remains bounded by JWT and token expiry.

The create/start response adds `replacedSessions: [{ "id": "<uuid>", "whiteboardServerArchiveComplete": false }]`, which is empty when no live session was replaced.

The initiating teacher interface displays `Previous session ended, but its latest whiteboard changes may not have been archived.` once when that array is nonempty.

This is an intentionally degraded end rather than a confirmed snapshot path: starting the next class remains status-first, but the data-loss risk is durable and visible instead of null or silent.

The revised Plan 094 file scope must include both stores, both initiating handlers, their response types and teacher consumers, event emission, API documentation, and integration tests for these existing producers.

The same remediation deletes the legacy Next.js `PATCH /api/sessions/[id]` shadow handler, its live Drizzle `endSession` helper, and the unused Drizzle `createSession` helper; updates the shadow-route allowlist and `TODO.md`; and proves no remaining production TypeScript session-status writer exists while excluding test fixtures from that scan.

It does not rely on the current Next.js proxy to keep those status producers unreachable.

### Go end-session sequence

1. The existing handler authorizes the represented user as the session teacher under the existing tenancy rules.
2. In one short transaction, Go acquires the exclusive session advisory lock, acquires or replaces the operation-owned lease, reads the sorted authoritative canvas-ID list, commits, and releases every database lock before any HTTP call.
3. Go calls Hocuspocus freeze with the matching token, canvas IDs, and one overall two-second deadline; retryable serializer conflicts and transport-class failures retry the same token with bounded jitter inside that same budget so a cached completed bundle can be recovered.
4. Go size-limits, strictly decodes, membership-checks, base64-decodes, and digest-checks the complete response before opening the end transaction.
5. For a valid bundle, Go begins a confirmed `database/sql` transaction through the existing pgx driver and takes the exclusive session advisory lock.
6. The confirmed transaction first performs the token-and-unexpired conditional session update to `status = 'ended'`, archive-complete true, and cleared lease.
7. Only after that conditional update succeeds does the same transaction batch-update every returned canvas using parameterized arrays or `UNNEST`, requiring every ID to belong to the session and the affected count to equal the bundle count.
8. If the batch succeeds, snapshot rows and ended status commit atomically.
9. If the conditional update affects no row or any batch step fails, Go rolls back the entire confirmed transaction so no true status or snapshot survives.
10. Go then opens a separate degraded transaction under the exclusive session advisory lock, applies the lease ownership rules, ends with archive-complete false and no bundle snapshots, and clears the consumed lease.
11. A database error that prevents both the confirmed and degraded transactions leaves the session live, triggers matching lease cleanup plus best-effort Hocuspocus unfreeze, emits no ended event, and returns the existing safe 500-class error.
12. If either end transition succeeds, Go emits the existing session-ended event, schedules the existing completion work, and best-effort calls complete with the matching token so Hocuspocus releases the bundle and capture reservation promptly; token-conditional expiry remains the fallback if the acknowledgment is lost.
13. A freeze, bundle, acknowledgment, or unfreeze failure is logged as an actionable operational event without tokens, document content, user content, or internal response bodies.

A freeze request that completes after Go's timeout may leave its Hocuspocus operation frozen only until token-matched cleanup or the lease expiry.

That state is correct when the database end succeeds because the session is ended.

If the database end subsequently fails, token-matched cleanup restores collaboration immediately when available and the lease expiry bounds the failure when cleanup is unavailable.

### Confirmed and degraded guarantees

A confirmed path requires a successful Hocuspocus fence and snapshot capture followed by atomic Go-owned bundle persistence and the matching, unexpired, advisory-lock-protected end update.

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

Expiry delays beyond Bun's maximum supported timer duration are chained or clamped and rechecked, while already expired tokens close immediately.

The close applies to writable and read-only canvas connections.

A per-connection hook performs the current internal authorization check before that connection is admitted to the document.

Permanent viewer or ended-session authorization applies `connectionConfig.readOnly = true`.

An active temporary freeze does not mutate `connectionConfig.readOnly`; although the pinned property is technically mutable, Bridge treats promotion to read-only as a one-way per-connection invariant because reversing it across concurrent message handling is unsupported and racy.

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

Canvas create, visibility update, delete, and floor mutation first take the shared session advisory lock and then their existing session-row lock, reject an unexpired freeze lease with `409 session_end_in_progress`, and hold both through their write transaction.

Lease acquisition takes the exclusive form of the same advisory lock before its session-row access, so the authoritative canvas-ID list cannot gain or lose a row between lease acquisition and the atomic end transaction.

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
- Class-session creation and scheduled-session start acquire the class guard plus lifecycle locks sorted by derived key and UUID, replace only rows still live, persist archive-complete false, clear and complete any returned token, complete associated in-progress scheduled sessions, emit ended events, return the replaced-session metadata, and succeed when Hocuspocus is unavailable.
- The legacy Next.js session PATCH route, live TypeScript end helper, and unused create helper are removed, the shadow-route allowlist and `TODO.md` are updated, and a production-source scan plus route test proves no TypeScript session-status end producer remains while ignoring test fixtures.
- A replacement racing an explicit confirmed end never overwrites the explicit true result; the opposite lock ordering and the legacy advisory caller are exercised under a deadlock timeout.
- Overlapping end requests cannot clear each other's leases or turn a stale snapshot into a successful archive result.
- A timed-out freeze response arriving after cleanup cannot reinstall or prolong the cleared lease.
- A crashed end request stops blocking mutations after the controlled 15-second lease expiry.
- A request that resumes after its lease expires still ends the session but must persist and return `whiteboardServerArchiveComplete: false`.
- A stale request cannot end or clear a session owned by a different unexpired lease token.
- A stale request encountering a different expired token may end status-first but must persist `whiteboardServerArchiveComplete: false` and must not reuse either freeze result.
- The lease-and-list transaction commits and releases its exclusive advisory lock before the freeze HTTP request, while the Hocuspocus validation callback acquires and releases the shared form without self-deadlock.
- A lost freeze response after successful capture is retried with the same token inside the original deadline and recovers the identical cached bundle without recapture.
- The atomic successful end update and shared advisory-lock mutation authorization prevent a post-expiry frame from being admitted between the success predicate and commit on a confirmed path.
- A degraded-path test pauses multiple independently authorized frames across several connections, commits the incomplete end, resumes the concurrent fan-in, and proves transient applies cannot persist while every later mutation and reconnect is denied.
- Session advisory locks are always acquired before database reads or row locks, and busy-session tests prove end completion under concurrent mutation authorization, canvas creation, visibility, deletion, floor changes, and the legacy one-argument session advisory-lock caller without deadlock or exceeding the approved bound.
- Lease acquire, replace, abort, and complete use the reserved exclusive advisory key, while mutation and Hocuspocus validation use its shared form; a paused validation observes a preceding abort commit.
- Go, TypeScript, and PostgreSQL fixture vectors produce the exact same signed second advisory key for all five boundary UUID prefixes.
- Lease acquisition, replacement, mutation authorization, Go freeze validation, and end completion use database `clock_timestamp()` semantics under controlled tests.
- Bundle validation rejects oversized HTTP, oversized decoded state, excessive aggregate state, duplicate or unexpected IDs, invalid base64, digest mismatch, and more than 50 entries before the end transaction.
- A valid subset bundle atomically updates exactly its loaded canvas rows and ends true; an invalid or affected-count-mismatch bundle rolls back the preceding conditional true update, updates no snapshots, and ends false only in the separate degraded transaction.
- A controlled lease-expiry boundary between the conditional true update and batch write cannot partially commit: batch success commits both under the acquired lock, while any injected batch failure rolls back both and takes the separate degraded path.
- A zero-snapshot confirmed bundle preserves unloaded persisted states and ends true.
- The end transaction rolls back both snapshot rows and session status on any database error.
- Teacher end authorization and cross-user isolation remain covered.
- Canvas creation covers teacher, present participant, invited participant, left participant, public outsider, platform admin, impersonator, ended session, and cap races.
- Create, visibility, delete, and floor mutation each reject an active freeze lease, proving the authoritative canvas list stays stable.
- Floor mutation covers teacher authorization, the three allowed values, rejection of `session`, and the existing row-lock invariant.
- The exact Plan 094 mint and ended-mutation test names are present rather than being represented only by broad table tests.

### Hocuspocus tests

- Freeze authenticates the bearer secret and validates strict input.
- The installed pinned `Server` preserves global `ws.maxPayload = 100 MiB`; existing attempt, chapter, broadcast, and longest session namespaces accept representative messages above the canvas limit, while the parsed canvas admission path accepts exactly 1,048,576 decoded update bytes and rejects 1,048,577 before shadow apply.
- A mutation racing a successful freeze proves that the last accepted scene is present in the returned bundle before connections close and is later persisted by the Go integration test.
- A mutation awaiting authorization when freeze begins is rejected before Yjs apply and relay.
- Mutation ordering acquires the admission turnstile with owned cancellation before uncached authorization, rechecks the fence after every awaited grant and authorization, and freeze awaits that same turnstile with its deadline after installing its fence before capture.
- A synchronous per-document admission counter permits exactly eight active-plus-queued mutations before allocating waiter resources; the ninth closes 1013, and a saturated burst followed by freeze or unload settles all eight generations without a timer, listener, promise, or counter leak.
- Half-open authorization, connection close, unload, and deadline each abort and settle the exact fetch; a cancelled turnstile grant releases synchronously on arrival; freeze cannot remain blocked behind either resource.
- Authorization denial, timeout, cancellation, frozen recheck, pending-struct rejection, shadow error, and ledger exhaustion each restore shadow/accounting and release the turnstile without entering `MessageReceiver.apply`.
- Two successive frames on one connection perform two Go rechecks, and a durable lease acquired between them is observed even when the local freeze map is empty.
- The installed promise microtask from the second freeze check through Yjs apply cannot be interleaved by the freeze HTTP macrotask.
- Snapshot capture waits for an already in-flight ordinary store through the shared save mutex, then returns the current encoded state without writing PostgreSQL.
- A capture or size-limit failure starts no later capture or connection close, returns the same failure to active duplicates, and retains only the matching barrier until lifecycle cleanup.
- An empty authoritative canvas list validates and fences the token, returns the exact empty success response, performs no document lookup or capture reservation, and completes as a confirmed end.
- Unfreeze is token-scoped and idempotent, and stale unfreeze cannot clear a newer freeze.
- A different active unfreeze token returns `409 freeze_token_mismatch`.
- A paused freeze validation and queued unfreeze serialize so that unfreeze is the final map action and the late validation cannot reinstall the barrier.
- A freeze request that arrives after its matching unfreeze revalidates through Go, observes the cleared lease, and cannot install a barrier.
- A cancelled or timed-out validation continuation cannot mutate the map after the per-session serializer is released.
- A validation response delayed beyond database expiry cannot install a barrier because elapsed monotonic time is subtracted and checked before installation.
- Each save-mutex wait and Go-validation fetch is paused across timeout and queued unfreeze to prove the serializer and cleanup wait for abort settlement; after acknowledgment, no encode, close, map, or result effect can occur after release.
- Duplicate operations coalesce, foreign tokens conflict without queueing, the queue stays bounded, and an idle serializer is evicted.
- A post-success duplicate receives the byte-identical cached bundle rather than an empty recapture; complete, unfreeze, and expiry release only its exact cache reservation, and a stale cleanup cannot release a replacement token's reservation.
- Matching complete paused during validation, save-mutex wait, capture, and response streaming cancels and settles the active work and readers, becomes the final serialized action, and cannot permit a late cache publish or release a replacement token's bytes.
- Transport abort after successful capture followed by a same-token request re-streams the identical entries, while successful complete acknowledgment frees them promptly and lost acknowledgment falls back to token-conditional expiry.
- A half-open first response hits its no-progress timeout, destroys and settles its writer, permits a same-token cached retry inside the overall deadline, and protects cached bytes with reader references until complete settles every writer.
- `ws` rejects every oversized websocket message during reassembly with code 1009; persisted-state load, shadow-result mutation, per-session aggregate, resident-document, and concurrent capture reservations each fail at their exact boundary before authoritative Yjs apply, snapshot encode, or connection close.
- An accepted reservation followed by a real installed-path `MessageReceiver.apply` failure exercises the `setImmediate` fallback, reconciles from the actual authoritative state, rebuilds the shadow, and releases the turnstile; successful ordinary, duplicate, and partially overlapping updates commit by pending identity plus origin/document rather than byte digest.
- A dependency-missing Yjs update is rejected from the shadow before authoritative handoff, and the later complete update can succeed without parked pending structs or ledger drift.
- Temporary-document load failure releases in the hook's `catch`; Hocuspocus authoritative apply failure is reclaimed by the unclaimed pending-load watchdog even though `afterLoadDocument` never ran.
- Failure during shadow construction, accounting shrink, either listener installation, a later after-load extension, or pre-registry completion leaves the watchdog armed and releases every partial resource; only the next-turn exact registry-and-generation check finalizes a successful load.
- Startup and controlled scheduling prove Bridge is the final after-load extension and no macrotask yield occurs before pinned registry insertion.
- Paused mutation authorization followed by last-socket disconnect makes unload abort and settle the admission under the turnstile before exact listener/accounting release, while a concurrent reload's new generation remains untouched.
- A reconnect arriving during `beforeUnloadDocument` makes pinned Hocuspocus abort destruction; the transient unloading flag clears and the surviving document retains its listener and accounting, while actual destroy releases both exactly once.
- Concurrent capture reservation never exceeds the 256 MiB capture-and-cache ledger, resident documents never exceed their separate 128 MiB ledger, and rejection or unload releases only the exact instance reservation.
- The maximum accepted document and 50-canvas request stay within the declared pre-reserved allocation; controlled instrumentation proves no encode begins without its 64 MiB reservation.
- Incremental response streaming honors backpressure and request abort, yields between document encodes, never constructs one aggregate JSON string, and leaves other-session websocket and awareness work schedulable.
- Serializer lookup, enqueue, last-dequeue eviction, expiry sweep, and token-conditional timer cleanup interleave under controlled scheduling without creating two serializers or deleting a replacement token.
- Go-validation timeout, zero remaining budget, and half-open HTTP each settle through owned `AbortController` cancellation, release cleanup, and permit no late effect or open handle.
- All microtask, `setImmediate`, response-destroy, backpressure, and maximum-timer-clamp scheduling tests execute under the production Bun runtime.
- The maximum 50 loaded canvases produce a bounded snapshot bundle inside the two-second Go deadline under the approved representative test latency.
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
- Class-session creation and scheduled-session start show the prior-session archive warning exactly once when `replacedSessions` is nonempty and show none for an empty array.
- Incomplete server-archive status survives the redirect, remains durably discoverable, and displays exactly once per archive visit.
- A non-teacher archive visitor receiving 403 from teacher-only canvas settings renders the archive without a completeness claim or settings error, while teacher true/false/null behavior remains distinct.
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
- A successful confirmed end captures every server-accepted loaded-canvas mutation before closing its connections, then persists the returned snapshots before committing the confirmed database end; a successful freeze followed by database failure makes no persistence claim.
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

Findings are deduplicated by concrete failure scenario and violated invariant, not by wording.

“Reopened twice” means the same scenario is found in two later verdict rounds after two separate response commits.

“No net reduction” compares the total deduplicated `[OPEN]` blocker count at consecutive checkpoints after incorporating both resolved and newly raised blockers.

These triggers apply prospectively once the governance rule is merged; Spec 013's current resumed cycle already took its explicit user-decision pause at Round 8.

After user direction, the uncapped consensus process resumes with round numbering preserved.

A passing design gate authorizes plan drafting or revision, not implementation.

The plan-review gate remains separately required before implementation.

The canonical rule is mirrored in `AGENTS.md`, `docs/reviewers.md`, `docs/development-workflow.md`, and `docs/coding-agent.md` in the Plan 094 remediation scope before those governance files are changed.

## Design Review

### Round 1 — 2026-08-10 — commit `b589dd7`

- `[FIXED]` `[sol][fable]` A global boolean freeze can be cleared by an overlapping end attempt, can reappear after a delayed request, and can strand a live session when best-effort unfreeze fails.
  The revision replaces it with a token-owned PostgreSQL lease, matching in-memory operation records, conditional cleanup, single-flight conflict behavior, and a 15-second expiry.
- `[FIXED]` `[fable]` A Hocuspocus restart after successful freeze but before database end can admit new writes while Go still reports archive success.
  The durable lease is now checked by connection authorization and every mutation recheck, so process restart cannot erase the freeze boundary.
- `[FIXED]` `[sol]` A final flush can be overwritten by an older ordinary store completing later.
  The final path now shares the document save mutex and holds it through the database write.
- `[FIXED]` `[sol][fable]` The prior archive-complete field overstated what could be known while clients coalesce unsent scenes.
  The contract is narrowed and renamed to `whiteboardServerArchiveComplete`, which covers only Yjs updates accepted by the server.
- `[FIXED]` `[sol]` `onLoadDocument` does not run for every connection to an already loaded document.
  Current read-only authorization now occurs in a per-connection hook.
- `[FIXED]` `[fable]` Archive incompleteness was transient browser state.
  The result is now persisted atomically on the session row and exposed to the teacher archive.
- `[FIXED]` `[fable]` The control API reused the signing secret and had no separate listener requirement.
  The revision specifies a distinct control secret, constant-time comparison, and an internal-only listener.
- `[FIXED]` `[fable]` The no-yield ordering assumption, fixed deadline load, timer overflow, multi-process assumption, and migration-shipping check were underspecified.
  The revision makes the no-yield and single-process invariants explicit, adds timer and migration checks, and retains the teacher-selected two-second bound with maximum-cap test evidence required before approval.

**Round 1 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

### Round 2 — 2026-08-10 — commit `d572d25`

- `[FIXED]` `[sol][fable]` An end request could outlive its 15-second lease and still commit a pre-expiry successful archive result after writes resumed.
  → Response in `aaf2f44` and `260ca47`: the successful end uses an atomic token-and-unexpired `UPDATE`, mutation authorization takes a shared row lock, and every expired-lease path records an incomplete server archive.
- `[FIXED]` `[fable]` Go, Node, and PostgreSQL clocks could disagree about lease expiry.
  → Response in `aaf2f44`: PostgreSQL `clock_timestamp()` is the wall-clock authority, and Node derives only a monotonic remaining duration from a fresh database validation.
- `[FIXED]` `[fable]` Final-flush liveness and lease validation could be a check-before-write race.
  → Response in `aaf2f44`: the live, token, and unexpired predicates execute in the snapshot write statement or its locking transaction.
- `[FIXED]` `[fable]` The durable per-mutation lease check was underspecified and could be weakened by caching.
  → Response in `aaf2f44` and `260ca47`: every mutation-bearing frame retains the uncached, shared-row-lock Go recheck, and the test contract now proves two consecutive frames observe an intervening durable lease.
- `[FIXED]` `[fable]` Older null archive state had no defined public representation.
  → Response in `aaf2f44`: null omits the optional boolean and warning and produces no completeness claim in the archive.
- `[FIXED]` `[fable]` A non-loopback internal listener could send its bearer and freeze token over plaintext.
  → Response in `aaf2f44` and `260ca47`: non-loopback traffic requires verified HTTPS, the Go client rejects non-loopback HTTP at startup, and the configuration test matrix enforces both sides.
- `[FIXED]` `[sol]` The internal URL default incorrectly reused the public websocket port.
  → Response in `aaf2f44`: a distinct `HOCUSPOCUS_CONTROL_PORT` defaults to 4001 and drives the loopback internal URL.
- `[FIXED]` `[sol]` Durable archive status had no ended-session-readable producer-to-consumer API.
  → Response in `aaf2f44`: the canvas-settings GET contract defines its response, represented-teacher authorization, null behavior, 403, 404, and integration matrix.
- `[FIXED]` `[sol]` The no-microtask invariant contradicted Hocuspocus 3.4.4's promise continuation.
  → Response in `aaf2f44`: the actual invariant permits the installed microtask and forbids an interleaving macrotask before apply and relay.
- `[FIXED]` `[sol]` Design approvals were not bound to a commit and could survive an unreviewed material edit.
  → Response in `aaf2f44`: both reviewers approve an exact commit and must be re-dispatched after every substantive revision.

**Round 2 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

### Round 3 — 2026-08-10 — commit `aaf2f44`

- `[FIXED]` `[fable]` The end transaction could check expiry before its write and admit a post-expiry mutation before committing a successful result.
  → Response in `260ca47`: success is a single conditional `UPDATE`, and mutation authorization takes a shared row lock that blocks behind the update and observes the committed ended status.
- `[FIXED]` `[fable]` A stale request encountering a different expired lease token had no defined state transition.
  → Response in `260ca47`: status may still end, but the stale request must record an incomplete archive and cannot reuse either operation's freeze result.
- `[FIXED]` `[sol]` A temporary freeze could irreversibly set an established writer's connection to read-only.
  → Response in `260ca47`: temporary freeze rejects or closes with retryable `session_freezing`; only permanent viewer or ended decisions set read-only, and reconnect restores writing after cleanup or expiry.
- `[FIXED]` `[sol][fable]` The control transport rules lacked client-side non-loopback HTTP rejection and an executable configuration matrix.
  → Response in `260ca47`: Go fails startup on non-loopback HTTP, Node fails on invalid secret, TLS, or bind configuration, and the named test matrix covers listener isolation and verified HTTPS.
- `[FIXED]` `[sol]` The uncached mutation rule lacked a same-connection proof.
  → Response in `260ca47`: two consecutive frames must perform distinct rechecks and observe a durable lease acquired between them with an empty local map.
- `[FIXED]` `[fable]` A newly valid token had no rule when an older in-memory token's monotonic timer remained active.
  → Response in `260ca47`: a freshly database-validated token replaces the stale in-memory entry because PostgreSQL is authoritative.
- `[FIXED]` `[fable]` The review ledger attributed resolution text to the older reviewed commit.
  → Response in this ledger revision: every response names the later commit that contains it, while headings retain the exact reviewed SHA.
- `[FIXED]` `[fable]` The old settings route, normative freeze-result write, control-port bind failure, and browser-warning consumption order were underspecified.
  → Response in `260ca47`: the old route is removed with same-phase client migration, matched unexpired end must persist the result, bind failure aborts startup, and durable confirmation precedes one-shot consumption.

**Round 3 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

### Round 4 — 2026-08-10 — commit `7b2e644`

- `[FIXED]` `[sol]` A degraded end could commit after an authorization transaction released its lock but before the allowed frame applied and relayed.
  → Response in `18e9216`: the design no longer claims impossible cross-process apply-and-relay atomicity when Hocuspocus cannot retain a fence; it defines the confirmed guarantee separately and proves that a degraded transient frame cannot persist, continue, or survive reconnect while the teacher receives the incomplete warning.
- `[FIXED]` `[fable]` The ledger self-certified findings as fixed even though the exact response commit had not been approved.
  → Response in `18e9216`: every finding is restored to `[OPEN]`, responses never change status, and only exact-commit reviewer approval permits a mechanical ledger-only transition to `[FIXED]`.
- `[FIXED]` `[fable]` An older asynchronous database-validation response could overwrite a newer in-memory freeze token.
  → Response in `18e9216`: the map stores database expiry and accepts a different token only when its database expiry is later than the current entry.
- `[FIXED]` `[fable]` Foreign expired and already-ended lease cleanup states were undefined.
  → Response in `18e9216`: any consumed expired lease is cleared, an already-ended completion never rewrites the archive result, and matching or expired residue may be cleaned safely.
- `[FIXED]` `[fable]` A row-level shared lock on every mutation risked multixact churn and lacked a universal lock order.
  → Response in `18e9216`: mutation and end use transaction-scoped shared/exclusive session advisory locks, always acquired before database access, and the busy-session test covers deadlock and latency.
- `[FIXED]` `[fable]` Plain-HTTP loopback and redirect behavior was ambiguous.
  → Response in `18e9216`: only IP-literal `127.0.0.0/8` and `::1` are accepted for HTTP, DNS names are rejected, and redirects are disabled.
- `[FIXED]` `[fable]` The reconnect horizon could expire before the 15-second freeze lease.
  → Response in `18e9216`: reconnect continues for at least 20 seconds with no individual delay above two seconds, and the crash test requires recovery without reload.
- `[FIXED]` `[fable]` Durable/browser warning disagreement, permanent viewer promotion, and stale old-route clients were underspecified.
  → Response in `18e9216`: a 200 durable result is authoritative, failed requests retain browser state, promotion requires reconnect, and stale-route 404 is surfaced as an error.

**Round 4 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

### Round 5 — 2026-08-10 — commit `5e8b4ab`

- `[FIXED]` `[sol][fable]` A database validation started before cleanup can return after token-matched unfreeze removed an empty map entry and reinstall the stale freeze barrier.
  The later-expiry comparison has no state to compare when the map is empty, so the accepted design still needs per-session freeze/unfreeze serialization or a bounded tombstone or generation retained after removal.
- `[FIXED]` `[fable]` The acceptance criterion still states unconditional post-freeze apply-and-relay atomicity even though the body intentionally limits that guarantee to confirmed paths.
- `[FIXED]` `[fable]` The reconnect horizon must reset on every retryable freeze rejection to survive consecutive end attempts.
- `[FIXED]` `[fable]` Advisory-lock participation needs to cover lease acquisition and replacement explicitly, use a reserved two-part application keyspace, and define equal-expiry token ordering without timestamp truncation ambiguity.
- `[FIXED]` `[fable]` Already-ended residual-lease cleanup must be deterministic, stale-route errors must tolerate an unparseable non-2xx body, and loopback checks must use canonical parsed addresses while rejecting IPv4-mapped IPv6 forms.

**Round 5 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

**Historical gate result:** BLOCKED under the former five-round rule.
The five-round maximum is exhausted with unresolved `[OPEN]` findings, so the hard safeguard requires user direction before any further design revision, Plan 094 re-plan, governance-file edit, or implementation.

### User direction — 2026-08-11

The user approved reopening the design with per-session serialized freeze and unfreeze operations and removed the design-review round cap.

The historical Round 5 block is therefore resolved as a process decision, while its technical findings remain `[OPEN]` until Sol and Fable approve the same revised commit.

### Round 6 — 2026-08-11 — commit `fd80fde`

- `[FIXED]` `[sol]` Validation delay was not subtracted from the database remaining duration, so an already-expired token could receive a new local interval.
  → Response in `816256a`: Node anchors the conservative deadline before the database request, subtracts all validation and transport delay, and rechecks before every stage.
- `[FIXED]` `[sol]` Cancellation tests did not cover final-flush and connection-close continuations after timeout and queued unfreeze.
  → Response in `816256a`: the serializer remains held until every started stage settles or acknowledges cancellation, no new stage starts after cancellation, connection close is synchronous and last, and each stage gets a paused-continuation regression.
- `[FIXED]` `[fable]` The proposed uncapped gate contradicted canonical capped governance before those files were changed.
  → Response in `816256a`: the future rule becomes globally effective only through the reviewed governance edit; current repository rules remain authoritative elsewhere, while Spec 013 continues solely under explicit user direction.
- `[FIXED]` `[fable]` Hocuspocus lease validation did not participate in the advisory-lock order and could race Go abort cleanup.
  → Response in `816256a`: validation takes the shared session advisory lock and therefore observes preceding exclusive cleanup.
- `[FIXED]` `[fable]` Unfreeze and serializer queues lacked execution bounds and eviction.
  → Response in `816256a`: unfreeze is local and database-free, waits behind bounded cancellation settlement, duplicate operations coalesce, foreign tokens do not queue, the queue has two bounded positions, and idle serializers are evicted.
- `[FIXED]` `[fable]` Stale clients could accept a schema-invalid 2xx response from the removed route.
  → Response in `816256a`: success requires both 2xx and exact response schema; every parse or schema failure is surfaced.
- `[FIXED]` `[fable]` The degraded-path acceptance contract did not state a bound on later mutations and the consensus loop had no non-convergence checkpoint.
  → Response in `816256a`: only an already-authorized frame may be transient, all later frames recheck or fail closed, connections close by JWT expiry, and every three non-converged rounds produces a non-blocking user checkpoint.
- `[FIXED]` `[fable]` Advisory-lock namespace, loopback parsing, redirect, and uncategorized reconnect behavior needed tighter bounds.
  → Response in `816256a`: the two-key Bridge class is scoped honestly, canonical IP parsing rejects mapped forms, redirects remain disabled, and unexpected live disconnects continue bounded retries without manual reload.

**Round 6 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

### Round 7 — 2026-08-11 — commit `7bf6269`

- `[FIXED]` `[fable]` Server-side statement timeout could not bound pool checkout or a half-open client socket, leaving cancellation settlement and the serializer unbounded.
  → Response in `de1dbae`: each freeze uses a short-lived one-connection control client with connect/query/socket bounds, postgres.js cancellation, forced `end({ timeout: 0 })`, and zero-budget rejection.
- `[FIXED]` `[fable]` The spec claimed it could retrofit exact-schema handling into an already loaded pre-deploy browser bundle.
  → Response in `de1dbae`: the route and floor UI are explicitly unshipped feature-branch work, `main` has no old caller, and only the new client carries the exact-schema contract.
- `[FIXED]` `[sol]` The cross-language second advisory key did not define its hash algorithm or signed mapping.
  → Response in `de1dbae`: the key is the signed reinterpretation of the canonical UUID's first eight hex digits, with exact boundary vectors for Go, TypeScript, and PostgreSQL.
- `[FIXED]` `[sol]` A delayed expiry timer could remove a newer replacement entry without serialized token and identity checks.
  → Response in `de1dbae`: timer, lazy, and sweep cleanup reacquire the serializer, compare expected token and entry identity, and perform atomic eviction checks.
- `[FIXED]` `[sol]` The degraded path incorrectly bounded transient fan-in to one frame even though Hocuspocus authorizations run independently.
  → Response in `de1dbae`: the guarantee and regression cover any finite set already authorized across multiple connections, while all later frames recheck or fail closed.
- `[FIXED]` `[fable]` Transaction-scoped advisory functions, flush-error behavior, atomic serializer eviction, duplicate-result lifetime, and rightful replacement retry were underspecified.
  → Response in `de1dbae`: only `pg_advisory_xact_lock*` is allowed, failure retains a bounded token barrier without closing, registry changes are synchronous, result sharing ends at settlement, and foreign-token conflicts are bounded by the active deadline.
- `[FIXED]` `[fable]` Sustained reconnects, repeated reviewer non-convergence, and sweep-driven serializer cleanup lacked operational bounds.
  → Response in `de1dbae`: reconnect grows to a jittered 30-second tail after the fast window, repeated non-convergence becomes a user decision, and serialized sweep cleanup performs eviction.

**Round 7 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

### Round 8 — 2026-08-11 — commit `40cb07c`

- `[FIXED]` `[sol][fable]` Pinned postgres.js 3.4.9 does not provide owned, awaited cancellation and physical socket destruction through `query.cancel()` and `end({ timeout: 0 })`, so the proposed control client can leave detached query and cancel sockets.
- `[FIXED]` `[fable]` A one-connection non-pipelined control client contradicts the parallel or batched 50-canvas final-flush deadline, while using the ordinary pool would violate isolation and risk pool poisoning.
- `[FIXED]` `[fable]` A rightful replacement token had no actual Go retry loop within the freeze budget, so retryable conflict could degrade immediately despite the no-starvation claim.
- `[FIXED]` `[fable]` The permanent test contract incorrectly retained a pre-merge `main` history fact, the PostgreSQL form of the advisory key remained unstated, and non-convergence metrics needed exact definitions.
- `[FIXED]` `[fable]` Post-settlement duplicate freeze behavior and categorized-to-uncategorized reconnect backoff transitions remained ambiguous.

**Round 8 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

**User-decision pause:** final snapshot persistence must either move into Go's owned database transaction boundary or use a new Node database primitive with genuinely owned and awaited socket abort.

### User direction — 2026-08-11

The user approved moving final snapshot persistence into Go.

### Round 8 responses — commit `2695e6a`

- `[FIXED]` `[sol][fable]` Node could not own and await postgres.js query and cancel sockets.
  → Hocuspocus no longer performs any database validation or snapshot write during freeze; its only asynchronous external operation is an owned, abortable HTTP validation callback to Go.
- `[FIXED]` `[fable]` The one-connection Node design contradicted the 50-canvas capture deadline and isolated-pool promise.
  → Hocuspocus now captures Yjs updates synchronously under document save mutexes and returns a bounded bundle; Go performs one batch persistence transaction.
- `[FIXED]` `[fable]` Retryable replacement-token conflicts had no retrier.
  → Go retries with bounded jitter inside the original two-second budget and degrades honestly if no attempt succeeds.
- `[FIXED]` `[fable]` The PostgreSQL advisory-key expression, pre-merge history evidence, non-convergence metrics, duplicate success, and reconnect category transition were underspecified.
  → The SQL bit-cast expression is exact; history evidence is pre-merge rather than a permanent test; metrics are deduplicated and prospective; successful bundles are cached idempotently under a global bound; and uncategorized disconnects move to the jittered long-tail policy.

**Round 8 responses await Sol and Fable confirmation on commit `2695e6a`.**

### Round 9 — 2026-08-11 — commit `36132fc`

- `[FIXED]` `[fable]` Holding the exclusive advisory lock across the Hocuspocus request could self-deadlock when the freeze-validation callback tried to acquire the shared form.
  → Response in `73b0764`: the lease-and-list transaction commits and releases all database locks before the control request, and the test contract pauses the callback to prove the ordering.
- `[FIXED]` `[fable]` Go retried serializer conflicts but not a transport loss after Hocuspocus had captured and cached a successful result.
  → Response in `73b0764`: transport-class failures retry the same token inside the original two-second budget and recover the identical cached bundle without recapture.
- `[FIXED]` `[fable]` Canvas creation and related mutations did not participate in the lifecycle advisory-lock order, so the authoritative canvas set could change during lease acquisition.
  → Response in `73b0764`: create, visibility, delete, and floor transactions take the shared lifecycle lock before their session-row lock, while lease acquisition takes the exclusive form before reading the list.
- `[FIXED]` `[fable]` A completed cached bundle had no prompt release acknowledgment and could retain its large reservation until lease expiry.
  → Response in `73b0764`: the control API now has token-scoped complete acknowledgment after either database end result, with token-conditional expiry as the lost-ack fallback.
- `[FIXED]` `[fable]` Synchronous multi-document encoding and aggregate response construction could stall the event loop and multiply memory use.
  → Response in `73b0764`: Hocuspocus reserves capture capacity before encoding, yields between documents, and streams entries with backpressure without constructing one aggregate JSON string.
- `[FIXED]` `[fable]` Empty authoritative canvas lists and the SHA-256 trust role were ambiguous.
  → Response in `73b0764`: an empty list has an exact validated confirmed response, while the digest is explicitly framing and corruption detection rather than an authorization boundary.
- `[FIXED]` `[sol]` Snapshot rows were ordered before the conditional ended update, allowing lease expiry between the writes to contradict the no-bundle degraded contract.
  → Response in `73b0764`: the confirmed transaction performs the conditional true end first, batches snapshots only after it succeeds, and rolls both back before a separate false/no-snapshot degraded transaction on any failure.
- `[FIXED]` `[sol][fable]` Size checks and cache accounting began only after synchronous Yjs encoding, so a large document or concurrent captures could allocate beyond the intended bound before rejection.
  → Response in `73b0764`: transport, persisted-load, cumulative document, resident-process, request-aggregate, and capture-ledger admission limits run before apply or encode, with exact reservation and release tests.
- `[FIXED]` `[sol]` Acceptance claimed every successful freeze persisted snapshots even when the later database transaction failed.
  → Response in `73b0764`: persistence is claimed only for a successful confirmed end; a successful freeze followed by database failure makes no persistence claim.

**Round 9 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

**Round 9 responses await Sol and Fable confirmation on commit `73b0764`.**

### Round 10 — 2026-08-11 — commit `0509fca`

- `[FIXED]` `[sol][fable]` The new complete operation mutated serializer-owned state without participating in the serializer or defining its matching-active-token behavior.
  → Response in `5933b14`: freeze, unfreeze, and complete share the serializer; one coalesced terminal cleanup cancels and settles validation, capture, and writers, complete wins a matching terminal race, and identity-checked removal is the final action.
- `[FIXED]` `[sol]` Incremental response streaming had no owned deadline or reader lifetime, so a half-open first stream could block recovery or allow completion to free bytes still in use.
  → Response in `5933b14`: capture publishes an immutable cache before streaming, writers run outside the serializer with reader references and no-progress plus absolute deadlines, forced response destruction is awaited, and complete settles all readers before release.
- `[FIXED]` `[sol]` The admission contract promised rollback after downstream apply failure through `beforeHandleMessage`, but that hook cannot observe pinned Hocuspocus's internal `MessageReceiver.apply` result.
  → Response in `5933b14`: an admission turnstile spans the library handoff, a direct synchronous Yjs update listener commits the matching shadow reservation during apply, and a `setImmediate` failure fallback rebuilds and rolls back before releasing the turnstile.
- `[FIXED]` `[sol][fable]` Acceptance wording could be read to promise database persistence before Hocuspocus closed connections, contrary to the protocol sequence.
  → Response in `5933b14`: the criterion now separately orders capture before close and Go persistence before the confirmed database-end commit.
- `[FIXED]` `[fable]` Current response-failure prose did not explicitly prohibit starting a 200 stream before all capture stages succeeded.
  → Response in `5933b14`: a 200 response begins only after complete capture and immutable cache publication, preserving the non-2xx whole-capture failure contract.
- `[FIXED]` `[fable]` The spec named Node scheduling although production uses Bun, and its oversized-message wording promised mutation classification before buffering that `ws` cannot perform.
  → Response in `5933b14`: scheduling and timer rules name Bun and run under Bun tests; the exact pinned `Server` `maxPayload` option rejects every oversized fragmented websocket message with code 1009 during reassembly.
- `[FIXED]` `[fable]` Cumulative admitted-update counters could permanently reject a busy but compact live document.
  → Response in `5933b14`: admission now validates the shadow document's current encoded state rather than lifetime traffic, while a scratch ledger bounds concurrent validation and unload releases exact instance accounting.
- `[FIXED]` `[fable]` The lock-order registry omitted the existing disjoint one-argument advisory-lock class.
  → Response in `5933b14`: the registry names the legacy `int8` class, requires two-`int4` lifecycle lock first if both are ever needed, and includes the current legacy caller in deadlock testing.

**Round 10 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

**Round 10 responses await Sol and Fable confirmation on commit `5933b14`.**

### Round 11 — 2026-08-11 — commit `0af5bd0`

- `[FIXED]` `[sol][fable]` Class-session creation and scheduled-session start directly ended existing live sessions outside the lifecycle protocol, silently losing unflushed state and leaving archive completeness null.
  → Response in `4a2848d`: both producers use a class guard plus sorted session lifecycle locks, durably record an intentionally degraded false result, clear and complete tokens, emit ended events, return replacement metadata, and show the initiating teacher a warning without depending on Hocuspocus.
- `[FIXED]` `[sol][fable]` Turnstile acquisition was not ordered relative to authorization and the second fence check, so a waiting accepted frame could apply after confirmed capture.
  → Response in `4a2848d`: the mutation path acquires the turnstile before uncached authorization and rechecks after every yield, while freeze installs its fence and then acquires the same turnstile before save mutex and capture.
- `[FIXED]` `[sol][fable]` Digest correlation assumed Yjs emitted the incoming update bytes and fallback assumed an unchanged authoritative document, both false for partial overlap and pending structs.
  → Response in `4a2848d`: the unique pending admission correlates by identity, origin, and document; fallback reconciles actual authoritative state; partially overlapping updates are covered; and shadow results with pinned-Yjs pending structs or deletes are rejected before handoff.
- `[FIXED]` `[sol]` Pre-handoff authorization denial, timeout, cancellation, or frozen recheck had no explicit shadow, accounting, and turnstile rollback.
  → Response in `4a2848d`: every pre-handoff exit synchronously restores shadow and accounting and releases the turnstile before rejecting, with an exhaustive rejection-path test matrix.
- `[FIXED]` `[sol]` The `ws.maxPayload` envelope allowance was unspecified, so the exact boundary test was not falsifiable.
  → Response in `4a2848d`: the pinned constructor receives exactly 1,048,625 bytes with its byte-level derivation, and fragmented boundary tests accept that value and reject 1,048,626 with code 1009.
- `[FIXED]` `[fable]` Initial load could fail before `afterLoadDocument` installed cleanup, leaking its reservation.
  → Response in `4a2848d`: `onLoadDocument` installs document-destroy cleanup before returning state, and tests distinguish apply failure from successful after-load, unload, and reload identities.
- `[FIXED]` `[fable]` Cached retry writer deadlines, retryable ledger exhaustion, writer wording, and mutable-readOnly rationale were ambiguous.
  → Response in `4a2848d`: each writer gets a fresh deadline bounded by remaining lease, transient ledger exhaustion uses retryable 1013, retries use a new writer over the immutable entry, and one-way read-only is stated as a Bridge concurrency invariant rather than a library limitation.

**Round 11 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

**Round 11 responses await Sol and Fable confirmation on commit `4a2848d`.**

### Round 12 — 2026-08-11 — commit `555a663`

- `[FIXED]` `[sol]` Failed authoritative initial load could not rely on document destruction because pinned Hocuspocus never registers or destroys that failed document.
  → Response in `62840cf`: the hook releases its own temporary apply failures and registers a generation-keyed pending reservation whose `setImmediate` watchdog releases it unless successful `afterLoadDocument` claims it.
- `[FIXED]` `[sol]` Mutation authorization became load-bearing under the turnstile without an owned timeout, cancellation, or cancelled-grant settlement rule.
  → Response in `62840cf`: mutation auth uses an owned 500-millisecond `AbortController`; close, unload, deadline, and cancellation abort and await settlement; both mutation and freeze waiters release a late cancelled grant synchronously.
- `[FIXED]` `[sol]` Unload did not serialize with pending admission, so a stale authorization could resume after listener/accounting removal or document replacement.
  → Response in `62840cf`: unload marks the exact generation, aborts and settles authorization/admission under the turnstile, rechecks registry identity, removes exact listener/accounting, and cannot touch a reload generation.
- `[FIXED]` `[sol][fable]` The canvas-derived global `ws.maxPayload` applied to every shared realtime namespace and would reject existing non-canvas documents above 1 MiB.
  → Response in `62840cf`: the listener preserves its 100 MiB compatibility cap and the parsed canvas hook alone enforces exactly 1,048,576 decoded update bytes before shadow apply, with explicit cross-namespace regression coverage and no pre-buffer claim.
- `[FIXED]` `[fable]` Legacy Next.js and Drizzle session writers remained potential status-ending producers outside the protocol.
  → Response in `62840cf`: the remediation deletes the shadow PATCH route and unused TypeScript create/end helpers, updates the inventory, and proves no TypeScript end writer remains rather than relying on proxy reachability.
- `[FIXED]` `[sol][fable]` Replacement ends omitted scheduled-session completion work.
  → Response in `62840cf`: every replaced session runs the same scheduled completion step as explicit end before event emission and token cleanup.
- `[FIXED]` `[fable]` Replacement lock sorting, actual schedule-store method naming, and non-teacher archive settings behavior needed precision.
  → Response in `62840cf`: locks sort by derived signed key plus UUID, the method is `StartScheduledSession`, and a non-teacher 403 produces no completeness claim or archive error.

**Round 12 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

**Round 12 responses await Sol and Fable confirmation on commit `62840cf`.**

### Round 13 — 2026-08-11 — commit `24087c5`

- `[FIXED]` `[sol][fable]` The pending-load watchdog was cancelled inside `afterLoadDocument` before shadow/listener initialization and before pinned Hocuspocus registry insertion, so later failure could leak an unregistered generation.
  → Response in `18df349`: after-load initialization is caught while the watchdog remains armed; the next-turn watchdog alone finalizes an exact initialized instance after registry insertion or removes every partial resource otherwise, with the final-extension/no-macrotask invariant enforced at startup.
- `[FIXED]` `[sol]` Individual admission waits were bounded but an authenticated burst could allocate an unbounded waiter, timer, abort, and listener population before the turnstile.
  → Response in `18df349`: a synchronous per-document counter admits at most eight active-plus-queued mutations before any waiter allocation, the ninth closes retryably, and saturated freeze/unload tests prove exact settlement.
- `[FIXED]` `[fable]` Irreversible cleanup in `beforeUnloadDocument` was unsafe because pinned Hocuspocus can abort unload after that hook when a reconnect arrives.
  → Response in `18df349`: the hook only cancels and settles pending admission, clears its transient flag, and retains instrumentation; exact Yjs destruction alone removes listeners and accounting, so an aborted unload remains operational.
- `[FIXED]` `[fable]` The production-writer scan and TypeScript helper wording were insufficiently scoped.
  → Response in `18df349`: the deletion distinguishes the live end helper from dead create helper, names the allowlist and `TODO.md`, and limits the scan to production while excluding fixtures.

**Round 13 verdicts:** `[sol]` CHANGES REQUESTED; `[fable]` CHANGES REQUESTED.

**Round 13 responses await Sol and Fable confirmation on commit `18df349`.**

### Round 14 — 2026-08-11 — commit `aa34784`

- `[sol]` No material findings.
  The load watchdog, destroy-only unload cleanup, eight-admission cap, transport compatibility, complete status-producer census, replacement locking, scheduled completion, and test contracts are coherent against the pinned implementation.
- `[fable]` No material findings.
  Two non-blocking wording nits remain: generation-keyed release must be idempotent when destroy precedes the watchdog, which the exact-once test already requires; and the larger Go bundle limits intentionally remain as defense in depth against a faulty peer despite tighter honest-Node limits.

**Round 14 verdicts on exact commit `aa34784e1e51596bf1b6779176a86187fe3306ab`:** `[sol]` APPROVE; `[fable]` APPROVE WITH NITS.

**Design-review gate result:** PASSED by consensus with no open blockers.

Following the gate rule, every historical finding above is mechanically transitioned from `[OPEN]` to `[FIXED]`; response prose did not self-certify resolution before both reviewers approved the same substantive commit.
