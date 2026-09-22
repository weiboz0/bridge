package config

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/handlers"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, 8002, cfg.Server.Port)
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://test@localhost/testdb")
	t.Setenv("NEXTAUTH_SECRET", "my-secret")
	t.Setenv("LLM_BACKEND", "anthropic")
	t.Setenv("LLM_MODEL", "claude-3")
	t.Setenv("LLM_BASE_URL", "https://api.anthropic.com")
	t.Setenv("PLATFORM_PORT", "9999")

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, "postgresql://test@localhost/testdb", cfg.Database.URL)
	assert.Equal(t, "my-secret", cfg.Auth.NextAuthSecret)
	assert.Equal(t, "anthropic", cfg.LLM.Backend)
	assert.Equal(t, "claude-3", cfg.LLM.Model)
	assert.Equal(t, "https://api.anthropic.com", cfg.LLM.BaseURL)
	assert.Equal(t, 9999, cfg.Server.Port)
}

func TestLoad_TOMLFile(t *testing.T) {
	// Clear env vars so TOML values are used
	t.Setenv("DATABASE_URL", "")

	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "test.toml")
	err := os.WriteFile(tomlPath, []byte(`
[server]
port = 7777
host = "127.0.0.1"

[database]
url = "postgresql://toml@localhost/tomldb"
`), 0644)
	require.NoError(t, err)

	cfg, err := Load(tomlPath)
	require.NoError(t, err)
	assert.Equal(t, 7777, cfg.Server.Port)
	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, "postgresql://toml@localhost/tomldb", cfg.Database.URL)
}

func TestLoad_EnvOverridesToml(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "test.toml")
	err := os.WriteFile(tomlPath, []byte(`
[database]
url = "postgresql://toml@localhost/tomldb"
`), 0644)
	require.NoError(t, err)

	// Env should override TOML
	t.Setenv("DATABASE_URL", "postgresql://env@localhost/envdb")

	cfg, err := Load(tomlPath)
	require.NoError(t, err)
	assert.Equal(t, "postgresql://env@localhost/envdb", cfg.Database.URL)
}

func TestLoad_NonexistentTOML(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.toml")
	require.NoError(t, err)
	// Should use defaults without error
	assert.Equal(t, 8002, cfg.Server.Port)
}

func TestLoad_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "bad.toml")
	err := os.WriteFile(tomlPath, []byte(`this is not valid toml {{{{`), 0644)
	require.NoError(t, err)

	_, err = Load(tomlPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config:")
}

func TestResolveLLMAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	t.Setenv("DASHSCOPE_API_KEY", "sk-dash-test")
	t.Setenv("GEMINI_API_KEY", "gm-test")

	assert.Equal(t, "sk-ant-test", resolveLLMAPIKey("anthropic"))
	assert.Equal(t, "sk-dash-test", resolveLLMAPIKey("dashscope"))
	assert.Equal(t, "sk-dash-test", resolveLLMAPIKey("aliyun"))
	assert.Equal(t, "sk-dash-test", resolveLLMAPIKey("qwen"))
	assert.Equal(t, "gm-test", resolveLLMAPIKey("gemini"))
	assert.Equal(t, "gm-test", resolveLLMAPIKey("google"))
	assert.Equal(t, "", resolveLLMAPIKey("unknown"))
}

