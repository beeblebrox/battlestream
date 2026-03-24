// Package gamestate maintains the in-memory Battlegrounds game state machine.
package gamestate

import (
	"log/slog"
	"strconv"
	"strings"
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
	GameID        string        `json:"game_id"`
	Phase         GamePhase     `json:"phase"`
	Turn          int           `json:"turn"`          // The BG turn the player sees (from player TURN tag)
	TavernTier    int           `json:"tavern_tier"`
	Player        PlayerState   `json:"player"`
	Opponent      *PlayerState  `json:"opponent,omitempty"`
	Board         []MinionState `json:"board,omitempty"`
	OpponentBoard []MinionState `json:"opponent_board,omitempty"`
	Modifications []StatMod     `json:"modifications,omitempty"`
	BuffSources     []BuffSource     `json:"buff_sources,omitempty"`
	AbilityCounters []AbilityCounter `json:"ability_counters,omitempty"`
	Enchantments    []Enchantment    `json:"enchantments,omitempty"`
	AvailableTribes []string         `json:"available_tribes,omitempty"`
	AnomalyCardID      string           `json:"anomaly_card_id,omitempty"`
	AnomalyName        string           `json:"anomaly_name,omitempty"`
	AnomalyDescription string           `json:"anomaly_description,omitempty"`
	StartTime     time.Time  `json:"start_time"`
	EndTime       *time.Time `json:"end_time,omitempty"`
	Placement     int        `json:"placement"`

	// Duos fields
	IsDuos                 bool             `json:"is_duos,omitempty"`
	Partner                *PlayerState     `json:"partner,omitempty"`
	PartnerBoard           *PartnerBoard    `json:"partner_board,omitempty"`
	PartnerBuffSources     []BuffSource     `json:"partner_buff_sources,omitempty"`
	PartnerAbilityCounters []AbilityCounter `json:"partner_ability_counters,omitempty"`
}

// PartnerBoard holds the last-seen partner board snapshot from combat copies.
type PartnerBoard struct {
	Minions []MinionState `json:"minions"`
	Turn    int           `json:"turn"`
	Stale   bool          `json:"stale"`
}

// PlayerState holds per-player stats.
type PlayerState struct {
	Name        string `json:"name"`
	HeroCardID  string `json:"hero_card_id"`
	Health      int    `json:"health"`
	MaxHealth   int    `json:"max_health"`
	Damage      int    `json:"damage"`
	Armor       int    `json:"armor"`
	CurrentGold int    `json:"current_gold"`
	MaxGold     int    `json:"max_gold"`
	SpellPower  int    `json:"spell_power"`
	TripleCount int    `json:"triple_count"`
	TavernTier  int    `json:"tavern_tier"`
	WinStreak   int    `json:"win_streak"`
	LossStreak  int    `json:"loss_streak"`
}

