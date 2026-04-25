package projection

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// block builds a json.RawMessage for a block with the given type and attrs.
func block(typ string, attrs map[string]any) json.RawMessage {
	m := map[string]any{"type": typ}
	if attrs != nil {
		m["attrs"] = attrs
	}
	b, _ := json.Marshal(m)
	return b
}

// ============ Always-include block types ============

func TestProjectBlocks_AlwaysInclude_Student(t *testing.T) {
	alwaysTypes := []string{
		"prose", "code-snippet", "media-embed", "paragraph", "heading",
		"bulletList", "orderedList", "listItem", "codeBlock", "blockquote",
		"horizontalRule", "hardBreak", "test-case-ref",
	}
	for _, typ := range alwaysTypes {
		t.Run(typ, func(t *testing.T) {
			blocks := []json.RawMessage{block(typ, map[string]any{"id": "b1"})}
			result := ProjectBlocks(blocks, RoleStudent, nil)
			require.Len(t, result, 1, "%s should be included for student", typ)
		})
	}
}

func TestProjectBlocks_AlwaysInclude_Teacher(t *testing.T) {
	alwaysTypes := []string{
		"prose", "code-snippet", "media-embed", "paragraph", "heading",
		"bulletList", "orderedList", "listItem", "codeBlock", "blockquote",
		"horizontalRule", "hardBreak", "test-case-ref",
	}
	for _, typ := range alwaysTypes {
		t.Run(typ, func(t *testing.T) {
			blocks := []json.RawMessage{block(typ, map[string]any{"id": "b1"})}
			result := ProjectBlocks(blocks, RoleTeacher, nil)
			require.Len(t, result, 1, "%s should be included for teacher", typ)
		})
	}
}

func TestProjectBlocks_AlwaysInclude_Admin(t *testing.T) {
	alwaysTypes := []string{
		"prose", "code-snippet", "media-embed", "paragraph", "heading",
		"bulletList", "orderedList", "listItem", "codeBlock", "blockquote",
		"horizontalRule", "hardBreak", "test-case-ref",
	}
	for _, typ := range alwaysTypes {
		t.Run(typ, func(t *testing.T) {
			blocks := []json.RawMessage{block(typ, map[string]any{"id": "b1"})}
			result := ProjectBlocks(blocks, RoleAdmin, nil)
			require.Len(t, result, 1, "%s should be included for admin", typ)
		})
	}
}

// ============ teacher-note ============

func TestProjectBlocks_TeacherNote_Student_Omitted(t *testing.T) {
	blocks := []json.RawMessage{block("teacher-note", map[string]any{"id": "tn1"})}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	assert.Empty(t, result)
}

func TestProjectBlocks_TeacherNote_Teacher_Included(t *testing.T) {
	blocks := []json.RawMessage{block("teacher-note", map[string]any{"id": "tn1"})}
	result := ProjectBlocks(blocks, RoleTeacher, nil)
	require.Len(t, result, 1)
}

func TestProjectBlocks_TeacherNote_Admin_Included(t *testing.T) {
	blocks := []json.RawMessage{block("teacher-note", map[string]any{"id": "tn1"})}
	result := ProjectBlocks(blocks, RoleAdmin, nil)
	require.Len(t, result, 1)
}

// ============ live-cue ============

func TestProjectBlocks_LiveCue_Student_Omitted(t *testing.T) {
	blocks := []json.RawMessage{block("live-cue", map[string]any{"id": "lc1"})}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	assert.Empty(t, result)
}

func TestProjectBlocks_LiveCue_Teacher_Included(t *testing.T) {
	blocks := []json.RawMessage{block("live-cue", map[string]any{"id": "lc1"})}
	result := ProjectBlocks(blocks, RoleTeacher, nil)
	require.Len(t, result, 1)
}

func TestProjectBlocks_LiveCue_Admin_Included(t *testing.T) {
	blocks := []json.RawMessage{block("live-cue", map[string]any{"id": "lc1"})}
	result := ProjectBlocks(blocks, RoleAdmin, nil)
	require.Len(t, result, 1)
}

// ============ assignment-variant ============

