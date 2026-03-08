# TODO — Issues, Concerns, and Improvements

Roughly ordered by impact / risk. Items marked [BUG] are known correctness
issues; [RISK] are fragile areas; [IMPROVEMENT] are quality-of-life or
forward-looking work.

---

## Parser

### [BUG] `EventPlayerUpdate` and `EventZoneChange` are defined but never emitted

`events.go` declares `EventPlayerUpdate` and `EventZoneChange` but the parser
never emits them. Zone changes arrive as `EventTagChange` with `tag=ZONE`, and
the processor handles them correctly that way. However, consumer code (REST/gRPC,
external tools) might register handlers expecting these event types and receive
nothing. Either remove the dead constants or route zone/player events through
them explicitly.

### [BUG] Timestamp has no date — wraps midnight incorrectly

`extractTimestamp` constructs a full `time.Time` by combining today's date with
the parsed `HH:MM:SS`. A game that crosses midnight will produce events with
timestamps that jump backwards. Games are typically short (<1h) so this rarely
manifests, but reparse of old logs will silently assign today's date to every
event regardless of when the log was written.

**Fix:** read the log file's mtime and use that as the reference date, or store a
session-start date when the first line of a session is seen.

### [BUG] `reBlockTag` requires exactly 4+ leading spaces

The indented tag regex `-\s{4,}tag=` hard-codes a minimum indent of 4 spaces. If
Blizzard ever changes the indentation depth, continuation lines will not be
recognised and multi-tag entity blocks will emit incomplete events (missing ATK,
HEALTH, ZONE, etc.) without any warning.

**Fix:** make the indent threshold configurable or at minimum log a warning when a
`FULL_ENTITY` block is flushed with an empty `Tags` map.

### [RISK] Parser state is not reset between games

`Parser.inBlock`, `Parser.pending`, and `Parser.blockStack` are never explicitly
cleared when an `EventGameStart` is seen. If the previous game ended mid-block
(e.g. HS crashed, log was truncated), the next game will begin with stale block
state and may emit a garbage `EventEntityUpdate` for the first non-block line.

**Fix:** call `p.flushPending()` and `p.blockStack = p.blockStack[:0]` inside the
`EventGameStart` arm of `Feed()`.

### [RISK] `reTagChange` is a catch-all with no priority over `reTurnStart`

