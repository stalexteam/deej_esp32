package deej

import (
	"fmt"
	"net"
	"strconv"

	"github.com/jfreymuth/pulse/proto"
	"github.com/stalexteam/deej_esp32/pkg/deej/util"
	"go.uber.org/zap"
)

type paSessionFinder struct {
	logger        *zap.SugaredLogger
	sessionLogger *zap.SugaredLogger

	client *proto.Client
	conn   net.Conn
}

func newSessionFinder(logger *zap.SugaredLogger) (SessionFinder, error) {
	client, conn, err := proto.Connect("")
	if err != nil {
		logger.Warnw("Failed to establish PulseAudio connection", "error", err)
		return nil, fmt.Errorf("establish PulseAudio connection: %w", err)
	}

	request := proto.SetClientName{
		Props: proto.PropList{
			"application.name": proto.PropListString("deej"),
		},
	}
	reply := proto.SetClientNameReply{}

	if err := client.Request(&request, &reply); err != nil {
		return nil, err
	}

	sf := &paSessionFinder{
		logger:        logger.Named("session_finder"),
		sessionLogger: logger.Named("sessions"),
		client:        client,
		conn:          conn,
	}

	sf.logger.Debug("Created PA session finder instance")

	return sf, nil
}

func (sf *paSessionFinder) GetAllSessions() ([]Session, error) {
	sessions := []Session{}

	// get the master sink session
	masterSink, err := sf.getMasterSinkSession()
	if err == nil {
		sessions = append(sessions, masterSink)
	} else {
		sf.logger.Warnw("Failed to get master audio sink session", "error", err)
	}

	// get the master source session
	masterSource, err := sf.getMasterSourceSession()
	if err == nil {
		sessions = append(sessions, masterSource)
	} else {
		sf.logger.Warnw("Failed to get master audio source session", "error", err)
	}

	// enumerate sink inputs and add sessions along the way
	if err := sf.enumerateAndAddSessions(&sessions); err != nil {
		sf.logger.Warnw("Failed to enumerate audio sessions", "error", err)
		return nil, fmt.Errorf("enumerate audio sessions: %w", err)
	}

	return sessions, nil
}

func (sf *paSessionFinder) GetAllDevices() ([]AudioDeviceInfo, error) {
	devices := []AudioDeviceInfo{}

	// Get all sinks (output devices)
	sinkRequest := proto.GetSinkInfoList{}
	sinkReply := proto.GetSinkInfoListReply{}
	if err := sf.client.Request(&sinkRequest, &sinkReply); err == nil {
		for _, sink := range sinkReply {
			if sink == nil {
				continue
			}
			// GetSinkInfoReply has SinkName field, use it as name
			name := sink.SinkName
			if name == "" {
				name = fmt.Sprintf("Sink %d", sink.SinkIndex)
			}
			
			description := ""
			if sink.Properties != nil {
				if descProp, ok := sink.Properties["device.description"]; ok {
					description = descProp.String()
				}
			}
			
			devices = append(devices, AudioDeviceInfo{
				Name:        name,
				Type:        "Output",
				Description: description,
			})
		}
	}

	// Get all sources (input devices)
	sourceRequest := proto.GetSourceInfoList{}
	sourceReply := proto.GetSourceInfoListReply{}
	if err := sf.client.Request(&sourceRequest, &sourceReply); err == nil {
		for _, source := range sourceReply {
			if source == nil {
				continue
			}
			// Skip monitor sources (virtual)
			if source.MonitorSourceIndex != proto.Undefined {
				continue
			}
			
			name := source.SourceName
			if name == "" {
				name = fmt.Sprintf("Source %d", source.SourceIndex)
			}
			
			description := ""
			// get description if available
			if source.Properties != nil {
				if descProp, ok := source.Properties["device.description"]; ok {
					description = descProp.String()
				}
			}
			
			devices = append(devices, AudioDeviceInfo{
				Name:        name,
				Type:        "Input",
				Description: description,
			})
		}
	}

	return devices, nil
}

func (sf *paSessionFinder) Release() error {
	if err := sf.conn.Close(); err != nil {
		sf.logger.Warnw("Failed to close PulseAudio connection", "error", err)
		return fmt.Errorf("close PulseAudio connection: %w", err)
	}

	sf.logger.Debug("Released PA session finder instance")

	return nil
}

func (sf *paSessionFinder) getMasterSinkSession() (Session, error) {
	request := proto.GetSinkInfo{
		SinkIndex: proto.Undefined,
	}
	reply := proto.GetSinkInfoReply{}

	if err := sf.client.Request(&request, &reply); err != nil {
		sf.logger.Warnw("Failed to get master sink info", "error", err)
		return nil, fmt.Errorf("get master sink info: %w", err)
	}

	// create the master sink session
	sink := newMasterSession(sf.sessionLogger, sf.client, reply.SinkIndex, reply.Channels, true)

	return sink, nil
}

func (sf *paSessionFinder) getMasterSourceSession() (Session, error) {
	request := proto.GetSourceInfo{
		SourceIndex: proto.Undefined,
	}
	reply := proto.GetSourceInfoReply{}

	if err := sf.client.Request(&request, &reply); err != nil {
		sf.logger.Warnw("Failed to get master source info", "error", err)
		return nil, fmt.Errorf("get master source info: %w", err)
	}

	// create the master source session
	source := newMasterSession(sf.sessionLogger, sf.client, reply.SourceIndex, reply.Channels, false)

	return source, nil
}

func (sf *paSessionFinder) enumerateAndAddSessions(sessions *[]Session) error {
	request := proto.GetSinkInputInfoList{}
	reply := proto.GetSinkInputInfoListReply{}

	if err := sf.client.Request(&request, &reply); err != nil {
		sf.logger.Warnw("Failed to get sink input list", "error", err)
		return fmt.Errorf("get sink input list: %w", err)
	}

	for _, info := range reply {
		name, ok := info.Properties["application.process.binary"]

		if !ok {
			sf.logger.Warnw("Failed to get sink input's process name",
				"sinkInputIndex", info.SinkInputIndex)

			continue
		}

		// Try to get PID from PulseAudio properties
		var processPath string
		if pidProp, ok := info.Properties["application.process.id"]; ok {
			// PropListEntry has a String() method to get the value
			pidStr := pidProp.String()
			if pidStr != "" {
				if pid, err := strconv.Atoi(pidStr); err == nil {
					if path, err := util.GetProcessPath(pid); err == nil {
						processPath = path
					}
				}
			}
		}

		// create the deej session object
		newSession := newPASession(sf.sessionLogger, sf.client, info.SinkInputIndex, info.Channels, name.String(), processPath)

		// add it to our slice
		*sessions = append(*sessions, newSession)

	}

	return nil
}
