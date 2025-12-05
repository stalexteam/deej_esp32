#pragma once
#include <stdint.h>

static inline int mixer_math(uint16_t pot_raw, uint16_t vref_raw, int invert) {
    if (vref_raw < 32)
        return 0;

    int cand = (int)((uint32_t)pot_raw * 100u / vref_raw);
    if (cand < 0) cand = 0;
    if (cand > 100) cand = 100;

    if (invert)
        cand = 100 - cand;

    return cand;
}