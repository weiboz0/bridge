package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 094 Phase 14 — E2E stack attestation.
//
// These tests need no database: a purpose-built database/sql driver stands in
// for the pool so every branch of the handler (including "the live database
// name does not end in _test", which a real _test database cannot produce) is
// reachable deterministically, and so "no query ran" is directly observable.
// The live-database behaviour is covered separately in
// e2e_stack_integration_test.go against the validated _test database.

// ---------------------------------------------------------------------------
// Fake database/sql driver
// ---------------------------------------------------------------------------

type e2eStackFakeQuery struct {
	text string
	args []driver.Value
}

type e2eStackFakeDatabase struct {
	mu       sync.Mutex
	name     string
	lockSeen bool
	queryErr error
	queries  []e2eStackFakeQuery
}

func (f *e2eStackFakeDatabase) record(text string, args []driver.Value) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, e2eStackFakeQuery{text: text, args: args})
}

func (f *e2eStackFakeDatabase) snapshot() (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.name, f.lockSeen, f.queryErr
}

func (f *e2eStackFakeDatabase) queryCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.queries)
}

func (f *e2eStackFakeDatabase) lastQuery(t *testing.T) e2eStackFakeQuery {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.NotEmpty(t, f.queries, "expected at least one query to have run")
	return f.queries[len(f.queries)-1]
}

var (
	e2eStackFakeRegistryMu sync.Mutex
	e2eStackFakeRegistry   = map[string]*e2eStackFakeDatabase{}
	e2eStackFakeSeq        int
)

type e2eStackFakeDriver struct{}

func (e2eStackFakeDriver) Open(dsn string) (driver.Conn, error) {
	e2eStackFakeRegistryMu.Lock()
	defer e2eStackFakeRegistryMu.Unlock()
	fake, ok := e2eStackFakeRegistry[dsn]
	if !ok {
		return nil, fmt.Errorf("e2e stack fake driver: unknown handle %q", dsn)
	}
	return &e2eStackFakeConn{fake: fake}, nil
}

type e2eStackFakeConn struct{ fake *e2eStackFakeDatabase }

func (c *e2eStackFakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("e2e stack fake driver: Prepare is not supported")
}

func (c *e2eStackFakeConn) Close() error { return nil }

func (c *e2eStackFakeConn) Begin() (driver.Tx, error) {
	return nil, errors.New("e2e stack fake driver: transactions are not supported")
}

func (c *e2eStackFakeConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	values := make([]driver.Value, 0, len(args))
	for _, arg := range args {
		values = append(values, arg.Value)
	}
	c.fake.record(query, values)
	name, lockSeen, queryErr := c.fake.snapshot()
	if queryErr != nil {
		return nil, queryErr
	}
	return &e2eStackFakeRows{name: name, lockSeen: lockSeen}, nil
}

type e2eStackFakeRows struct {
	name     string
	lockSeen bool
	done     bool
}

func (r *e2eStackFakeRows) Columns() []string { return []string{"current_database", "exists"} }
func (r *e2eStackFakeRows) Close() error      { return nil }
func (r *e2eStackFakeRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.name
	dest[1] = r.lockSeen
	return nil
}

func init() { sql.Register("bridge_e2e_stack_fake", e2eStackFakeDriver{}) }

// newE2EStackFakeDB returns a *sql.DB whose single observe query answers with
// the supplied live database name and lock visibility, plus a handle that
// records every statement and argument the handler sent.
func newE2EStackFakeDB(t *testing.T, liveName string, lockSeen bool) (*sql.DB, *e2eStackFakeDatabase) {
	t.Helper()
	fake := &e2eStackFakeDatabase{name: liveName, lockSeen: lockSeen}

	e2eStackFakeRegistryMu.Lock()
	e2eStackFakeSeq++
	handle := fmt.Sprintf("%s#%d", t.Name(), e2eStackFakeSeq)
	e2eStackFakeRegistry[handle] = fake
	e2eStackFakeRegistryMu.Unlock()

	db, err := sql.Open("bridge_e2e_stack_fake", handle)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
		e2eStackFakeRegistryMu.Lock()
		delete(e2eStackFakeRegistry, handle)
		e2eStackFakeRegistryMu.Unlock()
	})
	return db, fake
}

// ---------------------------------------------------------------------------
// Router + request helpers (mirroring cmd/api/main.go)
// ---------------------------------------------------------------------------

