package deej

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	// go get github.com/stalexteam/eventsource_go
	// or
	// go get github.com/stalexteam/eventsource_go@058ea8a0213a
	eventsource "github.com/stalexteam/eventsource_go"
	"go.uber.org/zap"
)

const (
	// SSE idle timeout - esphome sends ping every 10 seconds, so 12 seconds timeout is safe
	sseIdleTimeout = 12 * time.Second

	// Delay between reconnection attempts
	sseRetryDelay = 2 * time.Second
)

// SseIO provides a deej-aware abstraction layer to managing Server-Sent Events I/O
type SseIO struct {
	deej   *Deej
	logger *zap.SugaredLogger

	stopChannel chan bool
	connected   int32      // Atomic flag: 1 = connected, 0 = disconnected
	mu          sync.Mutex // Protects ctx, cancel, es, req, currentURL (not connected state)
	connecting  int32      // Atomic flag: 1 = connecting, 0 = not connecting

	req        *http.Request
	es         *eventsource.EventSource
	ctx        context.Context
	cancel     context.CancelFunc
	currentURL string // Stores the URL of the current connection for comparison on config reload
}

// NewSseIO creates an SseIO instance that uses the provided deej instance's connection info
func NewSseIO(deej *Deej, logger *zap.SugaredLogger) (*SseIO, error) {
	logger = logger.Named("sse")

	sio := &SseIO{
		deej:        deej,
		logger:      logger,
		stopChannel: make(chan bool),
		connected:   0,
	}

	logger.Debug("Created SSE i/o instance")

	// Config reload is handled by deej.go setupOnConfigReload()

	return sio, nil
}

// Start attempts to connect to the SSE endpoint
func (sio *SseIO) Start() error {
	sio.mu.Lock()

	if atomic.LoadInt32(&sio.connected) == 1 {
		sio.mu.Unlock()
		return errors.New("sse: already running")
	}

	url := sio.deej.config.ConnectionInfo.SSE_URL
	if strings.TrimSpace(url) == "" {
		sio.mu.Unlock()
		return fmt.Errorf("sse: empty ConnectionInfo.SSE_URL")
	}

	// Release lock before connect() - connect() will acquire it when needed
	sio.mu.Unlock()

	if err := sio.connect(sio.logger); err != nil {
		return fmt.Errorf("sse initial connect error: %w", err)
	}

	go func() {
		for {
			// Only run if we have a valid connection
			// Check connection state atomically
			connected := atomic.LoadInt32(&sio.connected) == 1
			sio.mu.Lock()
			es := sio.es
			sio.mu.Unlock()

			if connected && es != nil {
				err := sio.run(sio.logger)
				if err != nil {
					sio.logger.Warnw("SSE connection lost", "error", err.Error())
				}
			}

			sio.close(sio.logger)

			select {
			case <-sio.stopChannel:
				return
			case <-time.After(sseRetryDelay):
			}

			// Check stopChannel again before attempting reconnect (avoid deadlock)
			select {
			case <-sio.stopChannel:
				return
			default:
			}

			// Check if SSE is still the active interface before checking config
			// If we've switched to another interface, just exit silently
			sio.deej.ioMutex.Lock()
			isActive := sio.deej.io == sio
			sio.deej.ioMutex.Unlock()
			if !isActive {
				sio.logger.Debug("SSE is no longer the active interface, exiting retry loop")
				return
			}

			if sio.deej.config.ConnectionInfo.SSE_URL == "" {
				sio.logger.Info("SSE URL unset in config. Deej will be unable to reconnect. Shutting down.")
				sio.deej.notifier.Notify("SSE URL unset in config", "Shutting down.")
				sio.deej.signalStop()
				return
			}

			// Try to connect, but check if we should stop first
			// Use a channel to signal if connect should abort
			connectDone := make(chan error, 1)
			connectCtx, connectCancel := context.WithCancel(context.Background())
			go func() {
				defer connectCancel() // Ensure context is cancelled when goroutine exits
				// Check stopChannel before attempting connection to avoid goroutine leak
				select {
				case <-sio.stopChannel:
					select {
					case connectDone <- errors.New("connection aborted: stop requested"):
					case <-connectCtx.Done():
						// Context cancelled, don't send
					}
					return
				case <-connectCtx.Done():
					return
				default:
				}
				// Attempt connection
				err := sio.connect(sio.logger)
				// Try to send result, but respect cancellation
				select {
				case connectDone <- err:
				case <-connectCtx.Done():
					// Context cancelled, don't send
				case <-sio.stopChannel:
					// Stop requested, don't send
				}
			}()

			select {
			case <-sio.stopChannel:
				// Stop was called, abort connect attempt
				connectCancel() // Cancel the connect operation
				sio.close(sio.logger)
				return
			case err := <-connectDone:
				connectCancel() // Clean up context
				if err != nil {
					// Don't log "connection aborted" as a warning - it's expected when stopping
					if !strings.Contains(err.Error(), "connection aborted") {
						sio.logger.Warnw("SSE reconnect failed", "error", err.Error())
					}
					continue
				}
			}
		}
	}()

	return nil
}

