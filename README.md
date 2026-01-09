# 🧭 ESPHome Deej Fork

A **variant of [Deej](https://github.com/omriharel/deej)** that uses **[ESPHome](https://esphome.io/) (ESP32)** instead of Arduino, with flexible transport options and mute switch implementation.

---

## 💡 Overview

This fork allows the mixer to communicate over either **wired UART** or **Wi-Fi (SSE)**.

Key improvements:

* **Choice of transport layer**: users can now connect via a USB-UART cable or wirelessly over Wi-Fi.
* Multiple Deej software instances can connect to the same mixer over Wi-Fi simultaneously.
* Easy integration into **Home Assistant** (e.g., dimmer control).

You can select your preferred transport in the configuration. If both are configured, the software will attempt UART first. If UART connection fails (port doesn't exist) and SSE is configured, it will fallback to Wi-Fi SSE. If UART port is busy (already in use), the software will stop and notify you instead of falling back to SSE.

---

## ⚙️ Firmware & Configuration

To build and flash the ESP32, you’ll need **ESPHome**.
There are ready-to-use Docker containers and plenty of guides available online — setting it up shouldn’t be a problem.
It's a bit tricky because you'll need to put a header (mix_tools.h) file to the config folder, but і think - you can handle it.

If you use HA, everything is [extremely simple](https://drive.google.com/file/d/1BMy3CoxtEZqwKd2B-B0Jz64usQFDXO8t/view?usp=sharing) (through the espome plugin).

The ESP32 device configuration (`mix_*.yaml`) and the corresponding header file (`mix_tools.h`)
are located in this repository under:


```
esphome/
```

* mix_momentary.yaml - If you wish to use momentary switches to mute the sound. 
* mix_latching.yaml - for latching switches (preferred)
* **If you don't need the mute button functionality**, use mix_latching.yaml and comment out or remove the "binary_sensor" block from yaml.

There are 2 options: 
* hardcode your wifi credentials into firmware.
* (preferred) use empty wifi and improv_serial sections and https://web.esphome.io/ (after connecting to device use "vertical ..." -> "Configure WiFi") to configure wifi connection settings any time you want w/o reflashing. 

Optional things (uncomment to activate):
* factory reset button (erase wifi/other settings) 
* home assitant integration
* OTA updates
* Captive portal ([docs](https://esphome.io/components/captive_portal/))

---

## 🚀 Running Deej

### Requirements

* **config.yaml** must be in the same directory as the deej executable.
* For UART connection: ensure the ESP32 is connected via USB and the correct COM port is configured.
* For SSE connection: ensure the ESP32 is on the same network and the URL is correct.

**Linux-specific requirements:**
* **PulseAudio**: Deej requires PulseAudio to be running for audio session management.
* **System tray dependencies**: For building from source, you'll need:
  * `libgtk-3-dev`
  * `libappindicator3-dev`
  * `libwebkit2gtk-4.0-dev`
* **Text editor**: The tray menu uses `$EDITOR` environment variable if set, otherwise falls back to `xdg-open` (which uses your default text editor).

**Platform differences:**
* **Windows-only features** (not available on Linux):
  * `deej.current` - control the currently active window/app
  * `system` - control system sounds volume
  * Device targeting by full name (e.g., "Speakers (Realtek High Definition Audio)")
* **Linux**: Uses PulseAudio for audio session management. Process names are matched by binary name (e.g., `chrome` instead of `chrome.exe`).

### Features

* **Automatic reconnection**: If the connection is lost (UART or SSE), deej will automatically attempt to reconnect every 2 seconds.
* **Hot-reload configuration**: The `config.yaml` file is automatically watched for changes. When you save the file, deej will reload the configuration and notify you via a system notification.
* **System tray icon**: Deej runs in the system tray (Windows/Linux). Right-click the tray icon to:
  * Edit configuration (opens config.yaml in your default text editor)
  * Re-scan audio sessions (useful if new applications are not detected)
  * View version information
  * Quit deej
* **Logging**: All logs are saved to `logs/deej-latest-run.log` for troubleshooting.
  * **Audio devices list**: At startup, deej logs all available audio input/output devices. Check the log file to see device names that can be used in `config.yaml` for device targeting (Windows only).

### Command-line Options

* `--verbose` or `-v`: Enable verbose logging (useful for debugging connection issues)
* Set environment variable `DEEJ_NO_TRAY_ICON=1` to run without a tray icon (useful for headless setups or scripts)

---

## 🔌 Transport Options

You can now choose how Deej communicates with your mixer:

### 1. Wired UART (Serial)

* Connect the ESP32 to your PC using a USB cable.
* Configure port and baud rate in Deej.
* Reliable for low-latency setups or when Wi-Fi is unavailable.

### 2. Wi-Fi / Server-Sent Events (SSE)

* Use the ESPHome Event Source API to transmit data over Wi-Fi.
* Supports multiple Deej clients connecting simultaneously.
* No drivers needed for Windows, macOS, or Linux.
* Integrates easily with Home Assistant.

### 3. Hybrid Setup (ESP32 is connected to a Wi-Fi network)
* One computer is connected to the ESP32 via USB-UART cable and communicates with it exclusively over serial (configure only UART port and baud rate in `config.yaml`).
* Other computers on the same network can connect to the same ESP32 device over Wi-Fi using SSE (configure only SSE URL in their `config.yaml`).
* This allows one primary controller via UART while enabling additional computers to monitor or control the mixer wirelessly over the network.

---

## 🧾 Bill of Materials

| Qty | Item                 | Link                                                                |
| --- | -------------------- | ------------------------------------------------------------------- |
| 6×  | Potentiometer Module | [AliExpress](https://www.aliexpress.com/item/1005006733220962.html) |
| 1×  | ESP32 Board          | [AliExpress](https://www.aliexpress.com/item/1005009640243412.html) |
| 6×  | Switch               | Any kind of latching/momentary switch/button. (optionally)          |

  (desolder the **side pins** on the potentiometers before mounting)

  You can, at your discretion, freely change the number of potentiometers or switches, adjusting the YAML to suit your situation.

---

## 🔧 STL / Assembly

* STL files for **this exact BOM** that fit perfectly can be found in the `ref/` directory:
  * big_bot.stl, big_top.stl
  * small_bot.stl, small_top.stl

For assembly you will also need M3 hot inserts with a external diameter of 4.5mm, M3x8 screws, 20x20x2 mm silicone pads.

![Example](ref/20251021_223354.jpg)

---

## 🔌 Hardware Notes

The ESP32 ADC limit is about **3.12 V**, while its LDO outputs **3.3 V**.
To stay within range, lower the potentiometer reference voltage:

### Option 1 — use voltage Divider
**(!) in most cases you will need to change adc_attenuation field from 12db to 6db in device configuration yaml**

![Voltage Divider](ref/1.png)


### Option 2 — use diode (preferred)

Use a small PN diode to drop ≈ 0.2–0.7 V
*(actually, almost any diode will be fine — even one found in a junk box, as long as it’s not burned out)*.


![Diode](ref/2.png)

---

## 💡 Hints

* Avoid **ADC3** — it’s internally reserved.
* **GPIO 8** is used as the "ADC maximum" reference input.
* to use mute/unmute (sw0..sw5) just connect any switch/latching/momentary button between GPIO9..GPIO14 and GND and configure binary_sensor section in yaml, and, switches_mapping section in deej config.
* status led (blue) states: constantly ON = wifi not configured; blinking = connecting/not connected; constantly OFF = connected

---

## 🧱 License & Build

License, build process, and Deej binary behavior are the same as in the original project:
👉 [https://github.com/omriharel/deej](https://github.com/omriharel/deej)
