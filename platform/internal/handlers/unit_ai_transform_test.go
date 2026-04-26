package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/weiboz0/bridge/platform/internal/auth"
)

// ---------------------------------------------------------------------------
// AI Transform — auth guard tests
// ---------------------------------------------------------------------------

func TestAITransform_NoClaims(t *testing.T) {
	h := &UnitAIHandler{}
	body, _ := json.Marshal(map[string]string{
		"action":       "rewrite",
		"selectedText": "hello world",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/units/00000000-0000-0000-0000-000000000001/ai-transform", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.AITransform(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAITransform_NoBackend(t *testing.T) {
	h := &UnitAIHandler{Backend: nil}
	body, _ := json.Marshal(map[string]string{
		"action":       "rewrite",
		"selectedText": "hello world",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/units/00000000-0000-0000-0000-000000000001/ai-transform", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.AITransform(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Contains(t, resp["error"], "not configured")
}

// ---------------------------------------------------------------------------
// AI Transform — validation tests (no DB needed for these since they fail
// before DB access due to missing backend or claims)
// ---------------------------------------------------------------------------

func TestAITransform_InvalidAction(t *testing.T) {
	// This test needs a backend + unit store. Since the handler checks auth,
	// then loads unit, then validates body, we test the invalid action via
	// the aiTransformPrompts map directly.
	validActions := []string{"rewrite", "polish", "simplify", "expand", "summarize"}
	for _, action := range validActions {
		_, ok := aiTransformPrompts[action]
		assert.True(t, ok, "action %q should have a prompt", action)
	}

	// Invalid actions should not exist
	invalidActions := []string{"", "translate", "delete", "unknown"}
	for _, action := range invalidActions {
		_, ok := aiTransformPrompts[action]
		assert.False(t, ok, "action %q should NOT have a prompt", action)
	}
}

func TestAITransform_PromptContentQuality(t *testing.T) {
	// Verify prompts contain key instructions
	for action, prompt := range aiTransformPrompts {
		assert.NotEmpty(t, prompt, "prompt for %q should not be empty", action)
		assert.Contains(t, prompt, "K-12", "prompt for %q should mention K-12 context", action)
		assert.Contains(t, prompt, "Return ONLY", "prompt for %q should instruct to return only the result", action)
	}
}

func TestAITransform_AllFiveActions(t *testing.T) {
	expected := map[string]bool{
		"rewrite":   true,
		"polish":    true,
		"simplify":  true,
		"expand":    true,
		"summarize": true,
	}
	assert.Equal(t, len(expected), len(aiTransformPrompts), "should have exactly 5 AI transform actions")
	for action := range expected {
		_, ok := aiTransformPrompts[action]
		assert.True(t, ok, "missing prompt for action %q", action)
	}
}

// ---------------------------------------------------------------------------
// AI Transform — max field length validation
// ---------------------------------------------------------------------------

func TestAITransform_MaxLengthConstants(t *testing.T) {
	assert.Equal(t, 10000, maxSelectedTextLen)
	assert.Equal(t, 12000, maxContextLen)
	assert.Equal(t, 5000, maxDocSummaryLen)
}