func (sio *SseIO) connect(logger *zap.SugaredLogger) error {
	// Check stopChannel before acquiring lock to avoid deadlock
	select {
	case <-sio.stopChannel:
		return errors.New("connection aborted: stop requested")
	default:
	}

	// Check if already connected atomically (no lock needed)
	if atomic.LoadInt32(&sio.connected) == 1 {
		return errors.New("already connected")
	}

	// Prevent multiple simultaneous connection attempts
	if !atomic.CompareAndSwapInt32(&sio.connecting, 0, 1) {
		return errors.New("connection attempt already in progress")
	}
	defer atomic.StoreInt32(&sio.connecting, 0)

	url := sio.deej.config.ConnectionInfo.SSE_URL
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("sse: empty ConnectionInfo.SSE_URL")
	}

	// Create context that can be cancelled by Stop()
	sio.mu.Lock()
	if sio.cancel != nil {
		sio.cancel() // Cancel previous context if any
	}
	sio.ctx, sio.cancel = context.WithCancel(context.Background())
	sio.mu.Unlock()

	// Check stopChannel again after creating context
	select {
	case <-sio.stopChannel:
		sio.mu.Lock()
		if sio.cancel != nil {
			sio.cancel()
		}
		sio.mu.Unlock()
		return errors.New("connection aborted: stop requested")
	default:
	}

	req, err := http.NewRequestWithContext(sio.ctx, http.MethodGet, url, nil)
	if err != nil {
		sio.mu.Lock()
		if sio.cancel != nil {
			sio.cancel()
		}
		sio.mu.Unlock()
		return fmt.Errorf("create HTTP request: %w", err)
	}

	// Create eventsource under lock to avoid race conditions
	sio.mu.Lock()
	sio.req = req
	es := eventsource.New(sio.req)
	es.SetIdleTimeout(sseIdleTimeout)

	// Callbacks
	es.OnConnect = func(url string) {
		logger.Infow("Connected to SSE", "url", url)
	}

	es.OnDisconnect = func(url string, err error) {
		if err != nil {
			logger.Infow("Device disconnected", "url", url, "error", err.Error())
		} else {
			logger.Infow("Device disconnected gracefully", "url", url)
		}
	}

	es.OnError = func(url string, err error) {
		logger.Infow("Device seems offline or not responding", "url", url, "error", err.Error())
	}

	sio.es = es
	sio.mu.Unlock()

	logger.Debugw("Attempting SSE connection", "url", url)

	// Try to read first event to verify connection (without holding lock - Read() can block)
	// Use defer recover to handle panics from the library
	var ev eventsource.Event
	var readErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				readErr = fmt.Errorf("panic during initial read: %v", r)
			}
		}()
		ev, readErr = es.Read()
	}()

	// ErrEmptyLine is not a real error - it's a normal part of SSE protocol (keep-alive pings)
	// Only treat actual errors as failures
	if readErr != nil && !errors.Is(readErr, eventsource.ErrEmptyLine) {
		// Clean up on error
		sio.mu.Lock()
		if sio.cancel != nil {
			sio.cancel()
		}
		if sio.es != nil {
			// Close safely - es might be nil if already closed
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Debugw("Error closing eventsource during cleanup", "panic", r)
					}
				}()
				sio.es.Close()
			}()
			sio.es = nil
		}
		sio.mu.Unlock()

		// Use IsConnectionError for better error classification
		if eventsource.IsConnectionError(readErr) {
			return fmt.Errorf("SSE connection error: %w", readErr)
		}
		return fmt.Errorf("SSE read error: %w", readErr)
	}

	if readErr == nil && ev.Type == "state" {
		sio.deej.handleStateEvent(logger, ev.Data)
	}

	// Mark as connected atomically and save URL
	atomic.StoreInt32(&sio.connected, 1)
	sio.mu.Lock()
	sio.currentURL = url
	sio.mu.Unlock()
	logger.Infow("Connected to SSE endpoint", "url", url)

	return nil
}

