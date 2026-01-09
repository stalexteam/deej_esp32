#pragma once
#include <stdint.h>
#include "esphome.h"

#define MIXER_POT_COUNT_MAX 32
#define MIXER_SW_COUNT_MAX 32
#define MIXER_HYST 3   // 0.3%

static int mixer_pot_value[MIXER_POT_COUNT_MAX] = {0};
static int mixer_pot_max_id = -1;
static inline void hostsend_pot(int id) {
    ESP_LOGW("json", "{\"id\":\"sensor-pot%d\",\"value\":%d}", id, (int)(mixer_pot_value[id] / 10));
}

static inline int process_pot(
    uint16_t pot_id,
    uint16_t pot_raw,
    uint16_t vref_raw,
    uint8_t invert
) {
    if ((pot_id >= MIXER_POT_COUNT_MAX) || (vref_raw < 32))
        return 0;

    if (mixer_pot_max_id < pot_id)
        mixer_pot_max_id = pot_id;
        
    int cand = (int)((uint32_t)pot_raw * 1000u / vref_raw);
    if (invert)
        cand = 1000 - cand;
    if (cand < 15) cand = 0;
    if (cand > 985) cand = 1000;

    int last = mixer_pot_value[pot_id];
    int dif = cand - last;

    if ((dif > MIXER_HYST) || (dif < -MIXER_HYST) || (((cand == 0) || (cand == 1000)) && (mixer_pot_value[pot_id] != cand))){
        mixer_pot_value[pot_id] = cand;
        hostsend_pot(pot_id);
    }

    return mixer_pot_value[pot_id] / 10;
}


static bool mixer_sw_state[MIXER_SW_COUNT_MAX] = {false};
static int mixer_sw_max_id = -1;
inline void hostsend_sw(int id) {
    ESP_LOGW("json", "{\"id\":\"binary_sensor-sw%d\",\"value\":%s}", id, mixer_sw_state[id] ? "true" : "false");
}

static inline bool process_sw(int sw_id, bool value) {
    if (sw_id >= MIXER_SW_COUNT_MAX)
        return value;

    if (mixer_sw_max_id < sw_id)
        mixer_sw_max_id = sw_id;

    if (mixer_sw_state[sw_id] != value){
        mixer_sw_state[sw_id] = value;
        hostsend_sw(sw_id);
    }
    
    return value;

}

static inline void hostsend_all() {
    for (int i = 0; i <= mixer_pot_max_id; i++) {
        hostsend_pot(i);
    }
    for (int i = 0; i <= mixer_sw_max_id; i++) {
        hostsend_sw(i);
    }
}