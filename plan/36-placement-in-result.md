# 36 — TUI game result should show placement number

**Priority:** LOW
**Status:** DONE

## Problem

The TUI shows "WIN" for placements 1-4 and "LOSS" for 5-8, but does not show
the actual placement number alongside the result. This is misleading — a user
seeing "WIN" assumes 1st place when they actually placed 4th.

Current display (tui.go:407):
```
Result  WIN
```

Desired display:
```
Result  WIN #4
```

## Fix

Update `tui.go` game info panel (around line 404-411):

```go
if placement >= 1 && placement <= 4 {
    b.WriteString(styleLabel.Render("Result ") + styleWin.Render(fmt.Sprintf("WIN #%d", placement)) + "\n")
} else if placement > 0 {
    b.WriteString(styleLabel.Render("Result ") + styleLoss.Render(fmt.Sprintf("LOSS #%d", placement)) + "\n")
}
```

Note: The top-4 = WIN threshold is correct for standard 8-player BG. For BG
Duos (2 teams), top 2 = WIN, but we don't currently detect game mode. This can
be a follow-up if needed.

## Affected Files

- `internal/tui/tui.go` — game info panel result display (~line 404-411)
