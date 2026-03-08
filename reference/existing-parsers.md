# Existing Hearthstone Log Parser References

## Key Projects

### 1. HearthSim/python-hslog (Python)
- **URL**: https://github.com/HearthSim/python-hslog
- **Description**: Most mature Power.log deserializer. Used by HSReplay.net.
- **Local Player Detection**: `FriendlyPlayerExporter` class — analyzes which
  player gets card reveals (only the friendly player's cards are shown).
  Looks at initial tag changes and FULL_ENTITY packets to determine which
  "side" gets cards revealed.
- **Key Insight**: "A Power.log file does have a friendly player: the bottom
  player, whose cards are revealed."
- **Architecture**: Deserializes log into packet objects, then exports game state.

### 2. Breekys/HSBG_LogParser (BG-specific)
- **URL**: https://github.com/Breekys/HSBG_LogParser
- **Description**: Power.log parser specifically for Hearthstone Battlegrounds.
- **Relevance**: BG-specific parsing logic, handles the unique BG game flow.

### 3. Tespa/hearthstone-parser (Node.js)
- **URL**: https://github.com/Tespa/hearthstone-parser
- **Description**: Real-time log file parser that builds a game state tree.
- **Key Insight**: "When parsing through the logs, you might find two sets of
  data: GameState and PowerTaskList — they both essentially log the same
  information, but GameState is logged as soon as the client receives the
  information and PowerTaskList is logged once the client actually executes
  the action."
- **Recommendation**: Parse GameState for real-time tracking.

### 4. chevcast/hearthstone-log-watcher (Node.js)
- **URL**: https://github.com/CatDadCode/hearthstone-log-watcher
- **Description**: Monitors log file and emits events.
- **Local Player Detection**: "The game-start event fires when the watcher has
  gathered enough data to determine which of the two players is the local
  player. It was a lot more complicated to figure that out than one might think."

### 5. twanvl/hearthstone-battlegrounds-simulator (C++)
- **URL**: https://github.com/twanvl/hearthstone-battlegrounds-simulator
- **File**: `src/log_parser.cpp`
- **Description**: BG combat simulator with a log parser for importing board state.

### 6. postcasio/powerlog (TypeScript)
- **URL**: https://github.com/postcasio/powerlog
- **Description**: General Hearthstone Power.log parser.

## Local Player Identification Techniques

### Method 1: GameAccountId (Simplest, BG-specific)
In Battlegrounds, two players are created in CREATE_GAME:
- Local player: `GameAccountId=[hi=<nonzero> lo=<nonzero>]`
- Dummy player: `GameAccountId=[hi=0 lo=0]` + `BACON_DUMMY_PLAYER=1`

This is the simplest method and works reliably for BG.

### Method 2: Card Reveal Analysis (python-hslog approach)
Look at which player's cards are being revealed in FULL_ENTITY blocks.
The friendly player's hand cards are shown with actual CardIDs, while
the opponent's are hidden. More complex but works for all game modes.

### Method 3: CURRENT_PLAYER tracking
Track `TAG_CHANGE Entity=<playerEntity> tag=CURRENT_PLAYER value=1`.
The first player to get CURRENT_PLAYER=1 is the one whose turn it is.
In BG, the dummy player (player 15/Bob) gets CURRENT_PLAYER first.

## Entity Ownership via CONTROLLER

Every entity in the game has a CONTROLLER tag matching a PlayerID.
When parsing TAG_CHANGE for bracketed entities:

```
Entity=[entityName=Foo id=123 zone=PLAY zonePos=1 cardId=XXX player=7]
```

The `player=7` field = CONTROLLER. Compare against the local player's
PlayerID to determine ownership.

For FULL_ENTITY blocks, the CONTROLLER tag is in the body:

```
FULL_ENTITY - Creating ID=123 CardID=XXX
    tag=CONTROLLER value=7
```
