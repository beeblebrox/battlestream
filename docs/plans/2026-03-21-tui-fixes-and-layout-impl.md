# TUI Bug Fixes and Layout Overhaul Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix 5 bugs (crash on tier 7, buff leakage, partner scroll, first-game keys, cobra help on error) and add 4 features (partner buff pane, proportional layout, drag-resize dividers, persisted split ratios).

**Architecture:** Phase 1 fixes bugs in gamestate processor and TUI. Phase 2 adds partner buff data pipeline (processor → state → proto → gRPC → TUI), then rebuilds TUI layout with ratio-based sizing, drag dividers, and config persistence.

**Tech Stack:** Go 1.24, Bubbletea/Lipgloss (TUI), gRPC/protobuf, Viper (config), BadgerDB (storage)

**Test data:** `lastgames/Hearthstone_2026_03_20_20_10_45/Power.log` (4 duos games, tier 7 anomaly), `lastgames/PartnerMacPlayer.log` (partner perspective).

**Testing rule:** Use `go test -count=1 ./path/...` during iteration. Only use `-race` on the final validation at the very end.

---

## Phase 1: Bug Fixes

### Task 1: Cobra SilenceUsage

**Files:**
- Modify: `cmd/battlestream/main.go:40-45`

**Step 1: Add SilenceUsage and SilenceErrors to root command**

In `cmd/battlestream/main.go`, modify the root command definition:

```go
root := &cobra.Command{
    Use:   "battlestream",
    Short: "Hearthstone Battlegrounds stat tracker and overlay backend",
    Long: `battlestream monitors Hearthstone Battlegrounds games via log parsing,
persists aggregate stats, and exposes them via gRPC, REST, WebSocket, and file output.`,
    SilenceUsage:  true,
    SilenceErrors: true,
}
```

**Step 2: Build to verify no compile errors**

Run: `go build ./cmd/battlestream`
Expected: Clean build, no errors.

**Step 3: Commit**

```bash
git add cmd/battlestream/main.go
git commit -m "fix: add SilenceUsage/SilenceErrors to cobra root command

Prevents cobra from printing help/usage when RunE returns an error.
Previously, any TUI crash would show the full help text instead of
just the error message."
```

---

### Task 2: Tier 7+ Rendering

**Files:**
- Modify: `internal/tui/tui.go:857-881`
- Test: `internal/tui/tui_test.go`

**Step 1: Write failing test for tier 7 rendering**

Add to `internal/tui/tui_test.go`:

```go
func TestRenderTavernTierAnomalyTier7(t *testing.T) {
	result := renderTavernTier(7)
	if result == "" {
		t.Fatal("renderTavernTier(7) returned empty string")
	}
	if !strings.Contains(result, "7") {
		t.Errorf("expected tier 7 to contain '7', got %q", result)
	}
	// Should have 7 filled stars and 0 empty stars.
	if strings.Contains(result, "☆") {
		t.Errorf("tier 7 should have no empty stars, got %q", result)
	}
}
```

**Step 2: Run test to verify it passes (it should already — no crash, but empty stars are wrong)**

Run: `go test -count=1 -run TestRenderTavernTierAnomalyTier7 ./internal/tui/`

Check the output. `strings.Repeat("☆", 6-7)` returns `""` for negative count, so no crash. But we want to make sure it renders cleanly.

**Step 3: Fix renderTavernTier for tier 7+**

In `internal/tui/tui.go`, replace `renderTavernTier` and `tavernTierColor`:

```go
func renderTavernTier(tier int) string {
	if tier <= 0 {
		return styleValue.Render("—")
	}
	filled := tier
	empty := max(0, 6-tier)
	stars := strings.Repeat("★", filled) + strings.Repeat("☆", empty)
	color := tavernTierColor(tier)
	return lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("%d %s", tier, stars))
}

func tavernTierColor(tier int) lipgloss.Color {
	switch tier {
	case 1:
		return lipgloss.Color("244")
	case 2:
		return lipgloss.Color("33")
	case 3:
		return lipgloss.Color("34")
	case 4:
		return lipgloss.Color("214")
	case 5:
		return lipgloss.Color("202")
	case 6:
		return lipgloss.Color("196")
	case 7:
		return lipgloss.Color("201") // magenta — anomaly tier
	default:
		return lipgloss.Color("255")
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test -count=1 -run TestRenderTavernTier ./internal/tui/`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/tui.go internal/tui/tui_test.go
git commit -m "fix: handle tier 7+ in TUI rendering (anomaly support)

