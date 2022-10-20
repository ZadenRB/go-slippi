package slippi

import "github.com/blang/semver/v4"

// For more info on all data structures in this file, see
// https://github.com/project-slippi/slippi-wiki/blob/master/SPEC.md

// MessageSplitterPayload represents the MessageSplitter Slippi event.
type MessageSplitterPayload struct {
	Data            [512]uint8
	DataLength      uint16
	InternalCommand uint8
	LastMessage     bool
}

// EventPayloadsPayload represents the EventPayloads Slippi event.
type EventPayloadsPayload struct {
	PayloadSize  uint8
	PayloadSizes map[uint8]uint16
}

// PlayerType enumerates the different player types in Melee.
type PlayerType uint8

// PlayerTypes
const (
	Human PlayerType = iota
	CPU
	Demo
	Empty
)

// TeamShade enumerates the coloration changes for multiples of the same
// character on the same team.
type TeamShade uint8

// TeamShades
const (
	Normal TeamShade = iota
	Light
	Dark
)

// TeamID enumerates the possible team colors in Melee.
type TeamID uint8

// TeamIDs
const (
	Red TeamID = iota
	Blue
	Green
)

// DashbackFix enumerates the controller fixes for dashback.
type DashbackFix uint32

// DashbackFixes
const (
	DBOff DashbackFix = iota
	DBUCF
	DBDween
)

// ShieldDropFix enumerates the controller fixes for shield drops.
type ShieldDropFix uint32

// ShieldDropFixes
const (
	SDFixOff ShieldDropFix = iota
	SDUCF
	SDDween
)

// PlayerInfo contains information about a player in Melee.
type PlayerInfo struct {
	Index           uint8
	Port            uint8
	CharacterID     uint8
	PlayerType      PlayerType
	StockStartCount uint8
	CostumeIndex    uint8
	TeamShade       TeamShade
	Handicap        uint8
	TeamID          TeamID
	PlayerBitfield  uint8
	CPULevel        uint8
	OffenseRatio    float32
	DefenseRatio    float32
	ModelScale      float32
	DashbackFix     DashbackFix
	ShieldDropFix   ShieldDropFix
	Nametag         string
	DisplayName     string
	ConnectCode     string
	SlippiUID       string
}

// ItemSpawnBehavior enumerates item spawn frequencies.
type ItemSpawnBehavior int8

// ItemSpawnBehaviors
const (
	ItemsVeryLow = iota
	ItemsLow
	ItemsMedium
	ItemsHigh
	ItemsVeryHigh
	Items5
	Items6
	Items7
	Items8
	ItemsOff ItemSpawnBehavior = -1
)

// GameInfoBlock contains the Melee game info block.
type GameInfoBlock struct {
	GameBitfield1          uint8
	GameBitfield2          uint8
	GameBitfield3          uint8
	GameBitfield4          uint8
	BombRain               uint8
	IsTeams                bool
	ItemSpawnBehavior      ItemSpawnBehavior
	SelfDestructScoreValue int8
	Stage                  uint16
	GameTimer              uint32
	ItemSpawnBitfield1     uint8
	ItemSpawnBitfield2     uint8
	ItemSpawnBitfield3     uint8
	ItemSpawnBitfield4     uint8
	ItemSpawnBitfield5     uint8
	DamageRatio            float32
}

// Language enumerates the language options.
type Language uint8

// Languages
const (
	Japanese Language = iota
	English
)

// GameStartPayload represents the GameStart Slippi event.
type GameStartPayload struct {
	Version        semver.Version
	GameInfoBlock  GameInfoBlock
	Players        [4]PlayerInfo
	RandomSeed     uint32
	PAL            bool
	FrozenPS       bool
	MajorScene     uint8
	MinorScene     uint8
	LanguageOption Language
}

// FrameUpdate contains fields generic to pre- and post-frame update Slippi
// events.
type FrameUpdate struct {
	FrameNumber     int32
	PlayerIndex     uint8
	IsFollower      bool
	ActionStateID   uint16
	XPosition       float32
	YPosition       float32
	FacingDirection float32
	Percent         float32
}

// The FrameUpdatePayload is the interface that abstract pre- and post-frame
// updates to their shared FrameUpdate field.
type FrameUpdatePayload interface {
	GetFrameUpdate() FrameUpdate
}

