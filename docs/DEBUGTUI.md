# TUI Design Reference

This document covers the design guidelines that apply to **all** Bubbletea TUI
views in battlestream:

| Package              | Purpose                                   |
|----------------------|-------------------------------------------|
| `internal/tui`       | Live dashboard — connects to daemon via gRPC, shows current board/stats |
| `internal/debugtui`  | Step-through Power.log replay viewer for debugging parser and game-state issues |

Both TUIs use the same rendering conventions: shared colour palette,
[Bubbles](https://github.com/charmbracelet/bubbles) components, viewport
scrollbars, and per-panel mouse routing. New panels added to either TUI must
follow these guidelines.

All views use:
- [Bubbletea](https://github.com/charmbracelet/bubbletea) — elm-architecture event loop
- [Lipgloss](https://github.com/charmbracelet/lipgloss) — styling and layout
- [Bubbles](https://github.com/charmbracelet/bubbles) — `viewport`, `spinner`, `textinput`

---

## Panel Layout

### Live dashboard (`internal/tui`)

```
┌─ game ───────────────────────┐┌─ hero ──────────────────────────────────────┐
│ BATTLESTREAM                 ││ PlayerName                                  │
│ Game   game-1                ││ Health  ████████░░░ 30/40                   │
│ Phase  RECRUIT               ││ Armor   14                                  │
│ Turn   5                     ││ Triples 2                                   │
│ Tavern 3 ★★★☆☆☆             ││                                             │
└──────────────────────────────┘└─────────────────────────────────────────────┘
┌─ board (viewport+scrollbar) ─┐┌─ buff sources (viewport+scrollbar) ─────────┐
│ YOUR BOARD                   ││ BUFF SOURCES                                │
│   Minion A         12/10  │  ││ Bloodgems     +4/+0                      │  │
│   Minion B          3/2   │  ││ Nomi          +2/+2                      │  │
│                           │  ││                                           │  │
│                           ▼  ││ ABILITIES                                 │  │
└──────────────────────────────┘│ Bonus Gold    2                           │  │
                                ╰─────────────────────────────────────────────╯
┌─ session ───────────────────────────────────────────────────────────────────┐
│ SESSION  W: 3  L: 2  Avg 3.4  Games 5  Best #1                             │
└─────────────────────────────────────────────────────────────────────────────┘
  [r] Refresh game  [R] Refresh stats  [q] Quit  scroll: mouse wheel or drag
```

### Debug replay (`internal/debugtui`)

```
┌─ header ────────────────────────────────────────────────────────────────────┐
│ DEBUG REPLAY  Game N/N (PlayerName)                                         │
│ Step N/N  Turn N  Phase X  Tavern N ★★…                                    │
└─────────────────────────────────────────────────────────────────────────────┘
┌─ player ─────────────────────┐┌─ board (viewport+scrollbar) ────────────────┐
│ Name                         ││ BOARD (N)                                   │
│ Health ████████░░░ 30/40     ││   Minion A           12/10               │  │
│ Armor  N                     ││   Minion B            3/2                │  │
│ Triples N                    ││                                           │  │
└──────────────────────────────┘│                                           ▼  │
                                └─────────────────────────────────────────────┘
┌─ buff sources (viewport+sb) ─┐┌─ changes (viewport+scrollbar) ──────────────┐
│ BUFF SOURCES                 ││ CHANGES                                      │
│ Bloodgems     +4/+0       │  ││ + Minion A (12/10)                       │  │
│ Nomi          +2/+2       │  ││ Turn: 4 -> 5                             │  │
│                           │  ││                                           │  │
│ ABILITIES                 │  ││                                           │  │
│ Bonus Gold    2           │  ││                                           ▼  │
└──────────────────────────────┘└─────────────────────────────────────────────┘
┌─ event line ────────────────────────────────────────────────────────────────┐
│ TAG_CHANGE  EntityName  ZONE=PLAY                                           │
└─────────────────────────────────────────────────────────────────────────────┘
┌─ raw log (full-width viewport+scrollbar) ───────────────────────────────────┐
│ RAW LOG  N lines                                                            │
│ D 12:34:56.789 GameState.DebugPrintPower() - TAG_CHANGE ...             █  │
│ ...                                                                     │  │
│ ...                                                                     ▼  │
└─────────────────────────────────────────────────────────────────────────────┘
  h/l:step  [/]:turn  {/}:phase  g/G:start/end  t:jump  j/k:raw  J/K:panels …
```

### Column sizing

All widths are derived from the terminal width reported by `tea.WindowSizeMsg`:

```
innerW      = m.width - 4          // outer margin/border
halfW       = innerW/2 - 2         // each half-panel total width (incl. border+padding)
vpContentW  = innerW/2 - 7         // viewport content width inside a half-panel
                                   //   = halfW - 4 (border+padding) - 1 (scrollbar)
```

Height for the two middle rows is split evenly from the space remaining after
the fixed-height header, event line, help bar, and a minimum raw-log height.

---

## Scrollbars

Every viewport panel has a 1-character-wide scrollbar column on its right edge,
rendered by `renderScrollbar(vp viewport.Model, height int) string`
(in `internal/debugtui/render.go`).

| Character | Meaning                                         |
|-----------|-------------------------------------------------|
| ` `       | Blank — content fits without scrolling           |
| `│`       | Track — scrollable but cursor is not here        |
| `█`       | Thumb — current scroll position                  |
| `▲`       | Top arrow — content exists above current view    |
| `▼`       | Bottom arrow — content exists below current view |

The scrollbar is joined horizontally with the viewport via
`lipgloss.JoinHorizontal`, so `vpContentW` is 1 char narrower than the
available content area to accommodate it.

---

## Mouse Support

The program is launched with `tea.WithMouseCellMotion()` so all mouse events
are delivered as `tea.MouseMsg`.

### Wheel scrolling

Mouse wheel events (`msg.IsWheel()`) are routed to the viewport whose Y range
contains the cursor. Each call delegates to `viewport.Update(msg)` so the
viewport handles its own scroll acceleration.

### Scrollbar click-to-jump

Clicking (left button press) on a scrollbar column jumps the viewport to the
proportional position:

```
relY = clickY - trackY
pct  = relY / (trackH - 1)
YOffset = int(pct * float64(totalLines - trackH))
```

Implemented in `scrollbarJump(*viewport.Model, clickY, trackY, trackH int)`.

### Scrollbar drag-to-scrub

Holding the left button and dragging within — or outside of — the scrollbar
column continues to update the viewport scroll position on every
`MouseActionMotion` event while `Button == MouseButtonLeft`.

Drag state:

| Field         | Description                                    |
|---------------|------------------------------------------------|
| `scrubbing`   | True while left button is held on a scrollbar  |
| `scrubPanel`  | Index of the panel being scrubbed (0–3)        |
| `scrubTrackY` | Terminal Y of the scrollbar track's first row  |
| `scrubTrackH` | Height of the track (= viewport height)        |

### Panel position tracking

Scrollbar X columns and viewport Y positions are computed during each `View()`
call and stored in the model so the mouse handler (called from `Update()`) can
route events without re-rendering:

| Field           | Description                                             |
|-----------------|---------------------------------------------------------|
| `boardScrollX`  | Absolute terminal X of the board scrollbar column       |
| `buffScrollX`   | Absolute terminal X of the buff-sources scrollbar column |
| `changesScrollX`| Absolute terminal X of the changes scrollbar column     |
| `rawScrollX`    | Absolute terminal X of the raw-log scrollbar column     |
| `boardVPY/H`    | Terminal Y start and height of the board viewport       |
| `buffVPY/H`     | Terminal Y start and height of the buff viewport        |
| `changesVPY/H`  | Terminal Y start and height of the changes viewport     |
| `rawVPY/H`      | Terminal Y start and height of the raw-log viewport     |

Scrollbar X formulas (0-indexed from terminal left):

**Debug TUI** (`internal/debugtui`):
```
buffScrollX    = 2 + vpContentW                    // left half: border+pad + vp
boardScrollX   = halfW + 4 + 2 + vpContentW        // right half: left panel total + border+pad + vp
changesScrollX = boardScrollX                      // same column as board
rawScrollX     = 2 + (innerW - 4 - 1)             // full-width panel: border+pad + (contentW-1)
```

**Live TUI** (`internal/tui`):
```
boardScrollX   = 2 + vpContentW                   // left panel: border+pad + vp
modsScrollX    = (colW + 4) + 2 + vpContentW      // right panel: left total + border+pad + vp
```

VP Y formulas (border row + header row = +2 from panel start):

```
boardVPY   = row2StartY + 2
buffVPY    = row3StartY + 2
changesVPY = row3StartY + 2
rawVPY     = rawStartY  + 2
```

---

## Keyboard Shortcuts

| Key          | Action                                              |
|--------------|-----------------------------------------------------|
| `h` / `l`   | Step backward / forward one event                   |
| `[` / `]`   | Jump to previous / next BG turn                     |
| `{` / `}`   | Jump to previous / next phase boundary              |
| `g` / `G`   | Jump to first / last step                           |
| `t`          | Open turn-jump input (type number, Enter to go)     |
| `Esc`        | Cancel turn-jump input                              |
| `j` / `k`   | Scroll raw-log panel down / up one line             |
| `J` / `K`   | Scroll board+buff+changes panels down / up (synced) |
| `f`          | Cycle event type filter                             |
| `s`          | Return to game picker (multi-game files only)       |
| `q` / Ctrl+C | Quit                                                |

Mouse wheel and scrollbar click/drag work on any panel independently.
`J`/`K` keep the board, buff-sources, and changes panels in sync.

---

## Bubbles Components

These components are used across both TUIs:

| Component          | TUI   | Usage                                            |
|--------------------|-------|--------------------------------------------------|
| `viewport.Model`   | both  | All scrollable panels; `MouseWheelEnabled = true` on each |
| `spinner.Model`    | both  | Animated dot spinner while connecting / loading  |
| `textinput.Model`  | debug | Jump-to-turn input (`t` key), digit validator, `CharLimit=6` |

All viewports have `MouseWheelEnabled = true`, but wheel routing is handled
manually in the `Update()` mouse handler (not by the viewport's built-in
handler) so each panel scrolls independently based on cursor position.

### Rules for new panels

1. Any panel whose content can exceed the available height **must** use
   `viewport.Model` — do not pad or truncate manually.
2. Every `viewport.Model` must have a `tuiScrollbar` / `renderScrollbar` column
   joined horizontally to its right edge.
3. Reduce `vpContentW` by 1 from the available content area to accommodate the
   scrollbar column.
4. Track scrollbar column X and viewport Y/height in the model struct and update
   them during `View()` so the mouse handler can route events correctly.
5. Enable `tea.WithMouseCellMotion()` on the program; do **not** rely on the
   viewport's internal mouse handler.

---

## Non-interactive / Dump mode

`debugtui.Dump(path string, turn int, width int) (string, error)` renders the
TUI at a given BG turn to a plain string (ANSI escape codes included). Used by:

- `battlestream replay --dump --turn N --width W` — CLI rendering
- `TestDump_Golden` — golden screenshot tests

`turn=0` jumps to the last step.

---

## Testing

### Golden screenshot tests

`TestDump_Golden` in `internal/debugtui/replay_test.go` captures TUI output for
three reference points and compares against files in
`internal/debugtui/testdata/golden/`:

| Golden file      | Description                    |
|------------------|--------------------------------|
| `first-turn.txt` | State at BG turn 1              |
| `mid-game.txt`   | State at BG turn 5              |
| `last-turn.txt`  | State at the final step         |

Golden files store raw output including ANSI colour codes. Failures are
reported with a line-by-line diff with ANSI stripped for readability.

To regenerate:

```sh
go test ./internal/debugtui/ -update-golden
```

### Other tests

| Test                        | Coverage                                    |
|-----------------------------|---------------------------------------------|
| `TestLoadReplay`            | Full parse of the test log; verifies step counts, game summary, and final board |
| `TestDump_DoesNotPanic`     | `Dump()` at turn 1 returns non-empty output |
| `TestDump_LastTurn`         | `Dump()` with `turn=999` clamps to last step |
| `TestLoadAllGamesMultiFile` | Double-loading the same file produces 2× the games |
