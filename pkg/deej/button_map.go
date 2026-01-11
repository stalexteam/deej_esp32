package deej

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Button action types
const (
	ButtonActionSingle = "single"
	ButtonActionDouble = "double"
	ButtonActionLong   = "long"
)

// Action step types
const (
	ActionTypeExecute   = "execute"
	ActionTypeDelay     = "delay"
	ActionTypeKeystroke = "keystroke"
	ActionTypeTyping    = "typing"
)

// ButtonActionConfig represents configuration for a single action type (single/double/long)
type ButtonActionConfig struct {
	Exclusive bool         `json:"exclusive"` // Default: true
	Steps     []ActionStep `json:"steps"`
}

// WaitWnd represents window waiting configuration for execute action
type WaitWnd struct {
	Timeout int    `json:"timeout"`           // Required: timeout in milliseconds
	Focused bool   `json:"focused,omitempty"` // Optional: check if window is focused (default: false)
	Title   string `json:"title,omitempty"`   // Optional: window title for more precise search
}

// ActionStep represents a single step in an action sequence
type ActionStep struct {
	Type        string   `json:"type"` // execute, delay, keystroke, typing
	App         string   `json:"app,omitempty"`
	Args        []string `json:"args,omitempty"`
	Wait        bool     `json:"wait,omitempty"`         // For execute: wait for completion
	WaitTimeout int      `json:"wait_timeout,omitempty"` // For execute: timeout in milliseconds (0 = infinite, default: 0)
	WaitWnd     *WaitWnd `json:"wait_wnd,omitempty"`     // For execute: wait for window (only with wait: false)
	Ms          int      `json:"ms,omitempty"`           // For delay: duration in milliseconds
	Keys        string   `json:"keys,omitempty"`         // For keystroke: key combination
	Text        string   `json:"text,omitempty"`         // For typing: text to type
	CharDelay   int      `json:"char_delay,omitempty"`   // For typing: delay between characters in milliseconds (optional)
}

// ButtonConfig represents configuration for a single button
type ButtonConfig struct {
	Single *ButtonActionConfig `json:"single,omitempty"`
	Double *ButtonActionConfig `json:"double,omitempty"`
	Long   *ButtonActionConfig `json:"long,omitempty"`
}

// ButtonsMapping represents the complete button actions configuration
type ButtonsMapping struct {
	CancelOnReload bool                  `json:"cancel_on_reload"` // Default: false
	Buttons        map[int]*ButtonConfig `json:"buttons"`
	logger         *zap.SugaredLogger
}

// buttonsMap is the internal implementation
type buttonsMap struct {
	CancelOnReload bool
	Buttons        map[int]*ButtonConfig
	logger         *zap.SugaredLogger
}

// get returns the action configuration for a specific button and action type
func (bm *buttonsMap) get(buttonID int, actionType string) (*ButtonActionConfig, bool) {
	buttonConfig, ok := bm.Buttons[buttonID]
	if !ok {
		return nil, false
	}

	var actionConfig *ButtonActionConfig
	switch actionType {
	case ButtonActionSingle:
		actionConfig = buttonConfig.Single
	case ButtonActionDouble:
		actionConfig = buttonConfig.Double
	case ButtonActionLong:
		actionConfig = buttonConfig.Long
	default:
		return nil, false
	}

	if actionConfig == nil {
		return nil, false
	}

	return actionConfig, true
}

// iterate calls the provided function for each button configuration
func (bm *buttonsMap) iterate(f func(buttonID int, config *ButtonConfig)) {
	for buttonID, config := range bm.Buttons {
		f(buttonID, config)
	}
}

