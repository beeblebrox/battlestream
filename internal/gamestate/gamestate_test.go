package gamestate

import (
	"testing"
	"time"

	"battlestream.fixates.io/internal/parser"
)

// ── Machine tests ─────────────────────────────────────────────────────────────

func TestMachineInitialPhase(t *testing.T) {
	m := New()
	if m.State().Phase != PhaseIdle {
		t.Errorf("expected IDLE, got %s", m.State().Phase)
	}
}

func TestMachineGameStart(t *testing.T) {
	m := New()
	m.GameStart("game-1", time.Now())
	s := m.State()
	if s.Phase != PhaseLobby {
		t.Errorf("expected LOBBY, got %s", s.Phase)
	}
	if s.GameID != "game-1" {
		t.Errorf("expected game-1, got %q", s.GameID)
	}
	if s.Player.Health != 40 {
		t.Errorf("expected initial health 40, got %d", s.Player.Health)
	}
}

func TestMachineSetTurnPhases(t *testing.T) {
	m := New()
	m.GameStart("g", time.Now())

	m.SetTurn(1)
	if m.State().Phase != PhaseRecruit {
		t.Errorf("turn 1 should be RECRUIT, got %s", m.State().Phase)
	}

	m.SetTurn(2)
	if m.State().Phase != PhaseCombat {
		t.Errorf("turn 2 should be COMBAT, got %s", m.State().Phase)
	}

	m.SetTurn(7)
	if m.State().Phase != PhaseRecruit {
		t.Errorf("turn 7 should be RECRUIT, got %s", m.State().Phase)
	}
}

func TestMachineGameEnd(t *testing.T) {
	m := New()
	m.GameStart("g", time.Now())
	now := time.Now()
	m.GameEnd(3, now)
	s := m.State()
	if s.Phase != PhaseGameOver {
		t.Errorf("expected GAME_OVER, got %s", s.Phase)
	}
	if s.Placement != 3 {
		t.Errorf("expected placement 3, got %d", s.Placement)
	}
	if s.EndTime == nil || !s.EndTime.Equal(now) {
		t.Errorf("EndTime not set correctly")
	}
}

func TestMachinePlayerTagHealth(t *testing.T) {
	m := New()
	m.GameStart("g", time.Now())
	m.UpdatePlayerTag("HEALTH", "28")
	if m.State().Player.Health != 28 {
		t.Errorf("expected health 28, got %d", m.State().Player.Health)
	}
}

func TestMachinePlayerTagArmor(t *testing.T) {
	m := New()
	m.GameStart("g", time.Now())
	m.UpdatePlayerTag("ARMOR", "5")
	if m.State().Player.Armor != 5 {
		t.Errorf("expected armor 5, got %d", m.State().Player.Armor)
	}
}

func TestMachinePlayerTagSpellPower(t *testing.T) {
	m := New()
	m.GameStart("g", time.Now())
	m.UpdatePlayerTag("SPELL_POWER", "2")
	if m.State().Player.SpellPower != 2 {
		t.Errorf("expected spell power 2, got %d", m.State().Player.SpellPower)
	}
}

func TestMachineTavernTier(t *testing.T) {
	m := New()
	m.GameStart("g", time.Now())
	m.SetTavernTier(4)
	if m.State().TavernTier != 4 {
		t.Errorf("expected tavern tier 4, got %d", m.State().TavernTier)
	}
}

func TestMachineUpsertMinion(t *testing.T) {
	m := New()
	m.GameStart("g", time.Now())

	m.UpsertMinion(MinionState{EntityID: 10, Name: "Murloc", Attack: 2, Health: 1})
	if len(m.State().Board) != 1 {
		t.Fatalf("expected 1 minion, got %d", len(m.State().Board))
	}

	// Update existing
	m.UpsertMinion(MinionState{EntityID: 10, Name: "Murloc", Attack: 4, Health: 3})
	if len(m.State().Board) != 1 {
		t.Fatalf("expected still 1 minion after update, got %d", len(m.State().Board))
	}
	if m.State().Board[0].Attack != 4 {
		t.Errorf("expected attack 4 after update, got %d", m.State().Board[0].Attack)
	}

	// Add second
	m.UpsertMinion(MinionState{EntityID: 11, Name: "Mech", Attack: 3, Health: 3})
	if len(m.State().Board) != 2 {
		t.Fatalf("expected 2 minions, got %d", len(m.State().Board))
	}
}

func TestMachineRemoveMinion(t *testing.T) {
	m := New()
	m.GameStart("g", time.Now())
	m.UpsertMinion(MinionState{EntityID: 10, Name: "A"})
	m.UpsertMinion(MinionState{EntityID: 11, Name: "B"})
	m.RemoveMinion(10)

	board := m.State().Board
	if len(board) != 1 {
		t.Fatalf("expected 1 minion after removal, got %d", len(board))
	}
	if board[0].EntityID != 11 {
		t.Errorf("wrong minion remaining: %d", board[0].EntityID)
	}
}

