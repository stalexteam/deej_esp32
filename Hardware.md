# 🔧 Hardware Documentation

This document describes the hardware components, assembly, and electrical considerations for the ESP32 mixer.

---

## Bill of Materials 

|   Qty   | Item                 | Link                                                                   |
| ------- | -------------------- | ---------------------------------------------------------------------- |
|  1..6×  | Potentiometer Module | [AliExpress](https://www.aliexpress.com/item/1005006733220962.html)    |
|   1×    | ESP32 Board          | [AliExpress](https://www.aliexpress.com/item/1005009640243412.html)    |
|  0..6×  | Switch               | Any kind of latching/momentary switch/button (optional, mute feature)  |
|  0..6×  | Switch               | Any kind of momentary button (optional, button feature)                |

**Assembly materials:**
* M3 hot inserts with external diameter of 4.5mm
* M3x8 screws
* 20x20x2 mm silicone pads

**Note**: Desolder the **side pins** on the potentiometers before mounting.

You can freely change the number of potentiometers or switches, adjusting the YAML configuration to suit your situation.

---

## STL Files & Assembly

STL files for the exact BOM configuration are located in the `ref/` directory:

* **big_bot.stl, big_top.stl** - 6x sliders + place for 6x buttons/switches
  * Drill required count of holes for your switches
  * Designed for MTS-101 switches, but most switches with 6mm mount will fit
* **small_bot.stl, small_top.stl** - 6x sliders only

![Assembly Example](ref/20251021_223354.jpg)

---

## Electrical Schematics

### Slider Connection

Each potentiometer connects to ESP32 as follows:

```
Potentiometer          ESP32
-----------            ----
VCC (3.3V)      →      3.3V
GND            →      GND
Wiper (signal) →      GPIO1-7 (one per slider)
```

**Reference voltage**: GPIO8 is connected to 3.3V and serves as ADC maximum reference.

### Switch Connection

Mute switches connect between GPIO (with INPUT_PULLUP) and GND:

```
Switch          ESP32
------           ----
One terminal →   GPIO9-14 (one per switch)
Other terminal → GND
```

### Button Connection

Action buttons connect between GPIO (with INPUT_PULLUP) and GND:

```
Button          ESP32
------           ----
One terminal →   GPIO15, 16, 17, 18, 21, 40 (one per button)
Other terminal → GND
```

---

## ADC Voltage Limitation

### Problem

The ESP32 ADC limit is about **3.12 V**, while its LDO outputs **3.3 V**. If you connect a potentiometer directly to 3.3V, the ADC will saturate at maximum position.

### Solution Options

#### Option 1: Diode Drop (**Simpler, Preferred**)

Use a small PN diode to drop 0.2V+:

![Diode Schematic](ref/2.png)

#### Option 2: Voltage Divider 

Use a voltage divider to reduce the reference voltage:

![Voltage Divider Schematic](ref/1.png)

**⚠️ Important**: Change `adc_attenuation` from `12db` to `6db` in YAML configuration.

---

## GPIO Pin Assignments

### Default Pin Usage

| Function | GPIO Pins | Notes |
|----------|-----------|-------|
| ADC Reference | GPIO8 | Connected to 3.3V |
| Sliders (pot0-5) | GPIO1, GPIO2, GPIO4, GPIO5, GPIO6, GPIO7 | ADC inputs |
| Mute Switches (sw0-5) | GPIO9, GPIO10, GPIO11, GPIO12, GPIO13, GPIO14 | Digital inputs with pull-up |
| Action Buttons (btn0-5) | GPIO15, GPIO16, GPIO17, GPIO18, GPIO21, GPIO40 | Digital inputs with pull-up |
| Status LED | GPIO48 | Blue LED (output) |

### GPIO Pin Restrictions (ESP32-S3-N16R8)

⚠️ **Careful usage required** for your components:

| Mode/Function | GPIO Numbers | Reason |
|---------------|--------------|--------|
| Analog Input (with Wi-Fi ON) | GPIO12-GPIO18 | Belong to ADC2, which is occupied when using Wi-Fi |
| Boot | GPIO3, GPIO45, GPIO46, GPIO48 | Could cause boot issues when tied to GND |
| Boot | GPIO37 | Could cause boot issues when INPUT_PULLUP |
| Reserved | GPIO48 | Used for status LED (blue LED) |

---

## Multi-Wired Setup

For connecting multiple PCs via wired UART, you need additional hardware:

### Additional Components

* **UART isolators** - For galvanic isolation between ESP32 and, USB-UART converters.
* **Isolated DC-DC converters** - For power supply isolation
* **USB-UART converters** - One per computer (excluding the PC that ESP32 is directly connected to)

### Wiring

```
to-do (i'm lazy.)
```

**Configuration**: See [Firmware.md](Firmware.md#extra-uart-multi-wired-setup) for software setup.

---

## Power Supply

### USB Power

ESP32 can be powered via USB cable when connected to a computer.

**Requirements**:
* USB cable with data lines (not charge-only)
* USB port providing at least 500mA

### External Power

For standalone operation (Wi-Fi mode), ESP32 can be powered via:

* **USB power adapter** (5V, 1A minimum)
* **External 5V supply** connected to USB port or 5V pin
* **Battery pack** with USB output

---

## Related Documentation

* [Firmware.md](Firmware.md) - Firmware configuration and features
* [Software.md](Software.md) - Software features and usage
* [ESP32-S3 Datasheet](https://www.espressif.com/sites/default/files/documentation/esp32-s3_datasheet_en.pdf) - Official ESP32-S3 documentation
