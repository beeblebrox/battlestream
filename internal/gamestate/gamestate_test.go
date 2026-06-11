package gamestate

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"battlestream.fixates.io/internal/gamestate/action"
	"battlestream.fixates.io/internal/gamestate/builder"
	"battlestream.fixates.io/internal/gamestate/card"
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
	if s.Player.Health != 0 {
		t.Errorf("expected initial health 0 (set from hero HEALTH tag), got %d", s.Player.Health)
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

func TestMachineUpsertMinionPreservesEnchantments(t *testing.T) {
	m := New()

	m.UpsertMinion(MinionState{EntityID: 10, Name: "Murloc", Attack: 2, Health: 1})
	m.AddEnchantment(Enchantment{
		EntityID: 900, CardID: "BG_ShopBuff_Elemental", TargetID: 10,
		AttackBuff: 2, HealthBuff: 2,
	})

	board := m.State().Board
	if len(board) != 1 || len(board[0].Enchantments) != 1 {
		t.Fatalf("precondition failed: expected 1 minion with 1 enchantment, got %+v", board)
	}

	// Re-upsert the same entity without enchantments (golden upgrade /
	// repeated ZONE=PLAY re-registration) — the enchantment must survive.
	m.UpsertMinion(MinionState{EntityID: 10, Name: "Murloc", Attack: 4, Health: 3})

	board = m.State().Board
	if len(board) != 1 {
		t.Fatalf("expected 1 minion, got %d", len(board))
	}
	if board[0].Attack != 4 || board[0].Health != 3 {
		t.Errorf("expected stats updated to 4/3, got %d/%d", board[0].Attack, board[0].Health)
	}
	if len(board[0].Enchantments) != 1 {
		t.Fatalf("expected enchantment to survive re-upsert, got %d enchantments", len(board[0].Enchantments))
	}
	if board[0].Enchantments[0].EntityID != 900 {
		t.Errorf("expected enchantment entity 900, got %d", board[0].Enchantments[0].EntityID)
	}

	// An incoming minion that carries its own enchantments still replaces wholesale.
	m.UpsertMinion(MinionState{
		EntityID: 10, Name: "Murloc", Attack: 4, Health: 3,
		Enchantments: []Enchantment{{EntityID: 901, TargetID: 10}},
	})
	board = m.State().Board
	if len(board[0].Enchantments) != 1 || board[0].Enchantments[0].EntityID != 901 {
		t.Errorf("expected incoming enchantments to take precedence, got %+v", board[0].Enchantments)
	}
}

func TestMachineUpsertMinionMaxSeven(t *testing.T) {
	m := New()
	m.GameStart("g", time.Now())
	m.SetPhase(PhaseRecruit)

	for i := 0; i < 10; i++ {
		m.UpsertMinion(MinionState{EntityID: 100 + i, Name: fmt.Sprintf("M%d", i), Attack: 1, Health: 1})
	}
	s := m.State()
	if len(s.Board) > 7 {
		t.Errorf("expected max 7 board minions, got %d", len(s.Board))
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
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"HERO_ENTITY": "75"},
	})
}

func TestEntityRegistryPrunedAfterCombat(t *testing.T) {
	_, p := newProc()
	setupGame(p)

	// Simulate combat entities in dead zones
	for i := 500; i < 600; i++ {
		p.entityProps[i] = &entityInfo{CardType: "MINION", Zone: "REMOVEDFROMGAME"}
		p.entityController[i] = 15
	}
	for i := 600; i < 650; i++ {
		p.entityProps[i] = &entityInfo{CardType: "MINION", Zone: "GRAVEYARD"}
		p.entityController[i] = 15
	}
	// Keep some alive entities
	p.entityProps[999] = &entityInfo{CardType: "MINION", Zone: "PLAY"}
	p.entityController[999] = 7

	sizeBefore := len(p.entityProps)
	p.pruneStaleEntities()
	sizeAfter := len(p.entityProps)

	if sizeAfter >= sizeBefore {
		t.Errorf("expected pruning to reduce entity count, before=%d after=%d", sizeBefore, sizeAfter)
	}
	// The 150 dead entities should be gone
	if _, ok := p.entityProps[500]; ok {
		t.Error("REMOVEDFROMGAME entity should have been pruned")
	}
	if _, ok := p.entityProps[600]; ok {
		t.Error("GRAVEYARD entity should have been pruned")
	}
	// Active entity should remain
	if _, ok := p.entityProps[999]; !ok {
		t.Error("PLAY entity should not have been pruned")
	}
}

func TestProcessorGameStartIncrementsID(t *testing.T) {
	// Zero timestamps fall back to sequential game-<n> IDs (used when no
	// log timestamp is available, e.g. synthetic events in tests).
	m, p := newProc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Time{}})
	if m.State().GameID != "game-1" {
		t.Errorf("expected game-1, got %q", m.State().GameID)
	}
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Time{}})
	if m.State().GameID != "game-2" {
		t.Errorf("expected game-2, got %q", m.State().GameID)
	}
}

func TestProcessorGameStartTimestampID(t *testing.T) {
	// Non-zero timestamps produce a stable game-<unixmilli> ID so that
	// IDs survive daemon restarts and reparsing.
	m, p := newProc()
	ts := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: ts})
	want := fmt.Sprintf("game-%d", ts.UnixMilli())
	if m.State().GameID != want {
		t.Errorf("expected %q, got %q", want, m.State().GameID)
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
		PlayerID:   7,
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
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"PLAYER_TECH_LEVEL": "4"},
	})
	if m.State().TavernTier != 4 {
		t.Errorf("expected tavern tier 4, got %d", m.State().TavernTier)
	}
}

// TestTavernTierAttribution verifies the stricter guard on PLAYER_TECH_LEVEL /
// TAVERN_TIER: when localPlayerID is 0 (not yet set), a TAG_CHANGE with
// controllerID=0 must NOT set the tavern tier (the old code would false-positive
// because 0==0). After local player identification, only the matching controller
// may update the tier.
func TestTavernTierAttribution(t *testing.T) {
	m, p := newProc()
	// Start a game but do NOT call setupGame — localPlayerID stays 0.
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})

	// Sanity: localPlayerID must still be 0 at this point.
	if p.localPlayerID != 0 {
		t.Fatalf("expected localPlayerID=0 before player def, got %d", p.localPlayerID)
	}

	// A tag-change with controllerID=0 (unknown entity) and localPlayerID=0.
	// This should NOT set the tavern tier — the old guard would have matched 0==0.
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		PlayerID: 0, // controllerID resolves to 0
		EntityID: 99,
		Tags:     map[string]string{"PLAYER_TECH_LEVEL": "3"},
	})
	if tier := m.State().TavernTier; tier != 0 {
		t.Errorf("tier should not be set when localPlayerID=0 and controllerID=0, got %d", tier)
	}

	// Now identify the local player as PlayerID=7.
	p.Handle(parser.GameEvent{
		Type:     parser.EventPlayerDef,
		EntityID: 20,
		PlayerID: 7,
		Tags:     map[string]string{"hi": "144115193835963207", "lo": "30722021", "PLAYER_ID": "7"},
	})

	// Tag-change from the local player (controllerID=7) — must set tier.
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		PlayerID: 7,
		EntityID: 20,
		Tags:     map[string]string{"PLAYER_TECH_LEVEL": "3"},
	})
	if tier := m.State().TavernTier; tier != 3 {
		t.Errorf("expected tier 3 after local player tag-change, got %d", tier)
	}

	// Tag-change from the opponent (controllerID=5) — must NOT change tier.
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		PlayerID: 5,
		EntityID: 55,
		Tags:     map[string]string{"PLAYER_TECH_LEVEL": "5"},
	})
	if tier := m.State().TavernTier; tier != 3 {
		t.Errorf("opponent tier change should not affect local tier; expected 3, got %d", tier)
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
		PlayerID:   7,
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
		PlayerID:   7,
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
		PlayerID:   7,
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
	// GameID is timestamp-based (game-<unixmilli>) when a real log timestamp
	// is available; just verify it is non-empty and has the right prefix.
	if !strings.HasPrefix(s.GameID, "game-") {
		t.Errorf("GameID: expected prefix %q, got %q", "game-", s.GameID)
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

// ── Integration test ──────────────────────────────────────────────────────────

func TestIntegrationPowerLog(t *testing.T) {
	f, err := os.Open("testdata/power_log_game.txt")
	if err != nil {
		t.Skipf("test fixture not available: %v", err)
	}
	defer f.Close()

	ch := make(chan parser.GameEvent, 256)
	p := parser.New(ch)
	m := New()
	proc := NewProcessor(m)

	// Drain events concurrently to avoid channel deadlock.
	done := make(chan struct{})
	go func() {
		for e := range ch {
			proc.Handle(e)
		}
		close(done)
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		p.Feed(scanner.Text())
	}
	p.Flush()
	close(ch)
	<-done

	s := m.State()

	// Basic game state
	if s.Phase != PhaseGameOver {
		t.Errorf("Phase: expected GAME_OVER, got %s", s.Phase)
	}
	if s.Player.Name != "Moch#1358" {
		t.Errorf("Player.Name: expected Moch#1358, got %q", s.Player.Name)
	}
	if s.Placement != 1 {
		t.Errorf("Placement: expected 1, got %d", s.Placement)
	}
	if s.TavernTier != 5 {
		t.Errorf("TavernTier: expected 5, got %d", s.TavernTier)
	}
	if s.Player.TripleCount != 2 {
		t.Errorf("TripleCount: expected 2, got %d", s.Player.TripleCount)
	}

	// Board minions — 6 expected with exact stats
	type expectedMinion struct {
		Name   string
		Attack int
		Health int
	}
	want := []expectedMinion{
		{"Wildfire Elemental", 233, 181},
		{"Unleashed Mana Surge", 85, 64},
		{"Flaming Enforcer", 1022, 817},
		{"Timewarped Nomi", 22, 20},
		{"Acid Rainfall", 884, 724},
		{"Brann Bronzebeard", 4, 7},
	}

	if len(s.Board) != len(want) {
		for i, mn := range s.Board {
			t.Logf("  Board[%d]: %q (id=%d) %d/%d", i, mn.Name, mn.EntityID, mn.Attack, mn.Health)
		}
		t.Fatalf("Board: expected %d minions, got %d", len(want), len(s.Board))
	}

	// Build a lookup by name for flexible ordering
	boardByName := make(map[string]MinionState)
	for _, mn := range s.Board {
		boardByName[mn.Name] = mn
	}

	for _, w := range want {
		mn, ok := boardByName[w.Name]
		if !ok {
			t.Errorf("Board: missing minion %q", w.Name)
			continue
		}
		if mn.Attack != w.Attack || mn.Health != w.Health {
			t.Errorf("Board %q: expected %d/%d, got %d/%d",
				w.Name, w.Attack, w.Health, mn.Attack, mn.Health)
		}
	}

	// Anomaly detection — the test fixture has BG34_Anomaly_800 (Major Goldthorn Potion).
	if s.AnomalyCardID != "BG34_Anomaly_800" {
		t.Errorf("AnomalyCardID: expected BG34_Anomaly_800, got %q", s.AnomalyCardID)
	}
	if s.AnomalyName != "Major Goldthorn Potion" {
		t.Errorf("AnomalyName: expected Major Goldthorn Potion, got %q", s.AnomalyName)
	}
	if s.AnomalyDescription == "" {
		t.Error("AnomalyDescription should not be empty for BG34_Anomaly_800")
	}

	// Modifications should be board-wide only (Target starts with "Board")
	for _, mod := range s.Modifications {
		if !strings.HasPrefix(mod.Target, "Board") {
			t.Errorf("Modification should be board-wide, got Target=%q (turn %d, %s %+d)",
				mod.Target, mod.Turn, mod.Stat, mod.Delta)
		}
	}

	// BuffSources and Enchantments should be populated from the test fixture.
	// The fixture has TAVERN_SPELL_* tags and ENCHANTMENT entities.
	if len(s.Enchantments) == 0 {
		t.Log("Note: no enchantments tracked (may need local player minion attachments in fixture)")
	}
	// Log buff sources for visibility
	for _, bs := range s.BuffSources {
		t.Logf("BuffSource: %s +%d/+%d", bs.Category, bs.Attack, bs.Health)
	}

	// Duos detection — the test fixture is a Duos game.
	if !s.IsDuos {
		t.Errorf("IsDuos: expected true (test fixture is a Duos game)")
	}
	if s.Partner == nil {
		t.Fatal("Partner: expected non-nil in Duos game")
	}
	if s.Partner.Name == "" {
		t.Log("Note: partner name not resolved (may arrive after CREATE_GAME)")
	} else {
		t.Logf("Partner: %s", s.Partner.Name)
	}
}

// ── Enchantment/BuffSource tests ─────────────────────────────────────────────

func TestProcessorEnchantmentTracking(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Add a minion to the board
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 100,
		PlayerID: 7,
		CardID:   "BG_MINION_001",
		Tags: map[string]string{
			"ATK": "5", "HEALTH": "5", "CONTROLLER": "7",
			"CARDTYPE": "MINION", "ZONE": "PLAY",
		},
		EntityName: "Test Minion",
	})

	if len(m.State().Board) != 1 {
		t.Fatalf("expected 1 minion on board, got %d", len(m.State().Board))
	}

	// Create an enchantment attached to the minion with CREATOR pointing to a source
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 200,
		CardID:   "BG20_303e", // Nomi enchantment (should match ElementalShopBuff pattern if we add it)
		Tags: map[string]string{
			"CARDTYPE":              "ENCHANTMENT",
			"ATTACHED":             "100",
			"CREATOR":             "50",
			"TAG_SCRIPT_DATA_NUM_1": "10",
			"TAG_SCRIPT_DATA_NUM_2": "8",
			"CONTROLLER":           "7",
		},
	})

	s := m.State()
	if len(s.Enchantments) != 1 {
		t.Fatalf("expected 1 enchantment, got %d", len(s.Enchantments))
	}
	ench := s.Enchantments[0]
	if ench.TargetID != 100 {
		t.Errorf("enchantment target: expected 100, got %d", ench.TargetID)
	}
	if ench.AttackBuff != 10 || ench.HealthBuff != 8 {
		t.Errorf("enchantment buffs: expected +10/+8, got +%d/+%d", ench.AttackBuff, ench.HealthBuff)
	}

	// Check per-minion enchantment
	board := s.Board
	if len(board[0].Enchantments) != 1 {
		t.Errorf("expected 1 enchantment on minion, got %d", len(board[0].Enchantments))
	}

	// Per-minion enchantments should NOT create BuffSources (only Dnt enchantments do).
	for _, bs := range s.BuffSources {
		if bs.Category == CatGeneral && (bs.Attack != 0 || bs.Health != 0) {
			t.Errorf("per-minion enchantment should not create GENERAL buff source, got +%d/+%d", bs.Attack, bs.Health)
		}
	}
}

