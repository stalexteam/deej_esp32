package deej

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	defaultWaitTimeout = 30 * time.Second
)

// ActionError represents an error that occurred during action execution
type ActionError struct {
	Type    string
	Message string
	Step    *ActionStep
	Err     error
}

func (e *ActionError) Error() string {
	if e.Step != nil {
		return fmt.Sprintf("%s in step %s: %s", e.Type, e.Step.Type, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

func (e *ActionError) Unwrap() error {
	return e.Err
}

// Error types
const (
	ErrorTimeout              = "timeout"
	ErrorExecutionFailed      = "execution_failed"
	ErrorPermissionDenied     = "permission_denied"
	ErrorKeystrokeUnavailable = "keystroke_unavailable"
)

// ButtonHandler manages button action execution
// It handles button press events, executes action sequences, and manages process lifecycle
type ButtonHandler struct {
	logger         *zap.SugaredLogger
	notifier       Notifier                     // Notifier for showing user notifications
	config         *ButtonsMapping               // Current button configuration (protected by configMutex)
	configMutex    sync.RWMutex                  // Protects config field
	runningActions map[string]context.CancelFunc // Active action contexts keyed by "buttonID_actionType" (protected by actionsMutex)
	actionsMutex   sync.RWMutex                  // Protects runningActions map
	// Tracked processes for forced termination on cancel_on_reload
	trackedProcesses map[string]*exec.Cmd   // Linux: tracked exec.Cmd processes (protected by processMutex)
	trackedHandles   map[string]interface{} // Windows: tracked syscall.Handle (stored as interface{} for build tag compatibility, protected by processMutex)
	processMutex     sync.RWMutex           // Protects trackedProcesses and trackedHandles
}

// NewButtonHandler creates a new ButtonHandler instance
func NewButtonHandler(d *Deej, logger *zap.SugaredLogger) (*ButtonHandler, error) {
	logger = logger.Named("button_handler")

	bh := &ButtonHandler{
		logger:           logger,
		notifier:         d.notifier,
		config:           nil,
		runningActions:   make(map[string]context.CancelFunc),
		trackedProcesses: make(map[string]*exec.Cmd),
		trackedHandles:   make(map[string]interface{}),
	}

	logger.Debug("ButtonHandler created")
	return bh, nil
}

// UpdateConfig updates the button handler configuration
func (bh *ButtonHandler) UpdateConfig(config *buttonsMap) {
	bh.configMutex.Lock()
	defer bh.configMutex.Unlock()

	if config == nil {
		bh.config = nil
		bh.logger.Debug("Button handler configuration cleared")
		return
	}

	// Convert to public ButtonsMapping
	bh.config = config.ToButtonsMapping()
}

// CancelAllActions cancels all currently running button actions and terminates tracked processes
// This is called on config reload (if cancel_on_reload is true) and on shutdown
func (bh *ButtonHandler) CancelAllActions() {
	bh.actionsMutex.Lock()
	actionsToCancel := make(map[string]context.CancelFunc)
	for key, cancel := range bh.runningActions {
		actionsToCancel[key] = cancel
	}
	bh.runningActions = make(map[string]context.CancelFunc)
	bh.actionsMutex.Unlock()

	// Cancel all action contexts
	count := 0
	for _, cancel := range actionsToCancel {
		if cancel != nil {
			cancel()
			count++
		}
	}

	// Force terminate all tracked processes
	bh.processMutex.Lock()
	processesToKill := make(map[string]*exec.Cmd)
	handlesToKill := make(map[string]interface{})
	for key, cmd := range bh.trackedProcesses {
		processesToKill[key] = cmd
	}
	for key, handle := range bh.trackedHandles {
		handlesToKill[key] = handle
	}
	bh.trackedProcesses = make(map[string]*exec.Cmd)
	bh.trackedHandles = make(map[string]interface{})
	bh.processMutex.Unlock()

	// Kill Linux processes
	for key, cmd := range processesToKill {
		if cmd != nil && cmd.Process != nil {
			bh.logger.Debugw("Force killing tracked process", "key", key, "pid", cmd.Process.Pid)
			_ = cmd.Process.Kill() // Ignore error, process may already be dead
		}
	}

	// Kill Windows processes (handled by platform-specific code)
	if len(handlesToKill) > 0 {
		for key, handle := range handlesToKill {
			bh.logger.Debugw("Force terminating tracked process handle", "key", key)
			if err := terminateProcessHandleImpl(handle); err != nil {
				bh.logger.Warnw("Failed to terminate process handle", "key", key, "error", err)
			}
			closeProcessHandleImpl(handle)
		}
	}

	if count > 0 || len(processesToKill) > 0 || len(handlesToKill) > 0 {
		bh.logger.Infow("Cancelled running button actions and terminated processes",
			"actions_count", count,
			"processes_count", len(processesToKill),
			"handles_count", len(handlesToKill))
	}
}

// HandleButtonPress handles a button press event
func (bh *ButtonHandler) HandleButtonPress(buttonID int, actionType string) error {
	bh.configMutex.RLock()
	config := bh.config
	bh.configMutex.RUnlock()

	if config == nil {
		bh.logger.Debugw("No button configuration available", "button", buttonID, "action", actionType)
		return nil
	}

	// Get button configuration
	buttonConfig, ok := config.Buttons[buttonID]
	if !ok {
		bh.logger.Debugw("No configuration for button", "button", buttonID, "action", actionType)
		return nil
	}

	// Get action configuration
	var actionConfig *ButtonActionConfig
	switch actionType {
	case ButtonActionSingle:
		actionConfig = buttonConfig.Single
	case ButtonActionDouble:
		actionConfig = buttonConfig.Double
	case ButtonActionLong:
		actionConfig = buttonConfig.Long
	default:
		bh.logger.Warnw("Unknown action type", "button", buttonID, "action", actionType)
		return fmt.Errorf("unknown action type: %s", actionType)
	}

	if actionConfig == nil {
		bh.logger.Debugw("No action configuration for button/action", "button", buttonID, "action", actionType)
		return nil
	}

	if len(actionConfig.Steps) == 0 {
		bh.logger.Debugw("Empty steps for button/action", "button", buttonID, "action", actionType)
		return nil
	}

	// Generate unique key for this action
	key := fmt.Sprintf("%d_%s", buttonID, actionType)

	// Copy actionConfig data to avoid race conditions
	// We copy the data before releasing the mutex and launching the goroutine
	exclusive := actionConfig.Exclusive
	steps := make([]ActionStep, len(actionConfig.Steps))
	copy(steps, actionConfig.Steps)

	bh.logger.Infow("Starting button action",
		"button", buttonID,
		"action", actionType,
		"exclusive", exclusive,
		"steps_count", len(steps),
		"steps", steps)

	// Check if action is exclusive and already running
	if exclusive {
		bh.actionsMutex.RLock()
		_, running := bh.runningActions[key]
		bh.actionsMutex.RUnlock()

		if running {
			bh.logger.Debugw("Action already running (exclusive)", "button", buttonID, "action", actionType)
			return nil
		}
	}

	// Create context for this action
	ctx, cancel := context.WithCancel(context.Background())

	// Track the action (track ALL actions, not just exclusive, for cancellation on reload)
	bh.actionsMutex.Lock()
	bh.runningActions[key] = cancel
	bh.actionsMutex.Unlock()

	// Execute action in goroutine to avoid blocking the main event handler
	go func() {
		// Recover from panics to prevent goroutine crash and application termination
		defer func() {
			if r := recover(); r != nil {
				bh.logger.Errorw("Panic in button action goroutine", "button", buttonID, "action", actionType, "panic", r)
			}

			// Cleanup: remove from running actions map
			bh.actionsMutex.Lock()
			delete(bh.runningActions, key)
			bh.actionsMutex.Unlock()

			// Cancel context to signal completion
			cancel()
		}()

		err := bh.executeAction(ctx, steps, buttonID, actionType, key)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				bh.logger.Debugw("Action cancelled", "button", buttonID, "action", actionType)
			} else {
				bh.logger.Warnw("Action execution failed", "button", buttonID, "action", actionType, "error", err)
				// Notify user about the error
				var title, message string
				if actionErr, ok := err.(*ActionError); ok && actionErr.Step != nil {
					// Extract user-friendly message from ActionError
					if actionErr.Step.Type == "execute" && actionErr.Step.App != "" {
						title = "Failed to execute application"
						message = fmt.Sprintf("Cannot find or run: %s\n\nPlease check your config.yaml file.", actionErr.Step.App)
					} else {
						title = "Button action failed"
						message = actionErr.Message
					}
				} else {
					// Generic error message
					title = "Button action failed"
					message = err.Error()
				}
				bh.notifier.Notify(title, message)
			}
		} else {
			bh.logger.Debugw("Action completed successfully", "button", buttonID, "action", actionType)
		}
	}()

	return nil
}

// executeAction executes a sequence of action steps
func (bh *ButtonHandler) executeAction(ctx context.Context, steps []ActionStep, buttonID int, actionType string, key string) error {
	for stepIdx, step := range steps {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}

		bh.logger.Debugw("Executing step", "button", buttonID, "action", actionType, "step", stepIdx, "type", step.Type)

		var err error
		switch step.Type {
		case ActionTypeExecute:
			err = executeActionPlatform(ctx, &step, buttonID, actionType, key, bh)
			// Note: Window readiness is verified using SendMessageTimeout in executeActionPlatform
			// No additional delay needed here
		case ActionTypeDelay:
			err = bh.executeDelay(ctx, &step)
		case ActionTypeKeystroke:
			err = keystrokeActionImpl(ctx, &step, bh.logger)
		case ActionTypeTyping:
			// Window readiness is verified using SendMessageTimeout in typingActionImpl
			// No fixed delay needed here - the platform-specific implementation handles it
			err = typingActionImpl(ctx, &step, bh.logger)
		default:
			err = fmt.Errorf("unknown step type: %s", step.Type)
		}

		if err != nil {
			return fmt.Errorf("step %d (%s) failed: %w", stepIdx, step.Type, err)
		}
	}

	return nil
}

