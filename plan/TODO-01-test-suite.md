# TODO-01 — Integration Test Suite

**Status:** IN PROGRESS
**Priority:** HIGH (verification foundation for all other bug fixes)

## Log files

### 2026-03-07 Game Log
**Source:**
`/chungus/battlenet/drive_c/Program Files (x86)/Hearthstone/Logs/Hearthstone_2026_03_07_12_40_40/Power.log`
(593,714 lines — full Battlegrounds game, 2026-03-07, started ~13:42)

**Fixture path:** `internal/gamestate/testdata/power_log_2026_03_07.txt` ✓ COPIED

### 2026-03-08 Game Log (with video reference)
**Source:**
`/chungus/battlenet/drive_c/Program Files (x86)/Hearthstone/Logs/Hearthstone_2026_03_08_09_16_08/Power.log`
(393,774 lines — full Battlegrounds game, 2026-03-08, started ~09:17)

**Fixture path:** `internal/gamestate/testdata/power_log_2026_03_08.txt` ✓ COPIED

**Video available** for this session — can verify health/armor/damage visually.

**Key facts (ground truth from log + video):**
- Hero: Delivery Deryl (entity 101, cardId=TB_BaconShop_HERO_36_SKIN_E, player=7)
- Player: Moch#1358 (entity 20, PlayerID=7)
- Base health: 30, starting armor: 16
- Turn 9: health should go from 30 to 23 (ARMOR=0, DAMAGE=7)
- Turn 11: gained 5 armor during recruit, lost it + more health in combat (DAMAGE=17)
- Turn 12: died (DAMAGE=32, effective HP = -2), final placement = 4
- Placement 4 = WIN (top 4 in standard BG)
- Bugs exposed: DAMAGE tag not tracked (plan 34), max health hardcoded (plan 35)

---

## Part 1 — Replay TUI (prerequisite for per-turn verification) ✓ DONE

The debug TUI (`battlestream replay`) is implemented in `internal/debugtui/`.
Supports step-through of every parsed event, game picker, dump mode.

Command:
```
battlestream replay --log <path>
battlestream replay --log <path> --dump --turn 5
```

---

## Part 2 — Verified bugs from this game log

### Bug A — Armor not tracked correctly ✓ FIXED

Armor tag changes were being attributed to the wrong hero entity (placeholder vs real hero).
Fixed as part of TODO-02 (hero identification). Entity 88 is the real hero; armor correctly
decrements from 14→11→8→3 across the game.

Integration test: `TestGameLog2026_03_07_FinalArmor` — PASS ✓

---

### Bug B1 — CatSpellcraft mislabeled ✓ FIXED

`CatSpellcraft` renamed to `CatNagaSpells`, display label changed to `"Naga Spells"`.
Confirmed in `internal/gamestate/categories.go`.

Integration test: `TestGameLog2026_03_07_NagaSpellsFinal` (value=72, display="Tier 19 · 0/4") — PASS ✓

---

### Bug C — Win/loss streak never set ✓ FIXED

Streak tracked via PREDAMAGE detection on the local hero entity.
Rounds with `PREDAMAGE > 0` on hero entity = loss; absence of PREDAMAGE after turn advance = win.
Depended on correct hero entity identification (TODO-02).

Integration test: `TestGameLog2026_03_07_WinLossStreak` (WinStreak=2, LossStreak=0) — PASS ✓

---

### Bug D — Tavern tier attribution — OPEN (plan 04)

Need to verify `PLAYER_TECH_LEVEL` attribution doesn't misfire when `controllerID == 0`.
Integration test present: `TestGameLog2026_03_07_TavernTierFinal` (expects 6) — PASS ✓
Plan 04 details the full fix.

---

## Part 3 — Test suite status

### Written and passing