const (
	// Any 64-character lowercase hex string is a well-formed nonce.
	e2eStackTestNonce    = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	e2eStackTestPath     = "/api/health/e2e-stack"
	e2eStackAbsentPath   = "/api/health/e2e-stack-not-a-real-route"
	e2eStackTestDBURL    = "postgresql://bridge@127.0.0.1:5432/bridge_test"
	e2eStackNonTestDBURL = "postgresql://bridge@127.0.0.1:5432/bridge_dev"
)

// buildE2EStackRouter reproduces the exact wiring in cmd/api/main.go: the
// handler is constructed and registered only when the flag is on, and it is
// handed the router's own NotFound handler.
func buildE2EStackRouter(t *testing.T, enabled bool, cfg E2EStackHandlerConfig) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	if enabled {
		cfg.NotFound = r.NotFoundHandler()
		NewE2EStackHandler(cfg).Routes(r)
	}
	return r
}

func e2eStackGet(t *testing.T, r chi.Router, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func e2eStackAttestURL(nonce string) string { return e2eStackTestPath + "?nonce=" + nonce }

type e2eStackResponse struct {
	status int
	header http.Header
	body   []byte
}

func capture(rec *httptest.ResponseRecorder) e2eStackResponse {
	return e2eStackResponse{status: rec.Code, header: rec.Result().Header.Clone(), body: rec.Body.Bytes()}
}

// ---------------------------------------------------------------------------
// Refusal paths
// ---------------------------------------------------------------------------

// With the flag off the route is never registered, so the path is not merely
// refused — it does not exist, and nothing touches the pool.
func TestE2EStack_DisabledReturns404AndRunsNoQuery(t *testing.T) {
	db, fake := newE2EStackFakeDB(t, "bridge_test", true)
	router := buildE2EStackRouter(t, false, E2EStackHandlerConfig{DB: db, DatabaseURL: e2eStackTestDBURL})

	got := capture(e2eStackGet(t, router, e2eStackAttestURL(e2eStackTestNonce)))
	absent := capture(e2eStackGet(t, router, e2eStackAbsentPath))

	assert.Equal(t, http.StatusNotFound, got.status)
	assert.Equal(t, absent.status, got.status)
	assert.Equal(t, absent.header, got.header)
	assert.Equal(t, absent.body, got.body)
	assert.Equal(t, "404 page not found\n", string(got.body))
	assert.Equal(t, 0, fake.queryCount(), "a disabled stack must not query the database")

	// An enabled router registers exactly that path; a disabled one does not.
	var routes []string
	require.NoError(t, chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, method+" "+route)
		return nil
	}))
	assert.Empty(t, routes, "no route may be registered while the flag is off")
}

func TestE2EStack_MalformedNonceReturns404AndRunsNoQuery(t *testing.T) {
	malformed := map[string]string{
		"missing query parameter": e2eStackTestPath,
		"empty":                   e2eStackTestPath + "?nonce=",
		"too short":               e2eStackAttestURL(strings.Repeat("a", 63)),
		"too long":                e2eStackAttestURL(strings.Repeat("a", 65)),
		"uppercase hex":           e2eStackAttestURL(strings.ToUpper(e2eStackTestNonce)),
		"non hex character":       e2eStackAttestURL(strings.Repeat("a", 63) + "g"),
		"leading whitespace":      e2eStackTestPath + "?nonce=%20" + e2eStackTestNonce,
		"trailing newline":        e2eStackTestPath + "?nonce=" + e2eStackTestNonce + "%0A",
		"wrong parameter name":    e2eStackTestPath + "?token=" + e2eStackTestNonce,
	}
	for name, target := range malformed {
		t.Run(name, func(t *testing.T) {
			db, fake := newE2EStackFakeDB(t, "bridge_test", true)
			router := buildE2EStackRouter(t, true, E2EStackHandlerConfig{DB: db, DatabaseURL: e2eStackTestDBURL})

			got := capture(e2eStackGet(t, router, target))
			absent := capture(e2eStackGet(t, router, e2eStackAbsentPath))

			assert.Equal(t, http.StatusNotFound, got.status)
			assert.Equal(t, absent.body, got.body)
			assert.Equal(t, absent.header, got.header)
			assert.Equal(t, 0, fake.queryCount(), "a malformed nonce must not reach the database")
		})
	}
}

