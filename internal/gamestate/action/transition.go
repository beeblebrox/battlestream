package action

import "time"

// ── Transition / lifecycle actions ───────────────────────────────────────────
// These actions are dispatched regardless of current phase and represent game
// lifecycle events: start, turn change, game over, reconnect, player identity.

// GameStartAction fires when a new game CREATE_GAME block is processed.
type GameStartAction struct {
	ActionBase
	GameID      string
	Timestamp   time.Time
	IsReconnect bool
}

func (a *GameStartAction) actionMarker()                                    {}
func (a *GameStartAction) AcceptTransition(v TransitionVisitor) error { return v.OnGameStart(a) }

// TurnTransitionAction fires when GameEntity TURN changes (every half-turn).
// NewPhase reflects the result: odd GameEntityTurn → PhaseRecruit, even → PhaseCombat.
// OnTurnTransition must flush pendingStatChanges before advancing the phase.
type TurnTransitionAction struct {
	ActionBase
	NewPhase       GamePhase
	GameEntityTurn int // raw GameEntity TURN (doubled; odd=recruit, even=combat)
}

func (a *TurnTransitionAction) actionMarker()                                        {}
func (a *TurnTransitionAction) AcceptTransition(v TransitionVisitor) error { return v.OnTurnTransition(a) }

// GameEndAction fires when GameEntity STATE=COMPLETE is seen.
type GameEndAction struct {
	ActionBase
	Placement int
	Timestamp time.Time
}

func (a *GameEndAction) actionMarker()                                   {}
func (a *GameEndAction) AcceptTransition(v TransitionVisitor) error { return v.OnGameEnd(a) }

// ReconnectAction fires when EventGameEntityTags confirms STATE=RUNNING + TURN > 1.
type ReconnectAction struct{ ActionBase }

func (a *ReconnectAction) actionMarker()                                      {}
func (a *ReconnectAction) AcceptTransition(v TransitionVisitor) error { return v.OnReconnect(a) }

// PlayerDefAction carries all data from a CREATE_GAME Player entity block.
// AccountHi non-zero identifies a real (local or partner) player vs. a bot.
type PlayerDefAction struct {
	ActionBase
	PlayerID      int
	BattleTag     string
	AccountHi     uint64 // non-zero = real player
	HeroEntityID  int    // HERO_ENTITY tag (0 if absent)
	DuoTeammateID int    // BACON_DUO_TEAMMATE_PLAYER_ID (0 if absent)
	InitialTurn   int    // TURN tag on player entity (>0 during reconnect replays)
	Resources     int    // RESOURCES tag (max gold for turn)
	ResourcesUsed int    // RESOURCES_USED tag (gold spent)
}

func (a *PlayerDefAction) actionMarker()                                     {}
func (a *PlayerDefAction) AcceptTransition(v TransitionVisitor) error { return v.OnPlayerDef(a) }

// PlayerNameAction fires when a DebugPrintGame PlayerID → BattleTag mapping arrives.
type PlayerNameAction struct {
	ActionBase
	PlayerID  int
	BattleTag string
}

func (a *PlayerNameAction) actionMarker()                                      {}
func (a *PlayerNameAction) AcceptTransition(v TransitionVisitor) error { return v.OnPlayerName(a) }
