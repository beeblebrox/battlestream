# Parser Specification

Formal specification for the battlestream log parser and event protocol.
Covers input format, regex contracts, event semantics, and processor invariants.

---

## 1. Input format

### 1.1 Log file

Source: Hearthstone `Power.log`, located via `internal/discovery`.

Encoding: UTF-8, LF or CRLF line endings, variable line length.

### 1.2 Line structure

Every relevant line has the form:

```
<level> <HH:MM:SS.nnnnnnn> <source>() - <content>
```

| Field | Values |
|-------|--------|
| `level` | `D` (debug), `W` (warning), `I` (info), `E` (error) |
| timestamp | `HH:MM:SS` with up to 7 decimal digits |
| `source` | see §1.3 |
| `content` | event-specific text |

### 1.3 Accepted sources

Only lines whose source matches `GameState\.DebugPrint(?:Power|Game)\(\)` are
processed. All other sources (notably `PowerTaskList.DebugPrintPower()`) are
discarded to prevent duplicate event processing.

---

## 2. Regex patterns (authoritative)

All patterns match against the **stripped** line (timestamp prefix removed).

| ID | Pattern | Purpose |
|----|---------|---------|
| `reGameStateSource` | `GameState\.DebugPrint(?:Power\|Game)\(\)` | Source filter |
| `reTimestamp` | `^[DWIE]\s+(\d{2}:\d{2}:\d{2}\.\d+)\s+` | Timestamp extraction |
| `reCreateGame` | `CREATE_GAME` | Game start sentinel |
| `rePlayerDef` | `Player\s+EntityID=(\d+)\s+PlayerID=(\d+)\s+GameAccountId=\[hi=(\d+)\s+lo=(\d+)\]` | Player entity definition |
| `rePlayerNameLine` | `PlayerID=(\d+),\s+PlayerName=(.+)` | Player name mapping |
| `reGameComplete` | `TAG_CHANGE\s+Entity=GameEntity\s+tag=STATE\s+value=COMPLETE` | Game end |
| `reFullEntity` | `(?:FULL_ENTITY\|SHOW_ENTITY)\s+-\s+(?:Creating\|Updating)\s+(?:ID=(\d+)\s+)?(?:Entity=(.+?)\s+)?CardID=(\S*)` | Entity block header |
| `reBlockTag` | `-\s{4,}tag=(\S+)\s+value=(\S*)` | Indented tag in entity block |
| `reTurnStart` | `TAG_CHANGE\s+Entity=GameEntity\s+tag=TURN\s+value=(\d+)` | GameEntity turn |
| `reTagChange` | `TAG_CHANGE\s+Entity=(.+?)\s+tag=(\S+)\s+value=(\S+)` | Generic tag change |
| `reBlockStart` | `BLOCK_START\s+BlockType=\w+\s+Entity=(.+?)\s+EffectCardId=` | Block context open |
| `reBlockEnd` | `BLOCK_END` | Block context close |
| `reEntityID` | `\bid=(\d+)\b` | Entity ID from bracketed notation |
| `rePlayerField` | `\bplayer=(\d+)\b` | Player ID from bracketed notation |
| `reCardIDField` | `\bcardId=(\S+?)[\s\]]` | Card ID from bracketed notation |

---

## 3. Event types

### 3.1 EventGameStart

**Trigger:** line matching `reCreateGame`

**Semantics:** A new game session begins. The processor resets all per-game state.

**Fields populated:**
- `Type = "GAME_START"`
- `Timestamp`
- `Tags = {}` (empty)

---

### 3.2 EventPlayerDef

**Trigger:** line matching `rePlayerDef` (within CREATE_GAME block)

**Semantics:** Declares a player slot. The local player is identified by `hi != "0"`.

**Fields populated:**
- `Type = "PLAYER_DEF"`
- `EntityID` — player entity's numeric ID
- `PlayerID` — player slot number (1-indexed)
- `Tags["hi"]`, `Tags["lo"]` — GameAccountId components
- `Tags["PLAYER_ID"]` — string copy of PlayerID