// buttonsMapFromConfig parses button actions configuration from viper
func buttonsMapFromConfig(userConfig *viper.Viper, logger *zap.SugaredLogger) *buttonsMap {
	logger = logger.Named("button_map")

	bm := &buttonsMap{
		CancelOnReload: false,
		Buttons:        make(map[int]*ButtonConfig),
		logger:         logger,
	}

	// Get button_actions section
	buttonActionsMap := userConfig.GetStringMap("button_actions")
	if buttonActionsMap == nil {
		logger.Debug("No button_actions section found in config")
		return bm
	}

	// Get cancel_on_reload (at root of button_actions)
	if cancelOnReload, ok := buttonActionsMap["cancel_on_reload"].(bool); ok {
		bm.CancelOnReload = cancelOnReload
	}

	// Parse button configurations
	for key, value := range buttonActionsMap {
		// Skip cancel_on_reload key
		if key == "cancel_on_reload" {
			continue
		}

		// Parse button ID
		buttonID, err := strconv.Atoi(key)
		if err != nil {
			logger.Warnw("Invalid button ID in config", "key", key, "error", err)
			continue
		}

		// Parse button configuration
		buttonConfigMap, ok := value.(map[string]interface{})
		if !ok {
			logger.Warnw("Invalid button configuration format", "button", buttonID)
			continue
		}

		buttonConfig := &ButtonConfig{}

		// Parse single action
		if singleMap, ok := buttonConfigMap[ButtonActionSingle].(map[string]interface{}); ok {
			buttonConfig.Single = parseActionConfig(singleMap, logger, buttonID, ButtonActionSingle)
		}

		// Parse double action
		if doubleMap, ok := buttonConfigMap[ButtonActionDouble].(map[string]interface{}); ok {
			buttonConfig.Double = parseActionConfig(doubleMap, logger, buttonID, ButtonActionDouble)
		}

		// Parse long action
		if longMap, ok := buttonConfigMap[ButtonActionLong].(map[string]interface{}); ok {
			buttonConfig.Long = parseActionConfig(longMap, logger, buttonID, ButtonActionLong)
		}

		bm.Buttons[buttonID] = buttonConfig
		logger.Debugw("Parsed button configuration", "button", buttonID)
	}

	logger.Infow("Loaded button actions configuration",
		"buttons_count", len(bm.Buttons),
		"cancel_on_reload", bm.CancelOnReload)

	return bm
}

