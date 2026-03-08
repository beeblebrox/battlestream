# Hearthstone Battlegrounds Power.log Format Reference

## Overview

The Power.log file records all game state changes as they happen. In Battlegrounds,
there are two "players" in the log: the local player and a dummy/opponent player.
The log contains **two parallel streams** of the same data:

- `GameState.DebugPrintPower()` — logged when the client receives data (real-time)
- `PowerTaskList.DebugPrintPower()` — logged when the client executes the action

**Parse only `GameState.DebugPrintPower()` lines** to avoid processing duplicates.

## Log Line Format

```
D HH:MM:SS.NNNNNNN <Source>() - <content>
```

- `D` = Debug level (also `W`, `I`, `E` for Warn/Info/Error)
- Timestamp: `HH:MM:SS.NNNNNNN` (nanosecond precision)
- Source: `GameState.DebugPrintPower`, `GameState.DebugPrintGame`, etc.
- Content is indented with spaces to show nesting depth

## Log Directory Structure

As of 2024+, Hearthstone creates timestamped session subdirectories:

```
Hearthstone/Logs/
  Hearthstone_2026_03_04_21_40_12/
    Power.log
    Zone.log
    LoadingScreen.log
    ...
```

NOT directly in `Logs/Power.log` anymore.

## Game Initialization Sequence

### 1. CREATE_GAME

```
GameState.DebugPrintPower() - CREATE_GAME
GameState.DebugPrintPower() -     GameEntity EntityID=19
GameState.DebugPrintPower() -         tag=CARDTYPE value=GAME
GameState.DebugPrintPower() -         tag=ZONE value=PLAY
GameState.DebugPrintPower() -         tag=ENTITY_ID value=19
...
```

### 2. Player Entities

Two players are created. The LOCAL player has a real GameAccountId:

```
Player EntityID=20 PlayerID=7 GameAccountId=[hi=144115193835963207 lo=30722021]
    tag=CONTROLLER value=7
    tag=CARDTYPE value=PLAYER
    tag=PLAYER_ID value=7
    tag=HERO_ENTITY value=37
    tag=PLAYER_TECH_LEVEL value=1
    ...
```

The DUMMY/OPPONENT player has `hi=0 lo=0` and `BACON_DUMMY_PLAYER=1`:

```
Player EntityID=21 PlayerID=15 GameAccountId=[hi=0 lo=0]
    tag=CONTROLLER value=15
    tag=CARDTYPE value=PLAYER
    tag=BACON_DUMMY_PLAYER value=1
    ...
```

### 3. Player Names

After CREATE_GAME, player names are printed:

```
GameState.DebugPrintGame() - PlayerID=7, PlayerName=Moch#1358
GameState.DebugPrintGame() - PlayerID=15, PlayerName=DirePants
```

## Identifying the Local Player

Multiple methods, in order of reliability:

1. **GameAccountId**: Local player has `hi≠0, lo≠0`. Dummy has `hi=0, lo=0`.
2. **BACON_DUMMY_PLAYER tag**: The non-local player has `BACON_DUMMY_PLAYER=1`.
3. **GameAccountId hi value**: The `hi` field contains the account region+ID.

The local player's **CONTROLLER value** (e.g., `7`) is the key for filtering all
subsequent entities. Any entity with `player=7` or `CONTROLLER=7` belongs to the
local player.

## Entity Formats in TAG_CHANGE

Entities appear in two formats:

### Simple (by name or numeric ID)

```
TAG_CHANGE Entity=Moch#1358 tag=TURN value=3
TAG_CHANGE Entity=GameEntity tag=TURN value=5
TAG_CHANGE Entity=4510 tag=HEALTH value=5
```

### Bracketed (with full metadata)

```
TAG_CHANGE Entity=[entityName=Millhouse Manastorm id=75 zone=PLAY zonePos=0 cardId=TB_BaconShop_HERO_49 player=7] tag=HEALTH value=30
```

Fields in brackets:
- `entityName` — display name
- `id` — numeric entity ID
- `zone` — current zone (PLAY, HAND, SETASIDE, GRAVEYARD, etc.)
- `zonePos` — position in zone (1-7 for board minions, 0 for heroes)
- `cardId` — card database ID
- `player` — **CONTROLLER player ID** (critical for ownership)

## FULL_ENTITY / SHOW_ENTITY Blocks

```
FULL_ENTITY - Creating ID=75 CardID=TB_BaconShop_HERO_49
    tag=CONTROLLER value=7
    tag=CARDTYPE value=HERO
    tag=HEALTH value=40
    tag=ZONE value=PLAY
    tag=ENTITY_ID value=75
    tag=ARMOR value=10
    ...
```

- `FULL_ENTITY - Creating` = new entity
- `SHOW_ENTITY - Updating` = revealed/updated entity
- Tags are indented under the entity header
- `CONTROLLER` tag = which player owns this entity

## Turn Numbering in Battlegrounds

**Critical**: BG has TWO turn counters that differ:

### GameEntity TURN (internal, doubled)