---

### 3.3 EventPlayerName

**Trigger:** line matching `rePlayerNameLine` (from `DebugPrintGame()`)

**Semantics:** Maps a PlayerID to a display name (BattleTag).

**Fields populated:**
- `Type = "PLAYER_NAME"`
- `PlayerID`
- `EntityName` — trimmed player name string

---

### 3.4 EventGameEnd

**Trigger:** line matching `reGameComplete`

**Semantics:** Game is complete. Processor flushes pending stat changes, applies
final placement, restores board snapshot.

**Fields populated:**
- `Type = "GAME_END"`
- `Timestamp`
- `Tags = {}` (empty; placement comes from earlier TAG_CHANGE events)

---

### 3.5 EventTurnStart

**Trigger:** `TAG_CHANGE Entity=GameEntity tag=TURN value=N`

**Semantics:** Internal game clock tick. The `TURN` value from GameEntity is
doubled relative to the player's view (odd = recruit, even = combat).

**Fields populated:**
- `Type = "TURN_START"`
- `Tags["TURN"]` — string integer value

---

### 3.6 EventEntityUpdate

**Trigger:** `FULL_ENTITY`/`SHOW_ENTITY` block (header + subsequent indented tag lines)

**Semantics:** Full state declaration for an entity. Tags are accumulated until the
next non-continuation line triggers `flushPending()`.

**Fields populated:**
- `Type = "ENTITY_UPDATE"`
- `EntityID` — from `ID=N` or `id=N` in Entity field
- `EntityName` — raw entity description
- `CardID` — from `CardID=` field
- `Tags` — all `tag=X value=Y` pairs accumulated from continuation lines
- `PlayerID` — resolved from `CONTROLLER` tag if present
- `BlockSource`, `BlockCardID` — from enclosing BLOCK_START if any

---

### 3.7 EventTagChange

**Trigger:** generic `TAG_CHANGE` line not already matched by a more specific pattern

**Semantics:** A single tag value change on an entity.

**Fields populated:**
- `Type = "TAG_CHANGE"`
- `EntityID` — 0 if entity is referenced by name only
- `PlayerID` — from `player=N` in bracketed notation
- `EntityName` — raw entity string (may be player name, bracketed notation, or bare integer)
- `Tags` — single entry `{tag: value}`
- `BlockSource`, `BlockCardID` — from enclosing BLOCK_START if any

**Note:** `EventPlayerUpdate` and `EventZoneChange` are defined in `events.go`
but are currently not emitted by the parser. Zone changes and player updates are
handled via `EventTagChange` in the processor.

---

## 4. Entity ID resolution rules

Applied in order by `extractEntityID(s string) int`:

1. If `s` contains `id=N` (bracketed notation) → use `N`
2. If `s` is a bare decimal integer string → use `strconv.Atoi(s)`
3. Otherwise → return `0` (unknown / name-only entity)

Player ID resolution (`extractPlayerField`) reads `player=N` from bracketed
notation; returns `0` if absent.

---

## 5. Block mode protocol

**Entry:** `FULL_ENTITY`/`SHOW_ENTITY` header sets `p.inBlock = true` and
initialises `p.pending`.

**Accumulation:** While `p.inBlock`, any line matching `reBlockTag` appends to
`p.pending.Tags` and returns immediately.

**Exit:** Any non-continuation line triggers `p.flushPending()`:
1. Resolves `PlayerID` from `Tags["CONTROLLER"]` if `PlayerID == 0`
2. Emits `p.pending` as `EventEntityUpdate`
3. Clears `p.inBlock` and `p.pending`

**Forced flush:** `Parser.Flush()` must be called by the consumer after delivering
the last line to handle EOF-terminated blocks.

---

## 6. Block stack protocol

**Push:** On `BLOCK_START`, the source entity's `{entityID, cardID}` is pushed onto
`p.blockStack`.

**Pop:** On `BLOCK_END`, the top entry is popped. Unmatched `BLOCK_END` lines are
silently ignored (stack already empty).

