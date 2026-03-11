package debugtui

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"battlestream.fixates.io/internal/parser"
)

var updateGolden = flag.Bool("update-golden", false, "overwrite golden files with current output")

const testLog = "../gamestate/testdata/power_log_game.txt"
const goldenDir = "testdata/golden"

// sharedReplay caches the parsed replay so the log file is only read once per
// test run instead of once per test function (~10s per parse with race detector).
var (
	sharedReplay     *Replay
	sharedReplayOnce sync.Once
	sharedReplayErr  error
)

func getSharedReplay(t *testing.T) *Replay {
	t.Helper()
	sharedReplayOnce.Do(func() {
		sharedReplay, sharedReplayErr = LoadReplay(testLog)
	})
	if sharedReplayErr != nil {
		t.Fatalf("LoadReplay: %v", sharedReplayErr)
	}
	return sharedReplay
}

func TestLoadReplay(t *testing.T) {
	replay := getSharedReplay(t)
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

func TestDump_DoesNotPanic(t *testing.T) {
	replay := getSharedReplay(t)
	out, err := DumpFromReplay(replay, 1, 120)
	if err != nil {
		t.Fatalf("DumpFromReplay: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestDump_LastTurn(t *testing.T) {
	replay := getSharedReplay(t)
	out, err := DumpFromReplay(replay, 999, 120)
	if err != nil {
		t.Fatalf("DumpFromReplay: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}

// TestDump_Golden captures TUI output for specific turns as golden screenshots.
// Run with -update-golden to regenerate the golden files.
//
// Each golden file stores raw TUI output (including ANSI colour codes) for an
// exact terminal width of 120 columns so diffs are stable across runs.
func TestDump_Golden(t *testing.T) {
	replay := getSharedReplay(t)
	cases := []struct {
		turn        int
		description string
	}{
		{8, "first-turn"},  // earliest available turn in this log (reconnect mid-game at turn 8)
		{10, "mid-game"},   // middle of the available turns (8–12)
		{0, "last-turn"},   // turn=0 → jump to last step
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			got, err := DumpFromReplay(replay, tc.turn, 120)
			if err != nil {
				t.Fatalf("DumpFromReplay(turn=%d): %v", tc.turn, err)
			}
			if got == "" {
				t.Fatal("empty output")
			}

			goldenFile := fmt.Sprintf("%s/%s.txt", goldenDir, tc.description)

			if *updateGolden {
				if err := os.MkdirAll(goldenDir, 0o755); err != nil {
					t.Fatalf("mkdir golden: %v", err)
				}
				if err := os.WriteFile(goldenFile, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("updated %s", goldenFile)
				return
			}

			want, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatalf("golden file missing — run: go test ./internal/debugtui/ -update-golden\nerr: %v", err)
			}

			if got != string(want) {
				// Show a line-by-line diff summary (ANSI stripped for readability).
				gotLines := strings.Split(stripANSI(got), "\n")
				wantLines := strings.Split(stripANSI(string(want)), "\n")
				t.Errorf("golden mismatch for %s (turn=%d): got %d lines, want %d lines",
					tc.description, tc.turn, len(gotLines), len(wantLines))
				for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
					g, w := "", ""
					if i < len(gotLines) {
						g = gotLines[i]
					}
					if i < len(wantLines) {
						w = wantLines[i]
					}
					if g != w {
						t.Logf("  line %d\n    got:  %q\n    want: %q", i+1, g, w)
					}
				}
			}
		})
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

// TestDump_FitsWithinHeight verifies the rendered output never exceeds the
// declared terminal height, which would cause the top line to be clipped.
func TestDump_FitsWithinHeight(t *testing.T) {
	replay := getSharedReplay(t)

	cases := []struct {
		turn   int
		width  int
		height int
	}{
		{1, 120, 30},
		{1, 120, 40},
		{1, 120, 50},
		{5, 120, 30},
		{5, 120, 40},
		{5, 120, 50},
		{0, 120, 30}, // last turn
		{0, 120, 40},
		{0, 120, 50},
		{5, 80, 40},  // narrow
		{5, 160, 40}, // wide
		{5, 200, 40}, // very wide
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("turn=%d/w=%d/h=%d", tc.turn, tc.width, tc.height), func(t *testing.T) {
			m := NewFromReplay(replay)
			m.width = tc.width
			m.height = tc.height
			m.selectGame(0)
			if tc.turn == 0 {
				if len(m.filtered) > 0 {
					m.cursor = len(m.filtered) - 1
				}
			} else {
				m.jumpToTurn(tc.turn)
			}
			out := m.View()
			lines := strings.Count(out, "\n") + 1
			// Trailing newline should not count as an extra line.
			if strings.HasSuffix(out, "\n") {
				lines--
			}
			if lines > tc.height {
				t.Errorf("output %d lines exceeds terminal height %d (overflow by %d)",
					lines, tc.height, lines-tc.height)
			}
		})
	}
}

// TestDump_TopLineNotClipped verifies the first visible line is the header
// border, not a clipped partial line.
func TestDump_TopLineNotClipped(t *testing.T) {
	replay := getSharedReplay(t)

	turns := []int{1, 5, 0}
	for _, turn := range turns {
		t.Run(fmt.Sprintf("turn=%d", turn), func(t *testing.T) {
			out, err := DumpFromReplay(replay, turn, 120)
			if err != nil {
				t.Fatalf("DumpFromReplay: %v", err)
			}
			lines := strings.Split(out, "\n")
			if len(lines) == 0 {
				t.Fatal("empty output")
			}

			// First line should be a border top.
			first := stripANSI(lines[0])
			if !strings.HasPrefix(first, "╭") {
				t.Errorf("first line should start with '╭', got: %q", first)
			}

			// Second line should contain "REPLAY".
			if len(lines) < 2 {
				t.Fatal("output too short")
			}
			second := stripANSI(lines[1])
			if !strings.Contains(second, "REPLAY") {
				t.Errorf("second line should contain 'REPLAY', got: %q", second)
			}
		})
	}
}

// TestDump_AllPanelsPresent verifies all TUI panels render with content.
func TestDump_AllPanelsPresent(t *testing.T) {
	replay := getSharedReplay(t)

	// Use last turn to get a fully populated game state.
	out, err := DumpFromReplay(replay, 0, 120)
	if err != nil {
		t.Fatalf("DumpFromReplay: %v", err)
	}
	stripped := stripANSI(out)

	panels := []string{
		"REPLAY",
		"Moch#1358",  // player name
		"BOARD",       // board panel title
		"BUFF SOURCES",
		"CHANGES",
		"RAW LOG",
	}
	for _, panel := range panels {
		if !strings.Contains(stripped, panel) {
			t.Errorf("expected panel %q not found in output", panel)
		}
	}

	// Check that board minions appear (last turn has 6 minions).
	minionCount := 0
	for _, line := range strings.Split(stripped, "\n") {
		// Minion lines have the format "  <name>  <atk>/<hp>"
		if strings.Contains(line, "/") && (strings.Contains(line, "Elemental") ||
			strings.Contains(line, "Cyclone") || strings.Contains(line, "Enforcer") ||
			strings.Contains(line, "Saloonkeeper") || strings.Contains(line, "Performer") ||
			strings.Contains(line, "Bronzebeard")) {
			minionCount++
		}
	}
	if minionCount == 0 {
		t.Error("expected at least one minion in the board panel")
	}
}

func TestLoadAllGamesMultiFile(t *testing.T) {
	// Loading the same file twice should produce 2x the games.
	// This test must call LoadAllGames directly (not the shared replay)
	// because it tests multi-file loading specifically.
	path := testLog
	replay, err := LoadAllGames([]string{path, path})
	if err != nil {
		t.Fatalf("LoadAllGames: %v", err)
	}

	single := getSharedReplay(t)

	if len(replay.Games) != 2*len(single.Games) {
		t.Errorf("expected %d games from double load, got %d",
			2*len(single.Games), len(replay.Games))
	}
	t.Logf("Single: %d games, Double: %d games", len(single.Games), len(replay.Games))
}
