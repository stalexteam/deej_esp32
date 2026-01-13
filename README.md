# 🧭 ESPHome Deej Fork

A **variant of [Deej](https://github.com/omriharel/deej)** that uses **[ESPHome](https://esphome.io/) (ESP32)** instead of Arduino, with flexible transport options and extended functionality.

---

## 💡 Overview

This fork transforms the original Arduino-based Deej mixer into an ESP32-powered solution with enhanced capabilities. The mixer can communicate over either **wired UART** or **Wi-Fi (SSE)**, providing flexibility for different use cases.

**Key improvements:**

* **Flexible transport layer**: Connect via USB-UART cable or wirelessly over Wi-Fi or connect deej.. to another deej.
* **Multi-client support**: Multiple Deej software instances can connect to the same mixer simultaneously over Wi-Fi
* **Home Assistant integration**: Easy integration for dimmer control and automation
* **Mute switches**: Hardware mute/unmute switches for audio channels
* **Action buttons**: Physical buttons for launching applications, executing commands, simulating keystrokes, and typing text
* **Path-based process matching**: Match processes by installation directory (e.g., `C:\Program Files (x86)\Steam`)
* **Slider override**: Set constant volume levels for specific slider

---

## ⚙️ Firmware

The firmware runs on **ESP32-S3** using **[ESPHome](https://esphome.io/)** framework. ESPHome provides a user-friendly (no!) YAML-based configuration system that makes it easy to customize the firmware without writing code.

**General information:**

* **Board**: ESP32-S3-N16R8
* **Framework**: ESPHome (ESP-IDF based)
* **Configuration**: YAML files with optional C++ custom components
* **Wi-Fi setup**: Can be configured (or not) via web interface after flashing or hardcoded before flashing.

For detailed information about building, flashing, and configuring the firmware, see **[Firmware.md](Firmware.md)**.

---

## 🔧 Hardware

The hardware consists of an ESP32 board, potentiometer modules, and optional switches/buttons for mute and action controls.

**Quick reference:**

* **BOM**: See [Hardware.md](Hardware.md#bill-of-materials)
* **STL files**: Available in `ref/` directory (big_bot.stl, big_top.stl, small_bot.stl, small_top.stl)
* **Assembly**: M3 hot inserts (4.5mm external diameter), M3x8 screws, 20x20x2 mm silicone pads

For detailed hardware information including schematics, ADC limitations, GPIO pin assignments, and assembly instructions, see **[Hardware.md](Hardware.md)**.

---

## 💻 Software

The Deej software is written in **Go** and provides a desktop application for Windows and Linux that connects to the ESP32 mixer.

**Getting started:**

* **Pre-built binaries**: Download from [releases](https://github.com/stalexteam/deej_esp32/releases/)
* **Build from source**: You can build it manually, or, use build scripts: [pkg/deej/scripts/README.md](pkg/deej/scripts/README.md)

For detailed software features, configuration options, and usage information, see **[Software.md](Software.md)**.

---

## 🔌 Usage Scenarios

The system supports multiple connection scenarios depending on your needs:

### 1. Wired UART (Serial)
**Best for**: Single PC setup, maximum stability

Connect ESP32 via USB cable. Configure `SERIAL_Port` and `SERIAL_BaudRate` in `config.yaml`.

### 2. Wi-Fi / Server-Sent Events (SSE)
**Best for**: Home Assistant integration, Multiple PCs

Connect ESP32 to Wi-Fi. Configure `SSE_URL` in `config.yaml` on each client.

### 3. Hybrid Setup
**Best for**: One primary PC via UART, additional PCs via Wi-Fi

Combine UART connection to one PC with SSE for others.

### 4. Multi-Wired
**Best for**: Multiple PCs, all wired (requires additional hardware)

Uses extra UART channel with galvanic isolation. See [Firmware.md](Firmware.md#multi-wired-setup) for details.

### 5. deej as Data Source (Relay Mode)
**Best for**: ESP32 connected to one PC, other PCs access via network

One deej instance acts as relay, proxying ESP32 data to multiple clients.

**Configuration**: See [Software.md](Software.md#configuration) for detailed configuration options and examples.

---

## 🧱 License

Same as in the original project:
👉 [https://github.com/omriharel/deej](https://github.com/omriharel/deej)


