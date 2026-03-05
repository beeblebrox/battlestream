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

func TestMachineSetTurn(t *testing.T) {
	m := New()
	m.GameStart("g", time.Now())

	m.SetTurn(1)
	if m.State().Turn != 1 {
		t.Errorf("expected turn 1, got %d", m.State().Turn)
	}
	if m.State().Phase != PhaseRecruit {
		t.Errorf("SetTurn should set RECRUIT, got %s", m.State().Phase)
	}

	m.SetTurn(3)
	if m.State().Turn != 3 {
		t.Errorf("expected turn 3, got %d", m.State().Turn)
	}
}

func TestMachineSetGameEntityTurnPhases(t *testing.T) {
	m := New()
	m.GameStart("g", time.Now())

	m.SetGameEntityTurn(1)
	if m.State().Phase != PhaseRecruit {
		t.Errorf("GameEntity turn 1 should be RECRUIT, got %s", m.State().Phase)
	}

	m.SetGameEntityTurn(2)
	if m.State().Phase != PhaseCombat {
		t.Errorf("GameEntity turn 2 should be COMBAT, got %s", m.State().Phase)
	}

	m.SetGameEntityTurn(7)
	if m.State().Phase != PhaseRecruit {
		t.Errorf("GameEntity turn 7 should be RECRUIT, got %s", m.State().Phase)
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

func newProc() (*Machine, *Processor) {
	m := New()
	return m, NewProcessor(m)
}

// setupGame sends a typical game start sequence to identify the local player.
func setupGame(p *Processor) {
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	// Local player: hi≠0
	p.Handle(parser.GameEvent{
		Type:     parser.EventPlayerDef,
		EntityID: 20,
		PlayerID: 7,
		Tags:     map[string]string{"hi": "144115193835963207", "lo": "30722021", "PLAYER_ID": "7"},
	})
	// Dummy player: hi=0
	p.Handle(parser.GameEvent{
		Type:     parser.EventPlayerDef,
		EntityID: 21,
		PlayerID: 15,
		Tags:     map[string]string{"hi": "0", "lo": "0", "PLAYER_ID": "15"},
	})
	// Player names
	p.Handle(parser.GameEvent{
		Type:       parser.EventPlayerName,
		PlayerID:   7,
		EntityName: "Moch#1358",
	})
	p.Handle(parser.GameEvent{
		Type:       parser.EventPlayerName,
		PlayerID:   15,
		EntityName: "DirePants",
	})
	// Hero entity assignment
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"HERO_ENTITY": "75"},
	})
}

func TestProcessorGameStartIncrementsID(t *testing.T) {
	m, p := newProc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	if m.State().GameID != "game-1" {
		t.Errorf("expected game-1, got %q", m.State().GameID)
	}
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	if m.State().GameID != "game-2" {
		t.Errorf("expected game-2, got %q", m.State().GameID)
	}
}

func TestProcessorLocalPlayerIdentification(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	if m.State().Player.Name != "Moch#1358" {
		t.Errorf("expected player name Moch#1358, got %q", m.State().Player.Name)
	}
}

func TestProcessorTurnFromPlayerEntity(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Player TURN tag (what the user sees)
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"TURN": "3"},
	})
	if m.State().Turn != 3 {
		t.Errorf("expected turn 3, got %d", m.State().Turn)
	}
}

func TestProcessorGameEntityTurnSetsPhase(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// GameEntity turn 1 = recruit
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "1"},
	})
	if m.State().Phase != PhaseRecruit {
		t.Errorf("expected RECRUIT on odd GameEntity turn, got %s", m.State().Phase)
	}

	// GameEntity turn 2 = combat
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "2"},
	})
	if m.State().Phase != PhaseCombat {
		t.Errorf("expected COMBAT on even GameEntity turn, got %s", m.State().Phase)
	}
}

func TestProcessorHealthOnlyFromLocalHero(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Register hero entity 75 as controlled by player 7
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 75,
		PlayerID: 7,
		CardID:   "TB_BaconShop_HERO_49",
		Tags:     map[string]string{"CONTROLLER": "7", "CARDTYPE": "HERO", "HEALTH": "30"},
	})

	// Local hero HEALTH change
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityID:   75,
		PlayerID:   7,
		EntityName: "[entityName=Millhouse id=75 zone=PLAY player=7]",
		Tags:       map[string]string{"HEALTH": "22"},
	})
	if m.State().Player.Health != 22 {
		t.Errorf("expected health 22 from local hero, got %d", m.State().Player.Health)
	}

	// Opponent hero HEALTH change — should NOT update local player.
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityID:   200,
		PlayerID:   15,
		EntityName: "[entityName=Rakanishu id=200 zone=PLAY player=15]",
		Tags:       map[string]string{"HEALTH": "30"},
	})
	if m.State().Player.Health != 22 {
		t.Errorf("opponent health should not affect local player, got %d", m.State().Player.Health)
	}
}

