#include "mix_tools.hpp"

// Definitions
#ifdef USE_EXTRA_UART
esphome::uart::UARTComponent *global_extra_uart = nullptr;
#endif
sensor::Sensor *global_vref_sensor = nullptr;

int mixer_pot_value[MIXER_POT_COUNT_MAX] = {-1,};
int mixer_pot_max_id = -1;

int mixer_sw_state[MIXER_SW_COUNT_MAX] = {-1,};
int mixer_sw_max_id = -1;
