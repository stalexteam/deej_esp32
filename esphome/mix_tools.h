// mix_tools.h
#pragma once
#include <cstdint>
#include <algorithm>

inline int mixer_math(uint16_t pot_raw, uint16_t vref_raw) {
  
  if (vref_raw < 32) return 0;
  int cand = static_cast<int>(static_cast<uint32_t>(pot_raw) * 100u / vref_raw);
  if (cand < 0) cand = 0;
  if (cand > 100) cand = 100;

  return cand;
}
