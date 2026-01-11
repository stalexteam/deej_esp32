//go:build linux
// +build linux

package deej

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// keystrokeActionImpl implements keystroke simulation for Linux
func keystrokeActionImpl(ctx context.Context, step *ActionStep, logger *zap.SugaredLogger) error {
	if step.Keys == "" {
		return fmt.Errorf("keys is required for keystroke action")
	}

	logger.Debugw("Simulating keystroke", "keys", step.Keys)

	// Check if xdotool is available
	if _, err := exec.LookPath("xdotool"); err != nil {
		return &ActionError{
			Type:    ErrorKeystrokeUnavailable,
			Message: "xdotool not found. Install it: sudo apt-get install xdotool",
			Step:    step,
			Err:     err,
		}
	}

	// Parse key combination (format: "Ctrl+Alt+T" or "Ctrl+Shift+A")
	keys := strings.Split(step.Keys, "+")
	if len(keys) == 0 {
		return fmt.Errorf("invalid key combination: %s", step.Keys)
	}

	// Build xdotool command
	// xdotool key ctrl+alt+t
	xdotoolKeys := buildXdotoolKeyString(keys)

	cmd := exec.CommandContext(ctx, "xdotool", "key", xdotoolKeys)

	if err := cmd.Run(); err != nil {
		// Check if it's a permission error
		if isPermissionError(err) {
			return &ActionError{
				Type:    ErrorPermissionDenied,
				Message: "Permission denied for keystroke. May need to run with appropriate permissions.",
				Step:    step,
				Err:     err,
			}
		}
		return fmt.Errorf("failed to send keystroke: %w", err)
	}

	return nil
}

// buildXdotoolKeyString builds xdotool key string from key combination
func buildXdotoolKeyString(keys []string) string {
	var parts []string

	for _, k := range keys {
		k = strings.TrimSpace(k)
		kLower := strings.ToLower(k)

		// Map common key names to xdotool format
		switch kLower {
		case "ctrl", "control":
			parts = append(parts, "ctrl")
		case "alt":
			parts = append(parts, "alt")
		case "shift":
			parts = append(parts, "shift")
		case "win", "windows", "meta", "super":
			parts = append(parts, "super")
		default:
			// Use the key as-is (xdotool will handle it)
			// Convert to lowercase for consistency
			parts = append(parts, strings.ToLower(k))
		}
	}

	return strings.Join(parts, "+")
}

// typingActionImpl implements text typing simulation for Linux
func typingActionImpl(ctx context.Context, step *ActionStep, logger *zap.SugaredLogger) error {
	if step.Text == "" {
		return fmt.Errorf("text is required for typing action")
	}

	logger.Debugw("Typing text", "text_length", len(step.Text), "char_delay", step.CharDelay)

	// Check if xdotool is available
	if _, err := exec.LookPath("xdotool"); err != nil {
		return &ActionError{
			Type:    ErrorKeystrokeUnavailable,
			Message: "xdotool not found. Install it: sudo apt-get install xdotool",
			Step:    step,
			Err:     err,
		}
	}

	// Process escape sequences in text
	processedText := processEscapeSequences(step.Text)

	// xdotool type command
	// If char_delay is set, use --delay option
	if step.CharDelay > 0 {
		cmd := exec.CommandContext(ctx, "xdotool", "type", "--delay", fmt.Sprintf("%d", step.CharDelay), processedText)
		if err := cmd.Run(); err != nil {
			if isPermissionError(err) {
				return &ActionError{
					Type:    ErrorPermissionDenied,
					Message: "Permission denied for typing. May need to run with appropriate permissions.",
					Step:    step,
					Err:     err,
				}
			}
			return fmt.Errorf("failed to type text: %w", err)
		}
	} else {
		// Type without delay
		cmd := exec.CommandContext(ctx, "xdotool", "type", processedText)
		if err := cmd.Run(); err != nil {
			if isPermissionError(err) {
				return &ActionError{
					Type:    ErrorPermissionDenied,
					Message: "Permission denied for typing. May need to run with appropriate permissions.",
					Step:    step,
					Err:     err,
				}
			}
			return fmt.Errorf("failed to type text: %w", err)
		}
	}

	return nil
}

