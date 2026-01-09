package deej

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jacobsa/go-serial/serial"
	"go.uber.org/zap"
)

// SerialIO provides a deej-aware abstraction layer to managing serial I/O
type SerialIO struct {
	comPort  string
	baudRate uint

	deej   *Deej
	logger *zap.SugaredLogger

	stopChannel chan bool
	mu          sync.Mutex // Protects connected, conn, and connOptions
	connected   bool
	connOptions serial.OpenOptions
	conn        io.ReadWriteCloser
}

const (
	// Delay between serial reconnection attempts
	serialRetryDelay = 2 * time.Second

	// InterCharacterTimeout for serial connection (milliseconds)
	// This is the timeout between characters before a read operation returns
	serialInterCharacterTimeout = 50
)

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*m`)
var jsonLogRegexp = regexp.MustCompile(`\[[A-Z]\]\[json:\d+\]:\s*(\{.*\})`)

func stripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

// NewSerialIO creates a SerialIO instance that uses the provided deej
// instance's connection info to establish communications with the arduino chip
func NewSerialIO(deej *Deej, logger *zap.SugaredLogger) (*SerialIO, error) {
	logger = logger.Named("serial")

	sio := &SerialIO{
		deej:        deej,
		logger:      logger,
		stopChannel: make(chan bool),
		connected:   false,
		conn:        nil,
	}

	logger.Debug("Created serial i/o instance")

	// respond to config changes
	sio.setupOnConfigReload()

	return sio, nil
}

// IsConnected returns whether the serial connection is currently active
func (sio *SerialIO) IsConnected() bool {
	sio.mu.Lock()
	defer sio.mu.Unlock()
	return sio.connected
}

// Start attempts to connect to our arduino chip
func (sio *SerialIO) Start() error {
	sio.mu.Lock()
	if sio.connected {
		sio.mu.Unlock()
		return errors.New("serial: already running")
	}
	sio.mu.Unlock()

	if err := sio.connect(sio.logger); err != nil {
		return fmt.Errorf("serial initial connect error: %w", err)
	}

	go func() {
		for {
			// Only run if we have a valid connection
			sio.mu.Lock()
			connected := sio.connected
			conn := sio.conn
			sio.mu.Unlock()

			if connected && conn != nil {
				err := sio.run(sio.logger)
				if err != nil {
					sio.logger.Warnw("Serial connection lost", "error", err.Error())
				}
			}

			sio.close(sio.logger)

			select {
			case <-sio.stopChannel:
				return
			case <-time.After(serialRetryDelay):
			}

			// Check if Serial is still the active interface before checking config
			// If we've switched to another interface, just exit silently
			sio.deej.ioMutex.Lock()
			isActive := sio.deej.io == sio
			sio.deej.ioMutex.Unlock()
			if !isActive {
				sio.logger.Debug("Serial is no longer the active interface, exiting retry loop")
				return
			}

			if sio.deej.config.ConnectionInfo.SERIAL_Port == "" || sio.deej.config.ConnectionInfo.SERIAL_BaudRate == 0 {
				sio.logger.Info("Serial port or baud rate unset in config. Deej will be unable to reconnect. Shutting down.")
				sio.deej.notifier.Notify("Serial port or baud rate unset in config", "Shutting down.")
				sio.deej.signalStop()
				return
			}

			if err := sio.connect(sio.logger); err != nil {
				sio.logger.Warnw("Serial reconnect failed", "error", err.Error())
				continue
			}
		}
	}()

	return nil
}

func (sio *SerialIO) connect(logger *zap.SugaredLogger) error {
	sio.mu.Lock()
	if sio.connected {
		sio.mu.Unlock()
		return errors.New("already connected")
	}

	sio.connOptions = serial.OpenOptions{
		PortName:              sio.deej.config.ConnectionInfo.SERIAL_Port,
		BaudRate:              uint(sio.deej.config.ConnectionInfo.SERIAL_BaudRate),
		DataBits:              8,
		StopBits:              1,
		MinimumReadSize:       0,
		InterCharacterTimeout: serialInterCharacterTimeout,
	}
	portName := sio.connOptions.PortName
	sio.mu.Unlock()

	logger.Debugw("Attempting serial connection", "port", portName, "baud", sio.connOptions.BaudRate)

	conn, err := serial.Open(sio.connOptions)
	if err != nil {
		// Provide more detailed error messages for common issues
		errMsg := err.Error()
		if strings.Contains(errMsg, "access is denied") || strings.Contains(errMsg, "permission denied") {
			logger.Errorw("Serial port access denied - port may be in use by another application",
				"port", portName, "error", err)
			return fmt.Errorf("serial port %s is busy or access denied: %w", portName, err)
		}
		if strings.Contains(errMsg, "no such file") || strings.Contains(errMsg, "cannot find") {
			logger.Errorw("Serial port does not exist - check port name in configuration",
				"port", portName, "error", err)
			return fmt.Errorf("serial port %s does not exist: %w", portName, err)
		}
		logger.Errorw("Failed to open serial port", "port", portName, "error", err)
		return fmt.Errorf("open serial port %s: %w", portName, err)
	}

	sio.mu.Lock()
	sio.conn = conn
	sio.connected = true
	sio.mu.Unlock()

	logger.Infow("Connected to serial port", "port", portName)

	return nil
}

func (sio *SerialIO) run(logger *zap.SugaredLogger) error {
	if sio.conn == nil {
		return errors.New("cannot run: connection is nil")
	}
	connReader := bufio.NewReader(sio.conn)
	lineChannel := sio.readLine(logger, connReader)

	for {
		select {
		case <-sio.stopChannel:
			return nil

		case line, ok := <-lineChannel:
			if !ok {
				return errors.New("serial connection lost")
			}
			sio.handleLine(logger, line)
		}
	}
}

// Stop signals us to shut down our serial connection, if one is active
func (sio *SerialIO) Stop() {
	sio.mu.Lock()
	connected := sio.connected
	sio.mu.Unlock()

	if connected {
		sio.logger.Debug("Shutting down serial connection")
		sio.stopChannel <- true
	} else {
		sio.logger.Debug("Not currently connected, nothing to stop")
	}
}

// WaitForStop waits for the connection to be fully stopped (for use during interface switching)
func (sio *SerialIO) WaitForStop(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sio.mu.Lock()
		connected := sio.connected
		sio.mu.Unlock()
		if !connected {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// SubscribeToSliderMoveEvents returns an unbuffered channel that receives
// a sliderMoveEvent struct every time a slider moves
func (sio *SerialIO) SubscribeToSliderMoveEvents() chan SliderMoveEvent {
	return sio.deej.SubscribeToSliderMoveEvents()
}

// SubscribeToSwitchEvents returns an unbuffered channel that receives a SwitchEvent every time a switch changes
func (sio *SerialIO) SubscribeToSwitchEvents() chan SwitchEvent {
	return sio.deej.SubscribeToSwitchEvents()
}

func (sio *SerialIO) setupOnConfigReload() {
	configReloadedChannel := sio.deej.config.SubscribeToChanges()

	go func() {
		for {
			_, ok := <-configReloadedChannel
			if !ok {
				// Channel closed, exit goroutine
				sio.logger.Debug("Config reload channel closed, exiting handler")
				return
			}
			// Connection restart is handled by deej.go setupOnConfigReload()
		}
	}()
}

func (sio *SerialIO) close(logger *zap.SugaredLogger) {
	sio.mu.Lock()
	conn := sio.conn
	portName := ""
	if sio.connOptions.PortName != "" {
		portName = sio.connOptions.PortName
	}
	sio.conn = nil
	sio.connected = false
	sio.mu.Unlock()

	if conn != nil {
		if err := conn.Close(); err != nil {
			logger.Warnw("Failed to close serial connection", "port", portName, "error", err.Error())
		} else {
			logger.Infow("Serial connection closed", "port", portName)
		}
	}
}

func (sio *SerialIO) readLine(logger *zap.SugaredLogger, reader *bufio.Reader) chan string {
	ch := make(chan string)

	go func() {
		defer close(ch) // Ensure channel is closed when goroutine exits
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				// Log read errors at info level for connection issues
				if err != io.EOF {
					logger.Infow("Serial read error, connection may be lost", "error", err)
				} else if sio.deej.Verbose() {
					logger.Debugw("Serial read EOF", "error", err)
				}
				// Channel will be closed, main loop will detect it
				return
			}

			if sio.deej.Verbose() {
				logger.Debugw("Read new line", "line", line)
			}

			// deliver the line to the channel
			select {
			case ch <- line:
			case <-sio.stopChannel:
				// Stop requested, exit
				return
			}
		}
	}()

	return ch
}

func (sio *SerialIO) handleLine(logger *zap.SugaredLogger, line string) {
	// Remove ANSI escape sequences
	clean := stripANSI(line)

	// Trim whitespace and newlines
	trimmed := strings.TrimSpace(clean)

	// Check if line is a pure JSON string (starts with { and ends with })
	if len(trimmed) > 0 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' {
		// Try to parse as pure JSON
		if sio.deej.Verbose() {
			logger.Debugw("Pure JSON line detected", "json", trimmed)
		}
		sio.deej.handleStateEvent(logger, []byte(trimmed))
		return
	}

	// Extract JSON from log tag format
	m := jsonLogRegexp.FindStringSubmatch(clean)
	if m == nil {
		return // Not our format
	}

	jsonPayload := m[1]

	if sio.deej.Verbose() {
		logger.Debugw("JSON payload received from log format", "json", jsonPayload)
	}

	// Use the common handleStateEvent from deej.go
	sio.deej.handleStateEvent(logger, []byte(jsonPayload))
}
