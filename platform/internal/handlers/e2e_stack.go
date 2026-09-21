package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/weiboz0/bridge/platform/internal/realtime"
	"github.com/weiboz0/bridge/platform/internal/store"
)

const (
	e2eStackObserveTimeout = 2 * time.Second
	e2eStackLogInterval    = 10 * time.Second
	e2eStackObserveQuery   = `SELECT current_database(), EXISTS (SELECT 1 FROM pg_locks l JOIN pg_database d ON d.oid = l.database WHERE d.datname = current_database() AND l.locktype = 'advisory' AND l.classid = $1::int8::oid AND l.objid = $2::int8::oid AND l.objsubid = 1 AND l.granted)`
)

var e2eStackNoncePattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var (
	e2eStackInstanceOnce sync.Once
	e2eStackInstance     string
)

// E2EStackHandlerConfig supplies the dependencies for the opt-in stack
// attestation endpoint. InstanceID, Clock, Logger, and NotFound are seams for
// tests; production callers leave them nil for safe defaults.
type E2EStackHandlerConfig struct {
	DB          *sql.DB
	DatabaseURL string
	NotFound    http.Handler
	Clock       func() time.Time
	InstanceID  func() string
	Logger      *slog.Logger
}

// E2EStackHandler proves that this API process can see a gate-held advisory
// lock on its own database pool. It deliberately reveals nothing on refusal.
type E2EStackHandler struct {
	db                 *sql.DB
	parsedDatabaseTest bool
	notFound           http.Handler
	clock              func() time.Time
	instanceID         func() string
	logger             *slog.Logger

	logMu   sync.Mutex
	lastLog map[string]time.Time
}

// NewE2EStackHandler records the parsed-name proof once, using the exact URL
// supplied to construct this process's pool. The live proof remains per
// request, because only it proves the pool's current destination.
func NewE2EStackHandler(cfg E2EStackHandlerConfig) *E2EStackHandler {
	notFound := cfg.NotFound
	if notFound == nil {
		notFound = http.NotFoundHandler()
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	instanceID := cfg.InstanceID
	if instanceID == nil {
		instanceID = e2eStackProcessInstanceID
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &E2EStackHandler{
		db:                 cfg.DB,
		parsedDatabaseTest: realtime.ValidateE2ECanvasControlFailureDatabaseURL(true, cfg.DatabaseURL) == nil,
		notFound:           notFound,
		clock:              clock,
		instanceID:         instanceID,
		logger:             logger,
		lastLog:            make(map[string]time.Time),
	}
}

// Routes registers the public handler only when main has accepted the explicit
// E2E opt-in. No caller should register it for a disabled stack.
func (h *E2EStackHandler) Routes(r chi.Router) {
	if h == nil || r == nil {
		return
	}
	r.Get("/api/health/e2e-stack", h.Attest)
}

// Attest returns a fingerprint only when this process observes the exact
// transaction-scoped advisory lock held by the E2E gate on a live _test DB.
func (h *E2EStackHandler) Attest(w http.ResponseWriter, r *http.Request) {
	if h == nil {
		http.NotFoundHandler().ServeHTTP(w, r)
		return
	}
	if r == nil || r.URL == nil || !e2eStackNoncePattern.MatchString(r.URL.Query().Get("nonce")) {
		h.refuse(w, r, "malformed_nonce")
		return
	}
	if !h.parsedDatabaseTest {
		h.refuse(w, r, "parsed_database_not_test")
		return
	}
	if h.db == nil {
		h.refuse(w, r, "observe_query_error")
		return
	}

	nonce := r.URL.Query().Get("nonce")
	objID := e2eStackObjectID(nonce)
	ctx, cancel := context.WithTimeout(r.Context(), e2eStackObserveTimeout)
	defer cancel()

	var databaseName string
	var lockSeen bool
	if err := h.db.QueryRowContext(ctx, e2eStackObserveQuery, store.E2EStackLockClass, objID).Scan(&databaseName, &lockSeen); err != nil {
		h.refuse(w, r, "observe_query_error")
		return
	}
	if !hasTestDatabaseSuffix(databaseName) {
		h.refuse(w, r, "live_database_not_test")
		return
	}
	if !lockSeen {
		h.refuse(w, r, "lock_unseen")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(struct {
		Fingerprint string `json:"fingerprint"`
		Instance    string `json:"instance"`
	}{
		Fingerprint: e2eStackFingerprint(nonce, databaseName),
		Instance:    h.instanceID(),
	})
}

func (h *E2EStackHandler) refuse(w http.ResponseWriter, r *http.Request, reason string) {
	h.logRefusal(reason)
	notFound := h.notFound
	if notFound == nil {
		notFound = http.NotFoundHandler()
	}
	notFound.ServeHTTP(w, r)
}

func (h *E2EStackHandler) logRefusal(reason string) {
	if h == nil || h.logger == nil {
		return
	}
	clock := h.clock
	if clock == nil {
		clock = time.Now
	}
	now := clock()
	h.logMu.Lock()
	last, logged := h.lastLog[reason]
	if !logged || now.Sub(last) >= e2eStackLogInterval {
		if h.lastLog == nil {
			h.lastLog = make(map[string]time.Time)
		}
		h.lastLog[reason] = now
		h.logMu.Unlock()
		h.logger.Warn("E2E stack attestation refused", "reason", reason)
		return
	}
	h.logMu.Unlock()
}

func e2eStackObjectID(nonce string) int64 {
	digest := sha256.Sum256([]byte(nonce))
	return int64(binary.BigEndian.Uint32(digest[:4]) & 0x7fffffff)
}

func e2eStackFingerprint(nonce, databaseName string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(nonce))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(databaseName))
	return hex.EncodeToString(hash.Sum(nil))
}

func hasTestDatabaseSuffix(databaseName string) bool {
	return len(databaseName) >= len("_test") && databaseName[len(databaseName)-len("_test"):] == "_test"
}

func e2eStackProcessInstanceID() string {
	e2eStackInstanceOnce.Do(func() {
		bytes := make([]byte, 16)
		if _, err := rand.Read(bytes); err != nil {
			// crypto/rand failure is exceptionally rare; retain a process-stable,
			// opaque identifier rather than making attestation panic.
			fallback := sha256.Sum256([]byte(time.Now().String()))
			bytes = fallback[:16]
		}
		e2eStackInstance = stringInstanceID(os.Getpid(), bytes)
	})
	return e2eStackInstance
}

func stringInstanceID(pid int, random []byte) string {
	return strconv.Itoa(pid) + "-" + hex.EncodeToString(random)
}
