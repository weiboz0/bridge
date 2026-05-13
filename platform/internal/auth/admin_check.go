package auth

import (
	"container/list"
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"
)

// Plan 065 Phase 1 — live admin status with a tiny TTL+LRU cache.
//
// Phase 3 wires this into RequireAuth so every authenticated request
// gets the *live* IsPlatformAdmin from the DB, overwriting the
// JWT-carried hint before any handler reads it. ~80 handler sites
// across the platform read claims.IsPlatformAdmin; the middleware
// injection makes them all automatically live without per-handler
// changes (Codex pass-2 audit).
//
// The cache is short-lived (60s) on purpose: admin promote/demote
// must propagate within a minute, and Bridge's user count is small
// enough (~1000) that the entire active set fits comfortably with
// room to spare.
//
// On DB error with no recent cached value, AdminAndStatus returns
// `(false, "", err)` so RequireAuth can fail-closed (no silent admin
// grant during a DB outage). The caller logs and proceeds.

const (
	defaultAdminCacheTTL  = 60 * time.Second
	defaultAdminCacheSize = 1024
)

// AdminChecker is the abstraction RequireAuth depends on for live
// admin/status state. Inject a real DB-backed impl in production; tests
// can use a stub.
type AdminChecker interface {
	IsAdmin(ctx context.Context, userID string) (bool, error)
	AdminAndStatus(ctx context.Context, userID string) (isAdmin bool, status string, err error)
	Purge(userID string)
}

// adminLookup is the lower-level interface AdminAndStatus uses to fetch
// from Postgres on a cache miss. UserStore.GetUserByID matches
// this signature in spirit; we narrow to the fields auth cares about.
type adminLookup interface {
	LookupAdminAndStatus(ctx context.Context, userID string) (isAdmin bool, status string, err error)
}

// userStoreAdminLookup adapts *store.UserStore to the adminLookup
// interface. We don't import store here to avoid a package cycle
// (store depends on auth indirectly via shared types in some
// future callsites); instead the wiring lives in cmd/api/main.go.

// SQLAdminLookup is a tiny, store-free lookup that hits the users
// table directly. Living here means the auth package doesn't need
// to import internal/store. It's the same indexed PK lookup
// store.UserStore.GetUserByID does.
type SQLAdminLookup struct {
	DB *sql.DB
}

// LookupAdminAndStatus reads users.is_platform_admin and users.status for the given id.
// Missing row returns (false, "", nil) so a deleted user is treated as
// non-admin (same as GetUserByID's nil-row → nil semantic).
func (s *SQLAdminLookup) LookupAdminAndStatus(ctx context.Context, userID string) (bool, string, error) {
	if s == nil || s.DB == nil {
		return false, "", errors.New("auth.SQLAdminLookup: DB is nil")
	}
	var isAdmin bool
	var status string
	err := s.DB.QueryRowContext(ctx,
		`SELECT is_platform_admin, status FROM users WHERE id = $1`,
		userID,
	).Scan(&isAdmin, &status)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return isAdmin, status, nil
}

// CachedAdminChecker wraps an adminLookup with a small TTL+LRU
// cache. Concurrency-safe; one mutex protects both the LRU list
// and the lookup map.
type CachedAdminChecker struct {
	lookup adminLookup
	ttl    time.Duration
	cap    int

	mu      sync.Mutex
	entries map[string]*list.Element // userID -> LRU element
	lru     *list.List               // front = most recent

	// now is overridable for tests; defaults to time.Now.
	now func() time.Time
}

// adminCacheEntry is what we store in each LRU element.
type adminCacheEntry struct {
	userID    string
	isAdmin   bool
	status    string
	expiresAt time.Time
}

// NewCachedAdminChecker returns a checker with default TTL (60s)
// and capacity (1024). Pass NewCachedAdminCheckerWithSize to tune.
func NewCachedAdminChecker(lookup adminLookup) *CachedAdminChecker {
	return NewCachedAdminCheckerWithSize(lookup, defaultAdminCacheTTL, defaultAdminCacheSize)
}

// NewCachedAdminCheckerWithSize lets tests configure a tighter cache.
func NewCachedAdminCheckerWithSize(lookup adminLookup, ttl time.Duration, capacity int) *CachedAdminChecker {
	if ttl <= 0 {
		ttl = defaultAdminCacheTTL
	}
	if capacity <= 0 {
		capacity = defaultAdminCacheSize
	}
	return &CachedAdminChecker{
		lookup:  lookup,
		ttl:     ttl,
		cap:     capacity,
		entries: make(map[string]*list.Element, capacity),
		lru:     list.New(),
		now:     time.Now,
	}
}

// IsAdmin returns the live admin status for userID, served from cache when
// fresh. Use AdminAndStatus when the caller also needs account status.
func (c *CachedAdminChecker) IsAdmin(ctx context.Context, userID string) (bool, error) {
	isAdmin, _, err := c.AdminAndStatus(ctx, userID)
	return isAdmin, err
}

// AdminAndStatus returns live admin and account status for userID, served from
// cache when fresh.
func (c *CachedAdminChecker) AdminAndStatus(ctx context.Context, userID string) (bool, string, error) {
	if userID == "" {
		return false, "", errors.New("auth.CachedAdminChecker: userID is empty")
	}

	now := c.now()

	c.mu.Lock()
	if elem, ok := c.entries[userID]; ok {
		entry := elem.Value.(*adminCacheEntry)
		if now.Before(entry.expiresAt) {
			c.lru.MoveToFront(elem)
			cached := entry.isAdmin
			status := entry.status
			c.mu.Unlock()
			return cached, status, nil
		}
		// Expired — drop and fall through to fetch.
		c.lru.Remove(elem)
		delete(c.entries, userID)
	}
	c.mu.Unlock()

	// Fetch outside the lock so a slow DB doesn't serialize all
	// callers. A concurrent caller for the same userID may race
	// to fetch as well; that's fine — last writer wins, both
	// produce the same value.
	isAdmin, status, err := c.lookup.LookupAdminAndStatus(ctx, userID)
	if err != nil {
		return false, "", err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Re-check: another goroutine may have inserted while we were
	// fetching from the DB. If a fresh entry now exists, prefer it
	// over our value — the racing goroutine's fetch ran later (it
	// had to acquire the lock after we released it), so its value
	// is more likely to reflect any concurrent admin promote/demote.
	// Codex Phase-1 review caught the original code's overwrite
	// bug, where an older slow fetch could clobber a newer value.
	if elem, ok := c.entries[userID]; ok {
		entry := elem.Value.(*adminCacheEntry)
		if now.Before(entry.expiresAt) {
			c.lru.MoveToFront(elem)
			return entry.isAdmin, entry.status, nil
		}
		// Existing entry is stale → drop and insert ours.
		c.lru.Remove(elem)
		delete(c.entries, userID)
	}
	entry := &adminCacheEntry{
		userID:    userID,
		isAdmin:   isAdmin,
		status:    status,
		expiresAt: now.Add(c.ttl),
	}
	elem := c.lru.PushFront(entry)
	c.entries[userID] = elem

	for c.lru.Len() > c.cap {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		c.lru.Remove(oldest)
		delete(c.entries, oldest.Value.(*adminCacheEntry).userID)
	}

	return isAdmin, status, nil
}

// Purge removes a single user's cached admin/status state.
func (c *CachedAdminChecker) Purge(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[userID]; ok {
		c.lru.Remove(elem)
		delete(c.entries, userID)
	}
}

// purge is exposed only for tests.
func (c *CachedAdminChecker) purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*list.Element, c.cap)
	c.lru = list.New()
}
