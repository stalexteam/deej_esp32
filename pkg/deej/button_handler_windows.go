//go:build windows
// +build windows

package deej

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/win"
	"go.uber.org/zap"
)

var (
	moduser32   = syscall.NewLazyDLL("user32.dll")
	modshell32  = syscall.NewLazyDLL("shell32.dll")
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	modole32    = syscall.NewLazyDLL("ole32.dll")

	procKeybdEvent               = moduser32.NewProc("keybd_event")
	procShellExecuteEx           = modshell32.NewProc("ShellExecuteExW")
	procWaitForSingleObject      = modkernel32.NewProc("WaitForSingleObject")
	procTerminateProcess         = modkernel32.NewProc("TerminateProcess")
	procGetProcessId             = modkernel32.NewProc("GetProcessId")
	procGetLastError             = modkernel32.NewProc("GetLastError")
	procCloseHandle              = modkernel32.NewProc("CloseHandle")
	procCoInitializeEx           = modole32.NewProc("CoInitializeEx")
	procCoUninitialize           = modole32.NewProc("CoUninitialize")
	procWaitForInputIdle         = moduser32.NewProc("WaitForInputIdle")
	procGetForegroundWindow      = moduser32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = moduser32.NewProc("GetWindowThreadProcessId")
	procSetForegroundWindow      = moduser32.NewProc("SetForegroundWindow")
	procAttachThreadInput        = moduser32.NewProc("AttachThreadInput")
	procGetCurrentThreadId       = modkernel32.NewProc("GetCurrentThreadId")
	procSendMessageTimeout       = moduser32.NewProc("SendMessageTimeoutW")
	procSetErrorMode             = modkernel32.NewProc("SetErrorMode")
)

const (
	KEYEVENTF_KEYUP          = 0x0002
	KEYEVENTF_UNICODE        = 0x0004
	SEE_MASK_NOCLOSEPROCESS  = 0x00000040
	SEE_MASK_UNICODE         = 0x00004000
	SEE_MASK_FLAG_NO_UI      = 0x00000100
	SEM_FAILCRITICALERRORS   = 0x0001
	SEM_NOOPENFILEERRORBOX   = 0x8000
	SEM_NOGPFAULTERRORBOX     = 0x0002
	SW_SHOWDEFAULT           = 10
	INFINITE                 = 0xFFFFFFFF
	COINIT_APARTMENTTHREADED = 0x2
	COINIT_DISABLE_OLE1DDE   = 0x4
	S_OK                     = 0x0
	RPC_E_CHANGED_MODE       = 0x80010106
	WM_NULL                  = 0x0000
	SMTO_ABORTIFHUNG         = 0x0002
	SMTO_BLOCK               = 0x0001

	// Timeouts and delays
	sendMessageTimeoutMs    = 100             // Timeout for SendMessageTimeout window readiness check (ms)
	defaultCharDelayMs      = 1               // Default delay between typed characters (ms)
	waitForInputIdleTimeout = 5 * time.Second // Timeout for WaitForInputIdle
)

// init sets up global error mode suppression for Windows
// This prevents Windows from showing error dialogs for missing files, critical errors, etc.
// SetErrorMode is called at package initialization to suppress error dialogs globally
func init() {
	procSetErrorMode.Call(
		SEM_FAILCRITICALERRORS | SEM_NOOPENFILEERRORBOX | SEM_NOGPFAULTERRORBOX,
	)
}

// keystrokeActionImpl implements keystroke simulation for Windows using keybd_event
func keystrokeActionImpl(ctx context.Context, step *ActionStep, logger *zap.SugaredLogger) error {
	if step.Keys == "" {
		return fmt.Errorf("keys is required for keystroke action")
	}

	// Parse key combination (format: "Ctrl+Alt+T" or "Ctrl+Shift+A")
	keys := strings.Split(step.Keys, "+")
	if len(keys) == 0 {
		return fmt.Errorf("invalid key combination: %s", step.Keys)
	}

	// Press modifiers first
	for i := 0; i < len(keys)-1; i++ {
		k := strings.TrimSpace(strings.ToLower(keys[i]))
		vk := getVirtualKeyCode(k)
		if vk != 0 {
			// Press modifier
			procKeybdEvent.Call(uintptr(vk), 0, 0, 0)
		}
	}

	// Press the main key
	mainKey := strings.TrimSpace(keys[len(keys)-1])
	vk := getVirtualKeyCode(mainKey)
	if vk != 0 {
		// Press main key
		procKeybdEvent.Call(uintptr(vk), 0, 0, 0)
		// Release main key
		procKeybdEvent.Call(uintptr(vk), 0, KEYEVENTF_KEYUP, 0)
	}

	// Release modifiers (in reverse order)
	for i := len(keys) - 2; i >= 0; i-- {
		k := strings.TrimSpace(strings.ToLower(keys[i]))
		vk := getVirtualKeyCode(k)
		if vk != 0 {
			// Release modifier
			procKeybdEvent.Call(uintptr(vk), 0, KEYEVENTF_KEYUP, 0)
		}
	}

	return nil
}

