package cli

import (
	"errors"
	"os"
	"path/filepath"
)

// findProjectRoot looks for the .dwight directory
// starting from the current directory and moving up.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".dwight")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New(".dwight directory not found in any parent directories")
		}
		dir = parent
	}
}
