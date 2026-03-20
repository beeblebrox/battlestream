# Entity Filtering Fixes — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix entity filtering gaps across the processor — partner board overcounting, local board edge cases, buff attribution logic, and entity registry hygiene.

**Architecture:** Targeted fixes at each collection/filtering point identified in the audit. Max-7 enforcement as a hard cap. Combat entity tracking scoped per-combat. Entity registry pruned between combats.

**Tech Stack:** Go 1.24, existing processor/machine architecture

**Design doc:** `docs/plans/2026-03-19-duos-tui-design.md` (parent feature)

---

### Task 1: Max 7 Enforcement on Partner Board

The partner board collection has no upper bound — `partnerCombatMinions` can grow to 12+ entities because opponent minions or mid-combat spawns may pass the filters.

**Files:**
- Modify: `internal/gamestate/processor.go` — `finalizePartnerCombat()` (~line 1068)
- Modify: `internal/gamestate/state.go` — `SetPartnerBoard()` (~line 662)
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write failing test**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test -race -count=1 -run TestPartnerBoardMaxSeven ./internal/gamestate/`

**Step 3: Implement**

In `SetPartnerBoard()`:
```go
func (m *Machine) SetPartnerBoard(minions []MinionState, turn int) {
    m.mu.Lock()
    defer m.mu.Unlock()
    if len(minions) > 7 {
        minions = minions[:7]
    }
    // ... rest unchanged
}
```

Also add a trim in `finalizePartnerCombat()` before calling `SetPartnerBoard`:
```go
// Sort by ZONE_POSITION (ascending) so positions 1-7 are kept
sort.Slice(p.partnerCombatMinions, func(i, j int) bool {
    return p.partnerCombatMinions[i].ZonePos < p.partnerCombatMinions[j].ZonePos
})
if len(p.partnerCombatMinions) > 7 {
    p.partnerCombatMinions = p.partnerCombatMinions[:7]
}
```

Note: `MinionState` doesn't have a `ZonePos` field currently. We need to either add one (for sorting) or just keep the first 7 by `ZONE_POSITION` order as stored during collection. Check whether the inline collection at line 844 already orders by zone position (it should, since entities arrive in ZONE_POSITION order from the log).

If entities arrive in position order, a simple `[:7]` trim is sufficient.

**Step 4: Run test to verify it passes**

**Step 5: Run full test suite**

Run: `go test -race -count=1 ./...`

**Step 6: Commit**

```bash
git add internal/gamestate/processor.go internal/gamestate/state.go internal/gamestate/gamestate_test.go
git commit -m "fix: enforce max 7 minions on partner board collection"
```

---

### Task 2: Diagnose the 12-Minion Root Cause

Before fixing further, we need to understand WHY 12 minions are collected. The filters look correct on paper — `CONTROLLER=localPlayerID`, `CARDTYPE=MINION`, `ZONE=PLAY`, `ZONE_POSITION > 0`. So either the filters aren't working as expected, or the log data has more entities matching than expected.

**Files:**
- Modify: `internal/gamestate/processor.go` — add temporary debug logging
- Test: replay dump comparison

**Step 1: Add debug logging to partner combat collection**

In both inline collection (line 844) and `collectPartnerCombatRetro()`, add `slog.Debug` with entity ID, CardID, CONTROLLER, ZONE_POSITION for every entity that passes the filters.

Also log the total count in `finalizePartnerCombat()`.

**Step 2: Run replay with debug logging**

```bash
./battlestream replay --dump --turn 13 --width 140 Hearthstone_2026_03_17_19_33_31/Power.log 2>&1 | grep -i "partner.*combat\|partner.*minion\|finalize"
```

**Step 3: Analyze which entities are collected**

Cross-reference the logged entity IDs against the Power.log to determine:
- Are these all from the same combat? Or from multiple combats?
- Are opponent minions leaking in? (Check CONTROLLER values)
- Are combat spawns/tokens included? (Check if they have ZONE_POSITION)
- Is `combatPhaseEntityIDs` contaminated with entities from the local player's combat?

**Step 4: Document findings**

Record which entities are false positives and why they passed the filters. This informs the specific fix needed in Task 3.

**Step 5: Remove debug logging**

Remove or gate behind a flag.

**Step 6: Commit**

```bash
git commit -m "chore: diagnose partner board overcounting"
```

---

### Task 3: Fix Partner Combat Entity Scoping

Based on Task 2 findings, fix the root cause. Likely one of:

**Option A: combatPhaseEntityIDs contaminated across combats**

If local combat entities are leaking into partner's retroactive scan, separate the tracking:

```go
// Replace single combatPhaseEntityIDs with per-combat scoping
partnerCombatEntityIDs []int  // only entities from partner's combat
```

Clear `partnerCombatEntityIDs` when partner combat starts, only append during partner's combat. Don't use `combatPhaseEntityIDs` for the retroactive scan at all.

**Option B: Opponent minions passing CONTROLLER check**

If the opponent in partner's fight has `CONTROLLER=localPlayerID` (which shouldn't happen but might in edge cases), add a secondary filter:
- Check `COPIED_FROM_ENTITY_ID` — if the source entity has `PLAYER_ID=partnerPlayerID`, it's partner's minion
- Or check which side of the combat the entity is on by correlating with the partner hero copy's ZONE_POSITION

**Option C: Mid-combat spawns having ZONE_POSITION**

If deathrattle summons or token spawns have ZONE_POSITION > 0, tighten the filter:
- Only collect entities that exist at combat START (before any ATTACK events)
- Or filter by `EXHAUSTED=1` (initial board minions are exhausted at start)

**Files:**
- Modify: `internal/gamestate/processor.go`
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Implement fix based on Task 2 findings**

**Step 2: Run full test suite**

**Step 3: Verify with replay dump**

```bash
./battlestream replay --dump --turn 13 --width 140 Hearthstone_2026_03_17_19_33_31/Power.log
```

Partner board should show exactly 7 or fewer minions.

**Step 4: Commit**

```bash
git commit -m "fix: scope partner combat entity collection to partner's combat only"
```

---

### Task 4: Max 7 Enforcement on Local Board

`tryAddMinionFromRegistry()` has no max 7 cap. During combat, simulation copies can briefly push the board past 7. Add a safety check.

**Files:**
- Modify: `internal/gamestate/state.go` — `UpsertMinion()`
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write failing test**

```go
func TestLocalBoardMaxSeven(t *testing.T) {
    m := New()
    m.GameStart("test", time.Now())
    m.SetPhase(PhaseRecruit)

    for i := 0; i < 10; i++ {
        m.UpsertMinion(MinionState{EntityID: 100 + i, Name: fmt.Sprintf("M%d", i), Attack: 1, Health: 1})
    }
    s := m.State()
    if len(s.Board) > 7 {
        t.Errorf("expected max 7 board minions, got %d", len(s.Board))
    }
}
```

**Step 2: Implement**

In `UpsertMinion()`, after the upsert-or-append logic:
```go
// Hard cap: BG board max is 7
if len(m.state.Board) > 7 {
    m.state.Board = m.state.Board[:7]
}
```

Only enforce during non-combat phases — during combat, entities are simulation copies that get cleared on phase transition anyway. Actually, simpler: always cap at 7. If combat pushes past 7, the excess are irrelevant.

**Step 3: Run tests**

**Step 4: Commit**

```bash
git commit -m "fix: enforce max 7 minions on local board"
```

---

### Task 5: Tighten Local Board CardType Check

`tryAddMinionFromRegistry()` allows `info.CardType == ""` (unknown type). This is intentional as a fallback, but could add non-minion entities to the board.

**Files:**
- Modify: `internal/gamestate/processor.go:1344`

**Step 1: Check if any test relies on CardType=="" entities being added**

Run: `grep -n 'CardType.*""' internal/gamestate/gamestate_test.go`

**Step 2: Tighten the check**

Change from:
```go
if info.CardType != "" && info.CardType != "MINION" {
    return
}
```
To:
```go
if info.CardType != "MINION" {
    return
}
```

**Step 3: Run full test suite**

If any tests fail because they relied on CardType=="" fallback, update them to set CardType="MINION" in their test events.

**Step 4: Verify with replay dumps**

```bash
./battlestream replay --dump --turn 10 --width 140 Hearthstone_2026_03_17_19_33_31/Power.log
```

Board should still show correct minions.

**Step 5: Commit**

```bash
git commit -m "fix: require CARDTYPE=MINION for board entries (remove empty-type fallback)"
```

---

### Task 6: Simplify Buff Source Enchantment Filter Logic

The filter in `handleEnchantmentEntity` (lines 1422-1435) has confusing inverted logic. Simplify it.

**Files:**
- Modify: `internal/gamestate/processor.go:1422-1435`
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Rewrite the filter**

Replace the nested negative checks with clear positive logic:

```go
// Determine if this enchantment should be tracked.
targetCtrl := p.entityController[info.AttachedTo]
enchCtrl := p.entityController[e.EntityID]
isDntLocal := p.isLocalDntTarget(e.EntityID)

