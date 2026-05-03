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