func TestMachineStatSnapshot(t *testing.T) {
	m := New()
	m.GameStart("g", time.Now())
	m.UpsertMinion(MinionState{EntityID: 1, Name: "X"})

	s1 := m.State()
	// Mutating the snapshot's Board must not affect the machine's state.
	s1.Board[0].Name = "MUTATED"

	s2 := m.State()
	if s2.Board[0].Name == "MUTATED" {
		t.Error("State() snapshot is not a deep copy — board slice was shared")
	}
}

// ── Processor tests ───────────────────────────────────────────────────────────

func proc() (*Machine, *Processor) {
	m := New()
	return m, NewProcessor(m)
}

func TestProcessorGameStartIncrementsID(t *testing.T) {
	m, p := proc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	if m.State().GameID != "game-1" {
		t.Errorf("expected game-1, got %q", m.State().GameID)
	}
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	if m.State().GameID != "game-2" {
		t.Errorf("expected game-2, got %q", m.State().GameID)
	}
}

func TestProcessorTurnUpdatesPhase(t *testing.T) {
	m, p := proc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "3"},
	})
	if m.State().Turn != 3 {
		t.Errorf("expected turn 3, got %d", m.State().Turn)
	}
	if m.State().Phase != PhaseRecruit {
		t.Errorf("expected RECRUIT, got %s", m.State().Phase)
	}
}

func TestProcessorTagChangeHealth(t *testing.T) {
	m, p := proc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange,
		Tags: map[string]string{"HEALTH": "22"},
	})
	if m.State().Player.Health != 22 {
		t.Errorf("expected health 22, got %d", m.State().Player.Health)
	}
}

func TestProcessorTagChangeTavernTier(t *testing.T) {
	m, p := proc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange,
		Tags: map[string]string{"PLAYER_TECH_LEVEL": "4"},
	})
	if m.State().TavernTier != 4 {
		t.Errorf("expected tavern tier 4, got %d", m.State().TavernTier)
	}
}

func TestProcessorEntityUpdateCreatesMinion(t *testing.T) {
	m, p := proc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	p.Handle(parser.GameEvent{
		Type:       parser.EventEntityUpdate,
		EntityID:   42,
		EntityName: "Murloc Tidehunter",
		CardID:     "EX1_506",
		Tags:       map[string]string{"ATK": "2", "HEALTH": "1"},
	})

	board := m.State().Board
	if len(board) != 1 {
		t.Fatalf("expected 1 minion, got %d", len(board))
	}
	if board[0].Attack != 2 || board[0].Health != 1 {
		t.Errorf("unexpected stats: %d/%d", board[0].Attack, board[0].Health)
	}
}

func TestProcessorEntityUpdateNoStatsIgnored(t *testing.T) {
	m, p := proc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 99,
		Tags:     map[string]string{}, // no ATK or HEALTH
	})
	if len(m.State().Board) != 0 {
		t.Error("entity without ATK/HEALTH should not be added to board")
	}
}

func TestProcessorGameEndSetsPlacement(t *testing.T) {
	m, p := proc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	p.Handle(parser.GameEvent{
		Type:      parser.EventGameEnd,
		Timestamp: time.Now(),
		Tags:      map[string]string{"PLAYER_LEADERBOARD_PLACE": "3"},
	})
	s := m.State()
	if s.Phase != PhaseGameOver {
		t.Errorf("expected GAME_OVER, got %s", s.Phase)
	}
	if s.Placement != 3 {
		t.Errorf("expected placement 3, got %d", s.Placement)
	}
}

// ── Full pipeline test ────────────────────────────────────────────────────────

func TestFullPipeline(t *testing.T) {
	// Simulate a short game via the parser → processor → machine pipeline.
	ch := make(chan parser.GameEvent, 64)
	p := parser.New(ch)
	m, proc := proc()

	lines := []string{
		"D 10:00:00.0000000 GameState.DebugPrintPower() - CREATE_GAME",
		"D 10:00:01.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=GameEntity tag=TURN value=1",
		"D 10:00:02.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=Fixates tag=HEALTH value=38",
		"D 10:00:03.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=Fixates tag=ARMOR value=3",
		"D 10:00:04.0000000 GameState.DebugPrintPower() - FULL_ENTITY - Creating Entity=Murloc Tidehunter CardID=EX1_506",
		"D 10:00:05.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=Fixates tag=GAME_RESULT value=WIN",
	}
	for _, l := range lines {
		p.Feed(l)
	}
	close(ch)
	for e := range ch {
		proc.Handle(e)
	}

	s := m.State()
	if s.GameID != "game-1" {
		t.Errorf("GameID: expected game-1, got %q", s.GameID)
	}
	if s.Phase != PhaseGameOver {
		t.Errorf("Phase: expected GAME_OVER, got %s", s.Phase)
	}
	if s.Player.Health != 38 {
		t.Errorf("Health: expected 38, got %d", s.Player.Health)
	}
	if s.Player.Armor != 3 {
		t.Errorf("Armor: expected 3, got %d", s.Player.Armor)
	}
}
