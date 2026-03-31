# Fix Duos Detection False Positives — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix false-positive duos detection caused by Hearthstone now emitting `BACON_DUOS_PUNISH_LEAVERS=1` in all BG games, not just duos.

**Architecture:** Demote `BACON_DUOS_PUNISH_LEAVERS` and `BACON_DUO_PASSABLE` from standalone duos signals to backup-only (require both). Keep `BACON_DUO_TEAMMATE_PLAYER_ID` as the sole authoritative signal. Add unset mechanism when `PUNISH_LEAVERS` changes to 0.

**Tech Stack:** Go, no new dependencies.

---

### Task 1: Update existing tests to expect new behavior

**Files:**
- Modify: `internal/gamestate/gamestate_test.go:2224-2279` (two existing tests)

**Step 1: Update `TestDuosDetectionViaPunishLeavers` (line 2224)**

This test currently sends only `BACON_DUOS_PUNISH_LEAVERS=1` and expects `IsDuos=true`. Under the new logic, PUNISH_LEAVERS alone is NOT sufficient. Change expectation to `IsDuos=false`.

```go
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
		t.Error("expected IsDuos=false — PUNISH_LEAVERS alone no longer triggers duos (HS patch 2026-03)")
	}
	if p.partnerPlayerID != 0 {
		t.Errorf("expected partnerPlayerID=0, got %d", p.partnerPlayerID)
	}
}
```

**Step 2: Update `TestDuosDetectionViaDuoPassable` (line 2254)**

This test sends only `BACON_DUO_PASSABLE=1` and expects `IsDuos=true`. Under the new logic, DUO_PASSABLE alone is NOT sufficient. Change expectation to `IsDuos=false`.

```go
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

	// BACON_DUO_PASSABLE alone no longer triggers duos detection
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1143,
		Tags: map[string]string{"BACON_DUO_PASSABLE": "1"},
	})

	if m.State().IsDuos {
		t.Error("expected IsDuos=false — DUO_PASSABLE alone no longer triggers duos (HS patch 2026-03)")
	}
}
```

**Step 3: Run tests to verify they FAIL (implementation not changed yet)**

Run: `go test -count=1 -run 'TestDuosDetectionViaPunishLeavers|TestDuosDetectionViaDuoPassable' ./internal/gamestate/`
Expected: Both tests FAIL — the old code still sets IsDuos=true from these signals alone.

**Step 4: Commit**

```
git add internal/gamestate/gamestate_test.go
git commit -m "test: update duos detection tests to expect new signal hierarchy

PUNISH_LEAVERS alone and DUO_PASSABLE alone should no longer trigger
duos mode. HS patch 2026-03 emits PUNISH_LEAVERS=1 for all BG games."
```

---

### Task 2: Add new tests for combined signal and unset behavior

**Files:**
- Modify: `internal/gamestate/gamestate_test.go` (add 3 new test functions after line 2279)

**Step 1: Add `TestDuosDetectionCombinedPunishLeaversAndPassable`**

Tests that PUNISH_LEAVERS=1 + DUO_PASSABLE=1 together DO trigger duos.

```go
func TestDuosDetectionCombinedPunishLeaversAndPassable(t *testing.T) {
	m, p := newProc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	// GameEntity tags with BACON_DUOS_PUNISH_LEAVERS
	p.Handle(parser.GameEvent{
		Type: parser.EventGameEntityTags,
		Tags: map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "1"},
	})
	// Local player (no BACON_DUO_TEAMMATE_PLAYER_ID)
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 14, PlayerID: 5,
		Tags: map[string]string{"hi": "144115193835963207", "lo": "30722021", "PLAYER_ID": "5"},
	})
	p.Handle(parser.GameEvent{
		Type: parser.EventPlayerDef, EntityID: 15, PlayerID: 13,
		Tags: map[string]string{"hi": "0", "lo": "0", "PLAYER_ID": "13"},
	})

	if m.State().IsDuos {
		t.Fatal("should not be duos from PUNISH_LEAVERS alone")
	}

	// DUO_PASSABLE confirms duos when PUNISH_LEAVERS is also active
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1143,
		Tags: map[string]string{"BACON_DUO_PASSABLE": "1"},
	})

	if !m.State().IsDuos {
		t.Error("expected IsDuos=true from combined PUNISH_LEAVERS + DUO_PASSABLE")
	}
}
```

**Step 2: Add `TestDuosUnsetViaPunishLeaversZero`**

Tests that `PUNISH_LEAVERS=0` TAG_CHANGE clears duos when it was set only via the backup path.

