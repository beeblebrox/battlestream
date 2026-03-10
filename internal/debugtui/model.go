package debugtui

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/parser"
)

// replayLoadedMsg is sent when async loading completes.
type replayLoadedMsg struct {
	replay *Replay
	err    error
}

// loadProgressMsg is sent periodically during loading to update the display.
type loadProgressMsg struct{}

// loadProgress tracks parsing progress from the background goroutine.
type loadProgress struct {
	linesRead  atomic.Int64
	gamesFound atomic.Int32
	fileIdx    atomic.Int32
}

// Model is the Bubbletea model for the debug replay TUI.
type Model struct {
	replay *Replay

	// Loading state.
	loading  bool
	paths    []string
	loadErr  error
	progress *loadProgress

	// Game picker state.
	picking    bool
	gameCursor int

	// Step view state (for the selected game).
	gameIdx   int    // index into replay.Games
	steps     []Step // slice of replay.Steps for selected game
	filtered  []int  // indices into steps after applying filter
	cursor    int    // position in filtered slice
	width     int
	height    int
	inputMode bool        // true while jump-to-turn input is active
	jumpInput textinput.Model // bubbles/textinput for jump-to-turn
	filter    int // index into eventTypeNames; 0 = ALL

	// bubbles/spinner for the async loading screen.
	loadSpinner spinner.Model

	// Viewport components (bubbles/viewport) for scrollable panels.
	// rawVP handles the raw log at the bottom (j/k + mouse wheel).
	// boardVP / buffVP / changesVP handle the middle panels (J/K, synced).
	rawVP     viewport.Model
	boardVP   viewport.Model
	buffVP    viewport.Model
	changesVP viewport.Model

	// Panel positions (updated each View frame) for mouse routing and scrubbing.
	row2StartY int
	row3StartY int
	rawStartY  int
	halfWBound int // X boundary between left and right half panels

	// Per-panel scrollbar column X and viewport Y/height.
	boardScrollX, boardVPY, boardVPH     int
	buffScrollX, buffVPY, buffVPH        int
	changesScrollX, changesVPY, changesVPH int
	rawScrollX, rawVPY, rawVPH           int

	// Drag-scrubbing state.
	scrubbing  bool
	scrubPanel int // 0=board 1=buff 2=changes 3=raw 4=partnerBoard 5=partnerBuff
	scrubTrackY int
	scrubTrackH int

	// Duos partner panel state.
	showPartner bool
	partnerBoardVP viewport.Model
	partnerBuffVP  viewport.Model
	partnerBoardScrollX, partnerBoardVPY, partnerBoardVPH int
	partnerBuffScrollX, partnerBuffVPY, partnerBuffVPH     int
}

func newJumpInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "turn #"
	ti.CharLimit = 6
	ti.Validate = func(s string) error {
		for _, c := range s {
			if c < '0' || c > '9' {
				return fmt.Errorf("digits only")
			}
		}
		return nil
	}
	return ti
}

func newLoadSpinner() spinner.Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorPurple)
	return sp
}

// New creates a debug TUI Model that loads replay data asynchronously.
func New(paths []string) *Model {
	return &Model{
		loading:     true,
		paths:       paths,
		progress:    &loadProgress{},
		jumpInput:   newJumpInput(),
		loadSpinner: newLoadSpinner(),
	}
}

// NewFromReplay creates a debug TUI Model from a pre-loaded replay (for tests).
func NewFromReplay(replay *Replay) *Model {
	m := &Model{
		replay:      replay,
		picking:     len(replay.Games) > 1,
		jumpInput:   newJumpInput(),
		loadSpinner: newLoadSpinner(),
	}
	if len(replay.Games) == 1 {
		m.selectGame(0)
	}
	return m
}

