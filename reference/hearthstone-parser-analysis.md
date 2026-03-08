# hearthstone-parser Analysis

**Project**: `hearthstone-parser` (TypeScript/Node.js) -- A real-time log watcher and event-emitting parser for Hearthstone's `output_log.txt` / `Power.log`.

## Overview

This parser watches the log file with chokidar, reads incremental chunks, runs each line through an ordered array of `LineParser` instances, and emits typed events. It maintains a `GameState` object with two players, card tracking, match logging, discovery tracking, and mulligan handling.

## Architecture

The parser uses two log stream prefixes simultaneously:
- **GameState stream**: `[Power] GameState.DebugPrintPower() -` (used for match log / block reading)
- **PowerTaskList stream**: `[Power] PowerTaskList.DebugPrintPower() -` (used for entity resolution and game-over/turn/tag events)
- **Zone stream**: `[Zone] ZoneChangeList.ProcessChanges()` (used for zone change tracking)

Line parsers are ordered -- the first match wins:
```typescript
export const lineParsers = [
    new MatchLogParser(),      // Block-level match log (GameState + PowerTaskList)
    new GameOverLineParser(),  // PLAYSTATE changes
    new GameStartLineParser(), // CREATE_GAME
    new NewPlayerLineParser(), // PlayerID= lines
    new TurnLineParser(),      // CURRENT_PLAYER changes
    new ZoneChangeLineParser(),// Zone transitions
    new TagChangeLineParser(), // Quest progress
    new GameTagChangeLineParser(), // STEP/TIMEOUT/MULLIGAN_STATE
    new MulliganStartParser(),
    new DiscoveryParser(),
    new MullinganResultParser(),
    new CardInitParser()
];
```

## All Regex Patterns

### Game Lifecycle

**Game start** (`GameStartLineParser`):
```typescript
/\[Power\] GameState\.DebugPrintPower\(\) -\s*CREATE_GAME/
```

**Game over** (`GameOverLineParser`):
```typescript
/\[Power\] PowerTaskList\.DebugPrintPower\(\) -\s+TAG_CHANGE Entity=(.*) tag=PLAYSTATE value=(LOST|WON|TIED)/
```

**New player** (`NewPlayerLineParser`):
```typescript
/\[Power\] GameState\.DebugPrintGame\(\) - PlayerID=(\d), PlayerName=(.*)$/
```

### Turn and Tag Tracking

**Turn change** (`TurnLineParser`) -- uses PowerTaskList:
```typescript
/^\[Power\] PowerTaskList\.DebugPrintPower\(\) -\s*TAG_CHANGE Entity=(.*) tag=CURRENT_PLAYER value=(\d)/
```

**Game tag change** (`GameTagChangeLineParser`) -- uses PowerTaskList:
```typescript
/^\[Power\] PowerTaskList.DebugPrintPower\(\) -\s+TAG_CHANGE Entity=(.*) tag=(.*) value=(.*)/
```

**Tag change (quest progress)** (`TagChangeLineParser`) -- uses PowerTaskList, entity string format:
```typescript
/^\[Power\] PowerTaskList.DebugPrintPower\(\) -\s+TAG_CHANGE Entity=\[entityName=(.*) id=(\d*) zone=.* zonePos=\d* cardId=(.*) player=(\d)\] tag=(.*) value=(\d*)/
```

### Zone Changes

**Zone change** (`ZoneChangeLineParser`) -- uses `[Zone]` log:
```typescript
/^\[Zone\] ZoneChangeList\.ProcessChanges\(\) - id=\d* local=.* \[entityName=(.*) id=(\d*) zone=.* zonePos=\d* cardId=(.*) player=(\d)\] zone from ?(FRIENDLY|OPPOSING)? ?(.*)? -> ?(FRIENDLY|OPPOSING)? ?(.*)?$/
```

### Mulligan

**Mulligan start** (`MulliganStartParser`):
```typescript
/\[Power\] GameState\.DebugPrintPower\(\) -\s*tag=MULLIGAN_STATE value=INPUT/
```

**Mulligan result** (`MullinganResultParser`):
```typescript
/\[Power\]\s+GameState\.DebugPrintEntitiesChosen\(\)\s+-\s+id=\w+\s+Player=(.*)\s+EntitiesCount=(\d+)/
```

**Begin phase end** (`CardInitParser`):
```typescript
/\[LoadingScreen\] MulliganManager.HandleGameStart\(\) - IsPastBeginPhase\(\)=False/
```

### Discovery

