package gamestate_test

// Integration tests against the 2026-03-07 game log (Power.log from a full BG game).
//
// All expected values are derived from the raw log (ground truth), not from parser
// output. The log is at internal/gamestate/testdata/power_log_2026_03_07.txt.
//
// Game summary (from raw log analysis):
//   Hero:        Spectre Teron (BG25_HERO_103_SKIN_D, entity 88, player=5=Moch#1358)
//   Total turns: 16
//   Result:      1st place (PLAYSTATE=WON at 14:16:10)
//   Armor:       started at 14, ended at 3 (no health damage taken)
//   W/L by turn: L L W W W W W W W W W W L W W W  (PREDAMAGE events on entity 88)
//   Tier prog:   T2@turn1, T3@turn5, T4@turn6, T5@turn7, T6@turn8
//   Naga spells: 72 total (tag=3809 final value for Moch#1358, but NO synergy minion on board → counter absent)
//
// Q7 (win/loss sequence) awaiting video confirmation from user.

import (
	"testing"

	"battlestream.fixates.io/internal/gamestate"
)

const logFile2026_03_07 = "testdata/power_log_2026_03_07.txt"

// TestGameLog2026_03_07_PlayerIdentification verifies the local player is Moch#1358.
func TestGameLog2026_03_07_PlayerIdentification(t *testing.T) {
	s := sharedLog2026State(t)
	if s.Player.Name != "Moch#1358" {
		t.Errorf("Player.Name: expected Moch#1358, got %q", s.Player.Name)
	}
}

// TestGameLog2026_03_07_HeroCardID verifies the chosen hero is Spectre Teron.
func TestGameLog2026_03_07_HeroCardID(t *testing.T) {
	s := sharedLog2026State(t)
	const want = "BG25_HERO_103_SKIN_D"
	if s.Player.HeroCardID != want {
		t.Errorf("HeroCardID: expected %q, got %q", want, s.Player.HeroCardID)
	}
}

// TestGameLog2026_03_07_Placement verifies Moch won the game (1st place).
func TestGameLog2026_03_07_Placement(t *testing.T) {
	s := sharedLog2026State(t)
	if s.Placement != 1 {
		t.Errorf("Placement: expected 1 (won), got %d", s.Placement)
	}
}

// TestGameLog2026_03_07_Phase verifies the game ended in GAME_OVER phase.
func TestGameLog2026_03_07_Phase(t *testing.T) {
	s := sharedLog2026State(t)
	if s.Phase != gamestate.PhaseGameOver {
		t.Errorf("Phase: expected PhaseGameOver, got %s", s.Phase)
	}
}

// TestGameLog2026_03_07_FinalArmor verifies armor ended at 3 (health stayed 30).
// Raw log: last tag=ARMOR on entity 88 = 3 at 14:07:50; no further changes before game end.
func TestGameLog2026_03_07_FinalArmor(t *testing.T) {
	s := sharedLog2026State(t)
	if s.Player.Armor != 3 {
		t.Errorf("Armor at game end: expected 3, got %d", s.Player.Armor)
	}
	if s.Player.Health != 30 {
		t.Errorf("Health at game end: expected 30 (no health damage taken), got %d", s.Player.Health)
	}
}

// TestGameLog2026_03_07_NagaSpellsFinal verifies that CatNagaSpells is absent in the
// final state for this game (Spectre Teron, no Naga synergy minions on board).
//
// Raw log: tag=3809 for Moch#1358 reached 72 at line 563604 (14:15:31), but since
// no Thaumaturgist / Arcane Cannoneer / Showy Cyclist / Groundbreaker was ever on
// the board, HasNagaSynergyMinion returns false and RemoveAbilityCounter is called
// on every tag=3809 event. The counter must be absent from AbilityCounters at game end.
func TestGameLog2026_03_07_NagaSpellsFinal(t *testing.T) {
	s := sharedLog2026State(t)

	for i := range s.AbilityCounters {
		if s.AbilityCounters[i].Category == gamestate.CatNagaSpells {
			t.Errorf("AbilityCounters: unexpected CatNagaSpells entry (value=%d, display=%q); "+
				"expected it to be absent because no Naga synergy minion was on the board",
				s.AbilityCounters[i].Value, s.AbilityCounters[i].Display)
		}
	}
}

// TestGameLog2026_03_07_TavernTierFinal verifies the player reached tier 6.
// Raw log: PLAYER_TECH_LEVEL value=6 at 13:53:48 (during BG turn 8).
func TestGameLog2026_03_07_TavernTierFinal(t *testing.T) {
	s := sharedLog2026State(t)
	if s.TavernTier != 6 {
		t.Errorf("TavernTier at game end: expected 6, got %d", s.TavernTier)
	}
}

// TestGameLog2026_03_07_TotalTurns verifies the game ran for 16 BG turns.
// Raw log: last player TURN change = 16 at 14:13:49.
func TestGameLog2026_03_07_TotalTurns(t *testing.T) {
	s := sharedLog2026State(t)
	if s.Turn != 16 {
		t.Errorf("Turn at game end: expected 16, got %d", s.Turn)
	}
}

// TestGameLog2026_03_07_AvailableTribes verifies correct tribe detection.
// This game has 5 available tribes: Pirate, Murloc, Beast, Quilboar, Demon.
// Banned tribes (Dragon, Elemental, Naga, Undead) only appear on multi-tribe
// minions that share a subset with an available tribe — these must be excluded.
func TestGameLog2026_03_07_AvailableTribes(t *testing.T) {
	s := sharedLog2026State(t)
	want := map[string]bool{"Pirate": true, "Murloc": true, "Beast": true, "Quilboar": true, "Demon": true}
	got := make(map[string]bool)
	for _, tribe := range s.AvailableTribes {
		got[tribe] = true
	}
	for tribe := range want {
		if !got[tribe] {
			t.Errorf("missing available tribe %q; got %v", tribe, s.AvailableTribes)
		}
	}
	for tribe := range got {
		if !want[tribe] {
			t.Errorf("unexpected tribe %q detected as available; got %v", tribe, s.AvailableTribes)
		}
	}
}

// TestGameLog2026_03_07_WinLossStreak verifies the win/loss streak at game end.
//
// Derived from PROPOSED_ATTACKER hero attacks (HDT LastAttackingHero pattern):
//   Rounds 1-2:  LOSS  (opponent hero is PROPOSED_ATTACKER → attacks local hero)
//   Rounds 3-15: WIN or TIE (local hero is PROPOSED_ATTACKER, or no hero attacks)
//   Round 13:    WIN  (armor drops from Imposing Percussionist self-damage, NOT combat loss)
//   Round 16:    WIN  (recorded at EventGameEnd from localCombatResult before GameEnd)
//
// At game end: WinStreak=12, LossStreak=0.
// Ties (no hero attack) preserve the current streak without incrementing.
func TestGameLog2026_03_07_WinLossStreak(t *testing.T) {
	s := sharedLog2026State(t)
	if s.Player.WinStreak != 12 {
		t.Errorf("WinStreak at game end: expected 12, got %d", s.Player.WinStreak)
	}
	if s.Player.LossStreak != 0 {
		t.Errorf("LossStreak at game end: expected 0 (last result was a win), got %d", s.Player.LossStreak)
	}
}
