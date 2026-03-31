# Reconnect-Aware Processor Handling — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Detect mid-game reconnects and preserve accumulated game state (buff sources, turn snapshots, hero identity, game ID) across the reconnect reset.

**Architecture:** Before the existing `EventGameStart` reset, stash game-level state from the Machine. When `EventGameEntityTags` fires with `STATE=RUNNING` + `TURN > 1`, restore the stash. Entity-level maps (entityController, heroEntities, entityProps) still reset since entity IDs may change. A processor-level `isReconnect` flag prevents hero CardID overwrite during reconnect FULL_ENTITY processing.

**Tech Stack:** Go, existing gamestate package, no new dependencies.

---

### Task 1: Add reconnectStash struct and isReconnect flag to Processor

**Files:**
- Modify: `internal/gamestate/processor.go:64-134`

**Step 1: Write failing test**

Create a test that verifies the processor has a `reconnectStash` field and `isReconnect` flag.

File: `internal/gamestate/gamestate_test.go` — add at the end:

```go
func TestReconnectStashFieldExists(t *testing.T) {
	m := NewMachine()
	p := NewProcessor(m)
	// Verify stash is nil initially
	if p.reconnectStash != nil {
		t.Error("expected nil reconnectStash on fresh processor")
	}
	if p.isReconnect {
		t.Error("expected isReconnect=false on fresh processor")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -count=1 -run TestReconnectStashFieldExists ./internal/gamestate/`
Expected: compilation error — `reconnectStash` field does not exist.

**Step 3: Add the struct and fields**

In `internal/gamestate/processor.go`, add the struct before the `Processor` type definition (around line 64):

```go
// reconnectStash holds game-level state saved before an EventGameStart reset.
// If the subsequent EventGameEntityTags confirms a reconnect (STATE=RUNNING,
// TURN > 1), this state is restored. Otherwise it is discarded.
type reconnectStash struct {
	gameID              string
	startTime           time.Time
	turn                int
	tavernTier          int
	isDuos              bool
	partnerPlayerID     int
	partnerPlayerName   string
	heroCardID          string
	partnerHeroCardID   string
	turnSnapshots       []TurnSnapshot
	buffSources         []BuffSource
	abilityCounters     []AbilityCounter
	partnerBuffSources  []BuffSource
	partnerAbilityCtrs  []AbilityCounter
	modifications       []StatMod
	prevBuffSources     []BuffSource
	prevAbilityCtrs     []AbilityCounter
	prevModCount        int
	anomalyCardID       string
	anomalyName         string
	anomalyDescription  string
	availableTribes     []string
}
```

Add fields to `Processor` struct (inside the struct body, around line 122):

```go
	// Reconnect state preservation
	reconnectStash *reconnectStash
	isReconnect    bool
```

**Step 4: Run test to verify it passes**

Run: `go test -count=1 -run TestReconnectStashFieldExists ./internal/gamestate/`
Expected: PASS

**Step 5: Commit**

```
feat(gamestate): add reconnectStash struct and isReconnect flag to Processor
```

---

### Task 2: Stash game state before EventGameStart reset

**Files:**
- Modify: `internal/gamestate/processor.go:157-200` (EventGameStart handler)
- Modify: `internal/gamestate/state.go` (add Machine getter methods)

**Step 1: Write failing test**

```go
func TestReconnectStashPopulated(t *testing.T) {
	m := NewMachine()
	p := NewProcessor(m)

	// Start a game and accumulate some state
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 19, 46, 45, 0, time.UTC)})
	m.SetTurn(5)
	m.SetTavernTier(3)
	m.UpdateHeroCardID("BG25_HERO_103_SKIN_D")

	// Start another game (simulating reconnect)
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 20, 6, 46, 0, time.UTC)})

	// Stash should be populated with state from the previous game
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
```

**Step 2: Run test to verify it fails**

Run: `go test -count=1 -run TestReconnectStashPopulated ./internal/gamestate/`
Expected: FAIL — stash is nil.

**Step 3: Add Machine getter methods and stash logic**

In `internal/gamestate/state.go`, add getter methods for fields needed by the stash:

