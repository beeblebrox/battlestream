# Combat spell-cast tracking fix

## Problem

The **Spells Played** counter (`CatSpellsCast`) undercounted. In the repro duos game
(local = Moch#1358, Naga build) the tracker showed **85** but the true total spells the
local player cast — including spells triggered by the local player's own Nagas during
combat — was **136**.

## Root cause

`CatSpellsCast` was driven from the `NUM_SPELLS_PLAYED_THIS_GAME` player tag (via
`OnTavernSpellPlayed`). That tag counts **only hand-played spells during the recruit
phase** and is frozen during combat. In the repro game it topped out at 85.

The Hearthstone engine also tracks player tag **3809** (`SpellsPlayedForNagas`), an
**all-phases** cumulative count of every spell the local side casts, including
combat-triggered casts. In the repro log:

| phase   | `NUM_SPELLS_PLAYED_THIS_GAME` | tag `3809` |
|---------|-------------------------------|------------|
| recruit | 79 → 85 (lockstep)            | 124 → 130  |
| combat (`BACON_CURRENT_COMBAT_PLAYER_ID=1`) | **frozen at 85** | 130 → 136 (+6) |

3809 ended at 136 = 85 hand + 51 combat. This matches HDT's
`SpellsPlayedForNagasCounter.cs`, which reads tag 3809 directly (`Counter = value`) and
gates on `entity.IsControlledBy(Game.Player.Id)` (local side only).

## Fix

1. **Spells Played now sourced from tag 3809** (`OnSpellcraftChanged`,
   `internal/gamestate/processor_visitor.go`). The handler sets `CatSpellsCast` to the
   absolute 3809 value (HDT-style, idempotent), in addition to the existing Naga-synergy
   `CatNagaSpells` "Tier N · M/4" display. A monotonic guard ignores transient decreases
   (e.g. the `Bacon_TagTransferPlayerE` reset/restore cycle that briefly mirrors/zeroes
   3809) so the displayed total never goes backwards.

2. **The `NUM_SPELLS_PLAYED_THIS_GAME` path is fully severed (dead plumbing removed).**
   Driving `CatSpellsCast` from both `NUM_SPELLS_PLAYED_THIS_GAME` (incremental) and 3809
   (absolute) would conflict, so the `NUM_SPELLS_PLAYED_THIS_GAME` `TAG_CHANGE` case in
   `processor.go` was removed entirely, along with the now-unused `spellsPlayedTotal` field
   and its reset. `CatSpellsCast` now has exactly one writer (`OnSpellcraftChanged`), so no
   second path can drive or double-count it. `OnTavernSpellPlayed` remains only as an
   (unused) `RecruitVisitor` interface method, documented as a no-op.

3. **Corrected the misleading doc comment** on `OnSpellcraftChanged` (was: "identical in
   scope to NUM_SPELLS_PLAYED_THIS_GAME … every spell played from hand"). It now documents
   that 3809 is the all-phases total including combat casts.

## Duos safety

Tag 3809 lives on each player entity and only ticks for that player's own casts.
`OnSpellcraftChanged` filters via `actionIsLocalPlayerEntity`, so only Moch#1358's 3809
counts. In the repro log, partner/opponent player entities ("Phoenix", "Musicisbreth")
carry their own 3809 and are filtered out, as is the DNT `TagTransferPlayerEnchant` mirror
entity (neither a registered player-entity ID nor the local player name). The combat
increments 131→136 occur in the window where `BACON_CURRENT_COMBAT_PLAYER_ID=1` (the local
player's own combat), confirming they are the local side's casts.

Regression tests `TestSpellsCastTag3809NonLocalIgnored` and
`TestSpellsCastCombatIgnoresNonLocalSide` lock this in.

## Limitation: Spellcraft (`CatSpellcraftCast`)

Spellcraft was NOT switched to 3809. Combat-triggered casts arrive `SETASIDE→PLAY` without
the `SPELLCRAFT_HINT` flag, and 3809 is a single undifferentiated total — there is no clean
signal to separate spellcraft casts from regular spell casts among combat casts. Per the
"don't over-engineer" guidance, Spellcraft still counts only **hand-played spellcraft
cards** (`OnCombatTavernSpell`, gated on `SPELLCRAFT_HINT` + `HAND→PLAY` + local
controller). Combat-triggered spellcraft is therefore not counted.

In the repro game this shows as Spellcraft = 35 (hand-played) vs Spells Played = 136.

## Monotonic guard: decision

`OnSpellcraftChanged` raises `CatSpellsCast` only when `a.Value > current` rather than HDT's
plain `Counter = value`. **On the LOCAL player entity, 3809 is strictly monotonic** — it only
increments. The transient reset/restore dips (3809→0→N) observed in the repro log live on
NON-local entities (the DNT `Bacon_TagTransferPlayerE` enchantment mirror and combat copies),
which the `isLocal` gate already excludes; so HDT's plain assignment would behave identically
here. The guard is kept as belt-and-suspenders against an unforeseen local dip and does **not**
complicate reconnect reasoning: absolute assignment is idempotent, so restoring the counter to
N and re-emitting 3809=N lands at N (locked by `TestSpellsCastReconnectIdempotent`).

## Reconnect safety

On a mid-game reconnect, the live `AbilityCounters` (including `CatSpellsCast=N`) are stashed
and restored by `RestoreFromReconnect`. The engine then re-emits 3809 at the same absolute
value before continuing. Because the counter is set to the **absolute** 3809 value (not
delta-accumulated), a re-emitted 3809=N keeps it at N and a later 3809=N+k sets N+k — it can
never become 2N. This is the key advantage over the old `NUM_SPELLS_PLAYED_THIS_GAME` delta
path, which could double-count on reconnect. Locked by `TestSpellsCastReconnectIdempotent`.

## Risks flagged for review

- **Monotonic guard vs. true 3809 reset.** Only a hypothetical in-place downward re-baseline
  of 3809 on the LOCAL entity mid-game (never observed) would cause the counter to lag. New
  games / reconnect-as-new-game are handled by the full per-game reset on `EventGameStart`.
- **Non-Naga / solo games — unconditional set.** `CatSpellsCast` is set on every local 3809
  tick regardless of board contents; it is NOT gated behind `HasNagaSynergyMinion` (only
  `CatNagaSpells` is). The `game_log_2026_03_07` game reaches 3809=72 with no synergy minion:
  `CatSpellsCast` is present at 72 while `CatNagaSpells` is absent (locked by
  `TestGameLog2026_03_07_SpellsPlayedFinal` and `TestSpellsCastSetWithoutNagaMinion`). Same
  presence semantics as before, just a more complete count.
- **3809 vs NUM baseline divergence.** 3809 carries a higher absolute baseline than
  NUM_SPELLS at the same instant (124 vs 79 in the repro). This is expected — 3809 already
  includes earlier combat casts — but it means the displayed number is the true engine
  spell total, which may differ from a naive "spells I clicked from hand" count.

## Validation

- `go build ./cmd/battlestream` — clean.
- `go test -count=1 ./...` — all packages pass; `go vet ./...` clean.
- `./battlestream replay --dump --turn 13 --width 150 <repro>` → **Spells Played = 136**
  (was 85). Progression across turns: 85 → 100 → 123 → 136.
