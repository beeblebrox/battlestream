# 22 — [IMPROVEMENT] No `DAMAGED`/`DEFENDING`/`ATTACKING` tag handling

**Priority:** LOW
**Area:** `internal/gamestate/processor.go`

## Problem

During combat, ATK/HEALTH changes that represent damage are currently indistinguishable
from genuine stat changes (buffs). The processor heuristically suppresses them via the
`PhaseCombat` check, but this is fragile:

- A buff that fires during combat (triggered ability, deathrattle, golden effect) would
  be silently dropped under the current heuristic.
- Conversely, damage events that arrive during recruit phase (unusual but possible with
  certain mechanics) would be misidentified as buffs.

HS logs do emit `DAMAGED`, `DEFENDING`, and `ATTACKING` tag changes during combat that
can be used to precisely identify damage vs. buff events.

## What HS logs provide

```
TAG_CHANGE Entity=<minion> tag=DAMAGED value=3
TAG_CHANGE Entity=<minion> tag=DEFENDING value=1
TAG_CHANGE Entity=<minion> tag=ATTACKING value=1
```

These precede or accompany HEALTH changes that represent damage, not buffs.

## Fix

### Step 1: Track `DAMAGED` tag per entity

In `handleTagChange`, when `tag == "DAMAGED"`, store `p.damagedEntities[entityID] = val`.
Clear this set at combat-start / recruit-phase transitions.

### Step 2: Use `DAMAGED` to filter HEALTH changes

When a `HEALTH` change arrives for an entity in `damagedEntities`, treat it as damage
(skip buff attribution) rather than a stat buff. This is more precise than the phase check.

### Step 3: Complement plan 20 (BLOCK_START BlockType)

Together with `BlockType=ATTACK` detection (plan 20), this provides two independent
signals to identify damage vs. buff. Either alone is an improvement over the current
phase-only heuristic.

## Files to change

- `internal/gamestate/processor.go` — handle `DAMAGED`/`DEFENDING`/`ATTACKING`;
  add `damagedEntities` map; use it in ATK/HEALTH change filtering

## Complexity

Medium — requires careful state management (when to clear the map) and testing to
ensure legitimate combat buffs are still captured.

## Verification

- Integration test: board stats correct (no false buff suppression, no damage misattributed
  as buff) after the change.
- Ideally, a test log that includes a triggered ability firing during combat.
