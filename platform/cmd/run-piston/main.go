// Command run-piston is a tiny CLI wrapper around sandbox.PistonClient.
//
// Plan 049 phase 2: the Python 101 importer (TypeScript) shells out to
// this binary to verify reference solutions against test cases before
// any DB writes happen. The CLI exists so the importer can reuse the
// production Piston client (single source of truth for HTTP, retries,
// timeouts, response shape) without standing up the full Go API
// service or wiring auth just for content imports.
//
// Protocol: read one JSON object from stdin, write one JSON object to
// stdout, exit 0 on success.
//
// Request:
//
//	{
//	  "language": "python",
//	  "source": "print(\"Hello\")\n",
//	  "stdin": "ignored if empty"
//	}
//
// Response (always JSON; check `error` first, then `exitCode`):
//
//	{
//	  "stdout": "...",
//	  "stderr": "...",
//	  "exitCode": 0,
//	  "error": null,
//	  "timeoutMs": 10000
//	}
//
// Exit codes:
//
//	0 — Piston call completed (run may still have non-zero exitCode)
//	1 — invocation error (bad input JSON, missing PISTON_URL, etc.)
//	2 — Piston transport error (network, HTTP non-2xx, decode failure)
//
// Configuration:
//
//	PISTON_URL — base URL (default http://localhost:2000, matches
//	             sandbox.NewPistonClient's default).
//
// Usage:
//
//	echo '{"language":"python","source":"print(1)\n","stdin":""}' \
//	  | go run ./cmd/run-piston
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/weiboz0/bridge/platform/internal/sandbox"
)

type request struct {
	Language string `json:"language"`
	Source   string `json:"source"`
	Stdin    string `json:"stdin"`
}

type response struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exitCode"`
	Signal    string `json:"signal,omitempty"`
	Error     string `json:"error,omitempty"`
	TimeoutMs int    `json:"timeoutMs"`
}

func main() {
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 4*1024*1024))
	if err != nil {
		fail(fmt.Sprintf("read stdin: %v", err), 1)
		return
	}
	if len(raw) == 0 {
		fail("stdin is empty; expected one JSON request object", 1)
		return
	}
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		fail(fmt.Sprintf("parse stdin JSON: %v", err), 1)
		return
	}
	if req.Language == "" || req.Source == "" {
		fail("language and source are required", 1)
		return
	}

	baseURL := os.Getenv("PISTON_URL")
	client := sandbox.NewPistonClient(baseURL)

	// ExecuteWithStdin already sets a 30s HTTP-level timeout on the
	// httpClient and 10s run/compile timeouts. Add a context timeout
	// as belt-and-braces in case the upstream PistonClient changes.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := client.ExecuteWithStdin(ctx, req.Language, req.Source, req.Stdin)
	if err != nil {
		emit(response{
			Error:     fmt.Sprintf("piston transport error: %v", err),
			TimeoutMs: 10000,
		}, 2)
		return
	}

	out := response{
		Stdout:    resp.Run.Stdout,
		Stderr:    resp.Run.Stderr,
		ExitCode:  resp.Run.Code,
		TimeoutMs: 10000,
	}
	if resp.Run.Signal != nil {
		out.Signal = *resp.Run.Signal
	}
	if resp.Compile != nil && resp.Compile.Code != 0 {
		// Bubble compile failures into stderr so the importer's diff
		// against expectedStdout is meaningful (otherwise stdout is
		// empty and the failure is silent).
		out.Stderr = resp.Compile.Stderr + "\n" + out.Stderr
		out.ExitCode = resp.Compile.Code
	}
	emit(out, 0)
}

func emit(r response, exitCode int) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		fmt.Fprintf(os.Stderr, "encode response: %v\n", err)
		os.Exit(1)
	}
	os.Exit(exitCode)
}

func fail(msg string, exitCode int) {
	emit(response{Error: msg}, exitCode)
}
