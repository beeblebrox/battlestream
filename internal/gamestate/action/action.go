// Package action defines the typed action hierarchy for Battlegrounds log processing.
// Every event from Power.log becomes a concrete action type. Phase (RECRUIT vs COMBAT)
// is enforced via unexported marker methods — the dispatcher can never route a combat
// action to a recruit handler or vice versa.
//
// Visitor interfaces (RecruitVisitor, CombatVisitor, TransitionVisitor) are defined
// in visitors.go alongside the action types to avoid import cycles.
package action

import "battlestream.fixates.io/internal/gamestate/card"

// EntityID is a numeric game entity identifier.
type EntityID int

// GamePhase represents the current phase of the Battlegrounds round.
type GamePhase int

const (
	PhaseUnknown GamePhase = iota
	PhaseIdle
	PhaseRecruit
	PhaseCombat
	PhaseGameOver
)

// TriggerKind describes what caused an action to occur.
type TriggerKind int

const (
	TriggerDirectPlay      TriggerKind = iota // player explicitly played/bought/sold
	TriggerBattlecry                          // triggered by a battlecry
	TriggerDeathrattle                        // triggered by a deathrattle
	TriggerRally                              // triggered by a rally effect
	TriggerPassiveAbility                     // passive card ability
	TriggerHeroPower                          // hero power activation
	TriggerReconnectReplay                    // synthetic event from reconnect replay
)

// BuffCategory matches the category constants in categories.go.
type BuffCategory = string

// ActionBase is embedded in all concrete action types.
type ActionBase struct {
	// Card is the card performing the action. Nil for player-tag-only events.
	Card *card.CardInfo

	// BlockCard is the card whose BLOCK_START contains this action.
	// Set when a battlecry, deathrattle, rally, or spell triggers this action.
	BlockCard *card.CardInfo

	Entity      EntityID
	Turn        int
	Phase       GamePhase // for logging/diagnostics only; dispatch uses the dispatcher's own phase
	TriggerKind TriggerKind
}

// Action is the common interface for all typed actions.
type Action interface {
	actionMarker()
}

// dropAction is the sentinel no-op type behind Drop. It deliberately implements
// only Action (not RecruitPhaseAction/CombatPhaseAction/TransitionAction), so it
// can never be routed to a visitor.
type dropAction struct{}

func (dropAction) actionMarker() {}

// Drop is the sentinel returned by builders for events that are deliberately
// consumed and discarded — e.g. phase-gated tags arriving outside their valid
// phase. It distinguishes "handled: do nothing" from a nil return, which means
// "not migrated: fall through to the legacy Processor.Handle path". The
// dispatcher treats Drop as a silent no-op, so call sites that route every
// non-nil action to Dispatch are correct without any special casing.
var Drop Action = dropAction{}

// RecruitPhaseAction can only be dispatched during PhaseRecruit.
// The unexported recruitPhase() method prevents accidental satisfaction
// by types defined outside this package.
type RecruitPhaseAction interface {
	Action
	recruitPhase()
	AcceptRecruit(RecruitVisitor) error
}

// CombatPhaseAction can only be dispatched during PhaseCombat.
type CombatPhaseAction interface {
	Action
	combatPhase()
	AcceptCombat(CombatVisitor) error
}

// TransitionAction is dispatched regardless of current phase (game lifecycle).
type TransitionAction interface {
	Action
	AcceptTransition(TransitionVisitor) error
}