func TestProcessorBuffSourceFromPlayerTag(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Simulate TAVERN_SPELL_ATTACK_INCREASE on local player
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"TAVERN_SPELL_ATTACK_INCREASE": "3"},
	})
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"TAVERN_SPELL_HEALTH_INCREASE": "2"},
	})

	s := m.State()
	var found bool
	for _, bs := range s.BuffSources {
		if bs.Category == CatTavernSpell {
			found = true
			if bs.Attack != 3 || bs.Health != 2 {
				t.Errorf("tavern spell buff: expected +3/+2, got +%d/+%d", bs.Attack, bs.Health)
			}
		}
	}
	if !found {
		t.Error("expected TAVERN_SPELL buff source, not found")
	}
}

func TestProcessorBloodgemValueComputation(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Bloodgem ATK value 0 → effective +1, value 2 → effective +3
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"BACON_BLOODGEMBUFFATKVALUE": "2"},
	})
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"BACON_BLOODGEMBUFFHEALTHVALUE": "1"},
	})

	s := m.State()
	for _, bs := range s.BuffSources {
		if bs.Category == CatBloodgem {
			if bs.Attack != 3 {
				t.Errorf("bloodgem ATK: expected 3 (raw 2 + 1), got %d", bs.Attack)
			}
			if bs.Health != 2 {
				t.Errorf("bloodgem HP: expected 2 (raw 1 + 1), got %d", bs.Health)
			}
			return
		}
	}
	t.Error("expected BLOODGEM buff source, not found")
}

// TestBloodgemCombatSaveRestoreIgnored verifies that the BG29_813e combat save/restore
// mechanism does not corrupt the bloodgem display. During combat, the game zeros the
// BACON_BLOODGEM* tags (save) and later restores them via PowerTaskList (filtered).
// Without the fix, ComputeBloodgemValue(0)=1 would leave a false "+1/+1" in the state.
func TestBloodgemCombatSaveRestoreIgnored(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Accumulate real bloodgem buffs during recruit: ATK raw=2 → effective 3, HP raw=4 → effective 5.
	bloodgemTag := func(tag, val string) {
		p.Handle(parser.GameEvent{
			Type:       parser.EventTagChange,
			PlayerID:   7,
			EntityName: "Moch#1358",
			Tags:       map[string]string{tag: val},
		})
	}
	bloodgemTag("BACON_BLOODGEMBUFFATKVALUE", "2")
	bloodgemTag("BACON_BLOODGEMBUFFHEALTHVALUE", "4")

	// Advance to combat (GameEntity TURN=2 → even = combat).
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "2"},
	})
	if m.State().Phase != PhaseCombat {
		t.Fatalf("expected COMBAT phase after TURN=2, got %s", m.State().Phase)
	}

	// Simulate BG29_813e save/restore: game fires real value then zeros both tags.
	bloodgemTag("BACON_BLOODGEMBUFFATKVALUE", "2")
	bloodgemTag("BACON_BLOODGEMBUFFATKVALUE", "0")
	bloodgemTag("BACON_BLOODGEMBUFFHEALTHVALUE", "0")

	// PowerTaskList restoration (filtered out by parser) never reaches us.
	// Display must show the recruit-phase values, not the spurious ComputeBloodgemValue(0)=1.
	s := m.State()
	for _, bs := range s.BuffSources {
		if bs.Category == CatBloodgem {
			if bs.Attack != 3 {
				t.Errorf("bloodgem ATK: expected 3 (recruit value), got %d (combat zero corrupted it)", bs.Attack)
			}
			if bs.Health != 5 {
				t.Errorf("bloodgem HP: expected 5 (recruit value), got %d (combat zero corrupted it)", bs.Health)
			}
			return
		}
	}
	t.Error("expected BLOODGEM buff source, not found")
}

func TestProcessorEnchantmentCleanupOnDeath(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Add minion
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 100,
		PlayerID: 7,
		CardID:   "BG_MINION_001",
		Tags: map[string]string{
			"ATK": "5", "HEALTH": "5", "CONTROLLER": "7",
			"CARDTYPE": "MINION", "ZONE": "PLAY",
		},
	})

	// Add enchantment
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 200,
		CardID:   "TEST_ENCHANT",
		Tags: map[string]string{
			"CARDTYPE":              "ENCHANTMENT",
			"ATTACHED":             "100",
			"CREATOR":             "50",
			"TAG_SCRIPT_DATA_NUM_1": "5",
			"CONTROLLER":           "7",
		},
	})

	if len(m.State().Enchantments) != 1 {
		t.Fatalf("expected 1 enchantment, got %d", len(m.State().Enchantments))
	}

	// Kill the minion
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 100,
		Tags:     map[string]string{"ZONE": "GRAVEYARD"},
	})

	if len(m.State().Enchantments) != 0 {
		t.Errorf("expected 0 enchantments after minion death, got %d", len(m.State().Enchantments))
	}

	// Per-minion enchantments don't create BuffSources, so nothing to check for zeroing.
}

func TestCategoryClassification(t *testing.T) {
	tests := []struct {
		cardID   string
		expected string
	}{
		{"BG_ShopBuff_Elemental", CatNomi},
		{"BG34_402pe", CatWhelp},
		{"BG25_011pe", CatUndead},
		{"BG34_170e", CatVolumizer},
		{"BG34_854pe", CatRightmost},
		{"BG31_808pe", CatBeetle},
		{"BG34_689e2", CatBloodgemBarrage},
		{"BG30_MagicItem_544pe", CatNomi},
		{"UNKNOWN_CARD_ID", CatGeneral},
	}
	for _, tt := range tests {
		got := ClassifyEnchantment(tt.cardID)
		if got != tt.expected {
			t.Errorf("ClassifyEnchantment(%q): expected %q, got %q", tt.cardID, tt.expected, got)
		}
	}
}

func TestBloodgemValueComputation(t *testing.T) {
	if v := ComputeBloodgemValue(0); v != 1 {
		t.Errorf("BloodGem(0): expected 1, got %d", v)
	}
	if v := ComputeBloodgemValue(2); v != 3 {
		t.Errorf("BloodGem(2): expected 3, got %d", v)
	}
	if v := ComputeBloodgemValue(-2); v != 1 {
		t.Errorf("BloodGem(-2): expected 1, got %d", v)
	}
}

// ── Counter-based BuffSource tests (HDT-style) ──────────────────────────────

// setupDntEntity creates a Dnt enchantment entity in the entity registry
// with the given CardID and initial SD values, controlled by the local player.
func setupDntEntity(p *Processor, entityID int, cardID string, sd1, sd2 int) {
	p.entityController[entityID] = 7
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: entityID,
		CardID:   cardID,
		Tags: map[string]string{
			"CARDTYPE":              "ENCHANTMENT",
			"ATTACHED":              "20",
			"CREATOR":               "50",
			"TAG_SCRIPT_DATA_NUM_1": fmt.Sprintf("%d", sd1),
			"TAG_SCRIPT_DATA_NUM_2": fmt.Sprintf("%d", sd2),
			"CONTROLLER":            "7",
		},
	})
}

// sendSD sends a TAG_CHANGE for TAG_SCRIPT_DATA_NUM_1 or NUM_2 on an entity.
func sendSD(p *Processor, entityID int, num int, value int) {
	tag := "TAG_SCRIPT_DATA_NUM_1"
	if num == 2 {
		tag = "TAG_SCRIPT_DATA_NUM_2"
	}
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: entityID,
		Tags:     map[string]string{tag: fmt.Sprintf("%d", value)},
	})
}

func TestCounterNomiShopBuff(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Create Dnt with initial SD1=0, SD2=0 (no buffs yet).
	setupDntEntity(p, 300, "BG_ShopBuff_Elemental", 0, 0)

	// SD1: 0→2→5 (differential: +2, +3 = total 5)
	sendSD(p, 300, 1, 2)
	sendSD(p, 300, 1, 5)
	// SD2: 0→2→5 (differential: +2, +3 = total 5)
	sendSD(p, 300, 2, 2)
	sendSD(p, 300, 2, 5)

	found := findBuffSource(m, CatNomi)
	if found == nil {
		t.Fatal("expected NOMI buff source, not found")
	}
	if found.Attack != 5 || found.Health != 5 {
		t.Errorf("Nomi ShopBuff: expected +5/+5, got +%d/+%d", found.Attack, found.Health)
	}
}

func TestCounterNomiStickerDifferential(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Nomi Sticker: SD1 applies to BOTH atk and hp.
	setupDntEntity(p, 301, "BG30_MagicItem_544pe", 0, 0)

	// SD1: 0→3→7 (differential: +3, +4 = total 7) → applies to both atk AND hp
	sendSD(p, 301, 1, 3)
	sendSD(p, 301, 1, 7)

	found := findBuffSource(m, CatNomi)
	if found == nil {
		t.Fatal("expected NOMI buff source from sticker, not found")
	}
	if found.Attack != 7 || found.Health != 7 {
		t.Errorf("Nomi Sticker: expected +7/+7, got +%d/+%d", found.Attack, found.Health)
	}
}

func TestCounterNomiCombined(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// ShopBuff SD1=10, SD2=10
	setupDntEntity(p, 300, "BG_ShopBuff_Elemental", 0, 0)
	sendSD(p, 300, 1, 10)
	sendSD(p, 300, 2, 10)

	// Sticker SD1=5 (applies to both)
	setupDntEntity(p, 301, "BG30_MagicItem_544pe", 0, 0)
	sendSD(p, 301, 1, 5)

	found := findBuffSource(m, CatNomi)
	if found == nil {
		t.Fatal("expected NOMI buff source, not found")
	}
	// ShopBuff +10/+10 + Sticker +5/+5 = +15/+15
	if found.Attack != 15 || found.Health != 15 {
		t.Errorf("Nomi combined: expected +15/+15, got +%d/+%d", found.Attack, found.Health)
	}
}

func TestCounterNomiAllDifferential(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Timewarped Nomi (Kitchen Dream): differential accumulation, same as regular Nomi.
	setupDntEntity(p, 310, "BG34_855pe", 0, 0)

	// SD1: 0→3→8 (differential: +3, +5 = total 8)
	sendSD(p, 310, 1, 3)
	sendSD(p, 310, 1, 8)
	// SD2: 0→2→6 (differential: +2, +4 = total 6)
	sendSD(p, 310, 2, 2)
	sendSD(p, 310, 2, 6)

	found := findBuffSource(m, CatNomiAll)
	if found == nil {
		t.Fatal("expected NOMI_ALL buff source, not found")
	}
	if found.Attack != 8 || found.Health != 6 {
		t.Errorf("Nomi All: expected +8/+6, got +%d/+%d", found.Attack, found.Health)
	}

	// Regular Nomi should be unaffected.
	nomi := findBuffSource(m, CatNomi)
	if nomi != nil && (nomi.Attack != 0 || nomi.Health != 0) {
		t.Errorf("regular Nomi should be 0/0, got +%d/+%d", nomi.Attack, nomi.Health)
	}
}

func TestCounterNomiAllSeparateFromNomi(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Both regular Nomi and Timewarped Nomi active simultaneously.
	setupDntEntity(p, 300, "BG_ShopBuff_Elemental", 0, 0)
	setupDntEntity(p, 310, "BG34_855pe", 0, 0)

	// Regular Nomi: +5/+5
	sendSD(p, 300, 1, 5)
	sendSD(p, 300, 2, 5)

	// Timewarped Nomi: +10/+10
	sendSD(p, 310, 1, 10)
	sendSD(p, 310, 2, 10)

	nomi := findBuffSource(m, CatNomi)
	if nomi == nil || nomi.Attack != 5 || nomi.Health != 5 {
		t.Errorf("regular Nomi: expected +5/+5, got %+v", nomi)
	}

	nomiAll := findBuffSource(m, CatNomiAll)
	if nomiAll == nil || nomiAll.Attack != 10 || nomiAll.Health != 10 {
		t.Errorf("Nomi All: expected +10/+10, got %+v", nomiAll)
	}
}

func TestCounterBeetleBaseOnly(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Beetle Dnt with SD1=2, SD2=1 → absolute with base 1/1 → 1+2=3, 1+1=2
	setupDntEntity(p, 302, "BG31_808pe", 2, 1)

	found := findBuffSource(m, CatBeetle)
	if found == nil {
		t.Fatal("expected BEETLE buff source, not found")
	}
	if found.Attack != 3 || found.Health != 2 {
		t.Errorf("Beetle base: expected +3/+2, got +%d/+%d", found.Attack, found.Health)
	}
}

func TestCounterBeetleWithBuffs(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Initial SD1=2, SD2=1
	setupDntEntity(p, 302, "BG31_808pe", 2, 1)

	// SD1: 2→5, SD2: 1→4 (absolute, not differential)
	sendSD(p, 302, 1, 5)
	sendSD(p, 302, 2, 4)

	found := findBuffSource(m, CatBeetle)
	if found == nil {
		t.Fatal("expected BEETLE buff source, not found")
	}
	// Absolute: base 1 + SD = 1+5=6, 1+4=5
	if found.Attack != 6 || found.Health != 5 {
		t.Errorf("Beetle buffed: expected +6/+5, got +%d/+%d", found.Attack, found.Health)
	}
}

func TestCounterRightmostAbsolute(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	setupDntEntity(p, 303, "BG34_854pe", 10, 10)

	found := findBuffSource(m, CatRightmost)
	if found == nil {
		t.Fatal("expected RIGHTMOST buff source, not found")
	}
	if found.Attack != 10 || found.Health != 10 {
		t.Errorf("Rightmost: expected +10/+10, got +%d/+%d", found.Attack, found.Health)
	}
}

func TestCounterUndeadAtkOnly(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Undead: SD1 only → ATK only
	setupDntEntity(p, 304, "BG25_011pe", 15, 5)

	found := findBuffSource(m, CatUndead)
	if found == nil {
		t.Fatal("expected UNDEAD buff source, not found")
	}
	if found.Attack != 15 || found.Health != 0 {
		t.Errorf("Undead: expected +15/+0, got +%d/+%d", found.Attack, found.Health)
	}
}

// findBuffSource returns a pointer to the BuffSource for the given category, or nil.
func findBuffSource(m *Machine, category string) *BuffSource {
	for _, bs := range m.State().BuffSources {
		if bs.Category == category {
			return &bs
		}
	}
	return nil
}

// setupNagaMinion adds a Thaumaturgist (BG31_924) to the local player's board,
// satisfying HasNagaSynergyMinion so tag=3809 events emit the ability counter.
func setupNagaMinion(p *Processor) {
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 500,
		CardID:   "BG31_924", // Thaumaturgist
		Tags: map[string]string{
			"CARDTYPE":   "MINION",
			"ZONE":       "PLAY",
			"ATK":        "3",
			"HEALTH":     "2",
			"CONTROLLER": "7",
		},
	})
}

