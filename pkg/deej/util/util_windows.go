package util

import (
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/win"
	"github.com/mitchellh/go-ps"
)

var (
	modkernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procQueryFullProcessImageNameW = modkernel32.NewProc("QueryFullProcessImageNameW")
)

const (
	getCurrentWindowInternalCooldown = time.Millisecond * 350

	// ERROR_ACCESS_DENIED is returned when a process (e.g. protected by anti-cheat) denies handle access
	errorAccessDenied = uintptr(5)
)

var (
	lastGetCurrentWindowResult []string
	lastGetCurrentWindowCall   = time.Now()
)

// processPathCache is a thread-safe PID -> path cache.
// It avoids repeated OpenProcess/QueryFullProcessImageNameW calls on every session refresh,
// which is especially important when running alongside games with anti-cheat protection.
type processPathCache struct {
	mu    sync.RWMutex
	paths map[int]string
}

func newProcessPathCache() *processPathCache {
	return &processPathCache{
		paths: make(map[int]string),
	}
}

// GlobalProcessPathCache is the package-level singleton used by Windows session code.
var GlobalProcessPathCache = newProcessPathCache()

// GetCached returns the cached path for a PID, and a bool indicating whether it was found.
func (c *processPathCache) GetCached(pid int) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	path, ok := c.paths[pid]
	return path, ok
}

// Set stores a path for a PID in the cache.
func (c *processPathCache) Set(pid int, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paths[pid] = path
}

// EvictStale removes cached entries whose PIDs are not present in activePIDs.
// Returns the number of evicted entries so the caller can log it.
func (c *processPathCache) EvictStale(activePIDs []int) int {
	active := make(map[int]struct{}, len(activePIDs))
	for _, pid := range activePIDs {
		active[pid] = struct{}{}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	evicted := 0
	for pid := range c.paths {
		if _, ok := active[pid]; !ok {
			delete(c.paths, pid)
			evicted++
		}
	}
	return evicted
}

// Size returns the number of entries currently in the cache (used for logging/diagnostics).
func (c *processPathCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.paths)
}

func getCurrentWindowProcessNames() ([]string, error) {

	// apply an internal cooldown on this function to avoid calling windows API functions too frequently.
	// return a cached value during that cooldown
	now := time.Now()
	if lastGetCurrentWindowCall.Add(getCurrentWindowInternalCooldown).After(now) {
		return lastGetCurrentWindowResult, nil
	}

	lastGetCurrentWindowCall = now

	// the logic of this implementation is a bit convoluted because of the way UWP apps
	// (also known as "modern win 10 apps" or "microsoft store apps") work.
	// these are rendered in a parent container by the name of ApplicationFrameHost.exe.
	// when windows's GetForegroundWindow is called, it returns the window owned by that parent process.
	// so whenever we get that, we need to go and look through its child windows until we find one with a different PID.
	// this behavior is most common with UWP, but it actually applies to any "container" process:
	// an acceptable approach is to return a slice of possible process names that could be the "right" one, looking
	// them up is fairly cheap and covers the most bases for apps that hide their audio-playing inside another process
	// (like steam, and the league client, and any UWP app)

	result := []string{}

	// a callback that will be called for each child window of the foreground window, if it has any
	enumChildWindowsCallback := func(childHWND *uintptr, lParam *uintptr) uintptr {

		// cast the outer lp into something we can work with (maybe closures are good enough?)
		ownerPID := (*uint32)(unsafe.Pointer(lParam))

		// get the child window's real PID
		var childPID uint32
		win.GetWindowThreadProcessId((win.HWND)(unsafe.Pointer(childHWND)), &childPID)

		// compare it to the parent's - if they're different, add the child window's process to our list of process names
		if childPID != *ownerPID {

			// warning: this can silently fail, needs to be tested more thoroughly and possibly reverted in the future
			actualProcess, err := ps.FindProcess(int(childPID))
			if err == nil {
				result = append(result, actualProcess.Executable())
			}
		}

		// indicates to the system to keep iterating
		return 1
	}

	// get the current foreground window
	hwnd := win.GetForegroundWindow()
	var ownerPID uint32

	// get its PID and put it in our window info struct
	win.GetWindowThreadProcessId(hwnd, &ownerPID)

	// check for system PID (0)
	if ownerPID == 0 {
		return nil, nil
	}

	// find the process name corresponding to the parent PID
	process, err := ps.FindProcess(int(ownerPID))
	if err != nil {
		return nil, fmt.Errorf("get parent process for pid %d: %w", ownerPID, err)
	}

	// add it to our result slice
	result = append(result, process.Executable())

	// iterate its child windows, adding their names too
	win.EnumChildWindows(hwnd, syscall.NewCallback(enumChildWindowsCallback), (uintptr)(unsafe.Pointer(&ownerPID)))

	// cache & return whichever executable names we ended up with
	lastGetCurrentWindowResult = result
	return result, nil
}

// IsAccessDeniedError returns true if the error is a Windows ERROR_ACCESS_DENIED (code 5).
// This typically means a process is protected by anti-cheat software or elevated privileges.
func IsAccessDeniedError(err error) bool {
	if errno, ok := err.(syscall.Errno); ok {
		return uintptr(errno) == errorAccessDenied
	}
	return false
}

// GetProcessPath returns the full Win32 path to the executable for the given PID.
// It first tries with PROCESS_QUERY_INFORMATION, then falls back to
// PROCESS_QUERY_LIMITED_INFORMATION (0x1000) which works for most protected processes.
// Uses PROCESS_NAME_WIN32 (flag=1) format, which returns a standard C:\... path
// and requires fewer privileges than PROCESS_NAME_NATIVE (flag=0).
func GetProcessPath(pid int) (string, error) {
	// try full query rights first
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		// fall back to limited query rights — works for most processes including many protected ones
		var limitedErr error
		handle, limitedErr = syscall.OpenProcess(0x1000 /* PROCESS_QUERY_LIMITED_INFORMATION */, false, uint32(pid))
		if limitedErr != nil {
			// wrap the original (more descriptive) error, mention the fallback failed too
			return "", fmt.Errorf("open process (pid %d): %w", pid, err)
		}
	}
	defer syscall.CloseHandle(handle)

	buf := make([]uint16, win.MAX_PATH)
	size := uint32(len(buf))

	// PROCESS_NAME_WIN32 (flag=1) returns a standard C:\... path.
	// PROCESS_NAME_NATIVE (flag=0) returns \Device\HarddiskVolume3\... and needs higher privileges.
	ret, _, lastErr := procQueryFullProcessImageNameW.Call(
		uintptr(handle),
		1, // PROCESS_NAME_WIN32
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)

	if ret == 0 {
		return "", fmt.Errorf("QueryFullProcessImageNameW (pid %d): %w", pid, lastErr)
	}

	return syscall.UTF16ToString(buf[:size]), nil
}
