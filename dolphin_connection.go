package slippi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/haormj/enet-go"
)

const MaxPeers = 32

// DolphinMessageType enumerates the types of messages Dolphin sends to a client.
type DolphinMessageType string

// DolphinMessageTypes
const (
	ConnectReply   DolphinMessageType = "connect_reply"
	ConnectRequest                    = "connect_request"
	MenuEvent                         = "menu_event"
	GameEvent                         = "game_event"
	StartGame                         = "start_game"
	EndGame                           = "end_game"
)

// DolphinConnection represents a connection to an instance of Dolphin.
type DolphinConnection struct {
	IpAddress        string
	Port             uint16
	ConnectionStatus ConnectionStatus
	GameCursor       int
	Nickname         string
	Version          string
	Peer             enet.ENetPeer
	send             chan<- *ConnectionEvent
}

// DolphinMessage represents a message sent from Dolphin to a client.
type DolphinMessage struct {
	Type       DolphinMessageType `json:"type"`
	Nick       string             `json:"nick,omitempty"`
	Version    string             `json:"version,omitempty"`
	Cursor     int                `json:"cursor"`
	NextCursor int                `json:"next_cursor,omitempty"`
	Payload    string             `json:"payload,omitempty"`
}

// NewDolphinConnection returns a new DolphinConnection instance.
func NewDolphinConnection() *DolphinConnection {
	return &DolphinConnection{
		IpAddress:        "",
		Port:             0,
		ConnectionStatus: Disconnected,
		GameCursor:       0,
		Nickname:         "",
		Version:          "",
		Peer:             nil,
		send:             nil,
	}
}

// GetStatus gets the current state of the DolphinConnection.
func (c *DolphinConnection) GetStatus() ConnectionStatus {
	return c.ConnectionStatus
}

// GetSettings gets the current settings of the DolphinConnection.
func (c *DolphinConnection) GetSettings() ConnectionSettings {
	return ConnectionSettings{
		IpAddress: c.IpAddress,
		Port:      c.Port,
	}
}

// GetDetails gets the current details of the DolphinConnection.
func (c *DolphinConnection) GetDetails() ConnectionDetails {
	return ConnectionDetails{
		ConsoleNick:    c.Nickname,
		GameDataCursor: c.GameCursor,
		Version:        c.Version,
	}
}

