package debugtui

import (
	"testing"

	"battlestream.fixates.io/internal/parser"
)

func TestLoadReplay(t *testing.T) {
	replay, err := LoadReplay("../gamestate/testdata/power_log_game.txt")
	if err != nil {
		t.Fatalf("LoadReplay: %v", err)
	}
	if len(replay.Steps) == 0 {
		t.Fatal("expected steps > 0")
	}
	t.Logf("Total steps: %d", len(replay.Steps))
	t.Logf("Total games: %d", len(replay.Games))

	if len(replay.Games) == 0 {
		t.Fatal("expected at least 1 game")
	}

	// Count by type
	counts := map[parser.EventType]int{}
	for _, s := range replay.Steps {
		counts[s.Event.Type]++
	}
	for typ, c := range counts {
		t.Logf("  %-20s %d", typ, c)
	}

	// Verify game summary
	g := replay.Games[len(replay.Games)-1]
	t.Logf("Last game: player=%q hero=%q place=%d turns=%d tier=%d phase=%s",
		g.PlayerName, g.HeroCardID, g.Placement, g.MaxTurn, g.TavernTier, g.Phase)

	if g.Phase != "GAME_OVER" {
		t.Errorf("expected GAME_OVER, got %s", g.Phase)
	}
	if g.PlayerName != "Moch#1358" {
		t.Errorf("expected player Moch#1358, got %q", g.PlayerName)
	}

	// Verify step slicing
	gameSteps := replay.Steps[g.StepStart:g.StepEnd]
	last := gameSteps[len(gameSteps)-1]
	if len(last.State.Board) != 6 {
		t.Errorf("expected 6 board minions, got %d", len(last.State.Board))
	}
	for _, mn := range last.State.Board {
		t.Logf("  %s %d/%d", mn.Name, mn.Attack, mn.Health)
	}

	if g.MaxTurn < 5 {
		t.Errorf("expected max turn >= 5, got %d", g.MaxTurn)
	}
}

func TestLoadAllGamesMultiFile(t *testing.T) {
	// Loading the same file twice should produce 2x the games.
	path := "../gamestate/testdata/power_log_game.txt"
	replay, err := LoadAllGames([]string{path, path})
	if err != nil {
		t.Fatalf("LoadAllGames: %v", err)
	}

	single, err := LoadReplay(path)
	if err != nil {
		t.Fatalf("LoadReplay: %v", err)
	}

	if len(replay.Games) != 2*len(single.Games) {
		t.Errorf("expected %d games from double load, got %d",
			2*len(single.Games), len(replay.Games))
	}
	t.Logf("Single: %d games, Double: %d games", len(single.Games), len(replay.Games))
}
