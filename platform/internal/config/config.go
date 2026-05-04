package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/joho/godotenv"
)

type Config struct {
	Server        ServerConfig        `toml:"server"`
	Database      DatabaseConfig      `toml:"database"`
	Auth          AuthConfig          `toml:"auth"`
	LLM           LLMConfig           `toml:"llm"`
	Sandbox       SandboxConfig       `toml:"sandbox"`
	Realtime      RealtimeConfig      `toml:"realtime"`
	BridgeSession BridgeSessionConfig `toml:"bridge_session"`
}

type ServerConfig struct {
	Port int    `toml:"port"`
	Host string `toml:"host"`
}

type DatabaseConfig struct {
	URL string `toml:"url"`
}

type AuthConfig struct {
	NextAuthSecret string `toml:"nextauth_secret"`
}

type LLMConfig struct {
	Backend string `toml:"backend"`
	Model   string `toml:"model"`
	BaseURL string `toml:"base_url"`
	APIKey  string `toml:"-"` // resolved from env at runtime, never in config file
}

type SandboxConfig struct {
	PistonURL string `toml:"piston_url"`
}

// RealtimeConfig — plan 053. The shared HMAC secret used to sign
// Hocuspocus connection JWTs (`POST /api/realtime/token`) and to
// gate the internal auth endpoint
// (`POST /api/internal/realtime/auth`). Both Go and the Hocuspocus
// Node process must read the SAME secret. Sourced from
// HOCUSPOCUS_TOKEN_SECRET; never lives in the TOML config file.
type RealtimeConfig struct {
	HocuspocusTokenSecret string `toml:"-"`
}

// BridgeSessionConfig — plan 065. Bridge-issued HS256 session
// tokens that Auth.js mints via POST /api/internal/sessions and
// the Go middleware verifies in place of decrypting the Auth.js
// JWE. Three separate values, each with a distinct blast radius:
//
//   - Secrets: the rotation list. First entry signs; any entry
//     verifies. Comma-separated in BRIDGE_SESSION_SECRETS, with
//     legacy single-name BRIDGE_SESSION_SECRET as a fallback so
//     dev environments don't have to learn the plural.
//   - InternalBearer: shared with Auth.js's mint helper to gate
//     POST /api/internal/sessions. Distinct from
//     HOCUSPOCUS_TOKEN_SECRET so a leak of the realtime callback
//     bearer cannot forge session cookies.
//   - AuthFlag: feature flag for the Phase-3 cutover. With OFF
//     (default), Go middleware reads bridge.session opportunistically
//     but still treats Auth.js JWE as primary. With ON, bridge.session
//     becomes the authoritative cookie and absent-but-not-invalid
//     falls back to JWE during rollout.
type BridgeSessionConfig struct {
	Secrets        []string `toml:"-"`
	InternalBearer string   `toml:"-"`
	AuthFlag       bool     `toml:"-"`
}

func Load(path string) (*Config, error) {
	// Load .env from project root (try both platform/ and parent directory)
	_ = godotenv.Load()
	_ = godotenv.Load("../.env")

	cfg := &Config{
		Server: ServerConfig{
			Port: 8002,
			Host: "0.0.0.0",
		},
	}

	// Load TOML if exists
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, cfg); err != nil {
				return nil, fmt.Errorf("config: %w", err)
			}
		}
	}

	// Override from env
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.Database.URL = v
	}
	if v := os.Getenv("NEXTAUTH_SECRET"); v != "" {
		cfg.Auth.NextAuthSecret = v
	}
	if v := os.Getenv("LLM_BACKEND"); v != "" {
		cfg.LLM.Backend = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
	}

	if v := os.Getenv("PISTON_URL"); v != "" {
		cfg.Sandbox.PistonURL = v
	}

	if v := os.Getenv("HOCUSPOCUS_TOKEN_SECRET"); v != "" {
		cfg.Realtime.HocuspocusTokenSecret = v
	}

	// Plan 065 — Bridge session secrets. Prefer the plural
	// (rotation-aware) BRIDGE_SESSION_SECRETS; fall back to the
	// singular for environments still using the simpler form.
	if v := os.Getenv("BRIDGE_SESSION_SECRETS"); v != "" {
		cfg.BridgeSession.Secrets = parseSecretList(v)
	} else if v := os.Getenv("BRIDGE_SESSION_SECRET"); v != "" {
		cfg.BridgeSession.Secrets = []string{v}
	}
	if v := os.Getenv("BRIDGE_INTERNAL_SECRET"); v != "" {
		cfg.BridgeSession.InternalBearer = v
	}
	if v := os.Getenv("BRIDGE_SESSION_AUTH"); v != "" {
		// Treat any of "1", "true", "TRUE", "yes" as ON. Anything
		// else (including empty) is OFF.
		cfg.BridgeSession.AuthFlag = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	// Resolve LLM API key from provider-specific env var
	cfg.LLM.APIKey = resolveLLMAPIKey(cfg.LLM.Backend)
	if v := os.Getenv("GO_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Server.Port)
	}

	return cfg, nil
}

// parseSecretList splits a comma-separated env value into a list of
// trimmed, non-empty entries. Plan 065's BRIDGE_SESSION_SECRETS uses
// this so operators can rotate by prepending the new secret.
func parseSecretList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// resolveLLMAPIKey maps backend names to their environment variable for API keys.
func resolveLLMAPIKey(backend string) string {
	envVars := map[string]string{
		"anthropic":  "ANTHROPIC_API_KEY",
		"openai":     "OPENAI_API_KEY",
		"aliyun":     "DASHSCOPE_API_KEY",
		"dashscope":  "DASHSCOPE_API_KEY",
		"qwen":       "DASHSCOPE_API_KEY",
		"gemini":     "GEMINI_API_KEY",
		"google":     "GEMINI_API_KEY",
		"openrouter": "OPENROUTER_API_KEY",
		"ark":        "ARK_API_KEY",
		"doubao":     "ARK_API_KEY",
		"nvidia":     "NVIDIA_API_KEY",
		"deepseek":   "DEEPSEEK_API_KEY",
		"ds":         "DEEPSEEK_API_KEY",
	}
	if envName, ok := envVars[backend]; ok {
		return os.Getenv(envName)
	}
	return ""
}
