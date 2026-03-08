# python-hslog Analysis

**Project**: `python-hslog` (Python) -- The canonical Python parser for Hearthstone Power.log files. Used by HSReplay.net and many other tools including `HSBG_LogParser`.

## Overview

This is a comprehensive, production-grade parser that converts raw Power.log text into a structured packet tree, then optionally exports to an entity tree. It handles all game modes including Battlegrounds and Mercenaries, spectator mode, multiple log format versions, and edge cases accumulated over years.

## Architecture

Three-layer design:

1. **`LogParser`** -- Reads lines, dispatches to handlers based on method prefix (`GameState.DebugPrintPower`, `GameState.SendOption`, etc.)
2. **Handlers** (`PowerHandler`, `OptionsHandler`, `ChoicesHandler`) -- Parse opcodes and create packet objects
3. **Exporters** (`EntityTreeExporter`, `FriendlyPlayerExporter`) -- Walk the packet tree to build entity state

Supporting infrastructure:
- **`PlayerManager`** (`player.py`) -- Resolves player names, entity IDs, and player IDs across multiple identification strategies
- **`tokens.py`** -- All regex patterns and string constants
- **`ParsingState`** -- Mutable state passed through all handlers

## All Regex Patterns (from `tokens.py`)

### Line-Level Patterns

**Timestamp** (outermost line wrapper):
```python
TIMESTAMP_RE = re.compile(r"^([DWE]) ([\d:.]+) (.+)$")
```
Captures: log level (D/W/E), timestamp (HH:MM:SS.mmm), rest of line.

**Power log line** (method dispatch):
```python
POWERLOG_LINE_RE = re.compile(r"([^(]+)\(\) - (.+)$")
```
Captures: method name (e.g. `GameState.DebugPrintPower`), data.

### Entity Format

**Entity reference** (the `_E` meta-pattern used everywhere):
```python
GAME_ENTITY = "GameEntity"
UNKNOWN_HUMAN_PLAYER = "UNKNOWN HUMAN PLAYER"
_E = r"(%s|%s|\[.+\]|\d+|.+)" % (GAME_ENTITY, UNKNOWN_HUMAN_PLAYER)
```
Matches: `GameEntity`, `UNKNOWN HUMAN PLAYER`, `[entityName=... id=N ...]`, a bare integer, or a player name.

**Entity ID extractor** (from bracketed entity strings):
```python
ENTITY_RE = re.compile(r"\[.*\s*id=(\d+)\s*.*\]")
```

### Game/Player Creation

**Game entity**:
```python
GAME_ENTITY_RE = re.compile(r"GameEntity EntityID=(\d+)")
```

**Player entity**:
```python
PLAYER_ENTITY_RE = re.compile(r"Player EntityID=(\d+) PlayerID=(\d+) GameAccountId=\[hi=(\d+) lo=(\d+)\]$")
```

**Game player meta** (from DebugPrintGame):
```python
GAME_PLAYER_META = re.compile(r"PlayerID=(\d+), PlayerName=(.*)")
```

### Power Opcodes

**CREATE_GAME**:
```python
CREATE_GAME_RE = re.compile(r"^CREATE_GAME$")
```

**BLOCK_START** (multiple versions for different client builds):
```python
ACTION_START_OLD_RE = re.compile(
    r"ACTION_START Entity=%s (?:SubType|BlockType)=(\w+) Index=(-1|\d+) Target=%s$" % (_E, _E)
)
ACTION_START_RE = re.compile(
    r"ACTION_START SubType=(\w+) Entity=%s EffectCardId=(.*) EffectIndex=(-1|\d+) Target=%s$" % (_E, _E)
)
BLOCK_START_12051_RE = re.compile(
    r"BLOCK_START BlockType=(\w+) Entity=%s EffectCardId=(.*) EffectIndex=(-1|\d+) Target=%s$" % (_E, _E)
)
BLOCK_START_20457_RE = re.compile(
    r"BLOCK_START BlockType=(\w+) Entity=%s EffectCardId=(.*) EffectIndex=(-1|\d+) Target=%s SubOption=(-1|\d+)$" % (_E, _E)
)
BLOCK_START_20457_TRIGGER_KEYWORD_RE = re.compile(
    r"BLOCK_START BlockType=(\w+) Entity=%s EffectCardId=(.*) EffectIndex=(-1|\d+) Target=%s SubOption=(-1|\d+) TriggerKeyword=(\w+)$" % (_E, _E)
)
```

**BLOCK_END / ACTION_END**:
```python
BLOCK_END_RE = re.compile(r"^(?:ACTION|BLOCK)_END$")
```

