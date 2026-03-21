# Duos Dnt Combat Split Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** In duos, split absolute Dnt buff sources (beetle, rightmost, whelp, undead, volumizer, bloodgem barrage) into local vs partner contributions using combat phase markers.

**Architecture:** `BACON_CURRENT_COMBAT_PLAYER_ID` already fires before each player's combat, and the processor already tracks `partnerCombatActive`. During partner combat, any Dnt TAG_CHANGE delta is the partner's contribution. We add `dntTeamTotal` and `dntPartnerAccum` maps to the Processor, and a `handleAbsoluteDntDuos` function that splits absolute Dnt values into local/partner using differential accumulation keyed on `partnerCombatActive`.

**Tech Stack:** Go 1.24, gamestate processor

**Test data:** `lastgames/Hearthstone_2026_03_20_20_10_45/Power.log` — game starting at line 2291292 (game-1774071196195). Local=PlayerID 7 (Moch), Partner=PlayerID 8 (Bernkastel). On turn 15, beetles show +67/+59 — all of it from partner combat (no beetle updates during local combat). Verified: `BACON_CURRENT_COMBAT_PLAYER_ID value=8` always precedes beetle TAG_CHANGE SD updates in this game.

**Testing rule:** Use `go test -count=1 ./path/...` during iteration. Only use `-race` on the final validation at the very end.

---

### Task 1: Add duos Dnt tracking fields to Processor

**Files:**
- Modify: `internal/gamestate/processor.go:86-87` (after `combatPhaseEntityIDs`)
- Modify: `internal/gamestate/processor.go:160-167` (game reset block)

**Step 1: Add fields to Processor struct**

After line 86 (`combatPhaseEntityIDs []int`), add:

```go
	// Duos absolute Dnt split tracking — separates local vs partner contributions.
	// dntTeamTotal tracks the last-seen raw SD value per category (no base offset).
	// dntPartnerAccum tracks the accumulated partner delta per category.
	dntTeamTotal    map[string][2]int // category → [sd1, sd2] raw values
	dntPartnerAccum map[string][2]int // category → [sd1, sd2] partner deltas
```

**Step 2: Initialize in NewProcessor**

In the `NewProcessor` function (around line 130), add after `partnerBuffs`:

```go
		dntTeamTotal:    make(map[string][2]int),
		dntPartnerAccum: make(map[string][2]int),
```

**Step 3: Reset on game start**

In the game-start reset block (around line 166, after `p.partnerBuffs = newBuffTracker()`), add:

```go
		p.dntTeamTotal = make(map[string][2]int)
		p.dntPartnerAccum = make(map[string][2]int)
```

**Step 4: Verify build**

Run: `go build ./cmd/battlestream`
Expected: Clean compile.

**Step 5: Commit**

```bash
git add internal/gamestate/processor.go
git commit -m "refactor: add duos Dnt split tracking fields to Processor"
```

---

### Task 2: Write handleAbsoluteDntDuos function

**Files:**
- Modify: `internal/gamestate/processor.go` (after `handleAbsoluteDnt` at line ~1577)

**Step 1: Add the duos split function**

Insert after `handleAbsoluteDnt`:

```go
// handleAbsoluteDntDuos splits an absolute Dnt value into local vs partner
// contributions in duos. Uses partnerCombatActive to attribute deltas:
// increments during partner combat go to partner, all others go to local.
func (p *Processor) handleAbsoluteDntDuos(category string, isSD1 bool, value, baseAtk, baseHp int) {
	idx := 0
	base := baseAtk
	if !isSD1 {
		idx = 1
		base = baseHp
	}

	prev := p.dntTeamTotal[category]
	delta := value - prev[idx]
	if delta < 0 {
		// New combat copy with lower value than accumulated — treat as reset.
		delta = 0
	}

	// Update team total.
	prev[idx] = value
	p.dntTeamTotal[category] = prev

	// Attribute delta to partner if their combat is active.
	if p.partnerCombatActive && delta > 0 {
		accum := p.dntPartnerAccum[category]
		accum[idx] += delta
		p.dntPartnerAccum[category] = accum
	}

	// Compute display values.
	teamTotal := p.dntTeamTotal[category]
	partnerAccum := p.dntPartnerAccum[category]

	localAtk := baseAtk + teamTotal[0] - partnerAccum[0]
	localHp := baseHp + teamTotal[1] - partnerAccum[1]

	p.localBuffs.buffSourceState[category] = [2]int{localAtk, localHp}
	p.machine.SetBuffSource(category, localAtk, localHp)

	if partnerAccum[0] > 0 || partnerAccum[1] > 0 {
		p.partnerBuffs.buffSourceState[category] = [2]int{partnerAccum[0], partnerAccum[1]}
		p.machine.SetPartnerBuffSource(category, partnerAccum[0], partnerAccum[1])
	}
}
```

