package action

// RecruitVisitor handles actions that can only occur during PhaseRecruit.
// All effects on game state are permanent (persist beyond combat).
type RecruitVisitor interface {
	OnMinionBought(*MinionBoughtAction) error
	OnMinionSold(*MinionSoldAction) error
	OnTavernUpgraded(*TavernUpgradedAction) error
	OnTavernSpellPlayed(*TavernSpellPlayedAction) error
	OnMinionPermStatChanged(*MinionPermStatChangedAction) error
	OnDntEnchantment(*DntEnchantmentAction) error
	OnPlayerTagChanged(*PlayerTagChangedAction) error
	OnEconomyChanged(*EconomyChangedAction) error
}

// CombatVisitor handles actions that can only occur during PhaseCombat.
// Stat changes are temporary (combat simulation; recruit board snapshot is preserved).
type CombatVisitor interface {
	OnMinionTempStatChanged(*MinionTempStatChangedAction) error
	OnHeroDamaged(*HeroDamagedAction) error
	OnMinionAttacked(*MinionAttackedAction) error
	OnDeathrattleTriggered(*DeathrattleTriggeredAction) error
	// OnCombatTavernSpell handles tavern spells triggered during combat (e.g.,
	// via rally). Action.Card is the spell; Action.BlockCard is the trigger source.
	OnCombatTavernSpell(*CombatTavernSpellAction) error
	OnCombatEconomyEffect(*CombatEconomyEffectAction) error
}

// TransitionVisitor handles game lifecycle events dispatched regardless of phase.
// OnTurnTransition is responsible for explicitly flushing any pendingStatChanges
// before the phase switch.
type TransitionVisitor interface {
	OnGameStart(*GameStartAction) error
	OnTurnTransition(*TurnTransitionAction) error
	OnGameEnd(*GameEndAction) error
	OnReconnect(*ReconnectAction) error
	OnPlayerDef(*PlayerDefAction) error
	OnPlayerName(*PlayerNameAction) error
}
