package action

// ── Transition / lifecycle actions ───────────────────────────────────────────
// These actions are dispatched regardless of current phase and represent game
// lifecycle events: start, turn change, game over, reconnect, player identity.

// GameStartAction fires when a new game CREATE_GAME block is processed.
type GameStartAction struct {
	ActionBase
	GameID   string
	IsDuos   bool
	IsReconnect bool
}

func (a *GameStartAction) actionMarker()                                    {}
func (a *GameStartAction) AcceptTransition(v TransitionVisitor) error { return v.OnGameStart(a) }

// TurnTransitionAction fires when GameEntity TURN changes (every half-turn).
// NewPhase reflects the result: odd TURN → PhaseRecruit, even → PhaseCombat.
// TransitionVisitor.OnTurnTransition must flush pendingStatChanges explicitly.
type TurnTransitionAction struct {
	ActionBase
	NewPhase   GamePhase
	TurnNumber int // player-visible BG turn (from player TURN tag, not GameEntity TURN)
}

func (a *TurnTransitionAction) actionMarker()                                        {}
func (a *TurnTransitionAction) AcceptTransition(v TransitionVisitor) error { return v.OnTurnTransition(a) }

// GameEndAction fires when GameEntity STATE=COMPLETE is seen.
type GameEndAction struct {
	ActionBase
	Placement int
}

func (a *GameEndAction) actionMarker()                                   {}
func (a *GameEndAction) AcceptTransition(v TransitionVisitor) error { return v.OnGameEnd(a) }

// ReconnectAction fires when a mid-game CREATE_GAME block is detected
// (GameID matches an in-progress game).
type ReconnectAction struct{ ActionBase }

func (a *ReconnectAction) actionMarker()                                      {}
func (a *ReconnectAction) AcceptTransition(v TransitionVisitor) error { return v.OnReconnect(a) }

// PlayerDefAction fires when a player entity definition arrives in CREATE_GAME.
type PlayerDefAction struct {
	ActionBase
	PlayerID  int
	BattleTag string
	AccountHi uint64 // non-zero for local player identification
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
