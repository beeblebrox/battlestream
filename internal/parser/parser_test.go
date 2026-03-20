package parser

import (
	"testing"
	"time"
)

// feed runs lines through a Parser, calls Flush, and collects all emitted events.
func feed(lines ...string) []GameEvent {
	ch := make(chan GameEvent, 64)
	p := New(ch)
	for _, l := range lines {
		p.Feed(l)
	}
	p.Flush() // emit any pending FULL_ENTITY block
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
	events := feed("D 10:00:00.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=GameEntity tag=STATE value=COMPLETE")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventGameEnd {
		t.Errorf("expected %s, got %s", EventGameEnd, events[0].Type)
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

// TestFullEntityWithBlockTags verifies that indented tag lines following a
// FULL_ENTITY header are accumulated into the event rather than lost.
func TestFullEntityWithBlockTags(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 GameState.DebugPrintPower() - FULL_ENTITY - Creating Entity=Murloc Tidehunter CardID=EX1_506",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     tag=ATK value=2",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     tag=HEALTH value=1",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     tag=ZONE value=PLAY",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     tag=COST value=2",
	)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventEntityUpdate {
		t.Errorf("expected %s, got %s", EventEntityUpdate, e.Type)
	}
	if e.CardID != "EX1_506" {
		t.Errorf("CardID: expected EX1_506, got %q", e.CardID)
	}
	if e.Tags["ATK"] != "2" {
		t.Errorf("ATK: expected 2, got %q", e.Tags["ATK"])
	}
	if e.Tags["HEALTH"] != "1" {
		t.Errorf("HEALTH: expected 1, got %q", e.Tags["HEALTH"])
	}
	if e.Tags["ZONE"] != "PLAY" {
		t.Errorf("ZONE: expected PLAY, got %q", e.Tags["ZONE"])
	}
}

// TestFullEntityFlushedByNextLine verifies that a non-block line after a
// FULL_ENTITY block causes the accumulated event to be emitted first.
func TestFullEntityFlushedByNextLine(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 GameState.DebugPrintPower() - FULL_ENTITY - Creating Entity=Murloc Tidehunter CardID=EX1_506",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     tag=ATK value=3",
		"D 10:00:01.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=Fixates tag=HEALTH value=28",
	)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != EventEntityUpdate {
		t.Errorf("event[0]: expected %s, got %s", EventEntityUpdate, events[0].Type)
	}
	if events[0].Tags["ATK"] != "3" {
		t.Errorf("ATK: expected 3, got %q", events[0].Tags["ATK"])
	}
	if events[1].Type != EventTagChange {
		t.Errorf("event[1]: expected %s, got %s", EventTagChange, events[1].Type)
	}
}

// TestConsecutiveFullEntities verifies two back-to-back FULL_ENTITY blocks
// both produce events with the correct tags.
func TestConsecutiveFullEntities(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 GameState.DebugPrintPower() - FULL_ENTITY - Creating Entity=Murloc Tidehunter CardID=EX1_506",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     tag=ATK value=2",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     tag=HEALTH value=1",
		"D 10:00:01.0000000 GameState.DebugPrintPower() - FULL_ENTITY - Creating Entity=Annoy-o-Tron CardID=GVG_085",
		"D 10:00:01.0000000 GameState.DebugPrintPower() -     tag=ATK value=1",
		"D 10:00:01.0000000 GameState.DebugPrintPower() -     tag=HEALTH value=2",
	)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].CardID != "EX1_506" || events[0].Tags["ATK"] != "2" {
		t.Errorf("first entity unexpected: %+v", events[0])
	}
	if events[1].CardID != "GVG_085" || events[1].Tags["ATK"] != "1" {
		t.Errorf("second entity unexpected: %+v", events[1])
	}
}

