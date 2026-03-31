# Reconnect-Aware Processor Handling

## Problem

When a player disconnects and reconnects mid-game, Hearthstone emits a new `CREATE_GAME` block in Power.log with the full current game state. The processor treats this as a brand-new game, wiping all accumulated state. This causes:

- Turn snapshots from before reconnect are lost
- Buff sources and ability counters reset to zero
- The correct hero CardID is overwritten by a placeholder or copy
- A new game ID is created, orphaning the original
- Modifications history is lost
- Partner identity and state are lost

## Detection

A reconnect CREATE_GAME is distinguishable from a fresh game by its GameEntity tags:

```
GameEntity EntityID=19
    tag=STATE value=RUNNING
    tag=TURN value=19        ← turn > 1 means mid-game
```

A fresh game has `TURN=1` (or no TURN tag) at CREATE_GAME time.

### Sequencing

Events arrive in this order:
1. `EventGameStart` — from `CREATE_GAME` line
2. `EventGameEntityTags` — from accumulated GameEntity `tag=` lines
3. `EventPlayerDef` — from `Player EntityID=...` lines
4. `EventEntityUpdate` — from `FULL_ENTITY` blocks (heroes, minions, etc.)

Detection happens at step 2, but reset happens at step 1. Solution: stash game-level state before reset at step 1, restore it if step 2 confirms reconnect.

## Design

### Stash before reset

On every `EventGameStart`, before the existing reset logic, capture a `reconnectStash` struct:

```go
type reconnectStash struct {
    gameID             string
    startTime          time.Time
    isDuos             bool
    partnerPlayerID    int
    partnerPlayerName  string
    heroCardID         string
    partnerHeroCardID  string
    turnSnapshots      []TurnSnapshot
    buffSources        []BuffSource
    abilityCounters    []AbilityCounter
    modifications      []Modification
    prevBuffSources    []BuffSource
    prevAbilityCtrs    []AbilityCounter
    prevModCount       int
    turn               int
}
```

This stash is stored on the processor. It is consumed (and cleared) when `EventGameEntityTags` fires.

### Reconnect detection in EventGameEntityTags

When `EventGameEntityTags` arrives, check:

```go
state := e.Tags["STATE"]
turn, _ := strconv.Atoi(e.Tags["TURN"])
if state == "RUNNING" && turn > 1 && p.reconnectStash != nil {
    p.restoreFromStash()
    p.isReconnect = true
}
p.reconnectStash = nil  // always clear after check
```

### Restore from stash

`restoreFromStash()` copies stashed values back into `machine.state`:

- `GameID` → original game ID (prevents duplicate DB entries)
- `StartTime` → original start time
- `IsDuos`, partner identity → preserved
- `HeroCardID`, `PartnerHeroCardID` → preserved
- `TurnSnapshots` → restored into machine
- `BuffSources`, `AbilityCounters` → restored into machine state
- `Modifications` → restored
- `prevBuffSources`, `prevAbilityCtrs`, `prevModCount` → restored for delta tracking
- `Turn` → restored (reconnect Player tags will also set this, but stash is authoritative)

### What resets regardless

Entity-level maps must be rebuilt because entity IDs may change on reconnect:

- `entityController` — rebuilt from FULL_ENTITY CONTROLLER tags
- `heroEntities` — rebuilt from FULL_ENTITY CARDTYPE=HERO
- `entityProps` — rebuilt from FULL_ENTITY blocks
- `playerEntityIDs`, `realPlayerIDs` — rebuilt from Player blocks

Player identity (`localPlayerID`, `localPlayerName`) is re-derived from the new CREATE_GAME Player blocks. This is correct — the Player entity IDs may differ.

### Hero identity protection

During reconnect, FULL_ENTITY blocks re-emit hero entities. The processor updates `machine.state.Player.HeroCardID` from these. With `isReconnect = true` and a non-empty stashed hero, skip hero CardID updates to preserve the real hero selected during mulligan.

The `isReconnect` flag is cleared on the next `EventGameStart`.

### Game ID continuity

The original game ID is preserved via the stash. This ensures:

- `HasGame()` dedup works (same game → one DB entry)
- Turn snapshots attach to the same game
- No orphaned partial games in the DB

### Acceptable losses

- Per-entity enchantment history from before reconnect — FULL_ENTITY re-emits current stats
- Board snapshot for combat restoration — rebuilt from reconnect FULL_ENTITY blocks
- Tribe tracking counters — rebuilt from reconnect entities
- Gold tracking resets — Player tags restore current values

## Testing

1. **Unit: reconnect detection** — GameEntity tags with STATE=RUNNING + TURN>1 triggers restore
2. **Unit: stash/restore** — buff sources, turn snapshots, hero CardID survive reconnect
3. **Unit: game ID preserved** — no duplicate game ID on reconnect
4. **Unit: hero not overwritten** — hero CardID from mulligan survives reconnect FULL_ENTITY
5. **Unit: fresh game not affected** — STATE=RUNNING + TURN=1 or STATE=COMPLETE does not trigger restore
6. **Integration: real Power.log** — the reconnect at 20:06 in the March 24 session produces correct game data
