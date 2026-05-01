package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunPiston_HappyPath spins up a fake Piston HTTP server and runs
// the binary against it, asserting the JSON contract.
func TestRunPiston_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/execute" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"language":"python","version":"3.10.0",
			"run":{"stdout":"Hello\n","stderr":"","code":0,"output":"Hello\n"}
		}`))
	}))
	defer srv.Close()

	// Build the binary in a tempdir to avoid polluting the repo and
	// to exercise the same code path as production (`go run` is fine
	// in dev, but `go test` should not depend on the toolchain
	// re-compiling on every invocation).
	bin := filepath.Join(t.TempDir(), "run-piston")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build run-piston: %v\n%s", err, out)
	}

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "PISTON_URL="+srv.URL)
	cmd.Stdin = strings.NewReader(`{"language":"python","source":"print('Hello')\n","stdin":""}`)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run-piston: %v\n%s", err, out)
	}

	var got response
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode response: %v\n%s", err, out)
	}
	if got.Stdout != "Hello\n" {
		t.Errorf("stdout = %q; want %q", got.Stdout, "Hello\n")
	}
	if got.ExitCode != 0 {
		t.Errorf("exitCode = %d; want 0", got.ExitCode)
	}
	if got.Error != "" {
		t.Errorf("error = %q; want empty", got.Error)
	}
}

// TestRunPiston_TransportError surfaces a 500 from Piston as a
// transport error (exit 2), with the error message captured in the
// JSON payload.
func TestRunPiston_TransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "kaboom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	bin := filepath.Join(t.TempDir(), "run-piston")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build run-piston: %v\n%s", err, out)
	}

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "PISTON_URL="+srv.URL)
	cmd.Stdin = strings.NewReader(`{"language":"python","source":"print(1)\n","stdin":""}`)
	out, err := cmd.Output()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("unexpected exec error: %v", err)
		}
	}
	if exitCode != 2 {
		t.Errorf("exitCode = %d; want 2 (transport error); stdout=%s", exitCode, out)
	}
	var got response
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if !strings.Contains(got.Error, "piston transport error") {
		t.Errorf("error = %q; want it to mention 'piston transport error'", got.Error)
	}
}

// TestRunPiston_InvalidStdin returns exit 1 with a parse error when
// stdin isn't valid JSON.
func TestRunPiston_InvalidStdin(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "run-piston")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build run-piston: %v\n%s", err, out)
	}

	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(`not json at all`)
	out, err := cmd.Output()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("unexpected exec error: %v", err)
		}
	}
	if exitCode != 1 {
		t.Errorf("exitCode = %d; want 1 (invocation error); stdout=%s", exitCode, out)
	}
	var got response
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if got.Error == "" {
		t.Error("error field should be set")
	}
}

// TestRunPiston_MissingFields rejects requests missing language/source.
func TestRunPiston_MissingFields(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "run-piston")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build run-piston: %v\n%s", err, out)
	}

	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(`{"language":"python"}`)
	_, err := cmd.Output()
	if ee, ok := err.(*exec.ExitError); ok {
		if ee.ExitCode() != 1 {
			t.Errorf("exitCode = %d; want 1", ee.ExitCode())
		}
	} else if err == nil {
		t.Error("expected exit 1, got 0")
	}
}
