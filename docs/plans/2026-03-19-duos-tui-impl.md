# Duos TUI Overhaul — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix duos detection/buff bugs, add partner board tracking via combat copies, handle disconnects, and unify the duos TUI experience across live and replay modes.

**Architecture:** Multi-signal duos detection in the processor with deferred partner resolution. Partner board captured from combat copy entities during `BACON_CURRENT_COMBAT_PLAYER_ID` events. Staleness timer for incomplete games. Scrollable partner board panel in both TUIs.

**Tech Stack:** Go 1.24, protobuf, Bubbletea/Lipgloss TUI, gRPC, BadgerDB

**Design doc:** `docs/plans/2026-03-19-duos-tui-design.md`

---

### Task 1: Parser — Capture GameEntity Tags from CREATE_GAME Block

The parser currently drops GameEntity block tags (like `BACON_DUOS_PUNISH_LEAVERS`) during CREATE_GAME because `reBlockTag` only fires when `p.inBlock` is true (FULL_ENTITY/SHOW_ENTITY context). We need to capture these tags and include them in the `EventGameStart` event.

**Files:**
- Modify: `internal/parser/parser.go:154-164`
- Test: `internal/parser/parser_test.go`

**Step 1: Write the failing test**

Add a test that feeds a CREATE_GAME block with `BACON_DUOS_PUNISH_LEAVERS` and verifies the `EventGameStart` event contains it.

```go
func TestCreateGameCapturesGameEntityTags(t *testing.T) {
    lines := []string{
        "D 20:04:12.2333830 GameState.DebugPrintPower() - CREATE_GAME",
        "D 20:04:12.2333830 GameState.DebugPrintPower() -     GameEntity EntityID=13",
        "D 20:04:12.2333830 GameState.DebugPrintPower() -         tag=CARDTYPE value=GAME",
        "D 20:04:12.2333830 GameState.DebugPrintPower() -         tag=BACON_DUOS_PUNISH_LEAVERS value=1",
        "D 20:04:12.2333830 GameState.DebugPrintPower() -     Player EntityID=14 PlayerID=5 GameAccountId=[hi=144115193835963207 lo=30722021]",
    }
    var events []GameEvent
    p := NewParser(func(e GameEvent) { events = append(events, e) })
    for _, l := range lines {
        p.Feed(l)
    }
    p.Flush()

    // Find EventGameStart
    var found bool
    for _, e := range events {
        if e.Type == EventGameStart {
            if e.Tags["BACON_DUOS_PUNISH_LEAVERS"] != "1" {
                t.Errorf("expected BACON_DUOS_PUNISH_LEAVERS=1, got %q", e.Tags["BACON_DUOS_PUNISH_LEAVERS"])
            }
            found = true
        }
    }
    if !found {
        t.Fatal("no EventGameStart emitted")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -count=1 -run TestCreateGameCapturesGameEntityTags ./internal/parser/`
Expected: FAIL — the tag is not captured.

**Step 3: Implement**

In `parser.go`, after emitting `EventGameStart`, enter a special `inCreateGame` state that captures GameEntity block tags into a deferred tags map. When the `Player EntityID=` line is seen (which marks the end of the GameEntity block), merge those tags into the `EventGameStart` event.

Approach: Change `EventGameStart` emission to be deferred. After `CREATE_GAME` is matched, set `p.inCreateGame = true` and store a reference to the EventGameStart. Capture `reBlockTag` matches while `inCreateGame && !inPlayerDef`. When `rePlayerDef` matches, the GameEntity block is over — emit the stored EventGameStart with accumulated tags.

Simpler approach: Add a `createGameTags map[string]string` field. When `inCreateGame` is true and `reBlockTag` matches and we haven't hit `rePlayerDef` yet, accumulate tags. When `rePlayerDef` is first seen, emit a synthetic `EventGameEntityTags` event with all accumulated tags, then process the player def normally.

Actually, the simplest approach: add a new event type `EventGameEntityTags` emitted when the GameEntity block ends (signaled by the first `Player EntityID=` line). The processor handles this event to extract duos signals.

