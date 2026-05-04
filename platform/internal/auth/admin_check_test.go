package auth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubLookup is a hand-rolled adminLookup we can drive deterministically.
type stubLookup struct {
	calls atomic.Int32
	value bool
	err   error
}

func (s *stubLookup) LookupIsAdmin(_ context.Context, _ string) (bool, error) {
	s.calls.Add(1)
	if s.err != nil {
		return false, s.err
	}
	return s.value, nil
}

func TestCachedAdminChecker_HappyPath(t *testing.T) {
	stub := &stubLookup{value: true}
	checker := NewCachedAdminChecker(stub)

	got, err := checker.IsAdmin(context.Background(), "user-1")
	require.NoError(t, err)
	assert.True(t, got)
	assert.Equal(t, int32(1), stub.calls.Load())
}

func TestCachedAdminChecker_EmptyUserIDRejected(t *testing.T) {
	checker := NewCachedAdminChecker(&stubLookup{})
	_, err := checker.IsAdmin(context.Background(), "")
	require.Error(t, err)
}

func TestCachedAdminChecker_CacheHitSkipsLookup(t *testing.T) {
	stub := &stubLookup{value: true}
	checker := NewCachedAdminChecker(stub)

	for i := 0; i < 5; i++ {
		got, err := checker.IsAdmin(context.Background(), "user-1")
		require.NoError(t, err)
		assert.True(t, got)
	}
	assert.Equal(t, int32(1), stub.calls.Load(), "expected exactly one DB lookup across 5 calls")
}