// getVirtualKeyCode returns Windows virtual key code for a key name
func getVirtualKeyCode(keyName string) uintptr {
	keyLower := strings.ToLower(keyName)

	switch keyLower {
	case "ctrl", "control":
		return 0x11 // VK_CONTROL
	case "alt":
		return 0x12 // VK_MENU (Alt key)
	case "shift":
		return 0x10 // VK_SHIFT
	case "win", "windows", "meta":
		return 0x5B // VK_LWIN
	case "f1":
		return 0x70 // VK_F1
	case "f2":
		return 0x71 // VK_F2
	case "f3":
		return 0x72 // VK_F3
	case "f4":
		return 0x73 // VK_F4
	case "f5":
		return 0x74 // VK_F5
	case "f6":
		return 0x75 // VK_F6
	case "f7":
		return 0x76 // VK_F7
	case "f8":
		return 0x77 // VK_F8
	case "f9":
		return 0x78 // VK_F9
	case "f10":
		return 0x79 // VK_F10
	case "f11":
		return 0x7A // VK_F11
	case "f12":
		return 0x7B // VK_F12
	case "f13":
		return 0x7C // VK_F13
	case "f14":
		return 0x7D // VK_F14
	case "f15":
		return 0x7E // VK_F15
	case "f16":
		return 0x7F // VK_F16
	case "f17":
		return 0x80 // VK_F17
	case "f18":
		return 0x81 // VK_F18
	case "f19":
		return 0x82 // VK_F19
	case "f20":
		return 0x83 // VK_F20
	case "f21":
		return 0x84 // VK_F21
	case "f22":
		return 0x85 // VK_F22
	case "f23":
		return 0x86 // VK_F23
	case "f24":
		return 0x87 // VK_F24
	case "enter", "return":
		return 0x0D // VK_RETURN
	case "tab":
		return 0x09 // VK_TAB
	case "escape", "esc":
		return 0x1B // VK_ESCAPE
	case "backspace":
		return 0x08 // VK_BACK
	case "delete", "del":
		return 0x2E // VK_DELETE
	case "insert", "ins":
		return 0x2D // VK_INSERT
	case "home":
		return 0x24 // VK_HOME
	case "end":
		return 0x23 // VK_END
	case "pageup", "pgup":
		return 0x21 // VK_PRIOR
	case "pagedown", "pgdn":
		return 0x22 // VK_NEXT
	case "up":
		return 0x26 // VK_UP
	case "down":
		return 0x28 // VK_DOWN
	case "left":
		return 0x25 // VK_LEFT
	case "right":
		return 0x27 // VK_RIGHT
	case "space", " ":
		return 0x20 // VK_SPACE
	case "printscreen", "prtsc", "prtscr", "print":
		return 0x2C // VK_SNAPSHOT
	case "scrolllock", "scroll":
		return 0x91 // VK_SCROLL
	case "pausebreak", "break":
		return 0x13 // VK_PAUSE
	case "capslock", "caps":
		return 0x14 // VK_CAPITAL
	case "numlock", "num":
		return 0x90 // VK_NUMLOCK
	case "menu", "contextmenu", "apps":
		return 0x5D // VK_APPS (Context Menu key)
	// NumPad keys
	case "numpad0", "np0":
		return 0x60 // VK_NUMPAD0
	case "numpad1", "np1":
		return 0x61 // VK_NUMPAD1
	case "numpad2", "np2":
		return 0x62 // VK_NUMPAD2
	case "numpad3", "np3":
		return 0x63 // VK_NUMPAD3
	case "numpad4", "np4":
		return 0x64 // VK_NUMPAD4
	case "numpad5", "np5":
		return 0x65 // VK_NUMPAD5
	case "numpad6", "np6":
		return 0x66 // VK_NUMPAD6
	case "numpad7", "np7":
		return 0x67 // VK_NUMPAD7
	case "numpad8", "np8":
		return 0x68 // VK_NUMPAD8
	case "numpad9", "np9":
		return 0x69 // VK_NUMPAD9
	case "numpadmultiply", "numpad*", "np*", "npmultiply":
		return 0x6A // VK_MULTIPLY
	case "numpadadd", "numpad+", "np+", "npadd":
		return 0x6B // VK_ADD
	case "numpadsubtract", "numpad-", "np-", "npsubtract":
		return 0x6D // VK_SUBTRACT
	case "numpaddecimal", "numpad.", "np.", "npdecimal":
		return 0x6E // VK_DECIMAL
	case "numpaddivide", "numpad/", "np/", "npdivide":
		return 0x6F // VK_DIVIDE
	case "numpadenter", "npenter":
		return 0x0D // VK_RETURN (same as Enter)
	// Media keys
	case "volumemute", "volmute", "mute":
		return 0xAD // VK_VOLUME_MUTE
	case "volumedown", "voldown":
		return 0xAE // VK_VOLUME_DOWN
	case "volumeup", "volup":
		return 0xAF // VK_VOLUME_UP
	case "medianexttrack", "nexttrack", "next":
		return 0xB0 // VK_MEDIA_NEXT_TRACK
	case "mediaprevtrack", "prevtrack", "prev", "previous":
		return 0xB1 // VK_MEDIA_PREV_TRACK
	case "mediastop", "stop":
		return 0xB2 // VK_MEDIA_STOP
	case "mediaplaypause", "playpause", "play", "pause":
		return 0xB3 // VK_MEDIA_PLAY_PAUSE
	default:
		// For single character keys, convert to virtual key code
		if len(keyName) == 1 {
			// Convert to uppercase and get VK code
			upper := strings.ToUpper(keyName)
			return uintptr(upper[0])
		}
		// Unknown key
		return 0
	}
}

