package action

import "time"

// ── Transition / lifecycle actions ───────────────────────────────────────────
// These actions are dispatched regardless of current phase and represent game
// lifecycle events: start, turn change, game over, reconnect, player identity.

// GameStartAction fires when a new game CREATE_GAME block is processed.
type GameStartAction struct {
	ActionBase
	GameID      string
	Timestamp   time.Time
	IsReconnect bool
}

func (a *GameStartAction) actionMarker()                              {}
func (a *GameStartAction) AcceptTransition(v TransitionVisitor) error { return v.OnGameStart(a) }

// TurnTransitionAction fires when GameEntity TURN changes (every half-turn).
// NewPhase reflects the result: odd GameEntityTurn → PhaseRecruit, even → PhaseCombat.
// OnTurnTransition must flush pendingStatChanges before advancing the phase.
type TurnTransitionAction struct {
	ActionBase
	NewPhase       GamePhase
	GameEntityTurn int // raw GameEntity TURN (doubled; odd=recruit, even=combat)
}

func (a *TurnTransitionAction) actionMarker() {}
func (a *TurnTransitionAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnTurnTransition(a)
}

// GameEndAction fires when GameEntity STATE=COMPLETE is seen.
type GameEndAction struct {
	ActionBase
	Placement int
	Timestamp time.Time
}

func (a *GameEndAction) actionMarker()                              {}
func (a *GameEndAction) AcceptTransition(v TransitionVisitor) error { return v.OnGameEnd(a) }

// ReconnectAction fires when EventGameEntityTags confirms STATE=RUNNING + TURN > 1.
// IsRunning is true when the game state tag is "RUNNING"; Turn is the TURN tag value.
// OnReconnect is a no-op unless both IsRunning and Turn > 1 are true.
// PunishLeaversActive is true when BACON_DUOS_PUNISH_LEAVERS=1 is present in the
// EventGameEntityTags block (needed for the backup duos detection conjunction check).
type ReconnectAction struct {
	ActionBase
	IsRunning           bool
	Turn                int
	PunishLeaversActive bool // BACON_DUOS_PUNISH_LEAVERS=1 present in EventGameEntityTags
}

func (a *ReconnectAction) actionMarker()                              {}
func (a *ReconnectAction) AcceptTransition(v TransitionVisitor) error { return v.OnReconnect(a) }

// PlayerDefAction carries all data from a CREATE_GAME Player entity block.
// AccountHi non-zero identifies a real (local or partner) player vs. a bot.
type PlayerDefAction struct {
	ActionBase
	PlayerID      int
	BattleTag     string
	AccountHi     uint64 // non-zero = real player
	HeroEntityID  int    // HERO_ENTITY tag (0 if absent)
	DuoTeammateID int    // BACON_DUO_TEAMMATE_PLAYER_ID (0 if absent)
	InitialTurn   int    // TURN tag on player entity (>0 during reconnect replays)
	Resources     int    // RESOURCES tag (max gold for turn)
	ResourcesUsed int    // RESOURCES_USED tag (gold spent)
}

func (a *PlayerDefAction) actionMarker()                              {}
func (a *PlayerDefAction) AcceptTransition(v TransitionVisitor) error { return v.OnPlayerDef(a) }

// PlayerNameAction fires when a DebugPrintGame PlayerID → BattleTag mapping arrives.
type PlayerNameAction struct {
	ActionBase
	PlayerID  int
	BattleTag string
}

func (a *PlayerNameAction) actionMarker()                              {}
func (a *PlayerNameAction) AcceptTransition(v TransitionVisitor) error { return v.OnPlayerName(a) }

// PlayerTriplesChangedAction fires when PLAYER_TRIPLES changes on a hero or player entity.
// Value is the cumulative triple count (absolute, not delta).
type PlayerTriplesChangedAction struct {
	ActionBase
	Value        int
	ControllerID int
	EntityName   string
}

func (a *PlayerTriplesChangedAction) actionMarker() {}
func (a *PlayerTriplesChangedAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnPlayerTriplesChanged(a)
}

