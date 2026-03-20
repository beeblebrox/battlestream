// Package tui implements the Bubbletea TUI dashboard for battlestream.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	bspb "battlestream.fixates.io/internal/api/grpc/gen/battlestream/v1"
	"battlestream.fixates.io/internal/gamestate"
)

// ============================================================
// Styles
// ============================================================

var (
	colorPurple  = lipgloss.Color("63")
	colorGold    = lipgloss.Color("220")
	colorGreen   = lipgloss.Color("46")
	colorRed     = lipgloss.Color("196")
	colorOrange  = lipgloss.Color("214")
	colorDim     = lipgloss.Color("240")
	colorMod     = lipgloss.Color("213")
	colorHelp    = lipgloss.Color("241")

	// Buff category colors
	colorBloodgem  = lipgloss.Color("196") // red
	colorNomi      = lipgloss.Color("208") // orange
	colorLightfang = lipgloss.Color("226") // yellow
	colorWhelp     = lipgloss.Color("39")  // blue
	colorTavern    = lipgloss.Color("141") // purple
	colorUndead    = lipgloss.Color("34")  // dark green
	colorElemental = lipgloss.Color("202") // dark orange
	colorBeetle    = lipgloss.Color("178") // dark yellow
	colorRightmost = lipgloss.Color("105") // light purple
	colorVolumizer = lipgloss.Color("81")  // cyan
	colorGeneral   = lipgloss.Color("249") // gray
	colorMuted   = lipgloss.Color("244")
	colorHealthFg = lipgloss.Color("46")
	colorHealthBg = lipgloss.Color("22")
	colorBarBg   = lipgloss.Color("235")

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPurple).
			Padding(0, 1)

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorGold)

	styleLabel   = lipgloss.NewStyle().Foreground(colorMuted)
	styleValue   = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	styleDim     = lipgloss.NewStyle().Foreground(colorDim)
	styleMod     = lipgloss.NewStyle().Foreground(colorMod)
	styleHelp    = lipgloss.NewStyle().Foreground(colorHelp)
	styleWin     = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	styleLoss    = lipgloss.NewStyle().Foreground(colorRed)
	stylePhase   = lipgloss.NewStyle().Foreground(colorOrange).Bold(true)
	styleErr      = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	styleGameOver = lipgloss.NewStyle().Foreground(colorDim).Bold(true)

	styleHealthBar = lipgloss.NewStyle().
			Foreground(colorHealthFg).
			Background(colorHealthBg)
	styleHealthBarEmpty = lipgloss.NewStyle().
				Background(colorBarBg)
)

// ============================================================
// Connection state
// ============================================================

type connState int

const (
	stateConnecting connState = iota
	stateConnected
	stateDisconnected
)

// ============================================================
// Messages
// ============================================================

type connectedMsg struct {
	client  *Client
	game    *bspb.GameState
	agg     *bspb.AggregateStats
	eventCh <-chan *bspb.GameEvent
}

type gameUpdateMsg struct{ game *bspb.GameState }
type aggUpdateMsg struct{ agg *bspb.AggregateStats }
type eventMsg struct{ event *bspb.GameEvent }
type disconnectedMsg struct{ err error }
type reconnectMsg struct{}
type aggTickMsg struct{}
type gameTickMsg struct{}

// ============================================================
// Model
// ============================================================

// Model is the root Bubbletea model for the TUI dashboard.
type Model struct {
	grpcAddr string
	ctx      context.Context
	cancel   context.CancelFunc

	connState connState
	connErr   error
	client    *Client
	eventCh   <-chan *bspb.GameEvent

	spinner spinner.Model
	game    *bspb.GameState
	agg     *bspb.AggregateStats

	width  int
	height int

	// Scrollable panels for board and buff/mods content.
	boardVP viewport.Model
	modsVP  viewport.Model

	// Panel positions (updated each View frame) for mouse routing.
	row2StartY int

	// Per-panel scrollbar column X and viewport Y/height.
	boardScrollX, boardVPY, boardVPH int
	modsScrollX, modsVPY, modsVPH   int

	// Drag-scrubbing state.
	scrubbing   bool
	scrubPanel  int // 0=board, 1=mods
	scrubTrackY int
	scrubTrackH int

	// Toggle states.
	showAnomalyDesc bool // toggle anomaly description display
	showLastResult  bool // toggle last combat result display
}