// typingActionImpl implements text typing simulation for Windows using keybd_event with KEYEVENTF_UNICODE
func typingActionImpl(ctx context.Context, step *ActionStep, logger *zap.SugaredLogger) error {
	// Get current foreground window for debugging and ensure it's focused
	fgHwnd, _, _ := procGetForegroundWindow.Call()
	var fgPID uint32
	var fgTitle string
	var targetThreadID uintptr
	var currentThreadID uintptr
	var inputAttached bool

	if fgHwnd != 0 {
		procGetWindowThreadProcessId.Call(fgHwnd, uintptr(unsafe.Pointer(&fgPID)))
		fgTitle = getWindowTitle(win.HWND(fgHwnd))
		logger.Infow("Starting typing action", "text_length", len(step.Text), "char_delay", step.CharDelay, "fg_hwnd", fgHwnd, "fg_pid", fgPID, "fg_title", fgTitle)

		// Ensure the foreground window is actually focused before typing
		// This is especially important for launcher applications where the window might lose focus
		// We need to attach thread input and keep it attached during typing for keybd_event to work correctly
		if fgHwnd != 0 {
			logger.Debugw("Ensuring window is focused before typing", "hwnd", fgHwnd, "title", fgTitle)

			// Get thread IDs
			var dummyPID uint32
			targetThreadID, _, _ = procGetWindowThreadProcessId.Call(uintptr(fgHwnd), uintptr(unsafe.Pointer(&dummyPID)))
			currentThreadID, _, _ = procGetCurrentThreadId.Call()

			// Attach thread input if threads are different
			// This is critical for keybd_event to work correctly with the target window
			if targetThreadID != currentThreadID {
				ret, _, _ := procAttachThreadInput.Call(
					currentThreadID,
					targetThreadID,
					1, // TRUE - attach
				)
				if ret != 0 {
					inputAttached = true
				}
			}

			// Set foreground window
			procSetForegroundWindow.Call(uintptr(fgHwnd))

			// Verify focus was set and use SendMessageTimeout to check window readiness
			// This is more reliable than fixed delays - it waits for the window to actually be ready
			newFgHwnd, _, _ := procGetForegroundWindow.Call()
			if newFgHwnd == fgHwnd {
				// Use SendMessageTimeout with WM_NULL to verify window is ready for input
				// SMTO_BLOCK: blocks the calling thread until the function returns
				// SMTO_ABORTIFHUNG: returns immediately if the window is hung
				// This is more reliable than a fixed delay
				timeoutMs := uint32(sendMessageTimeoutMs)
				var result uintptr
				ret, _, _ := procSendMessageTimeout.Call(
					uintptr(fgHwnd),                  // hWnd
					WM_NULL,                          // Msg
					0,                                // wParam
					0,                                // lParam
					SMTO_BLOCK|SMTO_ABORTIFHUNG,      // fuFlags: block until ready, abort if hung
					uintptr(timeoutMs),               // uTimeout
					uintptr(unsafe.Pointer(&result)), // lpdwResult
				)

				if ret == 0 {
					// SendMessageTimeout failed or timed out - window may not be ready
					logger.Warnw("SendMessageTimeout failed or timed out, window may not be ready", "hwnd", fgHwnd, "title", fgTitle)
					// Add a small delay as fallback only if SendMessageTimeout failed
					// Use longer delay if AttachThreadInput failed
					if inputAttached {
						time.Sleep(100 * time.Millisecond)
					} else {
						time.Sleep(200 * time.Millisecond)
					}
				}
			} else {
				logger.Warnw("Window focus may have changed", "expected_hwnd", fgHwnd, "actual_hwnd", newFgHwnd)
				// Add a small delay as fallback
				time.Sleep(100 * time.Millisecond)
			}
		}
	} else {
		logger.Infow("Starting typing action", "text_length", len(step.Text), "char_delay", step.CharDelay, "fg_hwnd", 0)
		logger.Warnw("No foreground window found, typing may not work correctly")
	}

	// Ensure we detach thread input when done
	defer func() {
		if inputAttached && targetThreadID != currentThreadID {
			procAttachThreadInput.Call(
				currentThreadID,
				targetThreadID,
				0, // FALSE - detach
			)
		}
	}()

	// Validate text is not empty
	if step.Text == "" {
		return fmt.Errorf("text is required for typing action")
	}

	// Process escape sequences in text
	processedText := processEscapeSequences(step.Text)

	// Convert to UTF-16 for Unicode support
	utf16Text := syscall.StringToUTF16(processedText)

	// Remove trailing null terminator if present (StringToUTF16 adds it)
	// Also filter out any null characters from the middle
	utf16TextFiltered := make([]uint16, 0, len(utf16Text))
	for _, char := range utf16Text {
		if char != 0 {
			utf16TextFiltered = append(utf16TextFiltered, char)
		}
	}

	// Type each character using keybd_event with KEYEVENTF_UNICODE
	// For KEYEVENTF_UNICODE:
	// bVk must be 0
	// bScan contains the 16-bit Unicode character
	for i, char := range utf16TextFiltered {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}

		// Add delay BEFORE typing character (except for the first one)
		// This ensures window has time to process previous events
		// If char_delay is specified, use it; otherwise use a default minimum delay
		if i > 0 {
			charDelay := step.CharDelay
			if charDelay == 0 {
				charDelay = defaultCharDelayMs // Default minimum delay between characters
			}
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-time.After(time.Duration(charDelay) * time.Millisecond):
			}
		}

		// Handle special keys (Enter, Tab, etc.)
		if char == '\n' {
			// Enter key - use virtual key to avoid issues
			sendVirtualKey(0x0D) // VK_RETURN
		} else if char == '\t' {
			// Tab key - use virtual key to avoid issues
			sendVirtualKey(0x09) // VK_TAB
		} else if char == '\r' {
			// Carriage return - same as Enter
			sendVirtualKey(0x0D) // VK_RETURN
		} else {
			// Use KEYEVENTF_UNICODE for all other characters
			// This is the most reliable method for text input
			// Press
			procKeybdEvent.Call(0, uintptr(char), KEYEVENTF_UNICODE, 0)
			// Delay to ensure key press is registered
			time.Sleep(10 * time.Millisecond)
			// Release
			procKeybdEvent.Call(0, uintptr(char), KEYEVENTF_UNICODE|KEYEVENTF_KEYUP, 0)
		}
	}

	// Ensure all modifier keys are released at the end
	// This prevents issues where modifiers might be stuck
	modifiers := []uintptr{0x10, 0x11, 0x12, 0x5B, 0x5C} // VK_SHIFT, VK_CONTROL, VK_MENU, VK_LWIN, VK_RWIN

	for _, vk := range modifiers {
		procKeybdEvent.Call(vk, 0, KEYEVENTF_KEYUP, 0)
		time.Sleep(5 * time.Millisecond) // Small delay between each modifier
	}

	time.Sleep(10 * time.Millisecond) // Small delay after releasing modifiers

	return nil
}

