package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func GetEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func IsTestMode() bool {
	return testing.Testing()
}

func IsFullPath(path string) bool {
	return filepath.IsAbs(path)
}

// GetProjectRoot walks up from the working directory to the folder holding
// go.mod; the Docker image keeps go.mod next to the binaries for this reason.
func GetProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}