// New creates a Model that will connect to the daemon at grpcAddr.
func New(grpcAddr string) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorPurple)
	return &Model{
		grpcAddr:       grpcAddr,
		ctx:            ctx,
		cancel:         cancel,
		connState:      stateConnecting,
		spinner:        sp,
		showLastResult: true,
	}
}

// Run starts the Bubbletea program.
func (m *Model) Run() error {
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

// Dump connects to the daemon, fetches current state, and returns the rendered
// view as a plain string. Useful for debugging without a TTY.
func Dump(grpcAddr string, width int) (string, error) {
	client, err := Dial(grpcAddr)
	if err != nil {
		return "", err
	}
	defer client.Close()

	ctx := context.Background()
	game, err := client.GetCurrentGame(ctx)
	if err != nil {
		return "", fmt.Errorf("fetching game: %w", err)
	}
	agg, err := client.GetAggregate(ctx)
	if err != nil {
		return "", fmt.Errorf("fetching aggregate: %w", err)
	}

	m := &Model{
		connState: stateConnected,
		game:      game,
		agg:       agg,
		width:     width,
		height:    40,
	}
	return m.View(), nil
}

// ============================================================
// Init
// ============================================================

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		connectCmd(m.ctx, m.grpcAddr),
	)
}

// ============================================================
// Update
// ============================================================

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.cancel()
			if m.client != nil {
				m.client.Close()
			}
			return m, tea.Quit
		case "r":
			if m.connState == stateConnected && m.client != nil {
				return m, fetchGameCmd(m.ctx, m.client)
			}
		case "R":
			if m.connState == stateConnected && m.client != nil {
				return m, fetchAggCmd(m.ctx, m.client)
			}
		case "d":
			if m.game != nil && m.game.AnomalyDescription != "" {
				m.showAnomalyDesc = !m.showAnomalyDesc
			}
		case "l":
			m.showLastResult = !m.showLastResult
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.MouseMsg:
		if m.connState == stateConnected {
			return m.handleMouse(msg)
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case connectedMsg:
		m.connState = stateConnected
		m.client = msg.client
		m.game = msg.game
		m.agg = msg.agg
		m.eventCh = msg.eventCh
		return m, tea.Batch(
			waitForEventCmd(m.eventCh),
			aggTickCmd(),
			gameTickCmd(),
		)

	case disconnectedMsg:
		m.connState = stateDisconnected
		m.connErr = msg.err
		if m.client != nil {
			m.client.Close()
			m.client = nil
		}
		m.eventCh = nil
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return reconnectMsg{}
		})

	case reconnectMsg:
		m.connState = stateConnecting
		return m, connectCmd(m.ctx, m.grpcAddr)

	case gameUpdateMsg:
		m.game = msg.game

	case aggUpdateMsg:
		m.agg = msg.agg

	case eventMsg:
		// Re-fetch full game state on any event, then wait for next event
		if m.client != nil {
			return m, tea.Batch(
				waitForEventCmd(m.eventCh),
				fetchGameCmd(m.ctx, m.client),
			)
		}

	case aggTickMsg:
		if m.client != nil {
			return m, tea.Batch(
				fetchAggCmd(m.ctx, m.client),
				tea.Tick(10*time.Second, func(t time.Time) tea.Msg { return aggTickMsg{} }),
			)
		}

	case gameTickMsg:
		if m.client != nil {
			return m, tea.Batch(
				fetchGameCmd(m.ctx, m.client),
				gameTickCmd(),
			)
		}
	}

	return m, nil
}

// ============================================================
// View
// ============================================================