// sendVirtualKey sends a virtual key press and release
func sendVirtualKey(vk uintptr) {
	// Press
	procKeybdEvent.Call(vk, 0, 0, 0)
	// Small delay
	time.Sleep(5 * time.Millisecond)
	// Release
	procKeybdEvent.Call(vk, 0, KEYEVENTF_KEYUP, 0)
}

// findWindowByPID finds the main window belonging to the specified process ID
// Prefers windows without a parent (top-level windows) and logs all found windows for debugging
func findWindowByPID(pid int, titleFilter string, logger *zap.SugaredLogger) win.HWND {
	var foundWindow win.HWND
	var candidateWindows []win.HWND
	targetPID := uint32(pid)

	// Callback function for EnumWindows
	enumProc := syscall.NewCallback(func(hwnd win.HWND, lParam uintptr) uintptr {
		var windowPID uint32
		win.GetWindowThreadProcessId(hwnd, &windowPID)

		// Check if this window belongs to our process
		if windowPID == targetPID {
			parent := win.GetParent(hwnd)
			isTopLevel := parent == 0
			isVisible := win.IsWindowVisible(hwnd)

			// Check if window is visible (required for user interaction)
			if isVisible {
				// If title filter is specified, check window title
				titleMatches := true
				if titleFilter != "" {
					title := getWindowTitle(hwnd)
					titleMatches = strings.Contains(strings.ToLower(title), strings.ToLower(titleFilter))
				}

				// Collect candidate windows (visible windows)
				if titleMatches {
					candidateWindows = append(candidateWindows, hwnd)

					// Prefer top-level windows (main windows), but accept any visible window
					if isTopLevel && foundWindow == 0 {
						foundWindow = hwnd
					}
				}
			}
		}
		return 1 // Continue enumeration
	})

	// Enumerate all top-level windows using user32.dll EnumWindows
	moduser32 := syscall.NewLazyDLL("user32.dll")
	procEnumWindows := moduser32.NewProc("EnumWindows")
	procEnumWindows.Call(uintptr(enumProc), 0)

	// If no top-level window found, use first candidate (any visible window)
	if foundWindow == 0 && len(candidateWindows) > 0 {
		foundWindow = candidateWindows[0]
	}

	if foundWindow == 0 {
		logger.Debugw("No window found for process", "pid", pid, "candidates", len(candidateWindows))
	}

	return foundWindow
}

