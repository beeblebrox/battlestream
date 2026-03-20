# Duos TUI Overhaul — Design Document

**Date:** 2026-03-19
**Status:** Approved

## Summary

Fix duos stat tracking bugs, add partner board visibility, improve disconnect
handling, and unify the duos experience across live and replay TUIs.

## Motivation

Three duos game sessions revealed multiple issues:

1. **Duos detection fails** in 2/3 sessions — `BACON_DUO_TEAMMATE_PLAYER_ID` is
   absent from reconnect CREATE_GAME blocks and some fresh game starts. Games
   display as solo, breaking all duos-specific logic.
2. **Buff sources misattributed** — when duos isn't detected, enchantment
   controller filtering drops local player's buffs (Dnt enchantments sometimes
   have `CONTROLLER=botID` in duos). Turn 10 boards with 70+ stat minions show
   only `Tavern Spells +1/+4`.
3. **Partner info broken** — name blank, hero shows raw CardID (`BG32_HERO_002`
   instead of "Buttons").
4. **Partner board believed impossible** — CLAUDE.md stated partner board is not
   available from Power.log. Log analysis proved this wrong: partner board IS
   visible during combat phase via SHOW_ENTITY reveals and
   `BACON_CURRENT_COMBAT_PLAYER_ID`.
5. **Disconnect handling missing** — games that crash without `STATE=COMPLETE`
   leave the daemon in a stuck "active game" state indefinitely.

## Evidence

### Session Hearthstone_2026_03_16_22_05_57 (Reconnect)

- Client crash, reconnected into new session directory
- `BACON_DUOS_PUNISH_LEAVERS` present on GameEntity, `BACON_DUO_TEAMMATE_PLAYER_ID` absent
- `IsPastBeginPhase()=True` in LoadingScreen.log (HDT reconnect signal)
- CREATE_GAME carries mid-game state (TURN=7)
- GameNetLogger.log: `reconnecting=True`

### Session Hearthstone_2026_03_16_22_58_06 (Undetected Duos)

- Fresh game start, completed normally
- `BACON_DUOS_PUNISH_LEAVERS` present, `BACON_DUO_TEAMMATE_PLAYER_ID` absent
- `BACON_DUO_PASSABLE` tags on cards (partner card passing)
- `BG_ShopBuff` Dnt entities had `player=13` (bot) early game, `player=5` (local) later
- Buff tracking showed `Tavern Spells +1/+4` at turn 10 despite 70+/50+ stat minions

### Session Hearthstone_2026_03_17_19_33_31 (Detected Duos, Crash)

- `BACON_DUO_TEAMMATE_PLAYER_ID=6` present — duos correctly detected
- Partner hero identified: entity 146, `BG32_HERO_002` (Buttons), `player=13`
- Partner name blank in TUI, hero shown as raw CardID
- Disconnected at turn 18, reconnect failed (`GameCanceled`), no `STATE=COMPLETE`
- 106MB Power.log ends abruptly

### Combat Entity Analysis

- ALL combat copy minions have `CONTROLLER=<botID>` regardless of owner
- `BACON_CURRENT_COMBAT_PLAYER_ID=<partnerID>` identifies partner's combat
- SHOW_ENTITY reveals card names/stats for combat copies
- `COPIED_FROM_ENTITY_ID` traces back to recruit-phase entities
- Partner hero copy gets `PLAYER_ID=<partnerID>` — distinguishes partner's side
- During recruit phase, partner board is NOT visible (hidden entities, empty CardID)

---

## Design

### 1. Duos Detection (Multi-Signal)

**Current:** Only `BACON_DUO_TEAMMATE_PLAYER_ID` on local player entity.

**New — three detection signals:**

1. **Primary:** `BACON_DUO_TEAMMATE_PLAYER_ID` on local player entity — gives
   partnerPlayerID directly (existing path).
2. **Fallback:** `BACON_DUOS_PUNISH_LEAVERS` on GameEntity during CREATE_GAME —
   present in all observed duos sessions. Sets `isDuos=true` without knowing
   partnerPlayerID yet.
3. **Confirmation:** `BACON_DUO_PASSABLE` on any entity — confirms duos if not
   already detected.

**Deferred partner resolution:** When duos is detected without a partner ID:

- `BACON_CURRENT_COMBAT_PLAYER_ID` with a value != localPlayerID identifies the
  partner (first non-local combat player seen).
