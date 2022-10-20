package slippi

import (
	"bytes"
	"os"
)

// SlpCalculator is the interface to represent calculators
type SlpCalculator interface {
	getChannels() map[ParserEvent][]chan interface{}
}

// A SlpGame contains information about a Slippi game.
type SlpGame struct {
	reader       *SlpReader
	parser       *SlpParser
	metadata     *Metadata
	gameInfo     *GameInfo
	gameInfoChan chan interface{}
	calculators  []SlpCalculator
}

// NewSlpGameFromBytes creates a new SlpGame from the provided bytes.
func NewSlpGameFromBytes(b []byte, calculators []SlpCalculator) (*SlpGame, error) {
	src := NewSlpSourceBytes(bytes.NewReader(b))

	return newSlpGame(src, calculators)
}

// NewSlpGameFromFile creates a new SlpGame from the provided file.
func NewSlpGameFromFile(f *os.File, calculators []SlpCalculator) (*SlpGame, error) {
	src := NewSlpSourceFile(f)

	return newSlpGame(src, calculators)
}

func newSlpGame(src *SlpSource, calculators []SlpCalculator) (*SlpGame, error) {
	reader, err := NewSlpReader(*src)
	if err != nil {
		return nil, err
	}

	gameInfoChan := make(chan interface{})
	parser := NewSlpParser(SlpParserOpts{Strict: false})
	parser.AddHandler(Started, gameInfoChan)

	// attach calculators
	for _, calculator := range calculators {
		for event, channels := range calculator.getChannels() {
			for _, channel := range channels {
				parser.AddHandler(event, channel)
			}
		}
	}

	game := &SlpGame{
		reader:       reader,
		parser:       parser,
		metadata:     nil,
		gameInfo:     nil,
		gameInfoChan: gameInfoChan,
		calculators:  calculators,
	}

	go func() {
		for val := range gameInfoChan {
			gameInfo := val.(*GameInfo)
			game.gameInfo = gameInfo
		}
	}()

	return game, nil
}

// Close the SlpGame's underlying channel.
func (g *SlpGame) Close() {
	close(g.gameInfoChan)
}

// AddCalculator adds a calculator to the SlpGame.
func (g *SlpGame) AddCalculator(c SlpCalculator) {
	g.calculators = append(g.calculators, c)
	for event, handlers := range c.getChannels() {
		for _, handler := range handlers {
			g.parser.AddHandler(event, handler)
		}
	}
}

// RemoveCalculator removes a calculator from the SlpGame.
func (g *SlpGame) RemoveCalculator(c SlpCalculator) {
	for i, calculator := range g.calculators {
		if calculator == c {
			g.calculators = append(g.calculators[:i], g.calculators[i+1:]...)
		}
	}
	for event, handlers := range c.getChannels() {
		for _, handler := range handlers {
			g.parser.AddHandler(event, handler)
		}
	}
}

// RemoveAllCalculators removes all calculators from the SlpGame.
func (g *SlpGame) RemoveAllCalculators() {
	for _, calculator := range g.calculators {
		for event, handlers := range calculator.getChannels() {
			for _, handler := range handlers {
				g.parser.AddHandler(event, handler)
			}
		}
	}

	g.calculators = make([]SlpCalculator, 0)
}

// GetGameInfo gets the game info of the SlpGame.
func (g *SlpGame) GetGameInfo() (*GameInfo, error) {
	if g.gameInfo != nil {
		return &*g.gameInfo, nil
	}

	gameInfo, complete := g.parser.GetGameInfo()
	if complete {
		g.gameInfo = gameInfo
		return &*g.gameInfo, nil
	}

	err := g.process(true)
	if err != nil {
		return nil, err
	}

	return &*g.gameInfo, nil
}

// GetLatestFrame gets the latest frame in the SlpGame.
func (g *SlpGame) GetLatestFrame() (*FrameEntry, error) {
	err := g.process(false)
	if err != nil {
		return nil, err
	}

	return g.parser.GetLatestFrame(), nil
}

// GetGameEnd gets the game end event from the SlpGame.
func (g *SlpGame) GetGameEnd() (*GameEndPayload, error) {
	err := g.process(false)
	if err != nil {
		return nil, err
	}

	return &*g.parser.GameEnd, nil
}

// GetFrames gets the frames from the SlpGame.
func (g *SlpGame) GetFrames() (map[int32]FrameEntry, error) {
	err := g.process(false)
	if err != nil {
		return nil, err
	}

	frames := make(map[int32]FrameEntry)
	for key, frame := range g.parser.Frames {
		frames[key] = frame
	}
	return frames, nil
}

// GetRollbackFrames gets the rollback frames from the SlpGame.
func (g *SlpGame) GetRollbackFrames() (map[int32][]FrameEntry, error) {
	err := g.process(false)
	if err != nil {
		return nil, err
	}

	rollbackFrames := make(map[int32][]FrameEntry, 0)
	for key, frame := range g.parser.Rollbacks.Frames {
		rollbackFrames[key] = frame
	}
	return rollbackFrames, nil
}

// GetMetadata gets the SlpGame's metadata.
func (g *SlpGame) GetMetadata() (*Metadata, error) {
	if g.metadata != nil {
		return &*g.metadata, nil
	}

	metadata, err := g.reader.GetMetadata()
	if err != nil {
		return nil, err
	} else if metadata == nil {
		return nil, nil
	}

	g.metadata = metadata

	return &*metadata, nil
}

func (g *SlpGame) process(onlyGameInfo bool) error {
	g.parser.Reset()

	stopYielding := func(*SlpEvent) bool {
		_, complete := g.parser.GetGameInfo()
		return onlyGameInfo && complete
	}

	events, err := g.reader.YieldEvents(stopYielding)
	if err != nil {
		return err
	}

	err = g.parser.ParseReplay(events)
	if err != nil {
		return err
	}
	return nil
}