Use max(0, 6-tier) for empty stars to avoid negative Repeat count.
Add magenta color for tier 7 (anomaly-enabled games)."
```

---

### Task 3: Partner Pane Scroll Fix

**Files:**
- Modify: `internal/tui/tui.go:700-751`

**Step 1: Fix mouse wheel routing to include partner pane**

In `internal/tui/tui.go`, replace the mouse wheel routing in `handleMouse()` (lines 700-710):

```go
func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Wheel: route to whichever panel the cursor is over.
	if tea.MouseEvent(msg).IsWheel() {
		var cmd tea.Cmd
		// Check partner pane first (below main panels).
		if m.game != nil && m.game.IsDuos &&
			msg.Y >= m.partnerVPY && msg.Y < m.partnerVPY+m.partnerVPH {
			m.partnerBoardVP, cmd = m.partnerBoardVP.Update(msg)
		} else if msg.X >= m.width/2 {
			m.modsVP, cmd = m.modsVP.Update(msg)
		} else {
			m.boardVP, cmd = m.boardVP.Update(msg)
		}
		return m, cmd
	}
```

**Step 2: Add partner scrollbar to identifyScrollbar()**

Replace `identifyScrollbar` (lines 734-742):

```go
func (m *Model) identifyScrollbar(x, y int) (panel, trackY, trackH int) {
	switch {
	case x == m.boardScrollX && y >= m.boardVPY && y < m.boardVPY+m.boardVPH:
		return 0, m.boardVPY, m.boardVPH
	case x == m.modsScrollX && y >= m.modsVPY && y < m.modsVPY+m.modsVPH:
		return 1, m.modsVPY, m.modsVPH
	case m.game != nil && m.game.IsDuos &&
		x == m.partnerScrollX && y >= m.partnerVPY && y < m.partnerVPY+m.partnerVPH:
		return 2, m.partnerVPY, m.partnerVPH
	}
	return -1, 0, 0
}
```

**Step 3: Add partner panel to scrubAt()**

Replace `scrubAt` (lines 744-751):

```go
func (m *Model) scrubAt(y int) {
	switch m.scrubPanel {
	case 0:
		tuiScrollbarJump(&m.boardVP, y, m.scrubTrackY, m.scrubTrackH)
	case 1:
		tuiScrollbarJump(&m.modsVP, y, m.scrubTrackY, m.scrubTrackH)
	case 2:
		tuiScrollbarJump(&m.partnerBoardVP, y, m.scrubTrackY, m.scrubTrackH)
	}
}
```

**Step 4: Update scrubPanel comment**

In the Model struct (line 145), update the comment:

```go
scrubPanel  int // 0=board, 1=mods, 2=partner
```

**Step 5: Build and run existing tests**

Run: `go build ./cmd/battlestream && go test -count=1 ./internal/tui/`
Expected: Clean build and all tests pass.

**Step 6: Commit**

```bash
git add internal/tui/tui.go
git commit -m "fix: route mouse wheel and scrollbar to partner board pane

Partner viewport was created but never wired into mouse handling.
Add Y-position check for wheel events, partner scrollbar detection
in identifyScrollbar(), and case 2 in scrubAt()."
```

---

### Task 4: Buff Attribution Fix (Opponent/Partner Leakage)

This is the most complex fix. The issue: during combat, TAG_SCRIPT_DATA changes on enchantment entities can be processed before their CONTROLLER is registered, allowing opponent/partner buffs to count toward the local player.

**Files:**
- Modify: `internal/gamestate/processor.go:688-696, 1466-1506, 1512-1521`
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Write failing test proving buff leakage**

Add to `internal/gamestate/gamestate_test.go`. This test simulates a combat scenario where an opponent's beetle Dnt enchantment arrives before CONTROLLER is set:

```go
func TestCombatBeetleDoesNotLeakToLocal(t *testing.T) {
	m := NewMachine()
	p := NewProcessor(m)

	// Simulate CREATE_GAME with local player ID 5.
	p.Process(parser.GameEvent{
		Type: parser.EventCreateGame,
		Players: []parser.PlayerInfo{
			{PlayerID: 5, BattleTag: "Local#1234", AccountHi: "1234"},
			{PlayerID: 6, BattleTag: "Opponent#5678"},
		},
	})

	// Move to combat phase.
	p.Process(parser.GameEvent{
		Type:     parser.EventTagChange,
		Entity:   "GameEntity",
		Tag:      "STEP",
		Value:    "MAIN_COMBAT",
	})

	// Opponent beetle Dnt enchantment created in FULL_ENTITY (controller=6).
	p.Process(parser.GameEvent{
		Type:     parser.EventFullEntity,
		EntityID: 9999,
		CardID:   "BG31_808pe",
		Tags: map[string]string{
			"CONTROLLER":           "6",
			"CARDTYPE":             "ENCHANTMENT",
			"ATTACHED":             "8888",
			"TAG_SCRIPT_DATA_NUM_1": "5",
			"TAG_SCRIPT_DATA_NUM_2": "3",
		},
	})

	// TAG_CHANGE on the same entity with SD values (simulates late arrival).
	p.Process(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 9999,
		Tag:      "TAG_SCRIPT_DATA_NUM_1",
		Value:    "10",
	})

	state := m.State()
	for _, bs := range state.BuffSources {
		if bs.Category == "BEETLE" {
			t.Errorf("opponent beetle buff leaked to local: ATK=%d HP=%d", bs.Attack, bs.Health)
		}
	}
}
```

**Step 2: Run test to see if it fails (proving leakage)**

Run: `go test -count=1 -run TestCombatBeetleDoesNotLeakToLocal ./internal/gamestate/`

If it passes (no leakage detected), the guard is working for this case. Adjust the test to match the actual leakage path found during investigation. The key scenario to test is when `entityController[entityID]` is 0 (unset) at the time of TAG_CHANGE processing.

**Step 3: Tighten the TAG_SCRIPT_DATA guard**

In `internal/gamestate/processor.go`, at lines 688-696, replace the guard:

```go
case "TAG_SCRIPT_DATA_NUM_1", "TAG_SCRIPT_DATA_NUM_2":
	if e.EntityID > 0 {
		p.updateEnchantmentScriptData(e.EntityID, tag, value)
		ctrl := p.entityController[e.EntityID]
		// Require explicit local ownership — reject if controller unknown (0)
		// or belongs to someone else.
		if ctrl == p.localPlayerID || p.isLocalDntTarget(e.EntityID) {
			p.handleDntTagChange(e.EntityID, tag, parseInt(value))
		}
	}