func TestProcessorSpellcraftCounter(t *testing.T) {
	m, p := newProc()
	setupGame(p)
	// Put a Naga synergy minion on the board so HasNagaSynergyMinion returns true.
	setupNagaMinion(p)

	// Tag 3809 value=9 → stacks=1+(9/4)=3, progress=9%4=1 → "Tier 3 · 1/4"
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"3809": "9"},
	})

	// Tag 3809 now drives BOTH the Naga synergy display (CatNagaSpells) and the
	// Spells Played counter (CatSpellsCast).
	ac := findAbilityCounter(m, CatNagaSpells)
	if ac == nil {
		t.Fatalf("expected NAGA_SPELLS counter, got nil")
	}
	if ac.Value != 9 {
		t.Errorf("expected raw value 9, got %d", ac.Value)
	}
	if ac.Display != "Tier 3 · 1/4" {
		t.Errorf("expected display \"Tier 3 · 1/4\", got %q", ac.Display)
	}

	// SPELLS_CAST must also be set to the absolute 3809 value.
	sc := findAbilityCounter(m, CatSpellsCast)
	if sc == nil || sc.Value != 9 {
		t.Errorf("expected SPELLS_CAST=9 from tag 3809, got %v", sc)
	}
}

// TestProcessorSpellcraftCounterNoSynergyMinion verifies that tag=3809 events do NOT
// emit a CatNagaSpells counter when no Naga synergy minion is on the board.
func TestProcessorSpellcraftCounterNoSynergyMinion(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// No Naga synergy minion added — board is empty.
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"3809": "9"},
	})

	s := m.State()
	for _, ac := range s.AbilityCounters {
		if ac.Category == CatNagaSpells {
			t.Errorf("expected no CatNagaSpells counter (no synergy minion on board), got value=%d display=%q",
				ac.Value, ac.Display)
		}
	}
	_ = m
}

func TestProcessorSpellcraftCounterUpdate(t *testing.T) {
	m, p := newProc()
	setupGame(p)
	// Put a Naga synergy minion on the board so HasNagaSynergyMinion returns true.
	setupNagaMinion(p)

	// First update
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"3809": "4"},
	})
	// Second update — should replace, not add
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"3809": "29"},
	})

	// CatNagaSpells must be updated in place (not appended).
	ac := findAbilityCounter(m, CatNagaSpells)
	if ac == nil {
		t.Fatalf("expected NAGA_SPELLS counter after updates, got nil")
	}
	// 29 → stacks=1+(29/4)=8, progress=29%4=1 → "Tier 8 · 1/4"
	if ac.Display != "Tier 8 · 1/4" {
		t.Errorf("expected display \"Tier 8 · 1/4\", got %q", ac.Display)
	}

	// SPELLS_CAST tracks the latest absolute value (29).
	sc := findAbilityCounter(m, CatSpellsCast)
	if sc == nil || sc.Value != 29 {
		t.Errorf("expected SPELLS_CAST=29, got %v", sc)
	}
}

func TestCounterFreeRefresh(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// BACON_FREE_REFRESH_COUNT on local player entity.
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"BACON_FREE_REFRESH_COUNT": "3"},
	})

	ac := findAbilityCounter(m, CatFreeRefresh)
	if ac == nil {
		t.Fatal("expected FREE_REFRESH ability counter, not found")
	}
	if ac.Value != 3 {
		t.Errorf("expected value 3, got %d", ac.Value)
	}
	if ac.Display != "3" {
		t.Errorf("expected display \"3\", got %q", ac.Display)
	}
}

func TestCounterFreeRefreshUpdate(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"BACON_FREE_REFRESH_COUNT": "1"},
	})
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"BACON_FREE_REFRESH_COUNT": "5"},
	})

	s := m.State()
	count := 0
	for _, ac := range s.AbilityCounters {
		if ac.Category == CatFreeRefresh {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 FREE_REFRESH counter, got %d", count)
	}
	ac := findAbilityCounter(m, CatFreeRefresh)
	if ac.Display != "5" {
		t.Errorf("expected display \"5\", got %q", ac.Display)
	}
}

func TestCounterGoldNextTurnBasic(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// BACON_PLAYER_EXTRA_GOLD_NEXT_TURN on local player entity.
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"BACON_PLAYER_EXTRA_GOLD_NEXT_TURN": "4"},
	})

	ac := findAbilityCounter(m, CatGoldNextTurn)
	if ac == nil {
		t.Fatal("expected GOLD_NEXT_TURN ability counter, not found")
	}
	if ac.Value != 4 {
		t.Errorf("expected value 4, got %d", ac.Value)
	}
	if ac.Display != "4" {
		t.Errorf("expected display \"4\", got %q", ac.Display)
	}
}

func TestCounterGoldNextTurnWithOverconfidence(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Set base gold.
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"BACON_PLAYER_EXTRA_GOLD_NEXT_TURN": "2"},
	})

	// Create an Overconfidence Dnt entity (BG28_884e) controlled by local player.
	p.entityController[500] = 7
	p.entityProps[500] = &entityInfo{CardID: "BG28_884e", Zone: ""}

	// Move Overconfidence Dnt into PLAY → overconfidence++
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 500,
		Tags:     map[string]string{"ZONE": "PLAY"},
	})

	ac := findAbilityCounter(m, CatGoldNextTurn)
	if ac == nil {
		t.Fatal("expected GOLD_NEXT_TURN counter after Overconfidence enters PLAY")
	}
	// Display should show "2 (+3 if win)" — 2 sure + 3 conditional from overconfidence
	if ac.Display != "2 (+3 if win)" {
		t.Errorf("expected display \"2 (+3 if win)\", got %q", ac.Display)
	}

	// Remove Overconfidence from PLAY → overconfidence--
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 500,
		Tags:     map[string]string{"ZONE": "GRAVEYARD"},
	})

	ac = findAbilityCounter(m, CatGoldNextTurn)
	if ac.Display != "2" {
		t.Errorf("after Overconfidence removed, expected display \"2\", got %q", ac.Display)
	}
}

func TestCounterGoldNextTurnClearsWhenZero(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Set the counter via the EXTRA_GOLD tag.
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"BACON_PLAYER_EXTRA_GOLD_NEXT_TURN": "4"},
	})
	if findAbilityCounter(m, CatGoldNextTurn) == nil {
		t.Fatal("expected GOLD_NEXT_TURN counter after EXTRA_GOLD=4")
	}

	// Tag returns to 0 — the counter must be cleared, not left stale.
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"BACON_PLAYER_EXTRA_GOLD_NEXT_TURN": "0"},
	})
	if ac := findAbilityCounter(m, CatGoldNextTurn); ac != nil {
		t.Errorf("expected GOLD_NEXT_TURN counter removed when total is 0, still present: %+v", *ac)
	}

	// Overconfidence-only path: counter appears while the Dnt is in PLAY...
	p.entityController[500] = 7
	p.entityProps[500] = &entityInfo{CardID: "BG28_884e", Zone: ""}
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 500,
		Tags:     map[string]string{"ZONE": "PLAY"},
	})
	if findAbilityCounter(m, CatGoldNextTurn) == nil {
		t.Fatal("expected GOLD_NEXT_TURN counter after Overconfidence enters PLAY")
	}

	// ...and is cleared again when it leaves play with sure gold still 0.
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 500,
		Tags:     map[string]string{"ZONE": "GRAVEYARD"},
	})
	if ac := findAbilityCounter(m, CatGoldNextTurn); ac != nil {
		t.Errorf("expected GOLD_NEXT_TURN counter removed after Overconfidence leaves PLAY, still present: %+v", *ac)
	}
}

func TestCounterGoldNextTurnMultipleOverconfidence(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"BACON_PLAYER_EXTRA_GOLD_NEXT_TURN": "1"},
	})

	// Two Overconfidence Dnts.
	for _, eid := range []int{501, 502} {
		p.entityController[eid] = 7
		p.entityProps[eid] = &entityInfo{CardID: "BG28_884e", Zone: ""}
		p.Handle(parser.GameEvent{
			Type:     parser.EventTagChange,
			EntityID: eid,
			Tags:     map[string]string{"ZONE": "PLAY"},
		})
	}

	ac := findAbilityCounter(m, CatGoldNextTurn)
	// 1 sure + 2*3=6 conditional bonus from two Overconfidence Dnts
	if ac.Display != "1 (+6 if win)" {
		t.Errorf("expected display \"1 (+6 if win)\", got %q", ac.Display)
	}
}

func TestCounterGoldNextTurnOverconfidenceResetOnTurnBoundary(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Set base gold.
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"BACON_PLAYER_EXTRA_GOLD_NEXT_TURN": "1"},
	})

	// Create an Overconfidence Dnt in PLAY.
	p.entityController[500] = 7
	p.entityProps[500] = &entityInfo{CardID: "BG28_884e", Zone: ""}
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 500,
		Tags:     map[string]string{"ZONE": "PLAY"},
	})

	ac := findAbilityCounter(m, CatGoldNextTurn)
	if ac == nil || ac.Display != "1 (+3 if win)" {
		t.Fatalf("before turn boundary: expected \"1 (+3 if win)\", got %v", ac)
	}

	// Simulate turn boundary: local player TURN tag changes.
	// Need bgTurnsStarted > 0 for the reset path.
	p.bgTurnsStarted = 1
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"TURN": "9"},
	})

	// Overconfidence should be reset — the Dnt may still be in PLAY
	// but the combat it applied to has resolved.
	ac = findAbilityCounter(m, CatGoldNextTurn)
	if ac.Display != "1" {
		t.Errorf("after turn boundary: expected display \"1\" (no overconfidence), got %q", ac.Display)
	}

	// Verify overconfidenceCount is actually zero.
	if p.localBuffs.overconfidenceCount != 0 {
		t.Errorf("overconfidenceCount should be 0 after turn boundary, got %d", p.localBuffs.overconfidenceCount)
	}
}

func TestParseInt(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"42", 42},
		{"0", 0},
		{"-5", -5},
		{" -3 ", -3},
		{"bad", 0},
		{"", 0},
	}
	for _, tc := range cases {
		got := parseInt(tc.input)
		if got != tc.want {
			t.Errorf("parseInt(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// TestIsLocalPlayerEntity verifies that PlayerID match takes priority over name
// match, and that name fallback is only used when localPlayerID is 0.
func TestIsLocalPlayerEntity(t *testing.T) {
	_, p := newProc()
	p.localPlayerID = 7
	p.localPlayerName = "Alice"

	// Name matches "Alice" but PlayerID is wrong → should be false (ID takes priority).
	e := parser.GameEvent{EntityName: "Alice", PlayerID: 99}
	if p.isLocalPlayerEntity(e) {
		t.Error("name match should be blocked when localPlayerID is known and PlayerID doesn't match")
	}

	// PlayerID matches but name is different → should be true.
	e = parser.GameEvent{EntityName: "Bob", PlayerID: 7}
	if !p.isLocalPlayerEntity(e) {
		t.Error("PlayerID match should return true regardless of name")
	}

	// localPlayerID not yet known (0): name match should work as fallback.
	p.localPlayerID = 0
	e = parser.GameEvent{EntityName: "Alice", PlayerID: 0}
	if !p.isLocalPlayerEntity(e) {
		t.Error("name fallback should return true when localPlayerID == 0 and name matches")
	}

	// localPlayerID not yet known (0): name doesn't match → false.
	e = parser.GameEvent{EntityName: "Bob", PlayerID: 0}
	if p.isLocalPlayerEntity(e) {
		t.Error("name fallback should return false when name doesn't match")
	}
}

func findAbilityCounter(m *Machine, category string) *AbilityCounter {
	for _, ac := range m.State().AbilityCounters {
		if ac.Category == category {
			return &ac
		}
	}
	return nil
}

// TestPendingStatChangesCapFlush verifies that the buffer is flushed early when
// more than maxPendingStatChanges entries accumulate without a turn-boundary event.
func TestPendingStatChangesCapFlush(t *testing.T) {
	_, p := newProc()

	// Start a game and advance to recruit phase.
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "1"},
	})

	// Manually set the local player ID (no CREATE_GAME block in this minimal setup).
	p.localPlayerID = 7

	// Populate entityProps and entityController for 5 fake minions owned by player 7.
	for i := 100; i < 105; i++ {
		p.entityController[i] = 7
		p.entityProps[i] = &entityInfo{
			Name:     fmt.Sprintf("Minion%d", i),
			CardType: "MINION",
			Zone:     "PLAY",
			Attack:   2,
			Health:   2,
		}
	}

	// Feed exactly maxPendingStatChanges+1 stat changes without any turn-boundary event.
	// The overflow guard must flush the buffer when the 201st entry is appended,
	// leaving the slice empty (the flush resets it to len 0).
	total := maxPendingStatChanges + 1
	for i := 0; i < total; i++ {
		entityID := 100 + (i % 5)
		// Keep the entity's stored Attack one below the value we're about to send,
		// so every call sees a real non-zero delta and actually appends to the buffer.
		info := p.entityProps[entityID]
		newAtk := info.Attack + 1
		e := parser.GameEvent{
			EntityID: entityID,
			PlayerID: 7,
			Tags:     map[string]string{"ATK": fmt.Sprintf("%d", newAtk)},
		}
		p.updateMinionStatByID(e.EntityID, e.EntityName, "ATK", newAtk)
		// Reset stored value so the next iteration for this entity sees a fresh delta.
		if info2 := p.entityProps[entityID]; info2 != nil {
			info2.Attack = newAtk - 1
		}
	}

	// After exactly maxPendingStatChanges+1 appends the guard fires and flushes,
	// so the buffer must be empty.
	if len(p.pendingStatChanges) != 0 {
		t.Errorf("expected pendingStatChanges to be flushed (len=0), got len=%d", len(p.pendingStatChanges))
	}
}

