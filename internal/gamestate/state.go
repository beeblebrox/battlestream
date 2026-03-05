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
	GameID        string
	Phase         GamePhase
	Turn          int // The BG turn the player sees (from player TURN tag)
	TavernTier    int
	Player        PlayerState
	Opponent      *PlayerState
	Board         []MinionState
	OpponentBoard []MinionState
	Modifications []StatMod
	StartTime     time.Time
	EndTime       *time.Time
	Placement     int
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
	mu              sync.RWMutex
	state           BGGameState
	gameEntityTurn  int // internal doubled turn from GameEntity
	boardSnapshot   []MinionState // board state before combat, restored on game over
}

// New creates a new Machine in IDLE phase.
func New() *Machine {
	return &Machine{
		state: BGGameState{Phase: PhaseIdle},
	}
}

// Phase returns the current game phase.
func (m *Machine) Phase() GamePhase {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.Phase
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
	m.gameEntityTurn = 0
}

// GameEnd marks the game as over.
func (m *Machine) GameEnd(placement int, t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Phase = PhaseGameOver
	m.state.Placement = placement
	m.state.EndTime = &t
	// Restore pre-combat board if the current board is empty (minions died during combat).
	if len(m.state.Board) == 0 && len(m.boardSnapshot) > 0 {
		m.state.Board = m.boardSnapshot
	}
	m.boardSnapshot = nil
}

// SetPhase updates the game phase.
func (m *Machine) SetPhase(phase GamePhase) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Phase = phase
}

// SetTurn sets the BG turn (the turn number the player sees).
// This comes from TAG_CHANGE Entity=<localPlayer> tag=TURN value=N.
func (m *Machine) SetTurn(turn int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Turn = turn
	// Player turn always starts a recruit phase.
	m.state.Phase = PhaseRecruit
}

// SetGameEntityTurn tracks the internal GameEntity TURN (doubled: odd=recruit, even=combat).
// Used only for phase detection, not for display.
func (m *Machine) SetGameEntityTurn(turn int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gameEntityTurn = turn
	// Use GameEntity turn for phase: odd = recruit, even = combat.
	if turn%2 == 1 {
		m.state.Phase = PhaseRecruit
	} else {
		m.state.Phase = PhaseCombat
		// Snapshot the board before combat starts — minions die during combat
		// and we want to preserve the pre-combat board for game-over display.
		m.boardSnapshot = append([]MinionState(nil), m.state.Board...)
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

// UpdateHeroCardID sets the hero card ID for the local player.
func (m *Machine) UpdateHeroCardID(cardID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Player.HeroCardID = cardID
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
	case "PLAYER_TRIPLES":
		p.TripleCount = parseInt(value)
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
