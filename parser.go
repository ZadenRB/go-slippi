package slippi

import (
	"errors"
	"fmt"
	"github.com/blang/semver/v4"
	"math"
)

const MaxRollbackFrames = 7

// SlpParserOpts contains options that determine how a SlpParser behaves.
type SlpParserOpts struct {
	Strict bool
}

// FrameUpdateType enumerates the types of frame updates.
type FrameUpdateType string

// FrameUpdateTypes
const (
	Pre  FrameUpdateType = "pre"
	Post                 = "post"
)

// FrameUpdates holds pre- and post-frame updates.
type FrameUpdates struct {
	Pre  *PreFrameUpdatePayload
	Post *PostFrameUpdatePayload
}

// A FrameEntry contains all relevant updates from a given frame.
type FrameEntry struct {
	Players            map[uint8]FrameUpdates
	Followers          map[uint8]FrameUpdates
	Items              []ItemUpdatePayload
	IsTransferComplete bool
}

// GameInfo contains the general information about a game of Melee.
type GameInfo struct {
	Version    semver.Version
	Teams      bool
	PAL        bool
	Stage      uint16
	Players    []PlayerInfo
	MajorScene uint8
	MinorScene uint8
}

// ParserEvent enumerates events sent by a SlpParser
type ParserEvent uint8

// ParserEvents
const (
	Started ParserEvent = iota
	Frame
	FinalizedFrame
	RollbackFrame
	Ended
)

// Rollbacks tracks the rollbacks within a replay.
type Rollbacks struct {
	Frames                map[int32][]FrameEntry
	Count                 int
	Lengths               []int
	playerIndex           int8
	lastFrameWasRollback  bool
	currentRollbackLength int
}

func (r *Rollbacks) checkIfRollbackFrame(frameIndex int32, frame *FrameEntry, playerIndex uint8) bool {
	if r.playerIndex == -1 {
		r.playerIndex = int8(playerIndex)
	} else if r.playerIndex != int8(playerIndex) {
		return false
	}

	if frame != nil {
		if frames, ok := r.Frames[frameIndex]; !ok {
			r.Frames[frameIndex] = append(make([]FrameEntry, 0), *frame)
		} else {
			r.Frames[frameIndex] = append(frames, *frame)
		}
		r.Count++
		r.currentRollbackLength++
		r.lastFrameWasRollback = true
	} else if r.lastFrameWasRollback {
		r.Lengths = append(r.Lengths, r.currentRollbackLength)
		r.currentRollbackLength = 0
		r.lastFrameWasRollback = false
	}

	return r.lastFrameWasRollback
}

// A SlpParser parses a replay into frames.
type SlpParser struct {
	Options            SlpParserOpts
	Frames             map[int32]FrameEntry
	Rollbacks          Rollbacks
	gameInfo           *GameInfo
	GameEnd            *GameEndPayload
	handlers           map[ParserEvent][]chan interface{}
	latestFrameIndex   int32
	lastFinalizedFrame int32
	gameInfoComplete   bool
}

// NewSlpParser creates a new SlpParser with the given SlpParserOpts.
func NewSlpParser(options SlpParserOpts) *SlpParser {
	return &SlpParser{
		Options:            options,
		Frames:             make(map[int32]FrameEntry),
		gameInfo:           nil,
		GameEnd:            nil,
		handlers:           make(map[ParserEvent][]chan interface{}),
		latestFrameIndex:   -124,
		lastFinalizedFrame: -124,
		gameInfoComplete:   false,
		Rollbacks: Rollbacks{
			Frames:                make(map[int32][]FrameEntry),
			Count:                 0,
			Lengths:               make([]int, 0),
			playerIndex:           -1,
			lastFrameWasRollback:  false,
			currentRollbackLength: 0,
		},
	}
}

// Reset resets the SlpParser's state. This does not reset parser options or
// remove event handler channels.
func (p *SlpParser) Reset() {
	p.Frames = make(map[int32]FrameEntry)
	p.gameInfo = nil
	p.GameEnd = nil
	p.latestFrameIndex = -124
	p.lastFinalizedFrame = -124
	p.gameInfoComplete = false
	p.Rollbacks = Rollbacks{
		Frames:                make(map[int32][]FrameEntry),
		Count:                 0,
		Lengths:               make([]int, 0),
		playerIndex:           -1,
		lastFrameWasRollback:  false,
		currentRollbackLength: 0,
	}
}

// GetPlayableFrameCount returns the number of playable frames parsed so far.
func (p *SlpParser) GetPlayableFrameCount() int32 {
	if p.latestFrameIndex < -39 {
		return 0
	}
	return p.latestFrameIndex + 39
}

// GetLatestFrame gets the latest frame parsed by the SlpParser.
func (p *SlpParser) GetLatestFrame() *FrameEntry {
	frameIndex := int32(math.Max(float64(p.latestFrameIndex), -123))
	if p.GameEnd != nil {
		frameIndex -= 1
	}

	frame := p.Frames[frameIndex]

	return &frame
}

