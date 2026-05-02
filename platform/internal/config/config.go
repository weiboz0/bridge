package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/joho/godotenv"
)

type Config struct {
	Server   ServerConfig   `toml:"server"`
	Database DatabaseConfig `toml:"database"`
	Auth     AuthConfig     `toml:"auth"`
	LLM      LLMConfig      `toml:"llm"`
	Sandbox  SandboxConfig  `toml:"sandbox"`
	Realtime RealtimeConfig `toml:"realtime"`
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

	// Resolve LLM API key from provider-specific env var
	cfg.LLM.APIKey = resolveLLMAPIKey(cfg.LLM.Backend)
	if v := os.Getenv("GO_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Server.Port)
	}

	return cfg, nil
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