- Once partnerPlayerID is known, retroactively scan cached hero entities for one
  with `PLAYER_ID=<partnerID>` to set partnerHeroID.
- Retroactively resolve partner name from cached player name events.

**File changes:** `internal/gamestate/processor.go` — `handlePlayerDef`,
`handleTagChange`, new `handleGameEntityTag` method.

### 2. Buff Attribution Fix

**Problem:** In duos, some local player Dnt enchantments have
`CONTROLLER=<botID>` instead of `CONTROLLER=<localPlayerID>`. The check at
`handleDntTagChange:1171` (`ctrl != p.localPlayerID`) drops them.

**Fix:** For Dnt enchantments, relax the controller check. Accept enchantments
where:

- `CONTROLLER == localPlayerID` (existing — works when correct), OR
- `ATTACHED` target is the local player's hero entity or player entity, OR
- `CREATOR` is the local player's entity

This is targeted to player-level Dnt enchantments only. Minion enchantments keep
strict controller matching to avoid counting opponent buffs.

**File changes:** `internal/gamestate/processor.go` — `handleDntTagChange`,
`handleEnchantmentEntity`.

### 3. Partner Board Tracking

**Data model** (`internal/gamestate/state.go`):

```go
type PartnerBoard struct {
    Minions []MinionState
    Turn    int   // BG turn when captured
    Stale   bool  // true if partner didn't fight this turn
}
```

Added to `BGGameState`:
```go
PartnerBoard *PartnerBoard  // nil until first partner combat
```

**Capture mechanism** (`internal/gamestate/processor.go`):

1. On `BACON_CURRENT_COMBAT_PLAYER_ID=<partnerID>`: set
   `partnerCombatActive=true`, begin collecting combat copy minions.
2. Collect FULL_ENTITY minions created during this combat. SHOW_ENTITY reveals
   give CardID. TAG_CHANGE gives buffed ATK/HEALTH.
3. Distinguish partner's minions from opponent's: the partner's hero copy in the
   combat gets `PLAYER_ID=<partnerID>`. Combat copies on the partner's side can
   be identified via `COPIED_FROM_ENTITY_ID` tracing back to entities with
   matching PLAYER_ID, or by the hero's side in the combat layout.
4. On combat end (next `BACON_CURRENT_COMBAT_PLAYER_ID` change or phase
   transition): snapshot collected minions into `PartnerBoard` with current turn.
5. On recruit phase start: if partner didn't fight this turn, set
   `PartnerBoard.Stale = true` (preserves previous data with staleness flag).

**Proto** (`proto/battlestream/v1/game.proto`):

Unreserve field 19 and add:
```protobuf
repeated MinionState partner_board = 19;
int32 partner_board_turn = 25;
bool partner_board_stale = 26;
```

### 4. Partner Display Fixes

**Partner name resolution:**

- Call `CardName(partner.HeroCardID)` in debug TUI renderer (live TUI already
  does this).
- With deferred partner resolution, retroactively set name once partnerPlayerID
  is known.
- Fallback: use hero entity's `entityName` from FULL_ENTITY block.

**Health display in duos:**

- Local player panel: health label shows "(Team)" suffix since health/armor are
  shared in duos.
