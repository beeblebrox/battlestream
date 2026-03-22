package tui

import (
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"battlestream.fixates.io/internal/config"
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

	// Startup notice (e.g. log.config was patched). Blocks until Enter.
	startupNotice string
}

// NewCombined creates a CombinedModel that starts in live mode.
func NewCombined(grpcAddr string, st *store.Store, logFiles []string, cfg *config.Config) *CombinedModel {
	live := New(grpcAddr, cfg)
	live.parentYOffset = 1 // mode indicator bar
	return &CombinedModel{
		mode:     modeLive,
		live:     live,
		grpcAddr: grpcAddr,
		logFiles: logFiles,
		store:    st,
	}
}

// SetStartupNotice sets a notice to display when the TUI starts.
func (c *CombinedModel) SetStartupNotice(msg string) {
	c.startupNotice = msg
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
	// Don't start the live model until the user dismisses the startup notice.
	if c.startupNotice != "" {
		return nil
	}
	return c.live.Init()
}

func (c *CombinedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// While startup notice is showing, block everything except Enter to confirm.
	// Allow WindowSizeMsg through so dimensions are captured for later.
	if c.startupNotice != "" {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			c.width = msg.Width
			c.height = msg.Height
			return c, nil
		case tea.KeyMsg:
			if msg.String() == "enter" {
				c.startupNotice = ""
				// Start the live model and send it the stored window size.
				cmds := []tea.Cmd{c.live.Init()}
				if c.width > 0 {
					inner := tea.WindowSizeMsg{Width: c.width, Height: c.height - 1}
					_, cmd := c.live.Update(inner)
					cmds = append(cmds, cmd)
				}
				return c, tea.Batch(cmds...)
			}
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return c, tea.Quit
			}
		}
		// Swallow all other messages while notice is up.
		return c, nil
	}

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
		default:
			slog.Debug("combined: forwarding key to live model", "key", msg.String(), "mode", c.mode)
		}

	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
		// Reserve 1 row for the mode indicator bar at the top.
		inner := tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height - 1}
		var cmds []tea.Cmd
		if c.live != nil {
			_, cmd := c.live.Update(inner)
			cmds = append(cmds, cmd)
		}
		if c.replay != nil {
			_, cmd := c.replay.Update(inner)
			cmds = append(cmds, cmd)
		}
		return c, tea.Batch(cmds...)

	case replayInitDoneMsg:
		c.replay = msg.model
		c.replayInitialized = true
		// Start the replay model's async loading (spinner, progress, file parsing).
		var cmds []tea.Cmd
		cmds = append(cmds, c.replay.Init())
		if c.width > 0 {
			_, cmd := c.replay.Update(tea.WindowSizeMsg{Width: c.width, Height: c.height - 1})
			cmds = append(cmds, cmd)
		}
		return c, tea.Batch(cmds...)
	}

	// Always forward to live model to keep event chain alive.
	var cmds []tea.Cmd
	if c.live != nil {
		_, cmd := c.live.Update(msg)
		cmds = append(cmds, cmd)
	}
	// Also forward to replay if it's the active mode and initialized.
	if c.mode == modeReplay && c.replay != nil {
		_, cmd := c.replay.Update(msg)
		cmds = append(cmds, cmd)
	}
	return c, tea.Batch(cmds...)
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
	// Re-establish live event chain and refresh state.
	var cmds []tea.Cmd
	if c.live != nil && c.live.client != nil {
		cmds = append(cmds, fetchGameCmd(c.live.ctx, c.live.client))
		if c.live.eventCh != nil {
			cmds = append(cmds, waitForEventCmd(c.live.eventCh))
		}
	}
	return c, tea.Batch(cmds...)
}

func (c *CombinedModel) initReplayCmd() tea.Cmd {
	st := c.store
	logFiles := c.logFiles
	return func() tea.Msg {
		done := make(chan replayInitDoneMsg, 1)
		go func() {
			var model *debugtui.Model

			// Try loading from database first.
			if st != nil {
				replay, err := debugtui.LoadAllFromStore(st)
				if err == nil && len(replay.Games) > 0 {
					model = debugtui.NewFromReplay(replay)
					model.SetSources(st, logFiles, debugtui.SourceDB)
					done <- replayInitDoneMsg{model: model}
					return
				}
			}

			// Fall back to log files.
			if len(logFiles) > 0 {
				model = debugtui.New(logFiles)
				model.SetSources(st, logFiles, 0)
				done <- replayInitDoneMsg{model: model}
				return
			}

			// No data available — create empty model.
			model = debugtui.NewFromReplay(&debugtui.Replay{})
			model.SetSources(st, logFiles, 0)
			done <- replayInitDoneMsg{model: model}
		}()

		select {
		case msg := <-done:
			return msg
		case <-time.After(5 * time.Second):
			// Timeout — return empty model to avoid freezing the TUI.
			model := debugtui.NewFromReplay(&debugtui.Replay{})
			model.SetSources(st, logFiles, 0)
			return replayInitDoneMsg{model: model}
		}
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
			body = "\n  Loading replay data…\n"
		}
	}

	// If startup notice is active, show ONLY the notice (full screen).
	if c.startupNotice != "" {
		noticeStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("202")).
			Padding(0, 1)
		promptStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true)
		return "\n" +
			noticeStyle.Render(" "+c.startupNotice+" ") + "\n\n" +
			promptStyle.Render("  Press Enter to continue...") + "\n"
	}

	return indicator + "\n" + body
}