func TestCachedAdminChecker_TTLExpiryRefetches(t *testing.T) {
	stub := &stubLookup{value: false}
	checker := NewCachedAdminCheckerWithSize(stub, 50*time.Millisecond, 16)

	_, err := checker.IsAdmin(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Equal(t, int32(1), stub.calls.Load())

	// Inside TTL → cache hit.
	_, _ = checker.IsAdmin(context.Background(), "user-1")
	assert.Equal(t, int32(1), stub.calls.Load())

	time.Sleep(70 * time.Millisecond)

	// After TTL → fresh lookup.
	stub.value = true
	got, err := checker.IsAdmin(context.Background(), "user-1")
	require.NoError(t, err)
	assert.True(t, got, "should reflect updated DB value after TTL expires")
	assert.Equal(t, int32(2), stub.calls.Load())
}

func TestCachedAdminChecker_DBErrorPropagates(t *testing.T) {
	dbErr := errors.New("connection refused")
	stub := &stubLookup{err: dbErr}
	checker := NewCachedAdminChecker(stub)

	got, err := checker.IsAdmin(context.Background(), "user-1")
	require.Error(t, err)
	assert.False(t, got, "fail-closed on DB error")
}

func TestCachedAdminChecker_CapacityEviction(t *testing.T) {
	stub := &stubLookup{value: true}
	checker := NewCachedAdminCheckerWithSize(stub, time.Hour, 3)

	// Fill the cache.
	for _, uid := range []string{"u1", "u2", "u3"} {
		_, err := checker.IsAdmin(context.Background(), uid)
		require.NoError(t, err)
	}
	assert.Equal(t, int32(3), stub.calls.Load())

	// Touch u1 and u2 to make u3 the LRU.
	_, _ = checker.IsAdmin(context.Background(), "u1")
	_, _ = checker.IsAdmin(context.Background(), "u2")
	assert.Equal(t, int32(3), stub.calls.Load(), "touches are cache hits")

	// Insert u4 → evicts u3.
	_, err := checker.IsAdmin(context.Background(), "u4")
	require.NoError(t, err)
	assert.Equal(t, int32(4), stub.calls.Load())

	// u1 and u2 still cached; u4 just inserted.
	_, _ = checker.IsAdmin(context.Background(), "u1")
	_, _ = checker.IsAdmin(context.Background(), "u2")
	_, _ = checker.IsAdmin(context.Background(), "u4")
	assert.Equal(t, int32(4), stub.calls.Load(), "u1, u2, u4 are still cached")

	// u3 was evicted → re-fetch.
	_, _ = checker.IsAdmin(context.Background(), "u3")
	assert.Equal(t, int32(5), stub.calls.Load())
}

func TestCachedAdminChecker_SeparatesPerUser(t *testing.T) {
	// Different userIDs maintain independent cache entries.
	stub := &stubLookup{value: true}
	checker := NewCachedAdminChecker(stub)

	_, _ = checker.IsAdmin(context.Background(), "alice")
	_, _ = checker.IsAdmin(context.Background(), "bob")
	_, _ = checker.IsAdmin(context.Background(), "alice")
	_, _ = checker.IsAdmin(context.Background(), "bob")

	assert.Equal(t, int32(2), stub.calls.Load(), "one lookup per distinct userID")
}

func TestCachedAdminChecker_PurgeForcesRefetch(t *testing.T) {
	stub := &stubLookup{value: true}
	checker := NewCachedAdminChecker(stub)

	_, _ = checker.IsAdmin(context.Background(), "user-1")
	checker.purge()
	_, _ = checker.IsAdmin(context.Background(), "user-1")

	assert.Equal(t, int32(2), stub.calls.Load())
}

func TestSQLAdminLookup_NilDBReturnsError(t *testing.T) {
	lookup := &SQLAdminLookup{}
	_, err := lookup.LookupIsAdmin(context.Background(), "user-1")
	require.Error(t, err)
}

// blockingLookup lets a test pause inside LookupIsAdmin so we can
// drive the cache-race scenario deterministically.
type blockingLookup struct {
	calls   atomic.Int32
	gate    chan struct{} // close to release blocked callers
	results []bool        // values to return on each call (sequential)
}

func (b *blockingLookup) LookupIsAdmin(_ context.Context, _ string) (bool, error) {
	idx := b.calls.Add(1) - 1
	<-b.gate
	if int(idx) < len(b.results) {
		return b.results[idx], nil
	}
	return false, nil
}

func TestCachedAdminChecker_RaceConcurrentFetchesConverge(t *testing.T) {
	// Codex Phase-1 review caught a race where the second goroutine
	// to acquire the lock (with its possibly-stale fetched value)
	// would clobber the first goroutine's already-inserted entry.
	// The fix: when the post-fetch re-check finds a still-valid
	// entry, prefer it (both goroutines return the same converged
	// value, and the cache holds a single consistent value).
	//
	// We can't deterministically force "A's fetch is stale, B's is
	// fresh" without DB-level race control, but we CAN verify the
	// cache converges to a single value after concurrent fetches
	// and that the value is one of the two fetched results.
	lookup := &blockingLookup{
		gate:    make(chan struct{}),
		results: []bool{true, false}, // first call returns true, second false
	}
	checker := NewCachedAdminChecker(lookup)

	type result struct {
		got bool
		err error
	}
	res := make(chan result, 2)

	go func() {
		got, err := checker.IsAdmin(context.Background(), "racer")
		res <- result{got, err}
	}()
	time.Sleep(20 * time.Millisecond)
	go func() {
		got, err := checker.IsAdmin(context.Background(), "racer")
		res <- result{got, err}
	}()
	time.Sleep(20 * time.Millisecond)

	// Both goroutines are now parked inside LookupIsAdmin. Release.
	close(lookup.gate)

	r1 := <-res
	r2 := <-res
	require.NoError(t, r1.err)
	require.NoError(t, r2.err)

	// Both racing fetches happened (the lock-released goroutine that
	// won the lock-race got its own value; the loser got the
	// already-inserted entry's value). After convergence, both
	// returns must match — they're reading or being told the same
	// cached state.
	cached, err := checker.IsAdmin(context.Background(), "racer")
	require.NoError(t, err)

	// The cached value must be one of the two fetched results
	// (no phantom third state) AND the loser-of-the-lock-race
	// must have returned the same value as what's now cached.
	assert.Contains(t, []bool{true, false}, cached,
		"cached value must be one of the fetched values")

	// At least one of the two return values must equal the cached
	// state. (The winner returned its own value, which IS the
	// cached state. The loser returned the winner's cached value.
	// So both should equal `cached`.)
	assert.Equal(t, cached, r1.got, "r1 must reflect the cached state")
	assert.Equal(t, cached, r2.got, "r2 must reflect the cached state")
}