```

The existing code already has this guard, but the leakage comes through `handleEnchantmentEntity` at line 1499. Tighten that path too:

At lines 1497-1506, add an explicit zero-controller check:

```go
// Process initial SD values from FULL_ENTITY/SHOW_ENTITY as counter updates.
if info.ScriptData1 != 0 || info.ScriptData2 != 0 {
	enchCtrl := p.entityController[e.EntityID]
	if enchCtrl == p.localPlayerID || (enchCtrl != 0 && p.isLocalDntTarget(e.EntityID)) {
		if info.ScriptData1 != 0 {
			p.handleDntTagChange(e.EntityID, "TAG_SCRIPT_DATA_NUM_1", info.ScriptData1)
		}
		if info.ScriptData2 != 0 {
			p.handleDntTagChange(e.EntityID, "TAG_SCRIPT_DATA_NUM_2", info.ScriptData2)
		}
	}
}
```

Also tighten the `handleDntTagChange` guard at line 1518-1521 to reject controller 0:

```go
ctrl := p.entityController[entityID]
if ctrl == 0 || (ctrl != p.localPlayerID && !p.isLocalDntTarget(entityID)) {
	return
}
```

**Step 4: Run test to verify it passes**

Run: `go test -count=1 -run TestCombatBeetle ./internal/gamestate/`
Expected: PASS

**Step 5: Run full gamestate tests**

Run: `go test -count=1 ./internal/gamestate/`
Expected: All tests pass (existing buff tracking tests should be unaffected since local player entities always have CONTROLLER set).

**Step 6: Commit**

```bash
git add internal/gamestate/processor.go internal/gamestate/gamestate_test.go
git commit -m "fix: reject buff attribution for entities with unknown controller

During combat, enchantment TAG_SCRIPT_DATA events could arrive before
CONTROLLER was registered (controller=0). Tighten guards in
handleDntTagChange and handleEnchantmentEntity to require explicit
local player controller, preventing opponent/partner buff leakage."
```

---

### Task 5: First-Game Key/Streak Bug Investigation

**Files:**
- Modify: `internal/tui/combined.go:75-160`
- Test: `internal/tui/tui_test.go`

**Step 1: Add debug logging for key forwarding**

In `internal/tui/combined.go`, add logging after the key switch in `Update()`:

```go
case tea.KeyMsg:
	switch msg.String() {
	case "tab":
		return c.switchMode()
	case "q", "ctrl+c":
		if c.mode == modeLive {
			c.live.cancel()
			if c.live.client != nil {
				c.live.client.Close()
			}
		}
		return c, tea.Quit
	default:
		slog.Debug("combined: forwarding key to live model", "key", msg.String(), "mode", c.mode)
	}
```

**Step 2: Write a unit test verifying key forwarding in CombinedModel**

Add to `internal/tui/tui_test.go`:

```go
func TestCombinedModelForwardsKeysToLive(t *testing.T) {
	m := New("127.0.0.1:50051")
	m.connState = stateConnected
	m.game = &bspb.GameState{Phase: "RECRUIT"}
	m.showLastResult = true

	// Simulate "l" key directly on the live model.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.showLastResult {
		t.Error("expected showLastResult to toggle to false after 'l' key")
	}

	// Toggle back.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if !m.showLastResult {
		t.Error("expected showLastResult to toggle back to true after second 'l' key")
	}
}
```

**Step 3: Run test**

Run: `go test -count=1 -run TestCombinedModelForwardsKeysToLive ./internal/tui/`
Expected: PASS (this verifies the direct path works)

**Step 4: Build and run all TUI tests**

Run: `go test -count=1 ./internal/tui/`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/tui/combined.go internal/tui/tui_test.go
git commit -m "fix: add debug logging for key forwarding in CombinedModel

Adds slog.Debug trace when keys are forwarded to the live model.
Also adds test verifying key toggle behavior on the live Model directly.
Helps diagnose the reported first-game key/streak bug."
```

---

## Phase 2: New Features

### Task 6: Partner Buff Data Pipeline — State and Proto