// Run starts the Bubbletea program.
func (m *Model) Run() error {
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

func (m *Model) selectGame(idx int) {
	if idx < 0 || idx >= len(m.replay.Games) {
		return
	}
	g := m.replay.Games[idx]
	m.gameIdx = idx
	m.steps = m.replay.Steps[g.StepStart:g.StepEnd]
	m.cursor = 0
	m.filter = 0
	m.picking = false
	m.rebuildFiltered()
	m.resetScrollAll()
	// Auto-detect Duos for partner panel display.
	m.showPartner = g.IsDuos
}

// resetScrollAll resets all viewport scroll positions to the top.
func (m *Model) resetScrollAll() {
	m.rawVP.GotoTop()
	m.boardVP.GotoTop()
	m.buffVP.GotoTop()
	m.changesVP.GotoTop()
	m.partnerBoardVP.GotoTop()
	m.partnerBuffVP.GotoTop()
}

// syncPanelScroll copies boardVP's YOffset to buffVP and changesVP so all
// three middle panels scroll in lockstep.
func (m *Model) syncPanelScroll() {
	m.buffVP.YOffset = m.boardVP.YOffset
	m.changesVP.YOffset = m.boardVP.YOffset
}

func (m *Model) rebuildFiltered() {
	m.filtered = m.filtered[:0]
	if m.filter == 0 {
		for i := range m.steps {
			m.filtered = append(m.filtered, i)
		}
	} else {
		target := parser.EventType(eventTypeNames[m.filter])
		for i, s := range m.steps {
			if s.Event.Type == target {
				m.filtered = append(m.filtered, i)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) currentStep() *Step {
	if len(m.filtered) == 0 {
		return nil
	}
	return &m.steps[m.filtered[m.cursor]]
}

func (m *Model) Init() tea.Cmd {
	if m.loading {
		return tea.Batch(
			loadReplayCmd(m.paths, m.progress),
			tickProgressCmd(),
			m.loadSpinner.Tick,
		)
	}
	return nil
}

func tickProgressCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return loadProgressMsg{}
	})
}

func loadReplayCmd(paths []string, prog *loadProgress) tea.Cmd {
	return func() tea.Msg {
		old := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		defer slog.SetDefault(old)

		replay, err := LoadAllGamesWithProgress(paths, prog)
		return replayLoadedMsg{replay: replay, err: err}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.loadSpinner, cmd = m.loadSpinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case loadProgressMsg:
		if m.loading {
			return m, tickProgressCmd()
		}
		return m, nil

	case replayLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.loadErr = msg.err
			return m, nil
		}
		m.replay = msg.replay
		if len(m.replay.Games) == 0 {
			m.loadErr = fmt.Errorf("no games found in %d log file(s)", len(m.paths))
			return m, nil
		}
		m.picking = len(m.replay.Games) > 1
		if len(m.replay.Games) == 1 {
			m.selectGame(0)
		}
		return m, nil

	case tea.MouseMsg:
		if !m.loading && !m.picking && m.loadErr == nil {
			return m.handleMouse(msg)
		}

	case tea.KeyMsg:
		if m.loading {
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.loadErr != nil {
			return m, tea.Quit
		}
		if m.picking {
			return m.handlePickerMode(msg)
		}
		if m.inputMode {
			return m.handleInputMode(msg)
		}
		return m.handleNormalMode(msg)
	}
	// Forward non-key messages to jumpInput when active (cursor blink etc.).
	if m.inputMode {
		var cmd tea.Cmd
		m.jumpInput, cmd = m.jumpInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ── Game picker ──────────────────────────────────────────────────

func (m *Model) handlePickerMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		if m.gameCursor < len(m.replay.Games)-1 {
			m.gameCursor++
		}
	case "k", "up":
		if m.gameCursor > 0 {
			m.gameCursor--
		}
	case "g":
		m.gameCursor = 0
	case "G":
		m.gameCursor = len(m.replay.Games) - 1
	case "enter":
		m.selectGame(m.gameCursor)
	}
	return m, nil
}

// ── Step input mode ──────────────────────────────────────────────

func (m *Model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.inputMode = false
		if turn, err := strconv.Atoi(m.jumpInput.Value()); err == nil {
			m.jumpToTurn(turn)
		}
		m.jumpInput.SetValue("")
		m.jumpInput.Blur()
	case "esc":
		m.inputMode = false
		m.jumpInput.SetValue("")
		m.jumpInput.Blur()
	default:
		var cmd tea.Cmd
		m.jumpInput, cmd = m.jumpInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ── Step normal mode ─────────────────────────────────────────────

func (m *Model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "s":
		if len(m.replay.Games) > 1 {
			m.picking = true
		}
	case "l", "right", "n":
		m.moveCursor(1)
	case "h", "left", "p":
		m.moveCursor(-1)
	case "]":
		m.nextTurn()
	case "[":
		m.prevTurn()
	case "}":
		m.nextPhase()
	case "{":
		m.prevPhase()
	case "g":
		m.cursor = 0
		m.resetScrollAll()
	case "G":
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
		m.resetScrollAll()
	case "t":
		m.inputMode = true
		m.jumpInput.SetValue("")
		m.jumpInput.Focus()
	case "j", "down":
		m.rawVP.ScrollDown(1)
	case "k", "up":
		m.rawVP.ScrollUp(1)
	case "J":
		m.boardVP.ScrollDown(1)
		m.syncPanelScroll()
	case "K":
		m.boardVP.ScrollUp(1)
		m.syncPanelScroll()
	case "d":
		if s := m.currentStep(); s != nil && s.State.IsDuos {
			m.showPartner = !m.showPartner
		}
	case "f":
		m.filter = (m.filter + 1) % len(eventTypeNames)
		m.rebuildFiltered()
		m.rawVP.GotoTop()
	}
	return m, nil
}

// ── Mouse handling ───────────────────────────────────────────────

func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Wheel events: route to the panel under the cursor.
	if tea.MouseEvent(msg).IsWheel() {
		var cmd tea.Cmd
		switch {
		case msg.Y >= m.rawStartY:
			m.rawVP, cmd = m.rawVP.Update(msg)
		case msg.Y >= m.row3StartY && msg.X < m.halfWBound:
			m.buffVP, cmd = m.buffVP.Update(msg)
		case msg.Y >= m.row3StartY:
			m.changesVP, cmd = m.changesVP.Update(msg)
		case msg.Y >= m.row2StartY && msg.X >= m.halfWBound:
			m.boardVP, cmd = m.boardVP.Update(msg)
		}
		return m, cmd
	}

	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button == tea.MouseButtonLeft {
			panel, trackY, trackH := m.identifyScrollbar(msg.X, msg.Y)
			if panel >= 0 {
				m.scrubbing = true
				m.scrubPanel = panel
				m.scrubTrackY = trackY
				m.scrubTrackH = trackH
				m.scrubAt(msg.Y)
			}
		}
	case tea.MouseActionMotion:
		if m.scrubbing && msg.Button == tea.MouseButtonLeft {
			m.scrubAt(msg.Y)
		}
	case tea.MouseActionRelease:
		m.scrubbing = false
	}
	return m, nil
}