// TestBoardSnapshotPhaseGate verifies that tryAddMinionFromRegistry only
// updates the board snapshot during PhaseRecruit, not during PhaseCombat.
// This prevents combat copies (which arrive in PLAY with base stats before
// receiving their buffed TAG_CHANGEs) from overwriting the correct
// recruit-phase snapshot that GameEnd restores.
func TestBoardSnapshotPhaseGate(t *testing.T) {
	m, p := newProc()
	setupGame(p)

	// Manually wire internal state for direct tryAddMinionFromRegistry calls.
	p.localPlayerID = 7

	// ── Recruit phase ────────────────────────────────────────────────────────
	// Enter recruit phase (GameEntity turn 1 = odd = recruit).
	m.SetGameEntityTurn(1)
	if m.Phase() != PhaseRecruit {
		t.Fatalf("expected RECRUIT after SetGameEntityTurn(1), got %s", m.Phase())
	}

	// Register a minion with buffed recruit-phase stats in the entity registry.
	p.entityProps[200] = &entityInfo{
		Name:     "Buffed Recruit Minion",
		CardID:   "TEST_001",
		CardType: "MINION",
		Attack:   5,
		Health:   8,
	}
	p.entityController[200] = 7

	// Call tryAddMinionFromRegistry in recruit phase — snapshot should update.
	p.tryAddMinionFromRegistry(200, 7)

	// The snapshot was taken during recruit phase; confirm minion is on board.
	board := m.State().Board
	if len(board) != 1 {
		t.Fatalf("expected 1 minion on board after recruit-phase add, got %d", len(board))
	}
	if board[0].Attack != 5 || board[0].Health != 8 {
		t.Errorf("recruit-phase minion stats wrong: got %d/%d, want 5/8", board[0].Attack, board[0].Health)
	}

	// ── Enter combat phase ───────────────────────────────────────────────────
	// SetGameEntityTurn(2) snapshots the board and transitions to PhaseCombat.
	m.SetGameEntityTurn(2)
	if m.Phase() != PhaseCombat {
		t.Fatalf("expected COMBAT after SetGameEntityTurn(2), got %s", m.Phase())
	}

	// The snapshot taken by SetGameEntityTurn(2) has the buffed recruit stats.
	// Simulate a combat copy: it arrives in PLAY with lower base stats.
	// Add it to the registry with base stats (as the log would show).
	p.entityProps[201] = &entityInfo{
		Name:     "Combat Copy",
		CardID:   "TEST_001",
		CardType: "MINION",
		Attack:   2, // base stats, NOT the buffed 5/8
		Health:   3,
	}
	p.entityController[201] = 7

	// Call tryAddMinionFromRegistry in combat phase — snapshot must NOT update.
	p.tryAddMinionFromRegistry(201, 7)

	// Board now has two minions (recruit minion + combat copy), but the snapshot
	// must still reflect only the pre-combat recruit board (one minion, 5/8).
	// Simulate GameEnd restoring the snapshot.
	m.GameEnd(1, time.Now())
	restoredBoard := m.State().Board
	if len(restoredBoard) != 1 {
		t.Fatalf("after GameEnd, expected 1 minion (recruit snapshot), got %d", len(restoredBoard))
	}
	if restoredBoard[0].Attack != 5 || restoredBoard[0].Health != 8 {
		t.Errorf("restored board has wrong stats: got %d/%d, want 5/8", restoredBoard[0].Attack, restoredBoard[0].Health)
	}
	if restoredBoard[0].EntityID != 200 {
		t.Errorf("restored board has wrong entity: got %d, want 200", restoredBoard[0].EntityID)
	}
}

// TestTavernBuffedMinionStats verifies that when a minion receives buffed stats
// while still owned by the tavern (CONTROLLER=15), and then the player buys it,
// the board shows the buffed stats — not the base stats from SHOW_ENTITY.
// Regression test for: Wrath Weaver showing 1/4 instead of 25/65.
func TestTavernBuffedMinionStats(t *testing.T) {
	m, p := newProc()
	setupGame(p)
	p.localPlayerID = 7

	m.SetGameEntityTurn(1) // recruit

	// ── Tavern creates minion with base stats (SHOW_ENTITY) ─────────────
	// CONTROLLER=15 (tavern), so this is NOT the local player's entity yet.
	p.Handle(parser.GameEvent{
		Type:       parser.EventEntityUpdate,
		EntityID:   18706,
		CardID:     "BGS_004",
		EntityName: "Wrath Weaver",
		PlayerID:   15,
		Tags: map[string]string{
			"CONTROLLER": "15",
			"CARDTYPE":   "MINION",
			"ZONE":       "PLAY",
			"ATK":        "1",
			"HEALTH":     "4",
			"CARDRACE":   "DEMON",
		},
	})

	// ── Tavern applies shop buff enchantment → real stats ───────────────
	// TAG_CHANGE ATK=25, HEALTH=65 while CONTROLLER is still 15.
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityID:   18706,
		EntityName: "Wrath Weaver",
		PlayerID:   15,
		Tags:       map[string]string{"HEALTH": "65"},
	})
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityID:   18706,
		EntityName: "Wrath Weaver",
		PlayerID:   15,
		Tags:       map[string]string{"ATK": "25"},
	})

	// ── CONTROLLER changes to local player (buy) ────────────────────────
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityID:   18706,
		EntityName: "Wrath Weaver",
		PlayerID:   15,
		Tags:       map[string]string{"CONTROLLER": "7"},
	})

	// ── Minion placed on board (ZONE=PLAY after buy) ────────────────────
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityID:   18706,
		EntityName: "Wrath Weaver",
		PlayerID:   7,
		Tags:       map[string]string{"ZONE": "PLAY"},
	})

	board := m.State().Board
	if len(board) != 1 {
		t.Fatalf("expected 1 minion on board, got %d", len(board))
	}
	if board[0].Attack != 25 {
		t.Errorf("Attack: want 25, got %d (entity registry missed tavern-phase TAG_CHANGE)", board[0].Attack)
	}
	if board[0].Health != 65 {
		t.Errorf("Health: want 65, got %d (entity registry missed tavern-phase TAG_CHANGE)", board[0].Health)
	}
}

// TestCombatCopyStatRecovery verifies that peak stats from combat copies
// (SETASIDE entities during combat) update the board snapshot.
// This is a secondary safety net for enchantment-only stat changes.
func TestCombatCopyStatRecovery(t *testing.T) {
	m, p := newProc()
	setupGame(p)
	p.localPlayerID = 7

	m.SetGameEntityTurn(1) // recruit
	p.entityProps[300] = &entityInfo{
		Name: "Wrath Weaver", CardID: "BGS_004", CardType: "MINION",
		Attack: 1, Health: 4, Zone: "PLAY",
	}
	p.entityController[300] = 7
	m.UpsertMinion(MinionState{EntityID: 300, CardID: "BGS_004", Name: "Wrath Weaver", Attack: 1, Health: 4})
	m.UpdateBoardSnapshot()

	m.SetGameEntityTurn(2) // combat

	// Combat copy in SETASIDE shows real stats then strips to base.
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 500, CardID: "BGS_004",
		EntityName: "Wrath Weaver", PlayerID: 7,
		Tags: map[string]string{"CONTROLLER": "7", "CARDTYPE": "MINION", "ZONE": "SETASIDE", "ATK": "1", "HEALTH": "4"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500, PlayerID: 7,
		Tags: map[string]string{"ATK": "25"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500, PlayerID: 7,
		Tags: map[string]string{"HEALTH": "65"},
	})
	// Stripped back to base.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500, PlayerID: 7,
		Tags: map[string]string{"ATK": "1"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500, PlayerID: 7,
		Tags: map[string]string{"HEALTH": "4"},
	})

	m.GameEnd(3, time.Now())
	restored := m.State().Board
	if len(restored) != 1 {
		t.Fatalf("expected 1 minion, got %d", len(restored))
	}
	if restored[0].Attack != 25 {
		t.Errorf("Attack: want 25, got %d", restored[0].Attack)
	}
	if restored[0].Health != 65 {
		t.Errorf("Health: want 65, got %d", restored[0].Health)
	}
}

// ── Duos detection and partner tracking tests ────────────────────────────────

// setupDuosGame sends a game start sequence with Duos tags to identify both
// local and partner players.
func setupDuosGame(p *Processor) {
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	// Local player: hi≠0, with BACON_DUO_TEAMMATE_PLAYER_ID
	p.Handle(parser.GameEvent{
		Type:     parser.EventPlayerDef,
		EntityID: 20,
		PlayerID: 7,
		Tags: map[string]string{
			"hi":                              "144115193835963207",
			"lo":                              "30722021",
			"PLAYER_ID":                       "7",
			"BACON_DUO_TEAMMATE_PLAYER_ID":    "8",
		},
	})
	// Partner player: hi≠0, PlayerID matches BACON_DUO_TEAMMATE_PLAYER_ID
	p.Handle(parser.GameEvent{
		Type:     parser.EventPlayerDef,
		EntityID: 21,
		PlayerID: 8,
		Tags: map[string]string{
			"hi":        "144115193835963208",
			"lo":        "30722022",
			"PLAYER_ID": "8",
		},
	})
	// Dummy players: hi=0
	p.Handle(parser.GameEvent{
		Type:     parser.EventPlayerDef,
		EntityID: 22,
		PlayerID: 15,
		Tags:     map[string]string{"hi": "0", "lo": "0", "PLAYER_ID": "15"},
	})
	p.Handle(parser.GameEvent{
		Type:     parser.EventPlayerDef,
		EntityID: 23,
		PlayerID: 16,
		Tags:     map[string]string{"hi": "0", "lo": "0", "PLAYER_ID": "16"},
	})
	// Player names
	p.Handle(parser.GameEvent{
		Type:       parser.EventPlayerName,
		PlayerID:   7,
		EntityName: "LocalPlayer#1234",
	})
	p.Handle(parser.GameEvent{
		Type:       parser.EventPlayerName,
		PlayerID:   8,
		EntityName: "PartnerPlayer#5678",
	})
}

func TestDuosDetection(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p)

	s := m.State()
	if !s.IsDuos {
		t.Error("expected IsDuos=true")
	}
	if s.Partner == nil {
		t.Fatal("expected Partner != nil")
	}
	if s.Partner.Name != "PartnerPlayer#5678" {
		t.Errorf("expected partner name PartnerPlayer#5678, got %q", s.Partner.Name)
	}
	if p.partnerPlayerID != 8 {
		t.Errorf("expected partnerPlayerID=8, got %d", p.partnerPlayerID)
	}
}

func TestSoloGameNoDuos(t *testing.T) {
	m, p := newProc()
	setupGame(p) // standard solo setup

	s := m.State()
	if s.IsDuos {
		t.Error("expected IsDuos=false for solo game")
	}
	if s.Partner != nil {
		t.Error("expected Partner=nil for solo game")
	}
}

func TestDuosPartnerHealthTracking(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p)

	// Assign partner hero entity (same as real Power.log).
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		PlayerID:   8,
		EntityName: "PartnerPlayer#5678",
		Tags:       map[string]string{"HERO_ENTITY": "100"},
	})

	// Register entity 100 as a hero entity owned by partner.
	p.entityController[100] = 8
	p.heroEntities[100] = true

	// HEALTH and DAMAGE arrive on the hero entity.
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 100,
		Tags:     map[string]string{"HEALTH": "30"},
	})
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 100,
		Tags:     map[string]string{"DAMAGE": "5"},
	})

	s := m.State()
	if s.Partner.Health != 30 {
		t.Errorf("expected partner health 30, got %d", s.Partner.Health)
	}
	if s.Partner.Damage != 5 {
		t.Errorf("expected partner damage 5, got %d", s.Partner.Damage)
	}
}

func TestDeferredPartnerResolution(t *testing.T) {
	m, p := newProc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	p.Handle(parser.GameEvent{
		Type: parser.EventGameEntityTags,
		Tags: map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "1"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 14, PlayerID: 5,
		Tags: map[string]string{"hi": "144115193835963207", "lo": "30722021", "PLAYER_ID": "5", "HERO_ENTITY": "33"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 15, PlayerID: 13,
		Tags: map[string]string{"hi": "0", "lo": "0", "PLAYER_ID": "13"},
	})
	// Player names
	p.Handle(parser.GameEvent{Type: parser.EventPlayerName, PlayerID: 5, EntityName: "Moch#1358"})

	// DUO_PASSABLE confirms duos when combined with PUNISH_LEAVERS
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1143,
		Tags: map[string]string{"BACON_DUO_PASSABLE": "1"},
	})

	// Partner hero appears with PLAYER_ID=6 via FULL_ENTITY
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 146, CardID: "BG32_HERO_002",
		EntityName: "Buttons",
		Tags: map[string]string{"CONTROLLER": "13", "CARDTYPE": "HERO", "PLAYER_ID": "6", "HEALTH": "30", "ZONE": "PLAY"},
	})

	// BACON_CURRENT_COMBAT_PLAYER_ID fires with partner's combat
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 14, PlayerID: 5,
		EntityName: "Moch#1358",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "6"},
	})

	// Partner should now be resolved
	if p.partnerPlayerID != 6 {
		t.Errorf("expected partnerPlayerID=6, got %d", p.partnerPlayerID)
	}
	s := m.State()
	if s.Partner == nil {
		t.Fatal("expected Partner != nil")
	}
	if s.Partner.HeroCardID != "BG32_HERO_002" {
		t.Errorf("expected partner hero BG32_HERO_002, got %q", s.Partner.HeroCardID)
	}
}

func TestPartnerBoardSnapshot(t *testing.T) {
	m := New()
	m.GameStart("test", time.Now())
	m.SetDuosMode(true)

	minions := []MinionState{
		{EntityID: 100, CardID: "BGS_119", Name: "Crackling Cyclone", Attack: 220, Health: 177},
		{EntityID: 101, CardID: "BG32_111", Name: "Mirror Monster", Attack: 150, Health: 132},
	}
	m.SetPartnerBoard(minions, 17)

	s := m.State()
	if s.PartnerBoard == nil {
		t.Fatal("expected PartnerBoard != nil")
	}
	if s.PartnerBoard.Turn != 17 {
		t.Errorf("expected turn 17, got %d", s.PartnerBoard.Turn)
	}
	if len(s.PartnerBoard.Minions) != 2 {
		t.Errorf("expected 2 minions, got %d", len(s.PartnerBoard.Minions))
	}
	if s.PartnerBoard.Stale {
		t.Error("expected not stale")
	}

	// Mark stale
	m.MarkPartnerBoardStale()
	s = m.State()
	if !s.PartnerBoard.Stale {
		t.Error("expected stale after MarkPartnerBoardStale")
	}

	// New snapshot clears stale
	m.SetPartnerBoard(minions, 18)
	s = m.State()
	if s.PartnerBoard.Stale {
		t.Error("expected not stale after new snapshot")
	}
}

func TestPartnerBoardCaptureFromCombat(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p)

	// Set up partner hero (PLAYER_ID=8, CONTROLLER=15=bot)
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 146, CardID: "BG32_HERO_002",
		EntityName: "Buttons",
		Tags: map[string]string{"CONTROLLER": "15", "CARDTYPE": "HERO", "PLAYER_ID": "8", "HEALTH": "30", "ZONE": "PLAY"},
	})

	// Advance to turn 5 to have a turn number
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"TURN": "5"},
	})

	// Partner's combat starts: BACON_CURRENT_COMBAT_PLAYER_ID = 8 (partner)
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "8"},
	})

	// Partner hero combat copy appears with CONTROLLER=7 (localPlayerID).
	// In duos, the partner's combat copies use CONTROLLER=localPlayerID,
	// while the opponent's copies use CONTROLLER=botID.
	// PLAYER_ID arrives via a separate TAG_CHANGE after FULL_ENTITY.
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 700, CardID: "BG32_HERO_002",
		Tags: map[string]string{
			"CONTROLLER": "7", "CARDTYPE": "HERO",
			"HEALTH": "30", "ZONE": "PLAY",
		},
	})
	// PLAYER_ID assigned via TAG_CHANGE after FULL_ENTITY
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 700,
		Tags: map[string]string{"PLAYER_ID": "8"},
	})

	// Partner's combat minions (same CONTROLLER=7 as localPlayerID)
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 703, CardID: "BGS_119",
		EntityName: "Crackling Cyclone",
		Tags: map[string]string{
			"CONTROLLER": "7", "CARDTYPE": "MINION",
			"ATK": "220", "HEALTH": "177", "ZONE": "PLAY", "ZONE_POSITION": "1",
		},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 704, CardID: "BG32_111",
		EntityName: "Mirror Monster",
		Tags: map[string]string{
			"CONTROLLER": "7", "CARDTYPE": "MINION",
			"ATK": "150", "HEALTH": "132", "ZONE": "PLAY", "ZONE_POSITION": "2",
		},
	})

	// Combat ends — local player's combat starts
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "7"},
	})

	s := m.State()
	if s.PartnerBoard == nil {
		t.Fatal("expected PartnerBoard != nil after partner combat")
	}
	if len(s.PartnerBoard.Minions) != 2 {
		t.Errorf("expected 2 partner minions, got %d", len(s.PartnerBoard.Minions))
	}
	if s.PartnerBoard.Minions[0].Attack != 220 {
		t.Errorf("expected first minion ATK=220, got %d", s.PartnerBoard.Minions[0].Attack)
	}
}

