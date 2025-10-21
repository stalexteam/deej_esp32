// mix_tools.h
#pragma once
#include <cstdint>
#include <algorithm>

inline int mixer_math(uint16_t pot_raw, uint16_t vref_raw, bool invert = false) {
    if (vref_raw < 32)
        return 0;

    int cand = static_cast<int>(static_cast<uint32_t>(pot_raw) * 100u / vref_raw);
    cand = std::clamp(cand, 0, 100);

    if (invert)
        cand = 100 - cand;

    return cand;
}