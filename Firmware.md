# ⚙️ Firmware Documentation

This document describes the ESPHome-based firmware for the ESP32 mixer.

---

## Platform & Framework

* **Framework**: [ESPHome](https://esphome.io/) (ESP-IDF based)
* **Minimum ESPHome version**: 2025.12.5

ESPHome provides a YAML-based configuration system that makes it easy to customize firmware behavior without writing C++ code. However, this project includes custom C++ components (`mix_tools.hpp` and `mix_tools.cpp`) for advanced functionality.

---

## Building & Flashing

### Prerequisites

**ESPHome**: Install via [ESPHome documentation](https://esphome.io/guide/installation.html) or, use Home Assistant add-on (if using HA)

### Configuration Files

All firmware configuration files are located in the `esphome/` directory:

* `mix_latching.yaml` - Configuration for latching switches (preferred for cases when mute switches not required)
* `mix_momentary.yaml` - Configuration for momentary switches
* `mix_tools.hpp` - C++ header with custom functions
* `mix_tools.cpp` - C++ source file (minimal, mostly definitions)

### Building Process

1. **Copy configuration files** to your ESPHome config directory:
   * Copy `mix_*.yaml` file (choose latching or momentary)
   * Copy `mix_tools.hpp` and `mix_tools.cpp` to the same directory
   * Modify .yaml (and .hpp) to fit your needs.

2. **Configure && Build**:
   ```bash
   esphome compile mix_latching.yaml
   ```

3. **Flash firmware**:
   ```bash
   esphome upload mix_latching.yaml
   ```
   Or use ESPHome dashboard/web interface for easier management.

### Home Assistant + ESPHome plugin

**Video tutorial**: [![ESP32 Flashing Process](https://img.youtube.com/vi/4NSVlBNJve0/0.jpg)](https://www.youtube.com/watch?v=4NSVlBNJve0)

---

## Configuration Options

### Default Features (Enabled by Default)

The following features are **active by default** in the provided configurations:


#### ✅ Sliders (Potentiometers)

**Status**: Always enabled

**Count**: 6 sliders (pot0..pot5)

**GPIO pins**: GPIO1, GPIO2, GPIO4, GPIO5, GPIO6, GPIO7

**Reference pin**: GPIO8 (ADC maximum reference)

**Function**: Volume control for audio applications


#### ✅ Wi-Fi Connection (Optional)

**Status**: Enabled after setup.

**Setup**: Configure via https://web.esphome.io/ after flashing, or in .yaml

**Status LED**: GPIO48 (blue LED)

* Constantly ON = Wi-Fi not configured
* Blinking = Connecting/not connected
* Constantly OFF = Connected


#### ✅ Web Server component

**Status**: Always enabled

**Port**: 80

**Access**: http://device-name.local/ (when connected to Wi-Fi)

**SSE source**: http://device-name.local/events/ (used to connect deej softare)

**Function**: Device monitoring & status


#### ✅ Action Buttons (Optional)

**Status**: Enabled by default, can be disabled

**Count**: 6 buttons (btn0..btn5)

**GPIO pins**: GPIO15, GPIO16, GPIO17, GPIO18, GPIO21, GPIO40

**Types**: Single click, double click, long press

**Function**: Trigger actions in deej software (launch apps, execute commands, etc.)


#### ✅ Mute Switches (Optional Features)

**Status**: Enabled by default, can be disabled by removing the sw* and related entities.

**Count**: 6 buttons (sw0..sw5)

**GPIO pins**: GPIO9..GPIO14 (sw0..sw5)

**Configuration**:
  
  **Latching switches**: Use `mix_latching.yaml` (preferred)
  
  **Momentary switches**: Use `mix_momentary.yaml`


#### 🔧 Extra UART (Multi-Wired Setup)

**Status**: Disabled by default

**GPIO pins**: GPIO41 (TX), GPIO42 (RX) - can be changed in YAML (default in example config)

**Purpose**: Enable additional wired UART channel for multi-PC setups (duplicate data stream)

**Configuration**:
1. In `mix_tools.hpp`: Uncomment: `#define USE_EXTRA_UART`
2. In YAML file: Uncomment `uart:` section
3. In YAML file: Uncomment `set_extra_uart(extra_uart);` in `on_boot:` section

**Requirements**: UART isolators and isolated DC-DC converters (see [Hardware.md](Hardware.md#multi-wired-setup))


#### 🔧 Home Assistant API

**Status**: Disabled by default

**Purpose**: Integrate mixer with Home Assistant for automation

**Configuration**: Uncomment `api:` section in YAML

**Usage**: Monitor sliders/switches/buttons state from Home Assistant, use in automations


#### 🔧 OTA Updates

**Status**: Disabled by default

**Purpose**: Update firmware over Wi-Fi without physical access

**Configuration**:
  
  1. Create `secrets.yaml` with `ota_password: "your_password"`
  
  2. Uncomment `ota:` section in YAML
  
  3. Set password in `ota_password: !secret ota_password`

**Security**: Always use a strong password


#### 🔧 Captive Portal

**Status**: Disabled by default

**Purpose**: Fallback Wi-Fi hotspot for configuration

**Configuration**:
  
  1. Uncomment `captive_portal:` section
  
  2. **Important**: Remove `CONFIG_ESP_WIFI_SOFTAP_SUPPORT: "n"` from `sdkconfig_options`

**Note**: Requires SoftAP support, which is disabled by default for performance


#### 🔧 Factory Reset Button

**Status**: Disabled by default

**Purpose**: Reset device to factory defaults (erase Wi-Fi settings)

**Configuration**: Uncomment `button:` section with `platform: factory_reset`

---

## Customizing Slider/Switch/Button Count

You can easily (remove excessive) adjust the number of sliders, switches, or buttons to match your hardware.

### Reducing Slider Count

**Example**: Use only 3 sliders instead of 6

1. **Remove sensor blocks**: Delete `s0_v`, `s1_v`, `s2_v` ADC sensors (keep only the ones you need)
2. **Remove template sensors**: Delete corresponding `pot0`, `pot1`, `pot2` template sensors
3. **Update process_pot calls**: Keep only the lambda functions for your sliders

**Note**: Slider IDs start at 0. If you use 3 sliders, they should be pot0, pot1, pot2.

### Reducing Switch Count

**Example**: Use only 2 switches instead of 6

1. **Remove binary_sensor blocks**: Delete `sw2`, `sw3`, `sw4`, `sw5` (keep only sw0, sw1)
2. **Update process_sw calls**: Keep only the lambda functions for your switches

### Reducing Button Count

**Example**: Use only 3 buttons instead of 6

1. **Remove button blocks**: Delete `btn3`, `btn4`, `btn5` (keep only btn0, btn1, btn2)
2. **Update script calls**: Keep only the script.execute calls for your buttons

### Adding More Components

The code supports up to 32 sliders/switches (`MIXER_POT_COUNT_MAX` and `MIXER_SW_COUNT_MAX` in `mix_tools.hpp`). To add more:

1. **Add sensor/binary_sensor/button blocks** in YAML
2. **Use available GPIO pins** (see [Hardware.md](Hardware.md#gpio-pin-restrictions))
3. **Call appropriate function**: `process_pot()`, `process_sw()`, or `process_btn()`

---

## GPIO Pin Restrictions

⚠️ **Important**: Not all GPIO pins can be used. Some pins have special functions or restrictions.

See [Hardware.md](Hardware.md#gpio-pin-restrictions) for complete GPIO pin information and restrictions.

**Quick reference** (ESP32-S3-N16R8):
* **ADC2 pins (GPIO12-GPIO18)**: Cannot be used for analog input when Wi-Fi is enabled
* **Boot pins (GPIO3, GPIO37, GPIO45, GPIO46, GPIO48)**: Can cause boot issues
* **GPIO48**: Used for status LED (blue LED)

---

## Configuration Parameters

### Substitutions (Top of YAML file)

These parameters control firmware behavior:

* `adc_attenuation`: ADC attenuation (6db or 12db) - affects voltage range
* `debounce_interval`: Switch/button debounce time in milliseconds (default: 30ms)
* `throtle_interval`: Throttle interval for sliders when moving (default: 75ms)
* `update_interval`: Normal update interval for sliders (default: 10ms)
* `adc_samples`: Number of ADC samples for averaging (default: 16)
* `click_timing_single`: Timing for single click detection (default: `[ON for at most 0.5s, OFF for at least 0.2s]`)
* `click_timing_double`: Timing for double click detection (default: `[ON for at most 0.5s, OFF for at most 0.4s, ON for at most 0.5s]`)
* `click_timing_long`: Timing for long press detection (default: `[ON for at least 0.8s]`)

### Adjusting for Your Hardware

* **Voltage divider**: If using voltage divider (pot vcc ~= 1.5v), change `adc_attenuation` to `6db` (to increase resolution, if not enought)
* **Diode drop**: If using diode, keep `adc_attenuation` at `12db` (default)
* **More sliders**: Increase `update_interval` if experiencing performance issues
* **Faster response**: Decrease `throtle_interval` (may cause performance issues)

---

## Wi-Fi Configuration

### Option 1: Hardcoded Credentials (Not Recommended)

Configure the `wifi:` section (there is commented section as example) with your SSID and password. Requires reflashing to change.

### Option 2: Improv Serial (Recommended)

Use empty `wifi:` section with `improv_serial:` component. After flashing:

1. Connect to device via USB
2. Open https://web.esphome.io/
3. Select your device
4. Click "Configure WiFi" (drop down menu button)
5. select SSID and use password to connect.

**Advantage**: Change Wi-Fi settings anytime without reflashing

---

## Troubleshooting

### Device Not Responding (UART) After Flashing

* Make sure that you not violates restricted GPIOS. (for example GPIO37 configured as input will cause that problem)
* try to connect via web.esphome.io, look for "USB JTAG" entry, or, use Device Manager (Windows) or `ls /dev/tty*` (Linux)

### Wi-Fi Not Connecting

* Use Improv Serial to reconfigure Wi-Fi
* Check signal strength via log (web.esphome.io)
* Verify SSID and password are correct
* Check if 2.4GHz network (ESP32 doesn't support 5GHz)

### ADC Readings Unstable

* Check your wirings!

### Buttons Not Working

* Verify GPIO pins are correct
* Check button wiring (INPUT_PULLUP mode, button between GPIO and GND)
* Adjust `debounce_interval` if buttons causing "double clicks" (triggers multiple times)
* Check `click_timing_*` parameters if clicks not detected

---

## Advanced Configuration

### Custom Functions

The `mix_tools.hpp` file contains custom functions that can be modified:

* `process_pot()`: Slider processing logic
* `process_sw()`: Switch processing logic  
* `process_btn()`: Button processing logic
* `hostsend_*()`: Data transmission functions

### Performance Tuning

The YAML includes extensive ESP-IDF SDK configuration for optimal performance:

* Wi-Fi task priority and CPU affinity
* PSRAM configuration
* Memory management
* Network stack tuning

These are pre-configured for best performance. Only modify if you understand the implications.

---

## Related Documentation

* [Hardware.md](Hardware.md) - Hardware details, schematics, GPIO information
* [Software.md](Software.md) - Software features and configuration
* [ESPHome Documentation](https://esphome.io/) - ESPHome framework reference