**Discover start**:
```typescript
/^\[Power\]\s+GameState\.DebugPrintEntityChoices\(\)\s+-\s+id=(\w+)\s+Player=(.*)\s+TaskList=.*\s+ChoiceType=GENERAL/
```

**Discover source**:
```typescript
/^\[Power\]\s+GameState\.DebugPrintEntityChoices\(\)\s-\s+Source=(.*)/
```

**Discover option**:
```typescript
/^\[Power\]\s+GameState\.DebugPrintEntityChoices\(\)\s-\s+Entities\[([0-9]+)\]=(.*)/
```

**Discover shown**:
```typescript
/^\[Power\]\s+ChoiceCardMgr\.WaitThenShowChoices\(\)\s+-\s+id=(\w+)\s+BEGIN/
```

**Discover chosen**:
```typescript
/^\[Power\]\s+GameState\.DebugPrintEntitiesChosen\(\)\s+-\s+Entities\[([0-9]+)\]=(.*)/
```

**Discover end**:
```typescript
/^\[Power\]\s+ChoiceCardMgr\.WaitThenHideChoicesFromPacket\(\)\s+-\s+id=(\w+)\s+END\s+WAIT/
```

### Block/Entity Reading (internal readers)

**Block start** (`BlockReader`):
```typescript
/\s*BLOCK_START BlockType=([A-Z]*) Entity=(.*) EffectCardId=(.*) EffectIndex=(.*) Target=(.*) SubOption=(.*) (?:TriggerKeyword=(.*))?/
```

**Tag change inside block** (`BlockReader`):
```typescript
/\s*TAG_CHANGE Entity=(.*) tag=(.*) value=([\w\d_.-]*)/
```

**Meta data** (`BlockReader`):
```typescript
/\s*META_DATA - Meta=([A-Z]+) Data=(\d*) Info(?:Count)?=(.*)/
```

**SubSpell start** (`BlockReader`):
```typescript
/\s*SUB_SPELL_START - SpellPrefabGUID=(.*):(.*) Source=(.*) TargetCount=(.*)/
```

**Full entity start** (`FullEntityReader`):
```typescript
/^(\s*)FULL_ENTITY - (Creating|Updating) (?:ID=(\d+)|(\[.*\])) CardID=(.*)/
```

**Change entity** (`FullEntityReader`):
```typescript
/^(\s*)CHANGE_ENTITY - Updating Entity=(\[.*\]) CardID=(.*)/
```

**Show entity** (`FullEntityReader`):
```typescript
/^(\s*)SHOW_ENTITY - Updating Entity=(.*) CardID=(.*)/
```

**Tag line (inside entity block)** (`FullEntityReader`):
```typescript
/^(\s*)tag=(.*) value=(.*)/
```

**Entity string parser** (`readEntityString` in `base.ts`):
```typescript
/\[entityName=(.*) (?:\[cardType=(.*)\] )?id=(\d*) zone=.* zonePos=\d* cardId=(.*) player=(\d)\]/
```

## How It Identifies the Local/Friendly Player

The parser uses **FRIENDLY/OPPOSING from the `[Zone]` log** during mulligan to establish which player is "bottom" (local) and which is "top" (opponent):

```typescript
// In ZoneChangeLineParser
if (data.toZone === 'HAND' || data.toZone === 'DECK') {
    if (data.toTeam === 'FRIENDLY') {
        player.position = 'bottom';
        otherPlayer.position = 'top';
    }
    if (data.toTeam === 'OPPOSING') {
        player.position = 'top';
        otherPlayer.position = 'bottom';
    }
}
```

The `[Zone]` log is the **only** source of FRIENDLY/OPPOSING information. The `[Power]` log does not contain this distinction. Once positions are established during mulligan, they persist for the game.

Additionally, the player receiving "The Coin" is identified as going second:
```typescript
if (data.cardName === 'The Coin' && data.toZone === 'HAND') {
    otherPlayer.turn = true;
}
```

The first player listed by `GameState.DebugPrintGame()` (PlayerID=1) is assigned `position: 'bottom'` initially, then corrected by zone data.

## How It Tracks Entity Ownership

Ownership is tracked via the `player=(\d)` field in entity strings:

```
[entityName=Fireball id=42 zone=HAND zonePos=3 cardId=CS2_029 player=1]
```

The `player` field maps to a `Player.id`, and `identifyPlayer()` converts that to `'top'` or `'bottom'`:

```typescript
export const identifyPlayer = (gameState: GameState, playerIndex: number) => {
    const player = gameState.getPlayerById(playerIndex);
    if (player) {
        return player.position;
    }
    // ...fallback
};
```