// The parsed name comes from the exact string this process's pool was built
// from, so a non-_test URL refuses before any query runs.
func TestE2EStack_NonTestParsedNameReturns404(t *testing.T) {
	for name, poolURL := range map[string]string{
		"development database": e2eStackNonTestDBURL,
		"production database":  "postgresql://bridge@db.internal:5432/bridge",
		"test-like prefix":     "postgresql://bridge@127.0.0.1:5432/_testing",
		"empty name":           "postgresql://bridge@127.0.0.1:5432/",
		"not a postgres URL":   "mysql://bridge@127.0.0.1:3306/bridge_test",
		"empty URL":            "",
	} {
		t.Run(name, func(t *testing.T) {
			// The live name and the lock are both perfect; only the parsed
			// name is wrong, so nothing but the parsed name can explain a 404.
			db, fake := newE2EStackFakeDB(t, "bridge_test", true)
			router := buildE2EStackRouter(t, true, E2EStackHandlerConfig{DB: db, DatabaseURL: poolURL})

			got := capture(e2eStackGet(t, router, e2eStackAttestURL(e2eStackTestNonce)))
			absent := capture(e2eStackGet(t, router, e2eStackAbsentPath))

			assert.Equal(t, http.StatusNotFound, got.status)
			assert.Equal(t, absent.body, got.body)
			assert.Equal(t, 0, fake.queryCount(), "a non-_test parsed name must not reach the database")
		})
	}
}

// The live name is the only proof of where the pool is actually pointed; a
// parsed _test URL that resolves somewhere else must still refuse.
func TestE2EStack_NonTestLiveNameReturns404(t *testing.T) {
	for name, liveName := range map[string]string{
		"development database": "bridge_dev",
		"production database":  "bridge",
		"suffix in the middle": "bridge_test_shadow",
		"prefix only":          "_test_bridge",
		"empty name":           "",
	} {
		t.Run(name, func(t *testing.T) {
			db, fake := newE2EStackFakeDB(t, liveName, true)
			router := buildE2EStackRouter(t, true, E2EStackHandlerConfig{DB: db, DatabaseURL: e2eStackTestDBURL})

			got := capture(e2eStackGet(t, router, e2eStackAttestURL(e2eStackTestNonce)))
			absent := capture(e2eStackGet(t, router, e2eStackAbsentPath))

			assert.Equal(t, http.StatusNotFound, got.status)
			assert.Equal(t, absent.body, got.body)
			assert.Equal(t, absent.header, got.header)
			// It refused *after* consulting the live name, not before.
			assert.Equal(t, 1, fake.queryCount())
		})
	}
}

func TestE2EStack_UnseenLockReturns404(t *testing.T) {
	db, fake := newE2EStackFakeDB(t, "bridge_test", false)
	router := buildE2EStackRouter(t, true, E2EStackHandlerConfig{DB: db, DatabaseURL: e2eStackTestDBURL})

	got := capture(e2eStackGet(t, router, e2eStackAttestURL(e2eStackTestNonce)))
	absent := capture(e2eStackGet(t, router, e2eStackAbsentPath))

	assert.Equal(t, http.StatusNotFound, got.status)
	assert.Equal(t, absent.body, got.body)
	assert.Equal(t, absent.header, got.header)

	// It asked the right question: the pinned statement, the reserved lock
	// class, and the nonce-derived object id.
	query := fake.lastQuery(t)
	assert.Equal(t, e2eStackObserveQuery, query.text)
	require.Len(t, query.args, 2)
	assert.Equal(t, int64(store.E2EStackLockClass), query.args[0])
	assert.Equal(t, e2eStackObjectID(e2eStackTestNonce), query.args[1])
}

