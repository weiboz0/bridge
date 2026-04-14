// Package skills contains Bridge-specific AI tools for the agentic loop.
package skills

import (
	"context"
	"fmt"
	"regexp"

	"github.com/weiboz0/bridge/platform/internal/tools"
)

// GradeLevel represents the student's grade band.
type GradeLevel string

const (
	GradeK5  GradeLevel = "K-5"
	Grade68  GradeLevel = "6-8"
	Grade912 GradeLevel = "9-12"
)

// --- Guardrails (migrated from src/lib/ai/guardrails.ts) ---

var solutionPatterns = []*regexp.Regexp{
	regexp.MustCompile("(?s)```(?:python|javascript|js|typescript|ts|cpp|c\\+\\+|java|rust|c)\\n.{200,}```"),
	regexp.MustCompile(`(?s)def\s+\w+\s*\([^)]*\):.{100,}`),
	regexp.MustCompile(`(?s)class\s+\w+.{150,}`),
	regexp.MustCompile(`(?s)function\s+\w+\s*\([^)]*\)\s*\{.{100,}`),
	regexp.MustCompile(`(?i)here(?:'s| is) the (?:complete |full )?(?:solution|answer|code)`),
	regexp.MustCompile(`(?i)just (?:copy|paste|use) this`),
}

// ContainsSolution checks if the LLM response gives away too much code.
func ContainsSolution(text string) bool {
	for _, p := range solutionPatterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

// FilterResponse replaces full-solution responses with a Socratic redirect.
func FilterResponse(text string) string {
	if ContainsSolution(text) {
		return "I was about to give you too much! Let me try again with a hint instead.\n\nWhat part of the problem are you finding most confusing? Let's break it down together."
	}
	return text
}

// --- System Prompts (migrated from src/lib/ai/system-prompts.ts) ---

const baseRules = `You are a patient coding tutor helping a student learn to program.

RULES:
- Ask guiding questions to help the student think through the problem
- Point to where the issue might be (e.g., "look at line 5"), but don't give the answer
- Never provide complete function implementations or full solutions
- If the student asks you to write the code for them, redirect them to think about the approach
- Celebrate small wins and encourage persistence
- Keep responses concise (2-4 sentences unless explaining a concept)`

var gradePrompts = map[GradeLevel]string{
	GradeK5: baseRules + `

GRADE LEVEL: Elementary (K-5)
- Use simple vocabulary and short sentences
- Use analogies from everyday life (building blocks, recipes, treasure maps)
- Be extra encouraging and patient
- Focus on visual thinking: "What do you see happening when you run this?"
- Reference block concepts if using Blockly: "Which purple block did you use?"`,

	Grade68: baseRules + `

GRADE LEVEL: Middle School (6-8)
- Explain concepts clearly but don't over-simplify
- Reference specific line numbers: "Take a look at line 7 — what value does x have there?"
- Use analogies when helpful but can be more technical
- Encourage reading error messages: "What does the error message tell you?"
- Help build debugging habits: "What did you expect to happen vs what actually happened?"`,

	Grade912: baseRules + `

GRADE LEVEL: High School (9-12)
- Use proper technical terminology
- Reference documentation and best practices
- Discuss trade-offs when relevant: "This works, but what happens if the list is empty?"
- Encourage independent problem-solving: "How would you test that this works?"
- Help develop computational thinking and code organization skills`,
}

// GetSystemPrompt returns the system prompt for the given grade level.
func GetSystemPrompt(grade GradeLevel) string {
	if prompt, ok := gradePrompts[grade]; ok {
		return prompt
	}
	return gradePrompts[Grade68]
}

// BuildChatSystemPrompt constructs the full system prompt for the AI chat handler.
func BuildChatSystemPrompt(gradeLevel GradeLevel, code, language string) string {
	prompt := GetSystemPrompt(gradeLevel)
	if code != "" {
		lang := language
		if lang == "" {
			lang = "python"
		}
		prompt += fmt.Sprintf("\n\nThe student's current code:\n```%s\n%s\n```", lang, code)
	}
	return prompt
}

// --- Tutor Tool (for agentic loop) ---

// Tutor is an AI tool that provides Socratic tutoring context.
type Tutor struct{}

func NewTutor() *Tutor { return &Tutor{} }

func (t *Tutor) GetName() string { return "tutor" }

func (t *Tutor) GetSpec() tools.ToolSpec {
	return tools.ToolSpec{
		Name:        "tutor",
		Description: "Get the appropriate tutoring system prompt and guardrail rules for a student based on their grade level. Use this to understand how to respond to the student appropriately.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"grade_level": map[string]any{
					"type":        "string",
					"enum":        []string{"K-5", "6-8", "9-12"},
					"description": "The student's grade level band",
				},
				"proposed_response": map[string]any{
					"type":        "string",
					"description": "Optional: check if a proposed response violates guardrails",
				},
			},
			"required": []string{"grade_level"},
		},
	}
}

func (t *Tutor) Invoke(ctx context.Context, inv tools.ToolInvocation) (tools.ToolResult, error) {
	gradeLevelStr, _ := inv.Payload["grade_level"].(string)
	proposedResponse, _ := inv.Payload["proposed_response"].(string)

	gradeLevel := GradeLevel(gradeLevelStr)
	systemPrompt := GetSystemPrompt(gradeLevel)

	result := map[string]any{
		"system_prompt": systemPrompt,
		"grade_level":   string(gradeLevel),
	}

	if proposedResponse != "" {
		filtered := FilterResponse(proposedResponse)
		result["guardrail_triggered"] = filtered != proposedResponse
		if filtered != proposedResponse {
			result["filtered_response"] = filtered
			result["reason"] = "Response contained a complete solution. Socratic method requires guiding, not giving answers."
		}
	}

	return tools.ToolResult{
		ToolName: t.GetName(),
		Status:   "ok",
		Payload:  result,
	}, nil
}
