// Package deej provides a machine-side client that pairs with an Arduino
// chip to form a tactile, physical volume control system/
package deej

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/stalexteam/deej_esp32/pkg/deej/util"
)

const (

	// when this is set to anything, deej won't use a tray icon
	envNoTray = "DEEJ_NO_TRAY_ICON"

	// Delay between stopping old interface and starting new one during config reload
	configReloadStopDelay = 50 * time.Millisecond

	// Timeout for waiting for interface to stop during switching
	interfaceStopTimeout = 500 * time.Millisecond
)

// IOInterface defines the common interface for all I/O implementations (Serial, SSE, etc.)
type IOInterface interface {
	Start() error
	Stop()
	WaitForStop(timeout time.Duration) bool // Wait for connection to be fully stopped (optional, returns false if not implemented)
	SubscribeToSliderMoveEvents() chan SliderMoveEvent
	SubscribeToSwitchEvents() chan SwitchEvent
}

var (
	potPattern = regexp.MustCompile(`^sensor-pot(\d+)$`)
	swPattern  = regexp.MustCompile(`^binary_sensor-sw(\d+)$`)
)

// Deej is the main entity managing access to all sub-components
type Deej struct {
	logger   *zap.SugaredLogger
	notifier Notifier
	config   *CanonicalConfig
	serial   *SerialIO
	sse      *SseIO
	io       IOInterface // active I/O interface (serial or sse)
	sessions *sessionMap

	stopChannel chan bool
	version     string
	verbose     bool
	stopping    sync.Once // Ensures signalStop is only called once

	// Common event consumers for all I/O implementations
	sliderMoveConsumers []chan SliderMoveEvent
	switchConsumers     []chan SwitchEvent
	consumersMutex      sync.RWMutex // Protects consumers slices

	// Synchronization for I/O operations
	ioMutex sync.Mutex // Protects io field and startIO() calls
}

// NewDeej creates a Deej instance
func NewDeej(logger *zap.SugaredLogger, verbose bool) (*Deej, error) {
	logger = logger.Named("deej")

	notifier, err := NewToastNotifier(logger)
	if err != nil {
		logger.Errorw("Failed to create ToastNotifier", "error", err)
		return nil, fmt.Errorf("create new ToastNotifier: %w", err)
	}

	config, err := NewConfig(logger, notifier)
	if err != nil {
		logger.Errorw("Failed to create Config", "error", err)
		return nil, fmt.Errorf("create new Config: %w", err)
	}

	d := &Deej{
		logger:              logger,
		notifier:            notifier,
		config:              config,
		stopChannel:         make(chan bool),
		verbose:             verbose,
		sliderMoveConsumers: []chan SliderMoveEvent{},
		switchConsumers:     []chan SwitchEvent{},
	}

	serial, err := NewSerialIO(d, logger)
	if err != nil {
		logger.Errorw("Failed to create SerialIO", "error", err)
		return nil, fmt.Errorf("create new SerialIO: %w", err)
	}
	d.serial = serial

	// Initialize SSE-based I/O (replacement for SerialIO)
	sse, err := NewSseIO(d, logger)
	if err != nil {
		logger.Errorw("Failed to create SseIO", "error", err)
		return nil, fmt.Errorf("create new SseIO: %w", err)
	}
	d.sse = sse

	sessionFinder, err := newSessionFinder(logger)
	if err != nil {
		logger.Errorw("Failed to create SessionFinder", "error", err)
		return nil, fmt.Errorf("create new SessionFinder: %w", err)
	}

	sessions, err := newSessionMap(d, logger, sessionFinder)
	if err != nil {
		logger.Errorw("Failed to create sessionMap", "error", err)
		return nil, fmt.Errorf("create new sessionMap: %w", err)
	}

	d.sessions = sessions

	logger.Debug("Created deej instance")

	return d, nil
}