func (m *Model) View() string {
	switch m.connState {
	case stateConnecting:
		return fmt.Sprintf("\n  %s Connecting to daemon at %s…\n",
			m.spinner.View(), m.grpcAddr)

	case stateDisconnected:
		return fmt.Sprintf("\n  %s  Lost connection to daemon: %v\n  Retrying…\n",
			styleErr.Render("✗"), m.connErr)
	}

	if m.width == 0 {
		return ""
	}

	// Column widths: two equal halves minus borders/padding.
	// styleBorder has Padding(0,1) so inner content area = colW - 4 (2 border + 2 padding).
	// vpContentW is the viewport width; the scrollbar takes 1 char, so vpContentW + 1 must
	// fit inside the inner area: vpContentW = colW - 5.
	colW := m.width/2 - 4
	vpContentW := colW - 5
	if vpContentW < 10 {
		vpContentW = 10
	}

	// ── Row 1: game header | hero stats ──────────────────────
	row1 := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderGamePanel(colW),
		m.renderHeroPanel(colW),
	)
	m.row2StartY = lipgloss.Height(row1)

	// Height budget: terminal minus row1, session bar (3), help (1), row2 border (2), row2 title (1).
	sessionH := 3
	available := m.height - m.row2StartY - sessionH - 1 - 3
	if available < 4 {
		available = 4
	}

	// Scrollbar column X positions (absolute terminal coordinates).
	m.boardScrollX = 2 + vpContentW                  // left panel: border+pad + vp
	m.modsScrollX = (colW + 4) + 2 + vpContentW      // right panel: left total + border+pad + vp

	// ── Row 2: board (viewport) | buff sources (viewport) ────
	boardTitle := "YOUR BOARD"
	if m.game != nil && m.game.Phase == "GAME_OVER" {
		boardTitle = "FINAL BOARD"
	}
	m.boardVP.Width = vpContentW
	m.boardVP.Height = available
	m.boardVP.MouseWheelEnabled = true
	m.boardVP.SetContent(m.boardItems())
	m.boardVPY = m.row2StartY + 2 // border(1) + title line(1)
	m.boardVPH = available
	boardVPView := lipgloss.JoinHorizontal(lipgloss.Top,
		m.boardVP.View(), tuiScrollbar(m.boardVP, available))
	boardPanel := styleBorder.Width(colW).Render(
		styleTitle.Render(boardTitle) + "\n" + boardVPView)

	m.modsVP.Width = vpContentW
	m.modsVP.Height = available
	m.modsVP.MouseWheelEnabled = true
	m.modsVP.SetContent(m.modsItems())
	m.modsVPY = m.row2StartY + 2
	m.modsVPH = available
	modsVPView := lipgloss.JoinHorizontal(lipgloss.Top,
		m.modsVP.View(), tuiScrollbar(m.modsVP, available))
	modsPanel := styleBorder.Width(colW).Render(
		styleTitle.Render("BUFF SOURCES") + "\n" + modsVPView)

	row2 := lipgloss.JoinHorizontal(lipgloss.Top, boardPanel, modsPanel)

	// ── Session stats ─────────────────────────────────────────
	rowSession := m.renderSessionBar(m.width - 4)

	// ── Help bar ──────────────────────────────────────────────
	helpText := "  [r] Refresh game  [R] Refresh stats  [d] Anomaly desc  [l] Last result  [q] Quit  scroll: mouse wheel"
	if m.game != nil && m.game.IsDuos {
		helpText = "  [r] Refresh  [R] Stats  [d] Anomaly desc  [l] Last result  [q] Quit  scroll: mouse wheel"
	}
	help := styleHelp.Render(helpText)

	return lipgloss.JoinVertical(lipgloss.Left, row1, row2, rowSession, help)
}

// ============================================================
// Panel renderers
// ============================================================

