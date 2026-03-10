# Parser — Walkthrough

This document walks through how battlestream converts raw Hearthstone `Power.log`
lines into structured game state, from file bytes on disk all the way through to
the in-memory `BGGameState`.

---

## Pipeline overview

```
Power.log (disk)
     │
     ▼
  Watcher          (internal/watcher)
  tail + fsnotify → chan Line
     │
     ▼
  Parser           (internal/parser)
  regex matching  → chan GameEvent
     │
     ▼
  Processor        (internal/gamestate)
  state machine   → Machine (BGGameState)
     │
     ▼
  Store / API      (internal/store, internal/api/*)
```

---

## Stage 1 — Watcher: raw line delivery

`watcher.New()` opens `Power.log` with `nxadm/tail` (which handles log rotation
and truncation) and emits each line on a `chan Line`.

`fsnotify` watches the session directory so that when Hearthstone starts a new
session and creates a new `Power.log`, the watcher can pick it up automatically.

The watcher does **no parsing** — it just delivers raw text lines.

---

## Stage 2 — Parser: regex matching

`parser.Parser` receives one raw line at a time via `Feed(line string)`.
All parsing is done with compiled regexps — there is no external dependency.

### 2.1 Source filter

The very first check is:

```go
reGameStateSource = regexp.MustCompile(`GameState\.DebugPrint(?:Power|Game)\(\)`)
```

Lines that do not match are **silently discarded**. This eliminates the
`PowerTaskList.DebugPrint*` family of lines, which echo the same events a second
time and would cause double-processing.

Only two sources survive:
- `GameState.DebugPrintPower()` — entity and tag events
- `GameState.DebugPrintGame()` — player name and ID mappings

### 2.2 Timestamp extraction

After the source check, `extractTimestamp()` strips the wall-clock prefix
(`D HH:MM:SS.nnnnnnn`) and returns a `time.Time`. If parsing fails, `time.Now()`
is used. The stripped content is then used for all subsequent matching.

### 2.3 Block stack — BLOCK_START / BLOCK_END

Hearthstone wraps groups of related events in `BLOCK_START` / `BLOCK_END` pairs.
The parser maintains a `blockStack []blockContext` that tracks the **source
entity** (entity ID + card ID) of every open block.

```
BLOCK_START BlockType=PLAY Entity=[entityName=Nomi ... cardId=BG_ShopBuff ...] ...
    TAG_CHANGE Entity=Murloc Tidehunter ...   ← attributed to Nomi block
BLOCK_END
```

When a `TAG_CHANGE` or `FULL_ENTITY` event is emitted from within a block, the
top of the stack is attached as `BlockSource` / `BlockCardID` on the event.
This allows the processor to know *what card caused* a tag change.

`BLOCK_START` and `BLOCK_END` lines are consumed entirely by the parser and do
**not** generate `GameEvent`s.

### 2.4 FULL_ENTITY / SHOW_ENTITY — multi-line block mode

A `FULL_ENTITY` or `SHOW_ENTITY` header spans multiple lines:

```
GameState.DebugPrintPower() - FULL_ENTITY - Creating ID=75 CardID=TB_BaconShop_HERO_49
GameState.DebugPrintPower() -     tag=HEALTH value=40
GameState.DebugPrintPower() -     tag=CARDTYPE value=HERO
GameState.DebugPrintPower() -     tag=ZONE value=DECK
```

The parser enters **block mode** (`inBlock = true`) when it sees the header.
While in block mode, lines matching `reBlockTag` (4+ leading spaces after ` - `)
are accumulated into `pending.Tags`. When a non-continuation line is encountered,
`flushPending()` emits the complete `EventEntityUpdate` with all tags bundled.

`Flush()` must be called after the last line to emit any trailing block.

### 2.5 Event dispatch (switch/case)

After block-mode handling, the stripped line is matched against event patterns in
priority order:

