package sandbox

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"os/exec"
)

// RunnerConfig holds resource limits and concurrency settings for the runner.
type RunnerConfig struct {
	TimeoutSeconds       int
	MaxMemoryMB          int
	MaxOutputBytes       int
	MaxOutputFiles       int
	MaxOutputSizeMB      int
	MaxConcurrentPerUser int
	MaxConcurrentTotal   int
}

// DefaultRunnerConfig returns sensible defaults for sandbox execution.
func DefaultRunnerConfig() RunnerConfig {
	return RunnerConfig{
		TimeoutSeconds:       30,
		MaxMemoryMB:          256,
		MaxOutputBytes:       1 << 20, // 1 MiB stdout
		MaxOutputFiles:       10,
		MaxOutputSizeMB:      100,
		MaxConcurrentPerUser: 3,
		MaxConcurrentTotal:   20,
	}
}

// Runner manages sandboxed skill execution.
type Runner struct {
	jail       JailBackend
	baseDir    string // e.g. ~/.bridge/userspace
	runnersDir string // path to runner scripts (runner.py, runner.sh)
	config     RunnerConfig
}

// NewRunner creates a Runner with the given jail backend and configuration.
func NewRunner(jail JailBackend, baseDir, runnersDir string, cfg RunnerConfig) *Runner {
	return &Runner{
		jail:       jail,
		baseDir:    baseDir,
		runnersDir: runnersDir,
		config:     cfg,
	}
}

// ExecuteRequest describes a single skill invocation.
type ExecuteRequest struct {
	UserID    string
	SkillDir  string         // host path to skill source
	Runtime   string         // "python" | "bash"
	Tool      string         // skill/tool name
	Payload   map[string]any // tool arguments
	SessionID string
	Access    []string // declared access: "media", "history", "workspace", "network"
}

// ExecuteResult is the outcome of a sandboxed execution.
type ExecuteResult struct {
	Status       string
	Payload      map[string]any
	Artifacts    []Artifact
	DurationMs   int64
	MemoryPeakMB int
}

// Execute runs a skill in a sandbox and returns the result. The context
// controls the overall timeout (in addition to the configured TimeoutSeconds).
func (r *Runner) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResult, error) {
	start := time.Now()

	// Look up runtime
	runtimes := DefaultRuntimes()
	rt, ok := runtimes[req.Runtime]
	if !ok {
		return nil, fmt.Errorf("unknown runtime: %q", req.Runtime)
	}

	// Create temp dirs for this invocation
	outputDir, err := os.MkdirTemp("", "sandbox-output-*")
	if err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}
	defer os.RemoveAll(outputDir)

	workDir, err := os.MkdirTemp("", "sandbox-work-*")
	if err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	// Determine skill dir: use the provided one, or fall back to runners dir
	skillDir := req.SkillDir
	if skillDir == "" {
		skillDir = r.runnersDir
	}

	// Build jail config
	jailCfg := JailConfig{
		Command:    rt.Command,
		Args:       []string{filepath.Join("/skill", rt.RunnerFile)},
		UserID:     req.UserID,
		WorkDir:    workDir,
		SkillDir:   skillDir,
		OutputDir:  outputDir,
		Env:        map[string]string{},
		MemoryMB:   r.config.MaxMemoryMB,
		CPUPercent: 0,
	}

	// For unsandboxed backend, the runner script path should be on the host FS
	if r.jail.Name() == "unsandboxed" {
		jailCfg.Args = []string{filepath.Join(skillDir, rt.RunnerFile)}
	}

	// Wire up optional access mounts
	for _, a := range req.Access {
		switch a {
		case "media":
			// Media dir would be resolved from the user's media path
			// For now, set it if a conventional path exists
		case "history":
			// History dir similarly
		case "workspace":
			// Use persistent user workspace instead of temp
			wsDir := GetUserWorkspaceDir(r.baseDir, req.UserID)
			_ = os.MkdirAll(wsDir, 0o755)
			jailCfg.WorkDir = wsDir
		}
	}

	// Apply timeout
	timeout := time.Duration(r.config.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build the jailed command
	cmdPath, cmdArgs, err := r.jail.BuildCommand(jailCfg)
	if err != nil {
		return nil, fmt.Errorf("build jail command: %w", err)
	}

	// Spawn the process
	cmd := exec.CommandContext(ctx, cmdPath, cmdArgs...)
	cmd.Dir = workDir
	cmd.Env = buildEnv(jailCfg.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	// Capture stderr for diagnostics
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	// Write JSON-RPC request to stdin
	rpcReq := InvokeRequest{
		ID:     1,
		Method: "invoke",
		Params: InvokeParams{
			Tool:    req.Tool,
			Payload: req.Payload,
			Context: InvokeContext{
				UserID:    req.UserID,
				SessionID: req.SessionID,
			},
		},
	}

	reqBytes, err := json.Marshal(rpcReq)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Write request and close stdin so the subprocess knows input is complete
	if _, err := stdin.Write(reqBytes); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("write to stdin: %w", err)
	}
	if _, err := stdin.Write([]byte("\n")); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("write newline: %w", err)
	}
	stdin.Close()

	// Read JSON-RPC response from stdout (first line)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, r.config.MaxOutputBytes), r.config.MaxOutputBytes)

	var respLine string
	if scanner.Scan() {
		respLine = scanner.Text()
	}

	// Wait for process to finish
	waitErr := cmd.Wait()

	// Log stderr if non-empty
	if stderrBuf.Len() > 0 {
		slog.Debug("sandbox stderr", "output", stderrBuf.String())
	}

	// Check for context timeout
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("sandbox execution timed out after %s", timeout)
	}

	if respLine == "" {
		errMsg := "no response from subprocess"
		if waitErr != nil {
			errMsg += ": " + waitErr.Error()
		}
		if stderrBuf.Len() > 0 {
			errMsg += " (stderr: " + stderrBuf.String() + ")"
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	// Parse the response
	var resp InvokeResponse
	if err := json.Unmarshal([]byte(respLine), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w (raw: %s)", err, respLine)
	}

	duration := time.Since(start)

	if resp.Error != nil {
		return &ExecuteResult{
			Status:     "error",
			Payload:    map[string]any{"error": resp.Error.Message, "code": resp.Error.Code},
			DurationMs: duration.Milliseconds(),
		}, nil
	}

	if resp.Result == nil {
		return nil, fmt.Errorf("response has neither result nor error")
	}

	// Scan output dir for artifacts
	artifacts := resp.Result.Artifacts
	if artifacts == nil {
		artifacts = scanOutputDir(outputDir)
	}

	return &ExecuteResult{
		Status:     resp.Result.Status,
		Payload:    resp.Result.Payload,
		Artifacts:  artifacts,
		DurationMs: duration.Milliseconds(),
	}, nil
}

// buildEnv constructs an os.Environ()-style slice from a map.
// Does NOT inherit host environment to prevent leaking secrets (API keys, DB URLs).
func buildEnv(env map[string]string) []string {
	result := make([]string, 0, len(env)+2)
	// Provide minimal safe defaults
	result = append(result, "PATH=/usr/local/bin:/usr/bin:/bin")
	result = append(result, "HOME=/tmp")
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

// scanOutputDir lists files in the output directory and returns them as artifacts.
func scanOutputDir(dir string) []Artifact {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var artifacts []Artifact
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		artifacts = append(artifacts, Artifact{
			Filename:  e.Name(),
			SizeBytes: info.Size(),
		})
	}
	return artifacts
}