Both `reTurnStart` and `reTagChange` match `TAG_CHANGE` lines; `reTurnStart` is
listed first in the switch. If a future Blizzard log version adds an intermediary
`TAG_CHANGE Entity=GameEntity tag=STATE ...` before the `TURN` line, it will fall
through to the generic handler and produce a spurious `EventTagChange` for
GameEntity (which the processor largely ignores, but it's noise). The current
approach of checking specific patterns first is correct but brittle — document
it clearly and add a test.

### [IMPROVEMENT] No structural parsing for BLOCK_START `BlockType`

`BlockType=ATTACK`, `BlockType=POWER`, `BlockType=PLAY`, etc. are all currently
ignored. Capturing block type would allow the processor to distinguish e.g.
combat attack blocks from spell play blocks for more precise buff attribution,
and would allow filtering combat stat changes more reliably than the phase check.

### [IMPROVEMENT] No position tracking (zone position)

Hearthstone entities have a `ZONE_POSITION` tag that encodes board position
(left to right). The parser reads it as a generic tag, and the processor ignores
it. Board position is essential for correctly interpreting position-dependent
buffs (e.g. `CatRightmost`) and for displaying minions in board order.

### [IMPROVEMENT] No `DAMAGED` / `DEFENDING` / `ATTACKING` tag handling

During combat, ATK/HEALTH changes that represent damage (not buffs) are currently
indistinguishable from genuine stat changes. The processor heuristically suppresses
them via the `PhaseCombat` check, but this is fragile — a buff that fires during
combat (e.g. triggered abilities) would be silently dropped.

---

## Processor / Game State

### [BUG] `isLocalPlayerEntity` uses player name string match which can false-positive

```go
if p.localPlayerName != "" && e.EntityName == p.localPlayerName {
    return true
}
```

If another player has the same BattleTag as the local player (unlikely but
theoretically possible in practice through name changes / edge cases), their
tag changes will be attributed to the local player. The entity ID check via
`localHeroID` / `entityController` is more reliable. The name fallback should be
a last resort only.

### [BUG] `handleTagChange` applies `PLAYER_TECH_LEVEL`/`TAVERN_TIER` when `controllerID == 0`

```go
if controllerID == p.localPlayerID || p.isLocalPlayerEntity(e) {
    p.machine.SetTavernTier(tier)
}
```

When `controllerID` resolves to `0` (unknown entity) and `isLocalPlayerEntity`
returns `false`, this is correct. But if `controllerID` is 0 and
`isLocalPlayerEntity` is also unreliable (name not yet set), an opponent's tier
change could be accepted. Add an explicit guard: only apply if either the entity
is positively identified as local.

### [BUG] `parseInt` silently accepts negative numbers as positive

```go
func parseInt(s string) int {
    n := 0
    for _, c := range s {
        if c >= '0' && c <= '9' { n = n*10 + int(c-'0') }
    }
    return n
}
```

The `-` character is ignored, so `parseInt("-5")` returns `5`. This will
misinterpret tag values that are legitimately negative (e.g. a debuff tag). Use
`strconv.Atoi` with error handling, or at minimum handle the leading minus sign.

### [RISK] Board snapshot / restore is unconditional on non-empty

`GameEnd` always restores `boardSnapshot` if non-empty, even if a valid recruit
board was already established after the last combat round. If the combat for the
final round produces combat copy entities that end up in the snapshot (via
`UpdateBoardSnapshot`), the restored board will have base-stat copies rather than
recruit-phase stats.

Verify with integration tests that `UpdateBoardSnapshot` (called from
`tryAddMinionFromRegistry` during PhaseCombat) only overwrites the snapshot with
combat copies that *already have* correct buffed stats from their TAG_CHANGE
sequence, not with the initial base-stat FULL_ENTITY values.

### [RISK] `pendingStatChanges` is never bounded

If a game runs for many turns with many board-wide buffs, `pendingStatChanges`
grows without limit until `flushPendingStatChanges` is called. In practice flush
is called at every turn boundary, so this is bounded by minions-per-turn. But if a
turn boundary event is missed (e.g. due to a parser edge case), the buffer leaks
across turns and the grouping logic will produce incorrect cross-turn matches.

**Fix:** add a per-turn cap or flush-on-capacity guard.

### [IMPROVEMENT] `gameSeq` is an in-process counter, not a stable game ID

`gameSeq` resets to 0 on daemon restart. If the daemon is restarted mid-session,
the next game gets `game-1` again, which will collide with a previously stored
game if the store key is based on this ID. The store uses BadgerDB with a separate
dedup key, but the collision will appear in logs and may confuse tooling.

**Fix:** derive game ID from the session start timestamp (first `CREATE_GAME`
timestamp), which is stable across restarts.

### [IMPROVEMENT] No opponent tracking

Opponent hero, health, board, and buffs are not tracked. `BGGameState.Opponent`
and `OpponentBoard` exist in the struct but are never populated. This is a major
feature gap for overlay/stream use cases that want to show who you're fighting.

### [IMPROVEMENT] No win/loss streak tracking

`PlayerState` has `WinStreak` and `LossStreak` fields but they are never
populated. The log contains `WINNING_PLAYER` and `LOSING_PLAYER` tag changes
during combat that could be used to derive streak data.

### [IMPROVEMENT] No coin/gold tracking

`PlayerState.CurrentGold` is declared but never set. Hearthstone logs `RESOURCES`
tag changes that encode available gold, which could be captured.

### [IMPROVEMENT] `Modifications []StatMod` — `Source`, `Category`, `CardID` always empty

The board-wide buff detection populates `Turn`, `Target`, `Stat`, `Delta` but
leaves `Source`, `Category`, and `CardID` blank. The block source context
(`BlockSource`, `BlockCardID`) is available on the event but is not used here to
fill in the source card.

---

## Categories / Buff Tracking

### [RISK] Enchantment CardID table is manually curated and will go stale

`categories.go` contains a hardcoded map from CardID strings to buff categories.
Every new BG season/patch that adds new buff mechanics requires a manual update.
There is no mechanism to detect missing entries at runtime.

**Mitigation:** The `buff-scout` agent (`.claude/agents/buff-scout.md`) provides a
workflow for auditing the HDT reference. Consider adding an automated scan of the
reference directory to flag unmapped enchantment IDs seen in game logs.

### [RISK] `CatLightfang` and `CatConsumed` have no Dnt handlers

These categories are defined in `categories.go` and appear in
`categoryByEnchantmentCardID`, but there is no corresponding case in
`handleDntTagChange`. Per-minion enchantments for these will be added to
`machine.AddEnchantment` correctly, but the cumulative buff counter will never
be updated — the TUI panel will show them as `0/0` or not at all.

Verify whether these categories actually use Dnt counters (they may be purely
per-minion enchantments), and document the answer explicitly.

### ~~[BUG] `CatSpellcraft` is a misnomer — this counter tracks Naga spells, not Spellcraft~~ RESOLVED

The constant `CatSpellcraft` and all associated display labels are incorrect. Tag `3809`
is **not** related to the Spellcraft keyword mechanic (which gives a temporary spell each
turn to Spellcraft minions). It is HDT's `SpellsPlayedForNagasCounter` — it counts the
**total number of spells cast this game** specifically for Naga minions whose permanent
buff scales with spells played (Arcane Cannoneer, Thaumaturgist, Showy Cyclist,
Groundbreaker). These minions receive a growing permanent buff per spell cast, and the
buff amount increases by +1 for every 4 spells played over the course of the game.

**Display is also wrong.** The current display formula `stacks (progress/4)` is copied
from HDT but never explained, and the label "Spellcraft" in the TUI will mislead any
user who knows what the Spellcraft keyword actually does.

**Fixes needed:**
1. Rename `CatSpellcraft` → `CatNagaSpells` (or similar) in `categories.go`,
   `processor.go`, and `CategoryDisplayName`.
2. Update `CategoryDisplayName` label from `"Spellcraft"` to something like
   `"Naga Spells"` or `"Spells Cast"`.
3. Update `PARSER_SPEC.md` and `PARSER.md` documentation which currently describe
   this counter incorrectly as "Spellcraft stacks".
4. Clarify the display: `N (M/4)` means "current buff-per-spell tier is +N
   (M of 4 spells into the next tier)". Consider a clearer format such as
   `"Tier N · M/4 spells"` or add a tooltip/label in the TUI.

### [IMPROVEMENT] No support for trinkets / artifacts (post-2025 mechanic)

Trinkets introduced in later BG patches may add new buff tag patterns or
enchantment CardIDs not yet covered. The buff-scout workflow should be run
against the latest HDT reference after each major patch.

---

## Architecture / Pipeline

### [RISK] Parser and Processor are tightly coupled via an unbuffered pattern

The `chan GameEvent` between parser and processor has no documented size contract.
In the daemon, if the processor blocks (e.g. waiting on a lock), the parser will
block on `p.out <- e`, which blocks the watcher's goroutine, which stops reading
from the tail, which can cause the tail buffer to fill. Under log replay (reparse),
this is especially risky with large logs.

**Fix:** document or enforce the channel buffer size; consider a larger buffer
(currently inherited from whatever the caller provides).

### [IMPROVEMENT] Reparse does not reset in-process seq counter

`battlestream reparse` re-feeds all historical log lines through a fresh parser
and processor but the daemon's in-process `gameSeq` counter is not reset first.
If the daemon is running while reparse is triggered, seq numbers will be
inconsistent between the daemon's live state and the reparsed store entries.

### [IMPROVEMENT] No replay / snapshot isolation

There is no way to ask "what was the board state at turn N?" All state is live
and mutable. For debugging, historical board snapshots are discarded after each
combat round.

### [IMPROVEMENT] Integration test coverage is limited to one log

`testdata/power_log_game.txt` (92K lines) covers one specific game. Edge cases
like Nomi All, Timewarped mechanics, late-game high-health scenarios, and games
ending during combat are not covered by automated tests.

The debug TUI provides golden-file screenshot tests (`internal/debugtui/testdata/golden/`)
for turn 1, mid-game, and last-turn views. See [docs/DEBUGTUI.md](DEBUGTUI.md)
for the full test strategy and how to add new golden files.

---

## API / Output

### [IMPROVEMENT] REST `/v1/game/current` returns a full deep copy on every poll

The `machine.State()` method deep-copies all slices (board, enchantments, mods,
buff sources) on every read. For polling clients or SSE subscribers, this creates
GC pressure proportional to board complexity. Consider a generation counter or
dirty flag so clients can detect no-change and the server can skip serialisation.

### [IMPROVEMENT] No game history endpoint pagination

`/v1/stats/games` returns all stored games. As the store grows over multiple
sessions, this response will become large. Add cursor-based pagination.

### [IMPROVEMENT] WebSocket / SSE emit full state on every change

The current SSE/WS implementation broadcasts the complete `BGGameState` snapshot
on every event. For high-frequency log phases (e.g. combat), this can produce
dozens of full-state pushes per second. Consider a delta/patch protocol or a
minimum emit interval.

---

## Operational

### [IMPROVEMENT] No structured error recovery for parser panics

If a regex match produces an unexpected capture group layout (e.g. due to a log
format change), the parser will panic with an index-out-of-range. Wrapping
`Feed()` in a recover and logging the offending line would make the daemon
resilient to unexpected log format changes.

### [IMPROVEMENT] Log verbosity is not configurable at runtime

`slog.Info` calls are scattered throughout the processor (player identification,
hero entity updates, etc.). There is no way to reduce verbosity without
recompiling. Add a log level flag or env var (`BS_LOG_LEVEL`).

### [IMPROVEMENT] No metrics / observability

No Prometheus or similar metrics are exported. Useful counters would include:
lines parsed/sec, events emitted per type, games completed, buff sources updated.
