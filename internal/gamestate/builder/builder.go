// Package builder converts parser.GameEvent values into typed action.Action values,
// enriching each with CardInfo from the catalog and TriggerKind from block context.
//
// The builder is the single location where raw log events are classified by phase
// and action type. If a classification is wrong, the ActionDispatcher will log a
// warning when it detects a phase mismatch — never silently corrupt state.
package builder

import (
	"strconv"
	"time"

	"battlestream.fixates.io/internal/gamestate/action"
	"battlestream.fixates.io/internal/gamestate/card"
	"battlestream.fixates.io/internal/parser"
)

// ActionBuilder converts parser.GameEvents into typed Actions.
// It is not goroutine-safe — call from a single event-loop goroutine.
type ActionBuilder struct {
	catalog        card.Catalog
	phase          action.GamePhase
	gameEntityTurn int // raw GameEntity TURN (doubled)
	// entityCards caches cardID by entity ID for tag-change events without CardID.
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

// Build converts e into a typed Action. Returns nil if the event type is not
// yet handled (non-migrated events during incremental migration). The dispatcher
// silently skips nil returns.
func (b *ActionBuilder) Build(e parser.GameEvent) action.Action {
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
	// EventGameEntityTags, EventEntityUpdate, EventTagChange: Phase 2+ / Phase 3+
	}
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (b *ActionBuilder) cardInfo(cardID string) *card.CardInfo {
	if cardID == "" {
		return nil
	}
	if info := b.catalog.Lookup(cardID); info != nil {
		return info
	}
	return &card.CardInfo{ID: cardID}
}

func (b *ActionBuilder) base(e parser.GameEvent) action.ActionBase {
	return action.ActionBase{
		Card:        b.cardInfo(e.CardID),
		BlockCard:   b.cardInfo(e.BlockCardID),
		Entity:      action.EntityID(e.EntityID),
		Phase:       b.phase,
		TriggerKind: action.TriggerDirectPlay,
	}
}

func tagInt(tags map[string]string, key string) int {
	if v, ok := tags[key]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 0
}

func tagUint64(tags map[string]string, key string) uint64 {
	if v, ok := tags[key]; ok {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

// ── Lifecycle builders ────────────────────────────────────────────────────────

func (b *ActionBuilder) buildGameStart(e parser.GameEvent) action.Action {
	b.phase = action.PhaseIdle
	b.gameEntityTurn = 0
	b.entityCards = make(map[action.EntityID]string)

	var gameID string
	if !e.Timestamp.IsZero() {
		gameID = "game-" + strconv.FormatInt(e.Timestamp.UnixMilli(), 10)
	}
	return &action.GameStartAction{
		ActionBase: b.base(e),
		GameID:     gameID,
		Timestamp:  e.Timestamp,
	}
}

func (b *ActionBuilder) buildPlayerDef(e parser.GameEvent) action.Action {
	tag := e.EntityName
	if tag == "" {
		tag = e.Tags["BATTLETAG"]
	}
	return &action.PlayerDefAction{
		ActionBase:    b.base(e),
		PlayerID:      e.PlayerID,
		BattleTag:     tag,
		AccountHi:     tagUint64(e.Tags, "hi"),
		HeroEntityID:  tagInt(e.Tags, "HERO_ENTITY"),
		DuoTeammateID: tagInt(e.Tags, "BACON_DUO_TEAMMATE_PLAYER_ID"),
		InitialTurn:   tagInt(e.Tags, "TURN"),
		Resources:     tagInt(e.Tags, "RESOURCES"),
		ResourcesUsed: tagInt(e.Tags, "RESOURCES_USED"),
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
	turn := tagInt(e.Tags, "TURN")
	newPhase := action.PhaseRecruit
	if turn%2 == 0 {
		newPhase = action.PhaseCombat
	}
	b.phase = newPhase
	b.gameEntityTurn = turn

	return &action.TurnTransitionAction{
		ActionBase:     b.base(e),
		NewPhase:       newPhase,
		GameEntityTurn: turn,
	}
}

func (b *ActionBuilder) buildGameEnd(e parser.GameEvent) action.Action {
	b.phase = action.PhaseGameOver
	return &action.GameEndAction{
		ActionBase: b.base(e),
		Placement:  tagInt(e.Tags, "PLAYER_LEADERBOARD_PLACE"),
		Timestamp:  time.Now(),
	}
}
