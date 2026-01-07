#pragma once
#include <stdint.h>

#define MIXER_POT_COUNT 16
#define MIXER_HYST 3   // 0.3%

static int16_t mixer_last[MIXER_POT_COUNT] = {
    0, 0, 0, 0,
    0, 0, 0, 0,
    0, 0, 0, 0,
    0, 0, 0, 0
};

static inline int mixer_math(
    uint16_t pot_id,
    uint16_t pot_raw,
    uint16_t vref_raw,
    uint8_t invert
) {
    if (pot_id >= MIXER_POT_COUNT)
        return 0;

    int16_t &last = mixer_last[pot_id];
    if (vref_raw < 32)
        return 0;

    int cand = (int)((uint32_t)pot_raw * 1000u / vref_raw);

    if (invert)
        cand = 1000 - cand;
    if (cand < 15) cand = 0;
    if (cand > 985)  cand = 1000;

    int dif = cand - last;
    if ((dif > MIXER_HYST) || (dif < -MIXER_HYST) || (cand == 0) || (cand == 1000))
        last = cand;

    return last / 10;
}