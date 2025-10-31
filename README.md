# 🧭 ESPHome Deej Fork

A **variant of [Deej](https://github.com/omriharel/deej)** that uses **[ESPHome](https://esphome.io/) (ESP32)** instead of Arduino.

---

## 💡 Overview

In this fork, the **transport layer is replaced**. (USB-UART > TCP)
Instead of using a serial interface, data is transmitted with help of **[ESPHome Event Source API](https://esphome.io/web-api/#event-source-api)** — a feature built into the ESPHome `web_server` component.

As a result:

* multiple Deej software instances can connect to the same mixer over Wi-Fi
* **no USB-UART drivers are needed** (hello from win11)
* ability to integrate mixer into home assistant (dimmer control for example)

---

## ⚙️ Firmware & Configuration

To build and flash the ESP32, you’ll need **ESPHome**.
There are ready-to-use Docker containers and plenty of guides available online — setting it up shouldn’t be a problem.

The device configuration (`.yaml`) and the corresponding header file (`.h`)
are located in this repository under:

```
esphome/
```

---

## 🧾 Bill of Materials

| Qty | Item                 | Link                                                                |
| --- | -------------------- | ------------------------------------------------------------------- |
| 6×  | Potentiometer Module | [AliExpress](https://www.aliexpress.com/item/1005006733220962.html) |
| 1×  | ESP32 Board          | [AliExpress](https://www.aliexpress.com/item/1005009640243412.html) |
| 6×  | Switch               | Any kind of latching button or switch. (optionally)                 |

  (desolder the **side pins** on the potentiometers before mounting)


---

## 🔧 STL/Assembly

* STL for **this exact BOM** fits perfectly can be found in 

```
esphome/stl.zip
```

For assembly you will also need M3 hot inserts with a external diameter of 4.5mm, M3x8 screws, 20x20x2 mm silicone pads.

![Example](ref/20251021_223354.jpg)

---

## 🔌 Hardware Notes

The ESP32 ADC limit is about **3.12 V**, while its LDO outputs **3.3 V**.
To stay within range, lower the potentiometer reference voltage:

### Option 1 — use voltage Divider

![Voltage Divider](ref/1.png)

### Option 2 — use diode

Use a small diode (e.g. 1N4148) to drop ≈ 0.2–0.3 V
*(actually, almost any will be fine — even one found in a junk box, as long as it’s not burned out)*.


**(!) you will need to change attenuation fields from 6db to 12db in esphome yaml**

![Diode](ref/2.png)

---

## 💡 Hints

* Avoid **ADC3** — it’s internally reserved.
* **GPIO 8** is used as the "ADC maximum" reference input.
* to use mute/unmute (sw0..sw5) just connect any switch/latching button between GPIO9..GPIO14 and GND and configure switches_mapping section.

---

## 🧱 License & Build

License, build process, and Deej binary behavior are the same as in the original project:
👉 [https://github.com/omriharel/deej](https://github.com/omriharel/deej)
