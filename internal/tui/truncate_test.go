package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	bspb "battlestream.fixates.io/internal/api/grpc/gen/battlestream/v1"
)

// TestRenderMinion_MultibyteName is a regression test for byte-indexed
// truncation: minion names containing multi-byte runes (e.g. "Brûlée",
// localized card names) were sliced mid-rune, producing invalid UTF-8
// garbage in the board panel.
func TestRenderMinion_MultibyteName(t *testing.T) {
	mn := &bspb.MinionState{
		Name:   strings.Repeat("é", 30), // 30 runes, 60 bytes — forces truncation
		Attack: 5,
		Health: 7,
	}
	out := renderMinion(mn, 24)
	if !utf8.ValidString(out) {
		t.Errorf("renderMinion produced invalid UTF-8: %q", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("expected truncated name to end with ellipsis, got %q", out)
	}
}

// TestTruncatePath_Multibyte covers the discovery wizard's path display:
// paths with multi-byte runes (e.g. a user dir like /home/jürgen) were
// sliced mid-rune when shortened with a leading ellipsis.
func TestTruncatePath_Multibyte(t *testing.T) {
	p := "/home/" + strings.Repeat("ü", 40) + "/Hearthstone"
	out := truncatePath(p, 20)
	if !utf8.ValidString(out) {
		t.Errorf("truncatePath produced invalid UTF-8: %q", out)
	}
	if got := utf8.RuneCountInString(out); got > 20 {
		t.Errorf("truncatePath result is %d runes, want <= 20: %q", got, out)
	}
	if !strings.HasPrefix(out, "…") {
		t.Errorf("expected leading ellipsis, got %q", out)
	}
	if !strings.HasSuffix(out, "/Hearthstone") {
		t.Errorf("expected path tail preserved, got %q", out)
	}
}
