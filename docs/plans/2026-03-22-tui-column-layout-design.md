# TUI Column-Based Layout Design

**Date:** 2026-03-22
**Status:** Approved

## Problem

The live TUI uses a row-based layout where left and right panels in each row are joined horizontally, forcing them to the same height. When the Game Info panel (left) is shorter than the Hero Stats panel (right, especially in duos with partner info), the Game Info panel gets padded with empty space, pushing the Board panel unnecessarily lower.

## Solution

Switch from row-first to column-first layout. Each column stacks its panels vertically with no wasted space, then the two columns are joined horizontally.

### Layout Change

**Before (row-first):**
```
row1    = JoinHorizontal(GamePanel, HeroPanel)        <- forced same height
row2    = JoinHorizontal(BoardVP,   BuffVP)            <- forced same height
row3    = JoinHorizontal(PartnerVP, PartnerBuffVP)     <- forced same height
result  = JoinVertical(row1, row2, row3, session, help)
```

**After (column-first):**
```
leftCol  = JoinVertical(GamePanel, BoardVP, [PartnerBoardVP])
rightCol = JoinVertical(HeroPanel, BuffVP, [PartnerBuffVP])
columns  = JoinHorizontal(Top, leftCol, rightCol)    <- aligned at top edge
result   = JoinVertical(columns, session, help)
```

`lipgloss.Top` is the alignment parameter — it aligns the two columns at their top edges.

### Viewport Height Computation

Each column computes its own viewport budget independently:

- Render header panels first (gamePanel, heroPanel) to measure natural heights
- Left VP budget: `totalH - Height(gamePanel) - sessionH - helpH - borders`
- Right VP budget: `totalH - Height(heroPanel) - sessionH - helpH - borders`
- If duos: split each budget by `hSplit` for main vs partner viewports

### Mouse Y Offset Fix

**Existing bug:** The `CombinedModel` adds a 1-row mode indicator bar at the top but forwards `tea.MouseMsg` to the live model without adjusting Y coordinates. All mouse hit-testing is off by 1 row.

**Fix:** Add a `parentYOffset int` field to `Model`. Parent views set this to communicate how many rows they render above the live model's content. All `msg.Y` references in `handleMouse` are adjusted:

```go
// In Model:
parentYOffset int

// Set by CombinedModel:
m.live.parentYOffset = 1  // mode indicator bar

// In handleMouse, at the top:
y := msg.Y - m.parentYOffset
x := msg.X
// Use x, y throughout instead of msg.X, msg.Y
```

This is flexible — any parent wrapper can set its own offset.

### What Changes

- `View()` method in `tui.go` — restructured from row-first to column-first (~50 lines)
- `handleMouse()` — use adjusted y coordinates throughout
- `Model` struct — add `parentYOffset int` field
- `combined.go` — set `parentYOffset = 1` on the live model
- Mouse Y positions (`boardVPY`, `modsVPY`, etc.) will now differ between columns

### What Stays the Same

- Vertical divider drag (vSplit) — controls left/right column width ratio
- Horizontal divider drag (hSplit) — controls board/partner height split
- Mouse wheel scrolling on all 4 viewports
- Scrollbar drag-scrubbing
- All panel content rendering functions untouched
- Session bar and help bar remain full-width at bottom
- Dump mode output
- Config persistence of split ratios
