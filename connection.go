package slippi

type ConnectionEvent struct {
	Type ConnectionEventType
	Payload interface{}
}
// ConnectionEventType enumerates the possible connection events emitted by a connection
type ConnectionEventType string

// ConnectionEvents
const (
	Connect ConnectionEventType = "connect"
	Message = "message"
	Handshake = "message"
	StatusChange = "statusChange"
	Data = "data"
	Error = "error"
)

// ConnectionStatus enumerates the possible states of a connection
type ConnectionStatus uint8

// ConnectionStatuses
const (
	Disconnected ConnectionStatus = iota
	Connecting
	Connected
	ReconnectWait
)

// Port enumerates the ports used
type Port uint16

// Ports
const (
	Default Port = 51441
	Legacy = 666
	RelayStart = 53741
)

type ConnectionDetails struct {
	ConsoleNick string
	GameDataCursor interface{}
	Version string
	ClientToken int
}

type ConnectionSettings struct {
	IpAddress string
	Port uint16
}

type Connection interface {
	GetStatus() ConnectionStatus
	GetSettings() ConnectionSettings
	GetDetails() ConnectionDetails
	Connect(ip string, port uint16) error
	Disconnect()
}
