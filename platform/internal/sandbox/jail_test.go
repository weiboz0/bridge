package sandbox

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBwrapAvailable(t *testing.T) {
	b := BwrapBackend{}
	if !b.Available() {
		t.Skip("bwrap not installed, skipping")
	}
	assert.Equal(t, "bwrap", b.Name())
}

func TestBwrapBuildCommand(t *testing.T) {
	b := BwrapBackend{}
	if !b.Available() {
		t.Skip("bwrap not installed, skipping")
	}

	cfg := JailConfig{
		Command:  "python3",
		Args:     []string{"/skill/runner.py"},
		UserID:   "user-1",
		WorkDir:  "/tmp/work",
		SkillDir: "/opt/skills/my_skill",
		OutputDir: "/tmp/output",
		MediaDir: "/home/user/media",
		Env:      map[string]string{"FOO": "bar"},
	}

	cmdPath, args, err := b.BuildCommand(cfg)
	require.NoError(t, err)

	// Command should be the bwrap binary
	assert.Contains(t, cmdPath, "bwrap")

	joined := strings.Join(args, " ")

	// Should contain namespace flags
	assert.Contains(t, joined, "--unshare-all")

	// Should mount skill read-only
	assert.Contains(t, joined, "--ro-bind /opt/skills/my_skill /skill")

	// Should mount work dir writable
	assert.Contains(t, joined, "--bind /tmp/work /workspace")

	// Should mount output dir writable
	assert.Contains(t, joined, "--bind /tmp/output /output")

	// Should mount media read-only
	assert.Contains(t, joined, "--ro-bind /home/user/media /user/media")

	// Should pass environment variable
	assert.Contains(t, joined, "--setenv FOO bar")

	// Should end with the command
	assert.Contains(t, joined, "-- python3 /skill/runner.py")
}

func TestBwrapBuildCommandNoOptionalMounts(t *testing.T) {
	b := BwrapBackend{}
	if !b.Available() {
		t.Skip("bwrap not installed, skipping")
	}

	cfg := JailConfig{
		Command:   "python3",
		Args:      []string{"/skill/runner.py"},
		UserID:    "user-1",
		WorkDir:   "/tmp/work",
		SkillDir:  "/opt/skills/my_skill",
		OutputDir: "/tmp/output",
	}

	_, args, err := b.BuildCommand(cfg)
	require.NoError(t, err)

	joined := strings.Join(args, " ")

	// Should NOT contain optional mounts
	assert.NotContains(t, joined, "/user/media")
	assert.NotContains(t, joined, "/user/history")
}

func TestUnsandboxedBuildCommand(t *testing.T) {
	u := UnsandboxedBackend{}
	assert.True(t, u.Available())
	assert.Equal(t, "unsandboxed", u.Name())

	cfg := JailConfig{
		Command:   "python3",
		Args:      []string{"runner.py"},
		UserID:    "user-1",
		WorkDir:   "/tmp/work",
		SkillDir:  "/opt/skills",
		OutputDir: "/tmp/output",
	}

	cmdPath, args, err := u.BuildCommand(cfg)
	require.NoError(t, err)

	// Should pass through the command (possibly resolved)
	assert.NotEmpty(t, cmdPath)
	assert.Equal(t, []string{"runner.py"}, args)
}

func TestDockerBuildCommand(t *testing.T) {
	d := DockerBackend{}
	if !d.Available() {
		t.Skip("docker not installed, skipping")
	}

	cfg := JailConfig{
		Command:    "python3",
		Args:       []string{"/skill/runner.py"},
		UserID:     "user-1",
		WorkDir:    "/tmp/work",
		SkillDir:   "/opt/skills/my_skill",
		OutputDir:  "/tmp/output",
		MemoryMB:   256,
		CPUPercent: 50,
	}

	cmdPath, args, err := d.BuildCommand(cfg)
	require.NoError(t, err)

	assert.Contains(t, cmdPath, "docker")

	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "--network none")
	assert.Contains(t, joined, "--memory 256m")
	assert.Contains(t, joined, "--cpus 0.50")
	assert.Contains(t, joined, "/opt/skills/my_skill:/skill:ro")
}

func TestSelectBackendFallback(t *testing.T) {
	// With no preference, should get something available
	b := SelectBackend("")
	assert.NotNil(t, b)
	assert.True(t, b.Available())

	// Requesting unsandboxed should always work
	b = SelectBackend("unsandboxed")
	assert.Equal(t, "unsandboxed", b.Name())

	// Requesting a non-existent backend falls back
	b = SelectBackend("nonexistent")
	assert.NotNil(t, b)
	assert.True(t, b.Available())
}
