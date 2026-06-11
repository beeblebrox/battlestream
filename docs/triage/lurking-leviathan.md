# Triage: Lurking Leviathan Buff Tracking

**Date:** 2026-05-08  
**Status:** **IMPLEMENTED** (2026-06-10) — commit `dc03a7b` on branch `feat/lurking-leviathan-counter`.

## Implementation Summary

Open questions answered from the real game log (`internal/gamestate/testdata/power_log_2026_05_08.txt`,
171 BG35_602e entities) and the HDT reference:

1. **Per-minion enchantment, no player-level Dnt.** No `BG35_602pe` appears in logs, and HDT
   has no counter class for BG35 — Option A below was correct.
2. **ATK only.** `TAG_SCRIPT_DATA_NUM_1` carries the +ATK granted to that Beast; no HP component
   is ever set.
3. **Own category:** `CatBeastBuff` (`"BEAST_BUFF"`), display name "Leviathan", group TYPE_BUFFS.

**Accumulation: max-seen SD1** (new `handleMaxDnt` in `processor.go`). Combat simulation copies
replay historical (lower) SD1 values, so neither "latest" nor "sum" is stable; the max is the
Leviathan's current permanently-improved buff level (176 at game end in the fixture log).

Changes (commit `dc03a7b`):
- `internal/gamestate/categories.go` — const, `BG35_602e` mapping, display name, group
- `internal/gamestate/processor.go` — `handleDntTagChange` case + `handleMaxDnt`
- `internal/tui/tui.go` — panel color
- `streamdeck-plugin/src/categories.ts` — `BEAST_BUFF` CATEGORY_META entry (gradient only;
  dedicated icon still TODO — renderer falls back gracefully without one)
- Tests: `TestCounterLeviathanWrath` (unit, combat-replay regression),
  `TestGameLog2026_05_08_LeviathanBuffSource` (real-log integration, asserts +176/+0)

---

*Original triage below (pre-implementation).*

---

## What is Lurking Leviathan?

A BG35 minion (`BG35_602` / `BG35_602_G`, golden) that buffs every Beast you summon with +2 ATK (+4 golden) permanently.

| Card ID        | Name               | Text                                                        |
|----------------|--------------------|-------------------------------------------------------------|
| `BG35_602`     | Lurking Leviathan  | Whenever you summon a Beast, give it +2 Attack and improve this permanently. |
| `BG35_602_G`   | Lurking Leviathan  | (golden) +4 Attack per Beast, otherwise same.               |
| `BG35_602e`    | Leviathan's Wrath  | Enchantment applied to each Beast when summoned. (+0/+0 base text — actual value from TAG_SCRIPT_DATA_NUM_1 or ATTAK delta) |

## Current State

- `BG35_602` and `BG35_602_G` are now in `internal/gamestate/cardnames.go` (added 2026-05-08 via fresh fetch from HearthstoneJSON).
- `BG35_602e` is **NOT** in `categoryByEnchantmentCardID` in `internal/gamestate/categories.go`.
- No tracking button for Lurking Leviathan exists in the Stream Deck plugin.

## What Needs to Happen

### 1. Verify enchantment appears in Power.log

The Power.log from the session where the user saw Lurking Leviathan is at:
```
/home/moch/.local/share/Steam/steamapps/compatdata/2375601304/pfx/drive_c/Program Files (x86)/Hearthstone/Logs/Hearthstone_2025_06_20_19_06_08/Power.log
```

**Problem:** This file is only 1140 bytes (basically empty — likely a session that didn't run BG). The other log directories in that folder also don't have a Power.log. There's no log file yet that shows the card in action.

**Action needed:** Play a BG game with Lurking Leviathan on board, then search the resulting Power.log for:
```bash
grep -i "BG35_602" /path/to/Power.log
```
Look for lines like:
```
TAG_CHANGE Entity=<beastEntityId> tag=ATK value=<N>
  [ENCHANTING_ENTITY id=<N> zone=PLAY zonePos=... cardId=BG35_602e ...]
```

### 2. Determine buff category

Lurking Leviathan gives type-specific buffs to **Beasts**. The right approach:

- **Option A: New category `CatLurkingLeviathan` / `"BEAST_BUFF"`** — Track via `BG35_602e` enchantment in `categoryByEnchantmentCardID`. This is the most precise approach.
- **Option B: Check if HDT has a Dnt counter** — If HDT has a `LurkingLeviathanCounter.cs` (player-level Dnt tag), it would be a player-level Dnt enchantment instead.

Before implementing, run the **buff-scout agent** to check HDT:
```
/buff-scout
```
Ask it to look for BG35_602 in HDT counter classes and HearthDb.

### 3. Add to categories.go

If verified as per-minion enchantment (`BG35_602e`), add:
```go
const CatBeastBuff = "BEAST_BUFF"  // or CatLurkingLeviathan = "LURKING_LEVIATHAN"
```

In `categoryByEnchantmentCardID`:
```go
"BG35_602e": CatBeastBuff,
```

Add to `DYNAMIC_CATEGORIES` set if it should appear on Stream Deck buff-slot buttons.

### 4. Add Stream Deck metadata

In `streamdeck-plugin/src/categories.ts`, add to `CATEGORY_META`:
```typescript
BEAST_BUFF: {
  displayName: 'Beast',
  gradient: ['#1a3a20', '#2d7a3a'],
  iconPath: ...
},
```

Add to `DYNAMIC_CATEGORIES` set.

## Why the User Reported It

The user saw "Lurking Leviathan" as an active buff source in their game, but the Stream Deck showed no button for it (because it's not in `DYNAMIC_CATEGORIES`) and the TUI would show it under an unrecognized category.

## Files to Touch

| File | Change |
|------|--------|
| `internal/gamestate/categories.go` | Add new const + enchantment mapping |
| `internal/gamestate/cardnames.go` | Already updated (2026-05-08) |
| `streamdeck-plugin/src/categories.ts` | Add CATEGORY_META entry |
| TUI categories display | Already uses `CATEGORY_META` — should auto-pick up |

## Open Questions

1. Does `BG35_602e` appear as a per-minion enchantment or is there a player-level Dnt counter?
2. Is this buff +ATK only (as the card text says) or does it also give HP in practice?
3. Does it need its own category or does it fit under a broader "Beast buffs" umbrella?
