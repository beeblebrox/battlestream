package debugtui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TestRenderTavernTier_Tier7NoPanic is a regression test for the tier-7
// anomaly tier: strings.Repeat("☆", 6-tier) panicked with a negative count.
// The live TUI guards this (tui.go renderTavernTier); the debugtui copy must too.
func TestRenderTavernTier_Tier7NoPanic(t *testing.T) {
	for tier := 0; tier <= 7; tier++ {
		out := renderTavernTier(tier) // must not panic for any tier
		if tier > 0 && out == "" {
			t.Errorf("tier %d: expected non-empty output", tier)
		}
	}

	// Tier 7 shows 7 filled stars and no empty stars.
	out := stripANSI(renderTavernTier(7))
	if !strings.HasPrefix(out, "7 ") {
		t.Errorf("tier 7: expected prefix \"7 \", got %q", out)
	}
	if got := strings.Count(out, "★"); got != 7 {
		t.Errorf("tier 7: expected 7 filled stars, got %d (%q)", got, out)
	}
	if strings.Contains(out, "☆") {
		t.Errorf("tier 7: expected no empty stars, got %q", out)
	}
}

// TestTavernTierColor_Tier7 verifies the anomaly tier gets the magenta color,
// matching the live TUI (tui.go tavernTierColor case 7).
func TestTavernTierColor_Tier7(t *testing.T) {
	if got, want := tavernTierColor(7), lipgloss.Color("201"); got != want {
		t.Errorf("tavernTierColor(7) = %q, want %q (magenta, anomaly tier)", got, want)
	}
}

// TestView_LoadingWithNilProgress_NoPanic is a regression test: the combined
// TUI constructs the model via NewFromReplay, which did not initialize
// m.progress; pressing L (switch source) sets m.loading=true, and the next
// View dereferenced the nil progress pointer.
func TestView_LoadingWithNilProgress_NoPanic(t *testing.T) {
	m := NewFromReplay(&Replay{})
	m.width = 120
	m.height = 40
	m.loading = true // what switchSource() does before the reload completes

	out := m.View() // must not panic
	if !strings.Contains(stripANSI(out), "Loading") {
		t.Errorf("expected loading screen, got %q", out)
	}
}

// TestUpdate_LoadErrEmbedded_DoesNotQuit is a regression test: when embedded
// in the combined TUI, a replay load error must not quit the whole program
// (it used to return tea.Quit on any key, killing the live dashboard too).
func TestUpdate_LoadErrEmbedded_DoesNotQuit(t *testing.T) {
	m := NewFromReplay(&Replay{})
	m.SetEmbedded(true)
	m.loadErr = fmt.Errorf("boom")

	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	_, cmd := m.Update(key)
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("embedded model with loadErr returned tea.Quit on a plain key; this kills the combined TUI")
		}
	}
}

// TestHandleMouse_ParentYOffset is a regression test for the off-by-one in
// the combined TUI's replay tab: the combined view reserves row 0 for the tab
// bar, shifting all mouse Y coordinates down by one, but hit-testing used raw
// msg.Y. With SetParentYOffset(1), a click on the last scrollbar row (terminal
// Y = view Y + 1) must register.
func TestHandleMouse_ParentYOffset(t *testing.T) {
	newModel := func(offset int) *Model {
		m := NewFromReplay(&Replay{})
		m.SetParentYOffset(offset)
		// Board scrollbar track at X=5, view rows 3..6 inclusive.
		m.boardScrollX = 5
		m.boardVPY = 3
		m.boardVPH = 4
		return m
	}
	press := func(m *Model, x, y int) {
		m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	}

	// Embedded (offset 1): terminal Y=7 maps to view Y=6 — last track row.
	m := newModel(1)
	press(m, 5, 7)
	if !m.scrubbing {
		t.Error("offset=1: click at terminal Y=7 (view Y=6) must hit the scrollbar track")
	}

	// Embedded: terminal Y=3 maps to view Y=2 — just above the track.
	m = newModel(1)
	press(m, 5, 3)
	if m.scrubbing {
		t.Error("offset=1: click at terminal Y=3 (view Y=2) must miss the scrollbar track")
	}

	// Standalone (offset 0): terminal Y=7 is below the track and must miss.
	m = newModel(0)
	press(m, 5, 7)
	if m.scrubbing {
		t.Error("offset=0: click at terminal Y=7 must miss the scrollbar track")
	}
}

// TestUpdate_LoadErrStandalone_QuitsOnKey verifies the standalone replay
// command keeps its "press any key to exit" behavior on load errors.
func TestUpdate_LoadErrStandalone_QuitsOnKey(t *testing.T) {
	m := NewFromReplay(&Replay{})
	m.loadErr = fmt.Errorf("boom")

	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	_, cmd := m.Update(key)
	if cmd == nil {
		t.Fatal("standalone model with loadErr: expected tea.Quit, got nil cmd")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatal("standalone model with loadErr: expected tea.Quit on key press")
	}
}