// TestPlayerLeaderboardPlace verifies PLAYER_LEADERBOARD_PLACE comes through
// as a regular TagChange so the processor can track it.
func TestPlayerLeaderboardPlace(t *testing.T) {
	events := feed("D 10:00:00.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=Fixates tag=PLAYER_LEADERBOARD_PLACE value=3")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventTagChange {
		t.Errorf("expected TagChange, got %s", e.Type)
	}
	if e.Tags["PLAYER_LEADERBOARD_PLACE"] != "3" {
		t.Errorf("expected PLAYER_LEADERBOARD_PLACE=3, got %q", e.Tags["PLAYER_LEADERBOARD_PLACE"])
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
	// Lines without the GameState source are now filtered out,
	// so we need to include it.
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

func TestPowerTaskListLinesFiltered(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 PowerTaskList.DebugPrintPower() - CREATE_GAME",
		"D 10:00:00.0000000 PowerTaskList.DebugPrintPower() - TAG_CHANGE Entity=Fixates tag=HEALTH value=28",
		"D 10:00:00.0000000 PowerTaskList.DebugPrintPower() - BLOCK_START BlockType=POWER",
	)
	if len(events) != 0 {
		t.Errorf("expected 0 events from PowerTaskList lines, got %d: %+v", len(events), events)
	}
}

func TestUnrecognisedLineProducesNoEvent(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 GameState.DebugPrintOptions() - id=1",
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
		"D 10:00:03.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=GameEntity tag=STATE value=COMPLETE",
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

// TestPlayerDef verifies parsing of Player entity definitions from CREATE_GAME.
func TestPlayerDef(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     Player EntityID=20 PlayerID=7 GameAccountId=[hi=144115193835963207 lo=30722021]",
	)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventPlayerDef {
		t.Errorf("expected %s, got %s", EventPlayerDef, e.Type)
	}
	if e.EntityID != 20 {
		t.Errorf("expected EntityID 20, got %d", e.EntityID)
	}
	if e.PlayerID != 7 {
		t.Errorf("expected PlayerID 7, got %d", e.PlayerID)
	}
	if e.Tags["hi"] != "144115193835963207" {
		t.Errorf("expected hi=144115193835963207, got %q", e.Tags["hi"])
	}
}

// TestPlayerDefDummy verifies parsing of the dummy player (hi=0 lo=0).
func TestPlayerDefDummy(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     Player EntityID=21 PlayerID=15 GameAccountId=[hi=0 lo=0]",
	)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Tags["hi"] != "0" {
		t.Errorf("expected hi=0 for dummy, got %q", e.Tags["hi"])
	}
	if e.PlayerID != 15 {
		t.Errorf("expected PlayerID 15, got %d", e.PlayerID)
	}
}

// TestPlayerNameParsing verifies extraction of PlayerID → PlayerName mapping.
func TestPlayerNameParsing(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 GameState.DebugPrintGame() - PlayerID=7, PlayerName=Moch#1358",
	)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventPlayerName {
		t.Errorf("expected %s, got %s", EventPlayerName, e.Type)
	}
	if e.PlayerID != 7 {
		t.Errorf("expected PlayerID 7, got %d", e.PlayerID)
	}
	if e.EntityName != "Moch#1358" {
		t.Errorf("expected name Moch#1358, got %q", e.EntityName)
	}
}

// TestPlayerFieldExtraction verifies player= is extracted from bracketed entities.
func TestPlayerFieldExtraction(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=[entityName=Millhouse id=75 zone=PLAY zonePos=0 cardId=TB_BaconShop_HERO_49 player=7] tag=HEALTH value=30",
	)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.PlayerID != 7 {
		t.Errorf("expected PlayerID 7, got %d", e.PlayerID)
	}
	if e.EntityID != 75 {
		t.Errorf("expected EntityID 75, got %d", e.EntityID)
	}
}

