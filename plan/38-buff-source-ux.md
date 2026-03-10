# 38 — Buff Source UX

**Priority:** MEDIUM
**Status:** DONE

## Problem

Buff counts (e.g. "Eternal Night: 1") are meaningless without context. Users
don't know what the number represents — is it 1 ATK? 1 HP? 1 application?
The raw count obscures the actual stat impact of each buff source.

## Resolution

Already implemented. TUI renders `+ATK/+HP` format (tui.go:524):
```go
line := fmt.Sprintf("%-14s +%d/+%d", name, bs.Attack, bs.Health)
```

`BuffSource` struct (state.go:86-91) has both `Attack` and `Health` fields.

## Test Data

- `internal/gamestate/testdata/power_log_2026_03_08b.txt` (turns 3-4 for Eternal Night buffs)

## Affected Files

- `internal/tui/tui.go` — BUFF SOURCES panel rendering
- Possibly `internal/gamestate/state.go` if BuffSource display helpers are needed
