# TUI Column-Based Layout Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Restructure the live TUI from row-based to column-based layout so panels pack tightly per-column, and fix the existing mouse Y offset bug from the CombinedModel tab bar.

**Architecture:** Replace the row-first join pattern (join left+right per row, stack rows) with column-first (stack panels per column, join columns). Add a `parentYOffset` field so parent views can communicate their chrome height for correct mouse routing.

**Tech Stack:** Go, charmbracelet/lipgloss, charmbracelet/bubbletea, charmbracelet/bubbles/viewport

---

### Task 1: Add parentYOffset field and fix mouse coordinate adjustment

**Files:**
- Modify: `internal/tui/tui.go:114-168` (Model struct)
- Modify: `internal/tui/tui.go:831-949` (handleMouse, identifyScrollbar, scrubAt)
- Modify: `internal/tui/combined.go:42-49` (NewCombined)

**Step 1: Add parentYOffset to Model struct**

In `internal/tui/tui.go`, add after line 167 (`cfg *config.Config`):

```go
	// Y offset from parent view (e.g. CombinedModel tab bar).
	// Parent sets this so mouse coordinates are adjusted correctly.
	parentYOffset int
```

**Step 2: Set parentYOffset in CombinedModel**

In `internal/tui/combined.go`, in `NewCombined()`, after `live: New(grpcAddr, cfg),` add a line to set the offset. Replace the return block:

```go
	live := New(grpcAddr, cfg)
	live.parentYOffset = 1 // mode indicator bar
	return &CombinedModel{
		mode:     modeLive,
		live:     live,
		grpcAddr: grpcAddr,
		logFiles: logFiles,
		store:    st,
	}
```

**Step 3: Adjust handleMouse to use offset-corrected coordinates**

In `internal/tui/tui.go`, replace the `handleMouse` method (lines 831-919) with:

```go
func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	x := msg.X
	y := msg.Y - m.parentYOffset

	// Wheel: route to whichever panel the cursor is over.
	if tea.MouseEvent(msg).IsWheel() {
		var cmd tea.Cmd
		// Check partner pane first (below main panels).
		if m.game != nil && m.game.IsDuos &&
			y >= m.partnerVPY && y < m.partnerVPY+m.partnerVPH {
			if x >= m.width/2 {
				m.partnerModsVP, cmd = m.partnerModsVP.Update(msg)
			} else {
				m.partnerBoardVP, cmd = m.partnerBoardVP.Update(msg)
			}
		} else if x >= m.width/2 {
			m.modsVP, cmd = m.modsVP.Update(msg)
		} else {
			m.boardVP, cmd = m.boardVP.Update(msg)
		}
		return m, cmd
	}

	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button == tea.MouseButtonLeft {
			// Check vertical divider.
			if x >= m.dividerX-1 && x <= m.dividerX+1 &&
				y >= m.row2StartY {
				m.draggingV = true
				return m, nil
			}
			// Check horizontal divider (Duos only).
			if m.game != nil && m.game.IsDuos &&
				y >= m.dividerY-1 && y <= m.dividerY+1 {
				m.draggingH = true
				return m, nil
			}
			// Scrollbar detection.
			panel, trackY, trackH := m.identifyScrollbar(x, y)
			if panel >= 0 {
				m.scrubbing = true
				m.scrubPanel = panel
				m.scrubTrackY = trackY
				m.scrubTrackH = trackH
				m.scrubAt(y)
			}
		}
	case tea.MouseActionMotion:
		if m.draggingV && msg.Button == tea.MouseButtonLeft {
			totalInner := m.width - 8
			newLeft := x - 4
			ratio := float64(newLeft) / float64(totalInner)
			if ratio < 0.2 {
				ratio = 0.2
			}
			if ratio > 0.8 {
				ratio = 0.8
			}
			m.vSplit = ratio
			return m, nil
		}
		if m.draggingH && msg.Button == tea.MouseButtonLeft {
			totalAvailable := m.height - m.row2StartY - 3 - 1 - 3
			newMain := y - m.row2StartY
			ratio := float64(newMain) / float64(totalAvailable)
			if ratio < 0.2 {
				ratio = 0.2
			}
			if ratio > 0.8 {
				ratio = 0.8
			}
			m.hSplit = ratio
			return m, nil
		}
		if m.scrubbing && msg.Button == tea.MouseButtonLeft {
			m.scrubAt(y)
		}
	case tea.MouseActionRelease:
		if m.draggingV || m.draggingH {
			m.draggingV = false
			m.draggingH = false
			if m.cfg != nil {
				m.cfg.TUI.VerticalSplit = m.vSplit
				m.cfg.TUI.HorizontalSplit = m.hSplit
				go m.cfg.SaveTUI() //nolint:errcheck // fire-and-forget
			}
			return m, nil
		}
		m.scrubbing = false
	}
	return m, nil
}
```

