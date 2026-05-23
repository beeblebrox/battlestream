package action

// ── Recruit-phase actions ─────────────────────────────────────────────────────
// These actions can ONLY arrive during PhaseRecruit. Their effects on game state
// are permanent (persist into and beyond the following combat phase).

// MinionBoughtAction fires when a minion moves from HAND/TAVERN to the player board.
type MinionBoughtAction struct {
	ActionBase
	BaseATK    int
	BaseHealth int
}

func (a *MinionBoughtAction) actionMarker()                           {}
func (a *MinionBoughtAction) recruitPhase()                           {}
func (a *MinionBoughtAction) AcceptRecruit(v RecruitVisitor) error { return v.OnMinionBought(a) }

// MinionSoldAction fires when a player sells a minion from the board.
type MinionSoldAction struct{ ActionBase }

func (a *MinionSoldAction) actionMarker()                          {}
func (a *MinionSoldAction) recruitPhase()                          {}
func (a *MinionSoldAction) AcceptRecruit(v RecruitVisitor) error { return v.OnMinionSold(a) }

// TavernUpgradedAction fires when the player purchases a tavern tier upgrade.
type TavernUpgradedAction struct {
	ActionBase
	NewTier int
}

func (a *TavernUpgradedAction) actionMarker()                             {}
func (a *TavernUpgradedAction) recruitPhase()                             {}
func (a *TavernUpgradedAction) AcceptRecruit(v RecruitVisitor) error { return v.OnTavernUpgraded(a) }

// TavernSpellPlayedAction fires when a player plays a tavern spell from hand
// during the recruit phase (direct play only; rally-triggered combat spells use
// CombatTavernSpellAction instead).
type TavernSpellPlayedAction struct{ ActionBase }

func (a *TavernSpellPlayedAction) actionMarker()                               {}
func (a *TavernSpellPlayedAction) recruitPhase()                               {}
func (a *TavernSpellPlayedAction) AcceptRecruit(v RecruitVisitor) error { return v.OnTavernSpellPlayed(a) }

// MinionPermStatChangedAction fires when a minion's ATK or Health changes
// permanently during recruit phase (from a buff, enchantment, or effect).
type MinionPermStatChangedAction struct {
	ActionBase
	DeltaATK int
	DeltaHP  int
}

func (a *MinionPermStatChangedAction) actionMarker()                                  {}
func (a *MinionPermStatChangedAction) recruitPhase()                                  {}
func (a *MinionPermStatChangedAction) AcceptRecruit(v RecruitVisitor) error { return v.OnMinionPermStatChanged(a) }

// DntEnchantmentAction fires when a player-level Dnt (Deathnote) enchantment's
// script data updates — these are the HDT buff source counters.
type DntEnchantmentAction struct {
	ActionBase
	Category   BuffCategory
	DeltaATK   int
	DeltaHP    int
	IsAbsolute bool // true = absolute tracking; false = differential
}

func (a *DntEnchantmentAction) actionMarker()                              {}
func (a *DntEnchantmentAction) recruitPhase()                              {}
func (a *DntEnchantmentAction) AcceptRecruit(v RecruitVisitor) error { return v.OnDntEnchantment(a) }

// PlayerTagChangedAction fires when a player-level tag changes (Bloodgem values,
// Elemental buff values, TavernSpell buff values, etc.).
type PlayerTagChangedAction struct {
	ActionBase
	Tag   string
	Value int
}

func (a *PlayerTagChangedAction) actionMarker()                                {}
func (a *PlayerTagChangedAction) recruitPhase()                                {}
func (a *PlayerTagChangedAction) AcceptRecruit(v RecruitVisitor) error { return v.OnPlayerTagChanged(a) }

// EconomyChangedAction fires when free-refresh count or next-turn gold bonus
// changes during recruit phase.
type EconomyChangedAction struct {
	ActionBase
	FreeRefreshDelta  int
	GoldNextTurnDelta int
}

func (a *EconomyChangedAction) actionMarker()                               {}
func (a *EconomyChangedAction) recruitPhase()                               {}
func (a *EconomyChangedAction) AcceptRecruit(v RecruitVisitor) error { return v.OnEconomyChanged(a) }
