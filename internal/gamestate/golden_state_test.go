package gamestate_test

// golden_state_test.go is the regression harness for the visitor architecture
// migration. It runs the same real game log through both the legacy Handle() path
// and the new ActionDispatcher path, then asserts the resulting BGGameState is
// identical field by field.
//
// At the start of migration (Phase 1), only lifecycle events are wired to the
// dispatcher, so most assertions are disabled (marked "TODO: enable in Phase N").
// Enable each assertion as its handler is migrated. A passing assertion here means
// that piece of the migration is complete and correct.
//
// The test file path is shared with game_log_2026_03_07_test.go.

import (
	"bufio"
	"os"
	"testing"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/gamestate/action"
	"battlestream.fixates.io/internal/gamestate/builder"
	"battlestream.fixates.io/internal/gamestate/card"
	"battlestream.fixates.io/internal/parser"
)

// runDispatchPath feeds all events from logPath through the ActionBuilder +
// ActionDispatcher + a GameStateMachine that implements all three visitors.
// Returns the final BGGameState.
//
// During migration, the dispatcher's visitor implementations are stubs that
// delegate to the legacy Processor for events not yet migrated.
func runDispatchPath(t *testing.T, logPath string) gamestate.BGGameState {
	t.Helper()

	f, err := os.Open(logPath)
	if err != nil {
		t.Skipf("test fixture not available: %v", err)
	}
	defer f.Close()

	ch := make(chan parser.GameEvent, 1024)
	p := parser.New(ch)

	// The catalog bridges the existing CardName function.
	catalog := card.NewFuncCatalog(gamestate.CardName)
	bldr := builder.New(catalog)

	// During Phase 1, the dispatch machine is a Machine with a Processor wired
	// as the visitor implementation. Events not yet migrated still go through
	// Processor.Handle(), maintaining behavior parity.
	m := gamestate.New()
	proc := gamestate.NewProcessor(m)
	disp := gamestate.NewActionDispatcher(
		proc.AsRecruitVisitor(),
		proc.AsCombatVisitor(),
		proc.AsTransitionVisitor(),
		proc.Touch,
	)

	drain := func() {
		for {
			select {
			case e := <-ch:
				if a := bldr.Build(e); a != nil {
					// Migrated event: use dispatcher path only.
					// Handle() is now a thin adapter that would call the same visitor
					// method — calling both would double-process the event.
					if err := disp.Dispatch(a); err != nil {
						t.Errorf("dispatch error: %v", err)
					}
				} else {
					// Not yet migrated: fall back to Handle().
					proc.Handle(e)
				}
			default:
				return
			}
		}
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		p.Feed(scanner.Text())
		drain()
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning log: %v", err)
	}
	p.Flush()
	drain()

	return m.State()
}

