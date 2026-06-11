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
	// touch refreshes the staleness clock on every dispatched action (including
	// action.Drop). Together with Processor.Handle calling Touch() itself, this
	// guarantees the clock advances for EVERY routed event regardless of which
	// path (dispatcher, deliberate drop, or legacy Handle) it takes — so
	// CheckStaleness never force-ends a live game just because its events were
	// migrated out of Handle. May be nil (tests that don't care about staleness).
	touch func()
}

// NewActionDispatcher constructs a dispatcher wired to the given visitors.
// The initial phase is PhaseIdle; it advances as TurnTransitionActions arrive.
// touch is invoked on every Dispatch call to refresh the staleness clock —
// pass the owning Processor's Touch method (nil is allowed for tests).
func NewActionDispatcher(r action.RecruitVisitor, c action.CombatVisitor, t action.TransitionVisitor, touch func()) *ActionDispatcher {
	return &ActionDispatcher{recruit: r, combat: c, transition: t, phase: action.PhaseIdle, touch: touch}
}

// Dispatch routes action a to the appropriate visitor.
func (d *ActionDispatcher) Dispatch(a action.Action) error {
	// Every dispatched action represents a live log event — refresh the
	// staleness clock even when the action is dropped below.
	if d.touch != nil {
		d.touch()
	}

	// action.Drop marks an event the builder deliberately consumed and
	// discarded (phase/value gate). It is not an error and must not reach a
	// visitor — return before any phase routing or logging.
	if a == action.Drop {
		return nil
	}

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
		// Log at Debug so timing issues between game-start and first turn are visible.
		slog.Debug("action: dropping non-transition action in idle/gameover phase — possible classifier timing issue",
			"phase", d.phase, "type", fmt.Sprintf("%T", a))
		return nil
	}
}
