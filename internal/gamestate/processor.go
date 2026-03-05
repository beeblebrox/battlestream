package gamestate

import (
	"fmt"
	"strconv"

	"battlestream.fixates.io/internal/parser"
)

// Processor consumes parser.GameEvents and updates a Machine.
type Processor struct {
	machine  *Machine
	gameSeq  int
}

// NewProcessor returns a Processor that updates the given Machine.
func NewProcessor(m *Machine) *Processor {
	return &Processor{machine: m}
}

// Handle processes a single GameEvent and updates the game state.
func (p *Processor) Handle(e parser.GameEvent) {
	switch e.Type {
	case parser.EventGameStart:
		p.gameSeq++
		gameID := fmt.Sprintf("game-%d", p.gameSeq)
		p.machine.GameStart(gameID, e.Timestamp)

	case parser.EventGameEnd:
		result := e.Tags["GAME_RESULT"]
		placement := 0
		if pl, ok := e.Tags["PLAYER_LEADERBOARD_PLACE"]; ok {
			placement, _ = strconv.Atoi(pl)
		}
		_ = result
		p.machine.GameEnd(placement, e.Timestamp)

	case parser.EventTurnStart:
		if t, ok := e.Tags["TURN"]; ok {
			turn, _ := strconv.Atoi(t)
			p.machine.SetTurn(turn)
		}

	case parser.EventTagChange:
		p.handleTagChange(e)

	case parser.EventEntityUpdate:
		p.handleEntityUpdate(e)
	}
}

func (p *Processor) handleTagChange(e parser.GameEvent) {
	for tag, value := range e.Tags {
		switch tag {
		case "HEALTH", "ARMOR", "SPELL_POWER":
			p.machine.UpdatePlayerTag(tag, value)
		case "PLAYER_TECH_LEVEL", "TAVERN_TIER":
			tier, _ := strconv.Atoi(value)
			if tier > 0 {
				p.machine.SetTavernTier(tier)
			}
		case "BACON_TRIPLE_CARD":
			p.machine.UpdatePlayerTag(tag, value)
		}
	}
}

func (p *Processor) handleEntityUpdate(e parser.GameEvent) {
	// Minimal: create/update a minion on the board if it has ATK/HEALTH tags
	if e.EntityID == 0 {
		return
	}
	atk, hasAtk := e.Tags["ATK"]
	hp, hasHp := e.Tags["HEALTH"]
	if !hasAtk && !hasHp {
		return
	}
	minion := MinionState{
		EntityID: e.EntityID,
		CardID:   e.CardID,
		Name:     e.EntityName,
	}
	if hasAtk {
		minion.Attack = parseInt(atk)
	}
	if hasHp {
		minion.Health = parseInt(hp)
	}
	p.machine.UpsertMinion(minion)
}