// identifyScrollbar returns the panel index and track info if (x,y) falls on a scrollbar.
// Returns panel=-1 if not on a scrollbar.
func (m *Model) identifyScrollbar(x, y int) (panel, trackY, trackH int) {
	switch {
	case x == m.boardScrollX && y >= m.boardVPY && y < m.boardVPY+m.boardVPH:
		return 0, m.boardVPY, m.boardVPH
	case x == m.buffScrollX && y >= m.buffVPY && y < m.buffVPY+m.buffVPH:
		return 1, m.buffVPY, m.buffVPH
	case x == m.changesScrollX && y >= m.changesVPY && y < m.changesVPY+m.changesVPH:
		return 2, m.changesVPY, m.changesVPH
	case x == m.rawScrollX && y >= m.rawVPY && y < m.rawVPY+m.rawVPH:
		return 3, m.rawVPY, m.rawVPH
	}
	return -1, 0, 0
}

func (m *Model) vpForPanel(panel int) *viewport.Model {
	switch panel {
	case 0:
		return &m.boardVP
	case 1:
		return &m.buffVP
	case 2:
		return &m.changesVP
	case 3:
		return &m.rawVP
	}
	return nil
}

func (m *Model) scrubAt(y int) {
	if vp := m.vpForPanel(m.scrubPanel); vp != nil {
		scrollbarJump(vp, y, m.scrubTrackY, m.scrubTrackH)
	}
}

