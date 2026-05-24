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
	// OnCombatOutcome records the win/loss result for the local player from a
	// hero-vs-hero attack (PROPOSED_ATTACKER + PROPOSED_DEFENDER resolution).
	OnCombatOutcome(*CombatOutcomeAction) error
	// OnCombatAttacker fires when GameEntity PROPOSED_ATTACKER changes during combat.
	// Marks partner board setup as done and buffers the hero attacker ID.
	OnCombatAttacker(*CombatAttackerAction) error
	// OnCombatDefender fires when GameEntity PROPOSED_DEFENDER changes during combat.
	// Resolves the win/loss outcome using pendingHeroAttackerID set by OnCombatAttacker.
	OnCombatDefender(*CombatDefenderAction) error
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
	OnPlayerTriplesChanged(*PlayerTriplesChangedAction) error
	OnGoldChanged(*GoldChangedAction) error
	OnTavernTierChanged(*TavernTierChangedAction) error
	OnPlacementChanged(*PlacementChangedAction) error
	OnSpellcraftChanged(*SpellcraftChangedAction) error
	OnPlayerBGTurnChanged(*PlayerBGTurnChangedAction) error
	OnAnomalyRegistered(*AnomalyRegisteredAction) error
	OnEnchantmentRegistered(*EnchantmentRegisteredAction) error
	OnHeroRegistered(*HeroRegisteredAction) error
	OnMinionRegistered(*MinionRegisteredAction) error
	OnHeroStatChanged(*HeroStatChangedAction) error
	OnDuosTeammate(*DuosTeammateAction) error
	OnHeroEntityAssigned(*HeroEntityAssignedAction) error
	OnDuosPassableChanged(*DuosPassableChangedAction) error
	OnDuosPunishLeaversChanged(*DuosPunishLeaversChangedAction) error
	OnEntityControllerChanged(*EntityControllerChangedAction) error
	OnCombatPlayerChanged(*CombatPlayerChangedAction) error
}