// TestGoldenState_DispatchProducesSameResultAsHandle is the migration regression
// harness. At Phase 1, the dispatcher path still delegates to Processor.Handle()
// for all events, so the result must be identical to the Handle()-only path.
//
// As events are migrated out of Handle(), add assertions here to verify
// the dispatcher path produces the same value for each field.
func TestGoldenState_DispatchProducesSameResultAsHandle(t *testing.T) {
	const logPath = logFile2026_03_07

	// Reference state: legacy Handle() path.
	ref := sharedLog2026State(t)

	// Dispatch path: ActionBuilder + ActionDispatcher + Processor stubs.
	got := runDispatchPath(t, logPath)

	// ── Phase 1 assertions (lifecycle only) ─────────────────────────────────
	// These should pass from day 1: the dispatcher sees game-start and turn
	// transitions but delegates all state mutation to Processor.Handle().
	if ref.GameID != got.GameID {
		t.Errorf("GameID: ref=%q got=%q", ref.GameID, got.GameID)
	}
	if ref.Turn != got.Turn {
		t.Errorf("Turn: ref=%d got=%d", ref.Turn, got.Turn)
	}
	if ref.Placement != got.Placement {
		t.Errorf("Placement: ref=%d got=%d", ref.Placement, got.Placement)
	}
	if ref.IsDuos != got.IsDuos {
		t.Errorf("IsDuos: ref=%v got=%v", ref.IsDuos, got.IsDuos)
	}
	if ref.Player.Name != got.Player.Name {
		t.Errorf("Player.Name: ref=%q got=%q", ref.Player.Name, got.Player.Name)
	}

	// ── Phase 2 assertions (economy / player tags) ───────────────────────────
	if ref.Player.TavernTier != got.Player.TavernTier {
		t.Errorf("Player.TavernTier: ref=%d got=%d", ref.Player.TavernTier, got.Player.TavernTier)
	}
	if ref.Player.Health != got.Player.Health {
		t.Errorf("Player.Health: ref=%d got=%d", ref.Player.Health, got.Player.Health)
	}
	if ref.Player.CurrentGold != got.Player.CurrentGold {
		t.Errorf("Player.CurrentGold: ref=%d got=%d", ref.Player.CurrentGold, got.Player.CurrentGold)
	}
	if ref.Player.TripleCount != got.Player.TripleCount {
		t.Errorf("Player.TripleCount: ref=%d got=%d", ref.Player.TripleCount, got.Player.TripleCount)
	}

	// ── Phase 4 assertions (Dnt enchantments) ───────────────────────────────
	if !buffSourcesEqual(ref.BuffSources, got.BuffSources) {
		t.Errorf("BuffSources mismatch:\n  ref=%v\n  got=%v", ref.BuffSources, got.BuffSources)
	}
	if !abilityCountersEqual(ref.AbilityCounters, got.AbilityCounters) {
		t.Errorf("AbilityCounters mismatch:\n  ref=%v\n  got=%v", ref.AbilityCounters, got.AbilityCounters)
	}

	// ── Phase 5 assertions (board / stat changes) ───────────────────────────────
	if len(ref.Board) != len(got.Board) {
		t.Errorf("Board length: ref=%d got=%d", len(ref.Board), len(got.Board))
	} else {
		for i, rm := range ref.Board {
			gm := got.Board[i]
			if rm.EntityID != gm.EntityID || rm.Attack != gm.Attack || rm.Health != gm.Health {
				t.Errorf("Board[%d]: ref={%d %s %d/%d} got={%d %s %d/%d}",
					i, rm.EntityID, rm.Name, rm.Attack, rm.Health,
					gm.EntityID, gm.Name, gm.Attack, gm.Health)
			}
		}
	}

	// ── Phase 6 assertions (combat / win-loss) ───────────────────────────────
	// PROPOSED_ATTACKER/DEFENDER still handled by Handle() in both paths.
	if ref.Player.WinStreak != got.Player.WinStreak {
		t.Errorf("Player.WinStreak: ref=%d got=%d", ref.Player.WinStreak, got.Player.WinStreak)
	}
	if ref.Player.LossStreak != got.Player.LossStreak {
		t.Errorf("Player.LossStreak: ref=%d got=%d", ref.Player.LossStreak, got.Player.LossStreak)
	}

	// ── Additional field assertions (all delegated through Handle()) ──────────
	if ref.Player.HeroCardID != got.Player.HeroCardID {
		t.Errorf("Player.HeroCardID: ref=%q got=%q", ref.Player.HeroCardID, got.Player.HeroCardID)
	}
	if ref.Player.Armor != got.Player.Armor {
		t.Errorf("Player.Armor: ref=%d got=%d", ref.Player.Armor, got.Player.Armor)
	}
	if ref.Player.Damage != got.Player.Damage {
		t.Errorf("Player.Damage: ref=%d got=%d", ref.Player.Damage, got.Player.Damage)
	}
	if ref.AnomalyCardID != got.AnomalyCardID {
		t.Errorf("AnomalyCardID: ref=%q got=%q", ref.AnomalyCardID, got.AnomalyCardID)
	}
	if len(ref.AvailableTribes) != len(got.AvailableTribes) {
		t.Errorf("AvailableTribes length: ref=%d got=%d", len(ref.AvailableTribes), len(got.AvailableTribes))
	}
	if len(ref.Modifications) != len(got.Modifications) {
		t.Errorf("Modifications length: ref=%d got=%d", len(ref.Modifications), len(got.Modifications))
	}
}

// TestGoldenState_PhaseEnforcement verifies that dispatching a combat action
// during recruit phase produces no state change and logs a warning.
// This is the fundamental invariant of the architecture.
func TestGoldenState_PhaseEnforcement(t *testing.T) {
	m := gamestate.New()
	proc := gamestate.NewProcessor(m)
	disp := gamestate.NewActionDispatcher(
		proc.AsRecruitVisitor(),
		proc.AsCombatVisitor(),
		proc.AsTransitionVisitor(),
		proc.Touch,
	)

	// Advance dispatcher to PhaseRecruit via a TurnTransitionAction (odd turn).
	turnAction := &action.TurnTransitionAction{
		NewPhase:       action.PhaseRecruit,
		GameEntityTurn: 1,
	}
	if err := disp.Dispatch(turnAction); err != nil {
		t.Fatalf("dispatching TurnTransitionAction: %v", err)
	}

	// Board must be empty before the test action.
	if len(m.State().Board) != 0 {
		t.Fatalf("expected empty board before test, got %d minions", len(m.State().Board))
	}

	// Dispatch a combat-phase action (MinionTempStatChangedAction) during PhaseRecruit.
	// The dispatcher must reject it: log a "builder bug" warning and skip it.
	wrongPhaseAction := &action.MinionTempStatChangedAction{
		Stat:     "ATK",
		NewValue: 99,
	}
	if err := disp.Dispatch(wrongPhaseAction); err != nil {
		t.Errorf("Dispatch returned error for wrong-phase action (expected nil): %v", err)
	}

	// Board must still be empty — the action must not have mutated state.
	if len(m.State().Board) != 0 {
		t.Errorf("Board was mutated by wrong-phase action: got %d minions", len(m.State().Board))
	}
}

// ── Comparison helpers ────────────────────────────────────────────────────────

func buffSourcesEqual(a, b []gamestate.BuffSource) bool {
	if len(a) != len(b) {
		return false
	}
	// Build map by category for order-independent comparison.
	m := make(map[string]gamestate.BuffSource, len(a))
	for _, bs := range a {
		m[bs.Category] = bs
	}
	for _, bs := range b {
		ref, ok := m[bs.Category]
		if !ok || ref.Attack != bs.Attack || ref.Health != bs.Health {
			return false
		}
	}
	return true
}

func abilityCountersEqual(a, b []gamestate.AbilityCounter) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]gamestate.AbilityCounter, len(a))
	for _, ac := range a {
		m[ac.Category] = ac
	}
	for _, ac := range b {
		ref, ok := m[ac.Category]
		if !ok || ref.Value != ac.Value {
			return false
		}
	}
	return true
}