| Pattern | Event emitted |
|---------|---------------|
| `CREATE_GAME` | `EventGameStart` |
| `Player EntityID=N PlayerID=N GameAccountId=[hi=N lo=N]` | `EventPlayerDef` |
| `PlayerID=N, PlayerName=...` | `EventPlayerName` |
| `TAG_CHANGE Entity=GameEntity tag=STATE value=COMPLETE` | `EventGameEnd` |
| `FULL_ENTITY` / `SHOW_ENTITY` header | enters block mode → `EventEntityUpdate` |
| `TAG_CHANGE Entity=GameEntity tag=TURN value=N` | `EventTurnStart` |
| `TAG_CHANGE Entity=... tag=... value=...` | `EventTagChange` |

Note that `reGameComplete` is checked *before* the generic `reTurnStart` check,
and `reTurnStart` is matched only on `Entity=GameEntity` to avoid collisions with
the player-entity `TURN` tag (which is handled in the processor via `EventTagChange`).

### 2.6 Entity ID resolution

Entity references in the log come in several forms:

| Form | Example | Strategy |
|------|---------|---------|
| `ID=N` on FULL_ENTITY | `ID=75` | `m[1]` capture group |
| Bracketed notation | `[entityName=Murloc id=42 ...]` | `reEntityID` extracts `id=N` |
| Bare integer | `Entity=10181` | `strconv.Atoi` fallback in `extractEntityID` |
| Player name string | `Entity=Moch#1358` | no numeric ID, EntityName set instead |

`extractPlayerField` similarly reads the `player=N` field from bracketed notation
to populate `PlayerID` on the event.

### 2.7 GameEvent struct

```go
type GameEvent struct {
    Type        EventType
    Timestamp   time.Time
    EntityID    int               // numeric entity ID (0 if unknown)
    PlayerID    int               // CONTROLLER / player= field
    Tags        map[string]string // all TAG → VALUE pairs
    EntityName  string            // raw entity description or name
    CardID      string            // CardID from FULL_ENTITY or block notation
    BlockSource int               // entity ID from enclosing BLOCK_START
    BlockCardID string            // card ID from enclosing BLOCK_START
}
```

All events flow through a `chan GameEvent` that the processor reads.

---

## Stage 3 — Processor: state machine

`gamestate.Processor` reads `GameEvent`s and mutates a `Machine`. It is the
semantic layer — the parser only does text recognition; the processor understands
*game logic*.

### 3.1 Local player identification

The CREATE_GAME block contains `Player` lines for every player in the lobby.
The local player is identified by having a **non-zero `hi` value** in its
`GameAccountId`. Dummy/placeholder players use `hi=0`.

Once identified, `localPlayerID` (the `CONTROLLER` value, e.g. `7`) and
`localHeroID` are stored. All subsequent filtering is done against these.

### 3.2 Entity registry (`entityProps`)

The processor maintains `entityProps map[int]*entityInfo` — a registry of
everything known about every entity seen so far:

- `CardID`, `Name`, `CardType`
- `Attack`, `Health`
- `Zone`
- `CreatorID`, `AttachedTo`
- `ScriptData1`, `ScriptData2`

This is populated from both `EventEntityUpdate` (FULL_ENTITY blocks) and
`EventTagChange` events. The registry enables **zone transition handling**: when
a TAG_CHANGE says `ZONE=PLAY`, the processor can look up the entity's card type
and stats from the registry and add it to the board.

**Name upgrade:** if a bare numeric string was stored as a placeholder name (e.g.
`"10181"` from `Entity=10181`), it is replaced when a real name becomes available.
The `isBareNumber()` helper detects this condition.

### 3.3 Phase machine

```
IDLE → LOBBY (EventGameStart / CREATE_GAME)
LOBBY → RECRUIT (EventTurnStart with GameEntity TURN odd, or player TURN tag)
RECRUIT → COMBAT (EventTurnStart with GameEntity TURN even → board snapshot taken)
COMBAT → RECRUIT (next GameEntity TURN odd)
any → GAME_OVER (EventGameEnd)
```