// getWindowTitle retrieves the title of a window
func getWindowTitle(hwnd win.HWND) string {
	moduser32 := syscall.NewLazyDLL("user32.dll")
	procGetWindowTextLength := moduser32.NewProc("GetWindowTextLengthW")
	procGetWindowText := moduser32.NewProc("GetWindowTextW")

	length, _, _ := procGetWindowTextLength.Call(uintptr(hwnd))
	if length == 0 {
		return ""
	}

	buf := make([]uint16, length+1)
	procGetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

// setWindowFocus attempts to bring a window to foreground and set focus
// Uses AttachThreadInput + SetForegroundWindow for better reliability
func setWindowFocus(hwnd win.HWND, logger *zap.SugaredLogger) bool {
	if hwnd == 0 {
		return false
	}

	// Get the thread ID of the target window
	// GetWindowThreadProcessId returns thread ID as return value, process ID via pointer parameter
	var dummyPID uint32
	targetThreadID, _, _ := procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&dummyPID)))

	// Get the current thread ID
	currentThreadID, _, _ := procGetCurrentThreadId.Call()

	// Attach input to allow SetForegroundWindow to work
	// This is needed because Windows restricts SetForegroundWindow to prevent focus stealing
	attached := false
	if targetThreadID != currentThreadID {
		ret, _, _ := procAttachThreadInput.Call(
			currentThreadID,
			targetThreadID,
			1, // TRUE - attach
		)
		if ret != 0 {
			attached = true
			defer func() {
				// Detach when done
				procAttachThreadInput.Call(
					currentThreadID,
					targetThreadID,
					0, // FALSE - detach
				)
			}()
		}
	}

	// Try to set foreground window
	ret, _, _ := procSetForegroundWindow.Call(uintptr(hwnd))
	if ret != 0 {
		logger.Debugw("SetForegroundWindow succeeded", "hwnd", hwnd, "attached", attached)
		return true
	}

	logger.Debugw("SetForegroundWindow failed", "hwnd", hwnd, "attached", attached)
	return false
}

// setHideWindow sets HideWindow flag for Windows to hide console window
func setHideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

// SHELLEXECUTEINFO structure for ShellExecuteEx
// Following the structure from nyaosorg/go-windows-su example
type shellExecuteInfo struct {
	size          uint32
	mask          uint32
	hwnd          uintptr
	verb          *uint16
	file          *uint16
	parameter     *uint16
	directory     *uint16
	show          int32
	instApp       uintptr
	idList        uintptr
	class         *uint16
	keyClass      uintptr
	hotkey        uint32
	iconOrMonitor uintptr
	hProcess      syscall.Handle
}