func TestPartnerBoardCaptureRetroactive(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p)

	// Set up partner hero (for partner identification)
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 146, CardID: "BG32_HERO_002",
		EntityName: "Buttons",
		Tags: map[string]string{"CONTROLLER": "15", "CARDTYPE": "HERO", "PLAYER_ID": "8", "HEALTH": "30", "ZONE": "PLAY"},
	})

	// Advance to turn 3
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"TURN": "3"},
	})

	// Local combat starts first: BACON_CURRENT_COMBAT_PLAYER_ID = 7
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "7"},
	})

	// Partner hero combat copy created BEFORE partner combat flag
	// (CONTROLLER=7=localPlayerID, not botID)
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 695, CardID: "BG32_HERO_002",
		Tags: map[string]string{
			"CONTROLLER": "7", "CARDTYPE": "HERO",
			"HEALTH": "30", "ZONE": "PLAY",
		},
	})
	// PLAYER_ID arrives via TAG_CHANGE
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 695,
		Tags: map[string]string{"PLAYER_ID": "8"},
	})

	// Partner minion created BEFORE partner combat flag
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 698, CardID: "BGS_119",
		EntityName: "Crackling Cyclone",
		Tags: map[string]string{
			"CONTROLLER": "7", "CARDTYPE": "MINION",
			"ATK": "10", "HEALTH": "5", "ZONE": "PLAY", "ZONE_POSITION": "1",
		},
	})

	// NOW partner combat flag fires
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "8"},
	})

	// Combat ends
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "0"},
	})

	s := m.State()
	if s.PartnerBoard == nil {
		t.Fatal("expected PartnerBoard != nil after retroactive partner combat capture")
	}
	if len(s.PartnerBoard.Minions) != 1 {
		t.Errorf("expected 1 partner minion (retroactive), got %d", len(s.PartnerBoard.Minions))
	}
	if len(s.PartnerBoard.Minions) > 0 && s.PartnerBoard.Minions[0].Attack != 10 {
		t.Errorf("expected first minion ATK=10, got %d", s.PartnerBoard.Minions[0].Attack)
	}
}

// setupDuosSnapshotBoard puts a local minion on the board during recruit and
// snapshots it, then advances to combat. Shared by the partner-combat-copy
// snapshot corruption tests.
func setupDuosSnapshotBoard(m *Machine, p *Processor) {
	m.SetGameEntityTurn(1) // recruit
	p.entityProps[300] = &entityInfo{
		Name: "Wrath Weaver", CardID: "BGS_004", CardType: "MINION",
		Attack: 5, Health: 8, Zone: "PLAY",
	}
	p.entityController[300] = 7
	m.UpsertMinion(MinionState{EntityID: 300, CardID: "BGS_004", Name: "Wrath Weaver", Attack: 5, Health: 8})
	m.UpdateBoardSnapshot()
	m.SetGameEntityTurn(2) // combat
}

// TestDuosPartnerCombatCopyDoesNotCorruptSnapshot is the regression test for
// audit finding H1: in duos, the PARTNER's combat copies have
// CONTROLLER=localPlayerID, so they were registered as local combat-copy peak
// trackers. A partner minion sharing a CardID with a local board minion would
// raise the local snapshot's stats to the partner's buffed values, corrupting
// the final restored board.
func TestDuosPartnerCombatCopyDoesNotCorruptSnapshot(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p)
	setupDuosSnapshotBoard(m, p)

	// Partner's combat starts.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags:       map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "8"},
	})

	// Partner's combat copy of the SAME CardID arrives SETASIDE with
	// CONTROLLER=7 (localPlayerID — see partner tracking comment in
	// handleEntityUpdate), then receives the partner's buffed stats.
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 500, CardID: "BGS_004",
		EntityName: "Wrath Weaver",
		Tags:       map[string]string{"CONTROLLER": "7", "CARDTYPE": "MINION", "ZONE": "SETASIDE", "ATK": "1", "HEALTH": "4"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500,
		Tags: map[string]string{"ATK": "44"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500,
		Tags: map[string]string{"HEALTH": "99"},
	})

	// Partner's combat ends.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags:       map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "0"},
	})

	m.GameEnd(3, time.Now())
	restored := m.State().Board
	if len(restored) != 1 {
		t.Fatalf("expected 1 minion on restored board, got %d", len(restored))
	}
	if restored[0].Attack != 5 {
		t.Errorf("Attack: want 5 (original), got %d (corrupted by partner combat copy)", restored[0].Attack)
	}
	if restored[0].Health != 8 {
		t.Errorf("Health: want 8 (original), got %d (corrupted by partner combat copy)", restored[0].Health)
	}
}

// TestDuosPartnerCombatCopyRegisteredBeforeFlag covers the retroactive case:
// the partner's combat copy is created BEFORE BACON_CURRENT_COMBAT_PLAYER_ID
// fires (and was therefore registered as a peak tracker), but its buffed stats
// arrive after the flag. Registering it must not corrupt the snapshot once the
// partner combat is identified.
func TestDuosPartnerCombatCopyRegisteredBeforeFlag(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p)
	setupDuosSnapshotBoard(m, p)

	// Partner combat copy created BEFORE the combat-player flag fires.
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 500, CardID: "BGS_004",
		EntityName: "Wrath Weaver",
		Tags:       map[string]string{"CONTROLLER": "7", "CARDTYPE": "MINION", "ZONE": "SETASIDE", "ATK": "1", "HEALTH": "4"},
	})

	// NOW the partner combat flag fires.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags:       map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "8"},
	})

	// Buffed partner stats arrive after the flag.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500,
		Tags: map[string]string{"ATK": "44"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500,
		Tags: map[string]string{"HEALTH": "99"},
	})

	// Partner's combat ends.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags:       map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "0"},
	})

	m.GameEnd(3, time.Now())
	restored := m.State().Board
	if len(restored) != 1 {
		t.Fatalf("expected 1 minion on restored board, got %d", len(restored))
	}
	if restored[0].Attack != 5 {
		t.Errorf("Attack: want 5 (original), got %d (corrupted by pre-flag partner combat copy)", restored[0].Attack)
	}
	if restored[0].Health != 8 {
		t.Errorf("Health: want 8 (original), got %d (corrupted by pre-flag partner combat copy)", restored[0].Health)
	}
}

// TestDuosLocalCombatCopyStillUpdatesSnapshot verifies the companion behavior:
// during the LOCAL player's combat in a duos game, combat-copy peak tracking
// must still recover enchantment-only recruit stats (the intended behavior of
// UpdateSnapshotFromCombatCopy — see TestCombatCopyStatRecovery for solo).
func TestDuosLocalCombatCopyStillUpdatesSnapshot(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p)
	setupDuosSnapshotBoard(m, p)

	// LOCAL player's combat starts.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags:       map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "7"},
	})

	// Local combat copy briefly shows the real (enchantment-inclusive) stats
	// before being stripped to base.
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 500, CardID: "BGS_004",
		EntityName: "Wrath Weaver",
		Tags:       map[string]string{"CONTROLLER": "7", "CARDTYPE": "MINION", "ZONE": "SETASIDE", "ATK": "1", "HEALTH": "4"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500,
		Tags: map[string]string{"ATK": "25"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500,
		Tags: map[string]string{"HEALTH": "65"},
	})
	// Stripped back to base.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500,
		Tags: map[string]string{"ATK": "1"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500,
		Tags: map[string]string{"HEALTH": "4"},
	})

	// Local combat ends.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags:       map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "0"},
	})

	m.GameEnd(3, time.Now())
	restored := m.State().Board
	if len(restored) != 1 {
		t.Fatalf("expected 1 minion on restored board, got %d", len(restored))
	}
	if restored[0].Attack != 25 {
		t.Errorf("Attack: want 25 (peak from local combat copy), got %d", restored[0].Attack)
	}
	if restored[0].Health != 65 {
		t.Errorf("Health: want 65 (peak from local combat copy), got %d", restored[0].Health)
	}
}

func TestDuosDntEnchantmentWithBotController(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p)

	// Set up local hero entity 33 controlled by local player 7
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 33, CardID: "TB_BaconShop_HERO_49",
		Tags: map[string]string{"CONTROLLER": "7", "CARDTYPE": "HERO", "HEALTH": "40", "ZONE": "PLAY"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"HERO_ENTITY": "33"},
	})

	// BG_ShopBuff Dnt enchantment with CONTROLLER=15 (bot), ATTACHED to local hero 33
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 500, CardID: "BG_ShopBuff",
		Tags: map[string]string{
			"CONTROLLER": "15", "CARDTYPE": "ENCHANTMENT",
			"ATTACHED": "33", "ZONE": "PLAY",
			"TAG_SCRIPT_DATA_NUM_1": "7", "TAG_SCRIPT_DATA_NUM_2": "7",
		},
	})

	s := m.State()
	found := false
	for _, bs := range s.BuffSources {
		if bs.Category == CatShopBuff {
			found = true
			if bs.Attack != 7 || bs.Health != 7 {
				t.Errorf("expected ShopBuff +7/+7, got +%d/+%d", bs.Attack, bs.Health)
			}
		}
	}
	if !found {
		t.Error("expected Shop Buff source to be tracked despite bot CONTROLLER")
	}
}

func TestDuosDetectionViaPunishLeavers(t *testing.T) {
	m, p := newProc()
	// Game start
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	// GameEntity tags with BACON_DUOS_PUNISH_LEAVERS
	p.Handle(parser.GameEvent{
		Type: parser.EventGameEntityTags,
		Tags: map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "1"},
	})
	// Local player (no BACON_DUO_TEAMMATE_PLAYER_ID)
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 14, PlayerID: 5,
		Tags: map[string]string{"hi": "144115193835963207", "lo": "30722021", "PLAYER_ID": "5", "HERO_ENTITY": "33"},
	})
	// Dummy player
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 15, PlayerID: 13,
		Tags: map[string]string{"hi": "0", "lo": "0", "PLAYER_ID": "13"},
	})

	s := m.State()
	if s.IsDuos {
		t.Error("expected IsDuos=false — PUNISH_LEAVERS alone is no longer sufficient for duos detection")
	}
	// Partner ID not yet known
	if p.partnerPlayerID != 0 {
		t.Errorf("expected partnerPlayerID=0 (deferred), got %d", p.partnerPlayerID)
	}
}

func TestDuosDetectionViaDuoPassable(t *testing.T) {
	m, p := newProc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 14, PlayerID: 5,
		Tags: map[string]string{"hi": "144115193835963207", "lo": "30722021", "PLAYER_ID": "5"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 15, PlayerID: 13,
		Tags: map[string]string{"hi": "0", "lo": "0", "PLAYER_ID": "13"},
	})

	if m.State().IsDuos {
		t.Fatal("should not be duos yet")
	}

	// BACON_DUO_PASSABLE on a card entity — alone is no longer sufficient
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1143,
		Tags: map[string]string{"BACON_DUO_PASSABLE": "1"},
	})

	if m.State().IsDuos {
		t.Error("expected IsDuos=false — DUO_PASSABLE alone is no longer sufficient for duos detection")
	}
}

func TestDuosDetectionCombinedPunishLeaversAndPassable(t *testing.T) {
	m, p := newProc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	// GameEntity tags with PUNISH_LEAVERS
	p.Handle(parser.GameEvent{
		Type: parser.EventGameEntityTags,
		Tags: map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "1"},
	})
	// Local player (no TEAMMATE tag)
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 14, PlayerID: 5,
		Tags: map[string]string{"hi": "144115193835963207", "lo": "30722021", "PLAYER_ID": "5"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 15, PlayerID: 13,
		Tags: map[string]string{"hi": "0", "lo": "0", "PLAYER_ID": "13"},
	})

	if m.State().IsDuos {
		t.Fatal("should not be duos after PUNISH_LEAVERS alone")
	}

	// DUO_PASSABLE arrives — combined with PUNISH_LEAVERS, should trigger duos
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1143,
		Tags: map[string]string{"BACON_DUO_PASSABLE": "1"},
	})

	if !m.State().IsDuos {
		t.Error("expected IsDuos=true from combined PUNISH_LEAVERS + DUO_PASSABLE")
	}
}

func TestDuosUnsetViaPunishLeaversZero(t *testing.T) {
	m, p := newProc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	// Set up combined signals
	p.Handle(parser.GameEvent{
		Type: parser.EventGameEntityTags,
		Tags: map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "1"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 14, PlayerID: 5,
		Tags: map[string]string{"hi": "144115193835963207", "lo": "30722021", "PLAYER_ID": "5"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 15, PlayerID: 13,
		Tags: map[string]string{"hi": "0", "lo": "0", "PLAYER_ID": "13"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1143,
		Tags: map[string]string{"BACON_DUO_PASSABLE": "1"},
	})

	if !m.State().IsDuos {
		t.Fatal("expected IsDuos=true from combined signals")
	}

	// PUNISH_LEAVERS=0 should clear duos (backup-only detection)
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1,
		EntityName: "GameEntity",
		Tags: map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "0"},
	})

	if m.State().IsDuos {
		t.Error("expected IsDuos=false after PUNISH_LEAVERS changed to 0")
	}
}

func TestDuosNotUnsetWhenFromTeammate(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p)

	if !m.State().IsDuos {
		t.Fatal("expected IsDuos=true from TEAMMATE_PLAYER_ID")
	}

	// PUNISH_LEAVERS=0 should NOT clear duos when set via authoritative source
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1,
		EntityName: "GameEntity",
		Tags: map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "0"},
	})

	if !m.State().IsDuos {
		t.Error("expected IsDuos to remain true — duos was set via authoritative TEAMMATE_PLAYER_ID")
	}
}

