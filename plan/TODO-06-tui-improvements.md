# TODO-06: TUI Clarity & Missing Info

Status: **DONE**
Priority: **MEDIUM**

## Issues

### 1. Available factions not shown

The TUI has no indication of which minion types (factions) are in the current
game's pool. Battlegrounds randomly excludes some tribes each game and this
is critical information for decision-making.

**What to do:**
- Parse the `BACON_EXCLUDED_MINION_TYPES` or equivalent tag from the game
  entity / lobby to determine which tribes are excluded.
- Display the list of *available* factions somewhere in the game header or
  hero panel (e.g. `Tribes  Beast, Demon, Dragon, Mech, Murloc`).
- State struct needs a field (e.g. `AvailableTribes []string`).

### 2. "Bonus Gold" counter unclear — depends on winning

The `GOLD_NEXT_TURN` ability counter (fed by `BACON_PLAYER_EXTRA_GOLD_NEXT_TURN`
tag + Overconfidence `BG28_884e` zone tracking) currently displays as
"Bonus Gold" with a raw number. The Overconfidence component only pays out
if the player *wins* the next combat round, but the TUI doesn't communicate
this conditional nature.

**What to do:**
- When `overconfidenceCount > 0`, annotate the display to make it clear
  which portion is conditional on winning, e.g.:
  `Bonus Gold    2 (+1 if win)` or split into two lines:
  `Bonus Gold    2 (sure) +1 (win)`.
- The processor already tracks `goldNextTurnSure` vs `overconfidenceCount`
  separately (`processor.go:73-74`), so the data is available — it just
  needs to be surfaced in the display string and/or the proto.

### 3. Win/loss last round not indicated

The gamestate tracks `WinStreak` and `LossStreak` (`state.go:50-51`) and
updates them via `RecordRoundWin`/`RecordRoundLoss`, but the TUI never
displays this information. The user has no way to see whether they won or
lost the previous combat round.

**What to do:**
- Show last round result in the hero panel or game panel, e.g.:
  `Last    W (streak: 3)` or `Last    L (streak: 2)`.
- The data is already in `PlayerState.WinStreak` / `LossStreak` and
  exposed via proto `Player.win_streak` / `Player.loss_streak`.

### 4. "Spell+" label is unclear

The hero panel shows `Spell+  0` (line 446 of tui.go). This is the player's
`SpellPower` value from the `SPELL_POWER` tag. The label "Spell+" is
cryptic — most BG players wouldn't know this refers to the tavern spell
damage/stat bonus modifier.

**What to do:**
- Rename to a clearer label. Options:
  - `Spell Pwr` — standard Hearthstone terminology
  - `Spell Dmg` — common shorthand
  - `Tavern +` — if it only applies to tavern spells in BG context
- Pick whichever fits the column width best (hero panel uses 8-char labels).

## Files likely affected

- `internal/tui/tui.go` — rendering changes for all 4 items
- `internal/gamestate/state.go` — add `AvailableTribes` field (item 1)
- `internal/gamestate/processor.go` — parse tribe exclusion tags (item 1),
  surface overconfidence breakdown in display string (item 2)
- `internal/gamestate/categories.go` — display name update if needed (item 2)
- `proto/battlestream/v1/game.proto` — add tribes field, update gold display
