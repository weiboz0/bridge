package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PistonClient communicates with a Piston code execution API.
// See https://github.com/engineer-man/piston
type PistonClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPistonClient creates a new Piston API client.
func NewPistonClient(baseURL string) *PistonClient {
	if baseURL == "" {
		baseURL = "http://localhost:2000"
	}
	return &PistonClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// PistonExecuteRequest is the request body for Piston's /api/v2/execute endpoint.
type PistonExecuteRequest struct {
	Language string       `json:"language"`
	Version  string       `json:"version"`
	Files    []PistonFile `json:"files"`
	Stdin    string       `json:"stdin,omitempty"`
	Args     []string     `json:"args,omitempty"`
	RunTimeout   int      `json:"run_timeout,omitempty"`   // ms
	CompileTimeout int    `json:"compile_timeout,omitempty"` // ms
}

// PistonFile represents a source file for Piston execution.
type PistonFile struct {
	Name    string `json:"name,omitempty"`
	Content string `json:"content"`
}

// PistonExecuteResponse is the response from Piston's /api/v2/execute endpoint.
type PistonExecuteResponse struct {
	Language string      `json:"language"`
	Version  string      `json:"version"`
	Run      PistonStage `json:"run"`
	Compile  *PistonStage `json:"compile,omitempty"`
}

// PistonStage holds stdout/stderr/exit code for a run or compile step.
type PistonStage struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Code   int    `json:"code"`
	Signal *string `json:"signal,omitempty"`
	Output string `json:"output"` // combined stdout+stderr
}

// PistonRuntime describes an available language runtime.
type PistonRuntime struct {
	Language string   `json:"language"`
	Version  string   `json:"version"`
	Aliases  []string `json:"aliases"`
}

// Execute runs code through Piston and returns the result.
func (c *PistonClient) Execute(ctx context.Context, language, code string) (*PistonExecuteResponse, error) {
	return c.ExecuteWithStdin(ctx, language, code, "")
}

// ExecuteWithStdin runs code through Piston with stdin input.
//
// Uses a 10s run/compile timeout (production grading default). Note
// that vanilla Piston caps run_timeout at 3000ms; deployments wanting
// a longer timeout must raise the Piston `run_timeout` config. For
// tooling that needs a smaller default (e.g., the run-piston CLI used
// by the Python 101 importer against a stock Piston instance), use
// ExecuteWithStdinTimeout instead.
func (c *PistonClient) ExecuteWithStdin(ctx context.Context, language, code, stdin string) (*PistonExecuteResponse, error) {
	return c.ExecuteWithStdinTimeout(ctx, language, code, stdin, 10000, 10000)
}

// ExecuteWithStdinTimeout runs code through Piston with custom run
// and compile timeouts in milliseconds. Vanilla Piston rejects
// timeouts > 3000ms by default; pass <= 3000 if running against a
// stock instance.
func (c *PistonClient) ExecuteWithStdinTimeout(ctx context.Context, language, code, stdin string, runTimeoutMs, compileTimeoutMs int) (*PistonExecuteResponse, error) {
	reqBody := PistonExecuteRequest{
		Language:       language,
		Version:        "*", // latest available
		Files:          []PistonFile{{Content: code}},
		Stdin:          stdin,
		RunTimeout:     runTimeoutMs,
		CompileTimeout: compileTimeoutMs,
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("piston: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/execute", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("piston: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("piston: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB max
	if err != nil {
		return nil, fmt.Errorf("piston: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("piston: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result PistonExecuteResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("piston: unmarshal response: %w", err)
	}

	return &result, nil
}

// ListRuntimes fetches available language runtimes from Piston.
func (c *PistonClient) ListRuntimes(ctx context.Context) ([]PistonRuntime, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2/runtimes", nil)
	if err != nil {
		return nil, fmt.Errorf("piston: create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("piston: request failed: %w", err)
	}
	defer resp.Body.Close()

	var runtimes []PistonRuntime
	if err := json.NewDecoder(resp.Body).Decode(&runtimes); err != nil {
		return nil, fmt.Errorf("piston: decode runtimes: %w", err)
	}

	return runtimes, nil
}
