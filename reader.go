package slippi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/blang/semver/v4"
	"github.com/jmank88/ubjson"
	"golang.org/x/text/encoding/japanese"
)

// InputType enumerates possible slp data sources.
type InputType int

// InputTypes
const (
	SlpFile InputType = iota
	SlpBytes
)

// A SlpSource wraps a reader and the type of the reader, used to determine its
// length.
type SlpSource struct {
	io.ReadSeeker
	InputType InputType
	length    int64
}

// NewSlpSourceFile returns a SlpSource wrapping the provided *os.File f.
func NewSlpSourceFile(f *os.File) *SlpSource {
	return &SlpSource{
		ReadSeeker: f,
		InputType:  SlpFile,
		length:     -1,
	}
}

// NewSlpSourceBytes returns a SlpSource wrapping the provided *bytes.Reader r.
func NewSlpSourceBytes(r *bytes.Reader) *SlpSource {
	return &SlpSource{
		ReadSeeker: r,
		InputType:  SlpBytes,
		length:     -1,
	}
}

// GetLength gets the length of the underlying data source of the SlpSource.
// If recalculate is true, the length will be recalculated. Otherwise, the
// length is only calculated on the first call to GetLength for a given
// SlpSource. If there is an error getting the length of the data source,
// a length of 0 is returned along with the error.
func (s *SlpSource) GetLength(recalculate bool) (int64, error) {
	if recalculate || s.length == -1 {
		switch s.InputType {
		case SlpFile:
			// cast reader to os.File
			var f *os.File
			f, ok := s.ReadSeeker.(*os.File)
			if !ok {
				s.length = -1
				return s.length, errors.New("failed to cast SlpFile data source to os.File")
			}

			// get length
			stat, err := f.Stat()
			if err != nil {
				s.length = -1
				return s.length, err
			}
			s.length = stat.Size()
		case SlpBytes:
			// cast reader to bytes.Buffer
			b, ok := s.ReadSeeker.(*bytes.Reader)
			if !ok {
				s.length = -1
				return s.length, errors.New("failed to cast SlpBytes data source to bytes.Buffer")
			}

			// get length
			s.length = int64(b.Len())
		default:
			s.length = -1
			return s.length, errors.New(fmt.Sprintf("unrecognized slp input type: %d", s.InputType))
		}
	}

	return s.length, nil
}

// A SlpReader reads data from Source and emits event payloads and metadata.
type SlpReader struct {
	Source         SlpSource
	include        map[byte]bool
	RawStart       int64
	RawLength      int64
	MetadataStart  int64
	MetadataLength int64
	PayloadSizes   map[byte]uint16
}

// NewSlpReader returns a SlpReader that reads from the provided SlpSource s.
func NewSlpReader(s SlpSource) (*SlpReader, error) {
	// get length
	length, err := s.GetLength(false)
	if err != nil {
		return nil, errors.New("failed to get length of replay data source")
	}

	// read preamble
	preamble := make([]byte, 15)
	_, err = s.Read(preamble)
	if err != nil {
		return nil, err
	}

	// verify preamble contents
	if bytes.Compare(preamble[:11], []byte{0x7B, 0x55, 0x03, 0x72, 0x61, 0x77, 0x5B, 0x24, 0x55, 0x23, 0x6c}) != 0 {
		return nil, errors.New(fmt.Sprintf("replay had an invalid preamble: %X\n", preamble[:11]))
	}

	// get raw data start and length
	var rawStart int64 = 15
	rawLength := int64(binary.BigEndian.Uint32(preamble[11:]))

	// calculate metadata start and length
	metadataStart := rawStart + rawLength + 10
	metadataLength := length - metadataStart - 1

	// read first 2 bytes of event payloads event
	eventPayloads := make([]byte, 2)
	_, err = s.Read(eventPayloads)
	if err != nil {
		return nil, err
	}

	// verify that the first event is an event payloads event
	if eventPayloads[0] != 0x35 {
		return nil, errors.New(fmt.Sprintf("expected event payloads event, got: %X\n", eventPayloads[0]))
	}

	// populate event payload sizes
	payloadSizes := make(map[byte]uint16)
	payloadSizes[eventPayloads[0]] = uint16(eventPayloads[1])
	payloadsLength := int(eventPayloads[1])
	payloadsBytesRead := 1
	eventInfo := make([]byte, 3)
	for payloadsBytesRead < payloadsLength {
		bytesRead, err := s.Read(eventInfo)
		if err != nil {
			return nil, err
		}

		payloadsBytesRead += bytesRead

		payloadSizes[eventInfo[0]] = binary.BigEndian.Uint16(eventInfo[1:])
	}

	include := make(map[byte]bool)

	include[0x10] = true

	for i := byte(0x35); i < 0x3E; i++ {
		include[i] = true
	}

	return &SlpReader{
		Source:         s,
		include:        include,
		RawStart:       rawStart,
		RawLength:      rawLength,
		MetadataStart:  metadataStart,
		MetadataLength: metadataLength,
		PayloadSizes:   payloadSizes,
	}, nil
}