func TestProcessorArmorOnlyFromLocalHero(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Register hero entity
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 75,
		PlayerID: 7,
		CardID:   "TB_BaconShop_HERO_49",
		Tags:     map[string]string{"CONTROLLER": "7", "CARDTYPE": "HERO", "HEALTH": "30", "ARMOR": "10"},
	})

	// Opponent armor — should NOT affect local player.
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityID:   200,
		PlayerID:   15,
		EntityName: "[entityName=Rakanishu id=200 zone=PLAY player=15]",
		Tags:       map[string]string{"ARMOR": "15"},
	})
	if m.State().Player.Armor != 10 {
		t.Errorf("opponent armor should not affect local player, expected 10, got %d", m.State().Player.Armor)
	}
}

func TestProcessorTagChangeTavernTier(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Tavern tier from local player entity
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"PLAYER_TECH_LEVEL": "4"},
	})
	if m.State().TavernTier != 4 {
		t.Errorf("expected tavern tier 4, got %d", m.State().TavernTier)
	}
}

func TestProcessorEntityUpdateCreatesLocalMinion(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Local player's minion (CONTROLLER=7)
	p.Handle(parser.GameEvent{
		Type:       parser.EventEntityUpdate,
		EntityID:   42,
		EntityName: "Murloc Tidehunter",
		CardID:     "EX1_506",
		PlayerID:   7,
		Tags:       map[string]string{"ATK": "2", "HEALTH": "1", "CONTROLLER": "7", "CARDTYPE": "MINION", "ZONE": "PLAY"},
	})

	board := m.State().Board
	if len(board) != 1 {
		t.Fatalf("expected 1 minion, got %d", len(board))
	}
	if board[0].Attack != 2 || board[0].Health != 1 {
		t.Errorf("unexpected stats: %d/%d", board[0].Attack, board[0].Health)
	}
}

func TestProcessorEntityUpdateFiltersOpponentMinion(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Opponent's minion (CONTROLLER=15) should NOT appear on local board
	p.Handle(parser.GameEvent{
		Type:       parser.EventEntityUpdate,
		EntityID:   99,
		EntityName: "Enemy Minion",
		CardID:     "EX1_999",
		PlayerID:   15,
		Tags:       map[string]string{"ATK": "5", "HEALTH": "5", "CONTROLLER": "15", "CARDTYPE": "MINION", "ZONE": "PLAY"},
	})

	if len(m.State().Board) != 0 {
		t.Error("opponent minion should not appear on local board")
	}
}

func TestProcessorEntityUpdateNoStatsIgnored(t *testing.T) {
	m, p := newProc()
	setupGame(p)
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
	m, p := newProc()
	setupGame(p)
	// Placement arrives as TagChange before GameEnd
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		Timestamp:  time.Now(),
		EntityName: "Moch#1358",
		Tags:       map[string]string{"PLAYER_LEADERBOARD_PLACE": "3"},
	})
	p.Handle(parser.GameEvent{
		Type:      parser.EventGameEnd,
		Timestamp: time.Now(),
		Tags:      map[string]string{},
	})
	s := m.State()
	if s.Phase != PhaseGameOver {
		t.Errorf("expected GAME_OVER, got %s", s.Phase)
	}
	if s.Placement != 3 {
		t.Errorf("expected placement 3, got %d", s.Placement)
	}
}

// TestProcessorPlacementFromTagChange verifies that PLAYER_LEADERBOARD_PLACE
// arriving as a TagChange before GAME_RESULT is used as the final placement.
func TestProcessorPlacementFromTagChange(t *testing.T) {
	m, p := newProc()
	setupGame(p)
	// Placement arrives first as a TagChange (real HS log order)
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		Timestamp:  time.Now(),
		EntityName: "Moch#1358",
		Tags:       map[string]string{"PLAYER_LEADERBOARD_PLACE": "5"},
	})
	// GameEnd has no placement in tags (as the real parser emits it)
	p.Handle(parser.GameEvent{
		Type:      parser.EventGameEnd,
		Timestamp: time.Now(),
		Tags:      map[string]string{},
	})
	if m.State().Placement != 5 {
		t.Errorf("expected placement 5 from prior TagChange, got %d", m.State().Placement)
	}
}

// TestProcessorPlacementResetOnNewGame verifies that pendingPlacement does not
// leak from one game into the next.
func TestProcessorPlacementResetOnNewGame(t *testing.T) {
	m, p := newProc()
	// Game 1
	setupGame(p)
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"PLAYER_LEADERBOARD_PLACE": "7"},
	})
	p.Handle(parser.GameEvent{Type: parser.EventGameEnd, Timestamp: time.Now(), Tags: map[string]string{}})
	// Game 2 — no placement event
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	p.Handle(parser.GameEvent{Type: parser.EventGameEnd, Timestamp: time.Now(), Tags: map[string]string{}})
	if m.State().Placement != 0 {
		t.Errorf("placement should be 0 when no PLAYER_LEADERBOARD_PLACE in game 2, got %d", m.State().Placement)
	}
}

