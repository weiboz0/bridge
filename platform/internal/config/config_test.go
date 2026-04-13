package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	t.Setenv("GO_PORT", "9999")

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