// scrollbarJump moves vp's scroll position to match a click at (clickY) in a
// scrollbar track that spans trackY..trackY+trackH-1.
func scrollbarJump(vp *viewport.Model, clickY, trackY, trackH int) {
	relY := clickY - trackY
	if relY < 0 || relY >= trackH || trackH <= 1 {
		return
	}
	maxOffset := vp.TotalLineCount() - trackH
	if maxOffset <= 0 {
		return
	}
	pct := float64(relY) / float64(trackH-1)
	target := int(pct * float64(maxOffset))
	if target < 0 {
		target = 0
	}
	if target > maxOffset {
		target = maxOffset
	}
	vp.YOffset = target
}

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	m.resetScrollAll()
}

func (m *Model) nextTurn() {
	if s := m.currentStep(); s != nil {
		currentTurn := s.Turn
		for i := m.cursor + 1; i < len(m.filtered); i++ {
			if m.steps[m.filtered[i]].Turn > currentTurn {
				m.cursor = i
				m.resetScrollAll()
				return
			}
		}
	}
}

func (m *Model) prevTurn() {
	if s := m.currentStep(); s != nil {
		currentTurn := s.Turn
		for i := m.cursor - 1; i >= 0; i-- {
			if m.steps[m.filtered[i]].Turn < currentTurn {
				targetTurn := m.steps[m.filtered[i]].Turn
				for j := i; j >= 0; j-- {
					if m.steps[m.filtered[j]].Turn < targetTurn {
						m.cursor = j + 1
						m.resetScrollAll()
						return
					}
				}
				m.cursor = 0
				m.resetScrollAll()
				return
			}
		}
	}
}

func (m *Model) nextPhase() {
	if s := m.currentStep(); s != nil {
		currentPhase := s.State.Phase
		for i := m.cursor + 1; i < len(m.filtered); i++ {
			if m.steps[m.filtered[i]].State.Phase != currentPhase {
				m.cursor = i
				m.resetScrollAll()
				return
			}
		}
	}
}

func (m *Model) prevPhase() {
	if s := m.currentStep(); s != nil {
		currentPhase := s.State.Phase
		for i := m.cursor - 1; i >= 0; i-- {
			if m.steps[m.filtered[i]].State.Phase != currentPhase {
				// Found different phase; now find its start.
				targetPhase := m.steps[m.filtered[i]].State.Phase
				for j := i; j >= 0; j-- {
					if m.steps[m.filtered[j]].State.Phase != targetPhase {
						m.cursor = j + 1
						m.resetScrollAll()
						return
					}
				}
				m.cursor = 0
				m.resetScrollAll()
				return
			}
		}
	}
}

func (m *Model) jumpToTurn(turn int) {
	// Land on the last step of the requested turn so the caller sees
	// end-of-turn state (after end-of-turn effects) rather than the boundary.
	last := -1
	for i, idx := range m.filtered {
		if m.steps[idx].Turn == turn {
			last = i
		} else if last >= 0 {
			break // moved past this turn
		}
	}
	if last >= 0 {
		m.cursor = last
		m.resetScrollAll()
		return
	}
	// Fallback: first step with Turn >= turn.
	for i, idx := range m.filtered {
		if m.steps[idx].Turn >= turn {
			m.cursor = i
			m.resetScrollAll()
			return
		}
	}
}

// ── View ─────────────────────────────────────────────────────────