```
TAG_CHANGE Entity=GameEntity tag=TURN value=1   ← Recruit phase 1
TAG_CHANGE Entity=GameEntity tag=TURN value=2   ← Combat phase 1
TAG_CHANGE Entity=GameEntity tag=TURN value=3   ← Recruit phase 2
TAG_CHANGE Entity=GameEntity tag=TURN value=4   ← Combat phase 2
```

- Odd = local player's recruit phase
- Even = opponent/combat phase
- This is NOT what the player sees in-game

### Player TURN (what the user sees)

```
TAG_CHANGE Entity=Moch#1358 tag=TURN value=1    ← BG Turn 1
TAG_CHANGE Entity=Moch#1358 tag=TURN value=2    ← BG Turn 2
TAG_CHANGE Entity=Moch#1358 tag=TURN value=3    ← BG Turn 3
```

**Use the local player's TURN value** for the display turn number.

## Phase Detection

Use `STEP` tag on GameEntity:

```
TAG_CHANGE Entity=GameEntity tag=STEP value=MAIN_READY      ← Turn starts
TAG_CHANGE Entity=GameEntity tag=STEP value=MAIN_START       ← Main phase
TAG_CHANGE Entity=GameEntity tag=STEP value=MAIN_END         ← Turn ending
TAG_CHANGE Entity=GameEntity tag=STEP value=MAIN_CLEANUP     ← Cleanup
TAG_CHANGE Entity=GameEntity tag=STEP value=MAIN_NEXT        ← Transition
```

In BG, recruit vs combat is determined by whether it's the local player's turn
(odd GameEntity TURN = recruit) or the dummy player's turn (even = combat).

## Important BG-Specific Tags

| Tag | Entity Type | Description |
|-----|------------|-------------|
| `PLAYER_TECH_LEVEL` | Player | Tavern tier (1-6) |
| `HEALTH` | Hero | Current health |
| `ARMOR` | Hero | Current armor |
| `ATK` | Minion | Attack value |
| `ZONE` | Any | Current zone |
| `ZONE_POSITION` | Minion | Board position (1-7) |
| `CONTROLLER` | Any | Owning player ID |
| `CARDTYPE` | Any | GAME, PLAYER, HERO, MINION, ENCHANTMENT, etc. |
| `PLAYER_LEADERBOARD_PLACE` | Player | Final placement (1-8) |
| `BACON_DUMMY_PLAYER` | Player | 1 = this is the opponent/dummy player |
| `BACON_TRIPLE_CARD` | Player | Triple count |
| `GAME_RESULT` | Player | WIN, LOSS, TIE |
| `NUM_TURNS_IN_PLAY` | Various | Increments each turn cycle |
| `PLAYER_TRIPLES` | Hero | Triple count for a specific hero |

## Entity Types (CARDTYPE values)

- `GAME` — the game entity itself (EntityID typically low, e.g., 1 or 19)
- `PLAYER` — player entity (has CONTROLLER, HERO_ENTITY)
- `HERO` — hero card (has HEALTH, ARMOR, controlled by a player)
- `MINION` — board minion (has ATK, HEALTH, ZONE)
- `ENCHANTMENT` — buff/enchantment attached to another entity
- `HERO_POWER` — hero power card
- `SPELL` — spell card

## Board State Tracking

Minions on the board have:
- `zone=PLAY` — currently on the board
- `zonePos=1..7` — position on the board (left to right)
- `player=N` — which player controls them

When a minion dies: `TAG_CHANGE Entity=[...] tag=ZONE value=GRAVEYARD`
When a minion is sold/returned: `TAG_CHANGE Entity=[...] tag=ZONE value=REMOVEDFROMGAME`

## Hero Health/Armor

Hero entities (CARDTYPE=HERO) with `player=<localPlayerID>` hold the
local player's health and armor. The hero card ID changes when the player
selects a hero during mulligan:

```
TAG_CHANGE Entity=Moch#1358 tag=HERO_ENTITY value=75
```

Then health/armor updates come on that hero entity:
```
TAG_CHANGE Entity=[entityName=Millhouse Manastorm id=75 zone=PLAY ...player=7] tag=HEALTH value=30
TAG_CHANGE Entity=[entityName=Millhouse Manastorm id=75 zone=PLAY ...player=7] tag=ARMOR value=10
```

## Game End

```
TAG_CHANGE Entity=Moch#1358 tag=PLAYSTATE value=LOST
TAG_CHANGE Entity=Moch#1358 tag=PLAYER_LEADERBOARD_PLACE value=5
```

## Important Notes

1. **Filter by `GameState.DebugPrintPower()`** — skip `PowerTaskList` to avoid duplicates
2. **Track CONTROLLER/player field** — essential to separate local vs opponent
3. **Use player TURN, not GameEntity TURN** — GameEntity doubles the count
4. **Hero entity changes at mulligan** — track HERO_ENTITY tag on player
5. **Multiple games per session** — a new CREATE_GAME resets everything
6. **Bob (the bartender)** is player 15/dummy — has BACON_DUMMY_PLAYER=1, HEALTH=60