// Initialize sets up components and starts to run in the background
func (d *Deej) Initialize() error {
	d.logger.Debug("Initializing")

	// load the config for the first time
	if err := d.config.Load(); err != nil {
		d.logger.Errorw("Failed to load config during initialization", "error", err)
		return fmt.Errorf("load config during init: %w", err)
	}

	// initialize the session map
	if err := d.sessions.initialize(); err != nil {
		d.logger.Errorw("Failed to initialize session map", "error", err)
		return fmt.Errorf("init session map: %w", err)
	}

	// decide whether to run with/without tray
	if _, noTraySet := os.LookupEnv(envNoTray); noTraySet {

		d.logger.Debugw("Running without tray icon", "reason", "envvar set")

		// run in main thread while waiting on ctrl+C
		d.setupInterruptHandler()
		d.run()

	} else {
		d.setupInterruptHandler()
		d.initializeTray(d.run)
	}

	return nil
}

// SetVersion causes deej to add a version string to its tray menu if called before Initialize
func (d *Deej) SetVersion(version string) {
	d.version = version
}

// Verbose returns a boolean indicating whether deej is running in verbose mode
func (d *Deej) Verbose() bool {
	return d.verbose
}

func (d *Deej) setupInterruptHandler() {
	interruptChannel := util.SetupCloseHandler()

	go func() {
		signal := <-interruptChannel
		d.logger.Debugw("Interrupted", "signal", signal)
		d.signalStop()
	}()
}

func (d *Deej) run() {
	d.logger.Info("Run loop starting")

	// watch the config file for changes
	go d.config.WatchConfigFileChanges()

	// setup config reload handler to switch between serial and SSE if needed
	d.setupOnConfigReload()

	// connect to the SERIAL/SSE endpoint for the first time
	go d.startIO()

	// wait until stopped (gracefully)
	<-d.stopChannel
	d.logger.Debug("Stop channel signaled, terminating")

	if err := d.stop(); err != nil {
		d.logger.Warnw("Failed to stop deej", "error", err)
		os.Exit(1)
	} else {
		// exit with 0
		os.Exit(0)
	}
}

func (d *Deej) signalStop() {
	d.stopping.Do(func() {
		d.logger.Debug("Signalling stop channel")
		select {
		case d.stopChannel <- true:
		default:
			// Channel already has a signal, ignore
		}
	})
}

func (d *Deej) stop() error {
	d.logger.Info("Stopping")

	d.config.StopWatchingConfigFile()

	// Stop I/O interface and wait for it to fully stop
	if d.io != nil {
		d.io.Stop()
		// Wait for interface to fully stop with timeout
		if d.io.WaitForStop(interfaceStopTimeout) {
			d.logger.Debug("I/O interface stopped successfully")
		} else {
			d.logger.Warn("I/O interface did not stop within timeout, proceeding anyway")
		}
	}

	// Close all event channels to signal goroutines to exit
	d.closeEventChannels()

	// release the session map
	if err := d.sessions.release(); err != nil {
		d.logger.Errorw("Failed to release session map", "error", err)
		return fmt.Errorf("release session map: %w", err)
	}

	d.stopTray()

	// attempt to sync on exit - this won't necessarily work but can't harm
	d.logger.Sync()

	return nil
}

// closeEventChannels closes all event channels to signal goroutines to exit
func (d *Deej) closeEventChannels() {
	d.consumersMutex.Lock()
	defer d.consumersMutex.Unlock()

	// Close all slider move event channels
	for _, ch := range d.sliderMoveConsumers {
		close(ch)
	}
	d.sliderMoveConsumers = nil

	// Close all switch event channels
	for _, ch := range d.switchConsumers {
		close(ch)
	}
	d.switchConsumers = nil

	d.logger.Debug("Closed all event channels")
}