**FULL_ENTITY**:
```python
FULL_ENTITY_CREATE_RE = re.compile(r"FULL_ENTITY - Creating ID=(\d+) CardID=(\w+)?$")
FULL_ENTITY_UPDATE_RE = re.compile(r"FULL_ENTITY - Updating %s CardID=(\w+)?$" % _E)
```

**SHOW_ENTITY**:
```python
SHOW_ENTITY_RE = re.compile(r"SHOW_ENTITY - Updating Entity=%s CardID=(\w+)$" % _E)
```

**HIDE_ENTITY**:
```python
HIDE_ENTITY_RE = re.compile(r"HIDE_ENTITY - Entity=%s tag=(\w+) value=(\w+)$" % _E)
```

**CHANGE_ENTITY**:
```python
CHANGE_ENTITY_RE = re.compile(r"CHANGE_ENTITY - Updating Entity=%s CardID=(\w+)$" % _E)
```

**TAG_CHANGE**:
```python
DEF_CHANGE = "DEF CHANGE"
TAG_CHANGE_RE = re.compile(r"TAG_CHANGE Entity=%s tag=(\w+) value=(\w+) ?(%s)?" % (_E, DEF_CHANGE))
```

**META_DATA**:
```python
META_DATA_RE = re.compile(r"META_DATA - Meta=(\w+) Data=%s InfoCount=(\d+)" % _E)
```

**RESET_GAME**:
```python
RESET_GAME_RE = re.compile(r"RESET_GAME$")
```

**SUB_SPELL_START / END**:
```python
SUB_SPELL_START_RE = re.compile(r"SUB_SPELL_START - SpellPrefabGUID=([\w:.()]+) Source=(\d+) TargetCount=(\d+)$")
SUB_SPELL_END_RE = re.compile(r"SUB_SPELL_END$")
```

**CACHED_TAG_FOR_DORMANT_CHANGE**:
```python
CACHED_TAG_FOR_DORMANT_CHANGE_RE = re.compile(r"CACHED_TAG_FOR_DORMANT_CHANGE Entity=%s tag=(\w+) value=(\w+)" % _E)
```

**VO_SPELL**:
```python
VO_SPELL_RE = re.compile(r"VO_SPELL - BrassRingGuid=(.*) - VoSpellPrefabGUID=(\w*)? - Blocking=(True|False) - AdditionalDelayInMs=(\d+)$")
```

**SHUFFLE_DECK**:
```python
SHUFFLE_DECK_RE = re.compile(r"SHUFFLE_DECK PlayerID=(\d+)$")
```

### Tag/Entity Details

**Tag value pair** (inside entity creation blocks):
```python
TAG_VALUE_RE = re.compile(r"tag=(\w+) value=(\w+)")
```

**Metadata info line**:
```python
METADATA_INFO_RE = re.compile(r"Info\[(\d+)\] = %s" % _E)
```

**SubSpell source/target**:
```python
SUB_SPELL_START_SOURCE_RE = re.compile(r"Source = %s" % _E)
SUB_SPELL_START_TARGETS_RE = re.compile(r"Targets\[(\d+)\] = %s" % _E)
```

### Choices

**Entity choices (current format)**:
```python
CHOICES_CHOICE_RE = re.compile(r"id=(\d+) Player=%s TaskList=(\d+)? ChoiceType=(\w+) CountMin=(\d+) CountMax=(\d+)$" % _E)
```

**Entity choices (old formats)**:
```python
CHOICES_CHOICE_OLD_1_RE = re.compile(r"id=(\d+) ChoiceType=(\w+)$")
CHOICES_CHOICE_OLD_2_RE = re.compile(r"id=(\d+) PlayerId=(\d+) ChoiceType=(\w+) CountMin=(\d+) CountMax=(\d+)$")
```

**Choice source/entities**:
```python
CHOICES_SOURCE_RE = re.compile(r"Source=%s$" % _E)
CHOICES_ENTITIES_RE = re.compile(r"Entities\[(\d+)\]=(\[.+\])$")
```

**Send choices**:
```python
SEND_CHOICES_CHOICE_RE = re.compile(r"id=(\d+) ChoiceType=(.+)$")
SEND_CHOICES_ENTITIES_RE = re.compile(r"m_chosenEntities\[(\d+)\]=(\[.+\])$")
```

**Entities chosen**:
```python
ENTITIES_CHOSEN_RE = re.compile(r"id=(\d+) Player=%s EntitiesCount=(\d+)$" % _E)
ENTITIES_CHOSEN_ENTITIES_RE = re.compile(r"Entities\[(\d+)\]=%s$" % _E)
```

