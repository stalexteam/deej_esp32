package deej

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	eventsource "github.com/stalexteam/eventsource_go"
	"go.uber.org/zap"
)

// SseServer provides an EventSource server that proxies ESP32 data to other deej instances
type SseServer struct {
	deej   *Deej
	logger *zap.SugaredLogger
	server *http.Server

	stopChannel chan bool
	running     int32 // Atomic flag: 1 = running, 0 = stopped

	// ConnectionManager manages all active SSE connections
	manager *eventsource.ConnectionManager

	// Event counter for SSE id field
	eventID int64

	// Current port (for tracking changes)
	currentPort int
	portMutex   sync.Mutex
}

const (
	// SSE retry timeout in milliseconds (as per ESP32 format)
	sseRetryTimeout = 30000

	// Ping interval
	pingInterval = 10 * time.Second
)

// NewSseServer creates a new SSE server instance
func NewSseServer(deej *Deej, logger *zap.SugaredLogger) (*SseServer, error) {
	logger = logger.Named("sse_server")

	manager := eventsource.NewConnectionManager()

	// Set up callbacks for connection events
	manager.SetOnConnect(func(encoder *eventsource.Encoder) {
		logger.Infow("New SSE client connected",
			"remote", encoder.RemoteAddr(),
			"path", encoder.Path())
	})

	manager.SetOnDisconnect(func(encoder *eventsource.Encoder) {
		logger.Debugw("SSE client disconnected",
			"remote", encoder.RemoteAddr(),
			"path", encoder.Path())
	})

	srv := &SseServer{
		deej:        deej,
		logger:      logger,
		stopChannel: make(chan bool),
		manager:     manager,
		eventID:     1,
		currentPort: 0,
	}

	logger.Debug("Created SSE server instance")

	return srv, nil
}

// Start starts the SSE server on the configured port
func (srv *SseServer) Start() error {
	port := srv.deej.config.ConnectionInfo.SSE_RELAY_PORT
	if port <= 0 {
		srv.logger.Debug("SSE_RELAY_PORT not configured, server will not start")
		return nil
	}

	srv.portMutex.Lock()
	currentPort := srv.currentPort
	srv.portMutex.Unlock()

	// If already running on the same port, no need to restart
	if atomic.LoadInt32(&srv.running) == 1 && currentPort == port {
		srv.logger.Debugw("SSE server already running on the same port", "port", port)
		return nil
	}

	// If running on different port, stop first
	if atomic.LoadInt32(&srv.running) == 1 {
		srv.logger.Infow("SSE server port changed, restarting", "old_port", currentPort, "new_port", port)
		srv.Stop()
		// Wait a bit for graceful shutdown
		time.Sleep(100 * time.Millisecond)
	}

	// Create handler using HandlerV2 with ConnectionManager
	handler := eventsource.HandlerV2(func(
		info *eventsource.ConnectionInfo,
		encoder *eventsource.Encoder,
		stop <-chan bool,
	) {
		// Send initial retry timeout (as per ESP32 format)
		if err := encoder.SetRetry(sseRetryTimeout); err != nil {
			if eventsource.IsConnectionError(err) {
				srv.logger.Debugw("Error sending retry, connection closed", "error", err)
			} else {
				srv.logger.Debugw("Error sending retry field", "error", err)
			}
			return
		}

		// Send ping event with metadata (as per ESP32 format: retry, then id, then ping)
		pingID := atomic.AddInt64(&srv.eventID, 1)
		pingData := map[string]interface{}{
			"title":   "Mixer",
			"comment": "",
			"ota":     false,
			"log":     false,
			"lang":    "en",
		}
		pingDataJSON, err := json.Marshal(pingData)
		if err != nil {
			srv.logger.Warnw("Failed to marshal ping data", "error", err)
			return
		}

		pingEvent := eventsource.Event{
			ID:   fmt.Sprintf("%d", pingID),
			Type: "ping",
			Data: pingDataJSON,
		}
		if err := encoder.Encode(pingEvent); err != nil {
			if eventsource.IsConnectionError(err) {
				srv.logger.Debugw("Error sending ping, connection closed", "error", err)
			} else {
				srv.logger.Debugw("Error sending ping event", "error", err)
			}
			return
		}

		// Send all known states to the new client (minimal format: only id and value)
		srv.sendAllStatesToEncoder(encoder)

		// Wait for client disconnect or server stop
		select {
		case <-stop:
			return
		case <-srv.stopChannel:
			return
		}
	})

	// Use HandlerWithManager to automatically manage connections
	handlerWithManager := eventsource.HandlerWithManager(srv.manager, handler)

	mux := http.NewServeMux()
	// Handle any URL path - all paths will serve SSE stream
	mux.HandleFunc("/", handlerWithManager.ServeHTTP)

	addr := fmt.Sprintf(":%d", port)
	srv.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	srv.portMutex.Lock()
	srv.currentPort = port
	srv.portMutex.Unlock()

	atomic.StoreInt32(&srv.running, 1)

	go func() {
		srv.logger.Infow("Starting SSE server", "addr", addr)
		if err := srv.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			srv.logger.Errorw("SSE server error", "error", err)
			atomic.StoreInt32(&srv.running, 0)
		}
	}()

	// Start ping goroutine
	go srv.pingLoop()

	return nil
}

