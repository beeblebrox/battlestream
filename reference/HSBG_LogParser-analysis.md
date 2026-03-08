# HSBG_LogParser Analysis

**Project**: `HSBG_LogParser/main.py` -- A thin wrapper around `python-hslog` specifically for Hearthstone Battlegrounds board state extraction.

## Overview

This is not a standalone parser. It uses `python-hslog` for all log parsing and the `hearthstone` library for entity/enum types. Its value is in the **BG-specific state extraction logic** it layers on top.

## Regex Patterns

**None defined directly.** All parsing is delegated to `hslog.parser.LogParser` and `hslog.export.EntityTreeExporter`. This project is purely a consumer of the parsed data.

## How It Identifies the Local/Friendly Player vs Opponents

It does not directly identify the local player. It operates on `Player` entity objects from the `hearthstone.entities` module, which are populated by `EntityTreeExporter.handle_player()`. The exporter resolves player names via `PlayerManager`.

## How It Tracks Entity Ownership (CONTROLLER tag)

Ownership is tracked via `GameTag.CONTROLLER`:

```python
def get_current_minions(self, player: Player):
    minions = []
    for e in player.entities:
        if e.tags[GameTag.CONTROLLER] == player.tags[GameTag.CONTROLLER] and e.zone == Zone.PLAY:
            if GameTag.CARDTYPE in e.tags.keys() and e.tags[GameTag.CARDTYPE] == CardType.MINION:
                minions.append(e)
    return minions
```

Key pattern: **Entity's `CONTROLLER` tag must match the Player's `CONTROLLER` tag.** This is the canonical way to determine who owns a minion.

## How It Handles Turns / Combat Phases in Battlegrounds

Combat detection uses a two-phase approach:

1. **Combat start**: Detected by a `BLOCK_START` with `BlockType=TRIGGER` where the triggering entity has `CardType.MOVE_MINION_HOVER_TARGET`. This is the BG-specific "arrange your board" phase trigger:

```python
def _packet_start(self, packet: Packet):
    if packet.__class__ == Block:
        if packet.type == BlockType.TRIGGER:
            if self.export.find_entity(packet.entity, "FULL_ENTITY").tags[GameTag.CARDTYPE] == CardType.MOVE_MINION_HOVER_TARGET:
                return True
    return False
```

2. **Combat end**: Detected by a `TAG_CHANGE` setting `STEP` to `MAIN_END`:

```python
def _packet_end(self, packet: Packet):
    if packet.__class__ == TagChange:
        if packet.tag == GameTag.STEP and packet.value == Step.MAIN_END:
            return True
    return False
```

The `get_packets_at_combat(packets, k)` method extracts packets for the k-th combat round.

## How It Tracks Board State

Board state is extracted by filtering entities where:
- `CONTROLLER` matches the player
- `zone == Zone.PLAY`
- `CARDTYPE == CardType.MINION`

This gives the current board for a given player at any point in the packet stream.

## Dual Log Streams

Not handled -- all parsing is delegated to `python-hslog` which uses `GameState.DebugPrintPower()` as its primary stream.

## BG-Specific Handling

- **`MOVE_MINION_HOVER_TARGET`**: Used as a combat phase marker. This card type only exists in Battlegrounds and represents the pre-combat arrangement target.
- **`Step.MAIN_END`**: Used to delimit combat end boundaries.
- **No BACON_ tag handling**: Despite being BG-focused, it does not directly reference BACON_ prefixed tags.
- **Combat round counting**: The `get_packets_at_combat(packets, k)` method counts combat rounds by counting occurrences of the `MOVE_MINION_HOVER_TARGET` trigger.
