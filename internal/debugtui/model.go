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
	logScroll int
	width     int
	height    int
	inputMode bool
	input     string
	filter    int // index into eventTypeNames; 0 = ALL
}

// New creates a debug TUI Model that loads replay data asynchronously.
func New(paths []string) *Model {
	return &Model{
		loading:  true,
		paths:    paths,
		progress: &loadProgress{},
	}
}

// NewFromReplay creates a debug TUI Model from a pre-loaded replay (for tests).
func NewFromReplay(replay *Replay) *Model {
	m := &Model{
		replay:  replay,
		picking: len(replay.Games) > 1,
	}
	if len(replay.Games) == 1 {
		m.selectGame(0)
	}
	return m
}

// Run starts the Bubbletea program.
func (m *Model) Run() error {
	p := tea.NewProgram(m, tea.WithAltScreen())
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
	m.logScroll = 0
	m.filter = 0
	m.picking = false
	m.rebuildFiltered()
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
		if turn, err := strconv.Atoi(m.input); err == nil {
			m.jumpToTurn(turn)
		}
		m.input = ""
	case "esc":
		m.inputMode = false
		m.input = ""
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= '0' && ch[0] <= '9' {
			m.input += ch
		}
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
		m.logScroll = 0
	case "G":
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
		m.logScroll = 0
	case "t":
		m.inputMode = true
		m.input = ""
	case "j", "down":
		m.logScroll++
	case "k", "up":
		if m.logScroll > 0 {
			m.logScroll--
		}
	case "f":
		m.filter = (m.filter + 1) % len(eventTypeNames)
		m.rebuildFiltered()
		m.logScroll = 0
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	m.logScroll = 0
}

func (m *Model) nextTurn() {
	if s := m.currentStep(); s != nil {
		currentTurn := s.Turn
		for i := m.cursor + 1; i < len(m.filtered); i++ {
			if m.steps[m.filtered[i]].Turn > currentTurn {
				m.cursor = i
				m.logScroll = 0
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
						m.logScroll = 0
						return
					}
				}
				m.cursor = 0
				m.logScroll = 0
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
				m.logScroll = 0
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
						m.logScroll = 0
						return
					}
				}
				m.cursor = 0
				m.logScroll = 0
				return
			}
		}
	}
}

