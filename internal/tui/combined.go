package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"battlestream.fixates.io/internal/debugtui"
	"battlestream.fixates.io/internal/store"
)

// tuiMode tracks which TUI is currently active.
type tuiMode int

const (
	modeLive   tuiMode = iota
	modeReplay
)

// CombinedModel wraps the live TUI and replay TUI, allowing runtime switching.
type CombinedModel struct {
	mode     tuiMode
	live     *Model
	replay   *debugtui.Model
	grpcAddr string
	logFiles []string
	store    *store.Store

	width  int
	height int

	replayInitialized bool
}

// NewCombined creates a CombinedModel that starts in live mode.
func NewCombined(grpcAddr string, st *store.Store, logFiles []string) *CombinedModel {
	return &CombinedModel{
		mode:     modeLive,
		live:     New(grpcAddr),
		grpcAddr: grpcAddr,
		logFiles: logFiles,
		store:    st,
	}
}

// Run starts the Bubbletea program with the combined model.
func (c *CombinedModel) Run() error {
	p := tea.NewProgram(c, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

// replayInitDoneMsg is sent when the replay model finishes async loading.
type replayInitDoneMsg struct {
	model *debugtui.Model
}

func (c *CombinedModel) Init() tea.Cmd {
	return c.live.Init()
}

func (c *CombinedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			return c.switchMode()
		case "q", "ctrl+c":
			// Quit from either mode
			if c.mode == modeLive {
				c.live.cancel()
				if c.live.client != nil {
					c.live.client.Close()
				}
			}
			return c, tea.Quit
		}

	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
		// Reserve 1 row for the mode indicator bar at the top.
		inner := tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height - 1}
		if c.live != nil {
			c.live.Update(inner)
		}
		if c.replay != nil {
			c.replay.Update(inner)
		}
		return c, nil

	case replayInitDoneMsg:
		c.replay = msg.model
		c.replayInitialized = true
		// Send window size to newly initialized replay model.
		if c.width > 0 {
			c.replay.Update(tea.WindowSizeMsg{Width: c.width, Height: c.height - 1})
		}
		return c, nil
	}

	// Route to active model.
	var cmd tea.Cmd
	switch c.mode {
	case modeLive:
		_, cmd = c.live.Update(msg)
	case modeReplay:
		if c.replay != nil {
			_, cmd = c.replay.Update(msg)
		}
	}
	return c, cmd
}

func (c *CombinedModel) switchMode() (tea.Model, tea.Cmd) {
	if c.mode == modeLive {
		c.mode = modeReplay
		if !c.replayInitialized {
			// Initialize replay model from store or log files.
			return c, c.initReplayCmd()
		}
		return c, nil
	}
	c.mode = modeLive
	return c, nil
}

func (c *CombinedModel) initReplayCmd() tea.Cmd {
	return func() tea.Msg {
		var model *debugtui.Model

		// Try loading from database first.
		if c.store != nil {
			replay, err := debugtui.LoadAllFromStore(c.store)
			if err == nil && len(replay.Games) > 0 {
				model = debugtui.NewFromReplay(replay)
				model.SetSources(c.store, c.logFiles, debugtui.SourceDB)
				return replayInitDoneMsg{model: model}
			}
		}

		// Fall back to log files.
		if len(c.logFiles) > 0 {
			model = debugtui.New(c.logFiles)
			model.SetSources(c.store, c.logFiles, 0)
			return replayInitDoneMsg{model: model}
		}

		// No data available — create empty model.
		model = debugtui.NewFromReplay(&debugtui.Replay{})
		model.SetSources(c.store, c.logFiles, 0)
		return replayInitDoneMsg{model: model}
	}
}

func (c *CombinedModel) View() string {
	// Mode indicator bar at the top.
	var indicator string
	liveLabel := " LIVE "
	replayLabel := " REPLAY "

	activeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("63"))
	inactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	if c.mode == modeLive {
		indicator = activeStyle.Render(liveLabel) + inactiveStyle.Render(" | "+replayLabel)
	} else {
		indicator = inactiveStyle.Render(liveLabel+" | ") + activeStyle.Render(replayLabel)
	}
	indicator += inactiveStyle.Render("  Tab: switch mode")

	// Active model view.
	var body string
	switch c.mode {
	case modeLive:
		if c.live != nil {
			body = c.live.View()
		}
	case modeReplay:
		if c.replay != nil {
			body = c.replay.View()
		} else {
			body = fmt.Sprintf("\n  Loading replay data…\n")
		}
	}

	return indicator + "\n" + body
}