// Connect connects to a Dolphin instance on the given IP and port.
func (c *DolphinConnection) Connect(ip string, port uint16) (<-chan *ConnectionEvent, error) {
	var receive <-chan *ConnectionEvent

	c.IpAddress = ip
	c.Port = port
	c.send, receive = MakeUnboundedChannel[ConnectionEvent]()

	if enet.Enet_initialize() != 0 {
		return nil, errors.New("failed to initialize enet")
	}
	defer enet.Enet_initialize()
	serverAddress := enet.NewENetAddress()
	enet.Enet_address_set_host(serverAddress, ip)
	serverAddress.SetPort(enet.NewEnetUint16(port))
	client := enet.Enet_host_create(nil, MaxPeers, 3, enet.NewEnetUint32(0), enet.NewEnetUint32(0))
	if client == nil {
		return nil, errors.New("failed to create enet client")
	}
	c.Peer = enet.Enet_host_connect(client, serverAddress, 3, enet.NewEnetUint32(1337))
	if c.Peer == nil {
		return nil, errors.New("failed to connect to server")
	}

	enet.Enet_peer_ping(c.Peer)
	c.send <- &ConnectionEvent{
		Type:    Connect,
		Payload: nil,
	}
	c.setStatus(Connected)

	// Decide how to handle errors in goroutine
	go func() {
		event := enet.NewENetEvent()
		for {
			if enet.Enet_host_service(client, event, enet.NewEnetUint32(1000)) > 0 {
				switch event.GetXtype() {
				case enet.ENET_EVENT_TYPE_NONE:
				case enet.ENET_EVENT_TYPE_CONNECT:
					c.GameCursor = 0

					request := DolphinMessage{
						Type:   ConnectRequest,
						Cursor: c.GameCursor,
					}

					bytes, err := json.Marshal(request)
					if err != nil {
						c.send <- &ConnectionEvent{
							Type:    Error,
							Payload: errors.New("failed to marshal connect request data"),
						}
					}

					packet := enet.NewENetPacket()
					dataPtr, dataLength := enet.BytesToUintptr(bytes)
					packet.SetData(enet.SwigcptrEnet_uint8(dataPtr))
					packet.SetDataLength(int64(dataLength))

					flags := []uint32{uint32(enet.ENET_PACKET_FLAG_RELIABLE)}
					flagsPtr, _ := enet.Uint32BytesToUintptr(flags)
					packet.SetFlags(enet.SwigcptrEnet_uint32(flagsPtr))

					if packet == nil {
						c.send <- &ConnectionEvent{
							Type:    Error,
							Payload: errors.New("failed to create connect request packet"),
						}
					}

					if ret := enet.Enet_peer_send(c.Peer, enet.NewEnetUint8(0), packet); ret != 0 {
						c.send <- &ConnectionEvent{
							Type:    Error,
							Payload: errors.New("failed to send connect request packet"),
						}
					}

					enet.DeleteENetPacket(packet)
				case enet.ENET_EVENT_TYPE_RECEIVE:
					packet := event.GetPacket()
					dataLength := int(packet.GetDataLength())
					if packet.GetDataLength() == 0 {
						continue
					}

					data := enet.UintptrToBytes(packet.GetData().Swigcptr(), dataLength)
					var message DolphinMessage
					err := json.Unmarshal(data, &message)
					if err != nil {
						c.send <- &ConnectionEvent{
							Type:    Error,
							Payload: err,
						}
					}

					c.send <- &ConnectionEvent{
						Type:    Message,
						Payload: message,
					}

					switch message.Type {
					case ConnectReply:
						c.setStatus(Connected)
						c.GameCursor = message.Cursor
						c.Nickname = message.Nick
						c.Version = message.Version
						c.send <- &ConnectionEvent{
							Type:    Handshake,
							Payload: c.GetDetails(),
						}
					case MenuEvent:
						fallthrough
					case GameEvent:
						payload := message.Payload
						if payload == "" {
							c.Disconnect()
							continue
						}
						c.updateCursor(message)

						gameData, err := base64.StdEncoding.DecodeString(payload)
						if err != nil {
							c.send <- &ConnectionEvent{
								Type:    Error,
								Payload: err,
							}
						}

						c.send <- &ConnectionEvent{
							Type:    Data,
							Payload: gameData,
						}
					case StartGame:
						c.updateCursor(message)
					case EndGame:
						c.updateCursor(message)
					}
				case enet.ENET_EVENT_TYPE_DISCONNECT:
					c.Disconnect()
				}
			}
		}
	}()

	c.setStatus(Connecting)

	return receive, nil
}

func (c *DolphinConnection) Disconnect() {
	if c.Peer != nil {
		enet.Enet_peer_disconnect(c.Peer, enet.NewEnetUint32(0))
		c.Peer = nil
	}
	c.setStatus(Disconnected)
}

func (c *DolphinConnection) setStatus(status ConnectionStatus) {
	if c.ConnectionStatus != status {
		c.ConnectionStatus = status
		c.send <- &ConnectionEvent{
			Type:    StatusChange,
			Payload: c.ConnectionStatus,
		}
	}
}

func (c *DolphinConnection) updateCursor(message DolphinMessage) {
	if c.GameCursor != message.Cursor {
		c.send <- &ConnectionEvent{
			Type:    Error,
			Payload: errors.New("unexpected game data cursor"),
		}
	}

	c.GameCursor = message.NextCursor
}