func (m *Model) jumpToTurn(turn int) {
	for i, idx := range m.filtered {
		if m.steps[idx].Turn >= turn {
			m.cursor = i
			m.logScroll = 0
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
		b.WriteString(styleTitle.Render("  Loading...") + "\n\n")
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

		hero := g.HeroCardID
		if hero == "" {
			hero = "—"
		}
		// Strip common prefixes for readability.
		hero = strings.TrimPrefix(hero, "TB_BaconShop_HERO_")
		hero = strings.TrimPrefix(hero, "BG_")
		if len(hero) > 30 {
			hero = hero[:29] + "…"
		}

		place := "—"
		if g.Placement > 0 {
			place = fmt.Sprintf("#%d", g.Placement)
		} else if g.Phase != "GAME_OVER" {
			place = string(g.Phase)
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

	// Header
	header := m.renderHeader(step, innerW)

	// Row 2: Player + Board (side by side)
	halfW := innerW/2 - 2
	playerPanel := m.renderPlayerPanel(step, halfW)
	boardPanel := m.renderBoardPanel(step, halfW)
	row2 := lipgloss.JoinHorizontal(lipgloss.Top, playerPanel, boardPanel)

	// Row 3: Buff sources + Changes (side by side)
	buffPanel := styleBorder.Width(halfW).Render(renderBuffSources(step.State))
	var prevState gamestate.BGGameState
	if m.cursor > 0 {
		prevIdx := m.filtered[m.cursor-1]
		prevState = m.steps[prevIdx].State
	}
	changes := computeChanges(prevState, step.State)
	changesPanel := styleBorder.Width(halfW).Render(renderChanges(changes))
	row3 := lipgloss.JoinHorizontal(lipgloss.Top, buffPanel, changesPanel)

	// Event summary
	eventLine := styleBorder.Width(innerW).Render(renderEvent(step.Event))

	// Raw log (scrollable, takes remaining height)
	usedHeight := lipgloss.Height(header) + lipgloss.Height(row2) +
		lipgloss.Height(row3) + lipgloss.Height(eventLine) + 1
	rawPanel := m.renderRawLog(step, innerW, m.height-usedHeight-2)

	// Help bar
	help := m.renderHelpBar()

	return lipgloss.JoinVertical(lipgloss.Left, header, row2, row3, eventLine, rawPanel, help)
}

func (m *Model) renderHeader(step *Step, w int) string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("DEBUG REPLAY"))

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

	if m.inputMode {
		b.WriteString("\n")
		b.WriteString(styleInputBox.Render(fmt.Sprintf("Jump to turn: %s_", m.input)))
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
	b.WriteString(styleLabel.Render("Health ") + renderHealthBar(p.Health, 40, 12) + "\n")
	if p.Armor > 0 {
		b.WriteString(styleLabel.Render("Armor  ") + styleValue.Render(fmt.Sprintf("%d", p.Armor)) + "\n")
	}
	b.WriteString(styleLabel.Render("Triples ") + styleValue.Render(fmt.Sprintf("%d", p.TripleCount)) + "\n")
	if p.HeroCardID != "" {
		b.WriteString(styleLabel.Render("Hero   ") + styleDim.Render(p.HeroCardID) + "\n")
	}

	return styleBorder.Width(w).Render(b.String())
}

func (m *Model) renderBoardPanel(step *Step, w int) string {
	var b strings.Builder
	title := "BOARD"
	if step.State.Phase == "GAME_OVER" {
		title = "FINAL BOARD"
	}
	b.WriteString(styleTitle.Render(title))
	b.WriteString(styleDim.Render(fmt.Sprintf(" (%d)", len(step.State.Board))))
	b.WriteString("\n")
	b.WriteString(renderBoard(step.State.Board))

	return styleBorder.Width(w).Render(b.String())
}

func (m *Model) renderRawLog(step *Step, w, maxHeight int) string {
	if maxHeight < 3 {
		maxHeight = 3
	}

	// Wrap raw lines to fit panel width (accounting for border+padding).
	contentW := w - 2
	if contentW < 20 {
		contentW = 20
	}
	var wrapped []string
	for _, line := range step.RawLines {
		wrapped = append(wrapped, wrapLine(line, contentW)...)
	}

	total := len(wrapped)

	if m.logScroll > total-maxHeight {
		m.logScroll = total - maxHeight
	}
	if m.logScroll < 0 {
		m.logScroll = 0
	}

	end := m.logScroll + maxHeight
	if end > total {
		end = total
	}

	var b strings.Builder
	b.WriteString(styleTitle.Render("RAW LOG"))
	b.WriteString(styleDim.Render(fmt.Sprintf("  %d lines", len(step.RawLines))))
	if total > maxHeight {
		b.WriteString(styleDim.Render(fmt.Sprintf("  [%d-%d/%d]", m.logScroll+1, end, total)))
	}
	b.WriteString("\n")

	for _, line := range wrapped[m.logScroll:end] {
		b.WriteString(styleRawLine.Render(line) + "\n")
	}

	return styleBorder.Width(w).Render(b.String())
}

func (m *Model) renderHelpBar() string {
	if m.inputMode {
		return styleHelp.Render("  Type turn number, Enter to jump, Esc to cancel")
	}
	help := "  h/l:step  [/]:turn  {/}:phase  g/G:start/end  t:jump  j/k:scroll  f:filter  q:quit"
	if len(m.replay.Games) > 1 {
		help += "  s:games"
	}
	return styleHelp.Render(help)
}