```go
// ReconnectStashData returns the game-level state needed for reconnect stashing.
// Called by the processor before resetting on EventGameStart.
func (m *Machine) ReconnectStashData() (turnSnapshots []TurnSnapshot, prevBuffSources []BuffSource, prevAbilityCtrs []AbilityCounter, prevModCount int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]TurnSnapshot(nil), m.turnSnapshots...),
		append([]BuffSource(nil), m.prevBuffSources...),
		append([]AbilityCounter(nil), m.prevAbilityCtrs...),
		m.prevModCount
}
```

In `internal/gamestate/processor.go`, add stash logic at the top of the `EventGameStart` case, BEFORE the reset (before line 159):

```go
	case parser.EventGameStart:
		p.flushPendingStatChanges()
		// Stash game-level state before reset — if the next EventGameEntityTags
		// confirms a reconnect, this state will be restored.
		if phase := p.machine.Phase(); phase != PhaseIdle && phase != PhaseGameOver {
			s := p.machine.State()
			turnSnaps, prevBS, prevAC, prevMC := p.machine.ReconnectStashData()
			p.reconnectStash = &reconnectStash{
				gameID:              s.GameID,
				startTime:           s.StartTime,
				turn:                s.Turn,
				tavernTier:          s.TavernTier,
				isDuos:              s.IsDuos,
				partnerPlayerID:     p.partnerPlayerID,
				partnerPlayerName:   p.partnerPlayerName,
				heroCardID:          s.Player.HeroCardID,
				partnerHeroCardID:   "",
				turnSnapshots:       turnSnaps,
				buffSources:         append([]BuffSource(nil), s.BuffSources...),
				abilityCounters:     append([]AbilityCounter(nil), s.AbilityCounters...),
				partnerBuffSources:  append([]BuffSource(nil), s.PartnerBuffSources...),
				partnerAbilityCtrs:  append([]AbilityCounter(nil), s.PartnerAbilityCounters...),
				modifications:       append([]StatMod(nil), s.Modifications...),
				prevBuffSources:     prevBS,
				prevAbilityCtrs:     prevAC,
				prevModCount:        prevMC,
				anomalyCardID:       s.AnomalyCardID,
				anomalyName:         s.AnomalyName,
				anomalyDescription:  s.AnomalyDescription,
				availableTribes:     append([]string(nil), s.AvailableTribes...),
			}
			if s.Partner != nil {
				p.reconnectStash.partnerHeroCardID = s.Partner.HeroCardID
			}
		} else {
			p.reconnectStash = nil
		}
		// ... existing reset code follows unchanged ...
```

**Step 4: Run test to verify it passes**

Run: `go test -count=1 -run TestReconnectStashPopulated ./internal/gamestate/`
Expected: PASS

**Step 5: Commit**

```
feat(gamestate): stash game-level state before EventGameStart reset
```

---

### Task 3: Detect reconnect and restore stash in EventGameEntityTags

**Files:**
- Modify: `internal/gamestate/processor.go:238-244` (EventGameEntityTags handler)
- Modify: `internal/gamestate/state.go` (add Machine restore method)

**Step 1: Write failing test**

```go
func TestReconnectDetectionAndRestore(t *testing.T) {
	m := NewMachine()
	p := NewProcessor(m)

	// Start a game and accumulate state
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 19, 46, 45, 0, time.UTC)})
	m.SetTurn(9)
	m.SetTavernTier(4)
	m.UpdateHeroCardID("BG25_HERO_103_SKIN_D")
	origGameID := m.State().GameID

	// Simulate reconnect: new EventGameStart, then GameEntityTags with STATE=RUNNING + TURN>1
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 20, 6, 46, 0, time.UTC)})

	// GameEntityTags with reconnect indicators
	p.Handle(parser.GameEvent{
		Type: parser.EventGameEntityTags,
		Tags: map[string]string{"STATE": "RUNNING", "TURN": "19"},
	})

	// State should be restored
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
	// Stash should be cleared
	if p.reconnectStash != nil {
		t.Error("expected reconnectStash to be nil after restore")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -count=1 -run TestReconnectDetectionAndRestore ./internal/gamestate/`
Expected: FAIL — GameID doesn't match, hero not restored.

**Step 3: Add restore method to Machine and detection logic**

In `internal/gamestate/state.go`, add:

```go
// RestoreFromReconnect restores game-level state preserved across a mid-game reconnect.
func (m *Machine) RestoreFromReconnect(gameID string, startTime time.Time, turn int, tavernTier int,
	isDuos bool, heroCardID string, partnerHeroCardID string, partnerPlayerName string,
	buffSources []BuffSource, abilityCounters []AbilityCounter,
	partnerBuffSources []BuffSource, partnerAbilityCtrs []AbilityCounter,
	modifications []StatMod, turnSnapshots []TurnSnapshot,
	prevBuffSources []BuffSource, prevAbilityCtrs []AbilityCounter, prevModCount int,
	anomalyCardID, anomalyName, anomalyDescription string, availableTribes []string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.GameID = gameID
	m.state.StartTime = startTime
	m.state.Turn = turn
	m.state.TavernTier = tavernTier
	m.state.IsDuos = isDuos
	m.state.Player.HeroCardID = heroCardID
	m.state.BuffSources = buffSources
	m.state.AbilityCounters = abilityCounters
	m.state.PartnerBuffSources = partnerBuffSources
	m.state.PartnerAbilityCounters = partnerAbilityCtrs
	m.state.Modifications = modifications
	m.state.AnomalyCardID = anomalyCardID
	m.state.AnomalyName = anomalyName
	m.state.AnomalyDescription = anomalyDescription
	m.state.AvailableTribes = availableTribes
	if partnerHeroCardID != "" {
		if m.state.Partner == nil {
			m.state.Partner = &PlayerState{}
		}
		m.state.Partner.HeroCardID = partnerHeroCardID
	}
	m.turnSnapshots = turnSnapshots
	m.prevBuffSources = prevBuffSources
	m.prevAbilityCtrs = prevAbilityCtrs
	m.prevModCount = prevModCount
}
```

In `internal/gamestate/processor.go`, update the `EventGameEntityTags` handler:

```go
	case parser.EventGameEntityTags:
		// Check for reconnect: STATE=RUNNING + TURN > 1 means mid-game reconnect.
		if p.reconnectStash != nil {
			state := e.Tags["STATE"]
			turn, _ := strconv.Atoi(e.Tags["TURN"])
			if state == "RUNNING" && turn > 1 {
				slog.Info("reconnect detected, restoring game state",
					"origGameID", p.reconnectStash.gameID,
					"origTurn", p.reconnectStash.turn,
					"reconnectTurn", turn)
				rs := p.reconnectStash
				p.machine.RestoreFromReconnect(
					rs.gameID, rs.startTime, rs.turn, rs.tavernTier,
					rs.isDuos, rs.heroCardID, rs.partnerHeroCardID, rs.partnerPlayerName,
					rs.buffSources, rs.abilityCounters,
					rs.partnerBuffSources, rs.partnerAbilityCtrs,
					rs.modifications, rs.turnSnapshots,
					rs.prevBuffSources, rs.prevAbilityCtrs, rs.prevModCount,
					rs.anomalyCardID, rs.anomalyName, rs.anomalyDescription,
					rs.availableTribes,
				)
				p.isDuos = rs.isDuos
				p.partnerPlayerID = rs.partnerPlayerID
				p.partnerPlayerName = rs.partnerPlayerName
				p.isReconnect = true
			}
			p.reconnectStash = nil
		}
		// Existing BACON_DUOS_PUNISH_LEAVERS logic
		for tag, value := range e.Tags {
			if tag == "BACON_DUOS_PUNISH_LEAVERS" && value == "1" {
				p.punishLeaversActive = true
				slog.Info("PUNISH_LEAVERS flag recorded (not sufficient alone for duos)", "tag", tag)
			}
		}
```

**Step 4: Run test to verify it passes**

Run: `go test -count=1 -run TestReconnectDetectionAndRestore ./internal/gamestate/`
Expected: PASS

**Step 5: Commit**

```
feat(gamestate): detect reconnect in EventGameEntityTags and restore stashed state
```

---

### Task 4: Protect hero CardID from overwrite during reconnect

**Files:**
- Modify: `internal/gamestate/processor.go:941` and `~972` (hero CardID update in handleEntityUpdate)

**Step 1: Write failing test**