func TestProjectBlocks_AssignmentVariant_Student_Omitted(t *testing.T) {
	blocks := []json.RawMessage{block("assignment-variant", map[string]any{"id": "av1"})}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	assert.Empty(t, result)
}

func TestProjectBlocks_AssignmentVariant_Teacher_Included(t *testing.T) {
	blocks := []json.RawMessage{block("assignment-variant", map[string]any{"id": "av1"})}
	result := ProjectBlocks(blocks, RoleTeacher, nil)
	require.Len(t, result, 1)
}

func TestProjectBlocks_AssignmentVariant_Admin_Included(t *testing.T) {
	blocks := []json.RawMessage{block("assignment-variant", map[string]any{"id": "av1"})}
	result := ProjectBlocks(blocks, RoleAdmin, nil)
	require.Len(t, result, 1)
}

// ============ problem-ref ============

func TestProjectBlocks_ProblemRef_VisibilityAlways_Student_Included(t *testing.T) {
	blocks := []json.RawMessage{block("problem-ref", map[string]any{
		"id": "pr1", "visibility": "always",
	})}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	require.Len(t, result, 1)
}

func TestProjectBlocks_ProblemRef_VisibilityWhenUnitActive_Student_Omitted(t *testing.T) {
	blocks := []json.RawMessage{block("problem-ref", map[string]any{
		"id": "pr1", "visibility": "when-unit-active",
	})}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	assert.Empty(t, result)
}

func TestProjectBlocks_ProblemRef_NoVisibility_Student_Omitted(t *testing.T) {
	// Missing visibility attribute → not "always" → omitted for students.
	blocks := []json.RawMessage{block("problem-ref", map[string]any{"id": "pr1"})}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	assert.Empty(t, result)
}

func TestProjectBlocks_ProblemRef_Teacher_AlwaysIncluded(t *testing.T) {
	// Teacher sees problem-ref regardless of visibility setting.
	for _, vis := range []string{"always", "when-unit-active", ""} {
		t.Run("visibility_"+vis, func(t *testing.T) {
			blocks := []json.RawMessage{block("problem-ref", map[string]any{
				"id": "pr1", "visibility": vis,
			})}
			result := ProjectBlocks(blocks, RoleTeacher, nil)
			require.Len(t, result, 1)
		})
	}
}

func TestProjectBlocks_ProblemRef_Admin_AlwaysIncluded(t *testing.T) {
	blocks := []json.RawMessage{block("problem-ref", map[string]any{
		"id": "pr1", "visibility": "when-unit-active",
	})}
	result := ProjectBlocks(blocks, RoleAdmin, nil)
	require.Len(t, result, 1)
}

// ============ solution-ref ============

func TestProjectBlocks_SolutionRef_RevealAlways_Student_Included(t *testing.T) {
	blocks := []json.RawMessage{block("solution-ref", map[string]any{
		"id": "sr1", "reveal": "always",
	})}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	require.Len(t, result, 1)
}

func TestProjectBlocks_SolutionRef_RevealAfterSubmit_NoState_Student_Omitted(t *testing.T) {
	blocks := []json.RawMessage{block("solution-ref", map[string]any{
		"id": "sr1", "reveal": "after-submit",
	})}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	assert.Empty(t, result)
}

func TestProjectBlocks_SolutionRef_RevealAfterSubmit_NilMap_Student_Omitted(t *testing.T) {
	blocks := []json.RawMessage{block("solution-ref", map[string]any{
		"id": "sr1", "reveal": "after-submit",
	})}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	assert.Empty(t, result)
}

func TestProjectBlocks_SolutionRef_RevealAfterSubmit_NotStarted_Student_Omitted(t *testing.T) {
	blocks := []json.RawMessage{block("solution-ref", map[string]any{
		"id": "sr1", "reveal": "after-submit",
	})}
	states := map[string]AttemptState{"sr1": AttemptNotStarted}
	result := ProjectBlocks(blocks, RoleStudent, states)
	assert.Empty(t, result)
}