// handleStateEvent processes state events from I/O interfaces (SSE or Serial)
// It extracts id and value from JSON data and dispatches appropriate events
func (d *Deej) handleStateEvent(logger *zap.SugaredLogger, data []byte) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		if d.Verbose() {
			logger.Debugw("Failed to parse JSON event", "error", err, "data", string(data))
		}
		return
	}

	id, _ := raw["id"].(string)
	if id == "" {
		return
	}

	// ---- POTENTIOMETER
	if m := potPattern.FindStringSubmatch(id); len(m) == 2 {
		var val float64
		var ok bool

		// JSON numbers are always parsed as float64 when using map[string]interface{}
		// This handles both SSE format: {"id":"sensor-pot2","value":81} and Serial: {"id": "sensor-pot2", "value": 73}
		if v, okFloat := raw["value"].(float64); okFloat {
			val = v
			ok = true
		}

		if !ok {
			return
		}

		idx, _ := strconv.Atoi(m[1])
		n := float32(val) / 100.0
		if n < 0 {
			n = 0
		} else if n > 1 {
			n = 1
		}
		if d.config.InvertSliders {
			n = 1 - n
		}

		move := SliderMoveEvent{
			SliderID:     idx,
			PercentValue: n,
		}

		if d.Verbose() {
			logger.Debugw("Slider moved", "event", move)
		}

		d.consumersMutex.RLock()
		consumers := make([]chan SliderMoveEvent, len(d.sliderMoveConsumers))
		copy(consumers, d.sliderMoveConsumers)
		d.consumersMutex.RUnlock()

		for _, c := range consumers {
			// Safely send to channel, handling closed channels
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Channel is closed, ignore
						if d.Verbose() {
							logger.Debugw("Channel closed, skipping event", "recover", r)
						}
					}
				}()
				select {
				case c <- move:
				default:
					// Channel is full, skip
				}
			}()
		}
		return
	}

	// ---- SWITCH
	if m := swPattern.FindStringSubmatch(id); len(m) == 2 {
		var state bool
		if v, ok := raw["value"].(bool); ok {
			state = v
		} else if sStr, ok := raw["state"].(string); ok {
			state = strings.ToUpper(sStr) == "ON"
		} else {
			return
		}

		idx, err := strconv.Atoi(m[1])
		if err != nil {
			if d.Verbose() {
				logger.Debugw("Failed to parse switch index", "error", err, "id", id)
			}
			return
		}

		sw := SwitchEvent{
			SwitchID: idx,
			State:    state,
		}

		if d.Verbose() {
			logger.Debugw("Switch changed", "event", sw)
		}

		d.consumersMutex.RLock()
		consumers := make([]chan SwitchEvent, len(d.switchConsumers))
		copy(consumers, d.switchConsumers)
		d.consumersMutex.RUnlock()

		for _, c := range consumers {
			// Safely send to channel, handling closed channels
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Channel is closed, ignore
						if d.Verbose() {
							logger.Debugw("Channel closed, skipping event", "recover", r)
						}
					}
				}()
				select {
				case c <- sw:
				default:
					// Channel is full, skip
				}
			}()
		}
		return
	}
}

// SubscribeToSliderMoveEvents returns an unbuffered channel that receives a SliderMoveEvent every time a slider moves
func (d *Deej) SubscribeToSliderMoveEvents() chan SliderMoveEvent {
	ch := make(chan SliderMoveEvent)
	d.consumersMutex.Lock()
	d.sliderMoveConsumers = append(d.sliderMoveConsumers, ch)
	d.consumersMutex.Unlock()
	return ch
}

// SubscribeToSwitchEvents returns an unbuffered channel that receives a SwitchEvent every time a switch changes
func (d *Deej) SubscribeToSwitchEvents() chan SwitchEvent {
	ch := make(chan SwitchEvent)
	d.consumersMutex.Lock()
	d.switchConsumers = append(d.switchConsumers, ch)
	d.consumersMutex.Unlock()
	return ch
}

