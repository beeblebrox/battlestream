# TODO-02 — Hero Entity Identification

**Status:** DONE
**Priority:** CRITICAL (armor, streak, heroCardID all depend on correct hero entity)

---

## Problem

In Battlegrounds logs, multiple entities share the local player's controller ID:

1. **Placeholder hero** (`TB_BaconShop_HERO_PH`, e.g. entity 33) — created first in
   `CREATE_GAME` block. Has `CONTROLLER=localPlayerID`, `ZONE=PLAY`, `CARDTYPE=HERO`.
   This is a temporary slot, not the real hero.

2. **Hero selection pool** — several hero card entities appear with `CONTROLLER=localPlayerID`
   before the player picks. Each has a real CardID but none is chosen yet.

3. **Real chosen hero** — the entity whose EntityID is assigned via
   `TAG_CHANGE Entity=<player> tag=HERO_ENTITY value=<entityID>` after hero selection.

4. **Ghost battle opponent copies** — during ghost battle combats, the opponent's hero
   entity is copied as a combat-copy. These temporarily appear with `player=<localPlayerID>`
   in the entity bracket notation, which was causing false hero matches.

## Root cause

The previous code set `localHeroID` from "first local HERO entity seen" or from the
initial `HERO_ENTITY` tag in the Player block. Both fire on the placeholder entity,
not the real hero, because:

- Fresh game log: Player block `HERO_ENTITY=33` (placeholder), real hero 88 not assigned
  until `TAG_CHANGE` at line 1445.
- Mid-game log: Player block `HERO_ENTITY=99` (real hero already set when log starts).

## Fix (implemented)

### Parser change: PlayerDef uses block mode

`rePlayerDef` now enters block mode (`p.inBlock = true`) so that subsequent
`tag=HERO_ENTITY value=X` lines within the Player block are captured in
`p.pending.Tags` before `EventPlayerDef` is emitted.

File: `internal/parser/parser.go`, `Feed()` — `rePlayerDef` case.

### Processor change: two-stage hero identification

`handlePlayerDef` reads `HERO_ENTITY` from block tags as a *tentative* initial value.
This works for mid-game logs (real hero already in Player block).

`HERO_ENTITY` TAG_CHANGE handler (`handleTagChange`) allows upgrade from placeholder
to real hero, but locks against further changes once the real hero is set:

```
shouldUpdate = (localHeroID == 0) OR (entityProps[localHeroID].CardID starts with "TB_BaconShop_HERO_PH")
```

This prevents ghost battle HERO_ENTITY tag changes from overwriting the real hero.

File: `internal/gamestate/processor.go`, `handleTagChange` — `case "HERO_ENTITY"`.

### Retroactive stat application

When `localHeroID` changes from placeholder (33) to real hero (88), the real hero's
cached `Health`, `Armor`, and `CardID` are immediately applied to `BGGameState.Player`
from `entityProps[heroID]`.

`entityInfo` struct gained an `Armor int` field so ARMOR tag changes on all hero
entities are cached regardless of whether they're the current `localHeroID`.

File: `internal/gamestate/processor.go`, `ARMOR` TAG_CHANGE case and `entityInfo`.

---

## Tests

All pass in `internal/gamestate/game_log_2026_03_07_test.go`:

- `TestGameLog2026_03_07_HeroCardID` — entity 88 = BG25_HERO_103_SKIN_D ✓
- `TestGameLog2026_03_07_FinalArmor` — Armor=3, Health=30 at game end ✓
- `TestGameLog2026_03_07_WinLossStreak` — WinStreak=2, LossStreak=0 ✓

The existing `power_log_game.txt` integration test and debugtui golden tests also pass.
Golden files were regenerated (`-update-golden`) to reflect the now-correct entity 99
identification for that log.

---

## Remaining concerns

- **Hero pool entities:** During hero selection, pool hero entities appear with
  `CONTROLLER=localPlayerID`. The placeholder upgrade path handles this correctly
  (they aren't assigned via HERO_ENTITY TAG_CHANGE), but they could pollute buff source
  counters if any BACON_ tags fire during selection. Low risk — no counters increment
  during selection phase.

- **Ghost battle completeness:** The lock `!strings.HasPrefix(cardID, "TB_BaconShop_HERO_PH")`
  prevents ghost HERO_ENTITY changes from overwriting. Verified against this log. Needs
  monitoring as ghost battle mechanics evolve.

- **`power_log_game.txt` golden values:** Entity 99 (`TB_BaconShop_HERO_90_SKIN_E`) is now
  identified as the real hero. This was verified by: entity 99 is the one assigned via
  HERO_ENTITY after hero selection in that log (not the placeholder entity 39). The golden
  files now show Armor=8, Hero=TB_BaconShop_HERO_90_SKIN_E. Awaiting video confirmation
  that this was the hero chosen in that game session.