### Options

```python
OPTIONS_ENTITY_RE = re.compile(r"id=(\d+)$")
OPTIONS_OPTION_RE = re.compile(r"(option) (\d+) type=(\w+) mainEntity=%s?$" % _E)
OPTIONS_OPTION_ERROR_RE = re.compile(r"(option) (\d+) type=(\w+) mainEntity=%s? error=(\w+) errorParam=(\d+)?$" % _E)
OPTIONS_SUBOPTION_RE = re.compile(r"(subOption|target) (\d+) entity=%s?$" % _E)
OPTIONS_SUBOPTION_ERROR_RE = re.compile(r"(subOption|target) (\d+) entity=%s? error=(\w+) errorParam=(\d+)?$" % _E)
SEND_OPTION_RE = re.compile(r"selectedOption=(\d+) selectedSubOption=(-1|\d+) selectedTarget=(\d+) selectedPosition=(\d+)")
```

### Spectator Mode

```python
SPECTATOR_MODE_TOKEN = "=================="
SPECTATOR_MODE_BEGIN_GAME = "Start Spectator Game"
SPECTATOR_MODE_BEGIN_FIRST = "Begin Spectating 1st player"
SPECTATOR_MODE_BEGIN_SECOND = "Begin Spectating 2nd player"
SPECTATOR_MODE_END_MODE = "End Spectator Mode"
SPECTATOR_MODE_END_GAME = "End Spectator Game"
```

## How It Identifies the Local/Friendly Player

`FriendlyPlayerExporter` determines the friendly player by finding the **first `SHOW_ENTITY` packet where the card is in `ZONE=HAND`**:

```python
class FriendlyPlayerExporter(BaseExporter):
    def handle_show_entity(self, packet):
        tags = dict(packet.tags)
        if GameTag.CONTROLLER in tags:
            self._controller_map[int(packet.entity)] = tags[GameTag.CONTROLLER]

        if tags.get(GameTag.ZONE) != Zone.HAND:
            return

        # The first SHOW_ENTITY packet will always be the friendly player's.
        self.friendly_player = self._controller_map[packet.entity]
```

**Rationale**: During game setup, the client first reveals (SHOW_ENTITY) the cards in the local player's hand. The opponent's cards remain hidden. So the controller of the first revealed hand card is the friendly player.

Also handles AI games: if one player has `lo=0` (AI) and there's exactly one non-AI player, that's the friendly player:
```python
def handle_player(self, packet):
    if packet.lo == 0:
        self._ai_player = packet.player_id
    else:
        self._non_ai_players.append(packet.player_id)
```

## How It Tracks Entity Ownership (CONTROLLER tag)

The `PlayerManager` maintains a `_entity_controller_map: Dict[int, int]` mapping entity IDs to player IDs:

```python
def register_controller(self, entity_id: int, player_id: int):
    self._entity_controller_map[entity_id] = player_id

def get_controller_by_entity_id(self, entity_id: int) -> Optional[int]:
    return self._entity_controller_map.get(entity_id)
```

CONTROLLER is registered in two places:

1. **During entity creation** (initial tags):
```python
if tag == GameTag.CONTROLLER:
    entity_id = coerce_to_entity_id(ps.entity_packet.entity)
    ps.manager.register_controller(int(entity_id), int(value))
```

2. **On TAG_CHANGE**:
```python
if tag == GameTag.CONTROLLER:
    entity_id = coerce_to_entity_id(entity)
    ps.manager.register_controller(int(entity_id), int(value))
```

This controller map is also used for **player name registration** -- during mulligan, the controller of offered cards identifies which player name belongs to which PlayerID.

## How It Handles Turns

Turn tracking is not done by the parser directly -- it creates TAG_CHANGE packets for tags like `STEP`, `TURN`, `CURRENT_PLAYER`, etc. Consumers of the packet tree handle turn logic.

The parser tracks `GameTag.FIRST_PLAYER` to identify who goes first:
```python
elif tag == GameTag.FIRST_PLAYER:
    entity_id = coerce_to_entity_id(entity)
    ps.manager.notify_first_player(int(entity_id))
```

For Battlegrounds specifically, turns work differently since there are 8 players. The parser itself doesn't interpret BG turn structure -- it just faithfully records the TAG_CHANGE packets.

## How It Tracks Board State

The parser itself does not track board state. It produces packets, and the `EntityTreeExporter` materializes entities with tags including `ZONE`, `ZONE_POSITION`, `CONTROLLER`, etc.

Board state is derived from the exported entity tree by filtering for:
- `Zone.PLAY` (on the board)
- Matching `CONTROLLER` value
- `CARDTYPE == MINION`