func TestDuosTeammateSignalAfterBackupDetection(t *testing.T) {
	m, p := newProc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	// Backup detection: PUNISH_LEAVERS + DUO_PASSABLE (no TEAMMATE tag in the def).
	p.Handle(parser.GameEvent{
		Type: parser.EventGameEntityTags,
		Tags: map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "1"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 14, PlayerID: 5,
		Tags: map[string]string{"hi": "144115193835963207", "lo": "30722021", "PLAYER_ID": "5"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 15, PlayerID: 13,
		Tags: map[string]string{"hi": "0", "lo": "0", "PLAYER_ID": "13"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1143,
		Tags: map[string]string{"BACON_DUO_PASSABLE": "1"},
	})
	if !m.State().IsDuos {
		t.Fatal("expected IsDuos=true from combined PUNISH_LEAVERS + DUO_PASSABLE")
	}

	// Authoritative teammate signal arrives later via TAG_CHANGE on the local
	// player entity — it must not be discarded just because duos is already set.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 14, PlayerID: 5,
		Tags: map[string]string{"BACON_DUO_TEAMMATE_PLAYER_ID": "8"},
	})
	if p.partnerPlayerID != 8 {
		t.Errorf("expected partnerPlayerID=8 recorded from teammate signal, got %d", p.partnerPlayerID)
	}
	if !p.duosFromTeammate {
		t.Error("expected duosFromTeammate=true after authoritative teammate signal")
	}

	// A later PUNISH_LEAVERS=0 must NOT clear duos — authoritative evidence exists.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1,
		EntityName: "GameEntity",
		Tags:       map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "0"},
	})
	if !m.State().IsDuos {
		t.Error("expected IsDuos to remain true — teammate signal arrived after backup detection")
	}
}

func TestSetDuosModeFalseClearsPartner(t *testing.T) {
	m, _ := newProc()
	m.SetDuosMode(true)

	s := m.State()
	if !s.IsDuos {
		t.Fatal("expected IsDuos=true after SetDuosMode(true)")
	}
	if s.Partner == nil {
		t.Fatal("expected Partner to be initialized")
	}

	m.SetDuosMode(false)

	s = m.State()
	if s.IsDuos {
		t.Error("expected IsDuos=false after SetDuosMode(false)")
	}
	if s.Partner != nil {
		t.Error("expected Partner=nil after SetDuosMode(false)")
	}
	if s.PartnerBoard != nil {
		t.Error("expected PartnerBoard=nil after SetDuosMode(false)")
	}
}

func TestStaleGameTimeout(t *testing.T) {
	m, p := newProc()
	setupGame(p)
	// Advance to recruit phase
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "Moch#1358",
		Tags: map[string]string{"TURN": "1"},
	})

	// Game is active
	if m.Phase() != PhaseRecruit {
		t.Fatalf("expected RECRUIT, got %s", m.Phase())
	}

	// Simulate 4 minutes of no events
	p.lastEventTime = time.Now().Add(-4 * time.Minute)
	p.CheckStaleness()

	if m.Phase() != PhaseGameOver {
		t.Errorf("expected GAME_OVER after staleness timeout, got %s", m.Phase())
	}
}

func TestStaleGameNoopWhenIdle(t *testing.T) {
	_, p := newProc()
	// No game started — CheckStaleness should be a no-op
	p.lastEventTime = time.Now().Add(-10 * time.Minute)
	p.CheckStaleness()
	// No panic or state change
}

// ── Dual-path routing regression tests (audit M1/M2) ─────────────────────────

// daemonRoute mirrors the daemon's routing contract in cmd/battlestream/main.go:
// a non-nil build result (concrete action OR action.Drop) goes to the dispatcher;
// only nil (not migrated) falls through to the legacy Processor.Handle path.
func daemonRoute(t *testing.T, bldr *builder.ActionBuilder, disp *ActionDispatcher, p *Processor, e parser.GameEvent) {
	t.Helper()
	if a := bldr.Build(e); a != nil {
		if err := disp.Dispatch(a); err != nil {
			t.Fatalf("dispatch error: %v", err)
		}
	} else {
		p.Handle(e)
	}
}

// Audit M1 regression: an event routed through the dispatcher (bypassing
// Processor.Handle) must refresh the staleness clock. Pre-fix, only Handle()
// updated lastEventTime, so dispatcher-routed events on a live game would let
// CheckStaleness force a spurious GAME_OVER.
func TestStalenessDispatcherPathRefreshesClock(t *testing.T) {
	m, p := newProc()
	setupGame(p)
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"TURN": "1"},
	})
	if m.Phase() != PhaseRecruit {
		t.Fatalf("expected RECRUIT, got %s", m.Phase())
	}

	disp := NewActionDispatcher(
		p.AsRecruitVisitor(), p.AsCombatVisitor(), p.AsTransitionVisitor(), p.Touch)

	// Clock is past the staleness deadline, then a dispatcher-routed event arrives.
	p.lastEventTime = time.Now().Add(-4 * time.Minute)
	if err := disp.Dispatch(&action.TurnTransitionAction{
		NewPhase:       action.PhaseRecruit,
		GameEntityTurn: 3,
	}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	p.CheckStaleness()
	if m.Phase() == PhaseGameOver {
		t.Error("dispatcher-routed event did not refresh the staleness clock: CheckStaleness force-ended a live game")
	}
}

// Audit M1 regression: a deliberately dropped (phase-gated) event still
// represents live log activity and must refresh the staleness clock.
func TestStalenessDroppedEventRefreshesClock(t *testing.T) {
	m, p := newProc()
	setupGame(p)
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "Moch#1358",
		Tags:       map[string]string{"TURN": "1"},
	})

	disp := NewActionDispatcher(
		p.AsRecruitVisitor(), p.AsCombatVisitor(), p.AsTransitionVisitor(), p.Touch)

	p.lastEventTime = time.Now().Add(-4 * time.Minute)
	if err := disp.Dispatch(action.Drop); err != nil {
		t.Fatalf("dispatch(Drop): %v", err)
	}

	p.CheckStaleness()
	if m.Phase() == PhaseGameOver {
		t.Error("dropped event did not refresh the staleness clock: CheckStaleness force-ended a live game")
	}
}

// Audit M2 regression: BACON_FREE_REFRESH_COUNT during COMBAT is deliberately
// gated by the builder. Pre-fix, the builder returned nil for the out-of-phase
// tag and the daemon fell through to Processor.Handle, whose case has NO phase
// guard — so the "gated" event leaked into the counter via the legacy path.
// With the routing contract, the builder returns action.Drop and the event is
// consumed without touching state on EITHER path.
func TestRoutingFreeRefreshDuringCombatNotCounted(t *testing.T) {
	m, p := newProc()
	setupGame(p)
	bldr := builder.New(card.NewFuncCatalog(CardName))
	disp := NewActionDispatcher(
		p.AsRecruitVisitor(), p.AsCombatVisitor(), p.AsTransitionVisitor(), p.Touch)

	freeRefresh := func(v string) parser.GameEvent {
		return parser.GameEvent{
			Type:       parser.EventTagChange,
			PlayerID:   7,
			EntityName: "Moch#1358",
			Tags:       map[string]string{"BACON_FREE_REFRESH_COUNT": v},
		}
	}
	gameEntityTurn := func(turn string) parser.GameEvent {
		return parser.GameEvent{
			Type: parser.EventTurnStart,
			Tags: map[string]string{"TURN": turn},
		}
	}

	// Recruit turn: the counter must work normally through the dispatcher path
	// (guards against over-aggressive drop classification breaking migrated tags).
	daemonRoute(t, bldr, disp, p, gameEntityTurn("1"))
	daemonRoute(t, bldr, disp, p, freeRefresh("2"))
	ac := findAbilityCounter(m, CatFreeRefresh)
	if ac == nil || ac.Value != 2 {
		t.Fatalf("recruit-phase free refresh should set counter to 2, got %+v", ac)
	}

	// Combat turn: the same tag must NOT update the counter through either path.
	daemonRoute(t, bldr, disp, p, gameEntityTurn("2"))
	if m.Phase() != PhaseCombat {
		t.Fatalf("expected COMBAT after even GameEntity turn, got %s", m.Phase())
	}
	daemonRoute(t, bldr, disp, p, freeRefresh("9"))
	ac = findAbilityCounter(m, CatFreeRefresh)
	if ac == nil || ac.Value != 2 {
		t.Errorf("combat-phase BACON_FREE_REFRESH_COUNT leaked through to the counter: got %+v, want value 2", ac)
	}
}

// Audit M2 regression: PROPOSED_ATTACKER outside combat is deliberately gated
// by the builder. Pre-fix, the nil build result fell through to Handle (whose
// case has no phase guard) and perturbed the win/loss streak machinery by
// setting pendingHeroAttackerID.
func TestRoutingProposedAttackerOutsideCombatDropped(t *testing.T) {
	_, p := newProc()
	setupGame(p)
	bldr := builder.New(card.NewFuncCatalog(CardName))
	disp := NewActionDispatcher(
		p.AsRecruitVisitor(), p.AsCombatVisitor(), p.AsTransitionVisitor(), p.Touch)

	// Recruit phase.
	daemonRoute(t, bldr, disp, p, parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "1"},
	})

	// Register a hero entity so a leaked event would be observable.
	p.heroEntities[99] = true

	daemonRoute(t, bldr, disp, p, parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityName: "GameEntity",
		Tags:       map[string]string{"PROPOSED_ATTACKER": "99"},
	})

	if p.pendingHeroAttackerID != 0 {
		t.Errorf("recruit-phase PROPOSED_ATTACKER leaked through to Handle: pendingHeroAttackerID=%d, want 0", p.pendingHeroAttackerID)
	}
}

// parseLogFile runs a Power.log through the parser and processor, returning final state.
func parseLogFile(t *testing.T, path string) BGGameState {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("test fixture not available: %v", err)
	}
	defer f.Close()

	ch := make(chan parser.GameEvent, 256)
	p := parser.New(ch)
	m := New()
	proc := NewProcessor(m)

	done := make(chan struct{})
	go func() {
		for e := range ch {
			proc.Handle(e)
		}
		close(done)
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		p.Feed(scanner.Text())
	}
	p.Flush()
	close(ch)
	<-done
	return m.State()
}

func TestIntegrationDuosPunishLeavers(t *testing.T) {
	s := parseLogFile(t, "testdata/duos_punish_leavers.txt")

	if !s.IsDuos {
		t.Error("expected IsDuos=true")
	}
	if s.Player.Name == "" {
		t.Error("expected non-empty player name")
	}
	// PUNISH_LEAVERS log may not have BACON_DUO_TEAMMATE_PLAYER_ID,
	// but should still detect duos mode.
	t.Logf("Player: %s, IsDuos: %v, Partner: %+v", s.Player.Name, s.IsDuos, s.Partner)
}

func TestPartnerBoardMaxSeven(t *testing.T) {
	m := New()
	m.GameStart("test", time.Now())
	m.SetDuosMode(true)

	// Try to set 12 minions
	minions := make([]MinionState, 12)
	for i := range minions {
		minions[i] = MinionState{EntityID: 100 + i, Name: fmt.Sprintf("Minion%d", i), Attack: 10, Health: 10}
	}
	m.SetPartnerBoard(minions, 5)

	s := m.State()
	if len(s.PartnerBoard.Minions) > 7 {
		t.Errorf("expected max 7 partner minions, got %d", len(s.PartnerBoard.Minions))
	}
}

func TestPartnerBoardCombatSpawnFiltering(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p)

	// Set up partner hero
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 146, CardID: "BG32_HERO_002",
		EntityName: "Buttons",
		Tags: map[string]string{"CONTROLLER": "15", "CARDTYPE": "HERO", "PLAYER_ID": "8", "HEALTH": "30", "ZONE": "PLAY"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"TURN": "5"},
	})

	// Partner combat starts
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "8"},
	})
	// Partner hero copy
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 700, CardID: "BG32_HERO_002",
		Tags: map[string]string{"CONTROLLER": "7", "CARDTYPE": "HERO", "HEALTH": "30", "ZONE": "PLAY"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 700,
		Tags: map[string]string{"PLAYER_ID": "8"},
	})

	// 7 initial board minions
	for i := 0; i < 7; i++ {
		p.Handle(parser.GameEvent{
			Type: parser.EventEntityUpdate, EntityID: 701 + i, CardID: "BGS_119",
			EntityName: fmt.Sprintf("Minion%d", i),
			Tags: map[string]string{
				"CONTROLLER": "7", "CARDTYPE": "MINION",
				"ATK": fmt.Sprintf("%d", 50+i*10), "HEALTH": fmt.Sprintf("%d", 40+i*10),
				"ZONE": "PLAY", "ZONE_POSITION": fmt.Sprintf("%d", i+1),
			},
		})
	}

	// First combat action — signals board setup is complete
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1,
		EntityName: "GameEntity",
		Tags: map[string]string{"PROPOSED_ATTACKER": "701"},
	})

	// Mid-combat spawns (deathrattle tokens) — should be filtered out
	// because they arrive after PROPOSED_ATTACKER
	for i := 0; i < 3; i++ {
		p.Handle(parser.GameEvent{
			Type: parser.EventEntityUpdate, EntityID: 750 + i, CardID: "BG34_Giant_584",
			EntityName: "Timewarped Kil'rek",
			Tags: map[string]string{
				"CONTROLLER": "7", "CARDTYPE": "MINION",
				"ATK": "4", "HEALTH": "7",
				"ZONE": "PLAY", "ZONE_POSITION": "1",
			},
		})
	}

	// Combat ends
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "7"},
	})

	s := m.State()
	if s.PartnerBoard == nil {
		t.Fatal("expected PartnerBoard != nil")
	}
	if len(s.PartnerBoard.Minions) != 7 {
		t.Errorf("expected 7 partner minions (no combat spawns), got %d", len(s.PartnerBoard.Minions))
	}
	// Verify we kept the initial board minions (buffed stats), not combat spawns (base stats)
	for i, mn := range s.PartnerBoard.Minions {
		if mn.Attack < 50 {
			t.Errorf("minion[%d] has base stats (atk=%d), expected buffed initial board minion", i, mn.Attack)
		}
	}
}

