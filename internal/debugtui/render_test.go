package debugtui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/parser"
)

// Regression tests for byte-indexed truncation/wrapping: multi-byte runes
// (accented letters, CJK, emoji in player names) were sliced mid-rune,
// producing invalid UTF-8 in the replay viewer.

func TestRenderMinion_MultibyteName(t *testing.T) {
	mn := gamestate.MinionState{
		Name:   strings.Repeat("é", 30), // 60 bytes, 30 runes — forces truncation
		Attack: 3,
		Health: 4,
	}
	out := renderMinion(mn)
	if !utf8.ValidString(out) {
		t.Errorf("renderMinion produced invalid UTF-8: %q", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("expected truncated name to contain ellipsis, got %q", out)
	}
}

func TestRenderEvent_MultibyteEntityName(t *testing.T) {
	e := parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityName: strings.Repeat("é", 50), // 100 bytes, 50 runes
	}
	out := renderEvent(e)
	if !utf8.ValidString(out) {
		t.Errorf("renderEvent produced invalid UTF-8: %q", out)
	}
}

func TestWrapLine_Multibyte(t *testing.T) {
	line := strings.Repeat("é", 40) // 80 bytes, 40 runes
	chunks := wrapLine(line, 25)
	for i, c := range chunks {
		if !utf8.ValidString(c) {
			t.Errorf("chunk %d is invalid UTF-8: %q", i, c)
		}
		if n := utf8.RuneCountInString(c); n > 25 {
			t.Errorf("chunk %d is %d runes, want <= 25", i, n)
		}
	}
	if got := strings.Join(chunks, ""); got != line {
		t.Errorf("wrapped chunks do not reassemble the original line:\n got %q\nwant %q", got, line)
	}
}

func TestWrapLine_ShortLineUnchanged(t *testing.T) {
	chunks := wrapLine("short", 25)
	if len(chunks) != 1 || chunks[0] != "short" {
		t.Errorf("wrapLine(short) = %q, want [short]", chunks)
	}
}
