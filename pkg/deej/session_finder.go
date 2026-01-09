package deej

// AudioDeviceInfo represents information about an audio device
type AudioDeviceInfo struct {
	Name        string // Friendly name of the device
	Type        string // "Output" or "Input"
	Description string // Device description (optional, may be empty)
}

// SessionFinder represents an entity that can find all current audio sessions
type SessionFinder interface {
	GetAllSessions() ([]Session, error)
	GetAllDevices() ([]AudioDeviceInfo, error) // Get list of all available audio devices

	Release() error
}