// EffectiveHealth returns the current effective health (Health - Damage).
func (p PlayerState) EffectiveHealth() int {
	return p.Health - p.Damage
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

// TurnSnapshot captures the game state at the end of a recruit phase turn,
// plus the deltas (buff changes) that occurred during that turn with source attribution.
type TurnSnapshot struct {
	Turn          int              `json:"turn"`
	State         BGGameState      `json:"state"`
	BuffDeltas    []BuffDelta      `json:"buff_deltas,omitempty"`
	AbilityDeltas []AbilityDelta   `json:"ability_deltas,omitempty"`
	Modifications []StatMod        `json:"modifications,omitempty"`
}

// BuffDelta records the change in a buff source for a single turn.
type BuffDelta struct {
	Category    string `json:"category"`
	AttackDelta int    `json:"attack_delta"`
	HealthDelta int    `json:"health_delta"`
	Source      string `json:"source,omitempty"`
	CardID      string `json:"card_id,omitempty"`
}

// AbilityDelta records the change in an ability counter for a single turn.
type AbilityDelta struct {
	Category   string `json:"category"`
	ValueDelta int    `json:"value_delta"`
	Source     string `json:"source,omitempty"`
}

// Machine manages the BGGameState and applies events.
type Machine struct {
	mu              sync.RWMutex
	state           BGGameState
	gameEntityTurn  int           // internal doubled turn from GameEntity
	boardSnapshot   []MinionState // board state before combat, restored on game over
	goldTotal       int           // last RESOURCES value
	goldUsed        int           // last RESOURCES_USED value

	// Partner (Duos) state
	partnerGoldTotal int
	partnerGoldUsed  int

	// Per-turn snapshot accumulation
	turnSnapshots   []TurnSnapshot
	prevBuffSources []BuffSource
	prevAbilityCtrs []AbilityCounter
	prevModCount    int
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
	s.PartnerBuffSources = append([]BuffSource(nil), m.state.PartnerBuffSources...)
	s.PartnerAbilityCounters = append([]AbilityCounter(nil), m.state.PartnerAbilityCounters...)
	s.Enchantments = append([]Enchantment(nil), m.state.Enchantments...)
	s.AvailableTribes = append([]string(nil), m.state.AvailableTribes...)
	// Deep copy partner
	if m.state.Partner != nil {
		p := *m.state.Partner
		s.Partner = &p
	}
	// Deep copy partner board
	if m.state.PartnerBoard != nil {
		pb := *m.state.PartnerBoard
		pb.Minions = make([]MinionState, len(m.state.PartnerBoard.Minions))
		for i, mn := range m.state.PartnerBoard.Minions {
			pb.Minions[i] = mn
			pb.Minions[i].Enchantments = append([]Enchantment(nil), mn.Enchantments...)
		}
		s.PartnerBoard = &pb
	}
	return s
}

// GameStart initialises a new game.
func (m *Machine) GameStart(gameID string, t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Reset all fields individually — zeroing *m while locked would destroy
	// the mutex and panic on the deferred Unlock.
	m.state = BGGameState{
		GameID:    gameID,
		Phase:     PhaseLobby,
		StartTime: t,
	}
	m.gameEntityTurn = 0
	m.boardSnapshot = nil
	m.goldTotal = 0
	m.goldUsed = 0
	m.partnerGoldTotal = 0
	m.partnerGoldUsed = 0
	m.turnSnapshots = nil
	m.prevBuffSources = nil
	m.prevAbilityCtrs = nil
	m.prevModCount = 0
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
		if len(m.boardSnapshot) > 7 {
			slog.Warn("board snapshot exceeds 7, trimming", "count", len(m.boardSnapshot))
			m.boardSnapshot = m.boardSnapshot[:7]
		}
		m.state.Board = m.boardSnapshot
	}
	// Capture final turn snapshot.
	if m.state.Turn > 0 {
		m.captureTurnSnapshot(m.state.Turn)
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
	// Capture snapshot for the previous turn before advancing.
	if m.state.Turn > 0 && turn > m.state.Turn {
		m.captureTurnSnapshot(m.state.Turn)
	}
	m.state.Turn = turn
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
		m.boardSnapshot = deepCopyBoard(m.state.Board)
		m.state.Phase = PhaseCombat
	}
}

// UpdateBoardSnapshot overwrites the board snapshot with the current board.
// Called during combat to keep the snapshot in sync as combat copies are added.
func (m *Machine) UpdateBoardSnapshot() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.boardSnapshot = deepCopyBoard(m.state.Board)
}

// deepCopyBoard returns a deep copy of a board slice, including Enchantment slices.
func deepCopyBoard(board []MinionState) []MinionState {
	if len(board) == 0 {
		return nil
	}
	cp := make([]MinionState, len(board))
	for i, mn := range board {
		cp[i] = mn
		if len(mn.Enchantments) > 0 {
			cp[i].Enchantments = make([]Enchantment, len(mn.Enchantments))
			copy(cp[i].Enchantments, mn.Enchantments)
		}
	}
	return cp
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
	if len(m.state.Board) >= 7 {
		return // BG board max is 7
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

// RecordRoundWin increments the win streak and resets the loss streak.
func (m *Machine) RecordRoundWin() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Player.WinStreak++
	m.state.Player.LossStreak = 0
}

// RecordRoundLoss increments the loss streak and resets the win streak.
func (m *Machine) RecordRoundLoss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Player.LossStreak++
	m.state.Player.WinStreak = 0
}

// SetTavernTier updates the current tavern tier.
func (m *Machine) SetTavernTier(tier int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.TavernTier = tier
}

// UpdateGold tracks RESOURCES (total) and RESOURCES_USED (spent) to compute current gold.
func (m *Machine) UpdateGold(tag string, value int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch tag {
	case "RESOURCES":
		m.goldTotal = value
		m.state.Player.MaxGold = value
	case "RESOURCES_USED":
		m.goldUsed = value
	}
	m.state.Player.CurrentGold = m.goldTotal - m.goldUsed
}