```go
func TestDuosUnsetViaPunishLeaversZero(t *testing.T) {
	m, p := newProc()
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
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
	// Both signals fire → duos activated via backup path
	p.Handle(parser.GameEvent{
		Type: parser.EventTagChange, EntityID: 1143,
		Tags: map[string]string{"BACON_DUO_PASSABLE": "1"},
	})
	if !m.State().IsDuos {
		t.Fatal("precondition: expected IsDuos=true from combined signals")
	}

	// PUNISH_LEAVERS goes to 0 → clears duos (was not from TEAMMATE_PLAYER_ID)
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityName: "GameEntity",
		Tags:       map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "0"},
	})
	if m.State().IsDuos {
		t.Error("expected IsDuos=false after PUNISH_LEAVERS changed to 0")
	}
}
```

**Step 3: Add `TestDuosNotUnsetWhenFromTeammate`**

Tests that `PUNISH_LEAVERS=0` does NOT clear duos when it was set via TEAMMATE_PLAYER_ID (authoritative).

```go
func TestDuosNotUnsetWhenFromTeammate(t *testing.T) {
	m, p := newProc()
	setupDuosGame(p) // sets duos via BACON_DUO_TEAMMATE_PLAYER_ID

	if !m.State().IsDuos {
		t.Fatal("precondition: expected IsDuos=true from TEAMMATE_PLAYER_ID")
	}

	// PUNISH_LEAVERS goes to 0 → should NOT clear duos (authoritative source was TEAMMATE)
	p.Handle(parser.GameEvent{
		Type:       parser.EventTagChange,
		EntityName: "GameEntity",
		Tags:       map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "0"},
	})
	if !m.State().IsDuos {
		t.Error("duos should remain true — was confirmed via TEAMMATE_PLAYER_ID")
	}
}
```

**Step 4: Run ALL new tests to verify they FAIL**

Run: `go test -count=1 -run 'TestDuosDetectionCombined|TestDuosUnset|TestDuosNotUnset' ./internal/gamestate/`
Expected: All 3 tests FAIL — implementation not yet changed.

**Step 5: Commit**

```
git add internal/gamestate/gamestate_test.go
git commit -m "test: add tests for combined duos signals and unset behavior"
```

---

### Task 3: Implement processor changes

**Files:**
- Modify: `internal/gamestate/processor.go:64-99` (Processor struct — add 2 fields)
- Modify: `internal/gamestate/processor.go:155-186` (EventGameStart reset — add 2 lines)
- Modify: `internal/gamestate/processor.go:234-243` (EventGameEntityTags — demote Signal 2)
- Modify: `internal/gamestate/processor.go:296-303` (handlePlayerDef — add duosFromTeammate)
- Modify: `internal/gamestate/processor.go:370-377` (handleTagChange BACON_DUO_PASSABLE — demote Signal 3)
- Add: new case in `handleTagChange` for `BACON_DUOS_PUNISH_LEAVERS` TAG_CHANGE

**Step 1: Add new fields to Processor struct (after line 79)**

Add these two fields after `isDuos bool`:

```go
	isDuos              bool
	punishLeaversActive bool // BACON_DUOS_PUNISH_LEAVERS=1 seen (not sufficient alone)
	duosFromTeammate    bool // duos confirmed via BACON_DUO_TEAMMATE_PLAYER_ID (authoritative)
```

**Step 2: Reset new fields in EventGameStart (after line 165)**

After `p.isDuos = false`, add:

```go
		p.isDuos = false
		p.punishLeaversActive = false
		p.duosFromTeammate = false
```

**Step 3: Demote Signal 2 — EventGameEntityTags (lines 234-243)**

Replace the entire `case parser.EventGameEntityTags:` block:

```go
	case parser.EventGameEntityTags:
		for tag, value := range e.Tags {
			if tag == "BACON_DUOS_PUNISH_LEAVERS" && value == "1" {
				p.punishLeaversActive = true
				slog.Info("PUNISH_LEAVERS flag recorded (not sufficient alone for duos)", "tag", tag)
			}
		}
```

**Step 4: Set duosFromTeammate in handlePlayerDef (lines 297-303)**

Add `p.duosFromTeammate = true` to the TEAMMATE_PLAYER_ID handler:

```go
		// Check for Duos tag in the Player block.
		if duoStr := e.Tags["BACON_DUO_TEAMMATE_PLAYER_ID"]; duoStr != "" {
			if partnerID, err := strconv.Atoi(duoStr); err == nil && partnerID > 0 {
				p.isDuos = true
				p.duosFromTeammate = true
				p.partnerPlayerID = partnerID
				p.machine.SetDuosMode(true)
				slog.Info("Duos detected from player def", "partnerPlayerID", partnerID)
			}
		}
```

