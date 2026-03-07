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
	BuffSources     []BuffSource     `json:"buff_sources,omitempty"`
	AbilityCounters []AbilityCounter `json:"ability_counters,omitempty"`
	Enchantments    []Enchantment    `json:"enchantments,omitempty"`
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
	EntityID     int           `json:"entity_id"`
	CardID       string        `json:"card_id"`
	Name         string        `json:"name"`
	Attack       int           `json:"attack"`
	Health       int           `json:"health"`
	MinionType   string        `json:"minion_type"`
	BuffAttack   int           `json:"buff_attack"`
	BuffHealth   int           `json:"buff_health"`
	Enchantments []Enchantment `json:"enchantments,omitempty"`
}

// StatMod records a buff or nerf applied during the game.
type StatMod struct {
	Turn     int    `json:"turn"`
	Target   string `json:"target"`              // "ALL", "BEAST", "MECH", entity name, etc.
	Stat     string `json:"stat"`                // "ATTACK", "HEALTH", "SPELL_POWER"
	Delta    int    `json:"delta"`
	Source   string `json:"source"`              // card name
	Category string `json:"category,omitempty"`  // buff category label
	CardID   string `json:"card_id,omitempty"`   // source card ID
}

// BuffSource holds the current effective value for a buff category.
type BuffSource struct {
	Category string `json:"category"` // "BLOODGEM", "NOMI", "WHELP", etc.
	Attack   int    `json:"attack"`   // current effective ATK buff value
	Health   int    `json:"health"`   // current effective HP buff value
}

// Enchantment represents a single enchantment entity attached to a minion.
type Enchantment struct {
	EntityID     int    `json:"entity_id"`
	CardID       string `json:"card_id"`
	SourceCardID string `json:"source_card_id"`
	SourceName   string `json:"source_name"`
	TargetID     int    `json:"target_id"`  // ATTACHED entity ID
	AttackBuff   int    `json:"attack_buff"`
	HealthBuff   int    `json:"health_buff"`
	Category     string `json:"category"`
}

// AbilityCounter tracks a non-stat ability value (e.g. Spellcraft stacks).
type AbilityCounter struct {
	Category string `json:"category"`
	Value    int    `json:"value"`   // raw tag value
	Display  string `json:"display"` // computed display string
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
	s.Board = make([]MinionState, len(m.state.Board))
	for i, mn := range m.state.Board {
		s.Board[i] = mn
		s.Board[i].Enchantments = append([]Enchantment(nil), mn.Enchantments...)
	}
	s.OpponentBoard = append([]MinionState(nil), m.state.OpponentBoard...)
	s.Modifications = append([]StatMod(nil), m.state.Modifications...)
	s.BuffSources = append([]BuffSource(nil), m.state.BuffSources...)
	s.AbilityCounters = append([]AbilityCounter(nil), m.state.AbilityCounters...)
	s.Enchantments = append([]Enchantment(nil), m.state.Enchantments...)
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
	// Always restore the pre-combat board snapshot — combat replaces recruit
	// entities with simulation copies that have base stats, so the snapshot
	// has the correct fully-buffed stats.
	if len(m.boardSnapshot) > 0 {
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
		// Snapshot the board before combat — minions die during combat and
		// are replaced by simulation copies with base stats. The recruit
		// board has the fully-buffed stats we want to preserve.
		m.boardSnapshot = append([]MinionState(nil), m.state.Board...)
		m.state.Phase = PhaseCombat
	}
}

// UpdateBoardSnapshot overwrites the board snapshot with the current board.
// Called during combat to keep the snapshot in sync as combat copies are added.
func (m *Machine) UpdateBoardSnapshot() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.boardSnapshot = append([]MinionState(nil), m.state.Board...)
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

// UpdateMinionStat updates a specific stat on a board minion.
// Returns true if the minion was found and updated.
func (m *Machine) UpdateMinionStat(entityID int, stat string, value int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, mn := range m.state.Board {
		if mn.EntityID == entityID {
			switch stat {
			case "ATK":
				m.state.Board[i].Attack = value
			case "HEALTH":
				m.state.Board[i].Health = value
			}
			return true
		}
	}
	return false
}

// currentTurn returns the current display turn (thread-safe).
func (m *Machine) currentTurn() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.Turn
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

// SetBuffSource upserts a buff source category with its current values.
func (m *Machine) SetBuffSource(category string, atk, hp int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, bs := range m.state.BuffSources {
		if bs.Category == category {
			m.state.BuffSources[i].Attack = atk
			m.state.BuffSources[i].Health = hp
			return
		}
	}
	m.state.BuffSources = append(m.state.BuffSources, BuffSource{
		Category: category,
		Attack:   atk,
		Health:   hp,
	})
}

// SetAbilityCounter upserts an ability counter by category.
func (m *Machine) SetAbilityCounter(category string, value int, display string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, ac := range m.state.AbilityCounters {
		if ac.Category == category {
			m.state.AbilityCounters[i].Value = value
			m.state.AbilityCounters[i].Display = display
			return
		}
	}
	m.state.AbilityCounters = append(m.state.AbilityCounters, AbilityCounter{
		Category: category,
		Value:    value,
		Display:  display,
	})
}

// AddEnchantment registers an enchantment and attaches it to the target minion.
func (m *Machine) AddEnchantment(ench Enchantment) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Update existing enchantment if entity ID matches (script data update).
	for i, e := range m.state.Enchantments {
		if e.EntityID == ench.EntityID {
			m.state.Enchantments[i] = ench
			m.attachEnchantmentToMinion(ench)
			return
		}
	}
	m.state.Enchantments = append(m.state.Enchantments, ench)
	m.attachEnchantmentToMinion(ench)
}

// attachEnchantmentToMinion adds or updates the enchantment on its target board minion.
// Must be called with m.mu held.
func (m *Machine) attachEnchantmentToMinion(ench Enchantment) {
	for i, mn := range m.state.Board {
		if mn.EntityID == ench.TargetID {
			for j, e := range mn.Enchantments {
				if e.EntityID == ench.EntityID {
					m.state.Board[i].Enchantments[j] = ench
					return
				}
			}
			m.state.Board[i].Enchantments = append(m.state.Board[i].Enchantments, ench)
			return
		}
	}
}

// RemoveEnchantmentsForEntity removes all enchantments targeting the given entity.
func (m *Machine) RemoveEnchantmentsForEntity(entityID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	filtered := m.state.Enchantments[:0]
	for _, e := range m.state.Enchantments {
		if e.TargetID != entityID {
			filtered = append(filtered, e)
		}
	}
	m.state.Enchantments = filtered
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