isRelevant := false
if targetCtrl == p.localPlayerID {
    // Enchantment on a local minion — always track
    isRelevant = true
} else if isDntLocal {
    // Dnt enchantment attached to local/bot player entity — track
    isRelevant = true
} else if enchCtrl == p.localPlayerID && category != CatGeneral {
    // Enchantment owned by local player on non-local target (e.g., aura effects)
    // Only track if it has a specific (non-general) category
    isRelevant = true
}

if !isRelevant {
    return
}
```

**Step 2: Run full test suite**

Verify no behavior change for existing tests.

**Step 3: Verify buff sources unchanged**

```bash
./battlestream replay --dump --turn 12 --width 140 Hearthstone_2026_03_16_22_58_06/Power.log
```

Buff values should be identical to before the refactor.

**Step 4: Commit**

```bash
git commit -m "refactor: simplify enchantment relevance filter for clarity"
```

---

### Task 7: Entity Registry Pruning Between Combats

`entityProps`, `entityController`, and `heroEntities` grow unbounded during a game. Combat creates hundreds of temporary entities that are never cleaned up.

**Files:**
- Modify: `internal/gamestate/processor.go` — add `pruneStaleEntities()` called on phase transitions

**Step 1: Write test**

```go
func TestEntityRegistryPrunedAfterCombat(t *testing.T) {
    _, p := newProc()
    setupGame(p)

    // Simulate combat entities
    for i := 500; i < 600; i++ {
        p.entityProps[i] = &entityInfo{CardType: "MINION", Zone: "REMOVEDFROMGAME"}
        p.entityController[i] = 15
    }

    sizeBefore := len(p.entityProps)
    p.pruneStaleEntities()
    sizeAfter := len(p.entityProps)

    if sizeAfter >= sizeBefore {
        t.Errorf("expected pruning to reduce entity count, before=%d after=%d", sizeBefore, sizeAfter)
    }
}
```

**Step 2: Implement**

```go
func (p *Processor) pruneStaleEntities() {
    for id, info := range p.entityProps {
        // Keep: entities in active zones (PLAY, HAND, SETASIDE)
        // Prune: REMOVEDFROMGAME, GRAVEYARD, DECK (dead combat copies)
        if info.Zone == "REMOVEDFROMGAME" || info.Zone == "GRAVEYARD" {
            delete(p.entityProps, id)
            delete(p.entityController, id)
            delete(p.heroEntities, id)
        }
    }
}
```

Call `pruneStaleEntities()` on transition to RECRUIT phase (in `SetGameEntityTurn` when odd turn number detected).

**Step 3: Run full test suite**

**Step 4: Verify no behavior change**

Test all 3 duos logs still produce correct output.

**Step 5: Commit**

```bash
git commit -m "perf: prune stale entities from registry on recruit phase transition"
```

---

### Task 8: Final Validation

**Step 1: Run full test suite**

```bash
go test -race -count=1 ./...
```

**Step 2: Run go vet**

```bash
go vet ./...
```

**Step 3: Test all duos logs**

```bash
./battlestream replay --dump --turn 5 --width 140 Hearthstone_2026_03_16_22_05_57/Power.log
./battlestream replay --dump --turn 12 --width 140 Hearthstone_2026_03_16_22_58_06/Power.log
./battlestream replay --dump --turn 13 --width 140 Hearthstone_2026_03_17_19_33_31/Power.log
```

Verify for each:
- Partner board has <= 7 minions
- Local board has <= 7 minions
- Buff sources show correct values
- No panics or unexpected behavior

**Step 4: Test the existing solo game log**

```bash
go test -race -count=1 -run TestProcessorIntegration ./internal/gamestate/
```

Ensure the filtering changes don't break solo game processing.

**Step 5: Commit any final fixes**

```bash
git commit -m "chore: final validation of entity filtering fixes"
```
