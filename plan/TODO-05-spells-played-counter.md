# TODO-05 — Spells Played Counter: Wrong Label + Always Shown

**Status:** DONE
**Priority:** HIGH (shows wrong data — 72 spells shown for a game with 0 Naga synergy minions)

---

## Problem

`CatNagaSpells` (tag 3809) is displayed as "Naga Spells" and shown every game.

Two bugs:

### A — Wrong label

"Naga Spells" implies Naga Spellcraft (the keyword mechanic: playing spells gives adjacent
minions +1/+1). Tag 3809 is actually `SpellsPlayedForNagasCounter` in HDT — it counts total
spells played this game for **Thaumaturgist-type minion synergy** (minions that gain +ATK per
spell played). These are completely different mechanics.

HDT source: `SpellsPlayedForNagasCounter.cs`
- `CardIdToShowInUI`: Thaumaturgist
- `RelatedCards`: Thaumaturgist, ArcaneCannoneer, ShowyCyclist, Groundbreaker
- `LocalizedName`: "Counter_PlayedSpells" (i.e. "Spells Played")

### B — Always shown regardless of board

HDT `ShouldShow()`: only display when `Counter > 1 && Board contains a related card`.
We always show it. In the 2026-03-07 game (Spectre Teron, no Thaumaturgist-type minions),
we showed "Naga Spells  Tier 19 · 0/4" for 72 spells — data that is irrelevant to that game.

---

## Ground truth (2026-03-07 log)

The player had **0** Thaumaturgist/ArcaneCannoneer/ShowyCyclist/Groundbreaker on board.
The test `TestGameLog2026_03_07_NagaSpellsFinal` asserts value=72 — correct tag value,
but the counter should NOT be shown/tested for this game at all.

---

## Fix

### 1 — Rename display

`categories.go`: Change `CatNagaSpells` display name from `"Naga Spells"` to `"Spells Played"`.

The category constant `CatNagaSpells = "NAGA_SPELLS"` can stay (it names the game mechanic
being tracked), but the human-readable display should reflect the counter's actual meaning.

### 2 — Board presence gate

In `processor.go`, when handling tag 3809, only emit the counter when a relevant minion is
on the local player's board:

```go
case "3809":
    if p.isLocalPlayerEntity(e) || p.isLocalHero(e, controllerID) {
        raw, _ := strconv.Atoi(value)
        if p.hasNagaSynergyMinion() {
            stacks := 1 + (raw / 4)
            progress := raw % 4
            display := fmt.Sprintf("Tier %d · %d/4", stacks, progress)
            p.machine.SetAbilityCounter(CatNagaSpells, raw, display)
        } else {
            p.machine.RemoveAbilityCounter(CatNagaSpells)
        }
    }
```

`hasNagaSynergyMinion()` checks `p.machine.State().Board` for any minion whose CardID
is one of the related cards (golden and non-golden).

Related card IDs (confirmed via `/hs-card`):
- Thaumaturgist → `BG31_924` / `BG31_924_G`
- ArcaneCannoneer → `BG31_928` / `BG31_928_G`
- ShowyCyclist → `BG31_925` / `BG31_925_G`
- Groundbreaker → `BG31_035` / `BG31_035_G`

### 3 — `RemoveAbilityCounter` on machine

`Machine`/`BGGameState` needs a way to remove an ability counter (currently can only set).
Add `RemoveAbilityCounter(category string)` that deletes the entry from `AbilityCounters`.

### 4 — Update test

`TestGameLog2026_03_07_NagaSpellsFinal`: currently asserts `value=72, display="Tier 19 · 0/4"`.
After the fix, this counter should NOT be present in the final state for this game.
Test should assert `AbilityCounter` for `CatNagaSpells` is absent.

---

## Related

- [17-enchantment-table-staleness.md](17-enchantment-table-staleness.md) — broader CardID staleness
- [TODO-01](TODO-01-test-suite.md) — test that needs updating