**Step 5: Demote Signal 3 — handleTagChange BACON_DUO_PASSABLE (lines 372-377)**

Replace the `case "BACON_DUO_PASSABLE":` block:

```go
		case "BACON_DUO_PASSABLE":
			if value == "1" && !p.isDuos && p.punishLeaversActive {
				p.isDuos = true
				p.machine.SetDuosMode(true)
				slog.Info("Duos detected from combined PUNISH_LEAVERS + DUO_PASSABLE")
			}
```

**Step 6: Add PUNISH_LEAVERS=0 unset handler in handleTagChange (after the DUO_PASSABLE case)**

Add a new case inside the `for tag, value := range e.Tags` switch block, right after the `BACON_DUO_PASSABLE` case:

```go
		case "BACON_DUOS_PUNISH_LEAVERS":
			if value == "0" && p.isDuos && !p.duosFromTeammate {
				p.isDuos = false
				p.punishLeaversActive = false
				p.machine.SetDuosMode(false)
				slog.Info("Duos cleared — PUNISH_LEAVERS changed to 0 (backup-only detection)")
			}
```

**Step 7: Run the updated + new tests**

Run: `go test -count=1 -run 'TestDuosDetectionViaPunishLeavers|TestDuosDetectionViaDuoPassable|TestDuosDetectionCombined|TestDuosUnset|TestDuosNotUnset|TestDuosDetection$|TestSoloGameNoDuos' ./internal/gamestate/`
Expected: ALL PASS.

**Step 8: Commit**

```
git add internal/gamestate/processor.go
git commit -m "fix: demote PUNISH_LEAVERS and DUO_PASSABLE from standalone duos signals

BACON_DUOS_PUNISH_LEAVERS=1 now appears in all BG games (HS patch
2026-03). BACON_DUO_PASSABLE fires on card properties in singles.

Signal hierarchy:
- BACON_DUO_TEAMMATE_PLAYER_ID: authoritative, sets duos immediately
- PUNISH_LEAVERS + DUO_PASSABLE combined: backup confirmation
- PUNISH_LEAVERS=0 TAG_CHANGE: clears duos if not from TEAMMATE"
```

---

### Task 4: Implement SetDuosMode(false) state cleanup

**Files:**
- Modify: `internal/gamestate/state.go:643-650` (SetDuosMode method)

**Step 1: Write a test for SetDuosMode(false)**

Add to `gamestate_test.go`:

```go
func TestSetDuosModeFalseClearsPartner(t *testing.T) {
	m := New()
	m.GameStart("test", time.Now())
	m.SetDuosMode(true)

	if !m.State().IsDuos || m.State().Partner == nil {
		t.Fatal("precondition: expected IsDuos=true and Partner!=nil")
	}

	m.SetDuosMode(false)
	s := m.State()
	if s.IsDuos {
		t.Error("expected IsDuos=false")
	}
	if s.Partner != nil {
		t.Error("expected Partner=nil after SetDuosMode(false)")
	}
	if s.PartnerBoard != nil {
		t.Error("expected PartnerBoard=nil after SetDuosMode(false)")
	}
}
```

**Step 2: Run to verify it FAILS**

Run: `go test -count=1 -run TestSetDuosModeFalseClearsPartner ./internal/gamestate/`
Expected: FAIL — Partner is not cleared.

**Step 3: Update SetDuosMode to handle false**

Replace lines 643-650 in `state.go`:

```go
// SetDuosMode enables/disables Duos tracking and initializes/clears partner state.
func (m *Machine) SetDuosMode(isDuos bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.IsDuos = isDuos
	if isDuos && m.state.Partner == nil {
		m.state.Partner = &PlayerState{}
	}
	if !isDuos {
		m.state.Partner = nil
		m.state.PartnerBoard = nil
	}
}
```

**Step 4: Run test to verify it PASSES**

Run: `go test -count=1 -run TestSetDuosModeFalseClearsPartner ./internal/gamestate/`
Expected: PASS.

**Step 5: Commit**

```
git add internal/gamestate/state.go internal/gamestate/gamestate_test.go
git commit -m "fix: SetDuosMode(false) clears Partner and PartnerBoard state"
```

---

### Task 5: Update `TestIntegrationDuosPunishLeavers` fixture

**Files:**
- Modify: `internal/gamestate/gamestate_test.go:2345-2357` (test expectations)
- Modify: `internal/gamestate/testdata/duos_punish_leavers.txt` (add TEAMMATE_PLAYER_ID to fixture)

The `duos_punish_leavers.txt` fixture has only `BACON_DUOS_PUNISH_LEAVERS=1` and no `BACON_DUO_TEAMMATE_PLAYER_ID` or `BACON_DUO_PASSABLE`. Under the new logic, this will correctly be treated as a singles game.

