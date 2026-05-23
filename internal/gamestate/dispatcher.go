package gamestate

import (
	"fmt"
	"log/slog"

	"battlestream.fixates.io/internal/gamestate/action"
)

// ActionDispatcher routes typed actions to the appropriate phase-specific visitor.
// Phase confusion is structurally impossible: recruit actions are only dispatched
// to the RecruitVisitor during PhaseRecruit, and combat actions only to the
// CombatVisitor during PhaseCombat. Wrong-phase actions are logged as builder
// classification bugs and skipped without mutating state.
type ActionDispatcher struct {
	recruit    action.RecruitVisitor
	combat     action.CombatVisitor
	transition action.TransitionVisitor
	phase      action.GamePhase
}

// NewActionDispatcher constructs a dispatcher wired to the given visitors.
// The initial phase is PhaseIdle; it advances as TurnTransitionActions arrive.
func NewActionDispatcher(r action.RecruitVisitor, c action.CombatVisitor, t action.TransitionVisitor) *ActionDispatcher {
	return &ActionDispatcher{recruit: r, combat: c, transition: t, phase: action.PhaseIdle}
}

// Dispatch routes action a to the appropriate visitor.
func (d *ActionDispatcher) Dispatch(a action.Action) error {
	// Lifecycle actions are phase-agnostic — dispatch first.
	if ta, ok := a.(action.TransitionAction); ok {
		if err := ta.AcceptTransition(d.transition); err != nil {
			return err
		}
		// Advance dispatcher phase after a turn transition.
		if tta, ok := a.(*action.TurnTransitionAction); ok {
			d.phase = tta.NewPhase
		}
		if ga, ok := a.(*action.GameStartAction); ok && !ga.IsReconnect {
			d.phase = action.PhaseIdle
		}
		if _, ok := a.(*action.GameEndAction); ok {
			d.phase = action.PhaseGameOver
		}
		return nil
	}

	switch d.phase {
	case action.PhaseRecruit:
		ra, ok := a.(action.RecruitPhaseAction)
		if !ok {
			slog.Warn("action: non-recruit action during recruit phase — builder bug",
				"type", fmt.Sprintf("%T", a))
			return nil
		}
		return ra.AcceptRecruit(d.recruit)

	case action.PhaseCombat:
		ca, ok := a.(action.CombatPhaseAction)
		if !ok {
			slog.Warn("action: non-combat action during combat phase — builder bug",
				"type", fmt.Sprintf("%T", a))
			return nil
		}
		return ca.AcceptCombat(d.combat)

	default:
		// PhaseIdle, PhaseGameOver, PhaseUnknown: drop non-transition actions.
		return nil
	}
}