The GameEntity `TURN` tag is doubled: odd values = recruit phase, even values =
combat phase. The player-entity `TURN` tag gives the actual BG turn number shown
in the UI.

On every recruit→combat transition, the current board is **snapshotted**
(`boardSnapshot`). When the game ends, the snapshot is restored — because combat
simulation copies have base (un-buffed) stats, while the recruit board had the
correct fully-buffed values.

### 3.4 Board management

Minions are added to `Board []MinionState` via `UpsertMinion` when:
- A `FULL_ENTITY`/`SHOW_ENTITY` block describes a MINION in ZONE=PLAY
- A `TAG_CHANGE ZONE=PLAY` fires for a known entity that is a local minion

Minions are removed via `RemoveMinion` when ZONE transitions to:
- `GRAVEYARD`
- `REMOVEDFROMGAME`
- `SETASIDE`

Removal is suppressed during `PhaseGameOver` to preserve the final board display.

`UpdateMinionStat` handles live stat changes (ATK/HEALTH TAG_CHANGE events),
updating the board in place.

### 3.5 Board-wide buff detection (Modifications)

During recruit phase, ATK/HEALTH changes on board minions are buffered as
`pendingStatChanges`. At turn boundaries (`flushPendingStatChanges`), the buffer
is grouped by `(turn, stat, delta)`. If **2 or more** minions share the same
turn/stat/delta, it is recorded as a `StatMod` with `Target = "Board (Nx)"`.

This heuristic catches board-wide effects (e.g. a spell buffing all minions +2/+2)
without false positives from individual combat damage noise. Combat phase changes
are excluded entirely.

### 3.6 Buff source tracking

The processor implements all 13 HDT BG counter types:

**Player tag counters** — TAG_CHANGE on the local player entity:
- `BACON_BLOODGEMBUFFATKVALUE` / `HEALTHVALUE` → `CatBloodgem`
- `BACON_ELEMENTAL_BUFFATKVALUE` / `HEALTHVALUE` → `CatElemental`
- `TAVERN_SPELL_ATTACK_INCREASE` / `HEALTH_INCREASE` → `CatTavernSpell`
- `BACON_FREE_REFRESH_COUNT` → `CatFreeRefresh` (AbilityCounter)
- `BACON_PLAYER_EXTRA_GOLD_NEXT_TURN` → `CatGoldNextTurn` (AbilityCounter)
- Tag `3809` (SpellsPlayedForNagasCounter — NOT the Spellcraft keyword) → `CatNagaSpells` (AbilityCounter, stacks formula: `1 + raw/4`)

**Dnt enchantment counters** (SD1/SD2 on player-level enchantment entities):
- `BG_ShopBuff_Elemental` → `CatNomi` (differential accumulation)
- `BG30_MagicItem_544pe` → `CatNomi` Nomi Sticker (SD1 applies to both ATK+HP)
- `BG34_855pe` → `CatNomiAll` Timewarped Nomi (differential)
- `BG31_808pe` → `CatBeetle` (absolute, base 1/1)
- `BG34_854pe` → `CatRightmost` (absolute)
- `BG34_402pe` → `CatWhelp` (absolute)
- `BG25_011pe` → `CatUndead` (SD1 only, ATK)
- `BG34_170e` → `CatVolumizer` (absolute)
- `BG34_689e2` → `CatBloodgemBarrage` (absolute)

**Zone-tracked enchantments** (ZONE transitions):
- `BG28_884e` (Overconfidence) entering/leaving PLAY → `CatGoldNextTurn` (+3 gold each)

**Differential vs Absolute:**
- *Differential*: Dnt tracks a running cumulative total; the processor stores the
  previous SD value and accumulates `delta = new - prev` into a running counter.
  Multiple Dnt entities (one per minion buffed by Nomi) are each tracked separately
  via `shopBuffPrev map[int][2]int`.
