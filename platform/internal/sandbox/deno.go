package sandbox

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DenoRunner spawns Deno subprocesses to execute skills with multi-turn host callbacks.
type DenoRunner struct {
	denoPath    string
	runnerPath  string
	timeoutSec  int
	maxMemoryMB int
}

// DenoRequest describes a single Deno skill invocation.
type DenoRequest struct {
	SkillDir  string
	Tool      string
	Payload   map[string]any
	UserID    string
	SessionID string
	Access    map[string]bool
	Entry     string // default "index.ts"
}

// DenoResult is the outcome of a Deno skill execution.
type DenoResult struct {
	Status  string         `json:"status"`
	Payload map[string]any `json:"payload"`
}

// NewDenoRunner creates a DenoRunner.
func NewDenoRunner(denoPath, runnerPath string, timeoutSec, maxMemoryMB int) *DenoRunner {
	return &DenoRunner{
		denoPath:    denoPath,
		runnerPath:  runnerPath,
		timeoutSec:  timeoutSec,
		maxMemoryMB: maxMemoryMB,
	}
}

// Execute runs a Deno skill and handles multi-turn host callbacks.
func (d *DenoRunner) Execute(ctx context.Context, req DenoRequest, dispatcher *HostDispatcher) (*DenoResult, error) {
	timeout := time.Duration(d.timeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	entry := req.Entry
	if entry == "" {
		entry = "index.ts"
	}

	// Validate SkillDir: must be absolute, no commas (commas split Deno permission lists)
	if !strings.HasPrefix(req.SkillDir, "/") {
		return nil, fmt.Errorf("skill dir must be absolute path: %q", req.SkillDir)
	}
	if strings.Contains(req.SkillDir, ",") {
		return nil, fmt.Errorf("skill dir must not contain commas: %q", req.SkillDir)
	}

	// Build Deno command with minimal permissions
	denoCacheDir := denoDir()
	args := []string{
		"run",
		"--no-prompt",
		"--deny-net",
		"--allow-env=SKILL_DIR,SKILL_ENTRY",
		"--allow-read=" + req.SkillDir + "," + d.runnerPath + "," + absDir(d.runnerPath) + "," + denoCacheDir,
		fmt.Sprintf("--v8-flags=--max-old-space-size=%d", d.maxMemoryMB),
		d.runnerPath,
	}

	cmd := exec.CommandContext(ctx, d.denoPath, args...)
	cmd.Env = []string{
		"SKILL_DIR=" + req.SkillDir,
		"SKILL_ENTRY=" + entry,
		"HOME=" + os.Getenv("HOME"),
		"DENO_DIR=" + denoDir(),
	}
	cmd.Dir = req.SkillDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	stderrBuf := &limitedWriter{max: 64 * 1024} // 64KB max stderr
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start deno: %w", err)
	}

	// Write invoke request
	invokeReq := InvokeRequest{
		ID:     1,
		Method: "invoke",
		Params: InvokeParams{
			Tool:    req.Tool,
			Payload: req.Payload,
			Context: InvokeContext{UserID: req.UserID, SessionID: req.SessionID},
		},
	}
	reqBytes, err := json.Marshal(invokeReq)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("marshal invoke request: %w", err)
	}
	if _, err := stdin.Write(append(reqBytes, '\n')); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("write to stdin: %w", err)
	}

	// Dispatch loop: read messages from Deno, handle host callbacks
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer

	var writeErr error
	for scanner.Scan() {
		line := scanner.Bytes()

		// Peek at the message to determine type
		var msg struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			slog.Warn("deno: unparseable JSON from subprocess", "error", err, "line", string(line))
			continue
		}

		if msg.Method != "" && IsHostCallback(line) {
			// Host callback — dispatch and respond
			var callback HostCallbackRequest
			if err := json.Unmarshal(line, &callback); err != nil {
				slog.Warn("deno: failed to parse host callback", "error", err)
				continue
			}

			result, dispErr := dispatcher.Dispatch(ctx, callback.Method, callback.Params)
			var resp HostCallbackResponse
			resp.ID = callback.ID
			if dispErr != nil {
				resp.Error = &InvokeError{Code: -1, Message: dispErr.Error()}
			} else {
				resultBytes, err := json.Marshal(result)
				if err != nil {
					resp.Error = &InvokeError{Code: -1, Message: fmt.Sprintf("marshal result: %v", err)}
				} else {
					resp.Result = resultBytes
				}
			}
			respBytes, err := json.Marshal(resp)
			if err != nil {
				slog.Error("deno: failed to marshal callback response", "error", err)
				continue
			}
			if _, err := stdin.Write(append(respBytes, '\n')); err != nil {
				writeErr = fmt.Errorf("write callback response: %w", err)
				break
			}
			continue
		}

		// Final result (id matches invoke id)
		if msg.ID == 1 {
			var response InvokeResponse
			if err := json.Unmarshal(line, &response); err != nil {
				_ = cmd.Wait()
				return nil, fmt.Errorf("unmarshal skill result: %w (raw: %s)", err, string(line))
			}

			_ = cmd.Wait()

			if response.Error != nil {
				return nil, fmt.Errorf("skill error: %s", response.Error.Message)
			}
			if response.Result == nil {
				return nil, fmt.Errorf("skill returned no result")
			}
			return &DenoResult{
				Status:  response.Result.Status,
				Payload: response.Result.Payload,
			}, nil
		}
	}

	// stdout closed without a result
	_ = cmd.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("skill timeout after %ds", d.timeoutSec)
	}
	if writeErr != nil {
		return nil, fmt.Errorf("skill communication failed: %w", writeErr)
	}
	errMsg := "skill process exited without result"
	if stderrBuf.Len() > 0 {
		errMsg += " (stderr: " + stderrBuf.String() + ")"
	}
	return nil, fmt.Errorf("%s", errMsg)
}

// absDir returns the directory containing the given file path.
func absDir(path string) string {
	dir := path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		dir = path[:i]
	}
	return dir
}

// denoDir returns the Deno cache directory.
func denoDir() string {
	if d := os.Getenv("DENO_DIR"); d != "" {
		return d
	}
	home := os.Getenv("HOME")
	if home != "" {
		return home + "/.deno"
	}
	return "/tmp/deno"
}

// limitedWriter is an io.Writer that stops accepting data after max bytes.
type limitedWriter struct {
	buf []byte
	max int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remaining := w.max - len(w.buf)
	if remaining <= 0 {
		return len(p), nil // silently discard
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *limitedWriter) Len() int       { return len(w.buf) }
func (w *limitedWriter) String() string  { return string(w.buf) }