// startIO starts the appropriate I/O interface based on configuration
func (d *Deej) startIO() {
	d.ioMutex.Lock()
	defer d.ioMutex.Unlock()

	serialConfigured := d.config.ConnectionInfo.SERIAL_Port != "" && d.config.ConnectionInfo.SERIAL_BaudRate != 0
	sseConfigured := d.config.ConnectionInfo.SSE_URL != ""

	if !serialConfigured && !sseConfigured {
		d.logger.Warnw("No I/O interface configured", "error", "neither serial nor SSE configured")
		d.notifier.Notify("No I/O interface configured!", "Please set up either a serial port or an SSE URL in the configuration.")
		d.signalStop()
		return
	}

	// Choose I/O interface based on configuration
	if serialConfigured {
		d.io = d.serial
		if err := d.serial.Start(); err != nil {
			d.logger.Warnw("Failed to start first-time serial connection", "error", err)

			if errors.Is(err, os.ErrPermission) { // If the port is busy, that's because something else is connected - notify and quit
				d.logger.Warnw("Serial port seems busy, notifying user and closing", "comPort", d.config.ConnectionInfo.SERIAL_Port)
				d.notifier.Notify(fmt.Sprintf("Can't connect to %s!", d.config.ConnectionInfo.SERIAL_Port), "This serial port is busy, make sure to close any serial monitor or other deej instance.")
				d.signalStop()
				return // no need to try SSE if serial is explicitly configured && busy

			} else if errors.Is(err, os.ErrNotExist) { // also notify if the COM port they gave isn't found, maybe their config is wrong
				if !sseConfigured {
					d.logger.Warnw("Provided COM port seems wrong, notifying user and closing", "comPort", d.config.ConnectionInfo.SERIAL_Port)
					d.notifier.Notify(fmt.Sprintf("Can't connect to %s!", d.config.ConnectionInfo.SERIAL_Port), "This serial port doesn't exist, check your configuration and make sure it's set correctly.")
					d.signalStop()
					return // no need to try SSE if serial is explicitly configured && faulty
				} else {
					d.logger.Warnw("Provided COM port seems wrongly configured; trying SSE transport layer", "comPort", d.config.ConnectionInfo.SERIAL_Port)
					d.notifier.Notify(fmt.Sprintf("Can't connect to %s!", d.config.ConnectionInfo.SERIAL_Port), "Provided COM port seems wrongly configured; trying SSE transport layer")
				}
			}
		} else {
			return // Serial started successfully, no need to try SSE
		}
	}

	if !sseConfigured {
		d.logger.Warnw("SSE URL is empty", "error", "no URL provided in config")
		d.signalStop()
		return
	}

	// Fallback to SSE if serial is not configured or failed to start
	d.io = d.sse
	if err := d.sse.Start(); err != nil {
		d.logger.Warnw("Failed to start first-time SSE connection", "error", err)

		// User-facing hint: URL might be wrong/unreachable
		url := d.config.ConnectionInfo.SSE_URL
		d.notifier.Notify(
			fmt.Sprintf("Can't connect to %s!", url), "Make sure the URL is correct and the ESPHome event stream is reachable.",
		)

		d.signalStop()
	}
}

