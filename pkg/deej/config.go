package deej

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"github.com/stalexteam/deej_esp32/pkg/deej/util"
	"go.uber.org/zap"
)

// CanonicalConfig provides application-wide access to configuration fields,
// as well as loading/file watching logic for deej's configuration file
type CanonicalConfig struct {
	SliderMapping   *sliderMap
	SwitchesMapping *switchMap

	ConnectionInfo struct {
		SSE_URL         string
		SERIAL_Port     string
		SERIAL_BaudRate int
	}

	InvertSliders  bool
	InvertSwitches bool

	SliderOverride map[int]int

	logger             *zap.SugaredLogger
	notifier           Notifier
	stopWatcherChannel chan bool

	reloadConsumers []chan bool

	userConfig     *viper.Viper
	internalConfig *viper.Viper
}

const (
	userConfigFilepath = "config.yaml"

	userConfigName     = "config"
	internalConfigName = "preferences"

	userConfigPath = "."

	configType = "yaml"

	configKey_SliderMapping   = "slider_mapping"
	configKey_SwitchesMapping = "switches_mapping"

	configKey_InvertSliders  = "invert_sliders"
	configKey_InvertSwitches = "invert_switches"
	configKey_SliderOverride = "slider_override"

	configKey_SSE_URL         = "SSE_URL"
	configKey_SERIAL_PORT     = "SERIAL_Port"
	configKey_SERIAL_BaudRate = "SERIAL_BaudRate"

	default_SSE_URL         = "" //http://mix.local/events
	default_SERIAL_PORT     = ""
	default_SERIAL_BaudRate = 0
)

// has to be defined as a non-constant because we're using path.Join
var internalConfigPath = path.Join(".", logDirectory)

// NewConfig creates a config instance for the deej object and sets up viper instances for deej's config files
func NewConfig(logger *zap.SugaredLogger, notifier Notifier) (*CanonicalConfig, error) {
	logger = logger.Named("config")

	cc := &CanonicalConfig{
		logger:             logger,
		notifier:           notifier,
		reloadConsumers:    []chan bool{},
		stopWatcherChannel: make(chan bool),
	}

	// distinguish between the user-provided config (config.yaml) and the internal config (logs/preferences.yaml)
	userConfig := viper.New()
	userConfig.SetConfigName(userConfigName)
	userConfig.SetConfigType(configType)
	userConfig.AddConfigPath(userConfigPath)

	userConfig.SetDefault(configKey_SliderMapping, map[string][]string{})
	userConfig.SetDefault(configKey_SwitchesMapping, map[string][]string{})
	userConfig.SetDefault(configKey_InvertSliders, false)
	userConfig.SetDefault(configKey_InvertSwitches, false)
	userConfig.SetDefault(configKey_SliderOverride, map[string]interface{}{})
	userConfig.SetDefault(configKey_SSE_URL, default_SSE_URL)
	userConfig.SetDefault(configKey_SERIAL_PORT, default_SERIAL_PORT)
	userConfig.SetDefault(configKey_SERIAL_BaudRate, default_SERIAL_BaudRate)

	internalConfig := viper.New()
	internalConfig.SetConfigName(internalConfigName)
	internalConfig.SetConfigType(configType)
	internalConfig.AddConfigPath(internalConfigPath)

	cc.userConfig = userConfig
	cc.internalConfig = internalConfig

	logger.Debug("Created config instance")

	return cc, nil
}

// Load reads deej's config files from disk and tries to parse them
func (cc *CanonicalConfig) Load() error {
	cc.logger.Debugw("Loading config", "path", userConfigFilepath)

	if !util.FileExists(userConfigFilepath) {
		cc.logger.Warnw("Config file not found", "path", userConfigFilepath)
		cc.notifier.Notify("Can't find configuration!",
			fmt.Sprintf("%s must be in the same directory as deej. Please re-launch", userConfigFilepath))
		return fmt.Errorf("config file doesn't exist: %s", userConfigFilepath)
	}

	if err := cc.userConfig.ReadInConfig(); err != nil {
		cc.logger.Warnw("Viper failed to read user config", "error", err)
		if strings.Contains(err.Error(), "yaml:") {
			cc.notifier.Notify("Invalid configuration!",
				fmt.Sprintf("Please make sure %s is in a valid YAML format.", userConfigFilepath))
		} else {
			cc.notifier.Notify("Error loading configuration!", "Please check deej's logs for more details.")
		}
		return fmt.Errorf("read user config: %w", err)
	}

	if err := cc.internalConfig.ReadInConfig(); err != nil {
		cc.logger.Debugw("Viper failed to read internal config", "error", err, "reminder", "this is fine")
	}

	if err := cc.populateFromVipers(); err != nil {
		cc.logger.Warnw("Failed to populate config fields", "error", err)
		return fmt.Errorf("populate config fields: %w", err)
	}

	cc.logger.Info("Loaded config successfully")
	cc.logger.Infow("Config values",
		"sliderMapping", cc.SliderMapping,
		"switchesMapping", cc.SwitchesMapping,
		"connectionInfo", cc.ConnectionInfo,
		"invertSliders", cc.InvertSliders,
		"invertSwitches", cc.InvertSwitches,
		"sliderOverride", cc.SliderOverride,
	)

	return nil
}

// SubscribeToChanges allows external components to receive updates when the config is reloaded
func (cc *CanonicalConfig) SubscribeToChanges() chan bool {
	c := make(chan bool)
	cc.reloadConsumers = append(cc.reloadConsumers, c)

	return c
}