func TestPartnerBoardSmallBoardNoSpawns(t *testing.T) {
	// A partner with <7 minions should not include combat spawns.
	// This verifies the PROPOSED_ATTACKER cutoff works even when
	// a simple count-based cap would fail.
	m, p := newProc()
	setupDuosGame(p)

	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 146, CardID: "BG32_HERO_002",
		EntityName: "Buttons",
		Tags: map[string]string{"CONTROLLER": "15", "CARDTYPE": "HERO", "PLAYER_ID": "8", "HEALTH": "30", "ZONE": "PLAY"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"TURN": "3"},
	})

	// Partner combat starts
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "8"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 700, CardID: "BG32_HERO_002",
		Tags: map[string]string{"CONTROLLER": "7", "CARDTYPE": "HERO", "HEALTH": "30", "ZONE": "PLAY"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 700,
		Tags: map[string]string{"PLAYER_ID": "8"},
	})

	// Only 3 initial board minions (early game)
	for i := 0; i < 3; i++ {
		p.Handle(parser.GameEvent{
			Type: parser.EventEntityUpdate, EntityID: 701 + i, CardID: "BGS_119",
			EntityName: fmt.Sprintf("Minion%d", i),
			Tags: map[string]string{
				"CONTROLLER": "7", "CARDTYPE": "MINION",
				"ATK": fmt.Sprintf("%d", 20+i*5), "HEALTH": fmt.Sprintf("%d", 15+i*5),
				"ZONE": "PLAY", "ZONE_POSITION": fmt.Sprintf("%d", i+1),
			},
		})
	}

	// Combat action fires
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1,
		EntityName: "GameEntity",
		Tags: map[string]string{"PROPOSED_ATTACKER": "701"},
	})

	// Deathrattle spawn — should NOT be collected
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 750, CardID: "BG34_Giant_584",
		EntityName: "Timewarped Kil'rek",
		Tags: map[string]string{
			"CONTROLLER": "7", "CARDTYPE": "MINION",
			"ATK": "4", "HEALTH": "7",
			"ZONE": "PLAY", "ZONE_POSITION": "1",
		},
	})

	// Combat ends
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "7"},
	})

	s := m.State()
	if s.PartnerBoard == nil {
		t.Fatal("expected PartnerBoard != nil")
	}
	if len(s.PartnerBoard.Minions) != 3 {
		t.Errorf("expected 3 partner minions (no spawns), got %d", len(s.PartnerBoard.Minions))
	}
	for i, mn := range s.PartnerBoard.Minions {
		if mn.Attack < 20 {
			t.Errorf("minion[%d] has base stats (atk=%d), expected initial board minion", i, mn.Attack)
		}
	}
}

func TestIntegrationDuosWithTeammateID(t *testing.T) {
	s := parseLogFile(t, "testdata/duos_with_teammate_id.txt")

	if !s.IsDuos {
		t.Error("expected IsDuos=true")
	}
	if s.Partner == nil {
		t.Fatal("expected Partner != nil")
	}
	if s.Partner.HeroCardID == "" {
		t.Error("expected non-empty partner HeroCardID")
	}
	t.Logf("Player: %s, Partner: %s (hero: %s)", s.Player.Name, s.Partner.Name, s.Partner.HeroCardID)
}

func TestPartnerBeetleBuffRoutedToPartnerSources(t *testing.T) {
	m, p := newProc()
	setupGame(p) // local player ID=7

	// Enable duos with partner ID=15 (the dummy player from setupGame).
	p.isDuos = true
	p.partnerPlayerID = 15

	// Partner beetle Dnt enchantment (controller=15).
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 500,
		CardID:   "BG31_808pe",
		Tags: map[string]string{
			"CONTROLLER":            "15",
			"CARDTYPE":              "ENCHANTMENT",
			"ATTACHED":              "8888",
			"TAG_SCRIPT_DATA_NUM_1": "5",
			"TAG_SCRIPT_DATA_NUM_2": "3",
		},
	})

	state := m.State()

	// Local should have no beetle buffs.
	for _, bs := range state.BuffSources {
		if bs.Category == "BEETLE" {
			t.Errorf("partner beetle leaked to local: ATK=%d HP=%d", bs.Attack, bs.Health)
		}
	}

	// Partner should have beetle buffs.
	found := false
	for _, bs := range state.PartnerBuffSources {
		if bs.Category == "BEETLE" {
			found = true
			if bs.Attack != 6 || bs.Health != 4 { // 5+1 base, 3+1 base
				t.Errorf("wrong partner beetle values: ATK=%d HP=%d, want 6/4", bs.Attack, bs.Health)
			}
		}
	}
	if !found {
		t.Error("expected partner beetle buff source but found none")
	}
}

func TestCombatBeetleDoesNotLeakToLocal(t *testing.T) {
	m, p := newProc()
	setupGame(p) // local player ID=7

	// Opponent beetle Dnt enchantment created (controller=15, not local).
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 9999,
		CardID:   "BG31_808pe",
		Tags: map[string]string{
			"CONTROLLER":            "15",
			"CARDTYPE":              "ENCHANTMENT",
			"ATTACHED":              "8888",
			"TAG_SCRIPT_DATA_NUM_1": "5",
			"TAG_SCRIPT_DATA_NUM_2": "3",
		},
	})

	// TAG_CHANGE on the same entity with SD values (simulates late arrival).
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 9999,
		Tags:     map[string]string{"TAG_SCRIPT_DATA_NUM_1": "10"},
	})

	state := m.State()
	for _, bs := range state.BuffSources {
		if bs.Category == "BEETLE" {
			t.Errorf("opponent beetle buff leaked to local: ATK=%d HP=%d", bs.Attack, bs.Health)
		}
	}
}

func TestDuosBotAttachedOpponentDntDoesNotLeakToLocal(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p) // local=PlayerID 7 (EntityID 20), bot=PlayerID 15 (EntityID 22)

	// Opponent's beetle Dnt enchantment: CONTROLLER=15 (bot), ATTACHED=22 (bot entity).
	// In duos, opponent combat copy Dnt enchantments are attached to the bot entity.
	// These must NOT be treated as local.
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 22684,
		CardID:   "BG31_808pe",
		Tags: map[string]string{
			"CONTROLLER":            "15",
			"CARDTYPE":              "ENCHANTMENT",
			"ATTACHED":              "22",
			"ZONE":                  "PLAY",
			"TAG_SCRIPT_DATA_NUM_1": "26",
			"TAG_SCRIPT_DATA_NUM_2": "13",
		},
	})

	// TAG_CHANGE updating the beetle Dnt values (combat progression).
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 22684,
		Tags:     map[string]string{"TAG_SCRIPT_DATA_NUM_1": "30"},
	})
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 22684,
		Tags:     map[string]string{"TAG_SCRIPT_DATA_NUM_2": "15"},
	})

	state := m.State()
	for _, bs := range state.BuffSources {
		if bs.Category == "BEETLE" {
			t.Errorf("opponent beetle Dnt leaked to local via bot entity: ATK=%d HP=%d", bs.Attack, bs.Health)
		}
	}
}

func TestDuosLocalPlayerDntStillTracked(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p) // local=PlayerID 7 (EntityID 20), bot=PlayerID 15 (EntityID 22)

	// Local player's beetle Dnt: CONTROLLER=7 (local), ATTACHED=20 (local player entity).
	p.Handle(parser.GameEvent{
		Type:     parser.EventEntityUpdate,
		EntityID: 500,
		CardID:   "BG31_808pe",
		Tags: map[string]string{
			"CONTROLLER":            "7",
			"CARDTYPE":              "ENCHANTMENT",
			"ATTACHED":              "20",
			"ZONE":                  "PLAY",
			"TAG_SCRIPT_DATA_NUM_1": "5",
			"TAG_SCRIPT_DATA_NUM_2": "3",
		},
	})

	state := m.State()
	found := false
	for _, bs := range state.BuffSources {
		if bs.Category == "BEETLE" {
			found = true
			// Beetle: value + base offset (1 ATK, 1 HP) = 5+1=6 ATK, 3+1=4 HP
			if bs.Attack != 6 || bs.Health != 4 {
				t.Errorf("wrong beetle values: ATK=%d HP=%d, want 6/4", bs.Attack, bs.Health)
			}
		}
	}
	if !found {
		t.Error("local player beetle Dnt not tracked after fix")
	}
}

func TestDuosBeetleDntSplitByPartnerCombat(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p) // local=PlayerID 7, partner=PlayerID 8, bot=PlayerID 15

	// Set up local hero entity 33 (needed for hero entity tracking).
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 33, CardID: "TB_BaconShop_HERO_49",
		Tags: map[string]string{"CONTROLLER": "7", "CARDTYPE": "HERO", "HEALTH": "40", "ZONE": "PLAY"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"HERO_ENTITY": "33"},
	})

	// SHOW_ENTITY: beetle Dnt combat copy arrives (before combat starts).
	// SD1=20, SD2=16 — this is the team total from prior rounds.
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 500, CardID: "BG31_808pe",
		Tags: map[string]string{
			"CONTROLLER": "7", "CARDTYPE": "ENCHANTMENT",
			"ATTACHED": "20", "ZONE": "PLAY",
			"TAG_SCRIPT_DATA_NUM_1": "20", "TAG_SCRIPT_DATA_NUM_2": "16",
		},
	})

	// Local combat starts (no beetle changes during local combat).
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "7"},
	})

	// Partner combat starts.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "8"},
	})

	// Beetle SD updates during partner combat (partner's beetles dying).
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500,
		Tags: map[string]string{"TAG_SCRIPT_DATA_NUM_1": "24"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500,
		Tags: map[string]string{"TAG_SCRIPT_DATA_NUM_2": "20"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500,
		Tags: map[string]string{"TAG_SCRIPT_DATA_NUM_1": "26"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 500,
		Tags: map[string]string{"TAG_SCRIPT_DATA_NUM_2": "22"},
	})

	// Combat ends.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "0"},
	})

	state := m.State()

	// Local should have the pre-partner-combat value: 20+1=21 ATK, 16+1=17 HP.
	localFound := false
	for _, bs := range state.BuffSources {
		if bs.Category == "BEETLE" {
			localFound = true
			if bs.Attack != 21 || bs.Health != 17 {
				t.Errorf("local beetle: got +%d/+%d, want +21/+17", bs.Attack, bs.Health)
			}
		}
	}
	if !localFound {
		t.Error("expected local beetle buff source")
	}

	// Partner should have the delta: (26-20)=6 ATK, (22-16)=6 HP.
	partnerFound := false
	for _, bs := range state.PartnerBuffSources {
		if bs.Category == "BEETLE" {
			partnerFound = true
			if bs.Attack != 6 || bs.Health != 6 {
				t.Errorf("partner beetle: got +%d/+%d, want +6/+6", bs.Attack, bs.Health)
			}
		}
	}
	if !partnerFound {
		t.Error("expected partner beetle buff source")
	}
}

func TestDuosBeetleDntMixedContributions(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p) // local=7, partner=8

	// SHOW_ENTITY: beetle Dnt combat copy with SD1=10, SD2=8 (prior total).
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 600, CardID: "BG31_808pe",
		Tags: map[string]string{
			"CONTROLLER": "7", "CARDTYPE": "ENCHANTMENT",
			"ATTACHED": "20", "ZONE": "PLAY",
			"TAG_SCRIPT_DATA_NUM_1": "10", "TAG_SCRIPT_DATA_NUM_2": "8",
		},
	})

	// Local combat starts.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "7"},
	})

	// Beetle SD increments during local combat (local beetles dying).
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 600,
		Tags: map[string]string{"TAG_SCRIPT_DATA_NUM_1": "14"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 600,
		Tags: map[string]string{"TAG_SCRIPT_DATA_NUM_2": "10"},
	})

	// Partner combat starts.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "8"},
	})

	// Beetle SD increments during partner combat (partner beetles dying).
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 600,
		Tags: map[string]string{"TAG_SCRIPT_DATA_NUM_1": "20"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 600,
		Tags: map[string]string{"TAG_SCRIPT_DATA_NUM_2": "16"},
	})

	// Combat ends.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 20, PlayerID: 7,
		EntityName: "LocalPlayer#1234",
		Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "0"},
	})

	state := m.State()

	// Local: initial 10 + local delta 4 = 14 raw, +1 base = 15 ATK.
	//        initial 8 + local delta 2 = 10 raw, +1 base = 11 HP.
	for _, bs := range state.BuffSources {
		if bs.Category == "BEETLE" {
			if bs.Attack != 15 || bs.Health != 11 {
				t.Errorf("local beetle: got +%d/+%d, want +15/+11", bs.Attack, bs.Health)
			}
		}
	}

	// Partner: delta during partner combat = (20-14)=6 ATK, (16-10)=6 HP.
	for _, bs := range state.PartnerBuffSources {
		if bs.Category == "BEETLE" {
			if bs.Attack != 6 || bs.Health != 6 {
				t.Errorf("partner beetle: got +%d/+%d, want +6/+6", bs.Attack, bs.Health)
			}
		}
	}
}

func TestSoloBeetleDntUnchanged(t *testing.T) {
	m, p := newProc()
	setupGame(p) // solo game, local=PlayerID 7

	// Beetle Dnt enchantment.
	p.Handle(parser.GameEvent{
		Type: parser.EventEntityUpdate, EntityID: 700, CardID: "BG31_808pe",
		Tags: map[string]string{
			"CONTROLLER": "7", "CARDTYPE": "ENCHANTMENT",
			"ATTACHED": "20", "ZONE": "PLAY",
			"TAG_SCRIPT_DATA_NUM_1": "10", "TAG_SCRIPT_DATA_NUM_2": "8",
		},
	})

	// TAG_CHANGE update.
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 700,
		Tags: map[string]string{"TAG_SCRIPT_DATA_NUM_1": "14"},
	})

	state := m.State()
	for _, bs := range state.BuffSources {
		if bs.Category == "BEETLE" {
			// 14+1=15 ATK, 8+1=9 HP
			if bs.Attack != 15 || bs.Health != 9 {
				t.Errorf("solo beetle: got +%d/+%d, want +15/+9", bs.Attack, bs.Health)
			}
			return
		}
	}
	t.Error("expected beetle buff source in solo game")
}

func TestReconnectStashFieldExists(t *testing.T) {
	m := New()
	p := NewProcessor(m)
	if p.reconnectStash != nil {
		t.Error("expected nil reconnectStash on fresh processor")
	}
	if p.isReconnect {
		t.Error("expected isReconnect=false on fresh processor")
	}
}

func TestReconnectStashPopulated(t *testing.T) {
	m := New()
	p := NewProcessor(m)

	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 19, 46, 45, 0, time.UTC)})
	m.SetTurn(5)
	m.SetTavernTier(3)
	m.UpdateHeroCardID("BG25_HERO_103_SKIN_D")

	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 20, 6, 46, 0, time.UTC)})

	if p.reconnectStash == nil {
		t.Fatal("expected reconnectStash to be populated after second EventGameStart")
	}
	if p.reconnectStash.heroCardID != "BG25_HERO_103_SKIN_D" {
		t.Errorf("stashed heroCardID: expected BG25_HERO_103_SKIN_D, got %q", p.reconnectStash.heroCardID)
	}
	if p.reconnectStash.turn != 5 {
		t.Errorf("stashed turn: expected 5, got %d", p.reconnectStash.turn)
	}
	if p.reconnectStash.tavernTier != 3 {
		t.Errorf("stashed tavernTier: expected 3, got %d", p.reconnectStash.tavernTier)
	}
}

