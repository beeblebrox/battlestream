package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	bspb "battlestream.fixates.io/internal/api/grpc/gen/battlestream/v1"
)

// makeTestGame builds a realistic game state with populated fields for testing.
func makeTestGame() *bspb.GameState {
	return &bspb.GameState{
		GameId:     "test-game-001",
		Phase:      "RECRUIT",
		Turn:       7,
		TavernTier: 4,
		Player: &bspb.PlayerStats{
			Name:       "TestPlayer#1234",
			HeroCardId: "BG_HERO_01",
			Health:     30,
			MaxHealth:  40,
			Damage:     8,
			Armor:      5,
			SpellPower: 2,
			TripleCount: 3,
			WinStreak:  2,
		},
		Board: []*bspb.MinionState{
			{EntityId: 1, Name: "Crackling Cyclone", CardId: "BG21_007", Attack: 7, Health: 7, MinionType: "ELEMENTAL", BuffAttack: 3, BuffHealth: 3},
			{EntityId: 2, Name: "Wildfire Elemental", CardId: "BG21_009", Attack: 14, Health: 13, MinionType: "ELEMENTAL"},
			{EntityId: 3, Name: "Flaming Enforcer", CardId: "BG30_100", Attack: 8, Health: 13, MinionType: "ELEMENTAL", BuffAttack: 5, BuffHealth: 10},
			{EntityId: 4, Name: "Rodeo Performer", CardId: "BG29_875", Attack: 3, Health: 4, MinionType: "BEAST"},
			{EntityId: 5, Name: "Brann Bronzebeard", CardId: "BG_LOE_077", Attack: 2, Health: 4, MinionType: "INVALID"},
			{EntityId: 6, Name: "Friendly Saloonkeeper", CardId: "BG30_101", Attack: 3, Health: 5, MinionType: "ELEMENTAL"},
			{EntityId: 7, Name: "Friendly Saloonkeeper", CardId: "BG30_101", Attack: 3, Health: 8, MinionType: "ELEMENTAL", BuffAttack: 0, BuffHealth: 3},
		},
		BuffSources: []*bspb.BuffSource{
			{Category: "BLOODGEM", Attack: 12, Health: 15},
			{Category: "NOMI", Attack: 8, Health: 8},
			{Category: "TAVERN_SPELL", Attack: 3, Health: 5},
			{Category: "WHELP", Attack: 6, Health: 0},
			{Category: "ELEMENTAL", Attack: 4, Health: 4},
		},
		AbilityCounters: []*bspb.AbilityCounter{
			{Category: "FREE_REFRESH", Value: 3, Display: "3"},
			{Category: "GOLD_NEXT_TURN", Value: 2, Display: "+2g"},
		},
		AvailableTribes: []string{"Demon", "Elemental", "Murloc", "Pirate", "Beast"},
	}
}

func makeTestAgg() *bspb.AggregateStats {
	return &bspb.AggregateStats{
		GamesPlayed:   5,
		Wins:          3,
		Losses:        2,
		AvgPlacement:  3.4,
		BestPlacement: 1,
	}
}

// renderTestView creates a Model with the given dimensions and renders it.
func renderTestView(game *bspb.GameState, agg *bspb.AggregateStats, width, height int) string {
	m := &Model{
		connState: stateConnected,
		game:      game,
		agg:       agg,
		width:     width,
		height:    height,
	}
	return m.View()
}

func TestView_FitsWithinHeight(t *testing.T) {
	game := makeTestGame()
	agg := makeTestAgg()

	heights := []int{30, 35, 40, 45, 50, 60}
	for _, h := range heights {
		t.Run(strings.Replace(strings.Replace(
			strings.Replace("h="+string(rune('0'+h/10))+string(rune('0'+h%10)), "\n", "", -1),
			"\r", "", -1), "\t", "", -1),
			func(t *testing.T) {
				out := renderTestView(game, agg, 120, h)
				lines := lipgloss.Height(out)
				if lines > h {
					t.Errorf("output height %d exceeds terminal height %d (overflow by %d)", lines, h, lines-h)
				}
			})
	}
}