// parseActionConfig parses a single action configuration (single/double/long)
func parseActionConfig(actionMap map[string]interface{}, logger *zap.SugaredLogger, buttonID int, actionType string) *ButtonActionConfig {
	config := &ButtonActionConfig{
		Exclusive: true, // Default value
		Steps:     []ActionStep{},
	}

	// Parse exclusive (default: true)
	if exclusive, ok := actionMap["exclusive"].(bool); ok {
		config.Exclusive = exclusive
	}

	// Parse steps
	stepsRaw, ok := actionMap["steps"]
	if !ok {
		logger.Debugw("No steps found for action", "button", buttonID, "action", actionType)
		return config
	}

	// Log the raw type for debugging
	logger.Debugw("Steps raw type", "button", buttonID, "action", actionType, "type", fmt.Sprintf("%T", stepsRaw), "value", fmt.Sprintf("%+v", stepsRaw))

	// Try to convert to slice - Viper may return different types
	var stepsSlice []interface{}
	switch v := stepsRaw.(type) {
	case []interface{}:
		stepsSlice = v
	case []map[string]interface{}:
		// Convert []map[string]interface{} to []interface{}
		stepsSlice = make([]interface{}, len(v))
		for i, m := range v {
			stepsSlice[i] = m
		}
	default:
		// Try to use reflection or convert via interface{}
		logger.Warnw("Steps is not a recognized slice type, attempting conversion", "button", buttonID, "action", actionType, "type", fmt.Sprintf("%T", stepsRaw))
		// Try to convert via interface{} slice
		if reflectValue := reflect.ValueOf(stepsRaw); reflectValue.Kind() == reflect.Slice {
			stepsSlice = make([]interface{}, reflectValue.Len())
			for i := 0; i < reflectValue.Len(); i++ {
				stepsSlice[i] = reflectValue.Index(i).Interface()
			}
		} else {
			logger.Warnw("Steps is not a slice", "button", buttonID, "action", actionType, "type", fmt.Sprintf("%T", stepsRaw))
			return config
		}
	}

	for stepIdx, stepInterface := range stepsSlice {
		// Log the step interface type for debugging
		logger.Debugw("Step interface type", "button", buttonID, "action", actionType, "step", stepIdx, "type", fmt.Sprintf("%T", stepInterface))

		// Try to convert to map[string]interface{}
		var stepMap map[string]interface{}
		var ok bool

		// Direct type assertion
		stepMap, ok = stepInterface.(map[string]interface{})
		if !ok {
			// Try map[interface{}]interface{} (Viper sometimes returns this)
			if mapAny, okAny := stepInterface.(map[interface{}]interface{}); okAny {
				stepMap = make(map[string]interface{})
				for k, v := range mapAny {
					keyStr := fmt.Sprintf("%v", k)
					stepMap[keyStr] = v
				}
				ok = true
			} else {
				// Try using reflection for more flexible conversion
				stepValue := reflect.ValueOf(stepInterface)
				if stepValue.Kind() == reflect.Map {
					stepMap = make(map[string]interface{})
					for _, key := range stepValue.MapKeys() {
						keyStr := fmt.Sprintf("%v", key.Interface())
						stepMap[keyStr] = stepValue.MapIndex(key).Interface()
					}
					ok = true
				} else {
					logger.Warnw("Invalid step format", "button", buttonID, "action", actionType, "step", stepIdx, "type", fmt.Sprintf("%T", stepInterface), "kind", stepValue.Kind())
					continue
				}
			}
		}

		step := ActionStep{}

		// Parse type (required)
		if stepType, ok := stepMap["type"].(string); ok {
			step.Type = stepType
		} else {
			logger.Warnw("Step missing type", "button", buttonID, "action", actionType, "step", stepIdx)
			continue
		}

		// Parse step-specific fields based on type
		switch step.Type {
		case ActionTypeExecute:
			if app, ok := stepMap["app"].(string); ok {
				step.App = app
			}
			if args, ok := stepMap["args"].([]interface{}); ok {
				step.Args = make([]string, 0, len(args))
				for _, arg := range args {
					if argStr, ok := arg.(string); ok {
						step.Args = append(step.Args, argStr)
					}
				}
			}
			if wait, ok := stepMap["wait"].(bool); ok {
				step.Wait = wait
			}
			// Parse wait_timeout
			if waitTimeout, ok := stepMap["wait_timeout"].(float64); ok {
				step.WaitTimeout = int(waitTimeout)
			} else if waitTimeout, ok := stepMap["wait_timeout"].(int); ok {
				step.WaitTimeout = waitTimeout
			}
			// Parse wait_wnd
			waitWndRaw, hasWaitWnd := stepMap["wait_wnd"]
			logger.Debugw("Parsing wait_wnd", "button", buttonID, "action", actionType, "step", stepIdx, "has_wait_wnd", hasWaitWnd, "type", fmt.Sprintf("%T", waitWndRaw))

			if hasWaitWnd {
				var waitWndMap map[string]interface{}
				var ok bool

				// Try direct type assertion
				waitWndMap, ok = waitWndRaw.(map[string]interface{})
				if !ok {
					// Try map[interface{}]interface{} (Viper sometimes returns this)
					if mapAny, okAny := waitWndRaw.(map[interface{}]interface{}); okAny {
						waitWndMap = make(map[string]interface{})
						for k, v := range mapAny {
							keyStr := fmt.Sprintf("%v", k)
							waitWndMap[keyStr] = v
						}
						ok = true
					}
				}

				if ok {
					waitWnd := &WaitWnd{}
					logger.Debugw("wait_wnd map parsed", "button", buttonID, "action", actionType, "step", stepIdx, "map", fmt.Sprintf("%+v", waitWndMap))
					// Timeout is required
					if timeout, ok := waitWndMap["timeout"].(float64); ok {
						waitWnd.Timeout = int(timeout)
					} else if timeout, ok := waitWndMap["timeout"].(int); ok {
						waitWnd.Timeout = timeout
					}
					// Focused is optional
					if focused, ok := waitWndMap["focused"].(bool); ok {
						waitWnd.Focused = focused
					}
					// Title is optional
					if title, ok := waitWndMap["title"].(string); ok {
						waitWnd.Title = title
					}
					// Only set if timeout is valid (required field)
					if waitWnd.Timeout > 0 {
						step.WaitWnd = waitWnd
						logger.Debugw("wait_wnd configured", "button", buttonID, "action", actionType, "step", stepIdx, "timeout", waitWnd.Timeout, "focused", waitWnd.Focused)
					} else {
						logger.Warnw("wait_wnd timeout is invalid or missing", "button", buttonID, "action", actionType, "step", stepIdx)
					}
				} else {
					logger.Warnw("wait_wnd is not a map", "button", buttonID, "action", actionType, "step", stepIdx, "type", fmt.Sprintf("%T", waitWndRaw))
				}
			} else {
				logger.Debugw("wait_wnd not found in step", "button", buttonID, "action", actionType, "step", stepIdx)
			}

		case ActionTypeDelay:
			if ms, ok := stepMap["ms"].(float64); ok {
				step.Ms = int(ms)
			} else if ms, ok := stepMap["ms"].(int); ok {
				step.Ms = ms
			}

		case ActionTypeKeystroke:
			if keys, ok := stepMap["keys"].(string); ok {
				step.Keys = keys
			}

		case ActionTypeTyping:
			if text, ok := stepMap["text"].(string); ok {
				step.Text = text
			}
			if charDelay, ok := stepMap["char_delay"].(float64); ok {
				step.CharDelay = int(charDelay)
			} else if charDelay, ok := stepMap["char_delay"].(int); ok {
				step.CharDelay = charDelay
			}
		}

		config.Steps = append(config.Steps, step)
		logger.Debugw("Added step to action",
			"button", buttonID,
			"action", actionType,
			"step_idx", stepIdx,
			"step_type", step.Type,
			"step_details", fmt.Sprintf("%+v", step))
	}

	logger.Infow("Parsed action configuration",
		"button", buttonID,
		"action", actionType,
		"exclusive", config.Exclusive,
		"steps_count", len(config.Steps),
		"steps", config.Steps)

	return config
}

