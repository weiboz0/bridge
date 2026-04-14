package skills

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/tools"
)

func TestContainsSolution_LongCodeBlock(t *testing.T) {
	text := "Here's the code:\n```python\n" + string(make([]byte, 201)) + "```"
	assert.True(t, ContainsSolution(text))
}

func TestContainsSolution_LongJSCodeBlock(t *testing.T) {
	text := "Here's the code:\n```javascript\n" + string(make([]byte, 201)) + "```"
	assert.True(t, ContainsSolution(text))
}

func TestContainsSolution_LongCppCodeBlock(t *testing.T) {
	text := "Here's the code:\n```cpp\n" + string(make([]byte, 201)) + "```"
	assert.True(t, ContainsSolution(text))
}

func TestContainsSolution_JSFunctionDefinition(t *testing.T) {
	text := "function calculateTotal(items) {\n" + string(make([]byte, 101)) + "\n}"
	assert.True(t, ContainsSolution(text))
}

func TestContainsSolution_ShortCodeBlock(t *testing.T) {
	text := "Try this:\n```python\nprint('hello')\n```"
	assert.False(t, ContainsSolution(text))
}

func TestContainsSolution_CompleteSolution(t *testing.T) {
	assert.True(t, ContainsSolution("Here is the complete solution for you"))
	assert.True(t, ContainsSolution("Just copy this code"))
	assert.True(t, ContainsSolution("here's the full answer"))
}

func TestContainsSolution_Hint(t *testing.T) {
	assert.False(t, ContainsSolution("Try looking at line 5, what happens there?"))
	assert.False(t, ContainsSolution("What do you think the loop does?"))
}

func TestFilterResponse_Triggered(t *testing.T) {
	response := "Here is the complete solution:\n```python\n" + string(make([]byte, 250)) + "\n```"
	filtered := FilterResponse(response)
	assert.NotEqual(t, response, filtered)
	assert.Contains(t, filtered, "hint instead")
}

func TestFilterResponse_PassThrough(t *testing.T) {
	response := "Try checking line 3 — what value does x have?"
	assert.Equal(t, response, FilterResponse(response))
}

func TestGetSystemPrompt_AllGrades(t *testing.T) {
	for _, grade := range []GradeLevel{GradeK5, Grade68, Grade912} {
		prompt := GetSystemPrompt(grade)
		assert.Contains(t, prompt, "RULES:")
		assert.Contains(t, prompt, "GRADE LEVEL:")
	}
}

func TestGetSystemPrompt_DefaultsTo68(t *testing.T) {
	prompt := GetSystemPrompt("invalid")
	assert.Contains(t, prompt, "Middle School")
}

func TestBuildChatSystemPrompt_WithCode(t *testing.T) {
	prompt := BuildChatSystemPrompt(Grade68, "x = 1", "python")
	assert.Contains(t, prompt, "```python")
	assert.Contains(t, prompt, "x = 1")
}

func TestBuildChatSystemPrompt_NoCode(t *testing.T) {
	prompt := BuildChatSystemPrompt(GradeK5, "", "")
	assert.NotContains(t, prompt, "```")
}

func TestTutorTool_GetSpec(t *testing.T) {
	tutor := NewTutor()
	assert.Equal(t, "tutor", tutor.GetName())
	spec := tutor.GetSpec()
	assert.Equal(t, "tutor", spec.Name)
}

func TestTutorTool_Invoke(t *testing.T) {
	tutor := NewTutor()
	result, err := tutor.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"grade_level": "K-5"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
	assert.Contains(t, result.Payload["system_prompt"], "Elementary")
}

func TestTutorTool_InvokeWithGuardrail(t *testing.T) {
	tutor := NewTutor()
	result, err := tutor.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"grade_level":       "6-8",
			"proposed_response": "Here is the complete solution for you",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, true, result.Payload["guardrail_triggered"])
}

func TestTutorTool_InvokeNoGuardrail(t *testing.T) {
	tutor := NewTutor()
	result, err := tutor.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"grade_level":       "9-12",
			"proposed_response": "Try checking line 3",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, false, result.Payload["guardrail_triggered"])
}