| Test | File | Status |
|------|------|--------|
| `TestGameLog2026_03_07_PlayerIdentification` | `game_log_2026_03_07_test.go` | PASS ✓ |
| `TestGameLog2026_03_07_HeroCardID` | `game_log_2026_03_07_test.go` | PASS ✓ |
| `TestGameLog2026_03_07_Placement` | `game_log_2026_03_07_test.go` | PASS ✓ |
| `TestGameLog2026_03_07_Phase` | `game_log_2026_03_07_test.go` | PASS ✓ |
| `TestGameLog2026_03_07_FinalArmor` | `game_log_2026_03_07_test.go` | PASS ✓ |
| `TestGameLog2026_03_07_NagaSpellsFinal` | `game_log_2026_03_07_test.go` | PASS ✓ |
| `TestGameLog2026_03_07_TavernTierFinal` | `game_log_2026_03_07_test.go` | PASS ✓ |
| `TestGameLog2026_03_07_TotalTurns` | `game_log_2026_03_07_test.go` | PASS ✓ |
| `TestGameLog2026_03_07_WinLossStreak` | `game_log_2026_03_07_test.go` | PASS ✓ |

### Still to write

| Test | Asserts | Q# dependency | Notes |
|------|---------|---------------|-------|
| `TestGameLog2026_03_07_InitialArmor` | `Player.Armor == 14` at turn 1 | Q1 ✓ | Needs per-step snapshot; requires replay parser |
| `TestGameLog2026_03_07_ArmorPerTurn` | Armor 14→11→8→3 at turns 1,2,13 | Q4 ✓ | Same — needs turn-level snapshots |
| ~~`TestGameLog2026_03_07_NagaSpellsPerTurn`~~ | ~~Counter increments match Q6 schedule~~ | Q6 ✓ | **DROPPED** — Spectre Teron game has no Naga synergy minions; counter correctly absent (TODO-05) |
| `TestGameLog2026_03_07_TavernTierProgression` | T2@1, T3@5, T4@6, T5@7, T6@8 | Q8 ✓ | Needs per-turn snapshots |
| `TestGameLog2026_03_07_BoardAtGameEnd` | Board minions, count, stats | — | Need to grep log for final board |

The per-turn tests require the replay parser (`LoadAllGames`) to produce per-turn
`Step` snapshots. This is already implemented in `internal/debugtui/replay.go`.
These tests should use `debugtui.LoadReplay()` and step to a specific turn.

Note: `jumpToTurn` now returns the LAST step of a turn (end-of-turn state), so
snapshot tests at `--turn N` reflect the state after all effects in that turn fire.

---

## Part 4 — Q&A (ground truth from raw log)

