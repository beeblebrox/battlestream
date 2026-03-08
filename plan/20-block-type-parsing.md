# 20 — [IMPROVEMENT] `BLOCK_START` `BlockType` ignored

**Priority:** LOW
**Area:** `internal/parser/parser.go`, `internal/gamestate/processor.go`

## Problem

`BlockType=ATTACK`, `BlockType=POWER`, `BlockType=PLAY`, etc. are all currently ignored
when a `BLOCK_START` line is parsed. Only `BlockSource` and `BlockCardID` are captured.

Without block type, the processor cannot distinguish:
- A combat attack block (ATK/HEALTH changes = damage, not buffs)
- A spell play block (stat changes = buffs from the spell)
- A triggered ability block (stat changes during combat = legitimate buffs)

The current heuristic using `PhaseCombat` to suppress stat changes is fragile — any
buff that fires during combat (triggered abilities, deathrattles that buff) will be
silently dropped.

## Fix

### Step 1: Capture `BlockType` in the parser

Extend `reBlockStart` to capture the `BlockType` field:
```
BLOCK_START BlockType=ATTACK Entity=... EffectCardId=... ...
```

Add `BlockType string` to the `BlockStartEvent` (or whatever struct represents it).

### Step 2: Thread `BlockType` into the processor's block context

Store `p.blockType` alongside `p.blockSource` and `p.blockCardID`.

### Step 3: Use `BlockType` in stat change attribution

```go
// In handleTagChange for ATK/HEALTH changes:
if p.blockType == "ATTACK" {
    // This is a damage event, not a buff — skip for buff attribution
    return
}
if p.blockType == "POWER" || p.blockType == "PLAY" {
    // This is a spell/ability — eligible for buff attribution even during combat
}
```

This replaces the blunt `PhaseCombat` suppression with precise per-block filtering.

## Files to change

- `internal/parser/parser.go` — extend `reBlockStart`, capture `BlockType`
- `internal/gamestate/processor.go` — store `p.blockType`, use in stat change filtering

## Complexity

Medium — requires careful regex extension and processor logic update. Risk of regressions
in the integration test if the current heuristic is changed aggressively.

## Verification

- Unit test: `BLOCK_START BlockType=ATTACK` sets `blockType` to "ATTACK".
- Integration test: board stats still correct after the heuristic change.
- Verify a known triggered-ability buff (if present in sample log) is now correctly
  attributed rather than suppressed.
