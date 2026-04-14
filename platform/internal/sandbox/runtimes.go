package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Runtime describes an execution environment for sandboxed skills.
type Runtime struct {
	Name       string // identifier (e.g. "python", "bash")
	Command    string // binary name or path (e.g. "python3", "/bin/bash")
	RunnerFile string // runner script filename (e.g. "runner.py")
}

// DefaultRuntimes returns the built-in runtime definitions.
func DefaultRuntimes() map[string]Runtime {
	return map[string]Runtime{
		"python": {Name: "python", Command: "python3", RunnerFile: "runner.py"},
		"bash":   {Name: "bash", Command: "/bin/bash", RunnerFile: "runner.sh"},
		"deno":   {Name: "deno", Command: "deno", RunnerFile: "skill_runner.ts"},
	}
}

// DetectDeno is a convenience wrapper for DetectRuntime("deno").
func DetectDeno() (string, error) {
	return DetectRuntime("deno")
}

// DetectRuntime resolves the given runtime name to an absolute binary path.
// Checks PATH first, then common install locations (~/.deno/bin for deno).
func DetectRuntime(name string) (string, error) {
	runtimes := DefaultRuntimes()
	rt, ok := runtimes[name]
	if !ok {
		return "", fmt.Errorf("unknown runtime: %q", name)
	}
	// Try PATH first
	if path, err := exec.LookPath(rt.Command); err == nil {
		return path, nil
	}
	// Try common fallback locations
	for _, candidate := range runtimeFallbacks(rt.Command) {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("runtime %q binary %q not found in PATH or common locations", name, rt.Command)
}

// runtimeFallbacks returns additional paths to check for a binary.
func runtimeFallbacks(command string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	switch command {
	case "deno":
		return []string{
			filepath.Join(home, ".deno", "bin", "deno"),
		}
	case "python3":
		return []string{
			filepath.Join(home, ".local", "bin", "python3"),
		}
	}
	return nil
}
