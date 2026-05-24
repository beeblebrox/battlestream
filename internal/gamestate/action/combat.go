package action

// ── Combat-phase actions ──────────────────────────────────────────────────────
// These actions can ONLY arrive during PhaseCombat. Stat changes are temporary
// (simulation only; the board snapshot preserves recruit-phase stats).

// MinionTempStatChangedAction fires when a minion's ATK or Health changes during
// combat (simulation only; the board snapshot preserves recruit-phase stats).
// Stat is "ATK" or "HEALTH"; NewValue is the updated absolute value.
type MinionTempStatChangedAction struct {
	ActionBase
	Stat         string
	NewValue     int
	ControllerID int
}

func (a *MinionTempStatChangedAction) actionMarker()                                 {}
func (a *MinionTempStatChangedAction) combatPhase()                                  {}
func (a *MinionTempStatChangedAction) AcceptCombat(v CombatVisitor) error { return v.OnMinionTempStatChanged(a) }

// HeroDamagedAction fires when a hero takes damage during combat, indicating
// the combat outcome (win/loss). IsWin is true if the local player's hero dealt
// the attack (i.e., the opponent's hero was damaged).
type HeroDamagedAction struct {
	ActionBase
	Damage int
	IsWin  bool
}

func (a *HeroDamagedAction) actionMarker()                             {}
func (a *HeroDamagedAction) combatPhase()                              {}
func (a *HeroDamagedAction) AcceptCombat(v CombatVisitor) error { return v.OnHeroDamaged(a) }

// MinionAttackedAction fires when PROPOSED_ATTACKER and PROPOSED_DEFENDER are
// both resolved to hero entities (determining win/loss outcome).
type MinionAttackedAction struct {
	ActionBase
	DefenderEntity EntityID
}

func (a *MinionAttackedAction) actionMarker()                               {}
func (a *MinionAttackedAction) combatPhase()                                {}
func (a *MinionAttackedAction) AcceptCombat(v CombatVisitor) error { return v.OnMinionAttacked(a) }

// DeathrattleTriggeredAction fires when a deathrattle resolves during combat.
// Card carries the deathrattle source card info.
type DeathrattleTriggeredAction struct{ ActionBase }

func (a *DeathrattleTriggeredAction) actionMarker()                                   {}
func (a *DeathrattleTriggeredAction) combatPhase()                                    {}
func (a *DeathrattleTriggeredAction) AcceptCombat(v CombatVisitor) error { return v.OnDeathrattleTriggered(a) }

// CombatTavernSpellAction fires when a tavern spell is triggered during combat
// (e.g., via a rally effect). This is the explicit home for cross-phase
// combinations: Card = the spell; BlockCard = the rally/trigger source.
type CombatTavernSpellAction struct{ ActionBase }

func (a *CombatTavernSpellAction) actionMarker()                               {}
func (a *CombatTavernSpellAction) combatPhase()                                {}
func (a *CombatTavernSpellAction) AcceptCombat(v CombatVisitor) error { return v.OnCombatTavernSpell(a) }

// CombatEconomyEffectAction fires when an economy tag (e.g.,
// BACON_PLAYER_EXTRA_GOLD_NEXT_TURN) changes during combat. The effect applies
// to the NEXT recruit phase but originates from a combat trigger.
// Tag identifies the tag; Value is the new absolute value.
type CombatEconomyEffectAction struct {
	ActionBase
	Tag          string
	Value        int
	ControllerID int
}

func (a *CombatEconomyEffectAction) actionMarker()                                  {}
func (a *CombatEconomyEffectAction) combatPhase()                                   {}
func (a *CombatEconomyEffectAction) AcceptCombat(v CombatVisitor) error { return v.OnCombatEconomyEffect(a) }