**Attribution:** When emitting any event while `len(blockStack) > 0`, the top
entry is copied into `e.BlockSource` / `e.BlockCardID`.

---

## 7. Processor invariants

| Invariant | Notes |
|-----------|-------|
| Local player ID is set once per game from CREATE_GAME EventPlayerDef(hi!=0) | Never changes mid-game |
| Entity controller registry is append-only within a game | CONTROLLER TAG_CHANGE can update but not unset |
| Board only contains MINION entities in ZONE=PLAY owned by local player | Enforced on every add path |
| Board is cleared at GameStart | Fresh state per game |
| Board snapshot is taken at every recruit→combat transition | Overwrites previous snapshot |
| Board snapshot is restored at GameEnd | Ensures final board has recruit stats |
| Minion removal is blocked in PhaseGameOver | Board preserved for display |
| Pending stat changes are flushed at turn boundaries and GameEnd | No cross-turn leakage |
| Board-wide buff threshold is >= 2 minions with same (turn, stat, delta) | Prevents single-minion false positives |
| Buff source values are upserted (not appended) per category | One entry per category |
| Ability counters are upserted per category | One entry per category |
| Enchantments are upserted per entity ID | Script data updates replace previous |
| Duos partner hero identified by PLAYER_ID tag, not CONTROLLER | CONTROLLER is dummy bot ID shared with all non-local entities |
| Partner board/buff sources are not tracked | Power.log does not distinguish partner entities from opponents (requires memory reading) |
| Reconnect restores TURN/RESOURCES from Player def tags | Prevents Turn=0 / Gold=0 after mid-game reconnect |
| Reconnect restores DAMAGE/ARMOR from hero FULL_ENTITY tags | Prevents Health showing full after reconnect |

---

## 8. Phase transition table

| Current phase | Trigger | Next phase |
|--------------|---------|------------|
| `IDLE` | `EventGameStart` | `LOBBY` |
| `LOBBY` | GameEntity TURN odd | `RECRUIT` |
| `LOBBY` | player TURN tag | `RECRUIT` |
| `RECRUIT` | GameEntity TURN even | `COMBAT` (snapshot taken) |
| `COMBAT` | GameEntity TURN odd | `RECRUIT` |
| any except `GAME_OVER` | `EventGameEnd` | `GAME_OVER` |

---

## 9. Tag semantics reference

| Tag | Entity | Processor action |
|-----|--------|-----------------|
| `TURN` on `GameEntity` | n/a | `EventTurnStart` → `SetGameEntityTurn` |
| `TURN` on local player entity | local player | `SetTurn` (display turn), flush pending stats |
| `STATE=COMPLETE` on `GameEntity` | n/a | `EventGameEnd` → `GameEnd` |
| `HEALTH` on hero | local hero | `UpdatePlayerTag` |
| `HEALTH` on minion | local player minion | `updateMinionStat` → board update + pending buffer |
| `ATK` on minion | local player minion | `updateMinionStat` |
| `ARMOR` on hero | local hero | `UpdatePlayerTag` |
| `PLAYER_TECH_LEVEL` / `TAVERN_TIER` | local player | `SetTavernTier` |
| `PLAYER_TECH_LEVEL` / `TAVERN_TIER` | partner hero/player | `UpdatePartnerTag` (Duos only) |
| `PLAYER_TRIPLES` | local hero or player | `UpdatePlayerTag` |
| `PLAYER_TRIPLES` | partner hero or player | `UpdatePartnerTag` (Duos only) |
| `HEALTH` / `DAMAGE` / `ARMOR` | partner hero | `UpdatePartnerTag` (Duos only) |
| `PLAYER_LEADERBOARD_PLACE` | local player | buffered in `pendingPlacement`, applied at GameEnd |
| `ZONE=PLAY` | any entity | `tryAddMinionFromRegistry` |
| `ZONE=GRAVEYARD/REMOVEDFROMGAME/SETASIDE` | any entity | `RemoveMinion` + `RemoveEnchantmentsForEntity` |
| `HERO_ENTITY` | local player | updates `localHeroID` |
| `CONTROLLER` | any entity | updates `entityController` registry |
| `BACON_BLOODGEMBUFFATKVALUE` | local player | `updateBuffSourceFromPlayerTag` → `CatBloodgem` |
| `BACON_BLOODGEMBUFFHEALTHVALUE` | local player | `updateBuffSourceFromPlayerTag` → `CatBloodgem` |
| `BACON_ELEMENTAL_BUFFATKVALUE` | local player | `updateBuffSourceFromPlayerTag` → `CatElemental` |
| `BACON_ELEMENTAL_BUFFHEALTHVALUE` | local player | `updateBuffSourceFromPlayerTag` → `CatElemental` |
| `TAVERN_SPELL_ATTACK_INCREASE` | local player | `updateBuffSourceFromPlayerTag` → `CatTavernSpell` |
| `TAVERN_SPELL_HEALTH_INCREASE` | local player | `updateBuffSourceFromPlayerTag` → `CatTavernSpell` |
| `BACON_FREE_REFRESH_COUNT` | local player | `SetAbilityCounter(CatFreeRefresh)` |
| `BACON_PLAYER_EXTRA_GOLD_NEXT_TURN` | local player | `goldNextTurnSure` + `updateGoldNextTurnCounter` |
| `TAG_SCRIPT_DATA_NUM_1/2` | enchantment entity | `handleDntTagChange` → Dnt counter dispatch |
| `3809` (SpellsPlayedForNagasCounter) | local player | `SetAbilityCounter(CatNagaSpells, stacks formula)` |