// Validate validates the button mapping configuration
func (bm *buttonsMap) Validate() error {
	for buttonID, config := range bm.Buttons {
		if config.Single != nil {
			if err := bm.validateActionConfig(buttonID, ButtonActionSingle, config.Single); err != nil {
				return fmt.Errorf("button %d single action: %w", buttonID, err)
			}
		}
		if config.Double != nil {
			if err := bm.validateActionConfig(buttonID, ButtonActionDouble, config.Double); err != nil {
				return fmt.Errorf("button %d double action: %w", buttonID, err)
			}
		}
		if config.Long != nil {
			if err := bm.validateActionConfig(buttonID, ButtonActionLong, config.Long); err != nil {
				return fmt.Errorf("button %d long action: %w", buttonID, err)
			}
		}
	}
	return nil
}

// validateActionConfig validates a single action configuration
func (bm *buttonsMap) validateActionConfig(buttonID int, actionType string, config *ButtonActionConfig) error {
	if len(config.Steps) == 0 {
		return nil // Empty steps are allowed
	}

	for stepIdx, step := range config.Steps {
		switch step.Type {
		case ActionTypeExecute:
			if step.App == "" {
				return fmt.Errorf("step %d: app is required for execute action", stepIdx)
			}
			// Validate wait_timeout: can only be used with wait: true
			if step.WaitTimeout < 0 {
				return fmt.Errorf("step %d: wait_timeout must be non-negative (0 = infinite)", stepIdx)
			}
			if step.WaitTimeout > 0 && !step.Wait {
				return fmt.Errorf("step %d: wait_timeout can only be used when wait is true", stepIdx)
			}
			// Validate wait_wnd: can only be used with wait: false
			if step.WaitWnd != nil {
				if step.Wait {
					return fmt.Errorf("step %d: wait_wnd can only be used when wait is false", stepIdx)
				}
				if step.WaitWnd.Timeout <= 0 {
					return fmt.Errorf("step %d: wait_wnd.timeout must be positive", stepIdx)
				}
			}
		case ActionTypeDelay:
			if step.Ms <= 0 {
				return fmt.Errorf("step %d: ms must be positive for delay action", stepIdx)
			}
		case ActionTypeKeystroke:
			if step.Keys == "" {
				return fmt.Errorf("step %d: keys is required for keystroke action", stepIdx)
			}
		case ActionTypeTyping:
			if step.Text == "" {
				return fmt.Errorf("step %d: text is required for typing action", stepIdx)
			}
		default:
			return fmt.Errorf("step %d: unknown action type: %s", stepIdx, step.Type)
		}
	}

	return nil
}

// ToButtonsMapping converts internal buttonsMap to public ButtonsMapping
func (bm *buttonsMap) ToButtonsMapping() *ButtonsMapping {
	buttons := make(map[int]*ButtonConfig)
	for k, v := range bm.Buttons {
		buttons[k] = v
	}
	return &ButtonsMapping{
		CancelOnReload: bm.CancelOnReload,
		Buttons:        buttons,
	}
}
