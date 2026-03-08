# 12 — [IMPROVEMENT] Win/Loss streak not tracked

**Priority:** MEDIUM
**Area:** `internal/gamestate/processor.go`, `internal/gamestate/machine.go`

## Problem

`PlayerState.WinStreak` and `LossStreak` fields exist but are never set. HS logs emit
`WINNING_PLAYER` and `LOSING_PLAYER` tag changes during combat that can be used to
derive per-round outcomes and cumulative streaks.

## What HS logs provide

During combat resolution:
```
TAG_CHANGE Entity=GameEntity tag=WINNING_PLAYER value=<playerID>
TAG_CHANGE Entity=GameEntity tag=LOSING_PLAYER value=<playerID>
```

These tags identify which player won/lost each combat round.

## Fix

### Step 1: Handle `WINNING_PLAYER` / `LOSING_PLAYER` in `handleTagChange`

```go
case "WINNING_PLAYER":
    if val == p.localPlayerID {
        p.machine.RecordRoundWin()
    } else {
        p.machine.RecordRoundLoss()
    }
case "LOSING_PLAYER":
    // symmetric — only one of WINNING/LOSING needed if they're always paired
```

### Step 2: Implement `RecordRoundWin` / `RecordRoundLoss` in machine

```go
func (m *Machine) RecordRoundWin() {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.state.Player.WinStreak++
    m.state.Player.LossStreak = 0
}

func (m *Machine) RecordRoundLoss() {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.state.Player.LossStreak++
    m.state.Player.WinStreak = 0
}
```

### Step 3: Expose in TUI and API

Wire `WinStreak`/`LossStreak` into the TUI player panel and ensure they appear in
the REST/gRPC response (they likely already flow through if the struct fields are
already in the proto).

## Files to change

- `internal/gamestate/processor.go` — handle `WINNING_PLAYER`/`LOSING_PLAYER` tags
- `internal/gamestate/machine.go` — add `RecordRoundWin`/`RecordRoundLoss`
- `internal/tui/` — display streak in player panel if not already shown

## Complexity

Low-medium. The tag names need verification against actual log output.

## Verification

- Integration test: confirm `WinStreak` / `LossStreak` are non-zero at game end
  and match the expected outcome from the sample log.
