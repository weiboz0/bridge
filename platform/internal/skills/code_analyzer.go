package skills

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/weiboz0/bridge/platform/internal/tools"
)

// CodeAnalyzer performs static analysis on student code.
type CodeAnalyzer struct{}

func NewCodeAnalyzer() *CodeAnalyzer { return &CodeAnalyzer{} }

func (t *CodeAnalyzer) GetName() string { return "code_analyzer" }

func (t *CodeAnalyzer) GetSpec() tools.ToolSpec {
	return tools.ToolSpec{
		Name:        "code_analyzer",
		Description: "Analyze student code for common patterns, mistakes, and style issues. Returns structured feedback without running the code.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"language": map[string]any{
					"type":        "string",
					"description": "Programming language of the code",
				},
				"code": map[string]any{
					"type":        "string",
					"description": "The student's source code to analyze",
				},
			},
			"required": []string{"language", "code"},
		},
	}
}

func (t *CodeAnalyzer) Invoke(ctx context.Context, inv tools.ToolInvocation) (tools.ToolResult, error) {
	language, _ := inv.Payload["language"].(string)
	code, _ := inv.Payload["code"].(string)

	if language == "" || code == "" {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": "language and code are required"},
		}, nil
	}

	issues := analyzeCode(language, code)
	metrics := codeMetrics(code)

	return tools.ToolResult{
		ToolName: t.GetName(),
		Status:   "ok",
		Payload: map[string]any{
			"language":   language,
			"issues":     issues,
			"metrics":    metrics,
			"line_count": metrics["line_count"],
		},
	}, nil
}

// CodeIssue represents a code analysis finding.
type CodeIssue struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
}

func analyzeCode(language, code string) []CodeIssue {
	var issues []CodeIssue
	lines := strings.Split(code, "\n")

	switch language {
	case "python":
		issues = append(issues, analyzePython(lines)...)
	case "javascript", "typescript":
		issues = append(issues, analyzeJS(lines)...)
	case "cpp", "c":
		issues = append(issues, analyzeCpp(lines)...)
	case "java":
		issues = append(issues, analyzeJava(lines)...)
	}

	for i, line := range lines {
		if len(strings.TrimSpace(line)) > 120 {
			issues = append(issues, CodeIssue{
				Type:    "style",
				Message: "Line is very long (>120 chars). Consider breaking it up.",
				Line:    i + 1,
			})
		}
	}

	return issues
}

func analyzePython(lines []string) []CodeIssue {
	var issues []CodeIssue
	tabMixed := false

	for i, line := range lines {
		if !tabMixed && strings.HasPrefix(line, "\t") {
			for _, other := range lines {
				if strings.HasPrefix(other, "    ") {
					issues = append(issues, CodeIssue{
						Type: "warning", Message: "Mixed tabs and spaces for indentation", Line: i + 1,
					})
					tabMixed = true
					break
				}
			}
		}
		if matched, _ := regexp.MatchString(`^\s*if\s+.*[^=!<>]=[^=]`, line); matched {
			issues = append(issues, CodeIssue{
				Type: "warning", Message: "Possible assignment in if-condition (did you mean ==?)", Line: i + 1,
			})
		}
		trimmed := strings.TrimSpace(line)
		for _, kw := range []string{"def ", "if ", "for ", "while ", "class ", "elif ", "else"} {
			if strings.HasPrefix(trimmed, kw) && !strings.HasSuffix(trimmed, ":") && !strings.HasSuffix(trimmed, ":\\") {
				issues = append(issues, CodeIssue{
					Type: "error", Message: fmt.Sprintf("Missing colon after '%s' statement", strings.TrimSpace(kw)), Line: i + 1,
				})
				break
			}
		}
	}
	return issues
}

func analyzeJS(lines []string) []CodeIssue {
	var issues []CodeIssue
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "var ") {
			issues = append(issues, CodeIssue{
				Type: "style", Message: "Consider using 'let' or 'const' instead of 'var'", Line: i + 1,
			})
		}
		if matched, _ := regexp.MatchString(`[^=!]={2}[^=]`, line); matched {
			issues = append(issues, CodeIssue{
				Type: "warning", Message: "Using == instead of === (loose equality)", Line: i + 1,
			})
		}
	}
	return issues
}

func analyzeCpp(lines []string) []CodeIssue {
	var issues []CodeIssue
	hasIncludeIOStream := false
	hasMain := false
	for _, line := range lines {
		if strings.Contains(line, "#include <iostream>") || strings.Contains(line, "#include<iostream>") {
			hasIncludeIOStream = true
		}
		if strings.Contains(line, "int main") {
			hasMain = true
		}
	}
	if !hasIncludeIOStream && hasMain {
		issues = append(issues, CodeIssue{
			Type: "warning", Message: "Missing #include <iostream> — needed for cout/cin",
		})
	}
	for i, line := range lines {
		if strings.Contains(line, "using namespace std;") {
			issues = append(issues, CodeIssue{
				Type: "style", Message: "'using namespace std' is discouraged in larger programs", Line: i + 1,
			})
		}
	}
	return issues
}

func analyzeJava(lines []string) []CodeIssue {
	var issues []CodeIssue
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "System.out.println") && !strings.HasSuffix(trimmed, ";") {
			issues = append(issues, CodeIssue{
				Type: "error", Message: "Missing semicolon", Line: i + 1,
			})
		}
	}
	return issues
}

func codeMetrics(code string) map[string]any {
	lines := strings.Split(code, "\n")
	nonEmpty := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmpty++
		}
	}
	return map[string]any{
		"line_count":     len(lines),
		"non_empty_lines": nonEmpty,
		"char_count":     len(code),
	}
}