**Option A (preferred):** Add `BACON_DUO_TEAMMATE_PLAYER_ID` to the Player line in the fixture, making it a proper duos fixture. This preserves the test's purpose of validating a duos game detected through real Power.log data.

**Step 1: Check the fixture Player line**

Read the fixture to find the local Player EntityID line and add `BACON_DUO_TEAMMATE_PLAYER_ID` tag. The local player is `EntityID=14 PlayerID=5` (from line 44 of fixture). We need to find the second real player's PlayerID to use as the teammate value.

Run: `grep "GameAccountId=\[hi=" internal/gamestate/testdata/duos_punish_leavers.txt`

If there's a second real player, use their PlayerID. If not, add a synthetic one (e.g., value=6).

**Step 2: Add the tag to the fixture**

Add `tag=BACON_DUO_TEAMMATE_PLAYER_ID value=<partnerPlayerID>` after the existing Player entity tags (same timestamp prefix).

**Step 3: Run the integration test**

Run: `go test -count=1 -run TestIntegrationDuosPunishLeavers ./internal/gamestate/`
Expected: PASS (fixture now has authoritative signal).

**Step 4: Commit**

```
git add internal/gamestate/testdata/duos_punish_leavers.txt internal/gamestate/gamestate_test.go
git commit -m "test: add TEAMMATE_PLAYER_ID to duos_punish_leavers fixture

The fixture previously relied on PUNISH_LEAVERS alone. Since that
signal is no longer sufficient, add the authoritative signal."
```

---

### Task 6: Run full test suite

**Files:** None (verification only)

**Step 1: Run all gamestate tests**

Run: `go test -count=1 ./internal/gamestate/`
Expected: ALL PASS.

**Step 2: Run full project tests**

Run: `go test -count=1 ./...`
Expected: ALL PASS.

**Step 3: Run vet**

Run: `go vet ./...`
Expected: Clean.

---

### Task 7: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md:82-91` (Duos Support section)

**Step 1: Replace the duos detection docs**

Replace lines 84-87:

```markdown
Multi-signal duos detection (checked in order):
1. `BACON_DUO_TEAMMATE_PLAYER_ID` in CREATE_GAME Player block (preferred — identifies partner immediately)
2. `BACON_DUOS_PUNISH_LEAVERS` in GameEntity block tags (new — `EventGameEntityTags`)
3. `BACON_DUO_PASSABLE` TAG_CHANGE on card entities (fallback)
```

With:

```markdown
Duos detection signal hierarchy (HS patch 2026-03 changed `BACON_DUOS_PUNISH_LEAVERS` to appear in ALL BG games):
1. `BACON_DUO_TEAMMATE_PLAYER_ID` in CREATE_GAME Player block — **authoritative**, immediately sets duos
2. `BACON_DUOS_PUNISH_LEAVERS=1` + `BACON_DUO_PASSABLE=1` combined — backup; neither alone is sufficient
3. `BACON_DUOS_PUNISH_LEAVERS=0` TAG_CHANGE — clears duos if set only via backup path (not TEAMMATE_PLAYER_ID)
```

**Step 2: Commit**

```
git add CLAUDE.md
git commit -m "docs: update duos detection signal hierarchy in CLAUDE.md"
```

---

### Task 8: Reparse and verify

**Step 1: Backup BadgerDB**

Run: `cp -r /home/moch/.battlestream/profiles/beeblebrox/data /home/moch/.battlestream/profiles/beeblebrox/data-backup-$(date +%Y%m%d-%H%M%S)`

**Step 2: Build**

Run: `go build ./cmd/battlestream`

**Step 3: Kill daemon, reset, reparse**

```
ps aux | grep battlestream | grep -v grep | awk '{print $2}' | xargs -r kill -9
echo "yes" | ./battlestream db-reset
./battlestream reparse
```

**Step 4: Restart daemon**

Run: `./battlestream daemon > /tmp/battlestream-daemon.log 2>&1 &`

**Step 5: Verify via gRPC — latest game should NOT be duos**

Run: `grpcurl -plaintext 127.0.0.1:50051 battlestream.v1.BattlestreamService/GetCurrentGame | grep isDuos`
Expected: Field should be absent (false) or `"isDuos": false`.

**Step 6: Verify dashboard Solo filter has data**

Open `http://127.0.0.1:8080/dashboard/`, click "Solo" mode button. Should show at least 1 game.

**Step 7: Verify TUI shows singles layout**

Run: `./battlestream tui --dump --width 120 | head -40`
Should NOT show "PARTNER BOARD" or "PARTNER BUFFS" panels if current game is singles.
