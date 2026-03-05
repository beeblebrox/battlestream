package parser

import (
	"testing"
	"time"
)

// feed runs lines through a Parser and collects all emitted events.
func feed(lines ...string) []GameEvent {
	ch := make(chan GameEvent, 64)
	p := New(ch)
	for _, l := range lines {
		p.Feed(l)
	}
	close(ch)
	var events []GameEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func TestGameStart(t *testing.T) {
	events := feed("D 10:00:00.0000000 GameState.DebugPrintPower() - CREATE_GAME")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventGameStart {
		t.Errorf("expected %s, got %s", EventGameStart, events[0].Type)
	}
}

func TestTurnStart(t *testing.T) {
	events := feed("D 10:00:00.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=GameEntity tag=TURN value=7")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventTurnStart {
		t.Errorf("expected %s, got %s", EventTurnStart, events[0].Type)
	}
	if events[0].Tags["TURN"] != "7" {
		t.Errorf("expected TURN=7, got %q", events[0].Tags["TURN"])
	}
}

func TestGameEnd(t *testing.T) {
	events := feed("D 10:00:00.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=Fixates tag=GAME_RESULT value=LOSS")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventGameEnd {
		t.Errorf("expected %s, got %s", EventGameEnd, events[0].Type)
	}
	if events[0].Tags["GAME_RESULT"] != "LOSS" {
		t.Errorf("expected GAME_RESULT=LOSS, got %q", events[0].Tags["GAME_RESULT"])
	}
}

func TestTagChange(t *testing.T) {
	events := feed("D 10:00:00.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=Fixates tag=HEALTH value=28")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventTagChange {
		t.Errorf("expected %s, got %s", EventTagChange, e.Type)
	}
	if e.Tags["HEALTH"] != "28" {
		t.Errorf("expected HEALTH=28, got %q", e.Tags["HEALTH"])
	}
	if e.EntityName != "Fixates" {
		t.Errorf("expected entity name Fixates, got %q", e.EntityName)
	}
}

func TestFullEntity(t *testing.T) {
	events := feed("D 10:00:00.0000000 GameState.DebugPrintPower() - FULL_ENTITY - Creating Entity=Murloc Tidehunter CardID=EX1_506")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventEntityUpdate {
		t.Errorf("expected %s, got %s", EventEntityUpdate, e.Type)
	}
	if e.CardID != "EX1_506" {
		t.Errorf("expected CardID EX1_506, got %q", e.CardID)
	}
}

func TestEntityIDExtraction(t *testing.T) {
	events := feed("D 10:00:00.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=[entityName=Murloc id=42 zone=PLAY] tag=ATK value=3")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EntityID != 42 {
		t.Errorf("expected entity ID 42, got %d", events[0].EntityID)
	}
}

func TestTimestampParsing(t *testing.T) {
	events := feed("D 15:30:45.1234567 GameState.DebugPrintPower() - CREATE_GAME")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ts := events[0].Timestamp
	if ts.Hour() != 15 || ts.Minute() != 30 || ts.Second() != 45 {
		t.Errorf("unexpected timestamp: %v", ts)
	}
}

func TestNoTimestamp(t *testing.T) {
	before := time.Now()
	events := feed("GameState.DebugPrintPower() - CREATE_GAME")
	after := time.Now()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ts := events[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp without prefix should be ~now, got %v", ts)
	}
}

func TestUnrecognisedLineProducesNoEvent(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 GameState.DebugPrintOptions() - id=1",
		"D 10:00:00.0000000 PowerTaskList.DebugPrintPower() - BLOCK_START BlockType=POWER",
		"",
		"just some random text",
	)
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d: %+v", len(events), events)
	}
}

func TestMultipleEvents(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 GameState.DebugPrintPower() - CREATE_GAME",
		"D 10:00:01.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=GameEntity tag=TURN value=1",
		"D 10:00:02.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=Fixates tag=HEALTH value=40",
		"D 10:00:03.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=Fixates tag=GAME_RESULT value=WIN",
	)
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}
	types := []EventType{EventGameStart, EventTurnStart, EventTagChange, EventGameEnd}
	for i, want := range types {
		if events[i].Type != want {
			t.Errorf("event[%d]: expected %s, got %s", i, want, events[i].Type)
		}
	}
}