func (m *Model) View() string {
	if m.width == 0 {
		return ""
	}

	if m.loading {
		lines := m.progress.linesRead.Load()
		games := m.progress.gamesFound.Load()
		fileIdx := m.progress.fileIdx.Load()
		var b strings.Builder
		b.WriteString("\n")
		b.WriteString(m.loadSpinner.View() + " " + styleTitle.Render("Loading...") + "\n\n")
		b.WriteString(fmt.Sprintf("  File %d/%d", fileIdx+1, len(m.paths)))
		if int(fileIdx) < len(m.paths) {
			b.WriteString(styleDim.Render(fmt.Sprintf("  %s", filepath.Base(m.paths[fileIdx]))))
		}
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Lines: %d    Games: %d\n", lines, games))
		b.WriteString("\n")
		b.WriteString(styleHelp.Render("  q: cancel"))
		return b.String()
	}

	if m.loadErr != nil {
		return fmt.Sprintf("\n  Error: %v\n\n  Press any key to exit.", m.loadErr)
	}

	if m.picking {
		return m.viewPicker()
	}
	return m.viewStep()
}

func (m *Model) viewPicker() string {
	innerW := m.width - 4

	var b strings.Builder
	b.WriteString(styleTitle.Render("SELECT GAME"))
	b.WriteString(styleDim.Render(fmt.Sprintf("  %d games found", len(m.replay.Games))))
	b.WriteString("\n\n")

	// Column headers.
	header := fmt.Sprintf("  %-4s %-20s %-30s %-6s %-6s %-6s %s",
		"#", "PLAYER", "HERO", "PLACE", "TURNS", "TIER", "SOURCE")
	b.WriteString(styleDim.Render(header) + "\n")

	// Visible window for scrolling.
	maxVisible := m.height - 7
	if maxVisible < 5 {
		maxVisible = 5
	}
	scrollStart := 0
	if m.gameCursor >= maxVisible {
		scrollStart = m.gameCursor - maxVisible + 1
	}
	scrollEnd := scrollStart + maxVisible
	if scrollEnd > len(m.replay.Games) {
		scrollEnd = len(m.replay.Games)
	}

	for i := scrollStart; i < scrollEnd; i++ {
		g := m.replay.Games[i]
		cursor := "  "
		lineStyle := styleValue
		if i == m.gameCursor {
			cursor = styleTitle.Render("> ")
			lineStyle = lipgloss.NewStyle().Foreground(colorGold)
		}

		name := g.PlayerName
		if name == "" {
			name = "(unknown)"
		}
		if len(name) > 20 {
			name = name[:19] + "…"
		}

		hero := gamestate.CardName(g.HeroCardID)
		if hero == "" {
			hero = "—"
		}
		if len(hero) > 30 {
			hero = hero[:29] + "…"
		}

		place := "—"
		if g.Placement > 0 {
			place = fmt.Sprintf("#%d", g.Placement)
		} else if g.Phase != "GAME_OVER" {
			place = string(g.Phase)
		}
		if g.IsDuos {
			place += " DUO"
		}

		source := filepath.Base(filepath.Dir(g.SourceFile))
		if source == "." || source == "/" {
			source = filepath.Base(g.SourceFile)
		}

		line := fmt.Sprintf("%-4d %-20s %-30s %-6s %-6d %-6d %s",
			i+1, name, hero, place, g.MaxTurn, g.TavernTier, source)
		b.WriteString(cursor + lineStyle.Render(line) + "\n")
	}

	if len(m.replay.Games) > maxVisible {
		b.WriteString(styleDim.Render(fmt.Sprintf("\n  showing %d-%d of %d",
			scrollStart+1, scrollEnd, len(m.replay.Games))))
	}

	help := styleHelp.Render("  j/k:navigate  Enter:select  q:quit")

	return styleBorder.Width(innerW).Render(b.String()) + "\n" + help
}

