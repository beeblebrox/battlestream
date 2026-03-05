// Package tui implements a Bubbletea TUI dashboard for battlestream.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"battlestream.fixates.io/internal/gamestate"
)

// --- Styles ---

var (
	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63"))

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("220"))

	styleHealthHigh = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	styleHealthLow  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	styleDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleMod  = lipgloss.NewStyle().Foreground(lipgloss.Color("213"))
	styleHelp = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// --- Messages ---

type stateUpdateMsg struct {
	state gamestate.BGGameState
}

type tickMsg time.Time

// --- Model ---

// Model is the Bubbletea model for the TUI dashboard.
type Model struct {
	state     gamestate.BGGameState
	connected bool
	width     int
	height    int
	fetcher   func() gamestate.BGGameState
	ctx       context.Context
	cancel    context.CancelFunc
}

// New creates a new TUI Model.
// fetcher is called periodically to get the current game state
// (in production, replaced by a gRPC streaming client).
func New(fetcher func() gamestate.BGGameState) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	return &Model{
		fetcher: fetcher,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Run starts the Bubbletea program.
func (m *Model) Run() error {
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Init returns the initial command.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		fetchStateCmd(m.fetcher),
	)
}

// Update handles messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		case "r":
			return m, fetchStateCmd(m.fetcher)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case stateUpdateMsg:
		m.state = msg.state
		m.connected = true
		return m, tickCmd()
	case tickMsg:
		return m, fetchStateCmd(m.fetcher)
	}
	return m, nil
}

// View renders the TUI.
func (m *Model) View() string {
	if !m.connected {
		return "\n  Connecting to battlestream daemon...\n"
	}

	s := m.state
	halfW := m.width/2 - 2

	// Top row: game info + hero panel
	gamePanel := styleBorder.Width(halfW).Render(
		styleTitle.Render("BATTLESTREAM") + "\n" +
			fmt.Sprintf("Turn: %d  Tavern: %d\n", s.Turn, s.TavernTier) +
			fmt.Sprintf("Phase: %s", s.Phase),
	)

	health := s.Player.Health
	healthStyle := styleHealthHigh
	if health < 10 {
		healthStyle = styleHealthLow
	}
	heroPanel := styleBorder.Width(halfW).Render(
		fmt.Sprintf("Hero: %s\n", s.Player.HeroCardID) +
			fmt.Sprintf("Health: %s\n", healthStyle.Render(fmt.Sprintf("%d", health))) +
			fmt.Sprintf("Armor: %d\n", s.Player.Armor) +
			fmt.Sprintf("SpellPwr: %d\n", s.Player.SpellPower) +
			fmt.Sprintf("Triples: %d", s.Player.TripleCount),
	)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, gamePanel, heroPanel)

	// Board panels
	boardPanel := styleBorder.Width(halfW).Render(
		styleTitle.Render("YOUR BOARD") + "\n" +
			renderBoard(s.Board),
	)

	modsPanel := styleBorder.Width(halfW).Render(
		styleTitle.Render("MODIFICATIONS") + "\n" +
			renderMods(s.Modifications),
	)

	midRow := lipgloss.JoinHorizontal(lipgloss.Top, boardPanel, modsPanel)

	// Session bar
	sessionBar := styleBorder.Width(m.width - 4).Render(
		fmt.Sprintf("Phase: %s | Game: %s", s.Phase, s.GameID),
	)

	// Help bar
	helpBar := styleHelp.Render("[r] Refresh  [q] Quit")

	return lipgloss.JoinVertical(lipgloss.Left,
		topRow, midRow, sessionBar, helpBar,
	)
}

func renderBoard(board []gamestate.MinionState) string {
	if len(board) == 0 {
		return styleDim.Render("(empty)")
	}
	var sb strings.Builder
	for _, m := range board {
		sb.WriteString(fmt.Sprintf("[%s %d/%d", m.Name, m.Attack, m.Health))
		if m.BuffAttack != 0 || m.BuffHealth != 0 {
			sb.WriteString(fmt.Sprintf(" +%d/+%d", m.BuffAttack, m.BuffHealth))
		}
		sb.WriteString("]\n")
	}
	return sb.String()
}

func renderMods(mods []gamestate.StatMod) string {
	if len(mods) == 0 {
		return styleDim.Render("(none)")
	}
	// Show last 5
	start := len(mods) - 5
	if start < 0 {
		start = 0
	}
	var sb strings.Builder
	for _, mod := range mods[start:] {
		sb.WriteString(styleMod.Render(
			fmt.Sprintf("T%d: %+d %s %s\n", mod.Turn, mod.Delta, mod.Stat, mod.Target),
		))
	}
	return sb.String()
}

// --- Commands ---

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fetchStateCmd(fetcher func() gamestate.BGGameState) tea.Cmd {
	return func() tea.Msg {
		return stateUpdateMsg{state: fetcher()}
	}
}
