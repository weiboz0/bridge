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
	LLM      LLMConfig     `toml:"llm"`
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
	if v := os.Getenv("GO_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Server.Port)
	}

	return cfg, nil
}