// WaitForStop waits for the connection to be fully stopped (for use during interface switching)
func (sio *SseIO) WaitForStop(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&sio.connected) == 0 {
			return true
		}
		// Use shorter sleep to avoid holding lock too long
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func (sio *SseIO) run(logger *zap.SugaredLogger) error {
	// Get es and ctx safely under lock
	sio.mu.Lock()
	es := sio.es
	ctx := sio.ctx
	sio.mu.Unlock()

	if es == nil {
		return errors.New("cannot run: event source is nil")
	}

	eventLogger := logger.Named("eventstream")
	eventLogger.Debugw("Starting SSE read loop")

	for {
		select {
		case <-sio.stopChannel:
			return nil
		case <-ctx.Done():
			return errors.New("sse connection closed: context cancelled")
		default:
			// Re-check es under lock before reading (it might have been closed)
			sio.mu.Lock()
			es = sio.es
			sio.mu.Unlock()

			if es == nil {
				return errors.New("event source was closed")
			}

			// Check context before starting Read() to avoid unnecessary goroutine
			if ctx.Err() != nil {
				return fmt.Errorf("sse connection closed: %w", ctx.Err())
			}

			// Read event (library now handles long lines correctly with ReadBytes)
			// Use a channel to make Read() cancellable
			readDone := make(chan struct {
				ev  eventsource.Event
				err error
			}, 1)

			// Start read operation in a goroutine that can be cancelled
			readCtx, readCancel := context.WithCancel(ctx)
			go func() {
				defer readCancel() // Ensure context is cancelled when goroutine exits
				defer func() {
					if r := recover(); r != nil {
						// Handle panic from closed eventsource
						select {
						case readDone <- struct {
							ev  eventsource.Event
							err error
						}{eventsource.Event{}, fmt.Errorf("panic during read: %v", r)}:
						case <-readCtx.Done():
							// Context cancelled, don't send result
						}
					}
				}()

				// Check if we should abort before blocking read
				select {
				case <-readCtx.Done():
					select {
					case readDone <- struct {
						ev  eventsource.Event
						err error
					}{eventsource.Event{}, context.Canceled}:
					case <-sio.stopChannel:
						// Stop requested, don't send
					}
					return
				default:
				}

				ev, err := es.Read()
				// Try to send result, but respect cancellation
				select {
				case readDone <- struct {
					ev  eventsource.Event
					err error
				}{ev, err}:
				case <-readCtx.Done():
					// Context cancelled, don't send result
				case <-sio.stopChannel:
					// Stop requested, don't send result
				}
			}()

			select {
			case <-sio.stopChannel:
				readCancel() // Cancel the read operation
				return nil
			case <-ctx.Done():
				readCancel() // Cancel the read operation
				return errors.New("sse connection closed: context cancelled")
			case result := <-readDone:
				readCancel() // Clean up context
				ev, err := result.ev, result.err
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return errors.New("sse connection closed")
					}
					// Check if es was closed while reading
					sio.mu.Lock()
					esClosed := sio.es == nil
					sio.mu.Unlock()
					if esClosed {
						return errors.New("event source was closed during read")
					}
					// ErrEmptyLine is not a real error - it's a normal part of SSE protocol (keep-alive pings)
					// Only treat actual errors as failures
					if !errors.Is(err, eventsource.ErrEmptyLine) {
						connected := atomic.LoadInt32(&sio.connected) == 1
						if connected {
							// Use IsConnectionError for better error classification
							if eventsource.IsConnectionError(err) {
								eventLogger.Debugw("SSE connection error", "error", err)
							} else {
								eventLogger.Debugw("SSE read error", "error", err)
							}
						}
						// Connection lost, return to trigger reconnect
						if eventsource.IsConnectionError(err) {
							return fmt.Errorf("sse connection error: %w", err)
						}
						return fmt.Errorf("sse read error: %w", err)
					}
					// Empty line is not an error, continue reading
					continue
				}

				// Event is valid if err == nil, process it
				if ev.Type != "state" {
					if sio.deej.Verbose() {
						eventLogger.Debugw("Non-state event received", "type", ev.Type, "id", ev.ID)
					}
					continue
				}

				sio.deej.handleStateEvent(eventLogger, ev.Data)
			}
		}
	}
}

// Stop signals us to shut down our SSE connection, if one is active
func (sio *SseIO) Stop() {
	// Send stop signal first (non-blocking to avoid deadlock)
	select {
	case sio.stopChannel <- true:
	default:
		// Channel already has a signal, that's fine
	}

	// Cancel context to abort any ongoing connect() operations
	sio.mu.Lock()
	if sio.cancel != nil {
		sio.cancel()
	}
	sio.mu.Unlock()

	connected := atomic.LoadInt32(&sio.connected) == 1
	if connected {
		sio.logger.Debug("Shutting down SSE connection")
	} else {
		sio.logger.Debug("Not currently connected, nothing to stop")
	}
}

func (sio *SseIO) close(logger *zap.SugaredLogger) {
	sio.mu.Lock()
	defer sio.mu.Unlock()

	// cancel context to abort Read()
	if sio.cancel != nil {
		sio.cancel()
		sio.cancel = nil // Explicitly nil to prevent double cancellation
	}

	url := sio.currentURL
	if sio.es != nil {
		// Safe close with panic recovery
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Debugw("Error closing eventsource", "panic", r)
				}
			}()
			sio.es.Close()
		}()
		logger.Infow("SSE connection closed", "url", url)
		sio.es = nil
	}
	sio.req = nil // Explicitly nil the request
	sio.currentURL = ""
	atomic.StoreInt32(&sio.connected, 0)
}

// SubscribeToSliderMoveEvents returns an unbuffered channel that receives a SliderMoveEvent every time a slider moves
func (sio *SseIO) SubscribeToSliderMoveEvents() chan SliderMoveEvent {
	return sio.deej.SubscribeToSliderMoveEvents()
}

// SubscribeToSwitchEvents returns an unbuffered channel that receives a SwitchEvent every time a switch changes
func (sio *SseIO) SubscribeToSwitchEvents() chan SwitchEvent {
	return sio.deej.SubscribeToSwitchEvents()
}