// PreFrameUpdatePayload represents the PreFrameUpdate Slippi event.
type PreFrameUpdatePayload struct {
	FrameUpdate
	RandomSeed       uint32
	JoystickX        float32
	JoystickY        float32
	CStickX          float32
	CStickY          float32
	Trigger          float32
	ProcessedButtons uint32
	PhysicalButtons  uint16
	PhysicalLTrigger float32
	PhysicalRTrigger float32
	XAnalogUCF       uint8
}

// GetFrameUpdate implements the FrameUpdatePayload interface.
func (u PreFrameUpdatePayload) GetFrameUpdate() FrameUpdate {
	return u.FrameUpdate
}

// LCancelStatus enumerates possible L-Cancel statuses.
type LCancelStatus uint8

// LCancelStatuses
const (
	None LCancelStatus = iota
	Successful
	Unsuccessful
)

// HurtboxCollisionState enumerates possible hurtbox collision states.
type HurtboxCollisionState uint8

// HurtboxCollisionStates
const (
	Vulnerable HurtboxCollisionState = iota
	Invulnerable
	Intangible
)

// PostFrameUpdatePayload represents the PostFrameUpdate Slippi event.
type PostFrameUpdatePayload struct {
	FrameUpdate
	InternalCharacterID     uint8
	ShieldSize              float32
	LastHittingAttackID     uint8
	CurrentComboCount       uint8
	LastHitBy               uint8
	StocksRemaining         uint8
	ActionStateFrameCounter float32
	StateBitFlags1          uint8
	StateBitFlags2          uint8
	StateBitFlags3          uint8
	StateBitFlags4          uint8
	StateBitFlags5          uint8
	MiscAS                  float32
	Airborne                bool
	LastGroundID            uint16
	JumpsRemaining          uint8
	LCancelStatus           LCancelStatus
	HurtboxCollisionState   HurtboxCollisionState
	SelfInducedAirXSpeed    float32
	SelfInducedYSpeed       float32
	AttackBasedXSpeed       float32
	AttackBasedYSpeed       float32
	SelfInducedGroundXSpeed float32
	HitlagFramesRemaining   float32
	AnimationIndex          uint32
}

// GetFrameUpdate implements the FrameUpdatePayload interface.
func (u PostFrameUpdatePayload) GetFrameUpdate() FrameUpdate {
	return u.FrameUpdate
}

// GameEndMethod enumerates the game end methods in Melee.
type GameEndMethod uint8

// GameEndMethods
const (
	Unresolved GameEndMethod = 0
	Time                     = 1
	Game                     = 2
	Resolved                 = 3
	NoContest                = 7
)

// GameEndPayload represents the GameEnd Slippi event.
type GameEndPayload struct {
	GameEndMethod GameEndMethod
	LRASInitiator int8
}

// FrameStartPayload represents the FrameStart Slippi event.
type FrameStartPayload struct {
	FrameNumber       int32
	RandomSeed        uint32
	SceneFrameCounter uint32
}

// ItemUpdatePayload represents the ItemUpdate Slippi event.
type ItemUpdatePayload struct {
	FrameNumber      int32
	TypeID           uint16
	State            uint8
	FacingDirection  float32
	XVelocity        float32
	YVelocity        float32
	XPosition        float32
	YPosition        float32
	DamageTaken      uint16
	ExpirationTimer  float32
	SpawnID          uint32
	SamusMissileType uint8
	PeachTurnipFace  uint8
	IsLaunched       uint8
	ChargedPower     uint8
	Owner            int8
}

// FrameBookendPayload represents the FrameBookend Slippi event.
type FrameBookendPayload struct {
	FrameNumber          int32
	LatestFinalizedFrame int32
}

// GeckoListPayload represents the GeckoList Slippi event.
type GeckoListPayload struct {
	GeckoCodes []byte
}

// Command enumerates the command bytes of Slippi events.
type Command byte

/// Commands
const (
	EventPayloads Command = iota + 0x35
	GameStart
	PreFrameUpdate
	PostFrameUpdate
	GameEnd
	FrameStart
	ItemUpdate
	FrameBookend
	GeckoList
	MessageSplitter Command = 0x10
)

// SlpEvent contains the command of an event and its associated data.
type SlpEvent struct {
	Command Command
	Payload interface{}
}