func applyTagToPlayer(p *PlayerState, tag, value string) {
	switch tag {
	case "HEALTH":
		v := parseInt(value)
		p.Health = v
		// Track max health from the first (highest) HEALTH value seen.
		if v > p.MaxHealth {
			p.MaxHealth = v
		}
	case "DAMAGE":
		p.Damage = parseInt(value)
	case "ARMOR":
		p.Armor = parseInt(value)
	case "SPELL_POWER":
		p.SpellPower = parseInt(value)
	case "TAVERN_TIER", "PLAYER_TECH_LEVEL":
		p.TavernTier = parseInt(value)
	case "PLAYER_TRIPLES":
		p.TripleCount = parseInt(value)
	}
}

// AddAvailableTribe adds a tribe to the available tribes list if not already present.
func (m *Machine) AddAvailableTribe(tribe string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.state.AvailableTribes {
		if t == tribe {
			return
		}
	}
	m.state.AvailableTribes = append(m.state.AvailableTribes, tribe)
}

// RemoveAvailableTribe removes a tribe from the available list (used when a
// provisionally-registered tribe is discovered to be from a multi-tribe entity).
func (m *Machine) RemoveAvailableTribe(tribe string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range m.state.AvailableTribes {
		if t == tribe {
			m.state.AvailableTribes = append(m.state.AvailableTribes[:i], m.state.AvailableTribes[i+1:]...)
			return
		}
	}
}

