package deej

import (
	"os"

	"github.com/getlantern/systray"

	"github.com/stalexteam/deej_esp32/pkg/deej/icon"
	"github.com/stalexteam/deej_esp32/pkg/deej/util"
)

func (d *Deej) initializeTray(onDone func()) {
	logger := d.logger.Named("tray")

	onReady := func() {
		logger.Debug("Tray instance ready")

		systray.SetTemplateIcon(icon.DeejLogo, icon.DeejLogo)
		systray.SetTitle("Deej ESP32")
		systray.SetTooltip("Deej ESP32")

		editConfig := systray.AddMenuItem("Edit configuration", "Open config file with notepad")
		editConfig.SetIcon(icon.EditConfig)

		refreshSessions := systray.AddMenuItem("Re-scan audio sessions", "Manually refresh audio sessions if something's stuck")
		refreshSessions.SetIcon(icon.RefreshSessions)

		// Only enable stack trace dump in verbose/debug mode
		var dumpStack *systray.MenuItem
		if d.verbose {
			dumpStack = systray.AddMenuItem("Dump stack trace", "Output all goroutines stack trace to log (for debugging deadlocks)")
			dumpStack.SetIcon(icon.RefreshSessions) // Reuse icon, or we can add a new one later
		}

		if d.version != "" {
			systray.AddSeparator()
			versionInfo := systray.AddMenuItem(d.version, "")
			versionInfo.Disable()
		}

		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "Stop deej and quit")

		// wait on things to happen
		go func() {
			for {
				select {

				// quit
				case <-quit.ClickedCh:
					logger.Info("Quit menu item clicked, stopping")

					d.signalStop()

				// edit config
				case <-editConfig.ClickedCh:
					logger.Info("Edit config menu item clicked, opening config for editing")

					editor := "notepad.exe"
					if util.Linux() {
						// Try $EDITOR first, then fallback to common editors
						if editorEnv := os.Getenv("EDITOR"); editorEnv != "" {
							editor = editorEnv
						} else {
							// Try common Linux editors in order of preference
							editor = "xdg-open"
							// xdg-open will open with default text editor
						}
					}

					if err := util.OpenExternal(logger, editor, userConfigFilepath); err != nil {
						logger.Warnw("Failed to open config file for editing", "error", err)
					}

				// refresh sessions
				case <-refreshSessions.ClickedCh:
					logger.Info("Refresh sessions menu item clicked, triggering session map refresh")

					// performance: the reason that forcing a refresh here is okay is that users can't spam the
					// right-click -> select-this-option sequence at a rate that's meaningful to performance
					d.sessions.refreshSessions(true)
				}
			}
		}()

		// dump stack trace handler (only in verbose/debug mode)
		if d.verbose && dumpStack != nil {
			go func() {
				for {
					<-dumpStack.ClickedCh
					logger.Info("Dump stack trace menu item clicked, outputting all goroutines stack trace")
					util.DumpAllGoroutines(logger)
				}
			}()
		}

		// actually start the main runtime
		onDone()
	}

	onExit := func() {
		logger.Debug("Tray exited")
	}

	// start the tray icon
	logger.Debug("Running in tray")
	systray.Run(onReady, onExit)
}

func (d *Deej) stopTray() {
	d.logger.Debug("Quitting tray")
	systray.Quit()
}
