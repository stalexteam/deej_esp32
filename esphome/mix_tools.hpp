#pragma once
#include <stdint.h>
#include "esphome.h"
using namespace esphome;

//#define USE_EXTRA_UART // uncomment to use extra_uart

#define MIXER_POT_COUNT_MAX 32
#define MIXER_SW_COUNT_MAX 32
#define MIXER_HYST 3   // 0.3%


#ifdef USE_EXTRA_UART
extern esphome::uart::UARTComponent *global_extra_uart;
inline void set_extra_uart(esphome::uart::UARTComponent *uart_obj) {
    global_extra_uart = uart_obj;
}
#endif

extern sensor::Sensor *global_vref_sensor;
inline void set_vref(sensor::Sensor *sens) {
    global_vref_sensor = sens;
}

extern int mixer_pot_value[MIXER_POT_COUNT_MAX];
extern int mixer_pot_max_id;
inline void hostsend_pot(int id) {
    char buf[64] = {0, };
    snprintf(buf, sizeof(buf), "{\"id\":\"sensor-pot%d\",\"value\":%d}\n", id, (int)(mixer_pot_value[id] / 10));
    
    ESP_LOGW("json", buf);
    #ifdef USE_EXTRA_UART
    if (global_extra_uart != nullptr){
        global_extra_uart->write_str(buf); 
        global_extra_uart->write_str("\n");
    }
    #endif
}


inline void process_pot(
    uint16_t pot_id,
    adc::ADCSensor *adc_sens,
    template_::TemplateSensor *pot_sens,
    uint16_t pot_raw,
    uint32_t normal_ms,
    uint32_t throttle_ms,
    bool invert
) {
    if (pot_id >= MIXER_POT_COUNT_MAX || adc_sens == nullptr || pot_sens == nullptr) return;

    uint16_t vref_raw = 0;
    if (global_vref_sensor != nullptr && !std::isnan(global_vref_sensor->state)) {
        vref_raw = (uint16_t)global_vref_sensor->state;
    }

    int cand = 0;
    if (vref_raw >= 32) {
        cand = (int)((uint32_t)pot_raw * 1000u / vref_raw);
    }

    if (invert) cand = 1000 - cand;
    if (cand < 15) cand = 0;
    if (cand > 985) cand = 1000;

    int last = mixer_pot_value[pot_id];
    int dif = cand - last;

    bool changed = (abs(dif) > MIXER_HYST) || (((cand == 0) || (cand == 1000)) && (last != cand));

    if (changed) {
        mixer_pot_value[pot_id] = cand;
        hostsend_pot(pot_id);
        pot_sens->publish_state(mixer_pot_value[pot_id] / 10);
        
        if (adc_sens->get_update_interval() != throttle_ms) {
            adc_sens->set_update_interval(throttle_ms);
        }
    } else {
        if (adc_sens->get_update_interval() != normal_ms) {
            adc_sens->set_update_interval(normal_ms);
        }
    }
}

extern int mixer_sw_state[MIXER_SW_COUNT_MAX];
extern int mixer_sw_max_id;
inline void hostsend_sw(int id) {
    char buf[64] = {0, };
    snprintf(buf, sizeof(buf), "{\"id\":\"binary_sensor-sw%d\",\"value\":%s}", id, mixer_sw_state[id] ? "true" : "false");
   
    ESP_LOGW("json", buf);
    #ifdef USE_EXTRA_UART
    if (global_extra_uart != nullptr){
        global_extra_uart->write_str(buf); 
        global_extra_uart->write_str("\n");
    }
    #endif
    
}

inline bool process_sw(int sw_id, bool value) {
    if (sw_id >= MIXER_SW_COUNT_MAX)
        return value;

    if (mixer_sw_max_id < sw_id)
        mixer_sw_max_id = sw_id;

    if (mixer_sw_state[sw_id] != (int)value){
        mixer_sw_state[sw_id] = (int)value;
        hostsend_sw(sw_id);
    }
    
    return value;
}

inline void hostsend_all() {
    for (int i = 0; i <= mixer_pot_max_id; i++) {
        hostsend_pot(i);
    }
    for (int i = 0; i <= mixer_sw_max_id; i++) {
        hostsend_sw(i);
    }
}