func (m *Model) viewStep() string {
	if len(m.steps) == 0 {
		return "No events found."
	}

	step := m.currentStep()
	if step == nil {
		return "No events match current filter."
	}

	innerW := m.width - 4
	// vpContentW: interior of each half-panel minus border(2)+padding(2)+scrollbar(1).
	vpContentW := innerW/2 - 7
	if vpContentW < 10 {
		vpContentW = 10
	}

	// Header
	header := m.renderHeader(step, innerW)
	m.row2StartY = lipgloss.Height(header)

	halfW := innerW/2 - 2
	m.halfWBound = halfW + 2 // halfW + left border char

	// Scrollbar column X positions (absolute terminal coordinates).
	// Left-half panels (buff): border(1)+padding(1)+vpContentW = 2+vpContentW.
	// Right-half panels (board/changes): left panel total(halfW+4) + border(1)+padding(1)+vpContentW.
	m.buffScrollX = 2 + vpContentW
	m.boardScrollX = halfW + 4 + 2 + vpContentW
	m.changesScrollX = m.boardScrollX
	m.rawScrollX = 2 + (innerW - 4 - 1) // border+padding + (contentW-1)

	// ── Row 2: Player + Board ───────────────────────────────────────
	// Render the player panel first to measure its actual height, since it
	// is not viewport-bounded and may vary with game state content.
	playerPanel := m.renderPlayerPanel(step, halfW)
	playerPanelH := lipgloss.Height(playerPanel)

	// Height budget for variable-height panels.
	// Row 2 is anchored to playerPanelH (the taller of the two halves).
	// Board viewport = playerPanelH - 3 (border 2 + title 1).
	// Row 3 and raw panel share the remaining budget.
	// Fixed: header + row2(playerPanelH) + event(3) + rawMin(6) + help(1).
	headerH := lipgloss.Height(header)
	boardVPH := playerPanelH - 3 // border(2) + title(1)
	if boardVPH < 1 {
		boardVPH = 1
	}
	showPartnerRow := step.State.IsDuos && m.showPartner
	fixedH := headerH + playerPanelH + 3 + 6 + 1 // header + row2 + event + rawPanelMin + help
	rowOverhead := 3                               // row3: border(2) + title(1)
	if showPartnerRow {
		rowOverhead += 3 // row4: border(2) + title(1)
	}
	contentBudget := m.height - fixedH - rowOverhead
	if contentBudget < 4 {
		contentBudget = 4
	}
	contentDivisor := 2
	if showPartnerRow {
		contentDivisor = 3
	}
	maxContentH := contentBudget / contentDivisor
	// If partner panels would be too small, suppress them.
	if showPartnerRow && maxContentH < 2 {
		showPartnerRow = false
		rowOverhead -= 3
		contentBudget += 3
		maxContentH = contentBudget / 2
	}

	boardContent := renderBoard(step.State.Board)
	m.boardVP.Width = vpContentW
	m.boardVP.Height = boardVPH
	m.boardVP.MouseWheelEnabled = true
	m.boardVP.SetContent(boardContent)

	var boardHeader strings.Builder
	title := "BOARD"
	if step.State.Phase == "GAME_OVER" {
		title = "FINAL BOARD"
	}
	boardHeader.WriteString(styleTitle.Render(title))
	boardHeader.WriteString(styleDim.Render(fmt.Sprintf(" (%d)", len(step.State.Board))))
	// boardVPY: border(1) + header line(1) after row2 start.
	m.boardVPY = m.row2StartY + 2
	m.boardVPH = boardVPH
	boardVPView := lipgloss.JoinHorizontal(lipgloss.Top, m.boardVP.View(), renderScrollbar(m.boardVP, boardVPH))
	boardPanel := styleBorder.Width(halfW).Render(boardHeader.String() + "\n" + boardVPView)
	row2 := lipgloss.JoinHorizontal(lipgloss.Top, playerPanel, boardPanel)
	m.row3StartY = m.row2StartY + lipgloss.Height(row2)

	// ── Row 3: Buff sources + Changes ──────────────────────────────
	buffContent := renderBuffSources(step.State)
	m.buffVP.Width = vpContentW
	m.buffVP.Height = maxContentH
	m.buffVP.MouseWheelEnabled = true
	m.buffVP.SetContent(buffContent)
	// buffVPY / changesVPY: border(1) + header line(1) after row3 start.
	m.buffVPY = m.row3StartY + 2
	m.buffVPH = maxContentH
	m.changesVPY = m.row3StartY + 2
	m.changesVPH = maxContentH
	buffVPView := lipgloss.JoinHorizontal(lipgloss.Top, m.buffVP.View(), renderScrollbar(m.buffVP, maxContentH))
	buffPanel := styleBorder.Width(halfW).Render(styleTitle.Render("BUFF SOURCES") + "\n" + buffVPView)

	var prevState gamestate.BGGameState
	if m.cursor > 0 {
		prevIdx := m.filtered[m.cursor-1]
		prevState = m.steps[prevIdx].State
	}
	changes := computeChanges(prevState, step.State)
	changesContent := renderChanges(changes)
	m.changesVP.Width = vpContentW
	m.changesVP.Height = maxContentH
	m.changesVP.MouseWheelEnabled = true
	m.changesVP.SetContent(changesContent)
	changesVPView := lipgloss.JoinHorizontal(lipgloss.Top, m.changesVP.View(), renderScrollbar(m.changesVP, maxContentH))
	changesPanel := styleBorder.Width(halfW).Render(styleTitle.Render("CHANGES") + "\n" + changesVPView)
	row3 := lipgloss.JoinHorizontal(lipgloss.Top, buffPanel, changesPanel)

	// ── Row 4 (Duos): Partner info ──────────────────────────────────
	var row4 string
	if showPartnerRow {
		var pInfoStr strings.Builder
		partner := step.State.Partner
		if partner != nil {
			pInfoStr.WriteString(fmt.Sprintf("Name: %s\n", partner.Name))
			pInfoStr.WriteString(fmt.Sprintf("Hero: %s\n", partner.HeroCardID))
			pInfoStr.WriteString(fmt.Sprintf("Health: %d  Tier: %d  Triples: %d", partner.EffectiveHealth(), partner.TavernTier, partner.TripleCount))
		} else {
			pInfoStr.WriteString(styleDim.Render("(no partner data)"))
		}
		row4 = styleBorder.Width(innerW).Render(styleTitle.Render("PARTNER") + "\n" + pInfoStr.String())
	}

	// ── Event summary ───────────────────────────────────────────────
	eventLine := styleBorder.Width(innerW).Render(renderEvent(step.Event))

	// ── Raw log (viewport — height-bounded, mouse-scrollable) ───────
	usedHeight := lipgloss.Height(header) + lipgloss.Height(row2) +
		lipgloss.Height(row3) + lipgloss.Height(row4) + lipgloss.Height(eventLine)
	rawH := m.height - usedHeight - 4 // -4: raw panel border (2) + raw panel header (1) + help bar (1)
	if rawH < 3 {
		rawH = 3
	}
	m.rawStartY = m.height - rawH - 3 // border(2) + header(1)
	// rawVPY: border(1) + header line(1) after rawStartY.
	m.rawVPY = m.rawStartY + 2
	m.rawVPH = rawH
	rawPanel := m.renderRawLog(step, innerW, rawH)

	// Help bar
	help := m.renderHelpBar()

	parts := []string{header, row2, row3}
	if row4 != "" {
		parts = append(parts, row4)
	}
	parts = append(parts, eventLine, rawPanel, help)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *Model) renderHeader(step *Step, w int) string {
	var b strings.Builder
	title := "DEBUG REPLAY"
	if step.State.IsDuos {
		title = "DEBUG REPLAY [DUOS]"
	}
	b.WriteString(styleTitle.Render(title))

	// Game info.
	g := m.replay.Games[m.gameIdx]
	gameLabel := fmt.Sprintf("  Game %d/%d", m.gameIdx+1, len(m.replay.Games))
	if g.PlayerName != "" {
		gameLabel += fmt.Sprintf(" (%s)", g.PlayerName)
	}
	b.WriteString(styleDim.Render(gameLabel))
	b.WriteString("\n")

	pos := fmt.Sprintf("Step %d/%d", m.cursor+1, len(m.filtered))
	if m.filter != 0 {
		pos += fmt.Sprintf(" (filter: %s, %d total)", filterName(m.filter), len(m.steps))
	}
	b.WriteString(styleLabel.Render(pos))

	b.WriteString("  ")
	b.WriteString(styleLabel.Render("Turn "))
	b.WriteString(styleValue.Render(fmt.Sprintf("%d", step.Turn)))
	b.WriteString("  ")
	b.WriteString(styleLabel.Render("Phase "))
	b.WriteString(stylePhase.Render(string(step.State.Phase)))
	b.WriteString("  ")
	b.WriteString(styleLabel.Render("Tavern "))
	b.WriteString(renderTavernTier(step.State.TavernTier))

	if len(step.State.AvailableTribes) > 0 {
		b.WriteString("\n")
		b.WriteString(styleLabel.Render("Tribes "))
		b.WriteString(styleDim.Render(strings.Join(step.State.AvailableTribes, ", ")))
	}

	if m.inputMode {
		b.WriteString("\n")
		b.WriteString(styleInputBox.Render("Jump to turn: ") + m.jumpInput.View())
	}

	return styleBorder.Width(w).Render(b.String())
}