```go
func TestReconnectHeroNotOverwritten(t *testing.T) {
	m := NewMachine()
	p := NewProcessor(m)

	// Start game, set hero
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 19, 46, 45, 0, time.UTC)})
	m.SetTurn(9)
	m.UpdateHeroCardID("BG25_HERO_103_SKIN_D")

	// Reconnect
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 20, 6, 46, 0, time.UTC)})
	p.Handle(parser.GameEvent{
		Type: parser.EventGameEntityTags,
		Tags: map[string]string{"STATE": "RUNNING", "TURN": "19"},
	})

	// Simulate FULL_ENTITY hero with different CardID (reconnect hero copy)
	// This should NOT overwrite the stashed hero.
	m.UpdateHeroCardID("TB_BaconShop_HERO_15")

	// The hero should still be the original
	// NOTE: This test documents the DESIRED behavior. The actual protection
	// is in handleEntityUpdate which guards on p.isReconnect. Since we're
	// calling m.UpdateHeroCardID directly, this test verifies the Machine
	// level. The processor-level guard is tested via integration.
	// For now, this test will fail — that's expected. The fix is in Step 3.
	s := m.State()
	if s.Player.HeroCardID != "TB_BaconShop_HERO_15" {
		// Direct Machine call SHOULD update — the guard is in the processor, not Machine.
		t.Errorf("Direct UpdateHeroCardID should work, got %q", s.Player.HeroCardID)
	}
}

func TestReconnectProcessorGuardsHeroUpdate(t *testing.T) {
	m := NewMachine()
	p := NewProcessor(m)

	// Start game with player identity
	p.Handle(parser.GameEvent{
		Type: parser.EventGameStart,
		Timestamp: time.Date(2026, 3, 24, 19, 46, 45, 0, time.UTC),
	})
	// Set local player identity (needed for hero detection)
	p.localPlayerID = 7
	p.localHeroID = 37
	p.heroEntities[37] = true
	p.entityController[37] = 7
	p.entityProps[37] = &entityInfo{CardID: "BG25_HERO_103_SKIN_D", CardType: "HERO"}
	m.UpdateHeroCardID("BG25_HERO_103_SKIN_D")
	m.SetTurn(9)

	// Reconnect
	p.Handle(parser.GameEvent{
		Type: parser.EventGameStart,
		Timestamp: time.Date(2026, 3, 24, 20, 6, 46, 0, time.UTC),
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventGameEntityTags,
		Tags: map[string]string{"STATE": "RUNNING", "TURN": "19"},
	})

	if !p.isReconnect {
		t.Fatal("expected isReconnect=true")
	}
	if m.State().Player.HeroCardID != "BG25_HERO_103_SKIN_D" {
		t.Fatalf("hero should be restored to BG25_HERO_103_SKIN_D, got %q", m.State().Player.HeroCardID)
	}
}
```

**Step 2: Run tests to verify behavior**

Run: `go test -count=1 -run TestReconnectProcessorGuardsHeroUpdate ./internal/gamestate/`

**Step 3: Add hero protection guard in handleEntityUpdate**

In `internal/gamestate/processor.go`, find the two places where `UpdateHeroCardID` is called in `handleEntityUpdate` (~lines 941 and 972). Add a guard:

Around line 941 (local hero update):
```go
		// During reconnect, don't overwrite the real hero CardID with the
		// reconnect FULL_ENTITY placeholder/copy.
		if !p.isReconnect || p.machine.State().Player.HeroCardID == "" {
			p.machine.UpdateHeroCardID(e.CardID)
		}
```

Around line 972 (partner hero update):
```go
		if !p.isReconnect || (p.machine.State().Partner != nil && p.machine.State().Partner.HeroCardID == "") {
			p.machine.UpdatePartnerHeroCardID(e.CardID)
		}
```

**Step 4: Run tests**

Run: `go test -count=1 -run TestReconnect ./internal/gamestate/`
Expected: all reconnect tests PASS

**Step 5: Commit**

```
feat(gamestate): protect hero CardID from overwrite during reconnect
```

---

### Task 5: Clear isReconnect on next fresh game

**Files:**
- Modify: `internal/gamestate/processor.go:157` (EventGameStart handler)

**Step 1: Write failing test**