// Every reason a request can be refused must be indistinguishable from the
// route not existing: same status, same headers, same bytes.
func TestE2EStack_AllRefusalsAreByteIdentical(t *testing.T) {
	type scenario struct {
		name     string
		enabled  bool
		liveName string
		lockSeen bool
		queryErr error
		nilDB    bool
		poolURL  string
		target   string
	}
	scenarios := []scenario{
		{name: "flag off", enabled: false, liveName: "bridge_test", lockSeen: true, poolURL: e2eStackTestDBURL, target: e2eStackAttestURL(e2eStackTestNonce)},
		{name: "malformed nonce", enabled: true, liveName: "bridge_test", lockSeen: true, poolURL: e2eStackTestDBURL, target: e2eStackAttestURL("nope")},
		{name: "parsed name not test", enabled: true, liveName: "bridge_test", lockSeen: true, poolURL: e2eStackNonTestDBURL, target: e2eStackAttestURL(e2eStackTestNonce)},
		{name: "live name not test", enabled: true, liveName: "bridge_dev", lockSeen: true, poolURL: e2eStackTestDBURL, target: e2eStackAttestURL(e2eStackTestNonce)},
		{name: "lock unseen", enabled: true, liveName: "bridge_test", lockSeen: false, poolURL: e2eStackTestDBURL, target: e2eStackAttestURL(e2eStackTestNonce)},
		{name: "observe query error", enabled: true, liveName: "bridge_test", lockSeen: true, queryErr: errors.New("connection refused to postgresql://bridge:hunter2@127.0.0.1:5432/bridge_test"), poolURL: e2eStackTestDBURL, target: e2eStackAttestURL(e2eStackTestNonce)},
		{name: "nil pool", enabled: true, nilDB: true, poolURL: e2eStackTestDBURL, target: e2eStackAttestURL(e2eStackTestNonce)},
	}

	var reference *e2eStackResponse
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			cfg := E2EStackHandlerConfig{DatabaseURL: sc.poolURL}
			if !sc.nilDB {
				db, fake := newE2EStackFakeDB(t, sc.liveName, sc.lockSeen)
				fake.queryErr = sc.queryErr
				cfg.DB = db
			}
			router := buildE2EStackRouter(t, sc.enabled, cfg)

			got := capture(e2eStackGet(t, router, sc.target))
			absent := capture(e2eStackGet(t, router, e2eStackAbsentPath))

			assert.Equal(t, absent.status, got.status, "status must match an unregistered path on the same router")
			assert.Equal(t, absent.header, got.header, "headers must match an unregistered path on the same router")
			assert.Equal(t, absent.body, got.body, "body bytes must match an unregistered path on the same router")

			if reference == nil {
				captured := got
				reference = &captured
				return
			}
			assert.Equal(t, reference.status, got.status, "every refusal reason must share one status")
			assert.Equal(t, reference.header, got.header, "every refusal reason must share one header set")
			assert.Equal(t, reference.body, got.body, "every refusal reason must share one body")
		})
	}
	require.NotNil(t, reference)
}

// ---------------------------------------------------------------------------
// Success path
// ---------------------------------------------------------------------------