// SetInclude sets whether a given event (as specified by its command byte) will
// be read and emitted when YieldEvents is called on the SlpReader.
func (r *SlpReader) SetInclude(command byte, include bool) error {
	// reject unknown commands
	if command != 0x10 && (command < 0x35 || command > 0x3D) {
		return errors.New(fmt.Sprintf("unknown command: 0x%X", command))
	}

	r.include[command] = include
	return nil
}

type SlpEventResult struct {
	Event *SlpEvent
	Error error
}

// YieldEvents returns a channel to which it sends the events from the
// SlpSource.
func (r *SlpReader) YieldEvents(stopYielding func(*SlpEvent) bool) (<-chan *SlpEventResult, error) {
	// reset to start of raw data
	_, err := r.Source.Seek(r.RawStart, io.SeekStart)
	if err != nil {
		return nil, errors.New("failed to seek to start of replay")
	}

	send, receive := MakeUnboundedChannel[SlpEventResult]()

	// construct buffers for payloads
	payloadBuffers := make(map[byte][]byte)
	for event, payloadSize := range r.PayloadSizes {
		payloadBuffers[event] = make([]byte, payloadSize)
	}

	go func() {
		position := r.RawStart
		end := r.RawStart + r.RawLength - 1
		commandBuf := make([]byte, 1)
		for position < end {
			// read event byte
			bytesRead, err := r.Source.Read(commandBuf)
			if err != nil {
				send <- &SlpEventResult{
					Event: nil,
					Error: err,
				}
				close(send)
				return
			}
			position += int64(bytesRead)

			command := commandBuf[0]

			// ensure event payload size is known
			payload, ok := payloadBuffers[command]
			if !ok {
				send <- &SlpEventResult{
					Event: nil,
					Error: err,
				}
				close(send)
				return
			}

			include, ok := r.include[command]

			// skip events that are unknown or not included
			if !ok || !include {
				_, err = r.Source.Seek(int64(len(payload)), io.SeekCurrent)
				if err != nil {
					send <- &SlpEventResult{
						Event: nil,
						Error: err,
					}
					close(send)
					return
				}
				continue
			}

			// read event payload
			bytesRead, err = r.Source.Read(payload)
			if err != nil {
				send <- &SlpEventResult{
					Event: nil,
					Error: err,
				}
				close(send)
				return
			}
			position += int64(bytesRead)

			cmd := Command(command)
			event, err := parsePayload(cmd, payload)
			if err != nil {
				send <- &SlpEventResult{
					Event: nil,
					Error: err,
				}
				close(send)
				return
			}

			send <- &SlpEventResult{
				Event: event,
				Error: nil,
			}

			if stopYielding(event) {
				close(send)
				return
			}
		}

		close(send)
	}()

	return receive, nil
}