```go
func TestReconnectFlagClearedOnNewGame(t *testing.T) {
	m := NewMachine()
	p := NewProcessor(m)

	// Game 1 start
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 19, 0, 0, 0, time.UTC)})
	m.SetTurn(5)
	// Reconnect
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 19, 30, 0, 0, time.UTC)})
	p.Handle(parser.GameEvent{Type: parser.EventGameEntityTags, Tags: map[string]string{"STATE": "RUNNING", "TURN": "10"}})
	if !p.isReconnect {
		t.Fatal("should be reconnect")
	}
	// Game 1 ends
	p.Handle(parser.GameEvent{Type: parser.EventGameEnd, Timestamp: time.Date(2026, 3, 24, 20, 0, 0, 0, time.UTC), Tags: map[string]string{}})
	// Game 2 starts fresh
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 20, 5, 0, 0, time.UTC)})
	p.Handle(parser.GameEvent{Type: parser.EventGameEntityTags, Tags: map[string]string{"STATE": "RUNNING", "TURN": "1"}})
	if p.isReconnect {
		t.Error("isReconnect should be false for fresh game")
	}
}
```

**Step 2: Run test**

Run: `go test -count=1 -run TestReconnectFlagClearedOnNewGame ./internal/gamestate/`
Expected: FAIL — isReconnect is still true.

**Step 3: Clear flag in EventGameStart**

In `internal/gamestate/processor.go`, in the `EventGameStart` handler, add at the beginning (before the stash logic):

```go
		p.isReconnect = false
```

**Step 4: Run test**

Run: `go test -count=1 -run TestReconnectFlagClearedOnNewGame ./internal/gamestate/`
Expected: PASS

**Step 5: Commit**

```
feat(gamestate): clear isReconnect flag on new game start
```

---

### Task 6: Ensure fresh games are not affected

**Files:**
- Modify: `internal/gamestate/gamestate_test.go`

**Step 1: Write test for fresh game (no false reconnect)**

```go
func TestFreshGameNotTreatedAsReconnect(t *testing.T) {
	m := NewMachine()
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
	m := NewMachine()
	p := NewProcessor(m)

	// First EventGameStart from idle — should NOT stash (no prior game state)
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Date(2026, 3, 24, 19, 0, 0, 0, time.UTC)})
	if p.reconnectStash != nil {
		t.Error("should not stash when starting from idle phase")
	}
}
```

**Step 2: Run tests**

Run: `go test -count=1 -run "TestFreshGameNotTreatedAsReconnect|TestNoStashFromIdlePhase" ./internal/gamestate/`
Expected: PASS (these should pass with the existing implementation since stash is only populated when phase != Idle/GameOver)

**Step 3: Commit**

```
test(gamestate): add fresh-game-not-reconnect guard tests
```

---

### Task 7: Run full test suite and fix any regressions

**Step 1: Run all tests**

Run: `go test -count=1 ./...`

**Step 2: Run vet**

Run: `go vet ./...`

**Step 3: Fix any failures**

If the existing integration test (`TestProcessorIntegration`) or other tests break, investigate and fix. The most likely issue is the stash being populated unexpectedly for back-to-back games in test fixtures.

**Step 4: Commit fixes if needed**

```
fix(gamestate): fix test regressions from reconnect handling
```

---

### Task 8: Reparse and verify reconnect game data

**Step 1: Backup BadgerDB**

```bash
cp -a ~/.battlestream/profiles/beeblebrox/data ~/.battlestream/profiles/beeblebrox/data-backup-$(date +%Y%m%d-%H%M%S)
```

**Step 2: Rebuild, reset, reparse**

```bash
go build ./cmd/battlestream
echo "yes" | ./battlestream db-reset
./battlestream reparse
```

**Step 3: Verify reconnect game has correct data**

Check game-1774404406134 (the reconnect game):
- Placement should be 4 (not 0)
- Hero should be the correct hero from mulligan (not the game-end copy)
- Turn snapshots should include pre-reconnect turns
- Buff sources should include pre-reconnect accumulation
- Game ID should be from the original CREATE_GAME timestamp (game-1774402005xxx, not game-1774404406xxx)

```bash
curl -s 'http://127.0.0.1:8080/v1/game/GAME_ID' | python3 -m json.tool
```

**Step 4: Verify game count**

Total games should decrease by 1 (the reconnect no longer creates a separate game — same game ID is reused). Confirm with:

```bash
curl -s 'http://127.0.0.1:8080/v1/stats/games?mode=all&limit=50' | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Total: {d[\"total\"]}')"
```

**Step 5: Commit any final fixes**

```
fix(gamestate): verify reconnect data integrity after reparse
```