// SetAnomaly stores the anomaly card ID, resolved name, and description.
func (m *Machine) SetAnomaly(cardID, name, description string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.AnomalyCardID = cardID
	m.state.AnomalyName = name
	m.state.AnomalyDescription = description
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

// SetPartnerBuffSource upserts a partner buff source by category.
func (m *Machine) SetPartnerBuffSource(category string, atk, hp int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, bs := range m.state.PartnerBuffSources {
		if bs.Category == category {
			m.state.PartnerBuffSources[i].Attack = atk
			m.state.PartnerBuffSources[i].Health = hp
			return
		}
	}
	m.state.PartnerBuffSources = append(m.state.PartnerBuffSources, BuffSource{
		Category: category, Attack: atk, Health: hp,
	})
}

// SetPartnerAbilityCounter upserts a partner ability counter by category.
func (m *Machine) SetPartnerAbilityCounter(category string, value int, display string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, ac := range m.state.PartnerAbilityCounters {
		if ac.Category == category {
			m.state.PartnerAbilityCounters[i].Value = value
			m.state.PartnerAbilityCounters[i].Display = display
			return
		}
	}
	m.state.PartnerAbilityCounters = append(m.state.PartnerAbilityCounters, AbilityCounter{
		Category: category, Value: value, Display: display,
	})
}

// RemoveAbilityCounter removes the ability counter for the given category (no-op if absent).
func (m *Machine) RemoveAbilityCounter(category string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, ac := range m.state.AbilityCounters {
		if ac.Category == category {
			m.state.AbilityCounters = append(m.state.AbilityCounters[:i], m.state.AbilityCounters[i+1:]...)
			return
		}
	}
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

// ── Duos partner methods ─────────────────────────────────────────────────────

// SetDuosMode enables/disables Duos tracking and initializes partner state.
func (m *Machine) SetDuosMode(isDuos bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.IsDuos = isDuos
	if isDuos && m.state.Partner == nil {
		m.state.Partner = &PlayerState{}
	}
	if !isDuos {
		m.state.Partner = nil
		m.state.PartnerBoard = nil
	}
}

// UpdatePartnerTag applies a tag change to the partner player state.
func (m *Machine) UpdatePartnerTag(tag, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Partner == nil {
		m.state.Partner = &PlayerState{}
	}
	applyTagToPlayer(m.state.Partner, tag, value)
}

// UpdatePartnerName sets the partner player name.
func (m *Machine) UpdatePartnerName(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Partner == nil {
		m.state.Partner = &PlayerState{}
	}
	m.state.Partner.Name = name
}

// UpdatePartnerHeroCardID sets the hero card ID for the partner.
func (m *Machine) UpdatePartnerHeroCardID(cardID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Partner == nil {
		m.state.Partner = &PlayerState{}
	}
	m.state.Partner.HeroCardID = cardID
}


// UpdatePartnerGold tracks partner gold from RESOURCES/RESOURCES_USED.
func (m *Machine) UpdatePartnerGold(tag string, value int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Partner == nil {
		m.state.Partner = &PlayerState{}
	}
	switch tag {
	case "RESOURCES":
		m.partnerGoldTotal = value
		m.state.Partner.MaxGold = value
	case "RESOURCES_USED":
		m.partnerGoldUsed = value
	}
	m.state.Partner.CurrentGold = m.partnerGoldTotal - m.partnerGoldUsed
}

// SetPartnerBoard sets the partner board snapshot from combat copy minions.
func (m *Machine) SetPartnerBoard(minions []MinionState, turn int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(minions) > 7 {
		minions = minions[:7]
	}
	cp := make([]MinionState, len(minions))
	for i, mn := range minions {
		cp[i] = mn
		cp[i].Enchantments = append([]Enchantment(nil), mn.Enchantments...)
	}
	m.state.PartnerBoard = &PartnerBoard{
		Minions: cp,
		Turn:    turn,
		Stale:   false,
	}
}

// MarkPartnerBoardStale marks the partner board snapshot as stale (not updated this turn).
func (m *Machine) MarkPartnerBoardStale() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.PartnerBoard != nil {
		m.state.PartnerBoard.Stale = true
	}
}

// captureTurnSnapshot computes deltas and appends a snapshot for the given turn.
// Must be called with m.mu held.
func (m *Machine) captureTurnSnapshot(turn int) {
	snap := TurnSnapshot{
		Turn:  turn,
		State: m.deepCopyState(),
	}

	// Compute buff deltas.
	prevMap := make(map[string]BuffSource)
	for _, bs := range m.prevBuffSources {
		prevMap[bs.Category] = bs
	}
	for _, bs := range m.state.BuffSources {
		prev := prevMap[bs.Category]
		atkDelta := bs.Attack - prev.Attack
		hpDelta := bs.Health - prev.Health
		if atkDelta != 0 || hpDelta != 0 {
			snap.BuffDeltas = append(snap.BuffDeltas, BuffDelta{
				Category:    bs.Category,
				AttackDelta: atkDelta,
				HealthDelta: hpDelta,
			})
		}
	}

	// Compute ability deltas.
	prevAC := make(map[string]AbilityCounter)
	for _, ac := range m.prevAbilityCtrs {
		prevAC[ac.Category] = ac
	}
	for _, ac := range m.state.AbilityCounters {
		prev := prevAC[ac.Category]
		valDelta := ac.Value - prev.Value
		if valDelta != 0 {
			snap.AbilityDeltas = append(snap.AbilityDeltas, AbilityDelta{
				Category:   ac.Category,
				ValueDelta: valDelta,
			})
		}
	}

	// Capture modifications from this turn only.
	if m.prevModCount < len(m.state.Modifications) {
		snap.Modifications = append([]StatMod(nil), m.state.Modifications[m.prevModCount:]...)
	}

	m.turnSnapshots = append(m.turnSnapshots, snap)

	// Update prev state for next turn's delta computation.
	m.prevBuffSources = append([]BuffSource(nil), m.state.BuffSources...)
	m.prevAbilityCtrs = append([]AbilityCounter(nil), m.state.AbilityCounters...)
	m.prevModCount = len(m.state.Modifications)
}

// deepCopyState returns a deep copy of the current state (without lock).
func (m *Machine) deepCopyState() BGGameState {
	s := m.state
	s.Board = make([]MinionState, len(m.state.Board))
	for i, mn := range m.state.Board {
		s.Board[i] = mn
		s.Board[i].Enchantments = append([]Enchantment(nil), mn.Enchantments...)
	}
	s.OpponentBoard = append([]MinionState(nil), m.state.OpponentBoard...)
	s.Modifications = append([]StatMod(nil), m.state.Modifications...)
	s.BuffSources = append([]BuffSource(nil), m.state.BuffSources...)
	s.AbilityCounters = append([]AbilityCounter(nil), m.state.AbilityCounters...)
	s.PartnerBuffSources = append([]BuffSource(nil), m.state.PartnerBuffSources...)
	s.PartnerAbilityCounters = append([]AbilityCounter(nil), m.state.PartnerAbilityCounters...)
	s.Enchantments = append([]Enchantment(nil), m.state.Enchantments...)
	s.AvailableTribes = append([]string(nil), m.state.AvailableTribes...)
	if m.state.Partner != nil {
		p := *m.state.Partner
		s.Partner = &p
	}
	if m.state.PartnerBoard != nil {
		pb := *m.state.PartnerBoard
		pb.Minions = make([]MinionState, len(m.state.PartnerBoard.Minions))
		for i, mn := range m.state.PartnerBoard.Minions {
			pb.Minions[i] = mn
			pb.Minions[i].Enchantments = append([]Enchantment(nil), mn.Enchantments...)
		}
		s.PartnerBoard = &pb
	}
	return s
}

// TurnSnapshots returns the accumulated per-turn snapshots for the current game.
func (m *Machine) TurnSnapshots() []TurnSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]TurnSnapshot(nil), m.turnSnapshots...)
}

func parseInt(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
