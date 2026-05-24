package action

// ── Recruit-phase actions ─────────────────────────────────────────────────────
// These actions can ONLY arrive during PhaseRecruit. Their effects on game state
// are permanent (persist into and beyond the following combat phase).

// MinionBoughtAction fires when a minion's ZONE transitions to PLAY for the local player.
// ControllerID is the entity's controller player ID.
type MinionBoughtAction struct {
	ActionBase
	ControllerID int
}

func (a *MinionBoughtAction) actionMarker()                        {}
func (a *MinionBoughtAction) recruitPhase()                        {}
func (a *MinionBoughtAction) AcceptRecruit(v RecruitVisitor) error { return v.OnMinionBought(a) }

// MinionSoldAction fires when a player sells a minion from the board.
type MinionSoldAction struct{ ActionBase }

func (a *MinionSoldAction) actionMarker()                        {}
func (a *MinionSoldAction) recruitPhase()                        {}
func (a *MinionSoldAction) AcceptRecruit(v RecruitVisitor) error { return v.OnMinionSold(a) }

// TavernUpgradedAction fires when the player purchases a tavern tier upgrade.
type TavernUpgradedAction struct {
	ActionBase
	NewTier int
}

func (a *TavernUpgradedAction) actionMarker()                        {}
func (a *TavernUpgradedAction) recruitPhase()                        {}
func (a *TavernUpgradedAction) AcceptRecruit(v RecruitVisitor) error { return v.OnTavernUpgraded(a) }

// TavernSpellPlayedAction fires when a player plays a tavern spell from hand
// during the recruit phase (direct play only; rally-triggered combat spells use
// CombatTavernSpellAction instead).
type TavernSpellPlayedAction struct{ ActionBase }

func (a *TavernSpellPlayedAction) actionMarker() {}
func (a *TavernSpellPlayedAction) recruitPhase() {}
func (a *TavernSpellPlayedAction) AcceptRecruit(v RecruitVisitor) error {
	return v.OnTavernSpellPlayed(a)
}

// MinionPermStatChangedAction fires when a minion's ATK or Health changes
// during recruit phase (permanent — persists beyond combat).
// Stat is "ATK" or "HEALTH"; NewValue is the updated absolute value.
// EntityName is used to update the entity registry name if it was previously unknown.
type MinionPermStatChangedAction struct {
	ActionBase
	Stat         string
	NewValue     int
	EntityName   string
	ControllerID int
}

func (a *MinionPermStatChangedAction) actionMarker() {}
func (a *MinionPermStatChangedAction) recruitPhase() {}
func (a *MinionPermStatChangedAction) AcceptRecruit(v RecruitVisitor) error {
	return v.OnMinionPermStatChanged(a)
}

// DntEnchantmentAction fires when a Dnt (Deathnote) enchantment's
// TAG_SCRIPT_DATA_NUM_1 or TAG_SCRIPT_DATA_NUM_2 changes — these are the
// HDT buff source counters. Entity carries the enchantment's entity ID;
// the visitor dispatches to the correct handler via the entity's card ID.
type DntEnchantmentAction struct {
	ActionBase
	Tag   string // TAG_SCRIPT_DATA_NUM_1 or TAG_SCRIPT_DATA_NUM_2
	Value int    // raw script data value
}

func (a *DntEnchantmentAction) actionMarker()                        {}
func (a *DntEnchantmentAction) recruitPhase()                        {}
func (a *DntEnchantmentAction) AcceptRecruit(v RecruitVisitor) error { return v.OnDntEnchantment(a) }

// PlayerTagChangedAction fires when a player-level tag changes (Bloodgem values,
// Elemental buff values, TavernSpell buff values, etc.).
// ControllerID is the player= field (0 for bare-name entity references);
// EntityName is the entity's display name (used as a fallback when ControllerID is 0).
type PlayerTagChangedAction struct {
	ActionBase
	Tag          string
	Value        int
	ControllerID int
	EntityName   string
}

func (a *PlayerTagChangedAction) actionMarker() {}
func (a *PlayerTagChangedAction) recruitPhase() {}
func (a *PlayerTagChangedAction) AcceptRecruit(v RecruitVisitor) error {
	return v.OnPlayerTagChanged(a)
}

// EconomyChangedAction fires when an economy tag changes during recruit phase.
// Tag identifies which counter changed (BACON_FREE_REFRESH_COUNT or
// BACON_PLAYER_EXTRA_GOLD_NEXT_TURN); Value is the new absolute value.
// EntityName is used as a fallback when ControllerID is 0 (bare-name entity references).
type EconomyChangedAction struct {
	ActionBase
	Tag          string
	Value        int
	ControllerID int
	EntityName   string
}

func (a *EconomyChangedAction) actionMarker()                        {}
func (a *EconomyChangedAction) recruitPhase()                        {}
func (a *EconomyChangedAction) AcceptRecruit(v RecruitVisitor) error { return v.OnEconomyChanged(a) }
