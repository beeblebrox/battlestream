# 42 — Card Name Resolution

**Priority:** MEDIUM
**Status:** TODO

## Problem

Unknown cards display as raw CardIDs (e.g. `BG34_403`). Entity names are only
available from PowerTaskList debug lines (which the parser filters out), so
minions created via GameState-only FULL_ENTITY blocks have no name.

## Research Findings

**Infrastructure already exists:**
- `internal/gamestate/cardnames.go` — `CardName()` function with ~3000+ CardID → name pairs
- BG34_403 → "Eternal Tycoon" already in map (line 1559)
- Generated via `.claude/skills/gen-card-names/gen_card_names.py` from HearthstoneJSON API
- Proto `MinionState.name` field exists, gRPC serialization wired

**Root cause:**
- Power.log GameState lines: `FULL_ENTITY - Creating ID=14570 CardID=BG34_403` (no name)
- PowerTaskList lines: `FULL_ENTITY - Updating [entityName=Eternal Tycoon ...]` (has name)
- Parser filters to GameState only, so EntityName is often empty
- TUI falls back to CardID when `mn.Name` is empty

## Fix (minimal — ~2 lines)

In `processor.go` where minions are upserted (~line 765-770), add fallback:
```go
if info.Name == "" {
    info.Name = CardName(info.CardID)
}
```

No new packages, no runtime HTTP, no build scripts needed.

## Test Data

- `internal/gamestate/testdata/power_log_2026_03_08b.txt` (BG34_403 appears with 2/9 stats)

## Affected Files

- `internal/gamestate/processor.go` — add `CardName()` fallback when entity name is empty