Add to parser:
- `EventGameEntityTags` constant
- `inCreateGameEntity bool` and `createGameEntityTags map[string]string` fields
- On `CREATE_GAME`: set `inCreateGameEntity = true`, init tags map
- On `reBlockTag` while `inCreateGameEntity`: accumulate tag/value
- On `rePlayerDef`: if `inCreateGameEntity`, emit `EventGameEntityTags` with accumulated tags, then set `inCreateGameEntity = false`

**Step 4: Run test to verify it passes**

Run: `go test -race -count=1 -run TestCreateGameCapturesGameEntityTags ./internal/parser/`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/parser/parser.go internal/parser/parser_test.go
git commit -m "feat: capture GameEntity tags from CREATE_GAME block in parser"
```

---

### Task 2: Multi-Signal Duos Detection

Add fallback duos detection via `BACON_DUOS_PUNISH_LEAVERS` (from GameEntity tags) and `BACON_DUO_PASSABLE` (from TAG_CHANGE on card entities).

**Files:**
- Modify: `internal/gamestate/processor.go:59-109` (Processor struct), `124-150` (Handle/reset), `274-315` (handleTagChange duos cases)
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write failing tests**

```go
func TestDuosDetectionViaPunishLeavers(t *testing.T) {
    m, p := newProc()
    // Game start with BACON_DUOS_PUNISH_LEAVERS in GameEntity tags
    p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
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
    if !s.IsDuos {
        t.Error("expected IsDuos=true from BACON_DUOS_PUNISH_LEAVERS")
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

    // BACON_DUO_PASSABLE on a card entity triggers duos detection
    p.Handle(parser.GameEvent{
        Type: parser.EventTagChange, EntityID: 1143,
        Tags: map[string]string{"BACON_DUO_PASSABLE": "1"},
    })

    if !m.State().IsDuos {
        t.Error("expected IsDuos=true from BACON_DUO_PASSABLE")
    }
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -race -count=1 -run 'TestDuosDetectionViaPunishLeavers|TestDuosDetectionViaDuoPassable' ./internal/gamestate/`

**Step 3: Implement**

In `processor.go`:

1. Add `EventGameEntityTags` handling in `Handle()`:
```go
case parser.EventGameEntityTags:
    for tag, value := range e.Tags {
        if tag == "BACON_DUOS_PUNISH_LEAVERS" && value == "1" {
            if !p.isDuos {
                p.isDuos = true
                p.machine.SetDuosMode(true)
                slog.Info("Duos detected from GameEntity tag", "tag", tag)
            }
        }
    }
```

2. Add `BACON_DUO_PASSABLE` case in `handleTagChange()`:
```go
case "BACON_DUO_PASSABLE":
    if value == "1" && !p.isDuos {
        p.isDuos = true
        p.machine.SetDuosMode(true)
        slog.Info("Duos detected from BACON_DUO_PASSABLE")
    }
```

**Step 4: Run tests to verify they pass**

Run: `go test -race -count=1 -run 'TestDuosDetection' ./internal/gamestate/`

**Step 5: Run full test suite**

Run: `go test -race -count=1 ./...`

**Step 6: Commit**

```bash
git add internal/gamestate/processor.go internal/gamestate/gamestate_test.go
git commit -m "feat: multi-signal duos detection (PUNISH_LEAVERS, DUO_PASSABLE fallbacks)"
```

---

### Task 3: Deferred Partner Resolution

When duos is detected without `BACON_DUO_TEAMMATE_PLAYER_ID`, resolve the partner later via `BACON_CURRENT_COMBAT_PLAYER_ID`.

**Files:**
- Modify: `internal/gamestate/processor.go` — `handleTagChange` (`BACON_CURRENT_COMBAT_PLAYER_ID` case), new `resolvePartner()` method
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write failing test**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test -race -count=1 -run TestDeferredPartnerResolution ./internal/gamestate/`

**Step 3: Implement**

Add `resolvePartner(playerID int)` method to Processor:
- Sets `p.partnerPlayerID = playerID`
- Scans `p.heroEntities` and `p.entityProps` for a hero entity with `PLAYER_ID` matching `playerID`
- If found, sets `p.partnerHeroID`, calls `p.machine.UpdatePartnerHeroCardID()`
- Scans cached player names for the partner name

Add `BACON_CURRENT_COMBAT_PLAYER_ID` case in `handleTagChange()`:
```go
case "BACON_CURRENT_COMBAT_PLAYER_ID":
    if p.isLocalPlayerEntity(e) {
        combatPlayerID, _ := strconv.Atoi(value)
        if combatPlayerID > 0 && combatPlayerID != p.localPlayerID && p.isDuos && p.partnerPlayerID == 0 {
            p.resolvePartner(combatPlayerID)
        }
    }
```

Store `PLAYER_ID` tag values from `handleEntityUpdate` in `entityInfo` so `resolvePartner` can scan them.

**Step 4: Run test to verify it passes**

**Step 5: Run full test suite**

**Step 6: Commit**

```bash
git add internal/gamestate/processor.go internal/gamestate/gamestate_test.go
git commit -m "feat: deferred partner resolution via BACON_CURRENT_COMBAT_PLAYER_ID"
```

---

### Task 4: Buff Attribution Fix for Duos Dnt Enchantments

Relax the controller check for Dnt enchantments to also accept enchantments attached to local player entities.

**Files:**
- Modify: `internal/gamestate/processor.go:1090-1160` (`handleEnchantmentEntity`), `1164-1206` (`handleDntTagChange`), `568-576` (TAG_SCRIPT_DATA case)
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write failing test**

```go
func TestDuosDntEnchantmentWithBotController(t *testing.T) {
    m, p := newProc()
    setupDuosGame(p)

    // Register local player entity 14 and hero entity 33
    p.entityController[14] = 7 // local player
    p.entityController[33] = 7 // local hero

    // BG_ShopBuff Dnt enchantment created with CONTROLLER=botID (15),
    // but ATTACHED to local hero (33)
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
        if bs.Category == "Shop Buff" {
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
```

**Step 2: Run test to verify it fails**

**Step 3: Implement**

In `handleEnchantmentEntity` and `handleDntTagChange`, add a helper `isLocalDntEnchantment(entityID int) bool` that returns true if:
- `entityController[entityID] == localPlayerID`, OR
- the enchantment's `ATTACHED` target is the local hero or local player entity

Modify `handleDntTagChange:1170-1172`:
```go
ctrl := p.entityController[entityID]
if ctrl != p.localPlayerID && !p.isLocalDntTarget(entityID) {
    return
}
```

Add helper:
```go
func (p *Processor) isLocalDntTarget(entityID int) bool {
    info := p.entityProps[entityID]
    if info == nil {
        return false
    }
    // Check if ATTACHED target is local hero or local player entity
    if info.AttachedTo > 0 {
        if info.AttachedTo == p.localHeroID {
            return true
        }
        if pid, ok := p.playerEntityIDs[info.AttachedTo]; ok && pid == p.localPlayerID {
            return true
        }
    }
    return false
}
```

Apply same check in `handleEnchantmentEntity:1150-1158` for initial SD processing.

Also update the TAG_SCRIPT_DATA_NUM case in `handleTagChange:568-576`:
```go
case "TAG_SCRIPT_DATA_NUM_1", "TAG_SCRIPT_DATA_NUM_2":
    if e.EntityID > 0 {
        p.updateEnchantmentScriptData(e.EntityID, tag, value)
        ctrl := p.entityController[e.EntityID]
        if ctrl == p.localPlayerID || p.isLocalDntTarget(e.EntityID) {
            p.handleDntTagChange(e.EntityID, tag, parseInt(value))
        }
    }
```

**Step 4: Run test to verify it passes**

**Step 5: Run full test suite**

**Step 6: Commit**

```bash
git add internal/gamestate/processor.go internal/gamestate/gamestate_test.go
git commit -m "fix: accept Dnt enchantments attached to local hero regardless of CONTROLLER"
```

---

### Task 5: PartnerBoard Data Model & Machine Methods

Add the `PartnerBoard` struct and machine methods for capturing partner board snapshots.

**Files:**
- Modify: `internal/gamestate/state.go:24-48` (BGGameState), add PartnerBoard struct
- Modify: `internal/gamestate/state.go:183-205` (State() deep copy)
- Add machine methods: `SetPartnerBoard()`, `MarkPartnerBoardStale()`
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write failing test**

```go
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
```

**Step 2: Run test to verify it fails**

**Step 3: Implement**

In `state.go`:
```go
type PartnerBoard struct {
    Minions []MinionState `json:"minions"`
    Turn    int           `json:"turn"`
    Stale   bool          `json:"stale"`
}
```

Add `PartnerBoard *PartnerBoard` to `BGGameState`.

In `State()` deep copy, add:
```go
if m.state.PartnerBoard != nil {
    pb := *m.state.PartnerBoard
    pb.Minions = make([]MinionState, len(m.state.PartnerBoard.Minions))
    for i, mn := range m.state.PartnerBoard.Minions {
        pb.Minions[i] = mn
        pb.Minions[i].Enchantments = append([]Enchantment(nil), mn.Enchantments...)
    }
    s.PartnerBoard = &pb
}
```

Add machine methods:
```go
func (m *Machine) SetPartnerBoard(minions []MinionState, turn int) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.state.PartnerBoard = &PartnerBoard{
        Minions: append([]MinionState(nil), minions...),
        Turn:    turn,
        Stale:   false,
    }
}

func (m *Machine) MarkPartnerBoardStale() {
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.state.PartnerBoard != nil {
        m.state.PartnerBoard.Stale = true
    }
}
```

**Step 4: Run test to verify it passes**

**Step 5: Run full test suite**

**Step 6: Commit**

```bash
git add internal/gamestate/state.go internal/gamestate/gamestate_test.go
git commit -m "feat: add PartnerBoard data model and machine methods"
```

---

### Task 6: Partner Board Capture from Combat Copies

Track combat copy minions during partner's combat and snapshot them.

**Files:**
- Modify: `internal/gamestate/processor.go` — Processor struct (add combat tracking fields), `handleTagChange` (BACON_CURRENT_COMBAT_PLAYER_ID case), `handleEntityUpdate` (capture combat copies)
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write failing test**

```go
func TestPartnerBoardCaptureFromCombat(t *testing.T) {
    m, p := newProc()
    setupDuosGame(p)
    // Set partner hero
    p.partnerHeroID = 146
    p.heroEntities[146] = true
    p.entityController[146] = 13 // bot

    // Simulate partner's combat starting
    p.Handle(parser.GameEvent{
        Type: parser.EventTagChange, EntityID: 14, PlayerID: 7,
        EntityName: "Moch#1358",
        Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "8"},
    })

    // Partner hero combat copy gets PLAYER_ID=8
    p.Handle(parser.GameEvent{
        Type: parser.EventEntityUpdate, EntityID: 700, CardID: "BG32_HERO_002",
        Tags: map[string]string{
            "CONTROLLER": "15", "CARDTYPE": "HERO", "PLAYER_ID": "8",
            "HEALTH": "30", "ZONE": "PLAY",
        },
    })

    // Partner's combat minions (CONTROLLER=15=bot, same side as partner hero)
    // These are created as FULL_ENTITY then revealed via SHOW_ENTITY
    p.Handle(parser.GameEvent{
        Type: parser.EventEntityUpdate, EntityID: 703, CardID: "BGS_119",
        Tags: map[string]string{
            "CONTROLLER": "15", "CARDTYPE": "MINION",
            "ATK": "220", "HEALTH": "177", "ZONE": "PLAY",
            "COPIED_FROM_ENTITY_ID": "534",
        },
    })
    p.Handle(parser.GameEvent{
        Type: parser.EventEntityUpdate, EntityID: 704, CardID: "BG32_111",
        Tags: map[string]string{
            "CONTROLLER": "15", "CARDTYPE": "MINION",
            "ATK": "150", "HEALTH": "132", "ZONE": "PLAY",
            "COPIED_FROM_ENTITY_ID": "535",
        },
    })

    // Combat ends — local player's combat starts
    p.Handle(parser.GameEvent{
        Type: parser.EventTagChange, EntityID: 14, PlayerID: 7,
        EntityName: "Moch#1358",
        Tags: map[string]string{"BACON_CURRENT_COMBAT_PLAYER_ID": "7"},
    })

    s := m.State()
    if s.PartnerBoard == nil {
        t.Fatal("expected PartnerBoard != nil after partner combat")
    }
    if len(s.PartnerBoard.Minions) != 2 {
        t.Errorf("expected 2 partner minions, got %d", len(s.PartnerBoard.Minions))
    }
}
```

**Step 2: Run test to verify it fails**

**Step 3: Implement**

Add to Processor struct:
```go
// Partner combat tracking
partnerCombatActive   bool           // true while partner's combat is in progress
partnerCombatHeroCtrl int            // CONTROLLER of partner's hero copy in combat
partnerCombatMinions  []MinionState  // collected partner minions during combat
```

In `handleTagChange`, `BACON_CURRENT_COMBAT_PLAYER_ID` case:
```go
case "BACON_CURRENT_COMBAT_PLAYER_ID":
    if p.isLocalPlayerEntity(e) {
        combatPlayerID, _ := strconv.Atoi(value)

        // If partner combat was active and is now ending, snapshot the board
        if p.partnerCombatActive && combatPlayerID != p.partnerPlayerID {
            p.finalizePartnerCombat()
        }

        // Deferred partner resolution
        if combatPlayerID > 0 && combatPlayerID != p.localPlayerID && p.isDuos && p.partnerPlayerID == 0 {
            p.resolvePartner(combatPlayerID)
        }

        // Start tracking if this is partner's combat
        if combatPlayerID == p.partnerPlayerID {
            p.partnerCombatActive = true
            p.partnerCombatHeroCtrl = 0
            p.partnerCombatMinions = nil
        }
    }
```

In `handleEntityUpdate`, when processing HERO during partner combat, identify the partner hero copy's CONTROLLER. When processing MINION during partner combat, if the minion has the same CONTROLLER as the partner hero copy, add it to `partnerCombatMinions`.

Add `finalizePartnerCombat()`:
```go
func (p *Processor) finalizePartnerCombat() {
    p.partnerCombatActive = false
    if len(p.partnerCombatMinions) > 0 {
        turn := 0
        s := p.machine.State()
        turn = s.Turn
        p.machine.SetPartnerBoard(p.partnerCombatMinions, turn)
    }
    p.partnerCombatMinions = nil
}
```

Also call `finalizePartnerCombat()` on phase transition to RECRUIT (in `SetGameEntityTurn`).

Also call `MarkPartnerBoardStale()` on RECRUIT start if partner didn't fight this turn.

**Step 4: Run test to verify it passes**

**Step 5: Run full test suite**

**Step 6: Commit**

```bash
git add internal/gamestate/processor.go internal/gamestate/gamestate_test.go
git commit -m "feat: capture partner board from combat copy entities"
```

---

### Task 7: Partner Display Fixes

Fix partner name blank + raw CardID in debug TUI. Add "(Team)" health label.

**Files:**
- Modify: `internal/debugtui/model.go:987-1000` (partner info rendering)
- Modify: `internal/tui/tui.go:468-524` (hero panel, health label)
- Test: Manual via `replay --dump`

**Step 1: Fix debug TUI partner hero name**

In `model.go` partner info rendering, replace raw `partner.HeroCardID` with `gamestate.CardName(partner.HeroCardID)`:

```go
// Before:
pInfoStr.WriteString(fmt.Sprintf("Hero: %s\n", partner.HeroCardID))
// After:
heroName := gamestate.CardName(partner.HeroCardID)
if heroName == "" {
    heroName = partner.HeroCardID
}
pInfoStr.WriteString(fmt.Sprintf("Hero: %s\n", heroName))
```

**Step 2: Add "(Team)" to health label in live TUI duos mode**

In `tui.go` `renderHeroPanel()`, when `m.game.IsDuos`:
```go
healthLabel := "Health"
if m.game.IsDuos {
    healthLabel = "Health (Team)"
}
```

**Step 3: Remove redundant health from partner section**

In both TUIs, the partner section should NOT show health/armor (it's the same shared team pool shown in the local player section).

**Step 4: Test via replay dump**

Run: `./battlestream replay --dump --turn 10 --width 140 Hearthstone_2026_03_17_19_33_31/Power.log`

Verify:
- Partner hero shows "Buttons" not "BG32_HERO_002"
- Health label shows "(Team)"
- Partner section shows tier/triples/hero without redundant health

**Step 5: Commit**

```bash
git add internal/tui/tui.go internal/debugtui/model.go
git commit -m "fix: resolve partner hero name, add Team health label in duos"
```

---

### Task 8: Proto & Serialization Updates

Unreserve proto field 19, add partner board fields, regenerate, update serialization.

**Files:**
- Modify: `proto/battlestream/v1/game.proto:89` (unreserve 19, add fields)
- Run: `scripts/gen-proto.sh`
- Modify: `internal/api/grpc/server.go:272-315` (gameStateToProto)
- Modify: `internal/fileout/fileout.go` (if applicable)

**Step 1: Update proto**

```protobuf
// Replace:
reserved 19, 20, 21;
// With:
repeated MinionState partner_board = 19;
reserved 20, 21;
int32 partner_board_turn = 25;
bool partner_board_stale = 26;
```

**Step 2: Regenerate**

Run: `scripts/gen-proto.sh`

**Step 3: Update gRPC serialization**

In `server.go` `gameStateToProto()`, add after partner serialization:
```go
if s.PartnerBoard != nil {
    for _, mn := range s.PartnerBoard.Minions {
        gs.PartnerBoard = append(gs.PartnerBoard, minionToProto(mn))
    }
    gs.PartnerBoardTurn = int32(s.PartnerBoard.Turn)
    gs.PartnerBoardStale = s.PartnerBoard.Stale
}
```

**Step 4: Build and test**

Run: `go build ./cmd/battlestream && go test -race -count=1 ./...`

**Step 5: Commit**

```bash
git add proto/ internal/api/grpc/gen/ internal/api/grpc/server.go internal/fileout/
git commit -m "feat: add partner_board proto fields, update serialization"
```

---

### Task 9: Stale Game Timeout

Add a staleness timer that marks incomplete games as over after 3 minutes of inactivity.

**Files:**
- Modify: `internal/gamestate/processor.go` — add `lastEventTime`, `CheckStaleness()` method
- Modify: `cmd/battlestream/main.go` or daemon command — add periodic staleness check
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write failing test**

```go
func TestStaleGameTimeout(t *testing.T) {
    m, p := newProc()
    setupGame(p)

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
```

**Step 2: Run test to verify it fails**

**Step 3: Implement**

Add `lastEventTime time.Time` to Processor struct. Update it in `Handle()` on every event.

Add `CheckStaleness()`:
```go
const staleGameTimeout = 3 * time.Minute

func (p *Processor) CheckStaleness() {
    if p.machine.Phase() == PhaseIdle || p.machine.Phase() == PhaseGameOver {
        return
    }
    if p.lastEventTime.IsZero() {
        return
    }
    if time.Since(p.lastEventTime) > staleGameTimeout {
        slog.Warn("game stale, forcing game over", "lastEvent", p.lastEventTime)
        p.machine.GameEnd(0, time.Now()) // placement=0 means unknown
    }
}
```

In the daemon, add a goroutine that calls `processor.CheckStaleness()` every 30 seconds.

**Step 4: Run test to verify it passes**

**Step 5: Commit**

```bash
git add internal/gamestate/processor.go internal/gamestate/gamestate_test.go
git commit -m "feat: stale game timeout (3 min inactivity marks game over)"
```

---

### Task 10: Partner Board Panel — Live TUI

Add the scrollable partner board panel to the live TUI.

**Files:**
- Modify: `internal/tui/tui.go` — Model struct (add viewport), View(), Update(), handleMouse()

**Step 1: Add partnerBoardVP to Model**

Add `partnerBoardVP viewport.Model` field and initialize it in `New()`.

**Step 2: Render partner board content**

Add `partnerBoardItems()` method that renders minions in multi-column layout:
```go
func (m *Model) partnerBoardItems() string {
    if m.game == nil || m.game.PartnerBoard == nil {
        return styleDim.Render("(awaiting first combat)")
    }
    pb := m.game.PartnerBoard
    // Calculate columns from width
    // Render each minion as "Name  ATK/HP" in fixed-width columns
    // Return the formatted string
}
```

**Step 3: Add partner board panel to View()**

In `View()`, when `m.game.IsDuos`, render a full-width bordered panel below the main row:
```go
if m.game.IsDuos {
    title := "PARTNER BOARD"
    if m.game.PartnerBoard != nil {
        if m.game.PartnerBoard.Stale {
            title = fmt.Sprintf("PARTNER BOARD (Turn %d - last seen)", m.game.PartnerBoard.Turn)
        } else {
            title = fmt.Sprintf("PARTNER BOARD (Turn %d)", m.game.PartnerBoard.Turn)
        }
    }
    // render bordered panel with viewport content
}
```

**Step 4: Wire mouse scrolling**

In `handleMouse()`, add bounds check for the partner board panel area and route scroll events to `partnerBoardVP`.

**Step 5: Test via dump**

Build and run: `./battlestream tui --dump --width 140`

**Step 6: Commit**

```bash
git add internal/tui/tui.go
git commit -m "feat: partner board panel in live TUI with scrolling"
```

---

### Task 11: Partner Board Panel — Debug/Replay TUI

Add the same scrollable partner board panel to the debug/replay TUI.

**Files:**
- Modify: `internal/debugtui/model.go` — Model struct, View(), Update(), handleMouse()

**Step 1: Replace existing partner info panel with partner board panel**

The debug TUI already has `partnerBoardVP` and `partnerBuffVP` viewport fields (reserved but unused). Wire them up.

**Step 2: Render partner board in viewStep()**

Same multi-column rendering logic as the live TUI. Extract a shared helper to a common package or duplicate (since the rendering is short).

**Step 3: Update [d] toggle**

`[d]` toggles between showing the partner board panel vs the compact partner info panel (hero/tier/triples). Default to partner board.

**Step 4: Wire mouse scrolling**

Add bounds check for partner board panel in `handleMouse()`.

**Step 5: Test via replay dump**

Run: `./battlestream replay --dump --turn 10 --width 140 Hearthstone_2026_03_17_19_33_31/Power.log`

Verify partner board panel renders with turn label.

**Step 6: Commit**

```bash
git add internal/debugtui/model.go
git commit -m "feat: partner board panel in debug/replay TUI with scrolling"
```

---

### Task 12: Integration Tests with Real Duos Logs

Create integration tests using excerpts from the real duos game logs.

**Files:**
- Create: `internal/gamestate/testdata/duos_punish_leavers.txt` (excerpt from 22:58 session)
- Create: `internal/gamestate/testdata/duos_with_teammate_id.txt` (excerpt from 03/17 session)
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Extract test data**

Extract the first ~2000 lines of each Power.log (covers CREATE_GAME through early turns). These are small enough for test data.

**Step 2: Write integration tests**

```go
func TestIntegrationDuosPunishLeavers(t *testing.T) {
    // Parse duos_punish_leavers.txt
    // Verify: IsDuos=true, partner eventually resolved, buff sources non-empty
}

func TestIntegrationDuosWithTeammateID(t *testing.T) {
    // Parse duos_with_teammate_id.txt
    // Verify: IsDuos=true, partner hero identified, partner board captured
}
```

**Step 3: Run integration tests**

Run: `go test -race -count=1 -run TestIntegration ./internal/gamestate/`

**Step 4: Commit**

```bash
git add internal/gamestate/testdata/ internal/gamestate/gamestate_test.go
git commit -m "test: integration tests with real duos game logs"
```

---

### Task 13: Update CLAUDE.md Documentation

Update the duos documentation to reflect the new capabilities and fixed assumptions.

**Files:**
- Modify: `CLAUDE.md`

**Step 1: Update Duos Support section**

Update the "Not available" list — partner board IS now available (last seen from combat). Update the detection mechanism description. Document the new `PartnerBoard` data model.

**Step 2: Update Buff Source Tracking section**

Document the Dnt enchantment controller relaxation for duos.

**Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update duos documentation for partner board and detection fixes"
```

---

### Task 14: Final Validation

**Step 1: Run full test suite**

Run: `go test -race -count=1 ./...`

**Step 2: Run go vet**

Run: `go vet ./...`

**Step 3: Test with all 3 duos logs**

```bash
./battlestream replay --dump --turn 5 --width 140 Hearthstone_2026_03_16_22_05_57/Power.log
./battlestream replay --dump --turn 10 --width 140 Hearthstone_2026_03_16_22_58_06/Power.log
./battlestream replay --dump --turn 10 --width 140 Hearthstone_2026_03_17_19_33_31/Power.log
```

Verify for each:
- `[DUOS]` badge appears
- Partner name and hero resolved
- Buff sources show reasonable values
- Partner board panel renders (for the 03/17 log which has combat data)

**Step 4: Commit any fixes**

**Step 5: Final commit if needed**

```bash
git add -A && git commit -m "chore: final validation fixes"
```