**Files:**
- Modify: `internal/gamestate/state.go` (add PartnerBuffSources, PartnerAbilityCounters to BGGameState)
- Modify: `internal/gamestate/machine.go` (add setter methods)
- Modify: `proto/battlestream/v1/game.proto` (unreserve fields 20-21)
- Regenerate: `internal/api/grpc/gen/` (via `scripts/gen-proto.sh`)

**Step 1: Add partner buff fields to BGGameState**

In `internal/gamestate/state.go`, add to `BGGameState` after `PartnerBoard`:

```go
// Duos fields
IsDuos              bool              `json:"is_duos,omitempty"`
Partner             *PlayerState      `json:"partner,omitempty"`
PartnerBoard        *PartnerBoard     `json:"partner_board,omitempty"`
PartnerBuffSources  []BuffSource      `json:"partner_buff_sources,omitempty"`
PartnerAbilityCounters []AbilityCounter `json:"partner_ability_counters,omitempty"`
```

**Step 2: Add Machine methods for partner buff sources**

In `internal/gamestate/machine.go`, add methods (pattern mirrors existing `SetBuffSource`/`SetAbilityCounter`):

```go
func (m *Machine) SetPartnerBuffSource(category string, atk, hp int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, bs := range m.state.PartnerBuffSources {
		if bs.Category == category {
			m.state.PartnerBuffSources[i].Attack = atk
			m.state.PartnerBuffSources[i].Health = hp
			return
		}
	}
	m.state.PartnerBuffSources = append(m.state.PartnerBuffSources, BuffSource{
		Category: category, Attack: atk, Health: hp,
	})
}

func (m *Machine) SetPartnerAbilityCounter(category string, value int, display string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, ac := range m.state.PartnerAbilityCounters {
		if ac.Category == category {
			m.state.PartnerAbilityCounters[i].Value = value
			m.state.PartnerAbilityCounters[i].Display = display
			return
		}
	}
	m.state.PartnerAbilityCounters = append(m.state.PartnerAbilityCounters, AbilityCounter{
		Category: category, Value: value, Display: display,
	})
}
```

**Step 3: Update proto — unreserve fields 20-21**

In `proto/battlestream/v1/game.proto`, replace `reserved 20, 21;` with:

```protobuf
repeated BuffSource partner_buff_sources = 20;
repeated AbilityCounter partner_ability_counters = 21;
```

**Step 4: Regenerate proto**

Run: `scripts/gen-proto.sh`
Expected: Clean generation, updated files in `internal/api/grpc/gen/`.

**Step 5: Update gRPC serialization**

In `internal/api/grpc/server.go`, in `gameStateToProto()`, add after the partner board serialization (after line 321):

```go
for _, bs := range s.PartnerBuffSources {
	gs.PartnerBuffSources = append(gs.PartnerBuffSources, buffSourceToProto(bs))
}
for _, ac := range s.PartnerAbilityCounters {
	gs.PartnerAbilityCounters = append(gs.PartnerAbilityCounters, abilityCounterToProto(ac))
}
```

**Step 6: Build and test**

Run: `go build ./cmd/battlestream && go test -count=1 ./internal/gamestate/ ./internal/api/grpc/`
Expected: Clean build and all tests pass.

**Step 7: Commit**

```bash
git add internal/gamestate/state.go internal/gamestate/machine.go \
       proto/battlestream/v1/game.proto internal/api/grpc/gen/ \
       internal/api/grpc/server.go
git commit -m "feat: add partner buff source data pipeline

Unreserve proto fields 20-21 for partner_buff_sources and
partner_ability_counters. Add BGGameState fields, Machine setter
methods, and gRPC serialization."
```

---

### Task 7: Partner Buff Tracking in Processor

**Files:**
- Modify: `internal/gamestate/processor.go` (add partnerBuffs tracker, route partner enchantments)
- Test: `internal/gamestate/gamestate_test.go`

**Step 1: Add partnerBuffs tracker to Processor**

In `internal/gamestate/processor.go`, add to the Processor struct (after `localBuffs`):

```go
localBuffs   buffTracker
partnerBuffs buffTracker // tracks partner buff sources from combat enchantments
```

Initialize in `NewProcessor` (or wherever `localBuffs` is initialized):

```go
localBuffs:   newBuffTracker(),
partnerBuffs: newBuffTracker(),
```

**Step 2: Reset partnerBuffs on new game**

Find where `localBuffs` is reset (on new game start) and add `partnerBuffs` reset alongside it.

**Step 3: Route partner enchantments to partnerBuffs**

In `handleDntTagChange`, instead of returning early for partner-controlled entities, route them to the partner tracker. Replace the guard at lines 1518-1521:

```go
ctrl := p.entityController[entityID]
isLocal := ctrl == p.localPlayerID || p.isLocalDntTarget(entityID)
isPartner := p.isDuos && ctrl == p.partnerPlayerID

if ctrl == 0 || (!isLocal && !isPartner) {
	return
}
```