func TestE2EStack_NoStoreHeader(t *testing.T) {
	db, _ := newE2EStackFakeDB(t, "bridge_test", true)
	router := buildE2EStackRouter(t, true, E2EStackHandlerConfig{DB: db, DatabaseURL: e2eStackTestDBURL})

	rec := e2eStackGet(t, router, e2eStackAttestURL(e2eStackTestNonce))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		Fingerprint string `json:"fingerprint"`
		Instance    string `json:"instance"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, e2eStackFingerprint(e2eStackTestNonce, "bridge_test"), body.Fingerprint)
	assert.NotEmpty(t, body.Instance)
}

// ---------------------------------------------------------------------------
// Non-disclosure and refusal logging
// ---------------------------------------------------------------------------

func TestE2EStack_NeverLogsOrReturnsDatabaseURL(t *testing.T) {
	const secretUser = "bridge_e2e_secret_user"
	const secretPassword = "bridge_e2e_secret_password"
	testURL := "postgresql://" + secretUser + ":" + secretPassword + "@db.internal:5432/bridge_test?sslmode=disable"
	nonTestURL := "postgresql://" + secretUser + ":" + secretPassword + "@db.internal:5432/bridge_dev?sslmode=disable"

	objID := e2eStackObjectID(e2eStackTestNonce)
	key := int64(store.E2EStackLockClass)<<32 | objID
	forbidden := []string{
		testURL, nonTestURL, secretUser, secretPassword, "db.internal",
		e2eStackTestNonce,
		strconv.FormatInt(key, 10),
		strconv.FormatInt(objID, 10),
	}

	cases := []struct {
		name     string
		poolURL  string
		liveName string
		lockSeen bool
		queryErr error
		target   string
	}{
		{"malformed nonce", testURL, "bridge_test", true, nil, e2eStackAttestURL("nope")},
		{"parsed name not test", nonTestURL, "bridge_test", true, nil, e2eStackAttestURL(e2eStackTestNonce)},
		{"live name not test", testURL, "bridge_dev", true, nil, e2eStackAttestURL(e2eStackTestNonce)},
		{"lock unseen", testURL, "bridge_test", false, nil, e2eStackAttestURL(e2eStackTestNonce)},
		{"observe error carries the URL", testURL, "bridge_test", true, errors.New("dial " + testURL + " failed"), e2eStackAttestURL(e2eStackTestNonce)},
		{"success", testURL, "bridge_test", true, nil, e2eStackAttestURL(e2eStackTestNonce)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			db, fake := newE2EStackFakeDB(t, tc.liveName, tc.lockSeen)
			fake.queryErr = tc.queryErr
			router := buildE2EStackRouter(t, true, E2EStackHandlerConfig{
				DB:          db,
				DatabaseURL: tc.poolURL,
				Logger:      slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
			})

			rec := e2eStackGet(t, router, tc.target)
			for _, secret := range forbidden {
				if secret == "" {
					continue
				}
				assert.NotContains(t, logs.String(), secret, "refusal logs must never disclose %q", secret)
				assert.NotContains(t, rec.Body.String(), secret, "responses must never disclose %q", secret)
				for header, values := range rec.Header() {
					for _, value := range values {
						assert.NotContains(t, value, secret, "header %s must never disclose %q", header, secret)
					}
				}
			}
		})
	}
}

// The refusal reason is useful to an operator reading the server log and
// useless to a caller; it is also rate limited so a scanner cannot flood it.
func TestE2EStack_RefusalReasonIsLoggedAndRateLimited(t *testing.T) {
	var logs bytes.Buffer
	var mu sync.Mutex
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	advance := func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(d)
	}

	db, _ := newE2EStackFakeDB(t, "bridge_test", false)
	router := buildE2EStackRouter(t, true, E2EStackHandlerConfig{
		DB:          db,
		DatabaseURL: e2eStackTestDBURL,
		Clock:       clock,
		Logger:      slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})

	reasons := func() []string {
		var out []string
		for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
			if line == "" {
				continue
			}
			var record map[string]any
			require.NoError(t, json.Unmarshal([]byte(line), &record))
			reason, _ := record["reason"].(string)
			out = append(out, reason)
		}
		return out
	}
	countOf := func(reason string) int {
		n := 0
		for _, r := range reasons() {
			if r == reason {
				n++
			}
		}
		return n
	}

	malformed := e2eStackAttestURL("nope")
	wellFormed := e2eStackAttestURL(e2eStackTestNonce)

	e2eStackGet(t, router, malformed)
	require.Equal(t, 1, countOf("malformed_nonce"), "the first refusal of a reason is always logged")

	advance(time.Second)
	e2eStackGet(t, router, malformed)
	assert.Equal(t, 1, countOf("malformed_nonce"), "a repeat inside the window is suppressed")

	// A different reason is rate limited independently, so a suppressed reason
	// never hides another one.
	e2eStackGet(t, router, wellFormed)
	assert.Equal(t, 1, countOf("lock_unseen"), "a distinct reason logs immediately")
	assert.Equal(t, 1, countOf("malformed_nonce"))

	advance(e2eStackLogInterval - time.Second - time.Nanosecond)
	e2eStackGet(t, router, malformed)
	assert.Equal(t, 1, countOf("malformed_nonce"), "one nanosecond short of the window is still suppressed")

	advance(time.Nanosecond)
	e2eStackGet(t, router, malformed)
	assert.Equal(t, 2, countOf("malformed_nonce"), "exactly at the window the reason logs again")
	assert.Equal(t, 1, countOf("lock_unseen"), "the other reason kept its own window")

	for _, reason := range reasons() {
		assert.Contains(t, []string{"malformed_nonce", "lock_unseen"}, reason)
	}
}

// ---------------------------------------------------------------------------
// Instance identity
// ---------------------------------------------------------------------------

// The verifier fails a stack whose instance id changes, so the id must be
// minted once per process, not per handler, per module, or per request.
func TestE2EStack_InstanceIDIsStablePerProcess(t *testing.T) {
	first := e2eStackProcessInstanceID()
	assert.NotEmpty(t, first)
	assert.Equal(t, first, e2eStackProcessInstanceID(), "the process instance id must not be re-minted")

	// The gate exports the id through a shell environment block, so it must
	// stay inside scripts/check-e2e-stack.mjs's INSTANCE_PATTERN.
	assert.Regexp(t, regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`), first)
	assert.True(t, strings.HasPrefix(first, strconv.Itoa(os.Getpid())+"-"),
		"the instance id carries the pid so a restart is visible: %q", first)

	instanceOf := func(t *testing.T, router chi.Router) string {
		t.Helper()
		rec := e2eStackGet(t, router, e2eStackAttestURL(e2eStackTestNonce))
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		var body struct {
			Instance string `json:"instance"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		return body.Instance
	}

	dbA, _ := newE2EStackFakeDB(t, "bridge_test", true)
	dbB, _ := newE2EStackFakeDB(t, "bridge_test", true)
	routerA := buildE2EStackRouter(t, true, E2EStackHandlerConfig{DB: dbA, DatabaseURL: e2eStackTestDBURL})
	routerB := buildE2EStackRouter(t, true, E2EStackHandlerConfig{DB: dbB, DatabaseURL: e2eStackTestDBURL})

	sample := instanceOf(t, routerA)
	assert.Equal(t, first, sample, "the served id is the process id")
	for i := 0; i < 5; i++ {
		assert.Equal(t, sample, instanceOf(t, routerA), "repeated requests must report one id")
	}
	assert.Equal(t, sample, instanceOf(t, routerB), "a second handler in the same process reports the same id")

	// The encoding itself is pinned: pid, separator, hex randomness.
	assert.Equal(t, "4242-00ff", stringInstanceID(4242, []byte{0x00, 0xff}))
}

// ---------------------------------------------------------------------------
// Shared cross-language vector
// ---------------------------------------------------------------------------

type e2eStackVectorFile struct {
	Contract struct {
		Flag struct {
			Name         string `json:"name"`
			EnabledValue string `json:"enabledValue"`
		} `json:"flag"`
		TunnelOptIn struct {
			Name         string `json:"name"`
			EnabledValue string `json:"enabledValue"`
		} `json:"tunnelOptIn"`
		NoncePattern       string              `json:"noncePattern"`
		LockClass          int64               `json:"lockClass"`
		LockClassHex       string              `json:"lockClassHex"`
		Objsubid           int                 `json:"objsubid"`
		KeyDerivation      string              `json:"keyDerivation"`
		ObserveQuery       string              `json:"observeQuery"`
		Fingerprint        string              `json:"fingerprint"`
		TestDatabaseSuffix string              `json:"testDatabaseSuffix"`
		SuccessBody        map[string][]string `json:"successBody"`
		SuccessHeaders     map[string]string   `json:"successHeaders"`
		Paths              map[string]string   `json:"paths"`
		NonceQueryParam    string              `json:"nonceQueryParam"`
	} `json:"contract"`
	Cases []struct {
		Nonce       string `json:"nonce"`
		Database    string `json:"database"`
		Objid       int64  `json:"objid"`
		Key         string `json:"key"`
		Fingerprint string `json:"fingerprint"`
	} `json:"cases"`
	ComposeKeyBoundaries []struct {
		Objid int64  `json:"objid"`
		Key   string `json:"key"`
	} `json:"composeKeyBoundaries"`
	DisjointFrom struct {
		SessionLifecycleLockClass int64   `json:"sessionLifecycleLockClass"`
		ClassReplacementLockClass int64   `json:"classReplacementLockClass"`
		HashtextOneKeyHighWords   []int64 `json:"hashtextOneKeyHighWords"`
	} `json:"disjointFrom"`
}

func loadE2EStackVector(t *testing.T) e2eStackVectorFile {
	t.Helper()
	path := filepath.Join("..", "..", "..", "scripts", "tests", "e2e-stack-vector.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "the shared contract vector must exist at %s", path)
	var vector e2eStackVectorFile
	require.NoError(t, json.Unmarshal(raw, &vector))
	require.NotEmpty(t, vector.Cases)
	return vector
}

// The Go handler, the Bun hook, the Next.js route, and the gate script must
// derive identical keys and fingerprints; this pins the Go side to the file
// all four assert.
func TestE2EStack_MatchesSharedVector(t *testing.T) {
	vector := loadE2EStackVector(t)

	// Lock namespace.
	assert.Equal(t, vector.Contract.LockClass, int64(store.E2EStackLockClass))
	assert.Equal(t, vector.Contract.LockClassHex, fmt.Sprintf("%#x", store.E2EStackLockClass))

	// Nonce grammar: the exact pattern, and the behaviour it implies.
	assert.Equal(t, vector.Contract.NoncePattern, e2eStackNoncePattern.String())
	assert.True(t, e2eStackNoncePattern.MatchString(strings.Repeat("0", 64)))
	assert.False(t, e2eStackNoncePattern.MatchString(strings.Repeat("0", 63)))
	assert.False(t, e2eStackNoncePattern.MatchString(strings.Repeat("A", 64)))

	// The observe query is part of the contract, not an implementation detail:
	// it is what makes the proof per-database and one-key (objsubid = 1).
	assert.Equal(t, vector.Contract.ObserveQuery, e2eStackObserveQuery)
	assert.Contains(t, e2eStackObserveQuery, "d.datname = current_database()")
	assert.Contains(t, e2eStackObserveQuery, fmt.Sprintf("l.objsubid = %d", vector.Contract.Objsubid))

	// Test-database suffix.
	assert.True(t, hasTestDatabaseSuffix("bridge"+vector.Contract.TestDatabaseSuffix))
	assert.False(t, hasTestDatabaseSuffix("bridge_dev"))
	assert.False(t, hasTestDatabaseSuffix(vector.Contract.TestDatabaseSuffix[1:]))
	assert.True(t, hasTestDatabaseSuffix(vector.Contract.TestDatabaseSuffix))

	// Key derivation and fingerprint, including the non-ASCII database name.
	for _, tc := range vector.Cases {
		name := tc.Nonce[:8] + "…/" + tc.Database
		t.Run(name, func(t *testing.T) {
			objID := e2eStackObjectID(tc.Nonce)
			assert.Equal(t, tc.Objid, objID, "objid derivation drifted")
			key := int64(store.E2EStackLockClass)<<32 | objID
			assert.Equal(t, tc.Key, strconv.FormatInt(key, 10), "composed bigint key drifted")
			assert.Positive(t, key, "the composed key must stay a positive bigint")
			assert.Equal(t, tc.Fingerprint, e2eStackFingerprint(tc.Nonce, tc.Database), "fingerprint drifted")

			// And end to end: the handler serves exactly that fingerprint for
			// that live database name.
			db, _ := newE2EStackFakeDB(t, tc.Database, true)
			router := buildE2EStackRouter(t, true, E2EStackHandlerConfig{DB: db, DatabaseURL: e2eStackTestDBURL})
			rec := e2eStackGet(t, router, e2eStackTestPath+"?"+vector.Contract.NonceQueryParam+"="+tc.Nonce)
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

			var served map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &served))
			assert.Equal(t, tc.Fingerprint, served["fingerprint"])

			// The success body carries exactly the contract's Go fields.
			var fields []string
			for field := range served {
				fields = append(fields, field)
			}
			assert.ElementsMatch(t, vector.Contract.SuccessBody["go"], fields)
			for header, want := range vector.Contract.SuccessHeaders {
				assert.Equal(t, want, rec.Header().Get(header))
			}
		})
	}

	// The 31-bit boundaries of the object id stay inside one positive bigint.
	require.NotEmpty(t, vector.ComposeKeyBoundaries)
	for _, boundary := range vector.ComposeKeyBoundaries {
		key := int64(store.E2EStackLockClass)<<32 | boundary.Objid
		assert.Equal(t, boundary.Key, strconv.FormatInt(key, 10))
		assert.Positive(t, key)
	}

	// Path and flag names.
	assert.Equal(t, "BRIDGE_E2E_STACK", vector.Contract.Flag.Name)
	assert.Equal(t, "1", vector.Contract.Flag.EnabledValue)
	assert.Equal(t, "ALLOW_E2E_STACK_OVER_TUNNEL", vector.Contract.TunnelOptIn.Name)
	assert.Equal(t, "true", vector.Contract.TunnelOptIn.EnabledValue)

	db, _ := newE2EStackFakeDB(t, "bridge_test", true)
	router := buildE2EStackRouter(t, true, E2EStackHandlerConfig{DB: db, DatabaseURL: e2eStackTestDBURL})
	var routes []string
	require.NoError(t, chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, method+" "+route)
		return nil
	}))
	assert.Equal(t, []string{http.MethodGet + " " + vector.Contract.Paths["go"]}, routes,
		"the Go surface is exactly the contract's path, registered for GET only")
}