**Step 4: Run tests to verify no regressions**

Run: `go test -count=1 ./internal/tui/`
Expected: All tests pass (existing tests don't exercise mouse handling).

**Step 5: Run vet**

Run: `go vet ./internal/tui/`
Expected: No errors.

**Step 6: Commit**

```bash
git add internal/tui/tui.go internal/tui/combined.go
git commit -m "fix: adjust mouse Y coordinates for parent view offset

CombinedModel adds a tab bar row but wasn't offsetting mouse Y coords.
Add parentYOffset field set by parent views for correct hit-testing."
```

---

### Task 2: Restructure View() from row-based to column-based layout

**Files:**
- Modify: `internal/tui/tui.go:356-540` (View method)

**Step 1: Replace the View() method layout logic**

Replace lines 409-539 (from `// ── Row 1:` through the end of the function before the closing brace) with column-based layout. Keep lines 356-407 (early returns, width calculations) unchanged.

Replace the layout section starting at line 409 with:

```go
	// ── Header panels (rendered first to measure natural heights) ──
	gamePanel := m.renderGamePanel(colW)
	heroPanel := m.renderHeroPanel(rightColW)
	gamePanelH := lipgloss.Height(gamePanel)
	heroPanelH := lipgloss.Height(heroPanel)

	// Store divider X position for drag detection.
	m.dividerX = colW + 4

	// Use the taller header to compute row2StartY (for vertical divider detection).
	m.row2StartY = gamePanelH
	if heroPanelH > gamePanelH {
		m.row2StartY = heroPanelH
	}

	// ── Per-column viewport height budgets ──
	sessionH := 3
	helpH := 1
	borderOverhead := 3 // border top(1) + title line(1) + border bottom(1)

	leftAvailable := m.height - gamePanelH - sessionH - helpH - borderOverhead
	rightAvailable := m.height - heroPanelH - sessionH - helpH - borderOverhead
	if leftAvailable < 8 {
		leftAvailable = 8
	}
	if rightAvailable < 8 {
		rightAvailable = 8
	}

	hSplit := m.hSplit
	if hSplit <= 0 {
		hSplit = 0.7
	}

	var leftMainH, leftPartnerH, rightMainH, rightPartnerH int
	if m.game != nil && m.game.IsDuos {
		partnerBorderH := 3 // border overhead for partner panel
		leftMainH = int(hSplit * float64(leftAvailable - partnerBorderH))
		leftPartnerH = leftAvailable - leftMainH - partnerBorderH
		rightMainH = int(hSplit * float64(rightAvailable - partnerBorderH))
		rightPartnerH = rightAvailable - rightMainH - partnerBorderH
		const minH = 4
		if leftMainH < minH {
			leftMainH = minH
			leftPartnerH = leftAvailable - leftMainH - partnerBorderH
		}
		if leftPartnerH < minH {
			leftPartnerH = minH
			leftMainH = leftAvailable - leftPartnerH - partnerBorderH
		}
		if rightMainH < minH {
			rightMainH = minH
			rightPartnerH = rightAvailable - rightMainH - partnerBorderH
		}
		if rightPartnerH < minH {
			rightPartnerH = minH
			rightMainH = rightAvailable - rightPartnerH - partnerBorderH
		}
	} else {
		leftMainH = leftAvailable
		rightMainH = rightAvailable
	}

	// Scrollbar column X positions (absolute terminal coordinates).
	m.boardScrollX = 2 + vpContentW
	m.modsScrollX = (colW + 4) + 2 + rightVPW

	// ── Board panel (left column) ──
	boardTitle := "YOUR BOARD"
	if m.game != nil && m.game.Phase == "GAME_OVER" {
		boardTitle = "FINAL BOARD"
	}
	m.boardVP.Width = vpContentW
	m.boardVP.Height = leftMainH
	m.boardVP.MouseWheelEnabled = true
	m.boardVP.SetContent(m.boardItems())
	m.boardVPY = gamePanelH + 2 // border(1) + title(1)
	m.boardVPH = leftMainH
	boardVPView := lipgloss.JoinHorizontal(lipgloss.Top,
		m.boardVP.View(), tuiScrollbar(m.boardVP, leftMainH))
	boardPanel := styleBorder.Width(colW).Render(
		styleTitle.Render(boardTitle) + "\n" + boardVPView)

	// ── Buff sources panel (right column) ──
	m.modsVP.Width = rightVPW
	m.modsVP.Height = rightMainH
	m.modsVP.MouseWheelEnabled = true
	m.modsVP.SetContent(m.modsItems())
	m.modsVPY = heroPanelH + 2 // border(1) + title(1)
	m.modsVPH = rightMainH
	modsVPView := lipgloss.JoinHorizontal(lipgloss.Top,
		m.modsVP.View(), tuiScrollbar(m.modsVP, rightMainH))
	modsPanel := styleBorder.Width(rightColW).Render(
		styleTitle.Render("BUFF SOURCES") + "\n" + modsVPView)

	// ── Build columns ──
	leftPanels := []string{gamePanel, boardPanel}
	rightPanels := []string{heroPanel, modsPanel}

	// ── Partner panels (Duos) ──
	if m.game != nil && m.game.IsDuos {
		// Partner board (left column).
		m.partnerBoardVP.Width = vpContentW
		m.partnerBoardVP.Height = leftPartnerH
		m.partnerBoardVP.MouseWheelEnabled = true
		m.partnerBoardVP.SetContent(m.partnerBoardItems())

		title := "PARTNER BOARD"
		if len(m.game.PartnerBoard) > 0 {
			if m.game.PartnerBoardStale {
				title = fmt.Sprintf("PARTNER BOARD (T%d — last seen)", m.game.PartnerBoardTurn)
			} else {
				title = fmt.Sprintf("PARTNER BOARD (T%d)", m.game.PartnerBoardTurn)
			}
		}

		m.partnerVPY = gamePanelH + lipgloss.Height(boardPanel) + 2
		m.partnerVPH = leftPartnerH
		m.partnerScrollX = 2 + vpContentW
		partnerBoardVPView := lipgloss.JoinHorizontal(lipgloss.Top,
			m.partnerBoardVP.View(), tuiScrollbar(m.partnerBoardVP, leftPartnerH))
		partnerBoardPanel := styleBorder.Width(colW).Render(
			styleTitle.Render(title) + "\n" + partnerBoardVPView)

		// Partner buffs (right column).
		m.partnerModsVP.Width = rightVPW
		m.partnerModsVP.Height = rightPartnerH
		m.partnerModsVP.MouseWheelEnabled = true
		m.partnerModsVP.SetContent(m.partnerModsItems())

		partnerModsTitle := "PARTNER BUFFS"
		if len(m.game.PartnerBuffSources) > 0 && m.game.PartnerBoardStale {
			partnerModsTitle = "PARTNER BUFFS (last seen)"
		}

		m.partnerModsVPY = heroPanelH + lipgloss.Height(modsPanel) + 2
		m.partnerModsVPH = rightPartnerH
		m.partnerModsScrollX = (colW + 4) + 2 + rightVPW
		partnerModsVPView := lipgloss.JoinHorizontal(lipgloss.Top,
			m.partnerModsVP.View(), tuiScrollbar(m.partnerModsVP, rightPartnerH))
		partnerModsPanel := styleBorder.Width(rightColW).Render(
			styleTitle.Render(partnerModsTitle) + "\n" + partnerModsVPView)

		leftPanels = append(leftPanels, partnerBoardPanel)
		rightPanels = append(rightPanels, partnerModsPanel)

		// Horizontal divider Y: use max of left/right main panel bottoms.
		leftDivY := gamePanelH + lipgloss.Height(boardPanel)
		rightDivY := heroPanelH + lipgloss.Height(modsPanel)
		m.dividerY = leftDivY
		if rightDivY > leftDivY {
			m.dividerY = rightDivY
		}
	}

	leftCol := lipgloss.JoinVertical(lipgloss.Left, leftPanels...)
	rightCol := lipgloss.JoinVertical(lipgloss.Left, rightPanels...)
	columns := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)

	// ── Session stats ──
	rowSession := m.renderSessionBar(m.width - 4)

	// ── Help bar ──
	helpText := "  [r] Refresh game  [R] Refresh stats  [d] Anomaly desc  [l] Last result  [q] Quit  scroll: mouse wheel"
	if m.game != nil && m.game.IsDuos {
		helpText = "  [r] Refresh  [R] Stats  [d] Anomaly desc  [l] Last result  [q] Quit  scroll: mouse wheel"
	}
	help := styleHelp.Render(helpText)

	return lipgloss.JoinVertical(lipgloss.Left, columns, rowSession, help)
```

**Step 2: Run tests**

Run: `go test -count=1 ./internal/tui/`
Expected: All tests pass. `TestView_FitsWithinHeight` may need height adjustment if the column layout is more compact — check output.

**Step 3: Run vet**

Run: `go vet ./internal/tui/`
Expected: No errors.

**Step 4: Visual verification via dump**

Run: `go build ./cmd/battlestream && ./battlestream tui --dump --width 120`
Expected: Game Info panel (left) is shorter than Hero panel (right). Board panel starts immediately below Game Info — NOT aligned with where the Hero panel ends.

**Step 5: Commit**

```bash
git add internal/tui/tui.go
git commit -m "feat: switch TUI to column-based layout for tighter vertical packing

Each column stacks its panels independently so the board panel starts
right below the game info panel, not padded to match the taller hero
panel on the right."
```

---

### Task 3: Update hSplit drag to work with per-column heights

**Files:**
- Modify: `internal/tui/tui.go` (handleMouse hSplit drag section)

**Step 1: Update horizontal drag ratio computation**

The hSplit drag in `handleMouse` currently uses `m.row2StartY` as the top of the drag region. With column-based layout, the drag should use the larger of the two header panel heights. Since `m.row2StartY` is already set to `max(gamePanelH, heroPanelH)` in Task 2, this should work correctly.

Verify by reviewing the drag code — no code change needed if `row2StartY` is set correctly. Mark as verified.

**Step 2: Run full test suite**

Run: `go test -count=1 ./internal/tui/`
Expected: All tests pass.

**Step 3: Visual test of drag behavior**

Run the live TUI and verify:
1. Vertical divider drag (left-right) works
2. Horizontal divider drag (board-partner split) works in duos
3. Mouse wheel scroll works on all panels
4. Scrollbar drag-scrubbing works

---

### Task 4: Remove stale row2StartY field if no longer needed

**Files:**
- Modify: `internal/tui/tui.go`

**Step 1: Audit row2StartY usage**

`row2StartY` is used in:
1. `View()` — set to max header height (still used for height budget in hSplit drag)
2. `handleMouse()` — vertical divider detection (`y >= m.row2StartY`)
3. `handleMouse()` — hSplit drag calculation (`m.height - m.row2StartY - ...`)

It's still needed for divider detection. Keep it but rename the comment to clarify its new meaning: "Y position where viewport panels begin (max of header panel heights)."

In the Model struct, update the comment on line 138:

```go
	// Panel positions (updated each View frame) for mouse routing.
	// row2StartY: Y where viewport panels begin (max of left/right header heights).
	row2StartY int
```

**Step 2: Run tests and vet**

Run: `go test -count=1 ./internal/tui/ && go vet ./internal/tui/`
Expected: All pass.

**Step 3: Commit**

```bash
git add internal/tui/tui.go
git commit -m "refactor: clarify row2StartY comment for column-based layout"
```

---

### Task 5: Final validation

**Step 1: Run full test suite with race detector**

Run: `go test -race -count=1 ./...`
Expected: All tests pass.

**Step 2: Run linter**

Run: `go vet ./...`
Expected: No errors.

**Step 3: Visual comparison**

Run: `go build ./cmd/battlestream && ./battlestream tui --dump --width 120`

Verify:
- Game Info panel is compact (no wasted vertical space)
- Board panel starts immediately below Game Info
- Hero panel can be taller without affecting left column
- Buff Sources starts immediately below Hero panel
- Session bar and help bar are full-width at bottom
- All panel titles present: BATTLESTREAM, player name, YOUR BOARD/FINAL BOARD, BUFF SOURCES, SESSION
- If duos: PARTNER BOARD and PARTNER BUFFS panels present in their respective columns

**Step 4: Final commit if any fixups needed**

Only if tests or visual check revealed issues to fix.
