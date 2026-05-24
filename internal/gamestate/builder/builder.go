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
	// entityTypes caches CARDTYPE by entity ID for Phase 8 hero/minion disambiguation
	// in buildTagChange. Populated from EventEntityUpdate blocks (which return nil from Build).
	entityTypes map[action.EntityID]string
}

// New creates a builder backed by the given card catalog.
func New(catalog card.Catalog) *ActionBuilder {
	return &ActionBuilder{
		catalog:     catalog,
		phase:       action.PhaseIdle,
		entityCards: make(map[action.EntityID]string),
		entityTypes: make(map[action.EntityID]string),
	}
}

// Build converts e into a typed Action. Returns nil if the event type is not
// yet handled (non-migrated events during incremental migration). The dispatcher
// silently skips nil returns.
func (b *ActionBuilder) Build(e parser.GameEvent) action.Action {
	// Cache card ID by entity ID so tag-change events can inherit card info
	// from their entity's earlier FULL_ENTITY/SHOW_ENTITY block.
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
	case parser.EventTagChange:
		return b.buildTagChange(e)
	case parser.EventGameEntityTags:
		return b.buildReconnect(e)
	}

	// Populate entityTypes cache for Phase 8 (hero vs minion disambiguation in buildTagChange).
	// Returns nil — EventEntityUpdate is still handled by Handle() in Phase 7.4.
	if e.Type == parser.EventEntityUpdate && e.EntityID != 0 {
		if ct := e.Tags["CARDTYPE"]; ct != "" {
			b.entityTypes[action.EntityID(e.EntityID)] = ct
		}
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
	cardID := e.CardID
	// Fall back to the cached card ID for entities seen in earlier FULL_ENTITY blocks.
	if cardID == "" && e.EntityID != 0 {
		cardID = b.entityCards[action.EntityID(e.EntityID)]
	}
	return action.ActionBase{
		Card:        b.cardInfo(cardID),
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
	b.entityTypes = make(map[action.EntityID]string)

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

// buildReconnect handles EventGameEntityTags: emits a ReconnectAction carrying the
// current STATE and TURN values so OnReconnect can detect mid-game reconnects.
func (b *ActionBuilder) buildReconnect(e parser.GameEvent) action.Action {
	state := e.Tags["STATE"]
	turn := tagInt(e.Tags, "TURN")
	return &action.ReconnectAction{
		ActionBase:          b.base(e),
		IsRunning:           state == "RUNNING",
		Turn:                turn,
		PunishLeaversActive: e.Tags["BACON_DUOS_PUNISH_LEAVERS"] == "1",
	}
}

// buildTagChange handles EventTagChange events for migrated player/economy tags.
// Buff-source tags (Bloodgem, Elemental, TavernSpell) and economy counters are
// recruit-phase only. BACON_PLAYER_EXTRA_GOLD_NEXT_TURN is special: it fires
// during both recruit (normal gold grant) and combat (Overconfidence mechanic),
// so we emit the appropriate action type per phase.
func (b *ActionBuilder) buildTagChange(e parser.GameEvent) action.Action {
	for tag, rawStr := range e.Tags {
		switch tag {
		case "BACON_BLOODGEMBUFFATKVALUE", "BACON_BLOODGEMBUFFHEALTHVALUE",
			"BACON_ELEMENTAL_BUFFATKVALUE", "BACON_ELEMENTAL_BUFFHEALTHVALUE",
			"TAVERN_SPELL_ATTACK_INCREASE", "TAVERN_SPELL_HEALTH_INCREASE":
			if b.phase != action.PhaseRecruit {
				return nil
			}
			val, _ := strconv.Atoi(rawStr)
			return &action.PlayerTagChangedAction{
				ActionBase:   b.base(e),
				Tag:          tag,
				Value:        val,
				ControllerID: e.PlayerID,
				EntityName:   e.EntityName,
			}

		case "BACON_FREE_REFRESH_COUNT":
			if b.phase != action.PhaseRecruit {
				return nil
			}
			raw, _ := strconv.Atoi(rawStr)
			return &action.EconomyChangedAction{
				ActionBase:   b.base(e),
				Tag:          tag,
				Value:        raw,
				ControllerID: e.PlayerID,
				EntityName:   e.EntityName,
			}

		case "BACON_PLAYER_EXTRA_GOLD_NEXT_TURN":
			raw, _ := strconv.Atoi(rawStr)
			if raw < 0 {
				raw = 0
			}
			if b.phase == action.PhaseCombat {
				// Overconfidence and rally spells grant gold during combat.
				return &action.CombatEconomyEffectAction{
					ActionBase:   b.base(e),
					Tag:          tag,
					Value:        raw,
					ControllerID: e.PlayerID,
				}
			}
			if b.phase == action.PhaseRecruit {
				return &action.EconomyChangedAction{
					ActionBase:   b.base(e),
					Tag:          tag,
					Value:        raw,
					ControllerID: e.PlayerID,
					EntityName:   e.EntityName,
				}
			}
			return nil

		case "PLAYER_TRIPLES":
			val, _ := strconv.Atoi(rawStr)
			return &action.PlayerTriplesChangedAction{
				ActionBase:   b.base(e),
				Value:        val,
				ControllerID: e.PlayerID,
				EntityName:   e.EntityName,
			}

		case "RESOURCES", "RESOURCES_USED":
			val, _ := strconv.Atoi(rawStr)
			return &action.GoldChangedAction{
				ActionBase:   b.base(e),
				Tag:          tag,
				Value:        val,
				ControllerID: e.PlayerID,
				EntityName:   e.EntityName,
			}

		case "PLAYER_TECH_LEVEL", "TAVERN_TIER":
			tier, _ := strconv.Atoi(rawStr)
			return &action.TavernTierChangedAction{
				ActionBase:   b.base(e),
				Tag:          tag,
				Tier:         tier,
				ControllerID: e.PlayerID,
				EntityName:   e.EntityName,
			}

		case "PLAYER_LEADERBOARD_PLACE":
			val, _ := strconv.Atoi(rawStr)
			return &action.PlacementChangedAction{
				ActionBase:   b.base(e),
				Value:        val,
				ControllerID: e.PlayerID,
				EntityName:   e.EntityName,
			}

		case "3809":
			val, _ := strconv.Atoi(rawStr)
			return &action.SpellcraftChangedAction{
				ActionBase:   b.base(e),
				Value:        val,
				ControllerID: e.PlayerID,
				EntityName:   e.EntityName,
			}

		case "TURN":
			val, _ := strconv.Atoi(rawStr)
			return &action.PlayerBGTurnChangedAction{
				ActionBase:   b.base(e),
				Value:        val,
				ControllerID: e.PlayerID,
				EntityName:   e.EntityName,
			}

		case "BACON_DUO_TEAMMATE_PLAYER_ID":
			pid, _ := strconv.Atoi(rawStr)
			if pid > 0 {
				return &action.DuosTeammateAction{
					ActionBase:      b.base(e),
					PartnerPlayerID: pid,
					ControllerID:    e.PlayerID,
					EntityName:      e.EntityName,
				}
			}
			return nil

		case "HERO_ENTITY":
			heroID, _ := strconv.Atoi(rawStr)
			if heroID > 0 {
				return &action.HeroEntityAssignedAction{
					ActionBase:   b.base(e),
					HeroEntityID: heroID,
					ControllerID: e.PlayerID,
					EntityName:   e.EntityName,
				}
			}
			return nil

		case "PROPOSED_ATTACKER":
			if b.phase != action.PhaseCombat {
				return nil
			}
			if e.EntityName != "GameEntity" {
				return nil
			}
			attackerID, _ := strconv.Atoi(rawStr)
			isHero := attackerID > 0 && b.entityTypes[action.EntityID(attackerID)] == "HERO"
			return &action.CombatAttackerAction{
				ActionBase:     b.base(e),
				AttackerID:     attackerID,
				IsHeroAttacker: isHero,
			}

		case "PROPOSED_DEFENDER":
			if b.phase != action.PhaseCombat {
				return nil
			}
			if e.EntityName != "GameEntity" {
				return nil
			}
			defenderID, _ := strconv.Atoi(rawStr)
			if defenderID <= 0 {
				return nil
			}
			isHero := b.entityTypes[action.EntityID(defenderID)] == "HERO"
			return &action.CombatDefenderAction{
				ActionBase:     b.base(e),
				DefenderID:     defenderID,
				IsHeroDefender: isHero,
			}

		case "BACON_DUO_PASSABLE":
			val, _ := strconv.Atoi(rawStr)
			return &action.DuosPassableChangedAction{
				ActionBase: b.base(e),
				Value:      val,
			}

		case "BACON_DUOS_PUNISH_LEAVERS":
			val, _ := strconv.Atoi(rawStr)
			return &action.DuosPunishLeaversChangedAction{
				ActionBase: b.base(e),
				Value:      val,
			}

		case "CONTROLLER":
			ctrl, _ := strconv.Atoi(rawStr)
			if e.EntityID > 0 {
				return &action.EntityControllerChangedAction{
					ActionBase:   b.base(e),
					ControllerID: ctrl,
				}
			}
			return nil

		case "BACON_CURRENT_COMBAT_PLAYER_ID":
			combatPlayerID, _ := strconv.Atoi(rawStr)
			return &action.CombatPlayerChangedAction{
				ActionBase:     b.base(e),
				CombatPlayerID: combatPlayerID,
				ControllerID:   e.PlayerID,
				EntityName:     e.EntityName,
			}
		}
	}
	return nil
}
