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

func (a *MinionTempStatChangedAction) actionMarker() {}
func (a *MinionTempStatChangedAction) combatPhase()  {}
func (a *MinionTempStatChangedAction) AcceptCombat(v CombatVisitor) error {
	return v.OnMinionTempStatChanged(a)
}

// HeroDamagedAction fires when a hero takes damage during combat, indicating
// the combat outcome (win/loss). IsWin is true if the local player's hero dealt
// the attack (i.e., the opponent's hero was damaged).
type HeroDamagedAction struct {
	ActionBase
	Damage int
	IsWin  bool
}

func (a *HeroDamagedAction) actionMarker()                      {}
func (a *HeroDamagedAction) combatPhase()                       {}
func (a *HeroDamagedAction) AcceptCombat(v CombatVisitor) error { return v.OnHeroDamaged(a) }

// MinionAttackedAction fires when PROPOSED_ATTACKER and PROPOSED_DEFENDER are
// both resolved to hero entities (determining win/loss outcome).
type MinionAttackedAction struct {
	ActionBase
	DefenderEntity EntityID
}

func (a *MinionAttackedAction) actionMarker()                      {}
func (a *MinionAttackedAction) combatPhase()                       {}
func (a *MinionAttackedAction) AcceptCombat(v CombatVisitor) error { return v.OnMinionAttacked(a) }

// DeathrattleTriggeredAction fires when a deathrattle resolves during combat.
// Card carries the deathrattle source card info.
type DeathrattleTriggeredAction struct{ ActionBase }

func (a *DeathrattleTriggeredAction) actionMarker() {}
func (a *DeathrattleTriggeredAction) combatPhase()  {}
func (a *DeathrattleTriggeredAction) AcceptCombat(v CombatVisitor) error {
	return v.OnDeathrattleTriggered(a)
}

// CombatTavernSpellAction fires when a tavern spell is triggered during combat
// (e.g., via a rally effect). This is the explicit home for cross-phase
// combinations: Card = the spell; BlockCard = the rally/trigger source.
type CombatTavernSpellAction struct{ ActionBase }

func (a *CombatTavernSpellAction) actionMarker() {}
func (a *CombatTavernSpellAction) combatPhase()  {}
func (a *CombatTavernSpellAction) AcceptCombat(v CombatVisitor) error {
	return v.OnCombatTavernSpell(a)
}

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

func (a *CombatEconomyEffectAction) actionMarker() {}
func (a *CombatEconomyEffectAction) combatPhase()  {}
func (a *CombatEconomyEffectAction) AcceptCombat(v CombatVisitor) error {
	return v.OnCombatEconomyEffect(a)
}

// CombatOutcomeAction fires when a hero-vs-hero attack resolves (PROPOSED_ATTACKER
// + PROPOSED_DEFENDER both identify hero entities). IsLocalWin is true when the
// local hero was the attacker; false when the local hero was the defender (loss).
// If neither hero is the local hero, this action is not emitted.
type CombatOutcomeAction struct {
	ActionBase
	IsLocalWin bool
}

func (a *CombatOutcomeAction) actionMarker() {}
func (a *CombatOutcomeAction) combatPhase()  {}
func (a *CombatOutcomeAction) AcceptCombat(v CombatVisitor) error {
	return v.OnCombatOutcome(a)
}

// CombatAttackerAction fires when GameEntity PROPOSED_ATTACKER changes during combat.
// AttackerID is the entity ID of the attacking entity (0 = reset between attacks).
// IsHeroAttacker is true when the builder's entityTypes cache identifies the attacker as a HERO.
type CombatAttackerAction struct {
	ActionBase
	AttackerID     int
	IsHeroAttacker bool
}

func (a *CombatAttackerAction) actionMarker() {}
func (a *CombatAttackerAction) combatPhase()  {}
func (a *CombatAttackerAction) AcceptCombat(v CombatVisitor) error {
	return v.OnCombatAttacker(a)
}

// CombatDefenderAction fires when GameEntity PROPOSED_DEFENDER changes during combat.
// DefenderID is the entity ID of the defending entity. IsHeroDefender is true when
// the builder's entityTypes cache identifies the defender as a HERO.
// The visitor checks pendingHeroAttackerID to resolve win/loss outcome.
type CombatDefenderAction struct {
	ActionBase
	DefenderID     int
	IsHeroDefender bool
}

func (a *CombatDefenderAction) actionMarker() {}
func (a *CombatDefenderAction) combatPhase()  {}
func (a *CombatDefenderAction) AcceptCombat(v CombatVisitor) error {
	return v.OnCombatDefender(a)
}