func (m *Model) renderPlayerPanel(step *Step, w int) string {
	var b strings.Builder
	p := step.State.Player
	name := p.Name
	if name == "" {
		name = "Unknown"
	}
	b.WriteString(styleTitle.Render(name) + "\n")
	maxHP := p.MaxHealth
	if maxHP <= 0 {
		maxHP = 30
	}
	effectiveHP := p.Health - p.Damage
	b.WriteString(styleLabel.Render("Health ") + renderHealthBar(effectiveHP, maxHP, 12) + "\n")
	if p.Armor > 0 {
		b.WriteString(styleLabel.Render("Armor  ") + styleValue.Render(fmt.Sprintf("%d", p.Armor)) + "\n")
	}
	b.WriteString(styleLabel.Render("Triples ") + styleValue.Render(fmt.Sprintf("%d", p.TripleCount)) + "\n")
	if p.HeroCardID != "" {
		b.WriteString(styleLabel.Render("Hero   ") + styleDim.Render(gamestate.CardName(p.HeroCardID)) + "\n")
	}
	if p.WinStreak > 0 {
		b.WriteString(styleLabel.Render("Last   ") + styleWin.Render(fmt.Sprintf("WIN (streak: %d)", p.WinStreak)) + "\n")
	} else if p.LossStreak > 0 {
		b.WriteString(styleLabel.Render("Last   ") + styleLoss.Render(fmt.Sprintf("LOSS (streak: %d)", p.LossStreak)) + "\n")
	}

	return styleBorder.Width(w).Render(b.String())
}

