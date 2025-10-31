package deej

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bernerdschaefer/eventsource" // go get github.com/bernerdschaefer/eventsource
	"go.uber.org/zap"
)

// SseIO provides a deej-aware abstraction layer to managing Server-Sent Events I/O
type SseIO struct {
	deej   *Deej
	logger *zap.SugaredLogger

	stopChannel chan bool
	connected   bool

	req    *http.Request
	es     *eventsource.EventSource
	ctx    context.Context
	cancel context.CancelFunc

	lastKnownNumSliders int

	sliderMoveConsumers []chan SliderMoveEvent
	switchConsumers     []chan SwitchEvent

	idPattern *regexp.Regexp
}

// SliderMoveEvent represents a single slider move captured by deej
type SliderMoveEvent struct {
	SliderID     int
	PercentValue float32
}

type SwitchEvent struct {
	SwitchID int
	State    bool
}

// NewSseIO creates an SseIO instance that uses the provided deej instance's connection info
func NewSseIO(deej *Deej, logger *zap.SugaredLogger) (*SseIO, error) {
	logger = logger.Named("sse")

	s := &SseIO{
		deej:                deej,
		logger:              logger,
		stopChannel:         make(chan bool),
		connected:           false,
		sliderMoveConsumers: []chan SliderMoveEvent{},
		idPattern:           regexp.MustCompile(`^sensor-(\d+)$`),
		lastKnownNumSliders: 0,
	}

	logger.Debug("Created SSE i/o instance")

	// respond to config changes (mirror serial.go behavior)
	s.setupOnConfigReload()

	return s, nil
}

// Start attempts to connect to the SSE endpoint
func (s *SseIO) Start() error {
	if s.connected {
		s.logger.Warn("Already connected, can't start another without closing first")
		return errors.New("sse: connection already active")
	}

	url := s.deej.config.ConnectionInfo.URL
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("sse: empty ConnectionInfo.URL")
	}

	// build cancellable request
	var err error
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.req, err = http.NewRequestWithContext(s.ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("sse: build request: %w", err)
	}

	// eventsource client (reconnects internally based on Retry field)
	s.es = eventsource.New(s.req, 3*time.Second)

	s.logger.Debugw("Attempting SSE connection", "url", url)
	s.connected = true

	// read events or await a stop
	go func() {
		namedLogger := s.logger.Named("eventstream")
		namedLogger.Infow("Connected", "url", url)

		for {
			select {
			case <-s.stopChannel:
				s.close(namedLogger)
				return
			default:
				// blocking read of next SSE event
				ev, err := s.es.Read()
				if err != nil {
					if s.deej.Verbose() {
						namedLogger.Warnw("Failed to read SSE event", "error", err)
					}
					// Attempt to reconnect.
					continue
				}

				// Non-state events (e.g., ping) = health signal — ignore content
				if ev.Type != "state" {
					if s.deej.Verbose() {
						namedLogger.Debugw("Non-state event", "type", ev.Type, "id", ev.ID)
					}
					continue
				}

				// Handle state
				s.handleStateEvent(namedLogger, ev.Data)
			}
		}
	}()

	return nil
}

// Stop signals us to shut down our SSE connection, if one is active
func (s *SseIO) Stop() {
	if s.connected {
		s.logger.Debug("Shutting down SSE connection")
		s.stopChannel <- true
	} else {
		s.logger.Debug("Not currently connected, nothing to stop")
	}
}

func (s *SseIO) close(logger *zap.SugaredLogger) {
	// cancel context to abort Read()
	if s.cancel != nil {
		s.cancel()
	}

	logger.Debug("SSE connection closed")
	s.es = nil
	s.connected = false
}

// SubscribeToSliderMoveEvents returns an unbuffered channel that receives a SliderMoveEvent every time a slider moves
func (s *SseIO) SubscribeToSliderMoveEvents() chan SliderMoveEvent {
	ch := make(chan SliderMoveEvent)
	s.sliderMoveConsumers = append(s.sliderMoveConsumers, ch)
	return ch
}

func (s *SseIO) SubscribeToSwitchEvents() chan SwitchEvent {
	ch := make(chan SwitchEvent)
	s.switchConsumers = append(s.switchConsumers, ch)
	return ch
}

func (s *SseIO) setupOnConfigReload() {
	configReloadedChannel := s.deej.config.SubscribeToChanges()
	const stopDelay = 50 * time.Millisecond

	go func() {
		for {
			select {
			case <-configReloadedChannel:
				{
					// restart in case when config was changed.
					s.logger.Info("Detected changes in cofig, renew connection to retreive all values.")
					s.Stop()
					<-time.After(stopDelay)
					if err := s.Start(); err != nil {
						s.logger.Warnw("Failed to renew connection after parameter change", "error", err)
					} else {
						s.logger.Debug("Renewed connection successfully")
					}
				}
			}
		}
	}()
}

type sseStatePayload struct {
	ID    string `json:"id"`
	Value *int   `json:"value"` // optional in theory; ignore event if nil
}

var (
	potPattern = regexp.MustCompile(`^sensor-pot(\d+)$`)
	swPattern  = regexp.MustCompile(`^binary_sensor-sw(\d+)$`)
)

func (s *SseIO) handleStateEvent(logger *zap.SugaredLogger, data []byte) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}

	id, _ := raw["id"].(string)
	if id == "" {
		return
	}

	// ---- POTENTIOMETER
	if m := potPattern.FindStringSubmatch(id); len(m) == 2 {
		val, ok := raw["value"].(float64)
		if !ok {
			return
		}

		idx, _ := strconv.Atoi(m[1])
		n := float32(val) / 100.0
		if n < 0 {
			n = 0
		} else if n > 1 {
			n = 1
		}
		if s.deej.config.InvertSliders {
			n = 1 - n
		}

		move := SliderMoveEvent{
			SliderID:     idx,
			PercentValue: n,
		}

		if s.deej.Verbose() {
			logger.Debugw("Slider moved", "event", move)
		}

		for _, c := range s.sliderMoveConsumers {
			c <- move
		}
		return
	}

	// ---- SWITCH
	if m := swPattern.FindStringSubmatch(id); len(m) == 2 {
		var state bool
		if v, ok := raw["value"].(bool); ok {
			state = v
		} else if sStr, ok := raw["state"].(string); ok {
			state = strings.ToUpper(sStr) == "ON"
		} else {
			return
		}

		idx, _ := strconv.Atoi(m[1])
		sw := SwitchEvent{
			SwitchID: idx,
			State:    state,
		}

		if s.deej.Verbose() {
			logger.Debugw("Switch changed", "event", sw)
		}

		for _, c := range s.switchConsumers {
			c <- sw
		}
		return
	}
}
