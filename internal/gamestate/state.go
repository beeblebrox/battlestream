// Package gamestate maintains the in-memory Battlegrounds game state machine.
package gamestate

import (
	"sync"
	"time"
)

// GamePhase represents the current phase of a BG game.
type GamePhase string

const (
	PhaseIdle     GamePhase = "IDLE"
	PhaseLobby    GamePhase = "LOBBY"
	PhaseRecruit  GamePhase = "RECRUIT"
	PhaseCombat   GamePhase = "COMBAT"
	PhaseGameOver GamePhase = "GAME_OVER"
)

// BGGameState is the current state of a Battlegrounds game.
type BGGameState struct {
	GameID         string
	Phase          GamePhase
	Turn           int
	TavernTier     int
	Player         PlayerState
	Opponent       *PlayerState
	Board          []MinionState
	OpponentBoard  []MinionState
	Modifications  []StatMod
	StartTime      time.Time
	EndTime        *time.Time
	Placement      int
}

// PlayerState holds per-player stats.
type PlayerState struct {
	Name        string `json:"name"`
	HeroCardID  string `json:"hero_card_id"`
	Health      int    `json:"health"`
	Armor       int    `json:"armor"`
	CurrentGold int    `json:"current_gold"`
	SpellPower  int    `json:"spell_power"`
	TripleCount int    `json:"triple_count"`
	WinStreak   int    `json:"win_streak"`
	LossStreak  int    `json:"loss_streak"`
}

// MinionState describes a single minion on the board.
type MinionState struct {
	EntityID   int    `json:"entity_id"`
	CardID     string `json:"card_id"`
	Name       string `json:"name"`
	Attack     int    `json:"attack"`
	Health     int    `json:"health"`
	MinionType string `json:"minion_type"`
	BuffAttack int    `json:"buff_attack"`
	BuffHealth int    `json:"buff_health"`
}

// StatMod records a buff or nerf applied during the game.
type StatMod struct {
	Turn   int    `json:"turn"`
	Target string `json:"target"` // "ALL", "BEAST", "MECH", entity name, etc.
	Stat   string `json:"stat"`   // "ATTACK", "HEALTH", "SPELL_POWER"
	Delta  int    `json:"delta"`
	Source string `json:"source"` // card name
}

// Machine manages the BGGameState and applies events.
type Machine struct {
	mu    sync.RWMutex
	state BGGameState
}

// New creates a new Machine in IDLE phase.
func New() *Machine {
	return &Machine{
		state: BGGameState{Phase: PhaseIdle},
	}
}

// State returns a snapshot of the current game state.
func (m *Machine) State() BGGameState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.state
	// Deep copy slices
	s.Board = append([]MinionState(nil), m.state.Board...)
	s.OpponentBoard = append([]MinionState(nil), m.state.OpponentBoard...)
	s.Modifications = append([]StatMod(nil), m.state.Modifications...)
	return s
}

// GameStart initialises a new game.
func (m *Machine) GameStart(gameID string, t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = BGGameState{
		GameID:    gameID,
		Phase:     PhaseLobby,
		StartTime: t,
		Player:    PlayerState{Health: 40},
	}
}

// GameEnd marks the game as over.
func (m *Machine) GameEnd(placement int, t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Phase = PhaseGameOver
	m.state.Placement = placement
	m.state.EndTime = &t
}

// SetPhase updates the game phase.
func (m *Machine) SetPhase(phase GamePhase) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Phase = phase
}

// SetTurn updates the turn counter.
func (m *Machine) SetTurn(turn int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Turn = turn
	if turn%2 == 1 {
		m.state.Phase = PhaseRecruit
	} else {
		m.state.Phase = PhaseCombat
	}
}

// UpdatePlayerTag applies a tag change to the local player state.
func (m *Machine) UpdatePlayerTag(tag, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	applyTagToPlayer(&m.state.Player, tag, value)
}

// UpdatePlayerName sets the player name.
func (m *Machine) UpdatePlayerName(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Player.Name = name
}

// UpsertMinion inserts or updates a minion on the board.
func (m *Machine) UpsertMinion(minion MinionState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, mn := range m.state.Board {
		if mn.EntityID == minion.EntityID {
			m.state.Board[i] = minion
			return
		}
	}
	m.state.Board = append(m.state.Board, minion)
}

// RemoveMinion removes a minion from the board by entity ID.
func (m *Machine) RemoveMinion(entityID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	board := m.state.Board[:0]
	for _, mn := range m.state.Board {
		if mn.EntityID != entityID {
			board = append(board, mn)
		}
	}
	m.state.Board = board
}

// AddMod records a stat modification for this game.
func (m *Machine) AddMod(mod StatMod) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Modifications = append(m.state.Modifications, mod)
}

// SetTavernTier updates the current tavern tier.
func (m *Machine) SetTavernTier(tier int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.TavernTier = tier
}

func applyTagToPlayer(p *PlayerState, tag, value string) {
	switch tag {
	case "HEALTH":
		p.Health = parseInt(value)
	case "ARMOR":
		p.Armor = parseInt(value)
	case "SPELL_POWER":
		p.SpellPower = parseInt(value)
	case "TAVERN_TIER", "PLAYER_TECH_LEVEL":
		// handled separately
	case "BACON_TRIPLE_CARD":
		p.TripleCount++
	}
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