This is exactly what `HSBG_LogParser` does on top of this library.

## How It Handles Dual Log Streams

The parser uses **`GameState.DebugPrintPower`** as the primary (and only) stream:

```python
class PowerHandler(HandlerBase):
    def find_callback(self, method):
        if method == self.parse_method("DebugPrintPower"):
            return self.handle_data
        elif method == self.parse_method("DebugPrintGame"):
            return self.handle_game
```

Where `self._game_state_processor = "GameState"`, so it matches `GameState.DebugPrintPower`.

It does **not** parse `PowerTaskList.DebugPrintPower` at all. The rationale: GameState output arrives first and contains the same data. PowerTaskList is a duplicate that fires after visual processing.

Other method prefixes handled:
- `GameState.DebugPrintGame` -- Game metadata and player names
- `GameState.DebugPrintEntityChoices` / `DebugPrintChoices` -- Choice packets
- `GameState.SendChoices` -- Sent choices
- `GameState.DebugPrintEntitiesChosen` -- Chosen entities
- `GameState.DebugPrintOptions` -- Available options
- `GameState.SendOption` -- Sent options

## BG-Specific Handling

### Player Name Guessing Disabled for BG

The `PlayerManager._guess_player_entity_id` method explicitly skips its name-guessing heuristic for Battlegrounds because there are more than 2 players:

```python
if (
    len(self._players_by_name) == 1 and
    name != UNKNOWN_HUMAN_PLAYER and
    self._game_type != GameType.GT_BATTLEGROUNDS and
    not is_mercenaries_game_type(self._game_type)
):
    # Maybe we can figure the name out right there and then.
    # NOTE: This is a neat trick, but it doesn't really work (and leads to
    # errors) when there are lots of players such that we can't predict the
    # entity ids. Hence the check for Battlegrounds above.
```

### Broken Block Nesting Workaround

Battlegrounds games frequently omit `BLOCK_END` before options start. The `OptionsHandler` has a hack for this:

```python
@staticmethod
def _check_for_options_hack(ps: ParsingState, ts):
    # Battlegrounds games tend to omit the BLOCK_END just before options start. As
    # options will always be on the top level, we can safely close any remaining block
    # that is open at this time.
    if isinstance(ps.current_block, packets.Block):
        logging.warning("[%s] Broken option nesting. Working around...", ts)
        ps.block_end(ts)
        assert not isinstance(ps.current_block, packets.Block)
```

### RESET_GAME Support

Battlegrounds uses `RESET_GAME` between combat rounds. The parser creates a `ResetGame` packet, and the `EntityTreeExporter` calls `self.game.reset()` when encountering a `GAME_RESET` block type.

### Player Resolution Order

The player resolution system tracks `_player_resolution_order` to handle the fact that BG has many players and names may not be immediately available. The `UNKNOWN HUMAN PLAYER` handling is also BG-aware:

```python
elif (
    player.name == UNKNOWN_HUMAN_PLAYER and
    player.entity_id is None and
    player.player_id is None and
    self._game_type != GameType.GT_BATTLEGROUNDS and
    len(self._player_resolution_order) < len(self._players_by_player_id)
):
```

### No Direct BACON_ Tag Handling

The parser uses the `hearthstone` library's `GameTag` enum which includes BG-specific tags (like `BACON_*`), but python-hslog itself has no special logic for interpreting them. All tags are stored generically in entity tag dictionaries.

## Key Design Patterns

1. **Lazy player resolution**: Players start as `PlayerReference` objects with partial data (maybe just a name, or just an entity_id). Data is merged as more information becomes available. This handles the fact that player names, entity IDs, and player IDs arrive at different times.

2. **Entity string polymorphism**: The `_E` regex pattern matches 5 different formats (GameEntity, UNKNOWN HUMAN PLAYER, bracketed entity, bare integer, player name). `parse_entity_or_player()` returns either an int (entity ID) or a `PlayerReference`.

3. **Packet tree structure**: Blocks nest as a tree with parent pointers. Each block contains child packets. The tree is walked by exporters.

4. **Controller map for name resolution**: The CONTROLLER tag is tracked not just for ownership but as a key mechanism for resolving player names -- the controller of a mulligan card tells you which PlayerID owns it.

5. **Version-aware regex**: Multiple BLOCK_START patterns handle different client versions (pre-12051, 12051+, 20457+, 20457 with TriggerKeyword).

6. **Timestamp handling**: Logs only contain times, not dates. The parser handles midnight rollovers by incrementing the date when a new timestamp is earlier than the previous one.