// Stop stops the SSE server
func (srv *SseServer) Stop() {
	if atomic.LoadInt32(&srv.running) == 0 {
		return
	}

	srv.logger.Debug("Stopping SSE server")

	// Signal stop
	select {
	case srv.stopChannel <- true:
	default:
	}

	// Close all connections using ConnectionManager
	if srv.manager != nil {
		srv.manager.CloseAll()
		srv.logger.Debugw("Closed all SSE connections", "count", srv.manager.Count())
	}

	// Stop HTTP server with graceful shutdown
	if srv.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.server.Shutdown(ctx); err != nil {
			srv.logger.Warnw("Error during SSE server shutdown", "error", err)
			srv.server.Close()
		}
	}

	atomic.StoreInt32(&srv.running, 0)

	srv.portMutex.Lock()
	srv.currentPort = 0
	srv.portMutex.Unlock()

	srv.logger.Info("SSE server stopped")
}

// GetCurrentPort returns the current port the server is running on (0 if not running)
func (srv *SseServer) GetCurrentPort() int {
	srv.portMutex.Lock()
	defer srv.portMutex.Unlock()
	return srv.currentPort
}

// IsRunning returns whether the server is currently running
func (srv *SseServer) IsRunning() bool {
	return atomic.LoadInt32(&srv.running) == 1
}

// sendAllStatesToEncoder sends all known states to a client encoder (minimal format: only id and value)
func (srv *SseServer) sendAllStatesToEncoder(encoder *eventsource.Encoder) {
	srv.deej.stateMutex.RLock()
	// Copy sensor states
	sensorStates := make(map[string]map[string]interface{})
	for id, state := range srv.deej.sensorStates {
		stateCopy := make(map[string]interface{})
		for k, v := range state {
			stateCopy[k] = v
		}
		sensorStates[id] = stateCopy
	}
	// Copy switch states
	switchStates := make(map[string]map[string]interface{})
	for id, state := range srv.deej.switchStates {
		stateCopy := make(map[string]interface{})
		for k, v := range state {
			stateCopy[k] = v
		}
		switchStates[id] = stateCopy
	}
	srv.deej.stateMutex.RUnlock()

	// Send all sensor states (minimal format: only id and value)
	for id, state := range sensorStates {
		srv.sendStateToEncoder(encoder, id, state)
	}
	// Send all switch states (minimal format: only id and value)
	for id, state := range switchStates {
		srv.sendStateToEncoder(encoder, id, state)
	}
}

// sendStateToEncoder sends a state event to an encoder
// Uses minimal format: only id and value (as per requirement)
func (srv *SseServer) sendStateToEncoder(encoder *eventsource.Encoder, id string, state map[string]interface{}) {
	eventID := atomic.AddInt64(&srv.eventID, 1)

	// Create minimal format with only id and value (no state field needed)
	minimalState := map[string]interface{}{
		"id": id,
	}

	if value, ok := state["value"]; ok {
		minimalState["value"] = value
	}

	stateJSON, err := json.Marshal(minimalState)
	if err != nil {
		srv.logger.Warnw("Failed to marshal state data", "error", err, "id", id)
		return
	}

	event := eventsource.Event{
		ID:   fmt.Sprintf("%d", eventID),
		Type: "state",
		Data: stateJSON,
	}

	if err := encoder.Encode(event); err != nil {
		if eventsource.IsConnectionError(err) {
			srv.logger.Debugw("Error sending state event, connection closed", "error", err, "id", id)
		} else {
			srv.logger.Debugw("Error sending state event", "error", err, "id", id)
		}
		// ConnectionManager will automatically unregister failed connections
		return
	}
}

// NotifyStateChange notifies all clients about a state change
// Uses minimal format: only id and value (as per requirement)
func (srv *SseServer) NotifyStateChange(id string, state map[string]interface{}) {
	if atomic.LoadInt32(&srv.running) == 0 {
		return
	}

	if srv.manager == nil {
		return
	}

	// Extract value from state
	value, ok := state["value"]
	if !ok {
		// No value field, skip
		return
	}

	// Create minimal format state event (only id and value)
	minimalState := map[string]interface{}{
		"id":    id,
		"value": value,
	}

	stateJSON, err := json.Marshal(minimalState)
	if err != nil {
		srv.logger.Warnw("Failed to marshal state data for broadcast", "error", err, "id", id)
		return
	}

	eventID := atomic.AddInt64(&srv.eventID, 1)
	event := eventsource.Event{
		ID:   fmt.Sprintf("%d", eventID),
		Type: "state",
		Data: stateJSON,
	}

	// Use ConnectionManager.Broadcast to send to all clients
	if err := srv.manager.Broadcast(event); err != nil {
		if eventsource.IsConnectionError(err) {
			srv.logger.Debugw("Some connections failed during broadcast", "error", err)
		}
		// ConnectionManager automatically removes failed connections
	}
}

// pingLoop sends ping events periodically to all clients
func (srv *SseServer) pingLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-srv.stopChannel:
			return
		case <-ticker.C:
			if atomic.LoadInt32(&srv.running) == 0 {
				return
			}

			if srv.manager == nil {
				continue
			}

			pingData := map[string]interface{}{
				"title":   "Mixer",
				"comment": "",
				"ota":     false,
				"log":     false,
				"lang":    "en",
			}

			dataJSON, err := json.Marshal(pingData)
			if err != nil {
				srv.logger.Warnw("Failed to marshal ping data", "error", err)
				continue
			}

			eventID := atomic.AddInt64(&srv.eventID, 1)
			event := eventsource.Event{
				ID:   fmt.Sprintf("%d", eventID),
				Type: "ping",
				Data: dataJSON,
			}

			// Use ConnectionManager.Broadcast to send ping to all clients
			if err := srv.manager.Broadcast(event); err != nil {
				if eventsource.IsConnectionError(err) {
					srv.logger.Debugw("Some connections failed during ping broadcast", "error", err)
				}
				// ConnectionManager automatically removes failed connections
			}
		}
	}
}