// GoldChangedAction fires when RESOURCES or RESOURCES_USED changes on a player entity.
// Tag is "RESOURCES" (total gold) or "RESOURCES_USED" (spent gold).
type GoldChangedAction struct {
	ActionBase
	Tag          string
	Value        int
	ControllerID int
	EntityName   string
}

func (a *GoldChangedAction) actionMarker()                              {}
func (a *GoldChangedAction) AcceptTransition(v TransitionVisitor) error { return v.OnGoldChanged(a) }

// TavernTierChangedAction fires when PLAYER_TECH_LEVEL or TAVERN_TIER changes.
// Tag distinguishes the source tag; Tier is the new tier value (skipped if 0).
type TavernTierChangedAction struct {
	ActionBase
	Tag          string
	Tier         int
	ControllerID int
	EntityName   string
}

func (a *TavernTierChangedAction) actionMarker() {}
func (a *TavernTierChangedAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnTavernTierChanged(a)
}

// PlacementChangedAction fires when PLAYER_LEADERBOARD_PLACE changes for a local player entity
// or a local hero entity. Value is the placement (1-based; 0 is invalid/ignored by the visitor).
type PlacementChangedAction struct {
	ActionBase
	Value        int
	ControllerID int
	EntityName   string
}

func (a *PlacementChangedAction) actionMarker() {}
func (a *PlacementChangedAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnPlacementChanged(a)
}

// SpellcraftChangedAction fires when numeric tag 3809 changes on a local player entity.
// Value is the new Spellcraft stack count.
type SpellcraftChangedAction struct {
	ActionBase
	Value        int
	ControllerID int
	EntityName   string
}

func (a *SpellcraftChangedAction) actionMarker() {}
func (a *SpellcraftChangedAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnSpellcraftChanged(a)
}

// PlayerBGTurnChangedAction fires when the player TURN tag changes on the local player entity.
// Value is the actual BG turn number (1-indexed), not the GameEntity TURN (which is doubled).
type PlayerBGTurnChangedAction struct {
	ActionBase
	Value        int
	ControllerID int
	EntityName   string
}

func (a *PlayerBGTurnChangedAction) actionMarker() {}
func (a *PlayerBGTurnChangedAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnPlayerBGTurnChanged(a)
}

// AnomalyRegisteredAction fires when a FULL_ENTITY/SHOW_ENTITY with
// CARDTYPE=BATTLEGROUND_ANOMALY is processed. The visitor reads CardID from entityProps.
type AnomalyRegisteredAction struct {
	ActionBase
}

func (a *AnomalyRegisteredAction) actionMarker() {}
func (a *AnomalyRegisteredAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnAnomalyRegistered(a)
}

// EnchantmentRegisteredAction fires when a FULL_ENTITY/SHOW_ENTITY with
// CARDTYPE=ENCHANTMENT is processed. ControllerID is the resolved controller.
type EnchantmentRegisteredAction struct {
	ActionBase
	ControllerID int
}

func (a *EnchantmentRegisteredAction) actionMarker() {}
func (a *EnchantmentRegisteredAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnEnchantmentRegistered(a)
}

// HeroRegisteredAction fires when a FULL_ENTITY/SHOW_ENTITY with CARDTYPE=HERO
// is processed. The visitor reads all stats from entityProps.
type HeroRegisteredAction struct {
	ActionBase
	ControllerID int
	PlayerID     int               // PLAYER_ID tag value (partner identification in duos)
	Tags         map[string]string // full raw tag set for stat extraction
}

func (a *HeroRegisteredAction) actionMarker() {}
func (a *HeroRegisteredAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnHeroRegistered(a)
}

// MinionRegisteredAction fires when a FULL_ENTITY/SHOW_ENTITY with CARDTYPE=MINION
// is processed. The visitor reads stats from entityProps and handles board/combat-copy logic.
type MinionRegisteredAction struct {
	ActionBase
	ControllerID int
	Zone         string
	ZonePosition int
}