func TestLoad_LLMAPIKeyResolved(t *testing.T) {
	t.Setenv("LLM_BACKEND", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-resolved")
	t.Setenv("DATABASE_URL", "")

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", cfg.LLM.Backend)
	assert.Equal(t, "sk-ant-resolved", cfg.LLM.APIKey)
}

func TestLoad_RealtimeControlUsesNumericLoopbackDefaultAndSeparateSecret(t *testing.T) {
	t.Setenv("HOCUSPOCUS_TOKEN_SECRET", "jwt-signing-secret")
	t.Setenv("HOCUSPOCUS_CONTROL_SECRET", strings.Repeat("a", 64))
	t.Setenv("HOCUSPOCUS_INTERNAL_URL", "")
	t.Setenv("HOCUSPOCUS_CONTROL_PORT", "")

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:4001", cfg.Realtime.HocuspocusInternalURL)
	require.Equal(t, strings.Repeat("a", 64), cfg.Realtime.HocuspocusControlSecret)
	require.NotEqual(t, cfg.Realtime.HocuspocusTokenSecret, cfg.Realtime.HocuspocusControlSecret)
}

func TestLoad_E2ECanvasControlFailureUsesExactExplicitOptIn(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"", false},
		{"true", false},
		{"1", true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("BRIDGE_E2E_CANVAS_CONTROL_FAILURE", tc.value)
			cfg, err := Load("")
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.Realtime.E2ECanvasControlFailure)
		})
	}
}

func TestRealtimeControlConfig_FailsClosedForMissingSharedSecretAndUnsafeOverride(t *testing.T) {
	for _, tc := range []struct {
		name, controlURL, controlSecret, signingSecret string
	}{
		{"missing control secret", "http://127.0.0.1:4001", "", "jwt-signing-secret"},
		{"reused signing secret", "http://127.0.0.1:4001", strings.Repeat("a", 64), strings.Repeat("a", 64)},
		{"dns plaintext", "http://hocuspocus.internal:4001", strings.Repeat("a", 64), "jwt-signing-secret"},
		{"loopback hostname plaintext", "http://localhost:4001", strings.Repeat("a", 64), "jwt-signing-secret"},
		{"redirect-shaped override", "http://127.0.0.1:4001/path", strings.Repeat("a", 64), "jwt-signing-secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := RealtimeConfig{
				HocuspocusTokenSecret:   tc.signingSecret,
				HocuspocusInternalURL:   tc.controlURL,
				HocuspocusControlSecret: tc.controlSecret,
			}
			require.Error(t, cfg.ValidateControl())
		})
	}
}

// The listener port is an operator boundary, not a best-effort convenience
// default.  A malformed override must make startup fail before main can open
// a database connection or register a route on a different listener.
func TestLoad_RealtimeControlRejectsEveryInvalidExplicitPort(t *testing.T) {
	for _, port := range []string{"0", "65536", "-1", "4001.5", "not-a-port"} {
		t.Run(port, func(t *testing.T) {
			t.Setenv("HOCUSPOCUS_TOKEN_SECRET", "jwt-signing-secret")
			t.Setenv("HOCUSPOCUS_CONTROL_SECRET", strings.Repeat("a", 64))
			t.Setenv("HOCUSPOCUS_INTERNAL_URL", "")
			t.Setenv("HOCUSPOCUS_CONTROL_PORT", port)

			cfg, err := Load("")
			require.NoError(t, err)
			require.Error(t, cfg.Realtime.ValidateControl(), "invalid explicit port %q must fail before startup", port)
		})
	}
}

// ---------------------------------------------------------------------------
// Plan 094 Phase 14 — the E2E stack attestation flags and their pool binding
// ---------------------------------------------------------------------------

// A tiny database/sql driver so the attestation handler can be exercised here
// without a database: it answers the single observe query with a fixed live
// database name and lock visibility.
type e2eStackConfigFakeDriver struct{}

type e2eStackConfigFakeConn struct{ liveName string }

type e2eStackConfigFakeRows struct {
	liveName string
	done     bool
}

var (
	e2eStackConfigFakeMu   sync.Mutex
	e2eStackConfigFakeSeq  int
	e2eStackConfigFakeLive = map[string]string{}
)

func (e2eStackConfigFakeDriver) Open(handle string) (driver.Conn, error) {
	e2eStackConfigFakeMu.Lock()
	defer e2eStackConfigFakeMu.Unlock()
	liveName, ok := e2eStackConfigFakeLive[handle]
	if !ok {
		return nil, fmt.Errorf("unknown fake database %q", handle)
	}
	return &e2eStackConfigFakeConn{liveName: liveName}, nil
}