// See https://github.com/project-slippi/slippi-wiki/blob/master/SPEC.md
func parsePayload(command Command, payloadBytes []byte) (*SlpEvent, error) {
	var payload interface{}
	switch command {
	case MessageSplitter:
		payload = MessageSplitterPayload{
			Data:            *(*[512]uint8)(payloadBytes[0x0:0x200]),
			DataLength:      binary.BigEndian.Uint16(payloadBytes[0x200:0x202]),
			InternalCommand: payloadBytes[0x202],
			LastMessage:     payloadBytes[0x203] != 0,
		}
	case EventPayloads:
		payloadsLength := payloadBytes[0]
		payloadSizes := make(map[uint8]uint16)
		for position := byte(1); position < payloadsLength; position += 3 {
			payloadSizes[payloadBytes[position]] = binary.BigEndian.Uint16(payloadBytes[position+1 : position+3])
		}

		payload = EventPayloadsPayload{
			PayloadSize:  payloadsLength,
			PayloadSizes: payloadSizes,
		}
	case GameStart:
		getPlayerData := func(playerIndex int) (*PlayerInfo, error) {
			nametagOffset := 0x10 * playerIndex
			nametag, err := decodeShiftJIS(payloadBytes[0x160+nametagOffset : 0x170+nametagOffset])
			if err != nil {
				return nil, err
			}

			displayNameOffset := 0x1F * playerIndex
			displayName, err := decodeShiftJIS(payloadBytes[0x1A4+displayNameOffset : 0x1C3+displayNameOffset])
			if err != nil {
				return nil, err
			}

			connectCodeOffset := 0xA * playerIndex
			connectCode, err := decodeShiftJIS(payloadBytes[0x220+connectCodeOffset : 0x22B+connectCodeOffset])
			if err != nil {
				return nil, err
			}

			gameInfoOffset := 0x24 * playerIndex
			slippiUIDOffset := 0x1D * playerIndex
			fixOffset := 0x8 * playerIndex

			return &PlayerInfo{
				Index:           0,
				Port:            1,
				CharacterID:     payloadBytes[0x64+gameInfoOffset],
				PlayerType:      PlayerType(payloadBytes[0x65+gameInfoOffset]),
				StockStartCount: payloadBytes[0x66+gameInfoOffset],
				CostumeIndex:    payloadBytes[0x67+gameInfoOffset],
				TeamShade:       TeamShade(payloadBytes[0x6B+gameInfoOffset]),
				Handicap:        payloadBytes[0x6C+gameInfoOffset],
				TeamID:          TeamID(payloadBytes[0x6D+gameInfoOffset]),
				PlayerBitfield:  payloadBytes[0x70+gameInfoOffset],
				CPULevel:        payloadBytes[0x73+gameInfoOffset],
				OffenseRatio:    readFloat(payloadBytes[0x7C+gameInfoOffset : 0x80+gameInfoOffset]),
				DefenseRatio:    readFloat(payloadBytes[0x80+gameInfoOffset : 0x84+gameInfoOffset]),
				ModelScale:      readFloat(payloadBytes[0x84+gameInfoOffset : 0x88+gameInfoOffset]),
				DashbackFix:     DashbackFix(binary.BigEndian.Uint32(payloadBytes[0x140+fixOffset : 0x144+fixOffset])),
				ShieldDropFix:   ShieldDropFix(binary.BigEndian.Uint32(payloadBytes[0x144+fixOffset : 0x148+fixOffset])),
				Nametag:         nametag,
				DisplayName:     displayName,
				ConnectCode:     connectCode,
				SlippiUID:       string(nullTerminate(payloadBytes[0x248+slippiUIDOffset : 0x265+slippiUIDOffset])),
			}, nil
		}

		var players [4]PlayerInfo
		for i := 0; i < 4; i++ {
			playerInfo, err := getPlayerData(i)
			if err != nil {
				return nil, err
			}

			players[i] = *playerInfo
		}

		version := semver.Version{
			Major: uint64(payloadBytes[0]),
			Minor: uint64(payloadBytes[1]),
			Patch: uint64(payloadBytes[2]),
		}

		payload = GameStartPayload{
			Version: version,
			GameInfoBlock: GameInfoBlock{
				GameBitfield1:          payloadBytes[0x4],
				GameBitfield2:          payloadBytes[0x5],
				GameBitfield3:          payloadBytes[0x6],
				GameBitfield4:          payloadBytes[0x7],
				BombRain:               payloadBytes[0xA],
				IsTeams:                payloadBytes[0xC] != 0,
				ItemSpawnBehavior:      ItemSpawnBehavior(payloadBytes[0xF]),
				SelfDestructScoreValue: int8(payloadBytes[0x10]),
				Stage:                  binary.BigEndian.Uint16(payloadBytes[0x12:0x14]),
				GameTimer:              binary.BigEndian.Uint32(payloadBytes[0x14:0x18]),
				ItemSpawnBitfield1:     payloadBytes[0x27],
				ItemSpawnBitfield2:     payloadBytes[0x28],
				ItemSpawnBitfield3:     payloadBytes[0x29],
				ItemSpawnBitfield4:     payloadBytes[0x2A],
				ItemSpawnBitfield5:     payloadBytes[0x2B],
				DamageRatio:            readFloat(payloadBytes[0x34:0x38]),
			},
			Players:        players,
			RandomSeed:     binary.BigEndian.Uint32(payloadBytes[0x13C:0x140]),
			PAL:            payloadBytes[0x1A0] != 0,
			FrozenPS:       payloadBytes[0x1A1] != 0,
			MinorScene:     payloadBytes[0x1A2],
			MajorScene:     payloadBytes[0x1A3],
			LanguageOption: Language(payloadBytes[0x2BC]),
		}
	case PreFrameUpdate:
		frameNumber, err := readInt(payloadBytes[0x0:0x4])
		if err != nil {
			return nil, err
		}

		payload = PreFrameUpdatePayload{
			FrameUpdate: FrameUpdate{
				FrameNumber:     frameNumber,
				PlayerIndex:     payloadBytes[0x4],
				IsFollower:      payloadBytes[0x5] != 0,
				ActionStateID:   binary.BigEndian.Uint16(payloadBytes[0xA:0xC]),
				XPosition:       readFloat(payloadBytes[0xC:0x10]),
				YPosition:       readFloat(payloadBytes[0x10:0x14]),
				FacingDirection: readFloat(payloadBytes[0x14:0x18]),
				Percent:         readFloat(payloadBytes[0x3B:0x3F]),
			},
			RandomSeed:       binary.BigEndian.Uint32(payloadBytes[0x6:0xA]),
			JoystickX:        readFloat(payloadBytes[0x18:0x1C]),
			JoystickY:        readFloat(payloadBytes[0x1C:0x20]),
			CStickX:          readFloat(payloadBytes[0x20:0x24]),
			CStickY:          readFloat(payloadBytes[0x24:0x28]),
			Trigger:          readFloat(payloadBytes[0x28:0x2C]),
			ProcessedButtons: binary.BigEndian.Uint32(payloadBytes[0x2C:0x30]),
			PhysicalButtons:  binary.BigEndian.Uint16(payloadBytes[0x30:0x32]),
			PhysicalLTrigger: readFloat(payloadBytes[0x32:0x36]),
			PhysicalRTrigger: readFloat(payloadBytes[0x36:0x3A]),
			XAnalogUCF:       payloadBytes[0x3A],
		}
	case PostFrameUpdate:
		frameNumber, err := readInt(payloadBytes[0x0:0x4])
		if err != nil {
			return nil, err
		}

		payload = PostFrameUpdatePayload{
			FrameUpdate: FrameUpdate{
				FrameNumber:     frameNumber,
				PlayerIndex:     payloadBytes[0x4],
				IsFollower:      payloadBytes[0x5] != 0,
				ActionStateID:   binary.BigEndian.Uint16(payloadBytes[0x7:0x9]),
				XPosition:       readFloat(payloadBytes[0x9:0xD]),
				YPosition:       readFloat(payloadBytes[0xD:0x11]),
				FacingDirection: readFloat(payloadBytes[0x11:0x15]),
				Percent:         readFloat(payloadBytes[0x15:0x19]),
			},
			InternalCharacterID:     payloadBytes[0x6],
			ShieldSize:              readFloat(payloadBytes[0x19:0x1D]),
			LastHittingAttackID:     payloadBytes[0x1D],
			CurrentComboCount:       payloadBytes[0x1E],
			LastHitBy:               payloadBytes[0x1F],
			StocksRemaining:         payloadBytes[0x20],
			ActionStateFrameCounter: readFloat(payloadBytes[0x21:0x25]),
			StateBitFlags1:          payloadBytes[0x25],
			StateBitFlags2:          payloadBytes[0x26],
			StateBitFlags3:          payloadBytes[0x27],
			StateBitFlags4:          payloadBytes[0x28],
			StateBitFlags5:          payloadBytes[0x29],
			MiscAS:                  readFloat(payloadBytes[0x2A:0x2E]),
			Airborne:                payloadBytes[0x2E] != 0,
			LastGroundID:            binary.BigEndian.Uint16(payloadBytes[0x2F:0x31]),
			JumpsRemaining:          payloadBytes[0x31],
			LCancelStatus:           LCancelStatus(payloadBytes[0x32]),
			HurtboxCollisionState:   HurtboxCollisionState(payloadBytes[0x33]),
			SelfInducedAirXSpeed:    readFloat(payloadBytes[0x34:0x38]),
			SelfInducedYSpeed:       readFloat(payloadBytes[0x38:0x3C]),
			AttackBasedXSpeed:       readFloat(payloadBytes[0x3C:0x40]),
			AttackBasedYSpeed:       readFloat(payloadBytes[0x40:0x44]),
			SelfInducedGroundXSpeed: readFloat(payloadBytes[0x44:0x48]),
			HitlagFramesRemaining:   readFloat(payloadBytes[0x48:0x4C]),
			AnimationIndex:          binary.BigEndian.Uint32(payloadBytes[0x4C:0x50]),
		}
	case GameEnd:
		payload = GameEndPayload{
			GameEndMethod: GameEndMethod(payloadBytes[0x0]),
			LRASInitiator: int8(payloadBytes[0x1]),
		}
	case FrameStart:
		frameNumber, err := readInt(payloadBytes[0x0:0x4])
		if err != nil {
			return nil, err
		}

		payload = FrameStartPayload{
			FrameNumber:       frameNumber,
			RandomSeed:        binary.BigEndian.Uint32(payloadBytes[0x4:0x8]),
			SceneFrameCounter: binary.BigEndian.Uint32(payloadBytes[0x8:0xC]),
		}
	case ItemUpdate:
		frameNumber, err := readInt(payloadBytes[0x0:0x4])
		if err != nil {
			return nil, err
		}

		payload = ItemUpdatePayload{
			FrameNumber:      frameNumber,
			TypeID:           binary.BigEndian.Uint16(payloadBytes[0x4:0x6]),
			State:            payloadBytes[0x6],
			FacingDirection:  readFloat(payloadBytes[0x7:0xB]),
			XVelocity:        readFloat(payloadBytes[0xB:0xF]),
			YVelocity:        readFloat(payloadBytes[0xF:0x13]),
			XPosition:        readFloat(payloadBytes[0x13:0x17]),
			YPosition:        readFloat(payloadBytes[0x17:0x1B]),
			DamageTaken:      binary.BigEndian.Uint16(payloadBytes[0x1B:0x1D]),
			ExpirationTimer:  readFloat(payloadBytes[0x1D:0x21]),
			SpawnID:          binary.BigEndian.Uint32(payloadBytes[0x21:0x25]),
			SamusMissileType: payloadBytes[0x25],
			PeachTurnipFace:  payloadBytes[0x26],
			IsLaunched:       payloadBytes[0x27],
			ChargedPower:     payloadBytes[0x28],
			Owner:            int8(payloadBytes[0x29]),
		}
	case FrameBookend:
		frameNumber, err := readInt(payloadBytes[0x0:0x4])
		if err != nil {
			return nil, err
		}

		latestFinalizedFrame, err := readInt(payloadBytes[0x4:0x8])
		if err != nil {
			return nil, err
		}

		payload = FrameBookendPayload{
			FrameNumber:          frameNumber,
			LatestFinalizedFrame: latestFinalizedFrame,
		}
	case GeckoList:
		payload = GeckoListPayload{GeckoCodes: payloadBytes}
	default:
		return nil, errors.New(fmt.Sprintf("unknown command: 0x%X", command))
	}

	return &SlpEvent{
		Command: command,
		Payload: payload,
	}, nil
}