**Step 2: Modify handleAbsoluteDnt to dispatch to duos handler**

Replace the existing `handleAbsoluteDnt` function:

```go
// handleAbsoluteDnt sets a buff source from an absolute Dnt value plus base offset.
func (p *Processor) handleAbsoluteDnt(bt *buffTracker, setBS func(string, int, int), category string, isSD1 bool, value, baseAtk, baseHp int) {
	if p.isDuos {
		p.handleAbsoluteDntDuos(category, isSD1, value, baseAtk, baseHp)
		return
	}
	state := bt.buffSourceState[category]
	if isSD1 {
		state[0] = baseAtk + value
	} else {
		state[1] = baseHp + value
	}
	bt.buffSourceState[category] = state
	setBS(category, state[0], state[1])
}
```

**Step 3: Handle undead case (BG25_011pe)**

The undead Dnt (`BG25_011pe`) has a special inline handler in `handleDntTagChange` that bypasses `handleAbsoluteDnt`. Replace the existing case (around line 1552):

```go
	case "BG25_011pe":
		if isSD1 {
			if p.isDuos {
				p.handleAbsoluteDntDuos(CatUndead, true, value, 0, 0)
			} else {
				setBS(CatUndead, value, 0)
			}
		}
```

**Step 4: Verify build**

Run: `go build ./cmd/battlestream`
Expected: Clean compile.

**Step 5: Commit**

```bash
git add internal/gamestate/processor.go
git commit -m "feat: add handleAbsoluteDntDuos for combat-phase split tracking"
```

---

### Task 3: Write test for partner combat beetle attribution

**Files:**
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write test simulating partner combat beetle Dnt updates**

This test simulates a duos game where `BACON_CURRENT_COMBAT_PLAYER_ID` transitions to the partner, then beetle SD values increment during partner combat. The beetle buffs should appear under partner buffs, not local.

```go
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
```

**Step 2: Run test**

Run: `go test -count=1 -run TestDuosBeetleDntSplitByPartnerCombat ./internal/gamestate/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/gamestate/gamestate_test.go
git commit -m "test: verify beetle Dnt splits into local/partner by combat phase in duos"
```

---

### Task 4: Write test for mixed local+partner beetle contributions

**Files:**
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write test where both players contribute beetles**

This test simulates both local and partner having beetles. During local combat, SD increases (local beetles dying). During partner combat, SD increases more (partner beetles dying). The split should correctly attribute each delta.

```go
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
```

**Step 2: Run test**

Run: `go test -count=1 -run TestDuosBeetleDntMixedContributions ./internal/gamestate/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/gamestate/gamestate_test.go
git commit -m "test: verify mixed local+partner beetle contributions split correctly"
```

---

### Task 5: Write test for non-duos beetle still works

**Files:**
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write test confirming solo game beetle tracking is unchanged**

```go
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
```

**Step 2: Run test**

Run: `go test -count=1 -run TestSoloBeetleDntUnchanged ./internal/gamestate/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/gamestate/gamestate_test.go
git commit -m "test: verify solo game beetle tracking unaffected by duos split"
```

---

### Task 6: Run full test suite and verify with reparse

**Step 1: Run all gamestate tests**

Run: `go test -count=1 ./internal/gamestate/`
Expected: All pass.

**Step 2: Run full test suite with race detector**

Run: `go test -race -count=1 ./...`
Expected: All pass.

**Step 3: Run vet**

Run: `go vet ./...`
Expected: No issues.

**Step 4: Build**

Run: `go build ./cmd/battlestream`
Expected: Clean.