Then in each handler call, pass the appropriate buffTracker. Refactor `handleAbsoluteDnt` and similar to accept a `*buffTracker` parameter:

```go
func (p *Processor) handleAbsoluteDnt(bt *buffTracker, setMachine func(string, int, int), category string, isSD1 bool, value, baseAtk, baseHp int) {
	state := bt.buffSourceState[category]
	if isSD1 {
		state[0] = baseAtk + value
	} else {
		state[1] = baseHp + value
	}
	bt.buffSourceState[category] = state
	setMachine(category, state[0], state[1])
}
```

In `handleDntTagChange`, select tracker and machine setter:

```go
bt := &p.localBuffs
setBS := p.machine.SetBuffSource
if isPartner {
	bt = &p.partnerBuffs
	setBS = p.machine.SetPartnerBuffSource
}
```

Then update all handler calls in the switch to pass `bt` and `setBS`.

**Step 4: Write test for partner buff routing**

```go
func TestPartnerBeetleBuffRoutedToPartnerSources(t *testing.T) {
	m := NewMachine()
	p := NewProcessor(m)

	// CREATE_GAME with duos.
	p.Process(parser.GameEvent{
		Type: parser.EventCreateGame,
		Players: []parser.PlayerInfo{
			{PlayerID: 5, BattleTag: "Local#1234", AccountHi: "1234"},
			{PlayerID: 6, BattleTag: "Partner#5678"},
		},
		GameEntityTags: map[string]string{"BACON_DUOS_PUNISH_LEAVERS": "1"},
		DuoPartnerPlayerID: 6,
	})

	// Partner beetle Dnt enchantment.
	p.Process(parser.GameEvent{
		Type:     parser.EventFullEntity,
		EntityID: 500,
		CardID:   "BG31_808pe",
		Tags: map[string]string{
			"CONTROLLER":           "6",
			"CARDTYPE":             "ENCHANTMENT",
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
```

**Step 5: Run tests**

Run: `go test -count=1 -run TestPartnerBeetle ./internal/gamestate/`
Expected: PASS

Run: `go test -count=1 ./internal/gamestate/`
Expected: All pass (existing local buff tests unchanged).

**Step 6: Commit**

```bash
git add internal/gamestate/processor.go internal/gamestate/gamestate_test.go
git commit -m "feat: route partner buff enchantments to separate tracker

Partner-controlled Dnt enchantments now accumulate in partnerBuffs
instead of being discarded. Exposes via PartnerBuffSources on state.
Local buff attribution tightened to reject controller=0."
```

---

### Task 8: Partner Buff Sources TUI Pane

**Files:**
- Modify: `internal/tui/tui.go` (add partnerModsVP viewport, render partner buffs pane)

**Step 1: Add partnerModsVP to Model struct**

In `internal/tui/tui.go`, add to Model struct alongside existing viewports:

```go
boardVP        viewport.Model
modsVP         viewport.Model
partnerBoardVP viewport.Model
partnerModsVP  viewport.Model
```

Add tracking fields:

```go
partnerModsScrollX, partnerModsVPY, partnerModsVPH int
```

**Step 2: Add partnerModsItems() content function**

```go
func (m *Model) partnerModsItems() string {
	var b strings.Builder
	if m.game == nil || len(m.game.PartnerBuffSources) == 0 {
		return styleDim.Render("(awaiting combat data)")
	}

	sources := make([]*bspb.BuffSource, len(m.game.PartnerBuffSources))
	copy(sources, m.game.PartnerBuffSources)
	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			totalI := abs32(sources[i].Attack) + abs32(sources[i].Health)
			totalJ := abs32(sources[j].Attack) + abs32(sources[j].Health)
			if totalJ > totalI {
				sources[i], sources[j] = sources[j], sources[i]
			}
		}
	}

	for _, bs := range sources {
		if bs.Attack == 0 && bs.Health == 0 {
			continue
		}
		name := buffCategoryDisplayName(bs.Category)
		color := buffCategoryColor(bs.Category)
		style := lipgloss.NewStyle().Foreground(color)
		line := fmt.Sprintf("%-14s +%d/+%d", name, bs.Attack, bs.Health)
		b.WriteString(style.Render(line) + "\n")
	}

	if len(m.game.PartnerAbilityCounters) > 0 {
		b.WriteString("\n" + styleTitle.Render("ABILITIES") + "\n")
		for _, ac := range m.game.PartnerAbilityCounters {
			name := buffCategoryDisplayName(ac.Category)
			color := buffCategoryColor(ac.Category)
			style := lipgloss.NewStyle().Foreground(color)
			line := fmt.Sprintf("%-14s %s", name, ac.Display)
			b.WriteString(style.Render(line) + "\n")
		}
	}

	return b.String()
}
```

**Step 3: Update View() to render partner row as two columns**

In `View()`, replace the partner board section (lines 405-435) to render partner board + partner buffs side-by-side using the same `colW` as the main panels:

