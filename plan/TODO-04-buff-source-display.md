# TODO-04 — Buff Source Tracking: Shop Buff + Display Name Consolidation

**Status:** DONE
**Priority:** HIGH (buff sources missing from TUI; display names could silently desync)

---

## Problems

### A — BG_ShopBuff DNT not tracked

Staff of Enrichment (`BG28_886`) and its caster Timewarped Shadowdancer (`BG34_Giant_360`)
give future tavern minions +2/+2 each cast. The effect is tracked in a player-level DNT
enchantment (`cardId=BG_ShopBuff`, entity 4549 in 2026-03-07 log) via `TAG_SCRIPT_DATA_NUM_1`
(ATK) and `TAG_SCRIPT_DATA_NUM_2` (HP). This was completely untracked — neither TUI showed it.

In the 2026-03-07 game: Golden Shadowdancer (fires twice/turn from turn 7) accumulated
`+81/+79` across 16 turns. Zero was shown.

### B — Duplicate `buffCategoryDisplayName` maps in both TUIs

Both `internal/tui/tui.go` and `internal/debugtui/render.go` maintained independent
hardcoded maps of category→display name, duplicating `gamestate.CategoryDisplayName`.
When `CatShopBuff` was added to `categories.go`, neither TUI map was updated, so the
raw constant `SHOP_BUFF` was displayed instead of "Shop Buff".

---

## Solution

### A — BG_ShopBuff tracking

**`internal/gamestate/categories.go`:**
- Added `CatShopBuff = "SHOP_BUFF"` constant
- Added `"BG_ShopBuff": CatShopBuff` to `categoryByEnchantmentCardID`
- Added `CatShopBuff: "Shop Buff"` to `CategoryDisplayName`

**`internal/gamestate/processor.go`:**
- Added `case "BG_ShopBuff":` in `handleDntTagChange` switch, calling `handleGenericShopBuffDnt`
- `handleGenericShopBuffDnt` uses differential accumulation from `shopBuffPrev` (same
  pattern as other Dnt handlers) — absolute DNT value changes are converted to deltas
  to avoid double-counting from repeated same-value GameState/PowerTaskList pairs

Final value verified against log: `+81/+79` at game end matches DNT entity 4549's
last `TAG_SCRIPT_DATA_NUM_1=81` / `TAG_SCRIPT_DATA_NUM_2=79` at line 544904 (14:14:38).

### B — Single source of truth for display names

Both `buffCategoryDisplayName` functions replaced with delegation to `gamestate.CategoryDisplayName`:

```go
func buffCategoryDisplayName(cat string) string {
    if n, ok := gamestate.CategoryDisplayName[cat]; ok {
        return n
    }
    return cat
}
```

Any new category added to `categories.go` is now automatically surfaced in both TUIs.

---

## Verified

- Regular TUI `--dump`: `Shop Buff  +81/+79` ✓
- Replay `--dump --turn 16`: `Shop Buff  +81/+79` ✓ (after jumpToTurn fix — see TODO-01 Part 6)
- All tests pass

---

## Related

- [TODO-01](TODO-01-test-suite.md) — jumpToTurn fix that revealed the discrepancy
- [17-enchantment-table-staleness.md](17-enchantment-table-staleness.md) — broader issue of manually curated CardID map