// executeActionPlatform executes an application using ShellExecuteEx on Windows
// Following the approach from nyaosorg/go-windows-su with COM initialization
// Note: SetErrorMode is set globally in init() to suppress error dialogs
func executeActionPlatform(ctx context.Context, step *ActionStep, buttonID int, actionType string, key string, bh *ButtonHandler) error {
	// Check if file exists before attempting to launch
	// This prevents error dialogs from appearing
	if step.App != "" {
		// Try to resolve the executable path
		appPath := step.App
		// If it's not an absolute path, try to find it in PATH
		if !filepath.IsAbs(appPath) {
			// Check if file exists in current directory
			if _, err := os.Stat(appPath); os.IsNotExist(err) {
				// Try to find in PATH
				path, err := exec.LookPath(appPath)
				if err != nil {
					// File not found - return error without showing dialog
					return fmt.Errorf("executable not found: %s", step.App)
				}
				appPath = path
			}
		} else {
			// Absolute path - check if file exists
			if _, err := os.Stat(appPath); os.IsNotExist(err) {
				return fmt.Errorf("executable not found: %s", step.App)
			}
		}
		// Update step.App with resolved path
		step.App = appPath
	}

	// Initialize COM for current thread (required for ShellExecuteEx)
	// Following Pascal code: NeedUnitialize := Assigned(CoInitializeEx) and Succeeded(CoInitializeEx(...))
	hr, _, _ := procCoInitializeEx.Call(0, COINIT_APARTMENTTHREADED|COINIT_DISABLE_OLE1DDE)
	needUninitialize := hr == S_OK || hr == RPC_E_CHANGED_MODE // S_OK or RPC_E_CHANGED_MODE (already initialized)
	if needUninitialize {
		defer procCoUninitialize.Call()
	}

	// Initialize SHELLEXECUTEINFO structure
	// SEE_MASK_FLAG_NO_UI: Suppress error dialogs from ShellExecuteEx itself
	var sei shellExecuteInfo
	sei.size = uint32(unsafe.Sizeof(sei))
	sei.mask = SEE_MASK_NOCLOSEPROCESS | SEE_MASK_UNICODE | SEE_MASK_FLAG_NO_UI
	sei.hwnd = 0 // Use 0 as in the example
	sei.show = SW_SHOWDEFAULT

	// Convert strings to UTF-16
	var err error
	sei.verb, err = syscall.UTF16PtrFromString("open")
	if err != nil {
		return fmt.Errorf("failed to create verb string: %w", err)
	}

	sei.file, err = syscall.UTF16PtrFromString(step.App)
	if err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}

	var params string
	if len(step.Args) > 0 {
		params = strings.Join(step.Args, " ")
		sei.parameter, err = syscall.UTF16PtrFromString(params)
		if err != nil {
			return fmt.Errorf("invalid parameters: %w", err)
		}
	}

	// Directory is nil (use current directory)
	sei.directory = nil

	bh.logger.Debugw("Calling ShellExecuteEx",
		"app", step.App,
		"verb", "open",
		"mask", fmt.Sprintf("0x%x", sei.mask),
		"size", sei.size)

	// Call ShellExecuteExW
	status, _, _ := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&sei)))

	if status == 0 {
		// Get detailed error using GetLastError
		errCode, _, _ := procGetLastError.Call()
		err := syscall.Errno(errCode)

		bh.logger.Debugw("ShellExecuteEx failed",
			"app", step.App,
			"error_code", errCode,
			"error", err)

		return fmt.Errorf("ShellExecuteEx failed: %w", err)
	}

	// Check if process handle was returned
	if sei.hProcess == 0 || sei.hProcess == syscall.InvalidHandle {
		// No process was launched (e.g., opened existing document)
		bh.logger.Debugw("No process handle returned (may have opened existing document)", "app", step.App)
		return nil
	}

	// Track if handle has been closed to avoid double-close
	handleClosed := false
	defer func() {
		if !handleClosed {
			closeHandle(sei.hProcess)
		}
	}()

	// Get process ID for later use
	pid, err := getProcessID(sei.hProcess)
	if err != nil {
		bh.logger.Debugw("Failed to get process ID, but continuing", "app", step.App, "error", err)
		pid = 0
	}

	// Call WaitForInputIdle only if wait_wnd is not configured
	// If wait_wnd is used, we'll wait for the window to appear instead
	if pid != 0 && step.WaitWnd == nil {
		// Use a reasonable timeout for WaitForInputIdle
		timeoutMs := uint32(waitForInputIdleTimeout.Milliseconds())

		bh.logger.Debugw("Calling WaitForInputIdle", "app", step.App, "pid", pid, "timeout_ms", timeoutMs)
		ret, _, _ := procWaitForInputIdle.Call(uintptr(sei.hProcess), uintptr(timeoutMs))
		if ret != 0 {
			// WaitForInputIdle failed or timed out - not critical, continue anyway
			bh.logger.Debugw("WaitForInputIdle returned non-zero (process may still be initializing)", "app", step.App, "pid", pid, "ret", ret)
		} else {
			bh.logger.Debugw("WaitForInputIdle succeeded, process is ready", "app", step.App, "pid", pid)
		}
	}

	if step.Wait {
		// For wait: true, wait for process completion with timeout
		// Determine timeout: use wait_timeout if specified, otherwise use defaultWaitTimeout
		waitTimeout := defaultWaitTimeout
		if step.WaitTimeout > 0 {
			waitTimeout = time.Duration(step.WaitTimeout) * time.Millisecond
		} else if step.WaitTimeout == 0 {
			// 0 means infinite, use a very long timeout (but still cancellable via context)
			waitTimeout = 24 * time.Hour // Effectively infinite, but allows context cancellation
		}

		if pid != 0 {
			bh.logger.Debugw("Waiting for process to complete", "app", step.App, "pid", pid, "timeout", waitTimeout)
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, waitTimeout)
		defer cancel()

		// Wait for process in a goroutine to allow cancellation
		done := make(chan error, 1)
		go func() {
			done <- waitForProcess(timeoutCtx, sei.hProcess, waitTimeout)
		}()

		select {
		case <-ctx.Done():
			// Context was cancelled, terminate the process
			bh.logger.Debugw("Killing process due to context cancellation", "app", step.App)
			_ = terminateProcess(sei.hProcess)
			closeHandle(sei.hProcess)
			handleClosed = true
			return context.Canceled
		case err := <-done:
			// Always close handle after wait completes (success or failure)
			closeHandle(sei.hProcess)
			handleClosed = true

			if err != nil {
				// Timeout or error occurred - kill the process
				bh.logger.Debugw("Process wait failed (timeout or error), killing process", "app", step.App, "error", err)
				_ = terminateProcess(sei.hProcess)
				return err
			}
			return nil
		}
	} else {
		// For wait: false, handle wait_wnd if configured
		if step.WaitWnd != nil {

			timeout := time.Duration(step.WaitWnd.Timeout) * time.Millisecond
			checkFocused := step.WaitWnd.Focused

			// Check if process has terminated (launcher case)
			isLauncher := false
			if pid != 0 {
				if isProcessTerminated(sei.hProcess) {
					isLauncher = true
				}
			}

			if checkFocused {
				// For focused: true, wait for window to appear in foreground
				// For launchers: ignore PID check, accept any foreground window
				// For non-launchers: check that foreground window belongs to our process

				timeoutCtx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()

				pollInterval := 50 * time.Millisecond
				ticker := time.NewTicker(pollInterval)
				defer ticker.Stop()

				targetPID := uint32(pid)
				lastSetFocusTime := time.Time{}
				setFocusInterval := 200 * time.Millisecond

				for {
					select {
					case <-timeoutCtx.Done():
						if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
							// Timeout - kill process if still running and return error
							if !isLauncher {
								_ = terminateProcess(sei.hProcess)
							}
							closeHandle(sei.hProcess)
							handleClosed = true
							return &ActionError{
								Type:    ErrorTimeout,
								Message: fmt.Sprintf("Window did not appear in foreground within %v", timeout),
								Step:    step,
								Err:     timeoutCtx.Err(),
							}
						}
						return context.Canceled
					case <-ticker.C:
						// For non-launchers, check if process has terminated during wait (may become launcher)
						if !isLauncher && pid != 0 {
							if isProcessTerminated(sei.hProcess) {
								isLauncher = true
							}
						}

						// Check foreground window
						fgHwnd, _, _ := procGetForegroundWindow.Call()
						if fgHwnd == 0 {
							continue
						}

						var fgPID uint32
						procGetWindowThreadProcessId.Call(fgHwnd, uintptr(unsafe.Pointer(&fgPID)))

						// For launchers: ignore PID check and accept any foreground window
						// For non-launchers: check that foreground window belongs to our process
						if isLauncher || fgPID == targetPID {
							// Window is in foreground - verify it's ready for input using SendMessageTimeout (once)
							title := getWindowTitle(win.HWND(fgHwnd))

							// Use SendMessageTimeout with WM_NULL to verify window is ready for input
							// SMTO_BLOCK: blocks the calling thread until the function returns
							// SMTO_ABORTIFHUNG: returns immediately if the window is hung
							// This is more reliable than a fixed delay
							// Only call once when window is found, not in every tick
							// Use shorter timeout - if window is ready, it will respond quickly
							timeoutMs := uint32(sendMessageTimeoutMs)
							var result uintptr
							ret, _, _ := procSendMessageTimeout.Call(
								uintptr(fgHwnd),                  // hWnd
								WM_NULL,                          // Msg
								0,                                // wParam
								0,                                // lParam
								SMTO_BLOCK|SMTO_ABORTIFHUNG,      // fuFlags: block until ready, abort if hung
								uintptr(timeoutMs),               // uTimeout
								uintptr(unsafe.Pointer(&result)), // lpdwResult
							)

							if ret == 0 {
								// SendMessageTimeout failed or timed out - window may not be ready
								bh.logger.Debugw("SendMessageTimeout failed or timed out, window may not be ready", "hwnd", fgHwnd, "title", title)
							}

							// Start goroutine to wait for process completion (if not already terminated)
							handleClosed = true
							if !isLauncher {
								go func() {
									waitForProcess(context.Background(), sei.hProcess, INFINITE*time.Millisecond)
									closeHandle(sei.hProcess)
								}()
							} else {
								// Launcher already terminated, just close handle
								closeHandle(sei.hProcess)
							}
							return nil
						}

						// For non-launchers: try to set foreground window if we have a valid PID
						if !isLauncher && pid != 0 && time.Since(lastSetFocusTime) >= setFocusInterval {
							// Try to find window by PID and set focus
							hwnd := findWindowByPID(pid, step.WaitWnd.Title, bh.logger)
							if hwnd != 0 {
								setWindowFocus(hwnd, bh.logger)
								lastSetFocusTime = time.Now()
							}
						}
					}
				}
			}
		}

		// Start a goroutine to wait for process completion
		// Mark handle as closed so defer doesn't close it
		handleClosed = true
		go func() {
			waitForProcess(context.Background(), sei.hProcess, INFINITE*time.Millisecond)
			// Close handle after process completes
			closeHandle(sei.hProcess)
		}()

		return nil
	}
}

