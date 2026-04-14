package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateUserID(t *testing.T) {
	assert.NoError(t, validateUserID("user-123"))
	assert.NoError(t, validateUserID("da5cef74-66e5-4946-bf56-409b23f34503"))
	assert.Error(t, validateUserID(""))
	assert.Error(t, validateUserID("../etc"))
	assert.Error(t, validateUserID("../../passwd"))
	assert.Error(t, validateUserID("user/id"))
	assert.Error(t, validateUserID("user\\id"))
	assert.Error(t, validateUserID("user\x00id"))
}

func TestEnsureUserSpace(t *testing.T) {
	baseDir := t.TempDir()

	err := EnsureUserSpace(baseDir, "test-user")
	require.NoError(t, err)

	assert.DirExists(t, filepath.Join(baseDir, "test-user", "skills"))
	assert.DirExists(t, filepath.Join(baseDir, "test-user", "workspace"))
	assert.DirExists(t, filepath.Join(baseDir, "test-user", "venv"))
}

func TestEnsureUserSpace_PathTraversal(t *testing.T) {
	baseDir := t.TempDir()

	err := EnsureUserSpace(baseDir, "../escape")
	assert.Error(t, err)

	err = EnsureUserSpace(baseDir, "user/../../etc")
	assert.Error(t, err)
}

func TestGetUserDirs(t *testing.T) {
	assert.Equal(t, "/base/user-1/skills", GetUserSkillsDir("/base", "user-1"))
	assert.Equal(t, "/base/user-1/workspace", GetUserWorkspaceDir("/base", "user-1"))
}

func TestCleanupUserSpace(t *testing.T) {
	baseDir := t.TempDir()

	err := EnsureUserSpace(baseDir, "cleanup-test")
	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(baseDir, "cleanup-test"))

	err = CleanupUserSpace(baseDir, "cleanup-test")
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(baseDir, "cleanup-test"))
	assert.True(t, os.IsNotExist(err))
}

func TestCleanupUserSpace_PathTraversal(t *testing.T) {
	baseDir := t.TempDir()
	err := CleanupUserSpace(baseDir, "../escape")
	assert.Error(t, err)
}
