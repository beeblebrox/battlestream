package gamestate_test

// Integration tests against the 2026-05-08 game log (Power.log from a full BG Duos game).
//
// All expected values are derived from the raw log (ground truth), not from parser
// output. The log is at internal/gamestate/testdata/power_log_2026_05_08.txt.
//
// Game summary (from raw log analysis):
//   Mode:        Battlegrounds Duos
//   Hero:        Resistance Varden (BG22_HERO_004_SKIN_G, entity 99, player=3=Moch#1358)
//   Partner:     LoboSelvagem, hero TB_BaconShop_HERO_08_SKIN_D
//   Total turns: 14
//   Result:      LOST (Moch#1358), WON (AllWarNoLove)
//   Health:      30 (no health damage taken — 20 armor, all absorbed)
//   Armor:       0 (started with 20, fully depleted; damage=43 total)
//   Triples:     4
//   Tier:        4 at game end
//   Notable:     Final board is all-Beast; Lurking Leviathan (BG35_602) on board;
//                BG35_602e (Leviathan's Wrath) enchantment appears on final board minions.

import (
	"testing"

	"battlestream.fixates.io/internal/gamestate"
)

// TestGameLog2026_05_08_PlayerIdentification verifies the local player is Moch#1358.
func TestGameLog2026_05_08_PlayerIdentification(t *testing.T) {
	s := sharedLog20260508State(t)
	if s.Player.Name != "Moch#1358" {
		t.Errorf("Player.Name: expected Moch#1358, got %q", s.Player.Name)
	}
}

// TestGameLog2026_05_08_HeroCardID verifies the chosen hero is Resistance Varden.
func TestGameLog2026_05_08_HeroCardID(t *testing.T) {
	s := sharedLog20260508State(t)
	const want = "BG22_HERO_004_SKIN_G"
	if s.Player.HeroCardID != want {
		t.Errorf("HeroCardID: expected %q, got %q", want, s.Player.HeroCardID)
	}
}

// TestGameLog2026_05_08_Placement verifies Moch lost the game.
func TestGameLog2026_05_08_Placement(t *testing.T) {
	s := sharedLog20260508State(t)
	// LOST = non-zero placement (not 1st).
	if s.Placement == 1 {
		t.Errorf("Placement: should not be 1 (won), game was a loss")
	}
}

// TestGameLog2026_05_08_Phase verifies the game ended in GAME_OVER phase.
func TestGameLog2026_05_08_Phase(t *testing.T) {
	s := sharedLog20260508State(t)
	if s.Phase != gamestate.PhaseGameOver {
		t.Errorf("Phase: expected PhaseGameOver, got %s", s.Phase)
	}
}

// TestGameLog2026_05_08_FinalHealth verifies health was 30 (armor absorbed all damage).
// Raw log: Moch started with 20 armor; damage=43 total; health never dropped below 30.
func TestGameLog2026_05_08_FinalHealth(t *testing.T) {
	s := sharedLog20260508State(t)
	if s.Player.Health != 30 {
		t.Errorf("Health: expected 30, got %d", s.Player.Health)
	}
}

// TestGameLog2026_05_08_FinalArmor verifies armor ended at 0 (fully depleted by damage).
func TestGameLog2026_05_08_FinalArmor(t *testing.T) {
	s := sharedLog20260508State(t)
	if s.Player.Armor != 0 {
		t.Errorf("Armor: expected 0, got %d", s.Player.Armor)
	}
}

// TestGameLog2026_05_08_TavernTier verifies the game ended at tavern tier 4.
func TestGameLog2026_05_08_TavernTier(t *testing.T) {
	s := sharedLog20260508State(t)
	if s.TavernTier != 4 {
		t.Errorf("TavernTier: expected 4, got %d", s.TavernTier)
	}
}

// TestGameLog2026_05_08_Triples verifies 4 triples were made.
func TestGameLog2026_05_08_Triples(t *testing.T) {
	s := sharedLog20260508State(t)
	if s.Player.TripleCount != 4 {
		t.Errorf("TripleCount: expected 4, got %d", s.Player.TripleCount)
	}
}

// TestGameLog2026_05_08_Turn verifies the game ended on turn 14.
func TestGameLog2026_05_08_Turn(t *testing.T) {
	s := sharedLog20260508State(t)
	if s.Turn != 14 {
		t.Errorf("Turn: expected 14, got %d", s.Turn)
	}
}

// TestGameLog2026_05_08_DuosDetected verifies the game was detected as Duos.
func TestGameLog2026_05_08_DuosDetected(t *testing.T) {
	s := sharedLog20260508State(t)
	if !s.IsDuos {
		t.Error("IsDuos: expected true for this Duos game")
	}
}