func TestProjectBlocks_SolutionRef_RevealAfterSubmit_Submitted_Student_Included(t *testing.T) {
	blocks := []json.RawMessage{block("solution-ref", map[string]any{
		"id": "sr1", "reveal": "after-submit",
	})}
	states := map[string]AttemptState{"sr1": AttemptSubmitted}
	result := ProjectBlocks(blocks, RoleStudent, states)
	require.Len(t, result, 1)
}

func TestProjectBlocks_SolutionRef_RevealAfterSubmit_Passed_Student_Included(t *testing.T) {
	blocks := []json.RawMessage{block("solution-ref", map[string]any{
		"id": "sr1", "reveal": "after-submit",
	})}
	states := map[string]AttemptState{"sr1": AttemptPassed}
	result := ProjectBlocks(blocks, RoleStudent, states)
	require.Len(t, result, 1)
}

func TestProjectBlocks_SolutionRef_RevealAfterSubmit_Failed_Student_Included(t *testing.T) {
	blocks := []json.RawMessage{block("solution-ref", map[string]any{
		"id": "sr1", "reveal": "after-submit",
	})}
	states := map[string]AttemptState{"sr1": AttemptFailed}
	result := ProjectBlocks(blocks, RoleStudent, states)
	require.Len(t, result, 1)
}

func TestProjectBlocks_SolutionRef_NoReveal_Student_Omitted(t *testing.T) {
	// Missing reveal attribute → default hidden for students.
	blocks := []json.RawMessage{block("solution-ref", map[string]any{"id": "sr1"})}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	assert.Empty(t, result)
}

func TestProjectBlocks_SolutionRef_Teacher_AlwaysIncluded(t *testing.T) {
	for _, reveal := range []string{"always", "after-submit", ""} {
		t.Run("reveal_"+reveal, func(t *testing.T) {
			blocks := []json.RawMessage{block("solution-ref", map[string]any{
				"id": "sr1", "reveal": reveal,
			})}
			result := ProjectBlocks(blocks, RoleTeacher, nil)
			require.Len(t, result, 1)
		})
	}
}

func TestProjectBlocks_SolutionRef_Admin_AlwaysIncluded(t *testing.T) {
	blocks := []json.RawMessage{block("solution-ref", map[string]any{
		"id": "sr1", "reveal": "after-submit",
	})}
	result := ProjectBlocks(blocks, RoleAdmin, nil)
	require.Len(t, result, 1)
}

// ============ Mixed document ============

func TestProjectBlocks_MixedDoc_Teacher_SeesAll(t *testing.T) {
	blocks := []json.RawMessage{
		block("prose", map[string]any{"id": "b1"}),
		block("teacher-note", map[string]any{"id": "b2"}),
		block("live-cue", map[string]any{"id": "b3"}),
		block("problem-ref", map[string]any{"id": "b4", "visibility": "when-unit-active"}),
		block("solution-ref", map[string]any{"id": "b5", "reveal": "after-submit"}),
		block("assignment-variant", map[string]any{"id": "b6"}),
		block("test-case-ref", map[string]any{"id": "b7"}),
		block("paragraph", nil),
	}
	result := ProjectBlocks(blocks, RoleTeacher, nil)
	assert.Len(t, result, 8, "teacher should see all 8 blocks")
}

func TestProjectBlocks_MixedDoc_Student_Filtered(t *testing.T) {
	blocks := []json.RawMessage{
		block("prose", map[string]any{"id": "b1"}),            // included
		block("teacher-note", map[string]any{"id": "b2"}),     // omitted
		block("live-cue", map[string]any{"id": "b3"}),         // omitted
		block("problem-ref", map[string]any{"id": "b4", "visibility": "always"}), // included
		block("solution-ref", map[string]any{"id": "b5", "reveal": "always"}),    // included
		block("assignment-variant", map[string]any{"id": "b6"}),                  // omitted
		block("test-case-ref", map[string]any{"id": "b7"}),                       // included
		block("paragraph", nil),                                                   // included
	}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	assert.Len(t, result, 5, "student should see 5 of 8 blocks")

	// Verify which types are present.
	types := extractTypes(t, result)
	assert.Contains(t, types, "prose")
	assert.Contains(t, types, "problem-ref")
	assert.Contains(t, types, "solution-ref")
	assert.Contains(t, types, "test-case-ref")
	assert.Contains(t, types, "paragraph")
	assert.NotContains(t, types, "teacher-note")
	assert.NotContains(t, types, "live-cue")
	assert.NotContains(t, types, "assignment-variant")
}