- *Absolute*: Dnt contains the current effective value directly. The processor
  writes it to the machine each update.

### 3.7 Enchantment tracking

When a `FULL_ENTITY` with `CARDTYPE=ENCHANTMENT` is seen, `handleEnchantmentEntity`
is called. It:
1. Looks up the creator entity in the registry to get `sourceCardID`.
2. Classifies the enchantment via `ClassifyEnchantment()` (CardID lookup table) or
   `ClassifyCreator()` (creator lookup table).
3. Reads buff values from `TAG_SCRIPT_DATA_NUM_1/2`.
4. Calls `machine.AddEnchantment()` which attaches it to the target board minion.

### 3.8 Duos partner tracking

In Battlegrounds Duos, the local player's `BACON_DUO_TEAMMATE_PLAYER_ID` tag in the
Player definition identifies the partner's PlayerID. The partner's hero entity is
detected via its `PLAYER_ID` tag in the FULL_ENTITY block — **not** by CONTROLLER,
which is set to a dummy bot ID (e.g. 9) shared with all non-local entities.

**Available from Power.log (partner hero entity):**
- Hero name + CardID (from FULL_ENTITY block)
- ARMOR, DAMAGE (live TAG_CHANGE updates)
- PLAYER_TECH_LEVEL (tavern tier, live updates)
- PLAYER_TRIPLES (live updates)
- PLAYER_LEADERBOARD_PLACE

**Not available from Power.log (requires memory reading):**
- Partner board minions — all non-local minions share `CONTROLLER=<botID>`,
  making them indistinguishable from opponent or shop entities
- Partner buff sources / ability counters — same CONTROLLER limitation
- Partner gold (RESOURCES fires only on the local player entity)
- Partner BattleTag (no PlayerName event for the partner's PlayerID)

**Duos health model:** Health and armor are shared (team pool). The local hero
entity's HEALTH/DAMAGE/ARMOR tags reflect the team total. The partner hero entity
also carries DAMAGE/ARMOR but these are the same shared values.

### 3.9 Reconnect handling

When the Hearthstone client reconnects mid-game, it emits a new CREATE_GAME with
all entity state re-declared in FULL_ENTITY blocks. The processor captures initial
state from these blocks:

- **Player entity tags** (EventPlayerDef): TURN, RESOURCES, RESOURCES_USED
- **Hero entity tags** (EventEntityUpdate): HEALTH, DAMAGE, ARMOR,
  PLAYER_TECH_LEVEL, PLAYER_TRIPLES
- **Partner hero tags** (EventEntityUpdate): same set, via PLAYER_ID matching

This ensures that reconnected games show correct turn number, gold, health, and
stats rather than starting from zero.

---

## Key edge cases handled

| Situation | Handling |
|-----------|---------|
| Combat copies (SETASIDE entities with base stats) | Board snapshotted before combat; restored on GameEnd |
| Bare numeric entity names (`Entity=10181`) | `extractEntityID` falls back to `strconv.Atoi`; name upgraded when real name arrives |
| Player name vs entity name in TAG_CHANGE | Both `EntityName == localPlayerName` and `PlayerID == localPlayerID` checked |
| PowerTaskList duplicate events | Filtered by `reGameStateSource` at the top of `Feed()` |
| Trailing FULL_ENTITY block at EOF | `Parser.Flush()` must be called by the consumer |
| Hero entities appearing as board entities | `heroEntities` registry; skipped from minion board |
| Game end during combat (minions in SETASIDE) | Removal blocked in `PhaseGameOver`; snapshot restored |
| Multiple Nomi enchantments (one per minion) | Each entity tracked independently in `shopBuffPrev` |
| Duos partner hero has CONTROLLER=botID | Identified by PLAYER_ID tag, not CONTROLLER |
| Reconnect mid-game (second CREATE_GAME) | Initial tags from Player def + hero FULL_ENTITY restore state |