// waitForProcess waits for a process to complete
func waitForProcess(ctx context.Context, hProcess syscall.Handle, timeout time.Duration) error {
	// Convert timeout to milliseconds
	timeoutMs := uint32(timeout.Milliseconds())
	if timeoutMs == 0 {
		timeoutMs = INFINITE
	}

	// Use a channel to handle context cancellation
	done := make(chan error, 1)

	go func() {
		// Wait for process or timeout
		ret, _, _ := procWaitForSingleObject.Call(uintptr(hProcess), uintptr(timeoutMs))
		if ret == 0 {
			// Process completed (WAIT_OBJECT_0 = 0)
			done <- nil
		} else if ret == 0x102 { // WAIT_TIMEOUT
			done <- &ActionError{
				Type:    ErrorTimeout,
				Message: fmt.Sprintf("Process did not complete within %v", timeout),
			}
		} else {
			// Other error codes (WAIT_FAILED = 0xFFFFFFFF, etc.)
			done <- fmt.Errorf("WaitForSingleObject failed with code: 0x%x", ret)
		}
	}()

	select {
	case <-ctx.Done():
		// Context was cancelled, return cancellation error
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// closeHandle closes a Windows handle
func closeHandle(hProcess syscall.Handle) {
	if hProcess != 0 && hProcess != syscall.InvalidHandle {
		procCloseHandle.Call(uintptr(hProcess))
	}
}

// terminateProcess terminates a process
// Note: Does NOT close the handle - caller must close it
func terminateProcess(hProcess syscall.Handle) error {
	ret, _, _ := procTerminateProcess.Call(uintptr(hProcess), 1) // Exit code 1
	if ret == 0 {
		return fmt.Errorf("TerminateProcess failed")
	}
	return nil
}

// terminateProcessHandleImpl terminates a process handle (Windows implementation)
func terminateProcessHandleImpl(hProcess interface{}) error {
	if handle, ok := hProcess.(syscall.Handle); ok {
		return terminateProcess(handle)
	}
	return fmt.Errorf("invalid process handle type: %T", hProcess)
}

// closeProcessHandleImpl closes a process handle (Windows implementation)
func closeProcessHandleImpl(hProcess interface{}) {
	if handle, ok := hProcess.(syscall.Handle); ok {
		closeHandle(handle)
	}
}

// getProcessID gets the process ID from a process handle
func getProcessID(hProcess syscall.Handle) (int, error) {
	ret, _, _ := procGetProcessId.Call(uintptr(hProcess))
	if ret == 0 {
		return 0, fmt.Errorf("GetProcessId failed")
	}
	return int(ret), nil
}

// isProcessTerminated checks if a process has terminated
// Returns true if process has terminated, false if still running
func isProcessTerminated(hProcess syscall.Handle) bool {
	// Use WaitForSingleObject with 0 timeout to check process state without waiting
	// WAIT_OBJECT_0 (0) = process has terminated
	// WAIT_TIMEOUT (0x102) = process is still running
	ret, _, _ := procWaitForSingleObject.Call(uintptr(hProcess), 0)
	return ret == 0 // WAIT_OBJECT_0 means process terminated
}