// TestProcessorZoneGraveyardRemovesMinion verifies that a TAG_CHANGE with
// ZONE=GRAVEYARD removes the entity from the board.
func TestProcessorZoneGraveyardRemovesMinion(t *testing.T) {
	m, p := newProc()
	setupGame(p)
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 42,
		PlayerID: 7,
		Tags:     map[string]string{"ATK": "3", "HEALTH": "2", "CONTROLLER": "7", "CARDTYPE": "MINION", "ZONE": "PLAY"},
	})
	if len(m.State().Board) != 1 {
		t.Fatalf("expected 1 minion on board before death")
	}
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 42,
		Tags:     map[string]string{"ZONE": "GRAVEYARD"},
	})
	if len(m.State().Board) != 0 {
		t.Errorf("expected board empty after ZONE=GRAVEYARD, got %d minions", len(m.State().Board))
	}
}

// TestProcessorZonePlayDoesNotRemove verifies that ZONE=PLAY does not remove
// minions from the board.
func TestProcessorZonePlayDoesNotRemove(t *testing.T) {
	m, p := newProc()
	setupGame(p)
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 10,
		PlayerID: 7,
		Tags:     map[string]string{"ATK": "2", "HEALTH": "1", "CONTROLLER": "7", "CARDTYPE": "MINION", "ZONE": "PLAY"},
	})
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 10,
		Tags:     map[string]string{"ZONE": "PLAY"},
	})
	if len(m.State().Board) != 1 {
		t.Errorf("expected 1 minion after ZONE=PLAY, got %d", len(m.State().Board))
	}
}

// ── Full pipeline test ────────────────────────────────────────────────────────

func TestFullPipeline(t *testing.T) {
	// Simulate a BG game via the parser → processor → machine pipeline.
	ch := make(chan parser.GameEvent, 64)
	p := parser.New(ch)
	m, proc := newProc()

	lines := []string{
		// Game start
		"D 10:00:00.0000000 GameState.DebugPrintPower() - CREATE_GAME",
		// Player definitions
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     Player EntityID=20 PlayerID=7 GameAccountId=[hi=144115193835963207 lo=30722021]",
		"D 10:00:00.0000000 GameState.DebugPrintPower() -     Player EntityID=21 PlayerID=15 GameAccountId=[hi=0 lo=0]",
		// Player names
		"D 10:00:00.0000000 GameState.DebugPrintGame() - PlayerID=7, PlayerName=Moch#1358",
		"D 10:00:00.0000000 GameState.DebugPrintGame() - PlayerID=15, PlayerName=DirePants",
		// Hero entity created for local player
		"D 10:00:01.0000000 GameState.DebugPrintPower() - FULL_ENTITY - Creating ID=75 CardID=TB_BaconShop_HERO_49",
		"D 10:00:01.0000000 GameState.DebugPrintPower() -     tag=CONTROLLER value=7",
		"D 10:00:01.0000000 GameState.DebugPrintPower() -     tag=CARDTYPE value=HERO",
		"D 10:00:01.0000000 GameState.DebugPrintPower() -     tag=HEALTH value=30",
		"D 10:00:01.0000000 GameState.DebugPrintPower() -     tag=ARMOR value=10",
		// Hero assignment
		"D 10:00:01.5000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=Moch#1358 tag=HERO_ENTITY value=75",
		// GameEntity turn 1 (recruit)
		"D 10:00:02.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=GameEntity tag=TURN value=1",
		// Player turn 1
		"D 10:00:02.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=Moch#1358 tag=TURN value=1",
		// Local hero health update
		"D 10:00:03.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=[entityName=Millhouse id=75 zone=PLAY zonePos=0 cardId=TB_BaconShop_HERO_49 player=7] tag=HEALTH value=28",
		// Opponent hero health — should NOT affect local player
		"D 10:00:03.5000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=[entityName=Rakanishu id=200 zone=PLAY zonePos=0 cardId=TB_BaconShop_HERO_75 player=15] tag=HEALTH value=35",
		// PowerTaskList line — should be FILTERED OUT
		"D 10:00:03.7000000 PowerTaskList.DebugPrintPower() - TAG_CHANGE Entity=[entityName=Millhouse id=75 zone=PLAY player=7] tag=HEALTH value=99",
		// Game end
		"D 10:00:04.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=Moch#1358 tag=PLAYER_LEADERBOARD_PLACE value=3",
		"D 10:00:05.0000000 GameState.DebugPrintPower() - TAG_CHANGE Entity=GameEntity tag=STATE value=COMPLETE",
	}
	for _, l := range lines {
		p.Feed(l)
	}
	p.Flush()
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
	if s.Player.Name != "Moch#1358" {
		t.Errorf("Name: expected Moch#1358, got %q", s.Player.Name)
	}
	if s.Player.Health != 28 {
		t.Errorf("Health: expected 28 (not 35 from opponent, not 99 from PowerTaskList), got %d", s.Player.Health)
	}
	if s.Player.Armor != 10 {
		t.Errorf("Armor: expected 10, got %d", s.Player.Armor)
	}
	if s.Turn != 1 {
		t.Errorf("Turn: expected 1 (player turn, not GameEntity turn), got %d", s.Turn)
	}
	if s.Placement != 3 {
		t.Errorf("Placement: expected 3, got %d", s.Placement)
	}
}
