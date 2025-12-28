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
It's a bit tricky because you'll need to put a header (mix_tools.h) file to the config folder, but і think - you can handle it.

If you use HA, everything is [extremely simple](https://drive.google.com/file/d/1BMy3CoxtEZqwKd2B-B0Jz64usQFDXO8t/view?usp=sharing) (through the espome plugin): 

The ESP32 device configuration (`mix_*.yaml`) and the corresponding header file (`mix_tools.h`)
are located in this repository under:


```
esphome/
```

* mix_momentary.yaml - If you wish to use momentary switches to mute the sound. 
* mix_latching.yaml - for latching switches (preffered)
* **If you don't need the mute button functionality**, use mix_latching.yaml and comment out or remove the "binary_sensor" block from yaml.

there 2 option: 
* hardcode your wifi credentials into firmware.
* (preffered) use empty wifi and improv_serial sections and https://web.esphome.io/ (after connecting to device use "vertical ..." -> "Configure WiFi") to configue wifi connection settings any time you want w/o reflashing. 

Optional things (uncomment to activate):
* factory reset button (erase wifi/other settings) 
* home assitant integration
* OTA updates
* Captive portal https://esphome.io/components/captive_portal/


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
**(!) in most cases you will need to change adc_attenuation field from 12db to 6db in device configuration yaml**

![Voltage Divider](ref/1.png)


### Option 2 — use diode (preffered)

Use a small PN diode to drop ≈ 0.2–0.7 V
*(actually, almost any diode will be fine — even one found in a junk box, as long as it’s not burned out)*.


![Diode](ref/2.png)

---

## 💡 Hints

* Avoid **ADC3** — it’s internally reserved.
* **GPIO 8** is used as the "ADC maximum" reference input.
* to use mute/unmute (sw0..sw5) just connect any switch/latching/momentart button between GPIO9..GPIO14 and GND and configure binary_sensor section in yaml, and, switches_mapping section in deej config.
* status led (blue) states: constantly ON = wifi not configured; blinking = connecting/not connected; constantly OFF = connected

---

## 🧱 License & Build

License, build process, and Deej binary behavior are the same as in the original project:
👉 [https://github.com/omriharel/deej](https://github.com/omriharel/deej)
