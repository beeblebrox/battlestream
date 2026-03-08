# 32 — [IMPROVEMENT] No trinkets/artifacts support

**Priority:** LOW
**Area:** `internal/gamestate/categories.go`, `internal/gamestate/processor.go`

## Problem

Trinkets introduced in later BG patches (post-2025) may add new buff tag patterns or
enchantment CardIDs not yet covered by the existing `categoryByEnchantmentCardID` map.
No proactive support exists, and there is no mechanism to detect gaps after a patch.

This is partially addressed by plan 17 (runtime unknown CardID warning) and the
buff-scout workflow, but trinkets may also introduce new *tag* patterns (new GameTag
IDs, new counter structures) that go beyond enchantment CardID matching.

## Fix

### Step 1: Run buff-scout after each major BG patch

The `.claude/agents/buff-scout.md` workflow should be executed after each patch to:
1. Scan HDT reference for new `BgCounter` classes
2. Identify any new CardID constants in HearthDb reference
3. Report gaps vs. `categories.go`

### Step 2: Investigate trinket mechanic structure

Once trinkets are available in logs:
1. Capture a sample log with trinket activity
2. Identify which tags or enchantments they produce
3. Determine if they follow existing counter patterns or require new handling

### Step 3: Add trinket category if needed

If trinkets produce new buff patterns:
- Add `CatTrinket` (or specific names) to `categories.go`
- Add CardID mappings
- Add Dnt handler if they use Dnt counters
- Add TUI display in appropriate panel

### Step 4: Document in PARSER_SPEC.md

Once investigated, document the trinket mechanic handling (or deliberate non-handling)
in `docs/PARSER_SPEC.md`.

## Files to change (once investigated)

- `internal/gamestate/categories.go` — new category constants and CardID mappings
- `internal/gamestate/processor.go` — new Dnt handlers if needed
- `internal/tui/` — display new categories
- `docs/PARSER_SPEC.md` — documentation

## Complexity

Unknown until trinket mechanics are captured in logs. Likely low-medium if they follow
existing counter patterns; high if they introduce entirely new structures.

## Verification

- TUI correctly displays trinket-related buff categories after a game with trinket activity.
- buff-scout audit reports no gaps for known trinket enchantment CardIDs.
