# 13 — [IMPROVEMENT] `CurrentGold` not tracked

**Priority:** MEDIUM
**Status:** DONE
**Area:** `internal/gamestate/processor.go`, `internal/gamestate/state.go`

**Resolution:** Added `RESOURCES`/`RESOURCES_USED` TAG_CHANGE handling in `handleTagChange`.
`Machine.UpdateGold()` tracks both values and computes `CurrentGold = total - used`.
Gold fields stored on Machine as `goldTotal`/`goldUsed`.

## Problem

`PlayerState.CurrentGold` is declared but never set. Hearthstone logs `RESOURCES` tag
changes that encode available gold each turn.

## What HS logs provide

```
TAG_CHANGE Entity=<local player entity> tag=RESOURCES value=<gold>
TAG_CHANGE Entity=<local player entity> tag=RESOURCES_USED value=<spent>
```

`RESOURCES` = total gold available at start of turn.
`RESOURCES_USED` = gold spent so far this turn.
Remaining = `RESOURCES - RESOURCES_USED`.

## Fix

### Step 1: Handle `RESOURCES` and `RESOURCES_USED` in `handleTagChange`

```go
case "RESOURCES":
    if p.isLocalPlayer(controllerID, e) {
        p.machine.SetResources(val)
    }
case "RESOURCES_USED":
    if p.isLocalPlayer(controllerID, e) {
        p.machine.SetResourcesUsed(val)
    }
```

### Step 2: Compute `CurrentGold` in machine

```go
func (m *Machine) SetResources(total int) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.state.Player.CurrentGold = total - m.resourcesUsed
    m.resources = total
}

func (m *Machine) SetResourcesUsed(used int) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.resourcesUsed = used
    m.state.Player.CurrentGold = m.resources - used
}
```

Store `m.resources` and `m.resourcesUsed` as unexported machine fields.

### Step 3: Expose in TUI and API

Add gold display to TUI player panel (e.g., "Gold: 7/10").

## Files to change

- `internal/gamestate/processor.go` — handle `RESOURCES`/`RESOURCES_USED`
- `internal/gamestate/machine.go` — `SetResources`, `SetResourcesUsed`, internal fields
- `internal/tui/` — display gold

## Complexity

Low. Tag names are well-known from HDT reference.

## Verification

- Feed a sample log and assert `CurrentGold` is set to a reasonable non-zero value
  during the recruit phase of at least one turn.