// WatchConfigFileChanges starts watching for configuration file changes
// and attempts reloading the config when they happen
func (cc *CanonicalConfig) WatchConfigFileChanges() {
	cc.logger.Debugw("Starting to watch user config file for changes", "path", userConfigFilepath)

	const (
		minTimeBetweenReloadAttempts = time.Millisecond * 500
		delayBetweenEventAndReload   = time.Millisecond * 50
	)

	lastAttemptedReload := time.Now()

	// establish watch using viper as opposed to doing it ourselves, though our internal cooldown is still required
	cc.userConfig.WatchConfig()
	cc.userConfig.OnConfigChange(func(event fsnotify.Event) {

		// when we get a write event...
		if event.Op&fsnotify.Write == fsnotify.Write {

			now := time.Now()

			// ... check if it's not a duplicate (many editors will write to a file twice)
			if lastAttemptedReload.Add(minTimeBetweenReloadAttempts).Before(now) {

				// and attempt reload if appropriate
				cc.logger.Debugw("Config file modified, attempting reload", "event", event)

				// wait a bit to let the editor actually flush the new file contents to disk
				<-time.After(delayBetweenEventAndReload)

				if err := cc.Load(); err != nil {
					cc.logger.Warnw("Failed to reload config file", "error", err)
				} else {
					cc.logger.Info("Reloaded config successfully")
					cc.notifier.Notify("Configuration reloaded!", "Your changes have been applied.")

					cc.onConfigReloaded()
				}

				// don't forget to update the time
				lastAttemptedReload = now
			}
		}
	})

	// wait till they stop us
	<-cc.stopWatcherChannel
	cc.logger.Debug("Stopping user config file watcher")
	cc.userConfig.OnConfigChange(nil)
}

// StopWatchingConfigFile signals our filesystem watcher to stop
func (cc *CanonicalConfig) StopWatchingConfigFile() {
	cc.stopWatcherChannel <- true
	
	// Close all reload consumer channels to signal goroutines to exit
	cc.closeReloadChannels()
}

// closeReloadChannels closes all reload consumer channels to signal goroutines to exit
func (cc *CanonicalConfig) closeReloadChannels() {
	for _, ch := range cc.reloadConsumers {
		close(ch)
	}
	cc.reloadConsumers = nil
	cc.logger.Debug("Closed all config reload channels")
}

func (cc *CanonicalConfig) populateFromVipers() error {

	// merge the slider mappings from the user and internal configs
	cc.SliderMapping = sliderMapFromConfigs(
		cc.userConfig.GetStringMapStringSlice(configKey_SliderMapping),
		cc.internalConfig.GetStringMapStringSlice(configKey_SliderMapping),
	)

	cc.SwitchesMapping = switchMapFromConfigs(
		cc.userConfig.GetStringMapStringSlice(configKey_SwitchesMapping),
		cc.internalConfig.GetStringMapStringSlice(configKey_SwitchesMapping),
	)

	cc.ConnectionInfo.SSE_URL = cc.userConfig.GetString(configKey_SSE_URL)
	cc.ConnectionInfo.SERIAL_Port = cc.userConfig.GetString(configKey_SERIAL_PORT)
	cc.ConnectionInfo.SERIAL_BaudRate = cc.userConfig.GetInt(configKey_SERIAL_BaudRate)

	cc.InvertSliders = cc.userConfig.GetBool(configKey_InvertSliders)
	cc.InvertSwitches = cc.userConfig.GetBool(configKey_InvertSwitches)

	// Load slider override map
	cc.SliderOverride = make(map[int]int)
	overrideMap := cc.userConfig.GetStringMap(configKey_SliderOverride)
	for sliderIdxString, value := range overrideMap {
		sliderIdx, err := strconv.Atoi(sliderIdxString)
		if err != nil {
			cc.logger.Warnw("Invalid slider index in slider_override", "index", sliderIdxString, "error", err)
			continue
		}

		// Handle different possible types from YAML (int, float64, string, or nil)
		// nil or empty string means no override for this slider
		if value == nil {
			continue
		}

		var percent int
		switch v := value.(type) {
		case int:
			percent = v
		case float64:
			percent = int(v)
		case string:
			if v == "" {
				// Empty value means no override for this slider
				continue
			}
			parsed, err := strconv.Atoi(v)
			if err != nil {
				cc.logger.Warnw("Invalid slider override value", "slider", sliderIdx, "value", v, "error", err)
				continue
			}
			percent = parsed
		default:
			cc.logger.Warnw("Unexpected type for slider override value", "slider", sliderIdx, "type", fmt.Sprintf("%T", value))
			continue
		}

		// Validate percentage range
		if percent < 0 || percent > 100 {
			cc.logger.Warnw("Slider override value out of range", "slider", sliderIdx, "value", percent)
			if percent < 0 {
				percent = 0
			}
			if percent > 100 {
				percent = 100
			}
		}

		cc.SliderOverride[sliderIdx] = percent
	}

	cc.logger.Debug("Populated config fields from vipers")

	return nil
}

func (cc *CanonicalConfig) onConfigReloaded() {
	cc.logger.Debug("Notifying consumers about configuration reload")

	for _, consumer := range cc.reloadConsumers {
		// Safely send to channel, handling closed channels
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Channel is closed, ignore
					cc.logger.Debugw("Config reload channel closed, skipping notification", "recover", r)
				}
			}()
			select {
			case consumer <- true:
			default:
				// Channel is full, skip
			}
		}()
	}
}
