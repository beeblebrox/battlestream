package gamestate

import (
	"testing"

	"battlestream.fixates.io/internal/parser"
)

// TestCardNameKnownCard verifies that CardName returns the friendly display name
// for card IDs that exist in the map.
func TestCardNameKnownCard(t *testing.T) {
	cases := []struct {
		cardID string
		want   string
	}{
		{"BG_LOE_077", "Brann Bronzebeard"},
		{"BG34_403", "Eternal Tycoon"},
	}
	for _, tc := range cases {
		got := CardName(tc.cardID)
		if got != tc.want {
			t.Errorf("CardName(%q) = %q, want %q", tc.cardID, got, tc.want)
		}
	}
}

// TestCardNameUnknownCardFallsBackToCardID verifies that CardName returns the
// raw CardID when the card is not in the map, not an empty string.
func TestCardNameUnknownCardFallsBackToCardID(t *testing.T) {
	unknown := "FAKE_CARD_12345_XYZ"
	got := CardName(unknown)
	if got != unknown {
		t.Errorf("CardName(%q) = %q, want %q (fallback to CardID)", unknown, got, unknown)
	}
}

// TestCardNameEmptyCardIDReturnsEmpty verifies that CardName("") returns "",
// not a nil-deref or panic.
func TestCardNameEmptyCardID(t *testing.T) {
	got := CardName("")
	if got != "" {
		t.Errorf("CardName(\"\") = %q, want empty string", got)
	}
}

// TestProcessorMinionCardNameFallback verifies that when a minion arrives in an
// EntityUpdate with a known CardID but no EntityName, the board entry gets the
// friendly card name via CardName() lookup rather than a blank or raw CardID.
func TestProcessorMinionCardNameFallback(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Advance to RECRUIT phase (GameEntity turn 1 = odd = recruit).
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "1"},
	})

	// Send a FULL_ENTITY-style update for Brann with CardID but no EntityName.
	const brannCardID = "BG_LOE_077"
	p.Handle(parser.GameEvent{
		Type:       parser.EventEntityUpdate,
		EntityID:   101,
		PlayerID:   7,
		EntityName: "", // deliberately empty — should trigger CardName fallback
		CardID:     brannCardID,
		Tags: map[string]string{
			"CONTROLLER": "7",
			"CARDTYPE":   "MINION",
			"ZONE":       "PLAY",
			"ATK":        "3",
			"HEALTH":     "2",
		},
	})

	board := m.State().Board
	if len(board) == 0 {
		t.Fatal("expected 1 minion on board, got 0")
	}

	mn := board[0]
	wantName := CardName(brannCardID)
	if mn.Name != wantName {
		t.Errorf("minion Name = %q, want %q (CardName fallback)", mn.Name, wantName)
	}
	// Ensure Name is not the raw CardID (which would only happen if CardName returned
	// the CardID, meaning it wasn't in the map — acceptable fallback but let's verify
	// we didn't get an empty string).
	if mn.Name == "" {
		t.Errorf("minion Name is empty; expected either friendly name or CardID fallback")
	}
}

// TestProcessorMinionNamePreservedWhenPresent verifies that an EntityName
// provided in the log is preserved and not overwritten by the CardName lookup.
func TestProcessorMinionNamePreservedWhenPresent(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"value": "1"},
	})

	p.Handle(parser.GameEvent{
		Type:       parser.EventEntityUpdate,
		EntityID:   102,
		PlayerID:   7,
		EntityName: "Brann Bronzebeard", // name is already present in the log
		CardID:     "BG_LOE_077",
		Tags: map[string]string{
			"CONTROLLER": "7",
			"CARDTYPE":   "MINION",
			"ZONE":       "PLAY",
			"ATK":        "2",
			"HEALTH":     "4",
		},
	})

	board := m.State().Board
	if len(board) == 0 {
		t.Fatal("expected 1 minion on board, got 0")
	}
	if board[0].Name != "Brann Bronzebeard" {
		t.Errorf("minion Name = %q, want %q", board[0].Name, "Brann Bronzebeard")
	}
}

// TestTurnStartSetsRecruitPhase verifies EventTurnStart with TURN=1 (odd) sets RECRUIT phase.
func TestTurnStartSetsRecruitPhase(t *testing.T) {
	m, p := newProc()
	setupGame(p)
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "1"},
	})
	if m.State().Phase != PhaseRecruit {
		t.Errorf("expected PhaseRecruit after TURN=1 start, got %s", m.State().Phase)
	}
}