func (c *e2eStackConfigFakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unsupported")
}
func (c *e2eStackConfigFakeConn) Close() error              { return nil }
func (c *e2eStackConfigFakeConn) Begin() (driver.Tx, error) { return nil, errors.New("tx unsupported") }
func (c *e2eStackConfigFakeConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &e2eStackConfigFakeRows{liveName: c.liveName}, nil
}

func (r *e2eStackConfigFakeRows) Columns() []string { return []string{"current_database", "exists"} }
func (r *e2eStackConfigFakeRows) Close() error      { return nil }
func (r *e2eStackConfigFakeRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.liveName
	dest[1] = true // the gate's lock is always visible; only the URLs vary here
	return nil
}

func init() { sql.Register("bridge_e2e_stack_config_fake", e2eStackConfigFakeDriver{}) }

func newE2EStackConfigFakeDB(t *testing.T, liveName string) *sql.DB {
	t.Helper()
	e2eStackConfigFakeMu.Lock()
	e2eStackConfigFakeSeq++
	handle := fmt.Sprintf("%s#%d", t.Name(), e2eStackConfigFakeSeq)
	e2eStackConfigFakeLive[handle] = liveName
	e2eStackConfigFakeMu.Unlock()

	db, err := sql.Open("bridge_e2e_stack_config_fake", handle)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
		e2eStackConfigFakeMu.Lock()
		delete(e2eStackConfigFakeLive, handle)
		e2eStackConfigFakeMu.Unlock()
	})
	return db
}

const (
	e2eStackConfigTestURL    = "postgresql://bridge@127.0.0.1:5432/bridge_test"
	e2eStackConfigNonTestURL = "postgresql://bridge@127.0.0.1:5432/bridge_dev"
	e2eStackConfigNonce      = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func e2eStackConfigAttest(t *testing.T, poolURL string, db *sql.DB) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	handlers.NewE2EStackHandler(handlers.E2EStackHandlerConfig{
		DB:          db,
		DatabaseURL: poolURL,
		NotFound:    r.NotFoundHandler(),
	}).Routes(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health/e2e-stack?nonce="+e2eStackConfigNonce, nil))
	return rec
}

// The attestation's parsed-name proof must follow the exact string this
// process's pool was built from — cfg.Database.URL, which may come from TOML —
// and never os.Getenv("DATABASE_URL"). Otherwise a process pooled against
// development data could attest because some unrelated variable in the
// environment happened to name a _test database.
func TestConfig_E2EStackUsesPoolConnectionStringNotEnvironment(t *testing.T) {
	t.Run("environment names a _test database but the pool does not → refuse", func(t *testing.T) {
		t.Setenv("DATABASE_URL", e2eStackConfigTestURL)

		// The pool answers with a perfect live name and a visible lock; only
		// the string the pool was built from is wrong.
		rec := e2eStackConfigAttest(t, e2eStackConfigNonTestURL, newE2EStackConfigFakeDB(t, "bridge_test"))
		require.Equal(t, http.StatusNotFound, rec.Code,
			"a pool built from a non-_test URL must refuse even when DATABASE_URL names a _test database")
		assert.Equal(t, "404 page not found\n", rec.Body.String())
	})

	t.Run("environment names a non-test database but the pool is _test → attest", func(t *testing.T) {
		t.Setenv("DATABASE_URL", e2eStackConfigNonTestURL)

		rec := e2eStackConfigAttest(t, e2eStackConfigTestURL, newE2EStackConfigFakeDB(t, "bridge_test"))
		require.Equal(t, http.StatusOK, rec.Code,
			"the decision follows the pool's own connection string, not the environment; body: %s", rec.Body.String())
		assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	})

	t.Run("environment is unset entirely → the pool still decides", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")

		require.Equal(t, http.StatusOK,
			e2eStackConfigAttest(t, e2eStackConfigTestURL, newE2EStackConfigFakeDB(t, "bridge_test")).Code)
		require.Equal(t, http.StatusNotFound,
			e2eStackConfigAttest(t, e2eStackConfigNonTestURL, newE2EStackConfigFakeDB(t, "bridge_test")).Code)
	})

	// And the string handed to the handler in cmd/api/main.go is exactly
	// cfg.Database.URL, which Load resolves from TOML or the environment.
	t.Run("cfg.Database.URL is the string main.go hands the handler", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")
		dir := t.TempDir()
		tomlPath := filepath.Join(dir, "pool.toml")
		require.NoError(t, os.WriteFile(tomlPath, []byte("[database]\nurl = \""+e2eStackConfigTestURL+"\"\n"), 0o600))

		cfg, err := Load(tomlPath)
		require.NoError(t, err)
		require.Equal(t, e2eStackConfigTestURL, cfg.Database.URL)
		require.Equal(t, http.StatusOK,
			e2eStackConfigAttest(t, cfg.Database.URL, newE2EStackConfigFakeDB(t, "bridge_test")).Code,
			"a TOML-sourced pool URL must be honoured even with DATABASE_URL unset")
	})
}