// executeDelay executes a delay step
func (bh *ButtonHandler) executeDelay(ctx context.Context, step *ActionStep) error {
	if step.Ms <= 0 {
		return fmt.Errorf("delay ms must be positive, got %d", step.Ms)
	}

	bh.logger.Debugw("Delaying", "ms", step.Ms)

	select {
	case <-ctx.Done():
		return context.Canceled
	case <-time.After(time.Duration(step.Ms) * time.Millisecond):
		return nil
	}
}

// trackProcess tracks a Linux process (exec.Cmd) for forced termination on cancel_on_reload
// The process can be killed later via CancelAllActions
func (bh *ButtonHandler) trackProcess(key string, cmd *exec.Cmd) {
	bh.processMutex.Lock()
	defer bh.processMutex.Unlock()

	if cmd != nil && cmd.Process != nil {
		bh.trackedProcesses[key] = cmd
		bh.logger.Debugw("Tracking process", "key", key, "pid", cmd.Process.Pid)
	}
}

// untrackProcess untracks a Linux process when it completes or is no longer needed
func (bh *ButtonHandler) untrackProcess(key string, cmd *exec.Cmd) {
	bh.processMutex.Lock()
	defer bh.processMutex.Unlock()

	if existingCmd, ok := bh.trackedProcesses[key]; ok && existingCmd == cmd {
		delete(bh.trackedProcesses, key)
		if cmd != nil && cmd.Process != nil {
			bh.logger.Debugw("Untracking process", "key", key, "pid", cmd.Process.Pid)
		}
	}
}

// trackProcessHandle tracks a Windows process handle (syscall.Handle) for forced termination on cancel_on_reload
// The handle is stored as interface{} for build tag compatibility
func (bh *ButtonHandler) trackProcessHandle(key string, hProcess interface{}) {
	bh.processMutex.Lock()
	defer bh.processMutex.Unlock()

	bh.trackedHandles[key] = hProcess
	bh.logger.Debugw("Tracking process handle", "key", key)
}

// untrackProcessHandle untracks a Windows process handle when it completes or is no longer needed
func (bh *ButtonHandler) untrackProcessHandle(key string, hProcess interface{}) {
	bh.processMutex.Lock()
	defer bh.processMutex.Unlock()

	if existingHandle, ok := bh.trackedHandles[key]; ok && existingHandle == hProcess {
		delete(bh.trackedHandles, key)
		bh.logger.Debugw("Untracking process handle", "key", key)
	}
}