func (m *Model) renderRawLog(step *Step, w, h int) string {
	// Interior width: subtract border (2) + padding (2).
	contentW := w - 4
	if contentW < 20 {
		contentW = 20
	}

	// Wrap raw lines to content width and join into a single string for the viewport.
	var lines []string
	for _, line := range step.RawLines {
		lines = append(lines, wrapLine(line, contentW)...)
	}
	content := strings.Join(lines, "\n")

	// Configure and populate the viewport. SetContent preserves YOffset.
	// Reserve 1 char for the scrollbar column.
	m.rawVP.Width = contentW - 1
	m.rawVP.Height = h
	m.rawVP.MouseWheelEnabled = true
	m.rawVP.MouseWheelDelta = 3
	m.rawVP.SetContent(content)

	// Header line: title + line count.
	hdr := styleTitle.Render("RAW LOG")
	hdr += styleDim.Render(fmt.Sprintf("  %d lines", len(step.RawLines)))

	vpView := lipgloss.JoinHorizontal(lipgloss.Top, m.rawVP.View(), renderScrollbar(m.rawVP, h))
	return styleBorder.Width(w).Render(hdr + "\n" + vpView)
}

func (m *Model) renderHelpBar() string {
	if m.inputMode {
		return styleHelp.Render("  Type turn number, Enter to jump, Esc to cancel")
	}
	help := "  h/l:step  [/]:turn  {/}:phase  g/G:start/end  t:jump  j/k:raw log  J/K:panels  mouse:raw log  f:filter  q:quit"
	if s := m.currentStep(); s != nil && s.State.IsDuos {
		help += "  d:partner"
	}
	if len(m.replay.Games) > 1 {
		help += "  s:games"
	}
	return styleHelp.Render(help)
}
