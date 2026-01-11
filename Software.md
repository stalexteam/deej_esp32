# 💻 Software Documentation

This document describes the Deej software application (Go-based desktop application).

---

## Overview

The Deej software is a **Go-based desktop application** that runs on Windows and Linux (mainly, designed for windows). It connects to the ESP32 mixer via UART (Serial) or Wi-Fi (SSE) and controls system audio volume based on slider positions. (also can be connected to other deej instances)

---

## Getting Started

### Requirements

* **config.yaml** must be in the same directory as the deej executable
* **Windows**: No additional requirements
* **Linux**: 
  * **PulseAudio** must be running for audio session management
  * For building from source: `libgtk-3-dev`, `libappindicator3-dev`, `libwebkit2gtk-4.0-dev`

### Installation Options

#### Option 1: Pre-built Binaries (**Windows** only)

Download pre-built binaries from [GitHub Releases](https://github.com/your-repo/releases). 

#### Option 2: Build from Source

**It's recommended to use checkout to "TAGGED" commit.**

See [pkg/deej/scripts/README.md](pkg/deej/scripts/README.md) for automated build scripts description.

**Quick start**:
* **Windows**: Run `pkg/deej/scripts/windows/build-release.ps1`
* **Linux**: Run `pkg/deej/scripts/linux/build-release.sh`

---

## Configuration

### Configuration File

The software reads configuration from `config.yaml` in the same directory as the executable.

**Reference configuration**: Complete configuration examples are provided in the release packages or look for [pkg/deej/scripts/misc/default-config.yaml](pkg/deej/scripts/misc/default-config.yaml) in the repository.

---

## Features

### Automatic Reconnection

If the connection is lost (UART or SSE), deej automatically attempts to reconnect every 2 seconds.

### Hot-Reload Configuration

The `config.yaml` file is automatically watched for changes. When you save the file:
* Configuration is reloaded automatically
* System notification confirms reload
* Running button actions can be cancelled (if `cancel_on_reload: true`)

### System Tray Icon

Deej runs in the system tray (Windows/Linux). Right-click the tray icon to:
* **Edit configuration** - Opens `config.yaml` in your default text editor
* **Re-scan audio sessions** - Useful if new applications are not detected
* **View version information**
* **Quit deej**

### Logging

All logs are saved to `logs/deej-latest-run.log` for troubleshooting.

**Useful log information**:
* **Audio devices list**: At startup, deej logs all available audio input/output devices (Windows only)
* **Connection status**: UART/SSE connection events
* **Button actions**: Execution status and errors
* **Configuration errors**: Validation failures

---

## Platform Differences

### Windows-Only Features

* `deej.current` - Control the currently active window/app
* `system` - Control system sounds volume
* Device targeting by full name (e.g., "Speakers (Realtek High Definition Audio)")
* `wait_wnd` option for button actions (wait for window to appear)

### Linux-Specific

* Uses **PulseAudio** for audio session management
* Process names matched by binary name (e.g., `chrome` instead of `chrome.exe`)
* Requires `xdotool` for keystroke/typing actions: `sudo apt-get install xdotool`
* System tray requires GTK libraries

---

## Command-Line Options

* `--verbose` or `-v`: Enable verbose logging (useful for debugging connection issues)

### Environment Variables

* `DEEJ_NO_TRAY_ICON=1`: Run without a tray icon (useful for headless setups or scripts)

---

## Usage Scenarios (Transport layer config)

### Single PC Setup (Wired)

**Configuration**: Use Serial UART connection
```yaml
SERIAL_Port: COM18
SERIAL_BaudRate: 115200
```

### Multiple PCs (ESP32 utilizes wifi)

**Configuration**: Use Wi-Fi SSE connection
```yaml
SSE_URL: http://mix.local/events
```

### Hybrid Setup (Wired + ESP32 utilizes wifi)

ESP32 connected to one PC via UART, other PCs via Wi-Fi:
* Primary PC: Serial UART
* Other PCs: SSE URL pointing to ESP32

### Relay Mode (Deej configured with SSE_RELAY_PORT)

One deej instance acts as relay:
* Relay host: `SSE_RELAY_PORT: 8080`
* Clients: `SSE_URL: http://relay-host-ip:8080/events`

---

## Troubleshooting

### Application Not Detected

* **Re-scan audio sessions** from tray menu
* Check if application is actually playing audio
* Verify process name matches exactly (case-sensitive on Linux)
* Restart deej and check logs for audio devices / applications

### Slider Not Working

* Verify slider mapping in `config.yaml`
* Check ESP32 connection (UART or SSE)
* Enable verbose logging: `deej.exe --verbose`
* Check logs for connection errors

### Button Actions Not Working

* Verify button configuration in `config.yaml`
* Check if action is exclusive and previous action still running
* Enable verbose logging to see button press events
* Check logs for action execution errors
* **Linux**: Verify `xdotool` is installed for keystroke/typing actions

### Connection Issues

* **UART**: Check COM port and baud rate
* **SSE**: Verify ESP32 is on same network and URL is correct
* Check firewall settings (SSE requires network access)
* Enable verbose logging to see connection attempts

### Permission Errors

* **Windows**: Run as administrator if needed
* **Linux**: Check PulseAudio permissions
* **Linux**: May need to add user to `audio` group

---

## Advanced Features

### SSE Relay Server

When `SSE_RELAY_PORT` is configured, deej acts as an SSE server, proxying ESP32 data to multiple clients. Useful for:
* Exposing ESP32 data over network
* Multiple clients can access same mixer w/o ESP32 wifi utilization (by ethernet)

### Slider Override

Use `slider_override` to set constant volume levels, bypassing ESP32 slider values. Useful for:
* Testing specific volume levels
* Locking volume for certain sliders
* Hot-swapping configs without physical slider movement

---

## Related Documentation

* [Firmware.md](Firmware.md) - ESP32 firmware configuration
* [Hardware.md](Hardware.md) - Hardware components and assembly
* [pkg/deej/scripts/README.md](pkg/deej/scripts/README.md) - Build instructions
* Configuration examples are included in release packages