func (m *Model) renderGamePanel(w int) string {
	var b strings.Builder

	title := "BATTLESTREAM"
	if m.game != nil && m.game.IsDuos {
		title = "BATTLESTREAM [DUOS]"
	}
	b.WriteString(styleTitle.Render(title) + "\n")

	if m.game == nil {
		b.WriteString(styleDim.Render("waiting for game…") + "\n\n\n\n")
	} else {
		phase := m.game.Phase
		if phase == "" {
			phase = "IDLE"
		}
		b.WriteString(styleLabel.Render("Game   ") + styleValue.Render(m.game.GameId) + "\n")

		if phase == "GAME_OVER" {
			placement := int(m.game.Placement)
			if placement >= 1 && placement <= 4 {
				b.WriteString(styleLabel.Render("Result ") + styleWin.Render(fmt.Sprintf("WIN #%d", placement)) + "\n")
			} else if placement > 0 {
				b.WriteString(styleLabel.Render("Result ") + styleLoss.Render(fmt.Sprintf("LOSS #%d", placement)) + "\n")
			} else {
				b.WriteString(styleLabel.Render("Phase  ") + styleGameOver.Render("GAME OVER") + "\n")
			}
		} else {
			b.WriteString(styleLabel.Render("Phase  ") + stylePhase.Render(phase) + "\n")
		}

		b.WriteString(styleLabel.Render("Turn   ") + styleValue.Render(fmt.Sprintf("%d", m.game.Turn)) + "\n")
		b.WriteString(styleLabel.Render("Tavern ") + renderTavernTier(int(m.game.TavernTier)) + "\n")
		if len(m.game.AvailableTribes) > 0 {
			b.WriteString(styleLabel.Render("Tribes ") + styleDim.Render(strings.Join(m.game.AvailableTribes, ", ")) + "\n")
		}
		if m.game.AnomalyName != "" {
			label := "Anomaly"
			if m.game.AnomalyDescription != "" {
				label += " [d]"
			}
			b.WriteString(styleLabel.Render(label+" ") + styleValue.Render(m.game.AnomalyName) + "\n")
			if m.showAnomalyDesc && m.game.AnomalyDescription != "" {
				wrapped := lipgloss.NewStyle().Width(w - 10).Render(m.game.AnomalyDescription)
				b.WriteString("        " + styleDim.Render(wrapped) + "\n")
			}
		}
	}

	return styleBorder.Width(w).Render(b.String())
}

func (m *Model) renderHeroPanel(w int) string {
	var b strings.Builder

	if m.game == nil || m.game.Player == nil {
		b.WriteString(styleDim.Render("no player data") + "\n\n\n\n")
		return styleBorder.Width(w).Render(b.String())
	}

	p := m.game.Player
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
	healthLabel := "Health  "
	if m.game.IsDuos {
		healthLabel = "HP Team "
	}
	b.WriteString(styleLabel.Render(healthLabel) + renderHealthBar(effectiveHP, maxHP, 16) + "\n")
	armor := "—"
	if p.Armor > 0 {
		armor = fmt.Sprintf("%d", p.Armor)
	}
	b.WriteString(styleLabel.Render("Armor   ") + styleValue.Render(armor) + "\n")
	b.WriteString(styleLabel.Render("Triples ") + styleValue.Render(fmt.Sprintf("%d", p.TripleCount)) + "\n")
	b.WriteString(styleLabel.Render("Gold    ") + styleValue.Render(fmt.Sprintf("%d/%d", p.CurrentGold, p.MaxGold)) + "\n")
	if p.HeroCardId != "" {
		b.WriteString(styleLabel.Render("Hero    ") + styleValue.Render(gamestate.CardName(p.HeroCardId)) + "\n")
	}

	// Win/loss last round indicator (toggled via [l]).
	if m.showLastResult {
		if p.WinStreak > 0 {
			b.WriteString(styleLabel.Render("Last    ") + styleWin.Render(fmt.Sprintf("WIN (streak: %d)", p.WinStreak)) + "\n")
		} else if p.LossStreak > 0 {
			b.WriteString(styleLabel.Render("Last    ") + styleLoss.Render(fmt.Sprintf("LOSS (streak: %d)", p.LossStreak)) + "\n")
		}
	}

	// Partner section in Duos.
	if m.game.IsDuos && m.game.Partner != nil {
		partner := m.game.Partner
		b.WriteString(styleDim.Render("─ Partner ─") + "\n")
		if partner.Name != "" {
			b.WriteString(styleLabel.Render("Name    ") + styleValue.Render(partner.Name) + "\n")
		}
		if partner.HeroCardId != "" {
			b.WriteString(styleLabel.Render("Hero    ") + styleValue.Render(gamestate.CardName(partner.HeroCardId)) + "\n")
		}
		b.WriteString(styleLabel.Render("Tavern  ") + renderTavernTier(int(partner.TavernTier)) + "\n")
		b.WriteString(styleLabel.Render("Triples ") + styleValue.Render(fmt.Sprintf("%d", partner.TripleCount)) + "\n")
	}

	return styleBorder.Width(w).Render(b.String())
}

