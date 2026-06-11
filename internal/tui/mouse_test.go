package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func newWheelTestModel() *Model {
	m := &Model{
		connState: stateConnected,
		width:     120,
		height:    40,
	}
	content := strings.TrimRight(strings.Repeat("line\n", 50), "\n")
	m.boardVP = viewport.New(40, 5)
	m.boardVP.SetContent(content)
	m.boardVP.MouseWheelEnabled = true
	m.modsVP = viewport.New(40, 5)
	m.modsVP.SetContent(content)
	m.modsVP.MouseWheelEnabled = true
	return m
}

func wheelDownAt(x, y int) tea.MouseMsg {
	return tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
	}
}

// TestWheelRouting_UsesDividerPosition is a regression test: the wheel was
// routed by m.width/2, but the vertical divider is draggable (m.dividerX).
// After dragging the divider away from center, scrolling over a panel must
// target the panel actually under the cursor.
func TestWheelRouting_UsesDividerPosition(t *testing.T) {
	m := newWheelTestModel()
	m.dividerX = 30 // divider dragged left of center (width/2 = 60)

	// Cursor at x=45: right of the divider (30), left of width/2 (60).
	// This is over the right (mods) panel — the wheel must scroll modsVP.
	m.Update(wheelDownAt(45, 10))

	if m.modsVP.YOffset == 0 {
		t.Error("wheel over right panel (between dividerX and width/2) did not scroll modsVP")
	}
	if m.boardVP.YOffset != 0 {
		t.Errorf("wheel over right panel scrolled boardVP (YOffset=%d)", m.boardVP.YOffset)
	}

	// And left of the divider, the wheel must scroll the board panel.
	m2 := newWheelTestModel()
	m2.dividerX = 30
	m2.Update(wheelDownAt(20, 10))
	if m2.boardVP.YOffset == 0 {
		t.Error("wheel over left panel did not scroll boardVP")
	}
	if m2.modsVP.YOffset != 0 {
		t.Errorf("wheel over left panel scrolled modsVP (YOffset=%d)", m2.modsVP.YOffset)
	}
}

// TestWheelRouting_FallbackBeforeFirstView: before View has computed
// m.dividerX (0), routing falls back to the width midpoint instead of
// sending every wheel event to the right panel.
func TestWheelRouting_FallbackBeforeFirstView(t *testing.T) {
	m := newWheelTestModel()
	m.dividerX = 0 // View not rendered yet

	m.Update(wheelDownAt(20, 10)) // left half of a 120-wide screen
	if m.boardVP.YOffset == 0 {
		t.Error("with dividerX unset, wheel on left half did not scroll boardVP")
	}
	if m.modsVP.YOffset != 0 {
		t.Errorf("with dividerX unset, wheel on left half scrolled modsVP (YOffset=%d)", m.modsVP.YOffset)
	}
}