func readInt(b []byte) (int32, error) {
	var ret int32
	buf := bytes.NewBuffer(b)
	err := binary.Read(buf, binary.BigEndian, &ret)
	if err != nil {
		return 0, err
	}

	return ret, nil
}

func readFloat(b []byte) float32 {
	return math.Float32frombits(binary.BigEndian.Uint32(b))
}

func decodeShiftJIS(b []byte) (string, error) {
	dst := make([]byte, 128)
	_, _, err := japanese.ShiftJIS.NewDecoder().Transform(dst, b, true)
	if err != nil {
		return "", err
	}

	return string(nullTerminate(dst)), nil
}

func nullTerminate(b []byte) []byte {
	for i, data := range b {
		if data == 0x0 {
			return b[:i]
		}
	}

	return b
}

// A Metadata contains metadata about a Slippi game.
type Metadata struct {
	StartAt     string                    `ubjson:"startAt"`
	LastFrame   int32                     `ubjson:"lastFrame"`
	Players     map[string]PlayerMetadata `ubjson:"players"`
	PlayedOn    string                    `ubjson:"playedOn"`
	ConsoleNick string                    `ubjson:"consoleNick"`
}

// A PlayerMetadata contains metadata about a player.
type PlayerMetadata struct {
	Characters map[string]int32 `ubjson:"characters"`
	Names      Names            `ubjson:"names"`
}

// A Names contains the names of a player.
type Names struct {
	Netplay string `ubjson:"netplay"`
	Code    string `ubjson:"code"`
}

// GetMetadata gets metadata from the replay SlpReader is reading.
func (r SlpReader) GetMetadata() (*Metadata, error) {
	if r.MetadataLength <= 0 {
		return nil, nil
	}

	b := make([]byte, r.MetadataLength)

	_, err := r.Source.Seek(r.MetadataStart, io.SeekStart)
	if err != nil {
		return nil, err
	}

	_, err = r.Source.Read(b)
	if err != nil {
		return nil, err
	}

	metadata := &Metadata{}

	decoder := ubjson.NewDecoder(bytes.NewReader(b))
	err = decoder.Decode(metadata)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}
