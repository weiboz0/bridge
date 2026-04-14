package sandbox

import (
	"fmt"
	"os/exec"
)

// JailBackend abstracts the sandboxing mechanism used to isolate skill execution.
type JailBackend interface {
	// Name returns the backend identifier (e.g. "bwrap", "docker", "unsandboxed").
	Name() string
	// Available reports whether this backend can be used on the current system.
	Available() bool
	// BuildCommand wraps a command in the jail and returns the executable path and args.
	BuildCommand(cfg JailConfig) (string, []string, error)
}

// JailConfig describes the sandbox environment for a single invocation.
type JailConfig struct {
	Command    string            // executable inside the jail (e.g. "python3")
	Args       []string          // arguments (e.g. ["runner.py"])
	UserID     string            // for per-user isolation
	WorkDir    string            // writable working directory (host path)
	SkillDir   string            // read-only skill source (host path)
	OutputDir  string            // writable output for artifacts (host path)
	MediaDir   string            // read-only user media (host path, optional)
	HistoryDir string            // read-only user history (host path, optional)
	Env        map[string]string // environment variables passed to the process
	MemoryMB   int               // cgroup memory limit (0 = no limit)
	CPUPercent int               // cgroup CPU limit (0 = no limit)
}

// --- Bubblewrap backend (Linux namespace isolation) ---

// BwrapBackend uses bubblewrap for Linux namespace isolation.
type BwrapBackend struct{}

func (b BwrapBackend) Name() string { return "bwrap" }

func (b BwrapBackend) Available() bool {
	_, err := exec.LookPath("bwrap")
	return err == nil
}

func (b BwrapBackend) BuildCommand(cfg JailConfig) (string, []string, error) {
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return "", nil, fmt.Errorf("bwrap not found: %w", err)
	}

	args := []string{
		"--unshare-all",
		"--uid", "0",
		"--gid", "0",
		// Read-only system dirs
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/lib", "/lib",
		"--symlink", "/usr/lib64", "/lib64",
		"--symlink", "/usr/bin", "/bin",
		"--symlink", "/usr/sbin", "/sbin",
		// Virtual filesystems
		"--dev", "/dev",
		"--proc", "/proc",
		// Ephemeral tmp
		"--tmpfs", "/tmp",
		// Skill source (read-only)
		"--ro-bind", cfg.SkillDir, "/skill",
		// Writable dirs
		"--bind", cfg.WorkDir, "/workspace",
		"--bind", cfg.OutputDir, "/output",
	}

	// Optional media mount
	if cfg.MediaDir != "" {
		args = append(args, "--ro-bind", cfg.MediaDir, "/user/media")
	}

	// Optional history mount
	if cfg.HistoryDir != "" {
		args = append(args, "--ro-bind", cfg.HistoryDir, "/user/history")
	}

	// Environment variables
	for k, v := range cfg.Env {
		args = append(args, "--setenv", k, v)
	}

	// Separator and command
	args = append(args, "--", cfg.Command)
	args = append(args, cfg.Args...)

	return bwrap, args, nil
}

// --- Docker backend (macOS/Windows fallback) ---

// DockerBackend uses Docker containers for isolation.
type DockerBackend struct{}

func (d DockerBackend) Name() string { return "docker" }

func (d DockerBackend) Available() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

func (d DockerBackend) BuildCommand(cfg JailConfig) (string, []string, error) {
	docker, err := exec.LookPath("docker")
	if err != nil {
		return "", nil, fmt.Errorf("docker not found: %w", err)
	}

	args := []string{
		"run", "--rm", "-i",
		"--network", "none",
		"-v", cfg.SkillDir + ":/skill:ro",
		"-v", cfg.WorkDir + ":/workspace",
		"-v", cfg.OutputDir + ":/output",
	}

	if cfg.MediaDir != "" {
		args = append(args, "-v", cfg.MediaDir+":/user/media:ro")
	}
	if cfg.HistoryDir != "" {
		args = append(args, "-v", cfg.HistoryDir+":/user/history:ro")
	}
	if cfg.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", cfg.MemoryMB))
	}
	if cfg.CPUPercent > 0 {
		// Docker --cpus takes a float (1.0 = 100%)
		args = append(args, "--cpus", fmt.Sprintf("%.2f", float64(cfg.CPUPercent)/100.0))
	}
	for k, v := range cfg.Env {
		args = append(args, "-e", k+"="+v)
	}

	// Use a minimal Python image
	args = append(args, "python:3.12-slim")
	args = append(args, cfg.Command)
	args = append(args, cfg.Args...)

	return docker, args, nil
}

// --- Unsandboxed backend (dev/testing only) ---

// UnsandboxedBackend runs processes without any sandbox isolation.
// Only suitable for development and testing.
type UnsandboxedBackend struct{}

func (u UnsandboxedBackend) Name() string    { return "unsandboxed" }
func (u UnsandboxedBackend) Available() bool  { return true }

func (u UnsandboxedBackend) BuildCommand(cfg JailConfig) (string, []string, error) {
	cmd := cfg.Command
	// If the command is a bare name, try to resolve it
	if resolved, err := exec.LookPath(cfg.Command); err == nil {
		cmd = resolved
	}
	return cmd, cfg.Args, nil
}

// SelectBackend returns the best available backend. If preferred is set and
// available, it is used; otherwise the function falls through the chain:
// bwrap -> docker -> unsandboxed.
func SelectBackend(preferred string) JailBackend {
	backends := []JailBackend{
		BwrapBackend{},
		DockerBackend{},
		UnsandboxedBackend{},
	}

	// Try preferred first
	if preferred != "" {
		for _, b := range backends {
			if b.Name() == preferred && b.Available() {
				return b
			}
		}
	}

	// Fallback chain
	for _, b := range backends {
		if b.Available() {
			return b
		}
	}

	// UnsandboxedBackend.Available() always returns true, so we never reach here,
	// but return it for safety.
	return UnsandboxedBackend{}
}