func TestProjectBlocks_MixedDoc_Student_SolutionRevealLogic(t *testing.T) {
	blocks := []json.RawMessage{
		block("prose", map[string]any{"id": "b1"}),
		block("solution-ref", map[string]any{"id": "sr-visible", "reveal": "always"}),
		block("solution-ref", map[string]any{"id": "sr-hidden", "reveal": "after-submit"}),
		block("solution-ref", map[string]any{"id": "sr-revealed", "reveal": "after-submit"}),
	}
	states := map[string]AttemptState{
		"sr-revealed": AttemptSubmitted,
		// sr-hidden has no state → omitted
	}
	result := ProjectBlocks(blocks, RoleStudent, states)
	assert.Len(t, result, 3, "student should see prose + 2 solution-refs")

	types := extractTypes(t, result)
	assert.Equal(t, 2, countType(types, "solution-ref"))
}

// ============ Edge cases ============

func TestProjectBlocks_EmptyBlocks(t *testing.T) {
	result := ProjectBlocks([]json.RawMessage{}, RoleStudent, nil)
	assert.NotNil(t, result, "should return non-nil empty slice")
	assert.Empty(t, result)
}

func TestProjectBlocks_NilBlocks(t *testing.T) {
	result := ProjectBlocks(nil, RoleStudent, nil)
	assert.NotNil(t, result, "should return non-nil empty slice")
	assert.Empty(t, result)
}

func TestProjectBlocks_InvalidJSON_Skipped(t *testing.T) {
	blocks := []json.RawMessage{
		[]byte(`not valid json`),
		block("prose", map[string]any{"id": "b1"}),
	}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	require.Len(t, result, 1, "should skip invalid JSON block")
}

func TestProjectBlocks_UnknownBlockType_Student_Omitted(t *testing.T) {
	blocks := []json.RawMessage{block("unknown-widget", map[string]any{"id": "u1"})}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	assert.Empty(t, result, "unknown types should be omitted for students")
}

func TestProjectBlocks_UnknownBlockType_Teacher_Included(t *testing.T) {
	blocks := []json.RawMessage{block("unknown-widget", map[string]any{"id": "u1"})}
	result := ProjectBlocks(blocks, RoleTeacher, nil)
	require.Len(t, result, 1, "unknown types should be included for teachers")
}

func TestProjectBlocks_PreservesOriginalJSON(t *testing.T) {
	// Ensure the output blocks are the exact same json.RawMessage values
	// (no re-serialization that could lose unknown fields).
	original := json.RawMessage(`{"type":"prose","attrs":{"id":"b1"},"extra":"field","nested":{"deep":true}}`)
	blocks := []json.RawMessage{original}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	require.Len(t, result, 1)
	assert.Equal(t, string(original), string(result[0]))
}

func TestProjectBlocks_NilAttemptStates_SolutionRef(t *testing.T) {
	// Ensure nil attemptStates map doesn't panic.
	blocks := []json.RawMessage{block("solution-ref", map[string]any{
		"id": "sr1", "reveal": "after-submit",
	})}
	result := ProjectBlocks(blocks, RoleStudent, nil)
	assert.Empty(t, result)
}

func TestProjectBlocks_EmptyAttemptStates_SolutionRef(t *testing.T) {
	blocks := []json.RawMessage{block("solution-ref", map[string]any{
		"id": "sr1", "reveal": "after-submit",
	})}
	result := ProjectBlocks(blocks, RoleStudent, map[string]AttemptState{})
	assert.Empty(t, result)
}

// ============ helpers ============

func extractTypes(t *testing.T, blocks []json.RawMessage) []string {
	t.Helper()
	types := make([]string, 0, len(blocks))
	for _, b := range blocks {
		var hdr struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal(b, &hdr))
		types = append(types, hdr.Type)
	}
	return types
}

func countType(types []string, target string) int {
	n := 0
	for _, t := range types {
		if t == target {
			n++
		}
	}
	return n
}
