package router

import (
	"os"
	"path/filepath"
)

// createFile creates a file with all parent directories
func createFile(dst string) (*os.File, error) {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return os.Create(dst)
}
