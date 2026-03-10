package gamestate

import (
	"bufio"
	"os"
	"testing"

	"battlestream.fixates.io/internal/parser"
)

// TestDebugTurn8Board traces the board state at turn 8.
func TestDebugTurn8Board(t *testing.T) {
	f, err := os.Open("testdata/power_log_2026_03_08b.txt")
	if err != nil {
		t.Skipf("test fixture not available: %v", err)
	}
	defer f.Close()

	ch := make(chan parser.GameEvent, 256)
	p := parser.New(ch)
	m := New()
	proc := NewProcessor(m)

	// Track board state changes
	lastTurnReported := 0
	done := make(chan struct{})
	go func() {
		for e := range ch {
			proc.Handle(e)
			s := m.State()

			// Report board state when turn changes to 8
			if s.Turn == 8 && lastTurnReported < 8 {
				t.Logf("=== Turn 8 START (first time) ===")
				t.Logf("Board has %d minions:", len(s.Board))
				for i, mn := range s.Board {
					t.Logf("  [%d] %q (id=%d) %d/%d ATK/HP", i, mn.Name, mn.EntityID, mn.Attack, mn.Health)
				}
				t.Logf("Player RESOURCES: %d\n", s.Player.CurrentGold)
				lastTurnReported = 8
			}
		}
		close(done)
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		p.Feed(scanner.Text())
	}
	p.Flush()
	close(ch)
	<-done

	s := m.State()
	t.Logf("=== FINAL STATE ===")
	t.Logf("Board has %d minions:", len(s.Board))
	for i, mn := range s.Board {
		t.Logf("  [%d] %q (id=%d) %d/%d ATK/HP", i, mn.Name, mn.EntityID, mn.Attack, mn.Health)
	}
	t.Logf("Phase: %s", s.Phase)

	// Regression: board must never exceed 7 minions (ghost minion fix).
	if len(s.Board) > 7 {
		t.Errorf("Board has %d minions (max 7) — ghost minion bug", len(s.Board))
	}
}