// Both opt-ins are exact-value flags. Anything else — including the other
// flag's enabled value — leaves the surface off.
func TestLoad_E2EStackFlagsUseExactExplicitOptIn(t *testing.T) {
	t.Run("BRIDGE_E2E_STACK", func(t *testing.T) {
		for _, tc := range []struct {
			value string
			want  bool
		}{
			{"", false},
			{"0", false},
			{"true", false},
			{"TRUE", false},
			{"yes", false},
			{"on", false},
			{"01", false},
			{" 1", false},
			{"1 ", false},
			{"1", true},
		} {
			t.Run("value="+tc.value, func(t *testing.T) {
				t.Setenv("BRIDGE_E2E_STACK", tc.value)
				cfg, err := Load("")
				require.NoError(t, err)
				require.Equal(t, tc.want, cfg.E2EStack)
			})
		}
	})

	t.Run("ALLOW_E2E_STACK_OVER_TUNNEL", func(t *testing.T) {
		for _, tc := range []struct {
			value string
			want  bool
		}{
			{"", false},
			{"0", false},
			{"1", false},
			{"TRUE", false},
			{"True", false},
			{"yes", false},
			{" true", false},
			{"true ", false},
			{"true", true},
		} {
			t.Run("value="+tc.value, func(t *testing.T) {
				t.Setenv("ALLOW_E2E_STACK_OVER_TUNNEL", tc.value)
				cfg, err := Load("")
				require.NoError(t, err)
				require.Equal(t, tc.want, cfg.AllowE2EStackOverTunnel)
			})
		}
	})

	// The two flags are independent: the tunnel opt-in never turns the
	// surface on by itself.
	t.Run("the tunnel opt-in alone does not enable the surface", func(t *testing.T) {
		t.Setenv("BRIDGE_E2E_STACK", "")
		t.Setenv("ALLOW_E2E_STACK_OVER_TUNNEL", "true")
		cfg, err := Load("")
		require.NoError(t, err)
		assert.False(t, cfg.E2EStack)
		assert.True(t, cfg.AllowE2EStackOverTunnel)
	})

	// Neither flag is configurable from TOML — they are deliberately
	// environment-only (`toml:"-"`), so a checked-in config file can never
	// switch on a test surface.
	t.Run("TOML cannot enable either flag", func(t *testing.T) {
		t.Setenv("BRIDGE_E2E_STACK", "")
		t.Setenv("ALLOW_E2E_STACK_OVER_TUNNEL", "")
		dir := t.TempDir()
		tomlPath := filepath.Join(dir, "flags.toml")
		require.NoError(t, os.WriteFile(tomlPath, []byte(
			"e2e_stack = true\nallow_e2e_stack_over_tunnel = true\nE2EStack = true\n"), 0o600))

		cfg, err := Load(tomlPath)
		require.NoError(t, err)
		assert.False(t, cfg.E2EStack)
		assert.False(t, cfg.AllowE2EStackOverTunnel)
	})
}
