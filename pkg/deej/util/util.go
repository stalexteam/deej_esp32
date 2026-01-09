package util

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"

	"go.uber.org/zap"
)

// EnsureDirExists creates the given directory path if it doesn't already exist
func EnsureDirExists(path string) error {
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return fmt.Errorf("ensure directory exists (%s): %w", path, err)
	}

	return nil
}

// FileExists checks if a file exists and is not a directory before we
// try using it to prevent further errors.
func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// Linux returns true if we're running on Linux
func Linux() bool {
	return runtime.GOOS == "linux"
}

// SetupCloseHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS
func SetupCloseHandler() chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	return c
}

// DumpAllGoroutines writes stack traces of all goroutines to the logger
func DumpAllGoroutines(logger *zap.SugaredLogger) {
	buf := make([]byte, 1024*1024) // 1MB buffer
	n := runtime.Stack(buf, true)
	logger.Errorw("All goroutines stack trace", "stack", string(buf[:n]))
}

// GetCurrentWindowProcessNames returns the process names (including extension, if applicable)
// of the current foreground window. This includes child processes belonging to the window.
// This is currently only implemented for Windows
func GetCurrentWindowProcessNames() ([]string, error) {
	return getCurrentWindowProcessNames()
}

// OpenExternal spawns a detached window with the provided command and argument
func OpenExternal(logger *zap.SugaredLogger, cmd string, arg string) error {

	// use cmd for windows, bash for linux
	execCommandArgs := []string{"cmd.exe", "/C", "start", "/b", cmd, arg}
	if Linux() {
		execCommandArgs = []string{"/bin/bash", "-c", fmt.Sprintf("%s %s", cmd, arg)}
	}

	command := exec.Command(execCommandArgs[0], execCommandArgs[1:]...)

	if err := command.Run(); err != nil {
		logger.Warnw("Failed to spawn detached process",
			"command", cmd,
			"argument", arg,
			"error", err)

		return fmt.Errorf("spawn detached proc: %w", err)
	}

	return nil
}

// NormalizeScalar "trims" the given float32 to 2 points of precision (e.g. 0.15442 -> 0.15)
// This is used both for windows core audio volume levels and for cleaning up slider level values from serial
func NormalizeScalar(v float32) float32 {
	return float32(math.Floor(float64(v)*100) / 100.0)
}

// a helper to make sure volume snaps correctly to 0 and 100, where appropriate
func almostEquals(a float32, b float32) bool {
	return math.Abs(float64(a-b)) < 0.000001
}

var (
	windowsDrivePathRegex = regexp.MustCompile(`^[A-Za-z]:[/\\]`)
	uncPathRegex = regexp.MustCompile(`^[/\\]{2}[^/\\]+[/\\]`)
)

// checks if a string looks like a file path
func IsPath(s string) bool {
	// C:\ or C:/
	if windowsDrivePathRegex.MatchString(s) {
		return true
	}

	//\\server\share or //server/share
	if uncPathRegex.MatchString(s) {
		return true
	}

	// Unix/Linux absolute paths
	if strings.HasPrefix(s, "/") {
		return true
	}

	// relative paths with path separators
	if strings.Contains(s, string(filepath.Separator)) {
		return true
	}

	// check for backslashes in relative paths
	if runtime.GOOS == "windows" && strings.Contains(s, "\\") {
		return true
	}

	return false
}

// normalizes a path for comparison:
func NormalizePath(path string) (string, error) {
	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("get absolute path: %w", err)
	}

	normalized := filepath.Clean(absPath)

	// On Windows, make it lowercase for case-insensitive comparison
	if runtime.GOOS == "windows" {
		normalized = strings.ToLower(normalized)
	}

	return normalized, nil
}

// checks if a process path matches a target dir
func PathMatches(processPath string, targetPath string) bool {
	if processPath == "" || targetPath == "" {
		return false
	}

	normalizedProcessPath, err := NormalizePath(processPath)
	if err != nil {
		return false
	}

	normalizedTargetPath, err := NormalizePath(targetPath)
	if err != nil {
		return false
	}

	if !strings.HasSuffix(normalizedTargetPath, string(filepath.Separator)) {
		normalizedTargetPath += string(filepath.Separator)
	}

	return strings.HasPrefix(normalizedProcessPath, normalizedTargetPath)
}
