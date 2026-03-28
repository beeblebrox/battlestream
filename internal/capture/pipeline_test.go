package capture

import (
	"testing"
	"time"

	"battlestream.fixates.io/internal/parser"
)

func TestStateTrackerSnapshot(t *testing.T) {
	tracker := NewStateTracker()

	if tracker.InGame() {
		t.Fatal("should not be in game initially")
	}

	// Simulate game start (sets phase to LOBBY).
	tracker.Apply(parser.GameEvent{
		Type:      parser.EventGameStart,
		Timestamp: time.Now(),
	})

	// LOBBY is not considered "in game" — need a turn start for RECRUIT.
	if tracker.InGame() {
		t.Fatal("should not be in game during LOBBY phase")
	}

	// Local player definition (hi != 0 means local).
	tracker.Apply(parser.GameEvent{
		Type:     parser.EventPlayerDef,
		EntityID: 20,
		PlayerID: 7,
		Tags:     map[string]string{"hi": "144115193835963207", "lo": "30722021", "PLAYER_ID": "7"},
	})

	// Dummy bot player.
	tracker.Apply(parser.GameEvent{
		Type:     parser.EventPlayerDef,
		EntityID: 21,
		PlayerID: 15,
		Tags:     map[string]string{"hi": "0", "lo": "0", "PLAYER_ID": "15"},
	})

	// Turn start moves to RECRUIT phase (odd GameEntity turn = recruit).
	tracker.Apply(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "1"},
	})

	if !tracker.InGame() {
		t.Fatal("should be in game after turn start")
	}

	snap := tracker.Snapshot()
	if snap.Phase == "" {
		t.Error("phase should not be empty during game")
	}
	if snap.Phase != "RECRUIT" {
		t.Errorf("expected phase RECRUIT, got %s", snap.Phase)
	}
	if snap.GameID == "" {
		t.Error("game ID should not be empty")
	}
	if snap.Timestamp.IsZero() {
		t.Error("timestamp should not be zero")
	}
}

func TestStateTrackerInGamePhases(t *testing.T) {
	tracker := NewStateTracker()

	// Initially idle.
	if tracker.InGame() {
		t.Fatal("should not be in game when idle")
	}

	// Start game -> LOBBY (not in game).
	tracker.Apply(parser.GameEvent{
		Type:      parser.EventGameStart,
		Timestamp: time.Now(),
	})
	if tracker.InGame() {
		t.Fatal("LOBBY should not count as in game")
	}

	// Player defs required before turn processing works.
	tracker.Apply(parser.GameEvent{
		Type:     parser.EventPlayerDef,
		EntityID: 20,
		PlayerID: 7,
		Tags:     map[string]string{"hi": "144115193835963207", "lo": "30722021", "PLAYER_ID": "7"},
	})
	tracker.Apply(parser.GameEvent{
		Type:     parser.EventPlayerDef,
		EntityID: 21,
		PlayerID: 15,
		Tags:     map[string]string{"hi": "0", "lo": "0", "PLAYER_ID": "15"},
	})

	// Turn start -> RECRUIT (in game).
	tracker.Apply(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "1"},
	})
	if !tracker.InGame() {
		t.Fatal("RECRUIT should be in game")
	}
}
