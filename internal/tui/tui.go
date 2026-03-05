// Package tui implements the Bubbletea TUI dashboard for battlestream.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	bspb "battlestream.fixates.io/internal/api/grpc/gen/battlestream/v1"
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
	styleErr     = lipgloss.NewStyle().Foreground(colorRed).Bold(true)

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
type aggTickMsg struct{}

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
}

// New creates a Model that will connect to the daemon at grpcAddr.
func New(grpcAddr string) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorPurple)
	return &Model{
		grpcAddr:  grpcAddr,
		ctx:       ctx,
		cancel:    cancel,
		connState: stateConnecting,
		spinner:   sp,
	}
}

// Run starts the Bubbletea program.
func (m *Model) Run() error {
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
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
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

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
		)

	case disconnectedMsg:
		m.connState = stateDisconnected
		m.connErr = msg.err
		// Retry after 3 seconds
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			m.connState = stateConnecting
			return nil
		})

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

	// Column widths: two equal halves minus borders/padding
	colW := m.width/2 - 4

	// ── Row 1: game header | hero stats ──────────────────────
	row1 := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderGamePanel(colW),
		m.renderHeroPanel(colW),
	)

	// ── Row 2: board | modifications ─────────────────────────
	// Build content first, pad to equal line count, then apply border so
	// rounded corners stay intact on both panels.
	boardContent := m.boardContent()
	modsContent := m.modsContent()
	boardLines := strings.Count(boardContent, "\n")
	modsLines := strings.Count(modsContent, "\n")
	if boardLines < modsLines {
		boardContent += strings.Repeat("\n", modsLines-boardLines)
	} else if modsLines < boardLines {
		modsContent += strings.Repeat("\n", boardLines-modsLines)
	}
	row2 := lipgloss.JoinHorizontal(lipgloss.Top,
		styleBorder.Width(colW).Render(boardContent),
		styleBorder.Width(colW).Render(modsContent),
	)

	// ── Row 3: session stats ──────────────────────────────────
	row3 := m.renderSessionBar(m.width - 4)

	// ── Help bar ──────────────────────────────────────────────
	help := styleHelp.Render("  [r] Refresh game  [R] Refresh stats  [q] Quit")

	return lipgloss.JoinVertical(lipgloss.Left, row1, row2, row3, help)
}

// ============================================================
// Panel renderers
// ============================================================

func (m *Model) renderGamePanel(w int) string {
	var b strings.Builder

	b.WriteString(styleTitle.Render("BATTLESTREAM") + "\n")

	if m.game == nil {
		b.WriteString(styleDim.Render("waiting for game…") + "\n\n\n\n")
	} else {
		phase := m.game.Phase
		if phase == "" {
			phase = "IDLE"
		}
		b.WriteString(styleLabel.Render("Game   ") + styleValue.Render(m.game.GameId) + "\n")
		b.WriteString(styleLabel.Render("Phase  ") + stylePhase.Render(phase) + "\n")
		b.WriteString(styleLabel.Render("Turn   ") + styleValue.Render(fmt.Sprintf("%d", m.game.Turn)) + "\n")
		b.WriteString(styleLabel.Render("Tavern ") + renderTavernTier(int(m.game.TavernTier)) + "\n")
		// Blank line to match hero panel height (title + 5 data rows each)
		b.WriteString("")
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

	maxHP := int32(40)
	b.WriteString(styleLabel.Render("Health  ") + renderHealthBar(p.Health, maxHP, 16) + "\n")
	armor := "—"
	if p.Armor > 0 {
		armor = fmt.Sprintf("%d", p.Armor)
	}
	b.WriteString(styleLabel.Render("Armor   ") + styleValue.Render(armor) + "\n")
	b.WriteString(styleLabel.Render("Spell+  ") + styleValue.Render(fmt.Sprintf("%d", p.SpellPower)) + "\n")
	b.WriteString(styleLabel.Render("Triples ") + styleValue.Render(fmt.Sprintf("%d", p.TripleCount)) + "\n")

	return styleBorder.Width(w).Render(b.String())
}

func (m *Model) boardContent() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("YOUR BOARD") + "\n")

	if m.game == nil || len(m.game.Board) == 0 {
		b.WriteString(styleDim.Render("(empty)"))
	} else {
		for _, mn := range m.game.Board {
			b.WriteString(renderMinion(mn) + "\n")
		}
	}

	return b.String()
}

func (m *Model) modsContent() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("MODIFICATIONS") + "\n")

	if m.game == nil || len(m.game.Modifications) == 0 {
		b.WriteString(styleDim.Render("(none this game)"))
	} else {
		mods := m.game.Modifications
		if len(mods) > 8 {
			mods = mods[len(mods)-8:]
		}
		for _, mod := range mods {
			sign := "+"
			if mod.Delta < 0 {
				sign = ""
			}
			line := fmt.Sprintf("T%-2d %s%d %-6s %s",
				mod.Turn, sign, mod.Delta, mod.Stat, mod.Target)
			b.WriteString(styleMod.Render(line) + "\n")
		}
	}

	return b.String()
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
