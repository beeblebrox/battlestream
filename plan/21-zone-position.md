# 21 — [IMPROVEMENT] `ZONE_POSITION` tag ignored — board order wrong

**Priority:** LOW
**Area:** `internal/gamestate/processor.go`, `internal/gamestate/machine.go`

## Problem

Hearthstone entities have a `ZONE_POSITION` tag that encodes board position (left to right,
1-based). The parser reads it as a generic tag, and the processor ignores it. Board position
is essential for:
- Displaying minions in correct board order (currently arbitrary insertion order)
- Position-dependent buffs (e.g., `CatRightmost` — rightmost minion)
- Any future mechanic that references position (e.g., "adjacent minions")

## Fix

### Step 1: Handle `ZONE_POSITION` in `handleTagChange`

```go
case "ZONE_POSITION":
    p.machine.SetMinionPosition(e.EntityID, val)
```

### Step 2: Implement `SetMinionPosition` in machine

Store position in the `Minion` struct:
```go
type Minion struct {
    // ... existing fields ...
    Position int `json:"position"`
}
```

Update `SetMinionPosition` to find the minion by entity ID and set its position.

### Step 3: Sort board by position in `State()` (or on set)

When returning board state, sort minions by `Position` ascending. This ensures left-to-right
order in the API response and TUI display.

### Step 4: Use position in `CatRightmost` logic (if applicable)

Once position is tracked, `CatRightmost` buff source attribution can check `Position ==
len(board)` to identify the rightmost minion.

## Files to change

- `internal/gamestate/processor.go` — handle `ZONE_POSITION` tag
- `internal/gamestate/machine.go` — `Minion.Position`, `SetMinionPosition`, sort in `State()`
- Proto file — add `position` field to `Minion` message if not already present

## Complexity

Medium — threading position through the data model and ensuring sort is stable.

## Verification

- Integration test: board minions are returned in position order (1, 2, 3, ..., N).
- Manual verification with TUI: minions appear in correct left-to-right board order.
