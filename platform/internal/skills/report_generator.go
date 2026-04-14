package skills

import (
	"context"
	"fmt"
	"strings"

	"github.com/weiboz0/bridge/platform/internal/llm"
	"github.com/weiboz0/bridge/platform/internal/tools"
)

// ReportGenerator generates parent-facing progress reports using LLM.
type ReportGenerator struct {
	backend llm.Backend
}

func NewReportGenerator(backend llm.Backend) *ReportGenerator {
	return &ReportGenerator{backend: backend}
}

func (t *ReportGenerator) GetName() string { return "report_generator" }

func (t *ReportGenerator) GetSpec() tools.ToolSpec {
	return tools.ToolSpec{
		Name:        "report_generator",
		Description: "Generate a parent-facing progress report for a student. Summarizes session activity, topics covered, and AI interactions.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"student_name": map[string]any{
					"type":        "string",
					"description": "The student's name",
				},
				"sessions_attended": map[string]any{
					"type":        "integer",
					"description": "Number of sessions attended in the reporting period",
				},
				"topics_covered": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "List of topic titles covered",
				},
				"ai_interactions": map[string]any{
					"type":        "integer",
					"description": "Number of AI tutor interactions",
				},
				"code_submissions": map[string]any{
					"type":        "integer",
					"description": "Number of code submissions",
				},
				"grade_level": map[string]any{
					"type":        "string",
					"description": "Student's grade level (K-5, 6-8, 9-12)",
				},
			},
			"required": []string{"student_name", "grade_level"},
		},
	}
}

const reportPrompt = `You are writing a progress report for a parent about their child's coding education.

Guidelines:
- Be encouraging and positive, highlighting growth and effort
- Use clear, non-technical language that parents can understand
- Mention specific topics the student has been learning
- Suggest ways the parent can support learning at home
- Keep the report concise (3-5 paragraphs)
- Include a brief summary of strengths and areas for growth`

func (t *ReportGenerator) Invoke(ctx context.Context, inv tools.ToolInvocation) (tools.ToolResult, error) {
	studentName, _ := inv.Payload["student_name"].(string)
	gradeLevel, _ := inv.Payload["grade_level"].(string)
	sessionsAttended, _ := inv.Payload["sessions_attended"].(float64)
	aiInteractions, _ := inv.Payload["ai_interactions"].(float64)
	codeSubmissions, _ := inv.Payload["code_submissions"].(float64)

	var topicsList []string
	if topics, ok := inv.Payload["topics_covered"].([]any); ok {
		for _, t := range topics {
			if s, ok := t.(string); ok {
				topicsList = append(topicsList, s)
			}
		}
	}

	if studentName == "" {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": "student_name is required"},
		}, fmt.Errorf("missing student_name")
	}

	userMessage := fmt.Sprintf(`Generate a progress report for %s (%s grade level).

Activity summary:
- Sessions attended: %.0f
- Topics covered: %s
- AI tutor interactions: %.0f
- Code submissions: %.0f`,
		studentName, gradeLevel,
		sessionsAttended,
		strings.Join(topicsList, ", "),
		aiInteractions,
		codeSubmissions,
	)

	if t.backend == nil {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": "LLM backend not configured"},
		}, nil
	}

	resp, err := t.backend.Chat(ctx, []llm.Message{
		{Role: llm.RoleSystem, Content: reportPrompt},
		{Role: llm.RoleUser, Content: userMessage},
	}, llm.WithMaxTokens(1000))
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
			"report":       resp.Content,
			"student_name": studentName,
			"grade_level":  gradeLevel,
		},
	}, nil
}