The CONTROLLER tag is tracked inside `FullEntityReader` when reading entity tag blocks:
```typescript
if (tagData.tag === 'CONTROLLER') {
    this._entity.player = identifyPlayer(gameState, parseInt(tagData.value, 10));
}
```

## How It Handles Turns

Turn tracking uses `CURRENT_PLAYER` tag changes from PowerTaskList:

```typescript
// TurnLineParser
regex = /^\[Power\] PowerTaskList\.DebugPrintPower\(\) -\s*TAG_CHANGE Entity=(.*) tag=CURRENT_PLAYER value=(\d)/;
```

When `value=1`, the named player's turn begins. When `value=0`, it ends. Turn history is tracked with timestamps and durations. The opponent's turn state is toggled inversely (`opponent.turn = !data.turn`).

Game phases are tracked via `STEP` and `NEXT_STEP` tag changes on `GameEntity`:
- `STEP=MAIN_READY` starts the turn timer
- `NEXT_STEP=MAIN_READY` ends mulligan
- `MULLIGAN_STATE=INPUT` activates mulligan

**Note**: This is designed for standard 2-player Hearthstone. For Battlegrounds with 8+ players, the `CURRENT_PLAYER` pattern still applies but only for the two players in the current combat/shop phase.

## How It Tracks Board State (Zones)

Board state is tracked through the `[Zone]` log via `ZoneChangeLineParser`. The regex captures:
- Source team (FRIENDLY/OPPOSING)
- Source zone (HAND, DECK, PLAY, SECRET, GRAVEYARD, etc.)
- Destination team
- Destination zone

Cards are tracked per player with states: `'DECK' | 'HAND' | 'OTHERS'`.

Zone positions are tracked via `ZONE_POSITION` tags in block entries (used by `MatchLogParser` to detect cards entering play).

The parser also tracks:
- `cardCount` per player (incremented on DECK entry, decremented on DECK exit)
- Secrets (filtered by a card-to-class map)
- Quests (with progress tracking via `QUEST_PROGRESS` tag changes)

## How It Handles Dual Log Streams

This parser explicitly uses **both** GameState and PowerTaskList streams for different purposes:

```typescript
// In MatchLogParser
private readonly prefix = '[Power] GameState.DebugPrintPower() -' as const;
private readonly reader = new BlockReader(this.prefix);

private readonly powerPrefix = '[Power] PowerTaskList.DebugPrintPower() -' as const;
private readonly powerReader = new BlockReader(this.powerPrefix);
```

**GameState blocks** are used for the match log (card plays, attacks, triggers) because they arrive first and have better structural integrity for block nesting.

**PowerTaskList blocks** are used for entity name resolution:
```typescript
// Read the PowerTaskList block variant.
// These are bad for identifying card flow, but great for resolving card names.
const powerBlock = this.powerReader.readLine(line, gameState);
if (powerBlock) {
    const context = new BlockContext(gameState, powerBlock);
    for (const entity of context.getAllEntities()) {
        gameState.resolveEntity(entity);
    }
}
```

Other parsers (GameOver, Turn, TagChange, GameTagChange) use **PowerTaskList** exclusively. Zone changes use the `[Zone]` log. Game start and player join use **GameState**.

## BG-Specific Handling

**None.** This parser is designed for standard constructed/arena Hearthstone. There is no:
- BACON_ tag handling
- Multi-player (>2) support
- Shop phase / combat phase distinction
- Tavern tier tracking
- Bob / dummy player handling

The two-player assumption is hardcoded:
- `gameOverCount === 2` signals game end
- Opponent is always `getPlayerById(3 - player.id)`
- Only two positions exist: `'top'` and `'bottom'`

## Key Design Patterns Worth Noting

1. **Entity resolution is deferred**: Cards often appear as `UNKNOWN ENTITY [cardType=INVALID]` initially. The `GameState.resolveEntity()` method retroactively fills in card names across all match log entries.

2. **Block context pre-processing**: The `BlockContext` class flattens subspells, pre-indexes entities, and detects health changes before the match log processes entries.

3. **Zone log for FRIENDLY/OPPOSING**: Only the `[Zone]` log provides the local player distinction. This is the most reliable method and must be captured early (during mulligan).

4. **First-match-wins parsing**: Line parsers are ordered and the first match stops processing. `MatchLogParser` runs first because its `BlockReader` needs to consume all lines between BLOCK_START and BLOCK_END.
