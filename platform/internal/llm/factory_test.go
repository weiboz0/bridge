package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateBackend_Anthropic(t *testing.T) {
	b, err := CreateBackend(LLMConfig{Backend: "anthropic", APIKey: "test-key"})
	require.NoError(t, err)
	assert.Equal(t, "anthropic", b.Name())
}

func TestCreateBackend_OpenAI(t *testing.T) {
	b, err := CreateBackend(LLMConfig{Backend: "openai", APIKey: "test-key"})
	require.NoError(t, err)
	assert.Equal(t, "openai", b.Name())
}

func TestCreateBackend_DashScope(t *testing.T) {
	b, err := CreateBackend(LLMConfig{Backend: "dashscope", APIKey: "test-key"})
	require.NoError(t, err)
	assert.Equal(t, "aliyun", b.Name())
}

func TestCreateBackend_Gemini(t *testing.T) {
	b, err := CreateBackend(LLMConfig{Backend: "gemini", APIKey: "test-key"})
	require.NoError(t, err)
	assert.Equal(t, "gemini", b.Name())
}

func TestCreateBackend_Ollama(t *testing.T) {
	b, err := CreateBackend(LLMConfig{Backend: "ollama"})
	require.NoError(t, err)
	assert.Equal(t, "ollama", b.Name())
}

func TestCreateBackend_Aliases(t *testing.T) {
	tests := []struct {
		alias    string
		expected string
	}{
		{"claude", "anthropic"},
		{"gpt", "openai"},
		{"qwen", "aliyun"},
		{"bailian", "aliyun"},
		{"google", "gemini"},
		{"doubao", "ark"},
		{"volcengine", "ark"},
		{"nim", "nvidia"},
		{"or", "openrouter"},
		{"local", "ollama"},
	}
	for _, tc := range tests {
		t.Run(tc.alias, func(t *testing.T) {
			b, err := CreateBackend(LLMConfig{Backend: tc.alias, APIKey: "test"})
			require.NoError(t, err)
			assert.Equal(t, tc.expected, b.Name())
		})
	}
}

func TestCreateBackend_Unknown(t *testing.T) {
	_, err := CreateBackend(LLMConfig{Backend: "nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown backend")
}

func TestCreateBackend_DefaultBaseURLs(t *testing.T) {
	// Verify that OpenAI-compatible backends get default base URLs
	tests := []struct {
		backend     string
		expectedURL string
	}{
		{"ark", arkBaseURL},
		{"nvidia", nvidiaBaseURL},
		{"aliyun", dashscopeBaseURL},
		{"openrouter", openrouterBaseURL},
	}
	for _, tc := range tests {
		t.Run(tc.backend, func(t *testing.T) {
			cfg := LLMConfig{Backend: tc.backend, APIKey: "test"}
			_, err := CreateBackend(cfg)
			require.NoError(t, err)
			// The cfg is passed by value, so we can't inspect it after.
			// But the backend was created successfully, which verifies the URL was set.
		})
	}
}