---

## 10. Dnt enchantment dispatch table

| CardID | Category | Accumulation | SD1 | SD2 |
|--------|----------|-------------|-----|-----|
| `BG_ShopBuff_Elemental` | `NOMI` | Differential | ATK | HP |
| `BG30_MagicItem_544pe` | `NOMI` | Differential | ATK+HP | ignored |
| `BG34_855pe` | `NOMI_ALL` | Differential | ATK | HP |
| `BG31_808pe` | `BEETLE` | Absolute (base 1/1) | ATK | HP |
| `BG34_854pe` | `RIGHTMOST` | Absolute | ATK | HP |
| `BG34_402pe` | `WHELP` | Absolute | ATK | HP |
| `BG25_011pe` | `UNDEAD` | Absolute | ATK only | — |
| `BG34_170e` | `VOLUMIZER` | Absolute | ATK | HP |
| `BG34_689e2` | `BLOODGEM_BARRAGE` | Absolute | ATK | HP |

Differential: `counter += SD_new - SD_prev` per entity.
Absolute: `value = base + SD`.

`CatLightfang` and `CatConsumed` are per-minion only; they have no player-level Dnt counter and no `handleDntTagChange` case. HDT has no `LightfangCounter.cs` or `ConsumedCounter.cs`. Their enchantments are tracked via `handleEnchantmentEntity` instead.

---

## 11. Value computation rules

| Category | Raw tag | Formula | Notes |
|----------|---------|---------|-------|
| `BLOODGEM` | `BACON_BLOODGEMBUFFATKVALUE` | `raw + 1` | Tag value 0 → effective +1 |
| `BLOODGEM` | `BACON_BLOODGEMBUFFHEALTHVALUE` | `raw + 1` | Same offset |
| `ELEMENTAL` | `BACON_ELEMENTAL_BUFF*` | `max(0, raw)` | Clamp at zero |
| `TAVERN_SPELL` | `TAVERN_SPELL_*` | `raw` | Direct |
| `NAGA_SPELLS` | tag `3809` (SpellsPlayedForNagasCounter — NOT Spellcraft keyword) | stacks=`1 + raw/4`, progress=`raw%4` | Display: `"Tier N · M/4"` |
| `GOLD_NEXT_TURN` | player tag + Overconfidence | `sure + overconfidenceCount * 3` | Display: `"N (N+bonus)"` if bonus > 0 |