// setupOnConfigReload handles configuration changes and switches between serial and SSE if needed
func (d *Deej) setupOnConfigReload() {
	configReloadedChannel := d.config.SubscribeToChanges()

	go func() {
		for {
			<-configReloadedChannel

			// Acquire lock to prevent concurrent startIO() calls
			d.ioMutex.Lock()

			// Determine which interface should be active based on new config
			shouldUseSerial := d.config.ConnectionInfo.SERIAL_Port != "" && d.config.ConnectionInfo.SERIAL_BaudRate != 0
			sseConfigured := d.config.ConnectionInfo.SSE_URL != ""
			currentIsSerial := d.io == d.serial

			// Check if we need to switch interfaces or if current transport was removed
			needsSwitch := shouldUseSerial != currentIsSerial
			currentTransportRemoved := (currentIsSerial && !shouldUseSerial) || (!currentIsSerial && !sseConfigured)

			// If we need to switch interfaces or current transport was removed, try to switch
			if needsSwitch || currentTransportRemoved {
				// Check if at least one transport is available
				if !shouldUseSerial && !sseConfigured {
					// Both transports removed - this is the only case where we stop
					d.logger.Warnw("All transport configurations removed, stopping Deej", "wasSerial", currentIsSerial)
					d.notifier.Notify("All transport configurations removed!", "Please configure at least one transport (Serial or SSE) in the configuration.")
					d.ioMutex.Unlock()
					d.signalStop()
					continue
				}

				// At least one transport is available, switch to it
				if currentTransportRemoved {
					d.logger.Info("Detected removal of active transport, switching to available transport")
				} else {
					d.logger.Info("Detected I/O interface change in config, switching interfaces")
				}

				// Release lock before stopping interface and waiting (these operations can take time)
				d.ioMutex.Unlock()

				if d.io != nil {
					d.io.Stop()
					// Wait for interface to fully stop before starting new one
					if d.io.WaitForStop(interfaceStopTimeout) {
						d.logger.Debug("Previous interface stopped successfully")
					} else {
						d.logger.Warn("Previous interface did not stop within timeout, proceeding anyway")
					}
				}
				<-time.After(configReloadStopDelay)
				d.startIO() // startIO will acquire ioMutex
			} else if d.io != nil {
				// Same interface, but check if connection parameters changed
				if currentIsSerial && d.serial != nil && d.serial.IsConnected() {
					// Check if serial connection parameters changed
					d.serial.mu.Lock()
					currentPort := d.serial.connOptions.PortName
					currentBaud := d.serial.connOptions.BaudRate
					d.serial.mu.Unlock()

					if d.config.ConnectionInfo.SERIAL_Port != currentPort ||
						uint(d.config.ConnectionInfo.SERIAL_BaudRate) != currentBaud {
						d.logger.Info("Detected change in serial connection parameters, renewing connection")
						// Release ioMutex before stopping and starting (these operations can take time)
						d.ioMutex.Unlock()
						d.serial.Stop()
						<-time.After(configReloadStopDelay)
						if err := d.serial.Start(); err != nil {
							d.logger.Warnw("Failed to renew serial connection after parameter change", "error", err)
						} else {
							d.logger.Debug("Renewed serial connection successfully")
						}
					} else {
						d.ioMutex.Unlock()
					}
				} else if !currentIsSerial && d.sse != nil {
					// Check if SSE URL changed or if we need to connect
					d.sse.mu.Lock()
					currentSSEURL := d.sse.currentURL
					d.sse.mu.Unlock()
					newSSEURL := d.config.ConnectionInfo.SSE_URL
					isConnected := atomic.LoadInt32(&d.sse.connected) == 1

					if currentSSEURL != newSSEURL {
						if isConnected {
							d.logger.Info("Detected change in SSE URL, renewing connection", "old", currentSSEURL, "new", newSSEURL)
							// Release ioMutex before stopping and starting (these operations can take time)
							d.ioMutex.Unlock()
							d.sse.Stop()
							<-time.After(configReloadStopDelay)
							if err := d.sse.Start(); err != nil {
								d.logger.Warnw("Failed to renew SSE connection after URL change", "error", err)
							} else {
								d.logger.Debug("Renewed SSE connection successfully")
							}
						} else if newSSEURL != "" {
							// Not connected but URL is set, try to connect
							d.logger.Info("SSE not connected but URL configured, attempting connection", "url", newSSEURL)
							// Release ioMutex before starting (this operation can take time)
							d.ioMutex.Unlock()
							<-time.After(configReloadStopDelay)
							if err := d.sse.Start(); err != nil {
								d.logger.Warnw("Failed to start SSE connection", "error", err)
							} else {
								d.logger.Debug("SSE connection started successfully")
							}
						} else {
							d.ioMutex.Unlock()
						}
					} else {
						// URL unchanged
						if !isConnected && newSSEURL != "" {
							// URL unchanged but not connected, try to connect
							d.logger.Info("SSE URL unchanged but not connected, attempting connection", "url", newSSEURL)
							// Release ioMutex before starting (this operation can take time)
							d.ioMutex.Unlock()
							<-time.After(configReloadStopDelay)
							if err := d.sse.Start(); err != nil {
								d.logger.Warnw("Failed to start SSE connection", "error", err)
							} else {
								d.logger.Debug("SSE connection started successfully")
							}
						} else {
							d.logger.Debug("SSE URL unchanged and connection active, skipping reconnection")
							d.ioMutex.Unlock()
						}
					}
				} else {
					d.ioMutex.Unlock()
				}
			} else {
				d.ioMutex.Unlock()
			}
		}
	}()
}