func TestReconnectDetectionAndRestore(t *testing.T) {
	m := New()
	p := NewProcessor(m)

	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 19, 46, 45, 0, time.UTC)})
	m.SetTurn(9)
	m.SetTavernTier(4)
	m.UpdateHeroCardID("BG25_HERO_103_SKIN_D")
	origGameID := m.State().GameID

	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 20, 6, 46, 0, time.UTC)})
	p.Handle(parser.GameEvent{
		Type: parser.EventGameEntityTags,
		Tags: map[string]string{"STATE": "RUNNING", "TURN": "19"},
	})

	s := m.State()
	if s.GameID != origGameID {
		t.Errorf("GameID: expected %q, got %q", origGameID, s.GameID)
	}
	if s.Player.HeroCardID != "BG25_HERO_103_SKIN_D" {
		t.Errorf("HeroCardID: expected BG25_HERO_103_SKIN_D, got %q", s.Player.HeroCardID)
	}
	if s.TavernTier != 4 {
		t.Errorf("TavernTier: expected 4, got %d", s.TavernTier)
	}
	if !p.isReconnect {
		t.Error("expected isReconnect=true after reconnect detection")
	}
	if p.reconnectStash != nil {
		t.Error("expected reconnectStash to be nil after restore")
	}
}

func TestReconnectFlagClearedOnNewGame(t *testing.T) {
	m := New()
	p := NewProcessor(m)

	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 19, 0, 0, 0, time.UTC)})
	m.SetTurn(5)
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 19, 30, 0, 0, time.UTC)})
	p.Handle(parser.GameEvent{Type: parser.EventGameEntityTags, Tags: map[string]string{"STATE": "RUNNING", "TURN": "10"}})
	if !p.isReconnect {
		t.Fatal("should be reconnect")
	}
	p.Handle(parser.GameEvent{Type: parser.EventGameEnd, Timestamp: time.Date(2026, 3, 24, 20, 0, 0, 0, time.UTC), Tags: map[string]string{}})
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 20, 5, 0, 0, time.UTC)})
	p.Handle(parser.GameEvent{Type: parser.EventGameEntityTags, Tags: map[string]string{"STATE": "RUNNING", "TURN": "1"}})
	if p.isReconnect {
		t.Error("isReconnect should be false for fresh game")
	}
}

func TestFreshGameNotTreatedAsReconnect(t *testing.T) {
	m := New()
	p := NewProcessor(m)

	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 19, 0, 0, 0, time.UTC)})
	p.Handle(parser.GameEvent{
		Type: parser.EventGameEntityTags,
		Tags: map[string]string{"STATE": "RUNNING", "TURN": "1"},
	})

	if p.isReconnect {
		t.Error("fresh game with TURN=1 should not be treated as reconnect")
	}
	if p.reconnectStash != nil {
		t.Error("stash should be cleared after GameEntityTags")
	}
}

func TestNoStashFromIdlePhase(t *testing.T) {
	m := New()
	p := NewProcessor(m)

	// First EventGameStart from idle — should NOT stash (no prior game state)
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 19, 0, 0, 0, time.UTC)})
	if p.reconnectStash != nil {
		t.Error("should not stash when starting from idle phase")
	}
}

// ── MinionsSold counter tests ─────────────────────────────────────────────────

// setupRecruitPhase brings the processor into recruit phase (GameEntity turn 1)
// with the local player identified. Returns the machine and processor.
func setupRecruitPhase(t *testing.T) (*Machine, *Processor) {
	t.Helper()
	m, p := newProc()
	setupGame(p)
	// Advance to recruit phase via GameEntity TURN=1 (odd = recruit).
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "1"},
	})
	if m.State().Phase != PhaseRecruit {
		t.Fatalf("expected RECRUIT phase after EventTurnStart TURN=1, got %s", m.State().Phase)
	}
	return m, p
}

// minionSold fires OnMinionSold directly with the given entity ID.
func minionSold(p *Processor, entityID int) {
	_ = p.OnMinionSold(&action.MinionSoldAction{
		ActionBase: action.ActionBase{Entity: action.EntityID(entityID)},
	})
}

// TestMinionsSoldBasicIncrement verifies that one sell increments the counter to 1.
func TestMinionsSoldBasicIncrement(t *testing.T) {
	m, p := setupRecruitPhase(t)
	m.UpsertMinion(MinionState{EntityID: 300, Name: "Murloc", Attack: 1, Health: 1})

	minionSold(p, 300)

	ac := findAbilityCounter(m, "MINIONS_SOLD")
	if ac == nil {
		t.Fatal("expected MINIONS_SOLD ability counter to be set after one sell, got nil")
	}
	if ac.Value != 1 {
		t.Errorf("expected MINIONS_SOLD=1 after one sell, got %d", ac.Value)
	}
	if ac.Display != "1" {
		t.Errorf("expected display=%q, got %q", "1", ac.Display)
	}
}

// TestMinionsSoldMultipleSells verifies that 3 sells produce a counter value of 3.
func TestMinionsSoldMultipleSells(t *testing.T) {
	m, p := setupRecruitPhase(t)
	m.UpsertMinion(MinionState{EntityID: 301, Name: "A", Attack: 1, Health: 1})
	m.UpsertMinion(MinionState{EntityID: 302, Name: "B", Attack: 2, Health: 2})
	m.UpsertMinion(MinionState{EntityID: 303, Name: "C", Attack: 3, Health: 3})

	minionSold(p, 301)
	minionSold(p, 302)
	minionSold(p, 303)

	ac := findAbilityCounter(m, "MINIONS_SOLD")
	if ac == nil {
		t.Fatal("expected MINIONS_SOLD ability counter after 3 sells, got nil")
	}
	if ac.Value != 3 {
		t.Errorf("expected MINIONS_SOLD=3 after 3 sells, got %d", ac.Value)
	}
}

// TestMinionsSoldResetOnNewGame verifies that the counter is cleared when a new game starts.
func TestMinionsSoldResetOnNewGame(t *testing.T) {
	m, p := setupRecruitPhase(t)
	m.UpsertMinion(MinionState{EntityID: 401, Name: "X", Attack: 1, Health: 1})
	m.UpsertMinion(MinionState{EntityID: 402, Name: "Y", Attack: 2, Health: 2})

	minionSold(p, 401)
	minionSold(p, 402)

	ac := findAbilityCounter(m, "MINIONS_SOLD")
	if ac == nil || ac.Value != 2 {
		t.Fatalf("pre-condition: expected MINIONS_SOLD=2, got %v", ac)
	}

	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Time{}})

	s := m.State()
	for _, counter := range s.AbilityCounters {
		if counter.Category == "MINIONS_SOLD" {
			t.Errorf("expected MINIONS_SOLD counter absent after new game, got value=%d", counter.Value)
		}
	}
}

// TestMinionsSoldCombatDeathIgnored verifies that minion deaths during combat do not count.
func TestMinionsSoldCombatDeathIgnored(t *testing.T) {
	m, p := setupRecruitPhase(t)
	m.UpsertMinion(MinionState{EntityID: 500, Name: "Fighter", Attack: 3, Health: 3})

	// Transition to combat phase via even GameEntity TURN.
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "2"},
	})
	if m.State().Phase != PhaseCombat {
		t.Skipf("could not enter combat phase, skipping combat guard test")
	}

	minionSold(p, 500)

	ac := findAbilityCounter(m, "MINIONS_SOLD")
	if ac != nil && ac.Value > 0 {
		t.Errorf("expected MINIONS_SOLD=0 after combat death, got %d", ac.Value)
	}
}

// TestMinionsSoldNonBoardEntityIgnored verifies that zone changes for entities not on the
// local board (opponent minions, tavern minions, etc.) do not count as sells.
func TestMinionsSoldNonBoardEntityIgnored(t *testing.T) {
	_, p := setupRecruitPhase(t)

	// Fire OnMinionSold for entity 9999, which was never added to the local board.
	minionSold(p, 9999)

	// No board minion exists for 9999, so counter must remain absent/zero.
	m, _ := setupRecruitPhase(t) // fresh machine for reading (p's machine already modified)
	_ = m
	// Read from the same processor's machine via the action result.
	// Re-derive by calling OnMinionSold again with a fresh machine.
	// Simpler: just directly check via the processor's machine exposed by newProc.
	m2, p2 := newProc()
	setupGame(p2)
	p2.Handle(parser.GameEvent{Type: parser.EventTurnStart, Tags: map[string]string{"TURN": "1"}})
	minionSold(p2, 9999) // not on board
	ac := findAbilityCounter(m2, "MINIONS_SOLD")
	if ac != nil && ac.Value > 0 {
		t.Errorf("expected MINIONS_SOLD=0 for non-board entity, got %d", ac.Value)
	}
}

// tripleFormed fires OnPlayerTriplesChanged for the local player (entity 20, controller 7,
// name "Moch#1358" as set by setupGame) with the given cumulative triple count.
// This clears the tripleFormationActive gate opened by tripleEnchantGraveyard.
func tripleFormed(p *Processor, cumulativeTriples int) {
	_ = p.OnPlayerTriplesChanged(&action.PlayerTriplesChangedAction{
		ActionBase:   action.ActionBase{Entity: action.EntityID(20)},
		Value:        cumulativeTriples,
		ControllerID: 7,
		EntityName:   "Moch#1358",
	})
}

// tripleEnchantGraveyard simulates the TB_BaconShop_3ofKindChecke enchantment going
// GRAVEYARD (the first event in a triple sequence). Sets tripleFormationActive on the
// processor, which blocks subsequent MINIONS_SOLD increments until tripleFormed fires.
func tripleEnchantGraveyard(p *Processor, entityID int) {
	p.entityProps[entityID] = &entityInfo{CardID: "TB_BaconShop_3ofKindChecke"}
	_ = p.OnMinionSold(&action.MinionSoldAction{
		ActionBase: action.ActionBase{Entity: action.EntityID(entityID)},
	})
}

// TestMinionsSoldTriple_AllOnBoard verifies that a 2-board-minion triple is not counted
// as sells: the enchantment gate blocks both PLAY→SETASIDE board removals.
func TestMinionsSoldTriple_AllOnBoard(t *testing.T) {
	m, p := setupRecruitPhase(t)

	m.UpsertMinion(MinionState{EntityID: 201, Name: "Murloc A", Attack: 2, Health: 1})
	m.UpsertMinion(MinionState{EntityID: 202, Name: "Murloc B", Attack: 2, Health: 1})

	// Enchant fires first → gate opens.
	tripleEnchantGraveyard(p, 200)

	// Both board minions move PLAY→SETASIDE — gate suppresses counter increments.
	minionSold(p, 201)
	minionSold(p, 202)

	// PLAYER_TRIPLES fires → gate clears.
	tripleFormed(p, 1)

	ac := findAbilityCounter(m, "MINIONS_SOLD")
	if ac != nil {
		t.Errorf("expected MINIONS_SOLD absent after all-board triple, got value=%d", ac.Value)
	}
}

// TestMinionsSoldTriple_WithPriorSells verifies that a legitimate sell before the triple
// is preserved: the gate only suppresses the board removals during formation.
func TestMinionsSoldTriple_WithPriorSells(t *testing.T) {
	m, p := setupRecruitPhase(t)

	m.UpsertMinion(MinionState{EntityID: 301, Name: "Murloc A", Attack: 2, Health: 1})
	m.UpsertMinion(MinionState{EntityID: 302, Name: "Murloc B", Attack: 2, Health: 1})
	m.UpsertMinion(MinionState{EntityID: 303, Name: "Murloc C", Attack: 2, Health: 1})

	// One legitimate sell before the triple.
	minionSold(p, 301)

	// Enchant fires → gate opens. Two board removals suppressed.
	tripleEnchantGraveyard(p, 300)
	minionSold(p, 302)
	minionSold(p, 303)

	tripleFormed(p, 1)

	ac := findAbilityCounter(m, "MINIONS_SOLD")
	if ac == nil {
		t.Fatal("expected MINIONS_SOLD=1 after triple (real sell preserved), got nil")
	}
	if ac.Value != 1 {
		t.Errorf("expected MINIONS_SOLD=1 after triple, got %d", ac.Value)
	}
	if ac.Display != "1" {
		t.Errorf("expected display=%q, got %q", "1", ac.Display)
	}
}

// TestMinionsSoldTriple_HandOnly verifies that a triple where all 3 copies are in hand
// (0 board removals) does not corrupt the counter with a retroactive negative correction.
func TestMinionsSoldTriple_HandOnly(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Sell 2 real minions before the triple.
	m.UpsertMinion(MinionState{EntityID: 501, Name: "Rat", Attack: 1, Health: 1})
	m.UpsertMinion(MinionState{EntityID: 502, Name: "Rat", Attack: 1, Health: 1})
	minionSold(p, 501)
	minionSold(p, 502)

	// Hand-only triple: enchant fires, no board removals, PLAYER_TRIPLES fires.
	tripleEnchantGraveyard(p, 500)
	tripleFormed(p, 1)

	// The 2 real sells must be preserved — no correction should have fired.
	ac := findAbilityCounter(m, "MINIONS_SOLD")
	if ac == nil {
		t.Fatal("expected MINIONS_SOLD=2 after hand-only triple, got nil")
	}
	if ac.Value != 2 {
		t.Errorf("expected MINIONS_SOLD=2 after hand-only triple, got %d", ac.Value)
	}
}

// TestMinionsSoldTriple_MixedBoard verifies a 1-board-2-hand triple: only one SETASIDE
// fires from the board, but the gate suppresses it so no false positive accumulates.
func TestMinionsSoldTriple_MixedBoard(t *testing.T) {
	m, p := setupRecruitPhase(t)

	m.UpsertMinion(MinionState{EntityID: 601, Name: "Rat", Attack: 1, Health: 1})

	// Enchant → gate opens. One board removal suppressed. Two hand copies don't fire OnMinionSold.
	tripleEnchantGraveyard(p, 600)
	minionSold(p, 601)
	tripleFormed(p, 1)

	ac := findAbilityCounter(m, "MINIONS_SOLD")
	if ac != nil {
		t.Errorf("expected MINIONS_SOLD absent after 1-board-2-hand triple, got value=%d", ac.Value)
	}
}

// TestMinionsSoldTriple_FlagClearedAfterTriple verifies that sells after a triple are
// counted normally once tripleFormed clears the gate.
func TestMinionsSoldTriple_FlagClearedAfterTriple(t *testing.T) {
	m, p := setupRecruitPhase(t)

	m.UpsertMinion(MinionState{EntityID: 701, Name: "Rat A", Attack: 1, Health: 1})
	m.UpsertMinion(MinionState{EntityID: 702, Name: "Rat B", Attack: 1, Health: 1})
	m.UpsertMinion(MinionState{EntityID: 703, Name: "Rat C", Attack: 1, Health: 1})

	// Triple formation.
	tripleEnchantGraveyard(p, 700)
	minionSold(p, 701)
	minionSold(p, 702)
	tripleFormed(p, 1)

	// Now sell one more minion — gate is clear, this should count.
	m.UpsertMinion(MinionState{EntityID: 703, Name: "Rat C", Attack: 1, Health: 1})
	minionSold(p, 703)

	ac := findAbilityCounter(m, "MINIONS_SOLD")
	if ac == nil {
		t.Fatal("expected MINIONS_SOLD=1 after post-triple sell, got nil")
	}
	if ac.Value != 1 {
		t.Errorf("expected MINIONS_SOLD=1 after post-triple sell, got %d", ac.Value)
	}
}
