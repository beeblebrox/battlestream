package gamestate

// processor_visitor.go adds visitor adapter methods to Processor.
//
// During incremental migration (Phase 1+), Processor acts as a bridge: the
// ActionDispatcher calls visitor methods here, while legacy Handle() calls still
// process the same events in parallel. As each event type is migrated, its
// Handle() call is removed and the corresponding visitor method gains the real logic.
//
// Phase 1: All visitor methods are no-ops. Handle() does all work.
// Phase N: Each migrated event type moves its logic from Handle() to its visitor method.

import "battlestream.fixates.io/internal/gamestate/action"

// ── Visitor adapter accessors ─────────────────────────────────────────────────

// AsRecruitVisitor returns the Processor as a RecruitVisitor.
func (p *Processor) AsRecruitVisitor() action.RecruitVisitor { return p }

// AsCombatVisitor returns the Processor as a CombatVisitor.
func (p *Processor) AsCombatVisitor() action.CombatVisitor { return p }

// AsTransitionVisitor returns the Processor as a TransitionVisitor.
func (p *Processor) AsTransitionVisitor() action.TransitionVisitor { return p }

// ── RecruitVisitor (all no-ops in Phase 1) ───────────────────────────────────

func (p *Processor) OnMinionBought(_ *action.MinionBoughtAction) error            { return nil }
func (p *Processor) OnMinionSold(_ *action.MinionSoldAction) error                { return nil }
func (p *Processor) OnTavernUpgraded(_ *action.TavernUpgradedAction) error        { return nil }
func (p *Processor) OnTavernSpellPlayed(_ *action.TavernSpellPlayedAction) error  { return nil }
func (p *Processor) OnMinionPermStatChanged(_ *action.MinionPermStatChangedAction) error { return nil }
func (p *Processor) OnDntEnchantment(_ *action.DntEnchantmentAction) error        { return nil }
func (p *Processor) OnPlayerTagChanged(_ *action.PlayerTagChangedAction) error     { return nil }
func (p *Processor) OnEconomyChanged(_ *action.EconomyChangedAction) error        { return nil }

// ── CombatVisitor (all no-ops in Phase 1) ────────────────────────────────────

func (p *Processor) OnMinionTempStatChanged(_ *action.MinionTempStatChangedAction) error { return nil }
func (p *Processor) OnHeroDamaged(_ *action.HeroDamagedAction) error                    { return nil }
func (p *Processor) OnMinionAttacked(_ *action.MinionAttackedAction) error               { return nil }
func (p *Processor) OnDeathrattleTriggered(_ *action.DeathrattleTriggeredAction) error   { return nil }
func (p *Processor) OnCombatTavernSpell(_ *action.CombatTavernSpellAction) error        { return nil }
func (p *Processor) OnCombatEconomyEffect(_ *action.CombatEconomyEffectAction) error    { return nil }

// ── TransitionVisitor (no-ops in Phase 1; will gain logic in Phase 2) ────────

func (p *Processor) OnGameStart(_ *action.GameStartAction) error           { return nil }
func (p *Processor) OnTurnTransition(_ *action.TurnTransitionAction) error { return nil }
func (p *Processor) OnGameEnd(_ *action.GameEndAction) error               { return nil }
func (p *Processor) OnReconnect(_ *action.ReconnectAction) error           { return nil }
func (p *Processor) OnPlayerDef(_ *action.PlayerDefAction) error           { return nil }
func (p *Processor) OnPlayerName(_ *action.PlayerNameAction) error         { return nil }
