package deej

import (
	"strings"
	"sync"

	"go.uber.org/zap"
)

// Session represents a single addressable audio session
type Session interface {
	GetVolume() float32
	SetVolume(v float32) error

	GetMute() bool
	SetMute(v bool, silent bool) error

	GetSwitchMuteCount() int
	SetSwitchMuteCount(count int)
	AdjustSwitchMuteCount(delta int) int

	Key() string
	ProcessPath() string
	Release()
}

const (

	// ideally these would share a common ground in baseSession
	// but it will not call the child GetVolume correctly :/
	sessionCreationLogMessage = "Created audio session instance"

	// format this with s.humanReadableDesc and whatever the current volume is
	sessionStringFormat = "<session: %s, vol: %.2f>"
)

type baseSession struct {
	logger *zap.SugaredLogger
	system bool
	master bool

	// used by Key(), needs to be set by child
	name string

	// used by String(), needs to be set by child
	humanReadableDesc string

	switchMuteLock  sync.Mutex
	switchMuteCount int
}

func (s *baseSession) Key() string {
	if s.system {
		return systemSessionName
	}

	if s.master {
		return strings.ToLower(s.name) // could be master or mic, or any device's friendly name
	}

	return strings.ToLower(s.name)
}

func (s *baseSession) GetSwitchMuteCount() int {
	s.switchMuteLock.Lock()
	defer s.switchMuteLock.Unlock()
	return s.switchMuteCount
}

func (s *baseSession) SetSwitchMuteCount(count int) {
	if count < 0 {
		count = 0
	}
	s.switchMuteLock.Lock()
	s.switchMuteCount = count
	s.switchMuteLock.Unlock()
}

func (s *baseSession) AdjustSwitchMuteCount(delta int) int {
	s.switchMuteLock.Lock()
	s.switchMuteCount += delta
	if s.switchMuteCount < 0 {
		s.switchMuteCount = 0
	}
	current := s.switchMuteCount
	s.switchMuteLock.Unlock()
	return current
}