func TestView_TopLineNotClipped(t *testing.T) {
	game := makeTestGame()
	agg := makeTestAgg()

	out := renderTestView(game, agg, 120, 40)
	lines := strings.Split(out, "\n")

	if len(lines) == 0 {
		t.Fatal("empty output")
	}

	// The first line should be a border top (╭──...──╮).
	first := stripANSI(lines[0])
	if !strings.HasPrefix(first, "╭") {
		t.Errorf("first line should start with border top character '╭', got: %q", first)
	}

	// The second line should contain "BATTLESTREAM".
	if len(lines) < 2 {
		t.Fatal("output too short")
	}
	second := stripANSI(lines[1])
	if !strings.Contains(second, "BATTLESTREAM") {
		t.Errorf("second line should contain 'BATTLESTREAM', got: %q", second)
	}
}

func TestView_AllPanelsPresent(t *testing.T) {
	game := makeTestGame()
	agg := makeTestAgg()

	out := renderTestView(game, agg, 120, 40)
	stripped := stripANSI(out)

	// Check that all major panels render their titles.
	for _, panel := range []string{
		"BATTLESTREAM",
		"TestPlayer#1234",
		"YOUR BOARD",
		"BUFF SOURCES",
		"SESSION",
	} {
		if !strings.Contains(stripped, panel) {
			t.Errorf("expected panel %q not found in output", panel)
		}
	}

	// Check that board minions appear.
	for _, minion := range []string{
		"Crackling Cyclone",
		"Wildfire Elemental",
		"Brann Bronzebeard",
	} {
		if !strings.Contains(stripped, minion) {
			t.Errorf("expected minion %q not found in output", minion)
		}
	}

	// Check that buff sources appear.
	for _, buff := range []string{
		"+12/+15",
		"+8/+8",
	} {
		if !strings.Contains(stripped, buff) {
			t.Errorf("expected buff %q not found in output", buff)
		}
	}
}

func TestView_GameOverPhase(t *testing.T) {
	game := makeTestGame()
	game.Phase = "GAME_OVER"
	game.Placement = 2
	agg := makeTestAgg()

	out := renderTestView(game, agg, 120, 40)
	stripped := stripANSI(out)

	if !strings.Contains(stripped, "FINAL BOARD") {
		t.Error("expected 'FINAL BOARD' in GAME_OVER phase")
	}
	if !strings.Contains(stripped, "WIN #2") {
		t.Error("expected 'WIN #2' for placement 2")
	}
}

func TestView_MultipleWidths(t *testing.T) {
	game := makeTestGame()
	agg := makeTestAgg()

	widths := []int{80, 100, 120, 160, 200}
	for _, w := range widths {
		out := renderTestView(game, agg, w, 40)
		lines := lipgloss.Height(out)
		if lines > 40 {
			t.Errorf("width=%d: output height %d exceeds terminal height 40", w, lines)
		}
		if out == "" {
			t.Errorf("width=%d: empty output", w)
		}
	}
}

func TestView_NilGameState(t *testing.T) {
	agg := makeTestAgg()
	out := renderTestView(nil, agg, 120, 40)
	lines := lipgloss.Height(out)
	if lines > 40 {
		t.Errorf("nil game: output height %d exceeds terminal height 40", lines)
	}
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "waiting for game") {
		t.Error("expected 'waiting for game' for nil game state")
	}
}

// stripANSI removes ANSI escape sequences for readable diff output.
func stripANSI(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // consume 'm'
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func TestRenderTavernTierAnomalyTier7(t *testing.T) {
	result := renderTavernTier(7)
	if result == "" {
		t.Fatal("renderTavernTier(7) returned empty string")
	}
	if !strings.Contains(result, "7") {
		t.Errorf("expected tier 7 to contain '7', got %q", result)
	}
	// Should have 7 filled stars and 0 empty stars.
	if strings.Contains(result, "☆") {
		t.Errorf("tier 7 should have no empty stars, got %q", result)
	}
}
