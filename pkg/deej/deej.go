// Package deej provides a machine-side client that pairs with an Arduino
// chip to form a tactile, physical volume control system/
package deej

import (
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/omriharel/deej/pkg/deej/util"
)

const (

	// when this is set to anything, deej won't use a tray icon
	envNoTray = "DEEJ_NO_TRAY_ICON"
)

// Deej is the main entity managing access to all sub-components
type Deej struct {
	logger   *zap.SugaredLogger
	notifier Notifier
	config   *CanonicalConfig
	sse      *SseIO
	sessions *sessionMap

	stopChannel chan bool
	version     string
	verbose     bool
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
		logger:      logger,
		notifier:    notifier,
		config:      config,
		stopChannel: make(chan bool),
		verbose:     verbose,
	}

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

	// connect to the SSE endpoint for the first time
	go func() {
		if err := d.sse.Start(); err != nil {
			d.logger.Warnw("Failed to start first-time SSE connection", "error", err)

			// User-facing hint: URL might be wrong/unreachable
			url := d.config.ConnectionInfo.URL
			d.notifier.Notify(
				fmt.Sprintf("Can't connect to %s!", url),
				"Make sure the URL is correct and the ESPHome event stream is reachable.",
			)

			d.signalStop()
		}
	}()

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
	d.logger.Debug("Signalling stop channel")
	d.stopChannel <- true
}

func (d *Deej) stop() error {
	d.logger.Info("Stopping")

	d.config.StopWatchingConfigFile()
	d.sse.Stop()

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