// TestGameLog2026_05_08_PartnerName verifies the partner is LoboSelvagem.
func TestGameLog2026_05_08_PartnerName(t *testing.T) {
	s := sharedLog20260508State(t)
	if s.Partner == nil {
		t.Fatal("Partner: expected non-nil (Duos game)")
	}
	if s.Partner.Name != "LoboSelvagem" {
		t.Errorf("Partner.Name: expected LoboSelvagem, got %q", s.Partner.Name)
	}
}

// TestGameLog2026_05_08_FinalBoardSize verifies 6 minions on the final board.
func TestGameLog2026_05_08_FinalBoardSize(t *testing.T) {
	s := sharedLog20260508State(t)
	if len(s.Board) != 6 {
		t.Errorf("Board size: expected 6, got %d", len(s.Board))
	}
}

// TestGameLog2026_05_08_LurkingLeviathanOnBoard verifies Lurking Leviathan (BG35_602) is
// on the final board. This is notable as BG35_602 is the first BG35 card tracked by the
// parser; its presence confirms new-set card IDs are handled correctly.
func TestGameLog2026_05_08_LurkingLeviathanOnBoard(t *testing.T) {
	s := sharedLog20260508State(t)
	var found bool
	for _, m := range s.Board {
		if m.CardID == "BG35_602" {
			found = true
			if m.Attack != 7 {
				t.Errorf("Lurking Leviathan attack: expected 7, got %d", m.Attack)
			}
			if m.Health != 12 {
				t.Errorf("Lurking Leviathan health: expected 12, got %d", m.Health)
			}
			if m.Name != "Lurking Leviathan" {
				t.Errorf("Lurking Leviathan name: expected %q, got %q", "Lurking Leviathan", m.Name)
			}
			break
		}
	}
	if !found {
		t.Error("Lurking Leviathan (BG35_602) not found on final board")
	}
}

// TestGameLog2026_05_08_LeviathanWrathEnchantment verifies that BG35_602e (Leviathan's Wrath)
// appears as an enchantment on at least one board minion.
// Currently tracked as category GENERAL since no specific category is mapped for BG35_602e yet.
func TestGameLog2026_05_08_LeviathanWrathEnchantment(t *testing.T) {
	s := sharedLog20260508State(t)
	var found bool
	for _, m := range s.Board {
		for _, e := range m.Enchantments {
			if e.CardID == "BG35_602e" {
				found = true
				if e.SourceCardID != "BG35_602" {
					t.Errorf("BG35_602e SourceCardID: expected BG35_602, got %q", e.SourceCardID)
				}
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Error("BG35_602e (Leviathan's Wrath) enchantment not found on any board minion")
	}
}

// TestGameLog2026_05_08_AllBoardMinionsAreBeasts verifies the final board is all Beasts.
// This is the expected Beast-heavy build enabled by Lurking Leviathan.
func TestGameLog2026_05_08_AllBoardMinionsAreBeasts(t *testing.T) {
	s := sharedLog20260508State(t)
	for _, m := range s.Board {
		if m.MinionType != "BEAST" {
			t.Errorf("board minion %q (%s): expected BEAST type, got %q", m.Name, m.CardID, m.MinionType)
		}
	}
}

// TestGameLog2026_05_08_ShopBuffSource verifies the SHOP_BUFF buff source accumulated 79/79.
// Raw log: BG_ShopBuff player-level Dnt counter reached 79 ATK and 79 HP.
// Note: this value (79/79) previously caused display corruption on Stream Deck due to
// text overflow at fixed 52px font; the fitText auto-scaling fix was applied to address this.
func TestGameLog2026_05_08_ShopBuffSource(t *testing.T) {
	s := sharedLog20260508State(t)
	for _, bs := range s.BuffSources {
		if bs.Category == "SHOP_BUFF" {
			if bs.Attack != 79 {
				t.Errorf("SHOP_BUFF attack: expected 79, got %d", bs.Attack)
			}
			if bs.Health != 79 {
				t.Errorf("SHOP_BUFF health: expected 79, got %d", bs.Health)
			}
			return
		}
	}
	t.Error("SHOP_BUFF category not found in buff_sources")
}

// TestGameLog2026_05_08_FreeRefreshCounter verifies the FREE_REFRESH ability counter is 1.
func TestGameLog2026_05_08_FreeRefreshCounter(t *testing.T) {
	s := sharedLog20260508State(t)
	for _, ac := range s.AbilityCounters {
		if ac.Category == "FREE_REFRESH" {
			if ac.Value != 1 {
				t.Errorf("FREE_REFRESH value: expected 1, got %d", ac.Value)
			}
			return
		}
	}
	t.Error("FREE_REFRESH category not found in ability_counters")
}