// boardItems returns the scrollable board content (no title).
func (m *Model) boardItems() string {
	var b strings.Builder
	if m.game == nil || len(m.game.Board) == 0 {
		b.WriteString(styleDim.Render("(empty)"))
	} else {
		for _, mn := range m.game.Board {
			b.WriteString(renderMinion(mn) + "\n")
		}
	}
	return b.String()
}

// modsItems returns the scrollable buff-sources content (no outer title).
func (m *Model) modsItems() string {
	var b strings.Builder
	if m.game == nil || len(m.game.BuffSources) == 0 {
		// Fall back to old modifications display if no buff sources tracked.
		if m.game != nil && len(m.game.Modifications) > 0 {
			for _, mod := range m.game.Modifications {
				sign := "+"
				if mod.Delta < 0 {
					sign = ""
				}
				line := fmt.Sprintf("T%-2d %s%d %-6s %s",
					mod.Turn, sign, mod.Delta, mod.Stat, mod.Target)
				b.WriteString(styleMod.Render(line) + "\n")
			}
		} else {
			b.WriteString(styleDim.Render("(none this game)"))
		}
		return b.String()
	}

	// Sort by total buff magnitude (largest first).
	sources := make([]*bspb.BuffSource, len(m.game.BuffSources))
	copy(sources, m.game.BuffSources)
	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			totalI := abs32(sources[i].Attack) + abs32(sources[i].Health)
			totalJ := abs32(sources[j].Attack) + abs32(sources[j].Health)
			if totalJ > totalI {
				sources[i], sources[j] = sources[j], sources[i]
			}
		}
	}

	for _, bs := range sources {
		if bs.Attack == 0 && bs.Health == 0 {
			continue
		}
		name := buffCategoryDisplayName(bs.Category)
		color := buffCategoryColor(bs.Category)
		style := lipgloss.NewStyle().Foreground(color)
		line := fmt.Sprintf("%-14s +%d/+%d", name, bs.Attack, bs.Health)
		b.WriteString(style.Render(line) + "\n")
	}

	// Ability counters (e.g. Spellcraft stacks)
	if m.game != nil && len(m.game.AbilityCounters) > 0 {
		b.WriteString("\n" + styleTitle.Render("ABILITIES") + "\n")
		for _, ac := range m.game.AbilityCounters {
			name := buffCategoryDisplayName(ac.Category)
			color := buffCategoryColor(ac.Category)
			style := lipgloss.NewStyle().Foreground(color)
			line := fmt.Sprintf("%-14s %s", name, ac.Display)
			b.WriteString(style.Render(line) + "\n")
		}
	}

	return b.String()
}

func buffCategoryDisplayName(cat string) string {
	if n, ok := gamestate.CategoryDisplayName[cat]; ok {
		return n
	}
	return cat
}

func buffCategoryColor(cat string) lipgloss.Color {
	colors := map[string]lipgloss.Color{
		"BLOODGEM":        colorBloodgem,
		"BLOODGEM_BARRAGE": colorBloodgem,
		"NOMI":            colorNomi,
		"ELEMENTAL":       colorElemental,
		"TAVERN_SPELL":    colorTavern,
		"WHELP":           colorWhelp,
		"BEETLE":          colorBeetle,
		"RIGHTMOST":       colorRightmost,
		"UNDEAD":          colorUndead,
		"VOLUMIZER":       colorVolumizer,
		"LIGHTFANG":       colorLightfang,
		"NOMI_ALL":        colorNomi,
		"NAGA_SPELLS":     colorTavern,
		"FREE_REFRESH":    colorGold,
		"GOLD_NEXT_TURN":  colorGold,
		"CONSUMED":        colorDim,
		"GENERAL":         colorGeneral,
	}
	if c, ok := colors[cat]; ok {
		return c
	}
	return colorMod
}