// GetGameInfo gets the current parsed game info, as well as a boolean indicating
// if the full game info has been parsed yet.
func (p *SlpParser) GetGameInfo() (*GameInfo, bool) {
	if p.gameInfo == nil {
		return nil, p.gameInfoComplete
	} else {
		return &*p.gameInfo, p.gameInfoComplete
	}
}

// AddHandler attaches an event handler channel to a ParseEvent.
func (p *SlpParser) AddHandler(event ParserEvent, handler chan interface{}) {
	handlers, ok := p.handlers[event]
	if !ok {
		handlers = make([]chan interface{}, 0)
	}

	p.handlers[event] = append(handlers, handler)
}

// RemoveHandler removes an event handler channel from a ParseEvent.
func (p *SlpParser) RemoveHandler(event ParserEvent, toRemove chan interface{}) {
	if handlers, ok := p.handlers[event]; ok {
		for i, handler := range handlers {
			if handler == toRemove {
				p.handlers[event] = append(p.handlers[event][:i], p.handlers[event][i+1:]...)
			}
		}
	}
}

// RemoveAllHandlers removes all event handler channels from a ParseEvent.
func (p *SlpParser) RemoveAllHandlers(event ParserEvent) {
	p.handlers[event] = nil
}

// Trigger triggers the given ParserEvent with the given payload, sending it to
// all attached handler channels.
func (p *SlpParser) Trigger(event ParserEvent, payload interface{}) {
	if handlers, ok := p.handlers[event]; ok {
		for _, handler := range handlers {
			h := handler
			go func() {
				h <- payload
			}()
		}
	}
}

// ParseReplay processes events from the given SlpEventResult channel and updates
// the SlpParser's state accordingly.
func (p *SlpParser) ParseReplay(eventResults <-chan *SlpEventResult) error {
	for eventResult := range eventResults {
		if eventResult.Error != nil {
			flushChannel(eventResults)
			return eventResult.Error
		}

		event := *eventResult.Event

		err := p.handleEvent(event)
		if err != nil {
			flushChannel(eventResults)
			return err
		}
	}

	return nil
}

func flushChannel(channel <-chan *SlpEventResult) {
	for {
		select {
		case _, ok := <-channel:
			if !ok {
				return
			}
		default:
		}
	}
}

func (p *SlpParser) handleEvent(event SlpEvent) error {
	var err error = nil
	switch event.Command {
	case GameStart:
		p.handleGameStart(event.Payload.(GameStartPayload))
	case PreFrameUpdate:
		err = p.handleFrameUpdate(Pre, event.Payload.(PreFrameUpdatePayload))
	case PostFrameUpdate:
		err = p.handlePostFrameUpdate(event.Payload.(PostFrameUpdatePayload))
	case GameEnd:
		err = p.handleGameEnd(event.Payload.(GameEndPayload))
	case ItemUpdate:
		p.handleItemUpdate(event.Payload.(ItemUpdatePayload))
	case FrameBookend:
		err = p.handleFrameBookend(event.Payload.(FrameBookendPayload))
	}

	return err
}

func (p *SlpParser) handleGameStart(payload GameStartPayload) {
	players := make([]PlayerInfo, 0)

	// remove empty players
	for _, player := range payload.Players {
		if player.PlayerType != Empty {
			players = append(players, player)
		}
	}

	// set game info
	p.gameInfo = &GameInfo{
		Version:    payload.Version,
		Teams:      payload.GameInfoBlock.IsTeams,
		PAL:        payload.PAL,
		Stage:      payload.GameInfoBlock.Stage,
		Players:    players,
		MajorScene: payload.MajorScene,
		MinorScene: payload.MinorScene,
	}

	if payload.Version.GTE(semver.MustParse("1.6.0")) {
		p.completeGameInfo()
	}
}

func (p *SlpParser) handleFrameUpdate(updateType FrameUpdateType, payload FrameUpdatePayload) error {
	frameUpdate := payload.GetFrameUpdate()
	frameNumber := frameUpdate.FrameNumber
	isFollower := frameUpdate.IsFollower
	playerIndex := frameUpdate.PlayerIndex

	frame := p.getFrame(frameNumber)

	p.latestFrameIndex = frameNumber
	if updateType == Pre && !isFollower {
		currentFrame := p.Frames[frameNumber]
		if p.Rollbacks.checkIfRollbackFrame(frameNumber, &currentFrame, playerIndex) {
			p.Trigger(RollbackFrame, currentFrame)
		}
	}

	// add frame update to followers or players
	if isFollower {
		follower, ok := frame.Followers[playerIndex]
		if !ok {
			follower = FrameUpdates{
				Pre:  nil,
				Post: nil,
			}
		}

		switch updateType {
		case Pre:
			preFrameUpdate := payload.(PreFrameUpdatePayload)
			follower.Pre = &preFrameUpdate
		case Post:
			postFrameUpdate := payload.(PostFrameUpdatePayload)
			follower.Post = &postFrameUpdate
		}
		frame.Followers[playerIndex] = follower
	} else {
		player, ok := frame.Players[playerIndex]
		if !ok {
			player = FrameUpdates{
				Pre:  nil,
				Post: nil,
			}
		}

		switch updateType {
		case Pre:
			preFrameUpdate := payload.(PreFrameUpdatePayload)
			player.Pre = &preFrameUpdate
		case Post:
			postFrameUpdate := payload.(PostFrameUpdatePayload)
			player.Post = &postFrameUpdate
		}
		frame.Players[playerIndex] = player
	}

	p.Frames[frameNumber] = frame

	// emit frame here if file is from before frame bookending existed
	if p.gameInfo != nil && p.gameInfo.Version.LTE(semver.MustParse("2.2.0")) {
		p.Trigger(Frame, p.Frames[frameNumber])
		err := p.finalizeFrames(frameNumber - 1)
		if err != nil {
			return err
		}
	} else {
		frame.IsTransferComplete = false
		p.Frames[frameNumber] = frame
	}

	return nil
}