```go
// ── Row 3 (Duos): Partner board | Partner buff sources ──────
var rowPartner string
if m.game != nil && m.game.IsDuos {
	partnerH := 5

	// Partner board (left column).
	m.partnerBoardVP.Width = vpContentW
	m.partnerBoardVP.Height = partnerH
	m.partnerBoardVP.MouseWheelEnabled = true
	m.partnerBoardVP.SetContent(m.partnerBoardItems())

	title := "PARTNER BOARD"
	if len(m.game.PartnerBoard) > 0 {
		if m.game.PartnerBoardStale {
			title = fmt.Sprintf("PARTNER BOARD (T%d — last seen)", m.game.PartnerBoardTurn)
		} else {
			title = fmt.Sprintf("PARTNER BOARD (T%d)", m.game.PartnerBoardTurn)
		}
	}

	m.partnerVPY = m.row2StartY + lipgloss.Height(row2) + 2
	m.partnerVPH = partnerH
	m.partnerScrollX = 2 + vpContentW
	partnerBoardVPView := lipgloss.JoinHorizontal(lipgloss.Top,
		m.partnerBoardVP.View(), tuiScrollbar(m.partnerBoardVP, partnerH))
	partnerBoardPanel := styleBorder.Width(colW).Render(
		styleTitle.Render(title) + "\n" + partnerBoardVPView)

	// Partner buffs (right column).
	m.partnerModsVP.Width = vpContentW
	m.partnerModsVP.Height = partnerH
	m.partnerModsVP.MouseWheelEnabled = true
	m.partnerModsVP.SetContent(m.partnerModsItems())

	partnerModsTitle := "PARTNER BUFFS"
	if len(m.game.PartnerBuffSources) > 0 && m.game.PartnerBoardStale {
		partnerModsTitle = "PARTNER BUFFS (last seen)"
	}

	m.partnerModsVPY = m.partnerVPY
	m.partnerModsVPH = partnerH
	m.partnerModsScrollX = (colW + 4) + 2 + vpContentW
	partnerModsVPView := lipgloss.JoinHorizontal(lipgloss.Top,
		m.partnerModsVP.View(), tuiScrollbar(m.partnerModsVP, partnerH))
	partnerModsPanel := styleBorder.Width(colW).Render(
		styleTitle.Render(partnerModsTitle) + "\n" + partnerModsVPView)

	rowPartner = lipgloss.JoinHorizontal(lipgloss.Top, partnerBoardPanel, partnerModsPanel)
}
```

**Step 4: Update mouse routing for 4 panels**

Update `handleMouse()` wheel routing to check Y range for partner row, then X for left/right:

```go
if tea.MouseEvent(msg).IsWheel() {
	var cmd tea.Cmd
	if m.game != nil && m.game.IsDuos &&
		msg.Y >= m.partnerVPY && msg.Y < m.partnerVPY+m.partnerVPH {
		if msg.X >= m.width/2 {
			m.partnerModsVP, cmd = m.partnerModsVP.Update(msg)
		} else {
			m.partnerBoardVP, cmd = m.partnerBoardVP.Update(msg)
		}
	} else if msg.X >= m.width/2 {
		m.modsVP, cmd = m.modsVP.Update(msg)
	} else {
		m.boardVP, cmd = m.boardVP.Update(msg)
	}
	return m, cmd
}
```

Update `identifyScrollbar()` to add partner mods:

```go
case m.game != nil && m.game.IsDuos &&
	x == m.partnerModsScrollX && y >= m.partnerModsVPY && y < m.partnerModsVPY+m.partnerModsVPH:
	return 3, m.partnerModsVPY, m.partnerModsVPH
```

Update `scrubAt()`:

```go
case 3:
	tuiScrollbarJump(&m.partnerModsVP, y, m.scrubTrackY, m.scrubTrackH)
```

Update scrubPanel comment: `// 0=board, 1=mods, 2=partnerBoard, 3=partnerMods`

**Step 5: Build and test**

Run: `go build ./cmd/battlestream && go test -count=1 ./internal/tui/`
Expected: Clean build and all tests pass.

**Step 6: Commit**

```bash
git add internal/tui/tui.go
git commit -m "feat: partner buff sources pane in TUI (duos only)

Renders partner buff data in a right-column panel beside the partner
board, mirroring the local board/buff layout. Mouse wheel and
scrollbar routing updated for all 4 viewport panels."
```

---

### Task 9: Proportional Layout Engine

**Files:**
- Modify: `internal/tui/tui.go` (Model struct, View layout calculations)

**Step 1: Add split ratio fields to Model**

```go
// Layout split ratios (0.0 to 1.0).
vSplit float64 // vertical: fraction of width for left column (default 0.5)
hSplit float64 // horizontal: fraction of available height for main row vs partner (default 0.7)
```

**Step 2: Initialize defaults in New()**

```go
vSplit:         0.5,
hSplit:         0.7,
```

**Step 3: Replace fixed layout calculations in View()**

Replace `colW := m.width/2 - 4` with ratio-based calculation:

```go
// Column widths from vertical split ratio.
totalInner := m.width - 8 // total inner width minus borders/padding for both panels
leftInner := int(m.vSplit * float64(totalInner))
rightInner := totalInner - leftInner

// Enforce minimums (10 chars content + 5 for scrollbar/padding).
const minPanelW = 15
if leftInner < minPanelW {
	leftInner = minPanelW
	rightInner = totalInner - leftInner
}
if rightInner < minPanelW {
	rightInner = minPanelW
	leftInner = totalInner - rightInner
}

leftColW := leftInner
rightColW := rightInner
leftVPW := leftInner - 5
rightVPW := rightInner - 5
if leftVPW < 10 {
	leftVPW = 10
}
if rightVPW < 10 {
	rightVPW = 10
}
```

Update all panel rendering to use `leftColW`/`rightColW` and `leftVPW`/`rightVPW` instead of `colW`/`vpContentW`.

For the vertical split (Duos only), replace the fixed `partnerH := 5` with:

```go
// Height split: main panels vs partner panels.
totalAvailable := m.height - m.row2StartY - sessionH - 1 - 3
if totalAvailable < 8 {
	totalAvailable = 8
}
var mainH, partnerH int
if m.game != nil && m.game.IsDuos {
	mainH = int(m.hSplit * float64(totalAvailable))
	partnerH = totalAvailable - mainH - 3 // 3 for partner border/title
	const minH = 4
	if mainH < minH {
		mainH = minH
		partnerH = totalAvailable - mainH - 3
	}
	if partnerH < minH {
		partnerH = minH
		mainH = totalAvailable - partnerH - 3
	}
} else {
	mainH = totalAvailable
}
```

**Step 4: Update scrollbar X positions**

```go
m.boardScrollX = 2 + leftVPW
m.modsScrollX = (leftColW + 4) + 2 + rightVPW
m.partnerScrollX = 2 + leftVPW
m.partnerModsScrollX = (leftColW + 4) + 2 + rightVPW
```

**Step 5: Build and run tests**

Run: `go build ./cmd/battlestream && go test -count=1 ./internal/tui/`
Expected: Clean build and all tests pass. Existing height/width tests should still pass since the default ratio is 0.5 (same as current 50/50 split).

**Step 6: Commit**

```bash
git add internal/tui/tui.go
git commit -m "feat: ratio-based proportional layout engine

Replace fixed 50/50 column split with configurable vSplit/hSplit
ratios. Panels fill available space proportionally after meeting
minimum size constraints (10 chars wide, 4 lines tall)."
```

---

### Task 10: Mouse-Drag Dividers

**Files:**
- Modify: `internal/tui/tui.go` (drag state, mouse handling, divider detection)

**Step 1: Add drag state to Model**

```go
// Divider drag state.
draggingV    bool    // dragging vertical divider
draggingH    bool    // dragging horizontal divider
dividerX     int     // X position of vertical divider (computed in View)
dividerY     int     // Y position of horizontal divider (computed in View)
```

**Step 2: Compute divider positions in View()**

After computing row2 and rowPartner, store the divider positions:

```go
m.dividerX = leftColW + 4 // X column where the vertical divider sits
if m.game != nil && m.game.IsDuos {
	m.dividerY = m.row2StartY + lipgloss.Height(row2) // Y row of horizontal divider
}
```

**Step 3: Add divider detection to handleMouse()**

In the `MouseActionPress` handler, before scrollbar detection:

```go
case tea.MouseActionPress:
	if msg.Button == tea.MouseButtonLeft {
		// Check vertical divider.
		if msg.X >= m.dividerX-1 && msg.X <= m.dividerX+1 &&
			msg.Y >= m.row2StartY && msg.Y < m.row2StartY+m.height {
			m.draggingV = true
			return m, nil
		}
		// Check horizontal divider (Duos only).
		if m.game != nil && m.game.IsDuos &&
			msg.Y >= m.dividerY-1 && msg.Y <= m.dividerY+1 {
			m.draggingH = true
			return m, nil
		}
		// Existing scrollbar detection...
		panel, trackY, trackH := m.identifyScrollbar(msg.X, msg.Y)
		// ...
	}
```

**Step 4: Handle drag motion**

In the `MouseActionMotion` handler:

```go
case tea.MouseActionMotion:
	if m.draggingV && msg.Button == tea.MouseButtonLeft {
		totalInner := m.width - 8
		newLeft := msg.X - 4 // approximate: click X minus left border/padding
		ratio := float64(newLeft) / float64(totalInner)
		if ratio < 0.2 { ratio = 0.2 }
		if ratio > 0.8 { ratio = 0.8 }
		m.vSplit = ratio
		return m, nil
	}
	if m.draggingH && msg.Button == tea.MouseButtonLeft {
		totalAvailable := m.height - m.row2StartY - 3 - 1 - 3
		newMain := msg.Y - m.row2StartY
		ratio := float64(newMain) / float64(totalAvailable)
		if ratio < 0.2 { ratio = 0.2 }
		if ratio > 0.8 { ratio = 0.8 }
		m.hSplit = ratio
		return m, nil
	}
	if m.scrubbing && msg.Button == tea.MouseButtonLeft {
		m.scrubAt(msg.Y)
	}
```