func (a *MinionRegisteredAction) actionMarker() {}
func (a *MinionRegisteredAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnMinionRegistered(a)
}

// HeroStatChangedAction fires when HEALTH, ATK, ARMOR, DAMAGE, or SPELL_POWER changes
// on a hero entity. It is a TransitionAction because hero stat changes occur in both
// recruit (reconnect replay) and combat (hero damage) phases.
// The visitor reads entityProps for the ARMOR retroactive cache.
type HeroStatChangedAction struct {
	ActionBase
	Tag          string
	NewValue     int
	ControllerID int
}

func (a *HeroStatChangedAction) actionMarker() {}
func (a *HeroStatChangedAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnHeroStatChanged(a)
}

// DuosTeammateAction fires when BACON_DUO_TEAMMATE_PLAYER_ID changes on a player entity.
// PartnerPlayerID is the partner's player ID. This is a fallback/reconnect path — the
// authoritative path is BACON_DUO_TEAMMATE_PLAYER_ID in the CREATE_GAME Player block
// (already handled by OnPlayerDef).
type DuosTeammateAction struct {
	ActionBase
	PartnerPlayerID int
	ControllerID    int
	EntityName      string
}

func (a *DuosTeammateAction) actionMarker() {}
func (a *DuosTeammateAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnDuosTeammate(a)
}

// HeroEntityAssignedAction fires when HERO_ENTITY changes on the local or partner player entity.
// HeroEntityID is the entity ID of the hero card. The visitor applies retroactive ARMOR/stats
// from entityProps when the hero is identified.
type HeroEntityAssignedAction struct {
	ActionBase
	HeroEntityID int
	ControllerID int
	EntityName   string
}

func (a *HeroEntityAssignedAction) actionMarker() {}
func (a *HeroEntityAssignedAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnHeroEntityAssigned(a)
}

// DuosPassableChangedAction fires when BACON_DUO_PASSABLE changes via TAG_CHANGE.
// Value is the raw tag value as int (1 = passable, 0 = not passable).
// Combined with PunishLeaversActive, Value=1 triggers backup duos detection.
type DuosPassableChangedAction struct {
	ActionBase
	Value int
}

func (a *DuosPassableChangedAction) actionMarker() {}
func (a *DuosPassableChangedAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnDuosPassableChanged(a)
}

// DuosPunishLeaversChangedAction fires when BACON_DUOS_PUNISH_LEAVERS changes via TAG_CHANGE.
// Value is the raw tag value as int (1 = active, 0 = inactive).
// Note: the value=1 case during EventGameEntityTags is handled separately in OnReconnect.
type DuosPunishLeaversChangedAction struct {
	ActionBase
	Value int
}

func (a *DuosPunishLeaversChangedAction) actionMarker() {}
func (a *DuosPunishLeaversChangedAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnDuosPunishLeaversChanged(a)
}

// EntityControllerChangedAction fires when a TAG_CHANGE sets CONTROLLER on an entity.
// The visitor updates the entityController registry (entityID → controllerPlayerID).
// This is a pure registry update with no machine mutations.
type EntityControllerChangedAction struct {
	ActionBase
	ControllerID int
}

func (a *EntityControllerChangedAction) actionMarker() {}
func (a *EntityControllerChangedAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnEntityControllerChanged(a)
}

// CombatPlayerChangedAction fires when BACON_CURRENT_COMBAT_PLAYER_ID changes on the
// local player entity. CombatPlayerID is the player currently fighting (0 = no active combat).
// Must be a TransitionAction: the value=0 (end-of-combat) case fires at phase boundaries.
type CombatPlayerChangedAction struct {
	ActionBase
	CombatPlayerID int
	ControllerID   int
	EntityName     string
}

func (a *CombatPlayerChangedAction) actionMarker() {}
func (a *CombatPlayerChangedAction) AcceptTransition(v TransitionVisitor) error {
	return v.OnCombatPlayerChanged(a)
}
