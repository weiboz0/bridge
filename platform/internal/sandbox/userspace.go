package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnsureUserSpace creates the per-user directory structure under baseDir if it
// does not already exist. The layout is:
//
//	{baseDir}/{userID}/skills/
//	{baseDir}/{userID}/workspace/
//	{baseDir}/{userID}/venv/
func EnsureUserSpace(baseDir, userID string) error {
	if userID == "" {
		return fmt.Errorf("userID must not be empty")
	}

	dirs := []string{
		filepath.Join(baseDir, userID, "skills"),
		filepath.Join(baseDir, userID, "workspace"),
		filepath.Join(baseDir, userID, "venv"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create user space dir %s: %w", dir, err)
		}
	}
	return nil
}

// GetUserSkillsDir returns the path to the user's installed skills directory.
func GetUserSkillsDir(baseDir, userID string) string {
	return filepath.Join(baseDir, userID, "skills")
}

// GetUserWorkspaceDir returns the path to the user's writable workspace directory.
func GetUserWorkspaceDir(baseDir, userID string) string {
	return filepath.Join(baseDir, userID, "workspace")
}

// CleanupUserSpace removes the entire per-user directory tree.
func CleanupUserSpace(baseDir, userID string) error {
	if userID == "" {
		return fmt.Errorf("userID must not be empty")
	}
	dir := filepath.Join(baseDir, userID)
	return os.RemoveAll(dir)
}