**Step 5: Handle drag release**

```go
case tea.MouseActionRelease:
	if m.draggingV || m.draggingH {
		m.draggingV = false
		m.draggingH = false
		return m, nil
	}
	m.scrubbing = false
```

**Step 6: Build and test**

Run: `go build ./cmd/battlestream && go test -count=1 ./internal/tui/`
Expected: Clean build and all tests pass.

**Step 7: Commit**

```bash
git add internal/tui/tui.go
git commit -m "feat: mouse-drag dividers for pane resizing

Vertical divider between board/buff columns and horizontal divider
between main/partner rows are now draggable. Click-and-drag updates
the split ratio in real time. Ratios clamped to 0.2-0.8 range."
```

---

### Task 11: Persist Split Ratios to Config

**Files:**
- Modify: `internal/config/config.go` (add TUI config struct)
- Modify: `internal/tui/tui.go` (accept config, save on drag release)
- Modify: `cmd/battlestream/main.go` (pass config to TUI)

**Step 1: Add TUI config to Config struct**

In `internal/config/config.go`, add:

```go
type Config struct {
	ActiveProfile string                   `yaml:"active_profile,omitempty" mapstructure:"active_profile"`
	Profiles      map[string]*ProfileConfig `yaml:"profiles,omitempty" mapstructure:"profiles"`
	API           APIConfig                 `yaml:"api" mapstructure:"api"`
	Logging       LoggingConfig             `yaml:"logging" mapstructure:"logging"`
	TUI           TUIConfig                 `yaml:"tui,omitempty" mapstructure:"tui"`
}

type TUIConfig struct {
	VerticalSplit   float64 `yaml:"vertical_split,omitempty" mapstructure:"vertical_split"`
	HorizontalSplit float64 `yaml:"horizontal_split,omitempty" mapstructure:"horizontal_split"`
}
```

**Step 2: Add config save helper**

In `internal/config/config.go`, add a method to persist just the TUI section:

```go
func (c *Config) SaveTUI() error {
	path := configFilePath()
	if path == "" {
		return fmt.Errorf("no config file path")
	}
	// Read existing config, update TUI section, write back.
	v := viper.New()
	v.SetConfigFile(path)
	_ = v.ReadInConfig()
	v.Set("tui.vertical_split", c.TUI.VerticalSplit)
	v.Set("tui.horizontal_split", c.TUI.HorizontalSplit)
	return v.WriteConfig()
}
```

Find or add a `configFilePath()` helper that returns the path to `~/.battlestream/config.yaml`.

**Step 3: Update Model to accept and store config**

In `internal/tui/tui.go`, add a config reference to Model:

```go
cfg *config.Config
```

Update `New()` to accept config:

```go
func New(grpcAddr string, cfg *config.Config) *Model {
	vSplit := 0.5
	hSplit := 0.7
	if cfg != nil && cfg.TUI.VerticalSplit > 0 {
		vSplit = cfg.TUI.VerticalSplit
	}
	if cfg != nil && cfg.TUI.HorizontalSplit > 0 {
		hSplit = cfg.TUI.HorizontalSplit
	}
	// ... rest of constructor
```

**Step 4: Save on drag release**

In `handleMouse()`, in the `MouseActionRelease` handler, when a drag ends:

```go
case tea.MouseActionRelease:
	if m.draggingV || m.draggingH {
		wasDragging := m.draggingV || m.draggingH
		m.draggingV = false
		m.draggingH = false
		if wasDragging && m.cfg != nil {
			m.cfg.TUI.VerticalSplit = m.vSplit
			m.cfg.TUI.HorizontalSplit = m.hSplit
			go m.cfg.SaveTUI() // fire-and-forget, non-blocking
		}
		return m, nil
	}
	m.scrubbing = false
```

**Step 5: Update call sites**

Update `NewCombined`, `cmdRun()`, `cmdTUI()` in `cmd/battlestream/main.go` to pass config through to `tui.New()`.

**Step 6: Build and test**

Run: `go build ./cmd/battlestream && go test -count=1 ./internal/tui/ ./internal/config/`
Expected: Clean build and all tests pass.

**Step 7: Commit**

```bash
git add internal/config/config.go internal/tui/tui.go cmd/battlestream/main.go
git commit -m "feat: persist TUI split ratios to config.yaml

Drag-release saves vertical_split and horizontal_split to
~/.battlestream/config.yaml. Values loaded on TUI startup.
Defaults to 0.5/0.7 if not configured."
```

---

## Final Validation

### Task 12: Full Test Suite with Race Detector

**Step 1: Run full test suite with race detector**

Run: `go test -race -count=1 ./...`
Expected: All tests pass.

**Step 2: Run vet and lint**

Run: `go vet ./...`
Expected: No issues.

Run: `golangci-lint run` (if available)
Expected: No issues.

**Step 3: Build final binary**

Run: `go build ./cmd/battlestream`
Expected: Clean build.

**Step 4: Commit any remaining fixes**

If race detector or lint finds issues, fix and commit each individually.
