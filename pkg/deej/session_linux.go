package deej

import (
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/jfreymuth/pulse/proto"
)

// normal PulseAudio volume (100%)
const maxVolume = 0x10000

var errNoSuchProcess = errors.New("No such process")

type paSession struct {
	baseSession

	processName string

	client *proto.Client

	sinkInputIndex    uint32
	sinkInputChannels byte
}

type masterSession struct {
	baseSession

	client *proto.Client

	streamIndex    uint32
	streamChannels byte
	isOutput       bool
}

func newPASession(
	logger *zap.SugaredLogger,
	client *proto.Client,
	sinkInputIndex uint32,
	sinkInputChannels byte,
	processName string,
) *paSession {

	s := &paSession{
		client:            client,
		sinkInputIndex:    sinkInputIndex,
		sinkInputChannels: sinkInputChannels,
	}

	s.processName = processName
	s.name = processName
	s.humanReadableDesc = processName

	// use a self-identifying session name e.g. deej.sessions.chrome
	s.logger = logger.Named(s.Key())
	s.logger.Debugw(sessionCreationLogMessage, "session", s)

	return s
}

func newMasterSession(
	logger *zap.SugaredLogger,
	client *proto.Client,
	streamIndex uint32,
	streamChannels byte,
	isOutput bool,
) *masterSession {

	s := &masterSession{
		client:         client,
		streamIndex:    streamIndex,
		streamChannels: streamChannels,
		isOutput:       isOutput,
	}

	var key string

	if isOutput {
		key = masterSessionName
	} else {
		key = inputSessionName
	}

	s.logger = logger.Named(key)
	s.master = true
	s.name = key
	s.humanReadableDesc = key

	s.logger.Debugw(sessionCreationLogMessage, "session", s)

	return s
}

func (s *paSession) GetVolume() float32 {
	request := proto.GetSinkInputInfo{
		SinkInputIndex: s.sinkInputIndex,
	}
	reply := proto.GetSinkInputInfoReply{}

	if err := s.client.Request(&request, &reply); err != nil {
		s.logger.Warnw("Failed to get session volume", "error", err)
	}

	return parseChannelVolumes(reply.ChannelVolumes)
}

func (s *paSession) SetVolume(v float32) error {
	var WasMuted bool = s.GetMute()

	volumes := createChannelVolumes(s.sinkInputChannels, v)
	request := proto.SetSinkInputVolume{
		SinkInputIndex: s.sinkInputIndex,
		ChannelVolumes: volumes,
	}

	if err := s.client.Request(&request, nil); err != nil {
		s.logger.Warnw("Failed to set session volume", "error", err)
		return fmt.Errorf("adjust session volume: %w", err)
	}

	if WasMuted {
		s.SetMute(true, true)
	}

	s.logger.Debugw("Adjusting session volume", "to", fmt.Sprintf("%.2f", v))
	return nil
}

func (s *paSession) GetMute() bool {
	request := proto.GetSinkInputInfo{
		SinkInputIndex: s.sinkInputIndex,
	}
	reply := proto.GetSinkInputInfoReply{}

	if err := s.client.Request(&request, &reply); err != nil {
		s.logger.Warnw("Failed to get mute state", "error", err)
		return false
	}
	return reply.Muted
}

func (s *paSession) SetMute(v bool, silent bool) error {
	request := proto.SetSinkInputMute{
		SinkInputIndex: s.sinkInputIndex,
		Mute:           v,
	}
	if err := s.client.Request(&request, nil); err != nil {
		s.logger.Warnw("Failed to set mute state", "error", err)
		return fmt.Errorf("set mute: %w", err)
	}

	if !silent {
		s.logger.Debugw("Setting session mute state", "muted", v)
	}
	return nil
}

func (s *paSession) Release() {
	s.logger.Debug("Releasing audio session")
}

func (s *paSession) String() string {
	return fmt.Sprintf(sessionStringFormat, s.humanReadableDesc, s.GetVolume())
}

func (s *masterSession) GetVolume() float32 {
	var level float32

	if s.isOutput {
		request := proto.GetSinkInfo{
			SinkIndex: s.streamIndex,
		}
		reply := proto.GetSinkInfoReply{}

		if err := s.client.Request(&request, &reply); err != nil {
			s.logger.Warnw("Failed to get session volume", "error", err)
			return 0
		}

		level = parseChannelVolumes(reply.ChannelVolumes)
	} else {
		request := proto.GetSourceInfo{
			SourceIndex: s.streamIndex,
		}
		reply := proto.GetSourceInfoReply{}

		if err := s.client.Request(&request, &reply); err != nil {
			s.logger.Warnw("Failed to get session volume", "error", err)
			return 0
		}

		level = parseChannelVolumes(reply.ChannelVolumes)
	}

	return level
}

func (s *masterSession) SetVolume(v float32) error {
	var WasMuted bool = s.GetMute()

	volumes := createChannelVolumes(s.streamChannels, v)
	var request proto.RequestArgs

	if s.isOutput {
		request = &proto.SetSinkVolume{
			SinkIndex:      s.streamIndex,
			ChannelVolumes: volumes,
		}
	} else {
		request = &proto.SetSourceVolume{
			SourceIndex:    s.streamIndex,
			ChannelVolumes: volumes,
		}
	}

	if err := s.client.Request(request, nil); err != nil {
		s.logger.Warnw("Failed to set master volume", "error", err, "volume", v)
		return fmt.Errorf("adjust session volume: %w", err)
	}

	if WasMuted {
		s.SetMute(true, true)
	}

	s.logger.Debugw("Adjusting master volume", "to", fmt.Sprintf("%.2f", v))
	return nil
}

func (s *masterSession) GetMute() bool {
	if s.isOutput {
		request := proto.GetSinkInfo{SinkIndex: s.streamIndex}
		reply := proto.GetSinkInfoReply{}
		if err := s.client.Request(&request, &reply); err != nil {
			s.logger.Warnw("Failed to get mute state", "error", err)
			return false
		}
		return reply.Muted
	}

	request := proto.GetSourceInfo{SourceIndex: s.streamIndex}
	reply := proto.GetSourceInfoReply{}
	if err := s.client.Request(&request, &reply); err != nil {
		s.logger.Warnw("Failed to get mute state", "error", err)
		return false
	}
	return reply.Muted
}

func (s *masterSession) SetMute(v bool, silent bool) error {
	var request proto.RequestArgs
	if s.isOutput {
		request = &proto.SetSinkMute{
			SinkIndex: s.streamIndex,
			Mute:      v,
		}
	} else {
		request = &proto.SetSourceMute{
			SourceIndex: s.streamIndex,
			Mute:        v,
		}
	}

	if err := s.client.Request(request, nil); err != nil {
		s.logger.Warnw("Failed to set mute", "error", err)
		return fmt.Errorf("set mute: %w", err)
	}

	if !silent {
		s.logger.Debugw("Setting master mute state", "muted", v)
	}
	return nil
}

func (s *masterSession) Release() {
	s.logger.Debug("Releasing audio session")
}

func (s *masterSession) String() string {
	return fmt.Sprintf(sessionStringFormat, s.humanReadableDesc, s.GetVolume())
}

func createChannelVolumes(channels byte, volume float32) []uint32 {
	volumes := make([]uint32, channels)

	for i := range volumes {
		volumes[i] = uint32(volume * maxVolume)
	}

	return volumes
}

func parseChannelVolumes(volumes []uint32) float32 {
	var level uint32

	for _, volume := range volumes {
		level += volume
	}

	return float32(level) / float32(len(volumes)) / float32(maxVolume)
}