func abs32(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}

// ── Mouse handling ───────────────────────────────────────────────

func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Wheel: route to whichever panel the cursor is over.
	if tea.MouseEvent(msg).IsWheel() {
		var cmd tea.Cmd
		if msg.X >= m.width/2 {
			m.modsVP, cmd = m.modsVP.Update(msg)
		} else {
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

func (m *Model) identifyScrollbar(x, y int) (panel, trackY, trackH int) {
	switch {
	case x == m.boardScrollX && y >= m.boardVPY && y < m.boardVPY+m.boardVPH:
		return 0, m.boardVPY, m.boardVPH
	case x == m.modsScrollX && y >= m.modsVPY && y < m.modsVPY+m.modsVPH:
		return 1, m.modsVPY, m.modsVPH
	}
	return -1, 0, 0
}

func (m *Model) scrubAt(y int) {
	switch m.scrubPanel {
	case 0:
		tuiScrollbarJump(&m.boardVP, y, m.scrubTrackY, m.scrubTrackH)
	case 1:
		tuiScrollbarJump(&m.modsVP, y, m.scrubTrackY, m.scrubTrackH)
	}
}

func tuiScrollbarJump(vp *viewport.Model, clickY, trackY, trackH int) {
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

// tuiScrollbar renders a 1-char-wide vertical scrollbar for a viewport.
// Outputs exactly height lines joined by "\n". Blank column when no overflow.
func tuiScrollbar(vp viewport.Model, height int) string {
	if height <= 0 {
		return ""
	}
	if vp.TotalLineCount() <= height {
		return strings.Repeat(" \n", height-1) + " "
	}
	pct := vp.ScrollPercent()
	thumbPos := int(pct * float64(height-1))
	lines := make([]string, height)
	for i := 0; i < height; i++ {
		ch := "│"
		switch {
		case i == 0 && !vp.AtTop():
			ch = "▲"
		case i == height-1 && !vp.AtBottom():
			ch = "▼"
		case i == thumbPos:
			ch = "█"
		}
		lines[i] = styleDim.Render(ch)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderSessionBar(w int) string {
	var b strings.Builder

	if m.agg == nil {
		b.WriteString(styleDim.Render("Session stats loading…"))
	} else {
		a := m.agg
		wins := styleWin.Render(fmt.Sprintf("W: %d", a.Wins))
		losses := styleLoss.Render(fmt.Sprintf("L: %d", a.Losses))
		avg := styleValue.Render(fmt.Sprintf("%.1f", a.AvgPlacement))
		games := styleValue.Render(fmt.Sprintf("%d", a.GamesPlayed))

		b.WriteString(styleLabel.Render("SESSION  "))
		b.WriteString(wins + "  " + losses)
		b.WriteString(styleLabel.Render("  Avg ") + avg)
		b.WriteString(styleLabel.Render("  Games ") + games)

		if a.BestPlacement > 0 {
			b.WriteString(styleLabel.Render("  Best #") +
				styleWin.Render(fmt.Sprintf("%d", a.BestPlacement)))
		}
	}

	return styleBorder.Width(w).Render(b.String())
}

// ============================================================
// Rendering helpers
// ============================================================

func renderHealthBar(current, max int32, barWidth int) string {
	if max <= 0 {
		max = 40
	}
	pct := float64(current) / float64(max)
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(barWidth))
	empty := barWidth - filled

	bar := styleHealthBar.Render(strings.Repeat("█", filled)) +
		styleHealthBarEmpty.Render(strings.Repeat("░", empty))

	label := fmt.Sprintf(" %d/%d", current, max)
	color := colorGreen
	if pct < 0.25 {
		color = colorRed
	} else if pct < 0.5 {
		color = colorOrange
	}
	return bar + lipgloss.NewStyle().Foreground(color).Render(label)
}

func renderTavernTier(tier int) string {
	if tier <= 0 {
		return styleValue.Render("—")
	}
	stars := strings.Repeat("★", tier) + strings.Repeat("☆", 6-tier)
	color := tavernTierColor(tier)
	return lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("%d %s", tier, stars))
}

func tavernTierColor(tier int) lipgloss.Color {
	switch tier {
	case 1:
		return lipgloss.Color("244")
	case 2:
		return lipgloss.Color("33")
	case 3:
		return lipgloss.Color("34")
	case 4:
		return lipgloss.Color("214")
	case 5:
		return lipgloss.Color("202")
	case 6:
		return lipgloss.Color("196")
	default:
		return lipgloss.Color("255")
	}
}

func renderMinion(mn *bspb.MinionState) string {
	var sb strings.Builder

	name := mn.Name
	if name == "" {
		name = mn.CardId
	}
	if len(name) > 22 {
		name = name[:21] + "…"
	}
	sb.WriteString(styleValue.Render(fmt.Sprintf("%-22s", name)))
	sb.WriteString(styleLabel.Render(" "))

	// Attack / health
	stats := fmt.Sprintf("%d/%d", mn.Attack, mn.Health)
	sb.WriteString(lipgloss.NewStyle().Foreground(colorGold).Render(stats))

	// Buffs
	if mn.BuffAttack != 0 || mn.BuffHealth != 0 {
		buff := fmt.Sprintf(" (+%d/+%d)", mn.BuffAttack, mn.BuffHealth)
		sb.WriteString(styleWin.Render(buff))
	}

	// Tribe
	if mn.MinionType != "" && mn.MinionType != "INVALID" {
		sb.WriteString(styleDim.Render(fmt.Sprintf(" [%s]", strings.ToLower(mn.MinionType))))
	}

	// Enchantment count
	if len(mn.Enchantments) > 0 {
		sb.WriteString(styleDim.Render(fmt.Sprintf(" %d buffs", len(mn.Enchantments))))
	}

	return sb.String()
}

// ============================================================
// Commands
// ============================================================

func connectCmd(ctx context.Context, addr string) tea.Cmd {
	return func() tea.Msg {
		client, err := Dial(addr)
		if err != nil {
			return disconnectedMsg{err: err}
		}

		game, err := client.GetCurrentGame(ctx)
		if err != nil {
			client.Close()
			return disconnectedMsg{err: err}
		}

		agg, err := client.GetAggregate(ctx)
		if err != nil {
			client.Close()
			return disconnectedMsg{err: err}
		}

		eventCh, err := client.StreamEvents(ctx)
		if err != nil {
			client.Close()
			return disconnectedMsg{err: err}
		}

		return connectedMsg{
			client:  client,
			game:    game,
			agg:     agg,
			eventCh: eventCh,
		}
	}
}

func fetchGameCmd(ctx context.Context, client *Client) tea.Cmd {
	return func() tea.Msg {
		game, err := client.GetCurrentGame(ctx)
		if err != nil {
			return disconnectedMsg{err: err}
		}
		return gameUpdateMsg{game: game}
	}
}

func fetchAggCmd(ctx context.Context, client *Client) tea.Cmd {
	return func() tea.Msg {
		agg, err := client.GetAggregate(ctx)
		if err != nil {
			return nil // non-fatal
		}
		return aggUpdateMsg{agg: agg}
	}
}

// waitForEventCmd blocks until an event arrives on ch, then returns it as a tea.Msg.
func waitForEventCmd(ch <-chan *bspb.GameEvent) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return disconnectedMsg{err: fmt.Errorf("event stream closed")}
		}
		return eventMsg{event: e}
	}
}


func aggTickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return aggTickMsg{}
	})
}

func gameTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return gameTickMsg{}
	})
}