// TestFullEntityControllerFromBlockTags verifies CONTROLLER tag in block is
// resolved to PlayerID on the event.
func TestFullEntityControllerFromBlockTags(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 GameState.DebugPrintPower() - FULL_ENTITY - Creating ID=75 CardID=TB_BaconShop_HERO_49",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     tag=CONTROLLER value=7",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     tag=CARDTYPE value=HERO",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     tag=HEALTH value=40",
	)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.PlayerID != 7 {
		t.Errorf("expected PlayerID 7 from CONTROLLER, got %d", e.PlayerID)
	}
	if e.EntityID != 75 {
		t.Errorf("expected EntityID 75, got %d", e.EntityID)
	}
	if e.CardID != "TB_BaconShop_HERO_49" {
		t.Errorf("expected CardID TB_BaconShop_HERO_49, got %q", e.CardID)
	}
}

// TestParserStateResetOnGameStart verifies that stale block state from a
// previous game is silently discarded (not flushed) when CREATE_GAME is seen.
// A game that ended mid-block must not emit phantom EventEntityUpdate events
// before the EventGameStart of the next game.
func TestParserStateResetOnGameStart(t *testing.T) {
	events := feed(
		// Put the parser into inBlock=true with a partial pending entity.
		"D 21:11:50.1234567 GameState.DebugPrintPower() - BLOCK_START BlockType=PLAY Entity=[entityName=SomeCard id=99 zone=PLAY cardId=BG_FAKE_001 player=1] EffectCardId=",
		"D 21:11:50.1234567 GameState.DebugPrintPower() - FULL_ENTITY - Creating ID=99 CardID=BG_FAKE_001",
		// No closing tag lines — simulate a game that ended mid-block.
		// Now a new game starts.
		"D 21:11:50.1234567 GameState.DebugPrintPower() - CREATE_GAME",
	)

	// The partial stale block must be silently discarded — no phantom
	// EventEntityUpdate should appear before (or instead of) EventGameStart.
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 event (EventGameStart), got %d: %+v", len(events), events)
	}
	if events[0].Type != EventGameStart {
		t.Errorf("expected first event to be %s, got %s", EventGameStart, events[0].Type)
	}
}

// TestFullEntityIDFormat verifies the "FULL_ENTITY - Creating ID=N CardID=X" format.
func TestFullEntityIDFormat(t *testing.T) {
	events := feed(
		"D 10:00:00.0000000 GameState.DebugPrintPower() - FULL_ENTITY - Creating ID=37 CardID=TB_BaconShop_HERO_PH",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     tag=CONTROLLER value=7",
	)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EntityID != 37 {
		t.Errorf("expected EntityID 37, got %d", events[0].EntityID)
	}
	if events[0].CardID != "TB_BaconShop_HERO_PH" {
		t.Errorf("expected CardID TB_BaconShop_HERO_PH, got %q", events[0].CardID)
	}
}

func TestCreateGameCapturesGameEntityTags(t *testing.T) {
	events := feed(
		"D 20:04:12.2333830 GameState.DebugPrintPower() - CREATE_GAME",
		"D 20:04:12.2333830 GameState.DebugPrintPower() -     GameEntity EntityID=13",
		"D 20:04:12.2333830 GameState.DebugPrintPower() -         tag=CARDTYPE value=GAME",
		"D 20:04:12.2333830 GameState.DebugPrintPower() -         tag=BACON_DUOS_PUNISH_LEAVERS value=1",
		"D 20:04:12.2333830 GameState.DebugPrintPower() -     Player EntityID=14 PlayerID=5 GameAccountId=[hi=144115193835963207 lo=30722021]",
	)

	// Find EventGameEntityTags
	var found bool
	for _, e := range events {
		if e.Type == EventGameEntityTags {
			if e.Tags["BACON_DUOS_PUNISH_LEAVERS"] != "1" {
				t.Errorf("expected BACON_DUOS_PUNISH_LEAVERS=1, got %q", e.Tags["BACON_DUOS_PUNISH_LEAVERS"])
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no EventGameEntityTags emitted")
	}
}
