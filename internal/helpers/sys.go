package helpers

import (
	"fmt"
	"os"
)

func EnsureDir(path string, perm os.FileMode) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		// Directory doesn't exist, create it
		if err := os.MkdirAll(path, perm); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", path, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to stat directory %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s exists but is not a directory", path)
	}

	// Directory exists, correct permissions if needed
	if info.Mode().Perm() != perm {
		if err := os.Chmod(path, perm); err != nil {
			return fmt.Errorf("failed to correct permissions on %s: %w", path, err)
		}
	}

	return nil
}
