package skills

import (
	"context"
	"fmt"

	"github.com/weiboz0/bridge/platform/internal/llm"
	"github.com/weiboz0/bridge/platform/internal/tools"
)

// LessonGenerator creates lesson content using LLM.
type LessonGenerator struct {
	backend llm.Backend
}

func NewLessonGenerator(backend llm.Backend) *LessonGenerator {
	return &LessonGenerator{backend: backend}
}

func (t *LessonGenerator) GetName() string { return "lesson_generator" }

func (t *LessonGenerator) GetSpec() tools.ToolSpec {
	return tools.ToolSpec{
		Name:        "lesson_generator",
		Description: "Generate lesson content for a coding topic. Creates structured lesson material including explanation, examples, and exercises.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"topic": map[string]any{
					"type":        "string",
					"description": "The coding topic to create a lesson for (e.g., 'for loops', 'functions', 'arrays')",
				},
				"language": map[string]any{
					"type":        "string",
					"description": "Programming language for code examples",
				},
				"grade_level": map[string]any{
					"type":        "string",
					"enum":        []string{"K-5", "6-8", "9-12"},
					"description": "Target grade level",
				},
			},
			"required": []string{"topic", "language", "grade_level"},
		},
	}
}

const lessonPrompt = `You are a curriculum designer for K-12 coding education.

Create a structured lesson that includes:
1. **Introduction** — Brief explanation of the concept (2-3 sentences)
2. **Key Concepts** — 3-5 bullet points of what students will learn
3. **Example** — A clear, commented code example demonstrating the concept
4. **Starter Code** — A skeleton that students will complete (with TODO comments)
5. **Challenge** — An extension exercise for students who finish early

Guidelines:
- Adapt complexity to the grade level
- Use engaging, real-world examples
- Keep code examples short and focused
- Include comments explaining each step
- The starter code should have clear TODO markers`

func (t *LessonGenerator) Invoke(ctx context.Context, inv tools.ToolInvocation) (tools.ToolResult, error) {
	topic, _ := inv.Payload["topic"].(string)
	language, _ := inv.Payload["language"].(string)
	gradeLevel, _ := inv.Payload["grade_level"].(string)

	if topic == "" || language == "" || gradeLevel == "" {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": "topic, language, and grade_level are required"},
		}, nil
	}

	if t.backend == nil {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": "LLM backend not configured"},
		}, nil
	}

	userMessage := fmt.Sprintf("Create a lesson on \"%s\" using %s for %s grade level students.", topic, language, gradeLevel)

	resp, err := t.backend.Chat(ctx, []llm.Message{
		{Role: llm.RoleSystem, Content: lessonPrompt},
		{Role: llm.RoleUser, Content: userMessage},
	}, llm.WithMaxTokens(2000))
	if err != nil {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": err.Error()},
		}, nil
	}

	return tools.ToolResult{
		ToolName: t.GetName(),
		Status:   "ok",
		Payload: map[string]any{
			"lesson":      resp.Content,
			"topic":       topic,
			"language":    language,
			"grade_level": gradeLevel,
		},
	}, nil
}