| # | Question | Answer |
|---|----------|--------|
| Q1 | Armor at start of turn 1? | **14** (tag=ARMOR value=14, entity 88, log line 641) |
| Q2 | Armor ever go UP? | **No** — only decreased: 14→11→8→3 |
| Q3 | Armor/health at game end? | **Armor=3, Health=30** (last ARMOR change 14:07:50; Moch WON) |
| Q4 | Turns where armor changed? | Turn 1: 14→11 (-3), Turn 2: 11→8 (-3), Turn 13: 8→3 (-5) |
| Q5 | Total spells cast? | **72** (tag=3809 final value, Moch#1358, log line 563604, 14:15:31) |
| Q6 | Which turns were spells cast? | Every turn; totals: T1=1, T2=2, T3=3, T4=4, T5=9, T6=18, T7=25, T8=33, T9=35, T10=44, T11=52, T12=58, T13=66, T14=72 |
| Q7 | Round-by-round win/loss? | **L L W W W W W W W W W W L W W W** — via PREDAMAGE on entity 88 |
| Q8 | Tavern tier progression? | T2@turn1 (13:43:35), T3@turn5, T4@turn6, T5@turn7, T6@turn8 |
| Q9 | Hero played? | **Spectre Teron** (BG25_HERO_103_SKIN_D, entity 88, player=5) |
| Q10 | Final placement? | **1st place** (PLAYSTATE=WON Moch#1358 at 14:16:10) |

Total BG turns: **16** (turn 16 starts 14:13:49; game ends 14:16:10)

---

## Part 6 — Fixes made in this session

### sync.Once shared log parse ✓ DONE

Each integration test previously re-parsed the 593K-line fixture independently.
Under `-race` this caused timeouts (~5-6s per parse × 8 tests = ~45s+ total).

Added `internal/gamestate/log2026_helper_test.go` with `sharedLog2026State(t)`
using `sync.Once` — the fixture is parsed exactly once across all tests in the package.

### jumpToTurn shows end-of-turn state ✓ DONE

**Bug:** `jumpToTurn(N)` in `internal/debugtui/model.go` jumped to the FIRST step where
`Turn >= N` (the TURN tag boundary), before end-of-turn effects fire. So `--dump --turn 16`
showed `Shop Buff +65/+63` instead of `+81/+79` — mismatching the regular TUI.

**Root cause:** Timewarped Shadowdancer fires at end of recruit phase (still within turn 16,
at 14:14:37-38), but the TURN=16 boundary event fires at 14:13:49. The 16 ATK / 16 HP
difference was exactly those two Shadowdancer casts.

**Fix:** `jumpToTurn` now finds the LAST step with `Turn == N` (i.e., the final event of
that turn — `GAME_END` for the last turn). Replay `--dump --turn 16` now agrees with
the regular TUI. Golden file `last-turn.txt` regenerated.

---

## Part 5 — New issues found during this work

These emerged from implementing the above tests. Each is tracked in the plan directory.

### Card friendly names ✓ DONE

Both TUIs previously showed raw card IDs (e.g. `TB_BaconShop_HERO_90_SKIN_E`).

**What was done:**
- Created skill `/gen-card-names` (`.claude/skills/gen-card-names/gen_card_names.py`) that fetches
  the full HearthstoneJSON card database and generates `internal/gamestate/cardnames.go`
- `CardName(cardID string) string` function now available package-wide
- Debug TUI `renderPlayerPanel`: hero shows friendly name (e.g. "Festival Silas")
- Debug TUI game picker: replaced hacky prefix-stripping with `CardName()`
- Debug TUI change log: minion CardID fallback now uses `CardName()`
- Regular TUI `renderHeroPanel`: added "Hero" line using `CardName()`
- Golden test files regenerated to match

**To update after a patch:** run `/gen-card-names --force` then rebuild.

**Key names resolved:**
| CardID | Name |
|--------|------|
| `BG25_HERO_103_SKIN_D` | Spectre Teron |
| `TB_BaconShop_HERO_90_SKIN_E` | Festival Silas |
| `TB_BaconShop_HERO_KelThuzad` | Kel'Thuzad |
| `BGDUO_HERO_222` | Cho |

### Issue: Hero entity identification complexity

BG logs contain multiple hero entities with the local player's controller ID:
- `TB_BaconShop_HERO_PH` — placeholder set during CREATE_GAME (entity 33 in 2026 log)
- Pool heroes during hero selection (several entities, all local controller)
- Real chosen hero (entity 88 in 2026 log, assigned via TAG_CHANGE HERO_ENTITY at line 1445)
- Ghost battle opponent hero copies — appear with `player=<localPlayerID>` during combat

The fix (placeholder→real upgrade, retroactive stat apply) is tracked in **TODO-02**.

### Issue: Golden test files represent behavior, not ground truth

`internal/debugtui/testdata/golden/` files were previously generated with buggy hero
identification (entity 99 = real hero for `power_log_game.txt`). Regenerated after
the hero identification fix. Values are now correct but were never independently verified
against video footage for `power_log_game.txt`.

**Action:** When reviewing power_log_game.txt output, verify entity 99
(`TB_BaconShop_HERO_90_SKIN_E`) is actually the hero chosen in that game.

---

## Related plans

- [01](01-parseint-negative.md) — negative tag values
- [03](03-catspellcraft-rename.md) — CatSpellcraft rename (**DONE**)
- [04](04-tavern-tier-attribution.md) — tavern tier wrong player
- [12](12-win-loss-streak.md) — win/loss streak (**DONE**)
- [22](22-combat-damage-tags.md) — DAMAGED/DEFENDING tags
- [TODO-02](TODO-02-hero-identification.md) — hero entity identification
- [34](34-hero-damage-tracking.md) — hero DAMAGE tag not tracked (2026-03-08 log)
- [35](35-max-health-from-hero.md) — max health hardcoded to 40 (2026-03-08 log)
- [36](36-placement-in-result.md) — show placement number in TUI result
