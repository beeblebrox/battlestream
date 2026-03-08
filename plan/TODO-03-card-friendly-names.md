# TODO-03 — Card Friendly Names in TUIs

**Status:** DONE
**Priority:** HIGH (raw card IDs are unreadable to users)

---

## Problem

Both TUIs displayed raw Hearthstone card IDs wherever a card name was needed:
- Debug TUI player panel: `Hero   TB_BaconShop_HERO_90_SKIN_E`
- Debug TUI game picker: prefix-stripped ID (e.g. `90_SKIN_E`)
- Debug TUI change log: CardID fallback when EntityName missing
- Regular TUI: no hero displayed at all

---

## Solution

### `/gen-card-names` skill

`.claude/skills/gen-card-names/gen_card_names.py` — fetches the full HearthstoneJSON
card database (34,179 cards as of 2026-03-07), filters for BG-relevant heroes and
minions, and generates `internal/gamestate/cardnames.go`.

**Generated file:** `internal/gamestate/cardnames.go`
- `var cardNames map[string]string` — 3,069 BG card ID → display name entries
- `func CardName(cardID string) string` — returns name if known, cardID if not

**Cache:** `/.claude/skills/gen-card-names/cards_cache.json` — refreshed every 7 days.
Run `--force` to bypass.

**To update after a patch:**
```bash
/gen-card-names --force
go build ./...
go test ./internal/debugtui/ -update-golden
```

### TUI changes

| Location | Before | After |
|----------|--------|-------|
| Debug TUI player panel (`model.go:888`) | raw `HeroCardID` | `gamestate.CardName(HeroCardID)` |
| Debug TUI game picker (`model.go:678-686`) | prefix-strip hack | `gamestate.CardName(HeroCardID)` |
| Debug TUI change log — add (`render.go`) | `mn.CardID` fallback | `gamestate.CardName(mn.CardID)` |
| Debug TUI change log — remove (`render.go`) | `mn.CardID` fallback | `gamestate.CardName(mn.CardID)` |
| Debug TUI change log — change (`render.go`) | `mn.CardID` fallback | `gamestate.CardName(mn.CardID)` |
| Debug TUI minion render (`render.go:69`) | `mn.CardID` fallback | `gamestate.CardName(mn.CardID)` |
| Regular TUI hero panel (`tui.go`) | not shown | `gamestate.CardName(p.HeroCardId)` line added |

The regular TUI gained a gamestate import for `CardName()`.

---

## Known resolved names

| CardID | Friendly Name |
|--------|---------------|
| `BG25_HERO_103_SKIN_D` | Spectre Teron |
| `TB_BaconShop_HERO_90_SKIN_E` | Festival Silas |
| `TB_BaconShop_HERO_KelThuzad` | Kel'Thuzad |
| `BGDUO_HERO_222` | Cho |

---

## Remaining gaps

- **Minion EntityName is usually correct from the log** — the `CardName()` fallback
  only fires when `mn.Name == ""`, which happens for entities that were never fully
  described in a FULL_ENTITY block (e.g. combat copies of late-game minions). Low priority.

- **Opponent hero** — not yet displayed anywhere. When opponent tracking (plan 10)
  is added, `CardName()` should be used there too.

- **Proto layer** — the gRPC/REST API still returns raw `hero_card_id`. Consumers
  can call `CardName()` or we can add a `hero_name` field to the proto. Deferred.
