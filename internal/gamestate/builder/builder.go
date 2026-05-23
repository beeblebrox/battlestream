// Package builder converts parser.GameEvent values into typed action.Action values,
// enriching each with CardInfo from the catalog and TriggerKind from block context.
//
// The builder is the single location where raw log events are classified by phase
// and action type. If a classification is wrong, the ActionDispatcher will log a
// warning when it detects a phase mismatch — never silently corrupt state.
package builder

import (
	"fmt"
	"strconv"

	"battlestream.fixates.io/internal/gamestate/action"
	"battlestream.fixates.io/internal/gamestate/card"
	"battlestream.fixates.io/internal/parser"
)

// ActionBuilder converts parser.GameEvents into typed Actions.
// It is not goroutine-safe — call from a single event-loop goroutine.
type ActionBuilder struct {
	catalog     card.Catalog
	phase       action.GamePhase
	currentTurn int
	// entityCards caches cardID lookups by entity ID to survive tag changes
	// that arrive without a CardID.
	entityCards map[action.EntityID]string
}

// New creates a builder backed by the given card catalog.
func New(catalog card.Catalog) *ActionBuilder {
	return &ActionBuilder{
		catalog:     catalog,
		phase:       action.PhaseIdle,
		entityCards: make(map[action.EntityID]string),
	}
}

// Build converts e into a typed Action. Returns nil if no action is needed
// (e.g., an event not yet handled by the builder during incremental migration).
// The dispatcher silently skips nil actions.
func (b *ActionBuilder) Build(e parser.GameEvent) action.Action {
	// Cache entity → cardID for future tag-change lookups.
	if e.CardID != "" && e.EntityID != 0 {
		b.entityCards[action.EntityID(e.EntityID)] = e.CardID
	}

	switch e.Type {
	case parser.EventGameStart:
		return b.buildGameStart(e)
	case parser.EventPlayerDef:
		return b.buildPlayerDef(e)
	case parser.EventPlayerName:
		return b.buildPlayerName(e)
	case parser.EventTurnStart:
		return b.buildTurnTransition(e)
	case parser.EventGameEnd:
		return b.buildGameEnd(e)
	// EventGameEntityTags, EventEntityUpdate, EventTagChange handled in later phases.
	}
	return nil
}

// ── Card lookup helpers ───────────────────────────────────────────────────────

func (b *ActionBuilder) cardInfo(cardID string) *card.CardInfo {
	if cardID == "" {
		return nil
	}
	if info := b.catalog.Lookup(cardID); info != nil {
		return info
	}
	return &card.CardInfo{ID: cardID}
}

func (b *ActionBuilder) blockCardInfo(e parser.GameEvent) *card.CardInfo {
	return b.cardInfo(e.BlockCardID)
}

func (b *ActionBuilder) base(e parser.GameEvent) action.ActionBase {
	return action.ActionBase{
		Card:        b.cardInfo(e.CardID),
		BlockCard:   b.blockCardInfo(e),
		Entity:      action.EntityID(e.EntityID),
		Turn:        b.currentTurn,
		Phase:       b.phase,
		TriggerKind: action.TriggerDirectPlay,
	}
}

// ── Lifecycle builders ────────────────────────────────────────────────────────

func (b *ActionBuilder) buildGameStart(e parser.GameEvent) action.Action {
	b.phase = action.PhaseIdle
	b.entityCards = make(map[action.EntityID]string)
	return &action.GameStartAction{ActionBase: b.base(e)}
}

func (b *ActionBuilder) buildPlayerDef(e parser.GameEvent) action.Action {
	var hi uint64
	if v, ok := e.Tags["GameAccountId.hi"]; ok {
		hi, _ = strconv.ParseUint(v, 10, 64)
	}
	tag := e.EntityName
	if tag == "" {
		tag = e.Tags["BATTLETAG"]
	}
	return &action.PlayerDefAction{
		ActionBase: b.base(e),
		PlayerID:   e.PlayerID,
		BattleTag:  tag,
		AccountHi:  hi,
	}
}

func (b *ActionBuilder) buildPlayerName(e parser.GameEvent) action.Action {
	return &action.PlayerNameAction{
		ActionBase: b.base(e),
		PlayerID:   e.PlayerID,
		BattleTag:  e.EntityName,
	}
}

func (b *ActionBuilder) buildTurnTransition(e parser.GameEvent) action.Action {
	// EventTurnStart fires on GameEntity TURN changes.
	// Odd GameEntity TURN → PhaseRecruit; even → PhaseCombat.
	turn := 0
	if v, ok := e.Tags["TURN"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			turn = n
		}
	}

	newPhase := action.PhaseRecruit
	if turn%2 == 0 {
		newPhase = action.PhaseCombat
	}
	b.phase = newPhase

	return &action.TurnTransitionAction{
		ActionBase: b.base(e),
		NewPhase:   newPhase,
		TurnNumber: b.currentTurn,
	}
}

func (b *ActionBuilder) buildGameEnd(e parser.GameEvent) action.Action {
	b.phase = action.PhaseGameOver
	placement := 0
	if v, ok := e.Tags["PLAYER_LEADERBOARD_PLACE"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			placement = n
		}
	}
	return &action.GameEndAction{
		ActionBase: b.base(e),
		Placement:  placement,
	}
}

// Errorf formats a builder diagnostic message.
func errorf(format string, args ...any) error {
	return fmt.Errorf("builder: "+format, args...)
}

// suppress unused import warning during incremental development
var _ = errorf
