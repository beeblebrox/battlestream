package gamestate_test

// golden_state_duos_test.go is the Duos regression harness for the visitor
// architecture migration. It runs the 2026-05-08 Duos game log through both
// the legacy Handle() path and the new ActionDispatcher path, then asserts
// that the resulting BGGameState matches field by field.
//
// This test specifically exercises the Duos-only visitor methods that the
// solo 2026-03-07 log cannot reach:
//
//   OnDuosTeammate          — BACON_DUO_TEAMMATE_PLAYER_ID path
//   OnDuosPassableChanged   — BACON_DUO_PASSABLE backup detection path
//   OnDuosPunishLeaversChanged — BACON_DUOS_PUNISH_LEAVERS detection/clear path
//   OnEntityControllerChanged  — CONTROLLER tag tracking (critical for Duos Dnt)
//   OnCombatPlayerChanged   — BACON_CURRENT_COMBAT_PLAYER_ID (partner board capture)
//   OnHeroRegistered (partner path) — partner hero identification via PLAYER_ID tag
//   OnHeroStatChanged (partner path) — partner HEALTH/ARMOR/DAMAGE tracking
//   OnPlayerTriplesChanged (partner path) — partner triples
//   OnTavernTierChanged (partner path) — partner tavern tier
//
// The test uses the shared sharedLog20260508State() helper (Handle()-only path)
// as the reference, and runDispatchPath() (defined in golden_state_test.go) for
// the dispatcher path.

import (
	"testing"
)

// TestGoldenStateDuos_DispatchProducesSameResultAsHandle is the Duos migration
// regression harness. It verifies that the ActionDispatcher path produces the
// same BGGameState as the legacy Handle()-only path when processing the
// 2026-05-08 Duos game log.
func TestGoldenStateDuos_DispatchProducesSameResultAsHandle(t *testing.T) {
	// Reference state: legacy Handle()-only path (parsed once via sync.Once).
	ref := sharedLog20260508State(t)

	// Dispatcher path: ActionBuilder + ActionDispatcher + Processor stubs.
	got := runDispatchPath(t, logFile2026_05_08)

	// ── Basic game identity ───────────────────────────────────────────────────
	if ref.GameID != got.GameID {
		t.Errorf("GameID: ref=%q got=%q", ref.GameID, got.GameID)
	}
	if ref.Turn != got.Turn {
		t.Errorf("Turn: ref=%d got=%d", ref.Turn, got.Turn)
	}
	if ref.Placement != got.Placement {
		t.Errorf("Placement: ref=%d got=%d", ref.Placement, got.Placement)
	}

	// ── Duos-specific: IsDuos flag ────────────────────────────────────────────
	// Exercises: OnPlayerDef (BACON_DUO_TEAMMATE_PLAYER_ID), OnDuosTeammate,
	//            OnDuosPassableChanged, OnDuosPunishLeaversChanged.
	if ref.IsDuos != got.IsDuos {
		t.Errorf("IsDuos: ref=%v got=%v", ref.IsDuos, got.IsDuos)
	}

	// ── Local player fields ───────────────────────────────────────────────────
	if ref.Player.Name != got.Player.Name {
		t.Errorf("Player.Name: ref=%q got=%q", ref.Player.Name, got.Player.Name)
	}
	if ref.Player.HeroCardID != got.Player.HeroCardID {
		t.Errorf("Player.HeroCardID: ref=%q got=%q", ref.Player.HeroCardID, got.Player.HeroCardID)
	}
	if ref.Player.TavernTier != got.Player.TavernTier {
		t.Errorf("Player.TavernTier: ref=%d got=%d", ref.Player.TavernTier, got.Player.TavernTier)
	}
	if ref.Player.Health != got.Player.Health {
		t.Errorf("Player.Health: ref=%d got=%d", ref.Player.Health, got.Player.Health)
	}
	if ref.Player.Armor != got.Player.Armor {
		t.Errorf("Player.Armor: ref=%d got=%d", ref.Player.Armor, got.Player.Armor)
	}
	if ref.Player.Damage != got.Player.Damage {
		t.Errorf("Player.Damage: ref=%d got=%d", ref.Player.Damage, got.Player.Damage)
	}
	if ref.Player.TripleCount != got.Player.TripleCount {
		t.Errorf("Player.TripleCount: ref=%d got=%d", ref.Player.TripleCount, got.Player.TripleCount)
	}

	// ── Partner hero fields (Duos-specific) ──────────────────────────────────
	// Exercises: OnHeroRegistered (partner path), OnHeroStatChanged (partner path),
	//            OnPlayerTriplesChanged (partner path), OnTavernTierChanged (partner path).
	if ref.Partner == nil && got.Partner != nil {
		t.Errorf("Partner: ref=nil got=non-nil (%q)", got.Partner.Name)
	} else if ref.Partner != nil && got.Partner == nil {
		t.Errorf("Partner: ref=non-nil (%q) got=nil", ref.Partner.Name)
	} else if ref.Partner != nil && got.Partner != nil {
		if ref.Partner.Name != got.Partner.Name {
			t.Errorf("Partner.Name: ref=%q got=%q", ref.Partner.Name, got.Partner.Name)
		}
		if ref.Partner.HeroCardID != got.Partner.HeroCardID {
			t.Errorf("Partner.HeroCardID: ref=%q got=%q", ref.Partner.HeroCardID, got.Partner.HeroCardID)
		}
		if ref.Partner.TavernTier != got.Partner.TavernTier {
			t.Errorf("Partner.TavernTier: ref=%d got=%d", ref.Partner.TavernTier, got.Partner.TavernTier)
		}
		if ref.Partner.Health != got.Partner.Health {
			t.Errorf("Partner.Health: ref=%d got=%d", ref.Partner.Health, got.Partner.Health)
		}
		if ref.Partner.Armor != got.Partner.Armor {
			t.Errorf("Partner.Armor: ref=%d got=%d", ref.Partner.Armor, got.Partner.Armor)
		}
		if ref.Partner.TripleCount != got.Partner.TripleCount {
			t.Errorf("Partner.TripleCount: ref=%d got=%d", ref.Partner.TripleCount, got.Partner.TripleCount)
		}
	}

	// ── Buff sources (exercises OnEntityControllerChanged for Duos Dnt) ───────
	// In Duos, Dnt enchantments have CONTROLLER=<botID> but are ATTACHED to local hero.
	// isLocalDntTarget() relies on CONTROLLER tracking in entityController map,
	// which is populated by OnEntityControllerChanged.
	if !buffSourcesEqual(ref.BuffSources, got.BuffSources) {
		t.Errorf("BuffSources mismatch:\n  ref=%v\n  got=%v", ref.BuffSources, got.BuffSources)
	}
	if !abilityCountersEqual(ref.AbilityCounters, got.AbilityCounters) {
		t.Errorf("AbilityCounters mismatch:\n  ref=%v\n  got=%v", ref.AbilityCounters, got.AbilityCounters)
	}

	// ── Board (exercises OnMinionRegistered, enchantments) ───────────────────
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

	// ── Win/loss streaks ──────────────────────────────────────────────────────
	if ref.Player.WinStreak != got.Player.WinStreak {
		t.Errorf("Player.WinStreak: ref=%d got=%d", ref.Player.WinStreak, got.Player.WinStreak)
	}
	if ref.Player.LossStreak != got.Player.LossStreak {
		t.Errorf("Player.LossStreak: ref=%d got=%d", ref.Player.LossStreak, got.Player.LossStreak)
	}
}