// executeActionPlatform executes an application using exec.CommandContext on Linux
func executeActionPlatform(ctx context.Context, step *ActionStep, buttonID int, actionType string, key string, bh *ButtonHandler) error {
	if step.Wait {
		// For wait: true, use timeout context and wait for completion
		// Determine timeout: use wait_timeout if specified, otherwise use defaultWaitTimeout
		waitTimeout := defaultWaitTimeout
		if step.WaitTimeout > 0 {
			waitTimeout = time.Duration(step.WaitTimeout) * time.Millisecond
		} else if step.WaitTimeout == 0 {
			// 0 means infinite, use a very long timeout (but still cancellable via context)
			waitTimeout = 24 * time.Hour // Effectively infinite, but allows context cancellation
		}

		bh.logger.Debugw("Waiting for process to complete", "app", step.App, "timeout", waitTimeout)
		timeoutCtx, cancel := context.WithTimeout(ctx, waitTimeout)
		defer cancel()

		cmd := exec.CommandContext(timeoutCtx, step.App, step.Args...)

		err := cmd.Run()

		// Check if context was cancelled (not just timeout)
		if ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled) {
			// Context was cancelled, try to kill the process if it's still running
			if cmd.Process != nil {
				bh.logger.Debugw("Killing process due to context cancellation", "app", step.App)
				_ = cmd.Process.Kill() // Ignore error, process may already be dead
			}
			return context.Canceled
		}

		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			// Timeout occurred, kill the process
			if cmd.Process != nil {
				bh.logger.Debugw("Killing process due to timeout", "app", step.App)
				_ = cmd.Process.Kill() // Ignore error, process may already be dead
				// Also kill process group on Linux
				if cmd.Process.Pid > 0 {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
			}
			return &ActionError{
				Type:    ErrorTimeout,
				Message: fmt.Sprintf("Application did not complete within %v", waitTimeout),
				Step:    step,
				Err:     timeoutCtx.Err(),
			}
		}
		return err
	} else {
		// For wait: false, start the process and track it for potential killing on cancel_on_reload
		cmd := exec.CommandContext(ctx, step.App, step.Args...)

		err := cmd.Start()
		if err != nil {
			return err
		}

		// If wait_wnd is set, wait for window to appear
		if step.WaitWnd != nil {
			if err := waitForWindowImpl(ctx, cmd, step, bh.logger); err != nil {
				// Timeout or error - kill the process
				if cmd.Process != nil {
					bh.logger.Debugw("Killing process due to wait_wnd timeout or error", "app", step.App, "error", err)
					_ = cmd.Process.Kill()
				}
				return err
			}
		}

		// Start a goroutine to wait for process completion
		go func() {
			cmd.Wait()
		}()

		return nil
	}
}

// waitForWindowImpl waits for a process window to appear on Linux
// Note: This is not implemented on Linux - window detection requires X11 libraries
func waitForWindowImpl(ctx context.Context, cmdOrPID interface{}, step *ActionStep, logger *zap.SugaredLogger) error {
	var pid int
	switch v := cmdOrPID.(type) {
	case *exec.Cmd:
		if v.Process == nil {
			return fmt.Errorf("process not started")
		}
		pid = v.Process.Pid
	case int:
		pid = v
	default:
		return fmt.Errorf("unsupported type for waitForWindowImpl: %T", cmdOrPID)
	}

	logger.Warnw("wait_wnd is not supported on Linux", "app", step.App, "pid", pid)
	return &ActionError{
		Type:    ErrorExecutionFailed,
		Message: "wait_wnd is not supported on Linux. This feature is Windows-only.",
		Step:    step,
		Err:     errors.New("wait_wnd not supported on Linux"),
	}
}

// setHideWindow is a no-op on Linux (no console window to hide)
func setHideWindow(cmd *exec.Cmd) {
	// No-op on Linux
}

// terminateProcessHandleImpl terminates a process handle (Linux implementation - no-op)
func terminateProcessHandleImpl(hProcess interface{}) error {
	// On Linux, we don't use process handles, so this is a no-op
	return fmt.Errorf("process handles not supported on Linux")
}

// closeProcessHandleImpl closes a process handle (Linux implementation - no-op)
func closeProcessHandleImpl(hProcess interface{}) {
	// On Linux, we don't use process handles, so this is a no-op
}

// isPermissionError checks if an error is a permission error
func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	// Check for common permission error patterns
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "permission denied") ||
		strings.Contains(errStr, "eacces") ||
		strings.Contains(errStr, "access denied")
}
