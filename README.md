# 🧭 ESPHome Deej Fork

A **variant of [Deej](https://github.com/omriharel/deej)** that uses **[ESPHome](https://esphome.io/) (ESP32)** instead of Arduino.

---

## 💡 Overview

In this fork, the **transport layer is replaced**. (USB-UART > TCP)
Instead of using a serial interface, data is transmitted with help of **[ESPHome Event Source API](https://esphome.io/web-api/#event-source-api)** — a feature built into the ESPHome `web_server` component.

As a result:

* multiple Deej software instances can connect to the same mixer over Wi-Fi
* no more one-PC-per-mixer limitation
* and **no USB-UART drivers are needed** (hello from win11)

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

* STL for **this exact BOM** fits perfectly can be found in 

```
esphome/
```

  (desolder the **side pins** on the potentiometers before mounting)
  
---

## 🔌 Hardware Notes

The ESP32 ADC limit is about **3.12 V**, while its LDO outputs **3.3 V**.
To stay within range, lower the potentiometer reference voltage:

### Option 1 — use voltage Divider

![Voltage Divider](ref/1.png)

### Option 2 — use diode

Use a small diode (e.g. 1N4148) to drop ≈ 0.2–0.3 V
*(actually, almost any will be fine — even one found in a junk box, as long as it’s not burned out)*.
![Diode](ref/2.png)

---

## 💡 Hints

* Avoid **ADC3** — it’s internally reserved.
* **GPIO 8** is used as the "ADC maximum" reference input.


---

## 🧱 License & Build

License, build process, and Deej binary behavior are the same as in the original project:
👉 [https://github.com/omriharel/deej](https://github.com/omriharel/deej)