func (p *SlpParser) handlePostFrameUpdate(payload PostFrameUpdatePayload) error {
	err := p.handleFrameUpdate(Post, payload)
	if err != nil {
		return err
	}

	if p.gameInfoComplete {
		return nil
	}

	if payload.FrameNumber <= -123 {
		for i, player := range p.gameInfo.Players {
			if player.Index == payload.PlayerIndex {
				switch payload.InternalCharacterID {
				case 0x7:
					p.gameInfo.Players[i].CharacterID = 0x13
				case 0x13:
					p.gameInfo.Players[i].CharacterID = 0x12
				}
			}
		}
	}

	if payload.FrameNumber > -123 {
		p.completeGameInfo()
	}

	return nil
}

func (p *SlpParser) handleGameEnd(payload GameEndPayload) error {
	var err error = nil
	if p.latestFrameIndex > -124 && p.latestFrameIndex != p.lastFinalizedFrame {
		err = p.finalizeFrames(p.latestFrameIndex)
	}

	p.GameEnd = &payload

	p.Trigger(Ended, payload)

	return err
}

func (p *SlpParser) handleItemUpdate(payload ItemUpdatePayload) {
	frame := p.getFrame(payload.FrameNumber)

	frame.Items = append(frame.Items, payload)
	p.Frames[payload.FrameNumber] = frame
}

func (p *SlpParser) handleFrameBookend(payload FrameBookendPayload) error {
	latestFinalizedFrame := payload.LatestFinalizedFrame
	frameNumber := payload.FrameNumber
	frame := p.getFrame(frameNumber)

	frame.IsTransferComplete = true
	p.Frames[frameNumber] = frame

	p.Trigger(Frame, frame)

	validLatestFrame := p.gameInfo.MajorScene == 0x8
	var err error = nil
	if validLatestFrame && latestFinalizedFrame >= -123 {
		if p.Options.Strict && latestFinalizedFrame < frameNumber-MaxRollbackFrames {
			return errors.New(fmt.Sprintf("latestFinalizedFrame should be within %d frames of %d", MaxRollbackFrames, frameNumber))
		}
		err = p.finalizeFrames(latestFinalizedFrame)
	} else {
		err = p.finalizeFrames(frameNumber - MaxRollbackFrames)
	}

	return err
}

func (p *SlpParser) finalizeFrames(frameNumber int32) error {
	for p.lastFinalizedFrame < frameNumber {
		toFinalize := p.lastFinalizedFrame + 1
		frame, ok := p.Frames[toFinalize]
		if !ok {
			return nil
		}

		if p.Options.Strict {
			for _, player := range p.gameInfo.Players {
				playerFrameInfo, ok := frame.Players[player.Index]

				if !ok {
					if len(p.gameInfo.Players) > 2 {
						continue
					}

					return errors.New(fmt.Sprintf("could not finalize frame %d of %d: missing pre-frame update for player %d", toFinalize, frameNumber, player.Index))
				}

				if playerFrameInfo.Pre == nil || playerFrameInfo.Post == nil {
					missing := "pre"
					if playerFrameInfo.Pre != nil {
						missing = "post"
					}

					return errors.New(fmt.Sprintf("could not finalize frame %d of %d: missing %s-frame update for player %d", toFinalize, frameNumber, missing, player.Index))
				}
			}
		}

		p.Trigger(FinalizedFrame, frame)
		p.lastFinalizedFrame = toFinalize
	}

	return nil
}

func (p *SlpParser) completeGameInfo() {
	if p.gameInfoComplete {
		return
	}

	p.gameInfoComplete = true
	p.Trigger(Started, p.gameInfo)
}

func (p *SlpParser) getFrame(frameNumber int32) FrameEntry {
	frame, ok := p.Frames[frameNumber]
	if !ok {
		frame = FrameEntry{
			Players:            make(map[uint8]FrameUpdates, 0),
			Followers:          make(map[uint8]FrameUpdates, 0),
			Items:              make([]ItemUpdatePayload, 0),
			IsTransferComplete: false,
		}
	}

	return frame
}
