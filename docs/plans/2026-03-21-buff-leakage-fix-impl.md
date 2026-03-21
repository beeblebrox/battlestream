# Buff Leakage Fix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix opponent/partner Dnt enchantment buff sources leaking into the local player's BUFF SOURCES display in duos games.

**Architecture:** Remove the flawed bot-entity fallback in `isLocalDntTarget()` that treats all bot-attached enchantments as local. Local player Dnt enchantments always have `CONTROLLER == localPlayerID` — the bot fallback is unnecessary and actively harmful.

**Tech Stack:** Go 1.24, gamestate processor

**Test data:** `lastgames/Hearthstone_2026_03_20_20_10_45/Power.log` (4 duos games with beetle/undead leakage), `lastgames/PartnerMacPlayer.log` (partner perspective for cross-reference).

**Testing rule:** Use `go test -count=1 ./path/...` during iteration. Only use `-race` on the final validation at the very end.

---

### Task 1: Write regression test proving leakage

**Files:**
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write test that simulates opponent beetle Dnt leaking via bot entity**

The test creates a duos game, then simulates an opponent's beetle Dnt enchantment with CONTROLLER=bot and ATTACHED=bot entity. It should NOT appear in local buff sources.

```go
func TestDuosBotAttachedOpponentDntDoesNotLeakToLocal(t *testing.T) {
	m := NewMachine()
	p := NewProcessor(m)

	// CREATE_GAME: local=PlayerID 4 (EntityID 11), bot=PlayerID 12 (EntityID 12).
	p.Process(parser.GameEvent{
		Type: parser.EventCreateGame,
		Players: []parser.PlayerInfo{
			{PlayerID: 4, EntityID: 11, BattleTag: "Local#1234", AccountHi: "144115193835963207"},
			{PlayerID: 12, EntityID: 12, BattleTag: "", AccountHi: "0"},
		},
		GameEntityTags:     map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "1"},
		DuoPartnerPlayerID: 5,
	})

	// Opponent's beetle Dnt enchantment: CONTROLLER=12 (bot), ATTACHED=12 (bot entity).
	// This is a combat copy — in Power.log these appear as SHOW_ENTITY during combat.
	p.Process(parser.GameEvent{
		Type:     parser.EventShowEntity,
		EntityID: 22684,
		CardID:   "BG31_808pe",
		Tags: map[string]string{
			"CONTROLLER":           "12",
			"CARDTYPE":             "ENCHANTMENT",
			"ATTACHED":             "12",
			"ZONE":                 "PLAY",
			"TAG_SCRIPT_DATA_NUM_1": "26",
			"TAG_SCRIPT_DATA_NUM_2": "13",
		},
	})

	// TAG_CHANGE updating the beetle Dnt values (combat progression).
	p.Process(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 22684,
		Tag:      "TAG_SCRIPT_DATA_NUM_1",
		Value:    "30",
	})
	p.Process(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 22684,
		Tag:      "TAG_SCRIPT_DATA_NUM_2",
		Value:    "15",
	})

	state := m.State()
	for _, bs := range state.BuffSources {
		if bs.Category == "BEETLE" {
			t.Errorf("opponent beetle Dnt leaked to local via bot entity: ATK=%d HP=%d", bs.Attack, bs.Health)
		}
	}
}
```

**Step 2: Run test to verify it fails (proving the leakage exists)**

Run: `go test -count=1 -run TestDuosBotAttachedOpponentDntDoesNotLeakToLocal ./internal/gamestate/`
Expected: FAIL — "opponent beetle Dnt leaked to local via bot entity"

**Step 3: Commit the failing test**

```bash
git add internal/gamestate/gamestate_test.go
git commit -m "test: prove opponent Dnt buff leakage via bot entity in duos

Beetle Dnt enchantment with CONTROLLER=bot ATTACHED=bot incorrectly
counted in local buff sources. This test will pass after the fix."
```

---

### Task 2: Fix isLocalDntTarget bot-entity fallback

**Files:**
- Modify: `internal/gamestate/processor.go:1175-1199`

**Step 1: Remove the bot-entity fallback**

In `internal/gamestate/processor.go`, replace the `isLocalDntTarget` function:

```go
// isLocalDntTarget returns true if entityID is a Dnt enchantment attached to a
// local player entity — i.e., the local player entity or the local hero.
//
// In Duos, local player Dnt enchantments always have CONTROLLER == localPlayerID
// and ATTACHED == local player entity. Opponent combat copy Dnt enchantments have
// CONTROLLER == botPlayerID and ATTACHED == bot entity. We must NOT treat
// bot-attached enchantments as local — that was the source of buff leakage.
func (p *Processor) isLocalDntTarget(entityID int) bool {
	info := p.entityProps[entityID]
	if info == nil {
		return false
	}
	if info.AttachedTo > 0 {
		if info.AttachedTo == p.localHeroID {
			return true
		}
		if pid, ok := p.playerEntityIDs[info.AttachedTo]; ok {
			if pid == p.localPlayerID {
				return true
			}
		}
	}
	return false
}
```

The key change: remove lines 1191-1194 that treated bot-entity attachments as local in duos.

**Step 2: Run the regression test**

Run: `go test -count=1 -run TestDuosBotAttachedOpponentDntDoesNotLeakToLocal ./internal/gamestate/`
Expected: PASS

**Step 3: Run all gamestate tests**

Run: `go test -count=1 ./internal/gamestate/`
Expected: All pass. Existing buff tracking tests use CONTROLLER == localPlayerID on their enchantments, so they should be unaffected.

**Step 4: Verify with reparse + TUI dump**

```bash
echo "yes" | ./battlestream db-reset 2>/dev/null
./battlestream reparse 2>/dev/null
./battlestream daemon > /tmp/bs-daemon.log 2>&1 &
sleep 2
./battlestream tui --dump --width 160
```

Expected: BUFF SOURCES panel should NOT show Beetles or Undead (unless the local player legitimately earned them). Kill the daemon after verification.

**Step 5: Commit**

```bash
git add internal/gamestate/processor.go
git commit -m "fix: remove bot-entity fallback in isLocalDntTarget (duos buff leakage)

In duos, isLocalDntTarget() treated any Dnt enchantment attached to the
bot player entity as local. This is wrong — opponent combat copy Dnt
enchantments are also attached to the bot entity.

Local player Dnt always has CONTROLLER == localPlayerID and
ATTACHED == local player entity. The bot fallback was unnecessary
and caused opponent beetle/undead/etc buffs to leak into local stats.

Verified across 4 duos games: no legitimate local Dnt uses the bot
entity path."
```

---

### Task 3: Add test for local player Dnt still working

**Files:**
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write test proving local player Dnt still tracked correctly after fix**

```go
func TestDuosLocalPlayerDntStillTracked(t *testing.T) {
	m := NewMachine()
	p := NewProcessor(m)

	// CREATE_GAME: local=PlayerID 4 (EntityID 11), bot=PlayerID 12 (EntityID 12).
	p.Process(parser.GameEvent{
		Type: parser.EventCreateGame,
		Players: []parser.PlayerInfo{
			{PlayerID: 4, EntityID: 11, BattleTag: "Local#1234", AccountHi: "144115193835963207"},
			{PlayerID: 12, EntityID: 12, BattleTag: "", AccountHi: "0"},
		},
		GameEntityTags:     map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "1"},
		DuoPartnerPlayerID: 5,
	})

	// Local player's beetle Dnt: CONTROLLER=4 (local), ATTACHED=11 (local player entity).
	p.Process(parser.GameEvent{
		Type:     parser.EventShowEntity,
		EntityID: 500,
		CardID:   "BG31_808pe",
		Tags: map[string]string{
			"CONTROLLER":           "4",
			"CARDTYPE":             "ENCHANTMENT",
			"ATTACHED":             "11",
			"ZONE":                 "PLAY",
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
```

**Step 2: Run test**

Run: `go test -count=1 -run TestDuosLocalPlayerDntStillTracked ./internal/gamestate/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/gamestate/gamestate_test.go
git commit -m "test: verify local player Dnt still tracked in duos after bot-entity fix"
```

---

### Task 4: Final validation

**Step 1: Run full test suite with race detector**

Run: `go test -race -count=1 ./...`
Expected: All pass.

**Step 2: Run vet**

Run: `go vet ./...`
Expected: No issues.

**Step 3: Build**

Run: `go build ./cmd/battlestream`
Expected: Clean.
