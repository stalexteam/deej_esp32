package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func getCurrentWindowProcessNames() ([]string, error) {
	return nil, errors.New("Not implemented")
}

// GetProcessPath returns the full path to the executable for the given process ID
// On Linux, this reads /proc/PID/exe which is a symlink to the executable
func GetProcessPath(pid int) (string, error) {
	exePath := fmt.Sprintf("/proc/%d/exe", pid)
	
	// Read the symlink to get the actual path
	path, err := os.Readlink(exePath)
	if err != nil {
		return "", fmt.Errorf("read symlink %s: %w", exePath, err)
	}
	
	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	
	return absPath, nil
}