- Partner panel: shows tier, triples, hero only — removes redundant health
  display (it's the same shared pool).

### 5. Disconnect & Incomplete Game Handling

**Staleness timer:**

- Track timestamp of last meaningful Power.log event.
- If no events arrive for 3 minutes and game hasn't completed: mark game as
  `Phase=GAME_OVER` with synthetic completion.
- Placement set to 0 (unknown) — do not persist to stats DB (would skew
  aggregates).
- Clear "active game" state so TUI shows "Waiting for game..."

**Reconnect continuity:**

- On reconnect CREATE_GAME (detected by mid-game tags like `TURN>0`,
  `STATE=RUNNING`), re-run duos detection using multi-signal approach.
- Partner board snapshot from previous session is lost — stays nil until next
  partner combat.

**No GameNetLogger.log parsing for now.** The staleness timer handles
user-facing symptoms. GameNetLogger.log signals documented in
`plan/37-gamenetlogger-signals.md` for future work.

### 6. TUI Layout

**Both TUIs (live and debug/replay) share the same duos layout additions.**

#### Solo Mode

Unchanged.

#### Duos Mode

```
Row 1 (top):
  +-- GAME PANEL [DUOS] ----------+ +-- HERO PANEL ------------------+
  | Phase, Turn, Tavern, Anomaly  | | Player Name                    |
  |                               | | Health (Team) ████░ 23/30      |
  |                               | | Triples, Gold, Hero            |
  |                               | | Last WIN/LOSS                  |
  |                               | | -- Partner --                  |
  |                               | | Name: Fizzy                    |
  |                               | | Hero: Buttons                  |
  |                               | | Tavern *******  Triples: 4     |
  +-------------------------------+ +--------------------------------+

Row 2 (main):
  +-- YOUR BOARD ----------------+ +-- BUFF SOURCES ----------------+
  | (same as today)              | | (same as today)                |
  +------------------------------+ +--------------------------------+

Row 3 (duos only — scrollable viewport):
  +-- PARTNER BOARD (Turn 17) -----------------------------------------+
  | Crackling Cyclone      220/177  Mirror Monster       150/132       |
  | Nightmare Par-tea Gu~  210/185  Felfire Conjurer      88/71        |
  | Timewarped Nalaa        80/81                                      |
  +--------------------------------------------------------------------+

Row 4: Session stats bar
Row 5: Help bar
```

**Partner board panel:**

- Full width below main panels.
- Dynamic column count based on panel inner width:
  - 120+ chars: 3 columns
  - 80-119 chars: 2 columns
  - <80 chars: 1 column
- Each column is fixed-width (innerWidth / numColumns). Minion names
  left-aligned, stats right-aligned within column.
- Fills available vertical space. Scrollable via mouse wheel when cursor is
  over the panel (uses `viewport.Model`, same pattern as existing panels).
- Keyboard scroll with `J/K` when panel is focused (existing panel cycling).
- Title includes turn: "PARTNER BOARD (Turn 17)"
- If stale: "PARTNER BOARD (Turn 15 - last seen)"
- If no combat observed: "(awaiting first combat)"
- Always visible in duos mode (not toggled).

**Debug/Replay TUI:**

- Same partner board panel layout.
- `[d]` key toggles between partner board and partner info (hero/tier/triples)
  for height-constrained terminals.

### 7. Testing Strategy

**Unit tests:**

- Duos detection: all three signals, deferred partner resolution.
- Buff attribution: Dnt enchantments with `CONTROLLER=botID` but
  `ATTACHED=localHero` are tracked.
- Partner board capture: combat copy collection during partner combat, snapshot
  stores correct minions/turn, staleness flag.
- Staleness timeout: games without `STATE=COMPLETE` get cleaned up.

**Integration tests:**

Real duos log files as test data (extract relevant snippets to keep test data
manageable):

- `Hearthstone_2026_03_16_22_05_57/Power.log` — reconnect, duos without
  teammate tag
- `Hearthstone_2026_03_16_22_58_06/Power.log` — duos without teammate tag,
  full game
- `Hearthstone_2026_03_17_19_33_31/Power.log` — duos with teammate tag,
  crash/no complete

**TUI golden tests:**

Use `replay --dump --turn N` on duos logs and compare output:

- `[DUOS]` badge appears in all duos sessions
- Partner name and hero name are resolved (not blank/raw CardID)
- Partner board panel renders with correct turn label
- Buff sources show reasonable non-zero values

---

## Files Affected

| File | Changes |
|------|---------|
| `internal/gamestate/processor.go` | Duos detection, buff attribution, partner board capture, staleness timer |
| `internal/gamestate/state.go` | `PartnerBoard` struct, `BGGameState` field |
| `internal/gamestate/machine.go` | Partner board snapshot methods |
| `internal/gamestate/categories.go` | No changes expected |
| `internal/tui/tui.go` | Partner board panel, health "(Team)" label |
| `internal/debugtui/model.go` | Partner board panel, hero name resolution, toggle behavior |
| `proto/battlestream/v1/game.proto` | Unreserve field 19, add fields 25-26 |
| `internal/api/grpc/gen/` | Regenerated proto code |
| `internal/api/grpc/server.go` | Serialize partner board |
| `internal/fileout/fileout.go` | Serialize partner board |
| `internal/gamestate/*_test.go` | New unit + integration tests |
| `CLAUDE.md` | Update duos documentation |

## Non-Goals

- GameNetLogger.log parsing (documented in `plan/37-gamenetlogger-signals.md`)
- Partner buff source tracking (enchantments indistinguishable from opponents)
- Partner ability counters (same limitation)
- Opponent board tracking (separate concern, `plan/10-opponent-tracking.md`)
