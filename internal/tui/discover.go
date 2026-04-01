package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"battlestream.fixates.io/internal/config"
	"battlestream.fixates.io/internal/discovery"
)

// ============================================================
// Discovery wizard — step constants
// ============================================================

type discoverStep int

const (
	discoverStepScan    discoverStep = iota // path input + scan results list
	discoverStepNaming                      // profile name per install (only if >1)
	discoverStepActive                      // active profile radio (only if >1)
	discoverStepSummary                     // confirm + save
)

// ============================================================
// Internal types
// ============================================================

type discoveredInstall struct {
	info     *discovery.InstallInfo
	selected bool
	verified installVerification
}

type installVerification struct {
	exeFound bool
	logFound bool
}

type namedProfile struct {
	install *discovery.InstallInfo
	name    string
}

// scanResultMsg carries the async result of a directory scan.
type scanResultMsg struct {
	installs []*discovery.InstallInfo
	err      error
}

// ============================================================
// DiscoverModel
// ============================================================

// DiscoverModel is the Bubbletea model for the interactive setup wizard.
type DiscoverModel struct {
	step discoverStep

	// Step 1 — scan
	installs      []*discoveredInstall
	cursor        int
	listNote      string // transient feedback
	scanning      bool
	scanInput     textinput.Model
	inputFocused  bool // is the path input focused?
	clearExisting bool // if true, wipe existing profiles on save

	// Step 2 — profile naming (only when >1 install selected)
	profiles   []namedProfile
	nameInputs []textinput.Model
	nameIdx    int

	// Step 3 — active profile radio
	activeCursor int

	// Config context
	cfg         *config.Config
	cfgSavePath string

	// Layout
	width  int
	height int

	// Terminal state
	err  error
	done bool
}

// NewDiscoverModel constructs a DiscoverModel.
func NewDiscoverModel(cfg *config.Config, cfgSavePath string) *DiscoverModel {
	if cfg == nil {
		cfg = &config.Config{}
	}
	ti := textinput.New()
	ti.Placeholder = "e.g. ~ or /home/user/.wine or /mnt/games"
	ti.Width = 50
	home, _ := os.UserHomeDir()
	ti.SetValue(home)

	return &DiscoverModel{
		cfg:         cfg,
		cfgSavePath: cfgSavePath,
		scanInput:   ti,
		width:       80,
		height:      24,
	}
}

// verifyInstall checks whether the exe and log directory actually exist on disk.
func verifyInstall(info *discovery.InstallInfo) installVerification {
	v := installVerification{}
	for _, exe := range []string{
		filepath.Join(info.InstallRoot, "Hearthstone.exe"),
		filepath.Join(info.InstallRoot, "Hearthstone.app"),
	} {
		if _, err := os.Stat(exe); err == nil {
			v.exeFound = true
			break
		}
	}
	if _, err := os.Stat(info.LogPath); err == nil {
		v.logFound = true
	}
	return v
}

// baseProfileName derives the default profile name prefix for an install root.
func baseProfileName(installRoot string) string {
	lower := strings.ToLower(installRoot)
	switch {
	case strings.Contains(lower, "compatdata"):
		return "steam-proton"
	case strings.Contains(lower, "drive_c") || strings.Contains(lower, ".wine"):
		return "wine"
	default:
		return "main"
	}
}

// suggestProfileName derives a unique profile name, appending -2, -3, etc. when
// needed to avoid collisions within the usedNames set.
func suggestProfileName(installRoot string, usedNames map[string]bool) string {
	base := baseProfileName(installRoot)
	if !usedNames[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !usedNames[candidate] {
			return candidate
		}
	}
}

// installKind returns a short display label for an install ("Wine", "Proton", "Native").
func installKind(info *discovery.InstallInfo) string {
	lower := strings.ToLower(info.InstallRoot)
	switch {
	case strings.Contains(lower, "compatdata"):
		return "Proton"
	case strings.Contains(lower, "drive_c"):
		return "Wine"
	default:
		return "Native"
	}
}

// expandPath expands a leading ~ and any $ENV_VAR references.
func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, _ := os.UserHomeDir()
		p = home + p[1:]
	}
	return os.ExpandEnv(p)
}

// needsNaming returns true when naming is required (i.e. more than one install selected).
func (m *DiscoverModel) needsNaming() bool {
	count := 0
	for _, inst := range m.installs {
		if inst.selected {
			count++
		}
	}
	return count > 1
}

// selectedInstalls returns the currently selected installs.
func (m *DiscoverModel) selectedInstalls() []*discovery.InstallInfo {
	var out []*discovery.InstallInfo
	for _, inst := range m.installs {
		if inst.selected {
			out = append(out, inst.info)
		}
	}
	return out
}

// addInstall appends an install if not already present.
func (m *DiscoverModel) addInstall(info *discovery.InstallInfo) {
	for _, existing := range m.installs {
		if existing.info.InstallRoot == info.InstallRoot {
			return
		}
	}
	m.installs = append(m.installs, &discoveredInstall{
		info:     info,
		selected: true,
		verified: verifyInstall(info),
	})
	m.cursor = len(m.installs) - 1
}

// buildNamingInputs prepares the naming step inputs with collision-free suggestions.
func (m *DiscoverModel) buildNamingInputs(selected []*discovery.InstallInfo) {
	usedNames := make(map[string]bool)
	m.profiles = make([]namedProfile, len(selected))
	m.nameInputs = make([]textinput.Model, len(selected))
	for i, info := range selected {
		name := suggestProfileName(info.InstallRoot, usedNames)
		usedNames[name] = true
		m.profiles[i] = namedProfile{install: info}
		ti := textinput.New()
		ti.Placeholder = "profile name"
		ti.SetValue(name)
		ti.Width = 30
		m.nameInputs[i] = ti
	}
	m.nameIdx = 0
	if len(m.nameInputs) > 0 {
		m.nameInputs[0].Focus()
	}
}

// doScan performs an async directory scan.
func doScan(path string) tea.Cmd {
	return func() tea.Msg {
		if path == "" {
			found, err := discovery.DiscoverAll()
			return scanResultMsg{installs: found, err: err}
		}
		expanded := expandPath(path)
		if info, err := discovery.DiscoverFromRoot(expanded); err == nil {
			return scanResultMsg{installs: []*discovery.InstallInfo{info}}
		}
		found, err := discovery.WalkForAllInstalls(expanded)
		if err != nil || len(found) == 0 {
			return scanResultMsg{err: fmt.Errorf("no Hearthstone install found under %s", expanded)}
		}
		return scanResultMsg{installs: found}
	}
}

// ============================================================
// Init
// ============================================================

func (m *DiscoverModel) Init() tea.Cmd {
	m.scanning = true
	return doScan("")
}

// ============================================================
// Update
// ============================================================

func (m *DiscoverModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case scanResultMsg:
		m.scanning = false
		if msg.err != nil && len(msg.installs) == 0 {
			m.listNote = fmt.Sprintf("No installs found: %v", msg.err)
		} else {
			prev := len(m.installs)
			for _, info := range msg.installs {
				m.addInstall(info)
			}
			added := len(m.installs) - prev
			if added > 0 {
				m.listNote = fmt.Sprintf("Found %d install(s)", added)
			} else if len(msg.installs) > 0 {
				m.listNote = "Already in list — no new installs found"
			} else {
				m.listNote = "No Hearthstone installs found in that directory"
			}
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward non-key msgs to the text input when it is focused.
	if m.inputFocused {
		var cmd tea.Cmd
		m.scanInput, cmd = m.scanInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *DiscoverModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case discoverStepScan:
		return m.handleScanKey(msg)
	case discoverStepNaming:
		return m.handleNamingKey(msg)
	case discoverStepActive:
		return m.handleActiveKey(msg)
	case discoverStepSummary:
		return m.handleSummaryKey(msg)
	}
	return m, nil
}

// ----- Step 1: scan -----

func (m *DiscoverModel) handleScanKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inputFocused {
		switch msg.String() {
		case "ctrl+c":
			m.err = fmt.Errorf("setup cancelled")
			m.done = true
			return m, tea.Quit
		case "esc":
			m.inputFocused = false
			m.scanInput.Blur()
			return m, nil
		case "enter":
			path := strings.TrimSpace(m.scanInput.Value())
			m.inputFocused = false
			m.scanInput.Blur()
			m.scanning = true
			m.listNote = ""
			return m, doScan(path)
		}
		var cmd tea.Cmd
		m.scanInput, cmd = m.scanInput.Update(msg)
		return m, cmd
	}

	m.listNote = ""
	switch msg.String() {
	case "ctrl+c", "q":
		m.err = fmt.Errorf("setup cancelled")
		m.done = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.installs)-1 {
			m.cursor++
		}

	case " ":
		if len(m.installs) > 0 {
			m.installs[m.cursor].selected = !m.installs[m.cursor].selected
		}

	case "d", "backspace":
		if len(m.installs) > 0 {
			m.installs = append(m.installs[:m.cursor], m.installs[m.cursor+1:]...)
			if m.cursor >= len(m.installs) && m.cursor > 0 {
				m.cursor--
			}
		}

	case "c", "C":
		// Clear all: reset the install list and mark existing profiles for removal.
		m.installs = nil
		m.cursor = 0
		m.clearExisting = true
		m.listNote = "All existing profiles will be cleared on save. Scan to add new installs."

	case "/", "s", "S":
		m.inputFocused = true
		m.scanInput.Focus()
		return m, textinput.Blink

	case "r", "R":
		m.scanning = true
		m.listNote = ""
		return m, doScan("")

	case "enter", "right":
		sel := m.selectedInstalls()
		if len(sel) == 0 {
			m.listNote = "Select at least one install (SPACE to toggle)"
			return m, nil
		}
		return m.advanceFromScan()
	}
	return m, nil
}

// advanceFromScan decides the next step based on how many installs are selected.
func (m *DiscoverModel) advanceFromScan() (tea.Model, tea.Cmd) {
	sel := m.selectedInstalls()
	if m.needsNaming() {
		m.buildNamingInputs(sel)
		m.step = discoverStepNaming
	} else {
		// Single install — auto-name it, skip naming step entirely.
		m.profiles = []namedProfile{{
			install: sel[0],
			name:    baseProfileName(sel[0].InstallRoot),
		}}
		m.activeCursor = 0
		m.step = discoverStepSummary
	}
	return m, nil
}

// ----- Step 2: profile naming -----

func (m *DiscoverModel) handleNamingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.err = fmt.Errorf("setup cancelled")
		m.done = true
		return m, tea.Quit
	case "esc":
		m.step = discoverStepScan
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.nameInputs[m.nameIdx].Value())
		if name == "" {
			usedNames := make(map[string]bool)
			for i := 0; i < m.nameIdx; i++ {
				usedNames[m.profiles[i].name] = true
			}
			name = suggestProfileName(m.profiles[m.nameIdx].install.InstallRoot, usedNames)
		}
		m.profiles[m.nameIdx].name = name
		m.nameIdx++
		if m.nameIdx >= len(m.nameInputs) {
			m.activeCursor = 0
			if len(m.profiles) > 1 {
				m.step = discoverStepActive
			} else {
				m.step = discoverStepSummary
			}
			return m, nil
		}
		m.nameInputs[m.nameIdx-1].Blur()
		m.nameInputs[m.nameIdx].Focus()
		return m, nil
	}
	var cmd tea.Cmd
	m.nameInputs[m.nameIdx], cmd = m.nameInputs[m.nameIdx].Update(msg)
	return m, cmd
}

// ----- Step 3: active profile -----

func (m *DiscoverModel) handleActiveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.err = fmt.Errorf("setup cancelled")
		m.done = true
		return m, tea.Quit
	case "esc":
		m.nameIdx = 0
		for i := range m.nameInputs {
			m.nameInputs[i].Blur()
		}
		if len(m.nameInputs) > 0 {
			m.nameInputs[0].Focus()
		}
		m.step = discoverStepNaming
	case "up", "k":
		if m.activeCursor > 0 {
			m.activeCursor--
		}
	case "down", "j":
		if m.activeCursor < len(m.profiles)-1 {
			m.activeCursor++
		}
	case "enter":
		m.step = discoverStepSummary
	}
	return m, nil
}

// ----- Step 4: summary -----

func (m *DiscoverModel) handleSummaryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.err = fmt.Errorf("setup cancelled")
		m.done = true
		return m, tea.Quit
	case "esc":
		if len(m.profiles) > 1 {
			m.step = discoverStepActive
		} else if m.needsNaming() {
			m.nameIdx = 0
			if len(m.nameInputs) > 0 {
				m.nameInputs[0].Focus()
			}
			m.step = discoverStepNaming
		} else {
			m.step = discoverStepScan
		}
	case "enter", "y":
		if err := m.saveConfig(); err != nil {
			m.err = err
		}
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *DiscoverModel) saveConfig() error {
	if m.clearExisting {
		m.cfg.Profiles = make(map[string]*config.ProfileConfig)
		m.cfg.ActiveProfile = ""
	}
	for i, p := range m.profiles {
		pc := config.NewProfileConfig(p.name)
		pc.Hearthstone.InstallPath = p.install.InstallRoot
		pc.Hearthstone.LogPath = p.install.LogPath
		setActive := i == m.activeCursor
		m.cfg.AddProfile(p.name, pc, setActive)
	}
	return config.Save(m.cfg, m.cfgSavePath)
}

// Error returns any error that terminated the wizard.
func (m *DiscoverModel) Error() error { return m.err }

// Done reports whether the wizard has finished.
func (m *DiscoverModel) Done() bool { return m.done }

// ============================================================
// View
// ============================================================

var (
	styleWizBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPurple).
			Padding(0, 2)

	styleWizTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorGold)

	styleWizStep = lipgloss.NewStyle().
			Foreground(colorDim)

	styleWizCheck = lipgloss.NewStyle().
			Foreground(colorGreen)

	styleWizUncheck = lipgloss.NewStyle().
			Foreground(colorDim)

	styleWizCursor = lipgloss.NewStyle().
			Foreground(colorGold).
			Bold(true)

	styleWizNote = lipgloss.NewStyle().
			Foreground(colorDim).
			Italic(true)

	styleWizHelp = lipgloss.NewStyle().
			Foreground(colorHelp)

	styleWizRadioOn = lipgloss.NewStyle().
			Foreground(colorGold).
			Bold(true)

	styleWizRadioOff = lipgloss.NewStyle().
				Foreground(colorDim)

	styleWizVerifyOK = lipgloss.NewStyle().
				Foreground(colorGreen)

	styleWizVerifyMiss = lipgloss.NewStyle().
				Foreground(colorDim)

	styleWizWarning = lipgloss.NewStyle().
			Foreground(colorGold)

	styleWizInputActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorGold).
				Padding(0, 1)

	styleWizInputIdle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorDim).
				Padding(0, 1)
)

func (m *DiscoverModel) View() string {
	switch m.step {
	case discoverStepScan:
		return m.viewScan()
	case discoverStepNaming:
		return m.viewNaming()
	case discoverStepActive:
		return m.viewActive()
	case discoverStepSummary:
		return m.viewSummary()
	}
	return ""
}

func (m *DiscoverModel) innerWidth() int {
	w := m.width - 8
	if w < 40 {
		return 40
	}
	return w
}

// ----- Step 1: scan view -----

func (m *DiscoverModel) viewScan() string {
	iw := m.innerWidth()
	var sb strings.Builder

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render("Step 1 — Scan for Hearthstone installs"))
	sb.WriteString("\n\n")

	// Path input section.
	inputStyle := styleWizInputIdle
	if m.inputFocused {
		inputStyle = styleWizInputActive
	}
	sb.WriteString(inputStyle.Render(m.scanInput.View()))
	sb.WriteString("\n")
	if m.inputFocused {
		sb.WriteString(styleWizNote.Render("  ENTER to scan from this path · ESC to cancel"))
	} else {
		sb.WriteString(styleWizNote.Render("  Press / to edit path · R to rescan defaults · C to clear all"))
	}
	sb.WriteString("\n\n")

	// Clear-all notice.
	if m.clearExisting {
		sb.WriteString(styleWizWarning.Render("⚠  Existing profiles will be replaced on save"))
		sb.WriteString("\n\n")
	}

	// Results section.
	if m.scanning {
		sb.WriteString(styleWizNote.Render("  Scanning..."))
		sb.WriteString("\n\n")
	} else if len(m.installs) == 0 {
		sb.WriteString(styleWizNote.Render("No installs found."))
		sb.WriteString("\n")
		sb.WriteString(styleWizNote.Render("Press / to enter a custom path, then ENTER to scan. Or press R to rescan default locations."))
		sb.WriteString("\n\n")
	} else {
		sb.WriteString(styleWizStep.Render(fmt.Sprintf("%d install(s) found:", len(m.installs))))
		sb.WriteString("\n\n")
		for i, inst := range m.installs {
			cursor := "  "
			if i == m.cursor {
				cursor = styleWizCursor.Render("▶ ")
			}
			check := styleWizUncheck.Render("[ ]")
			if inst.selected {
				check = styleWizCheck.Render("[✓]")
			}
			kind := installKind(inst.info)
			root := truncatePath(inst.info.InstallRoot, iw-22)
			sb.WriteString(fmt.Sprintf("%s%s  %s  %s\n",
				cursor, check,
				styleValue.Render(root),
				styleDim.Render("("+kind+")")))

			// Verification badges.
			exeBadge := styleWizVerifyMiss.Render("· exe")
			if inst.verified.exeFound {
				exeBadge = styleWizVerifyOK.Render("✓ exe")
			}
			logBadge := styleWizVerifyMiss.Render("· logs")
			if inst.verified.logFound {
				logBadge = styleWizVerifyOK.Render("✓ logs")
			}
			logPath := truncatePath(inst.info.LogPath, iw-24)
			sb.WriteString(fmt.Sprintf("       %s  %s  %s\n",
				exeBadge, logBadge, styleDim.Render(logPath)))
		}
		sb.WriteString("\n")
	}

	if m.listNote != "" {
		sb.WriteString(styleWizNote.Render(m.listNote))
		sb.WriteString("\n\n")
	}

	help := "SPACE toggle · D remove · C clear all · / scan path · R rescan · ENTER continue · Q quit"
	sb.WriteString(styleWizHelp.Render(help))

	return styleWizBorder.Width(m.width - 4).Render(sb.String())
}

// ----- Step 2: profile naming view -----

func (m *DiscoverModel) viewNaming() string {
	var sb strings.Builder

	total := len(m.nameInputs)
	current := m.nameIdx + 1
	if current > total {
		current = total
	}

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render(fmt.Sprintf("Step 2 — Name profiles (%d of %d)", current, total)))
	sb.WriteString("\n\n")

	if m.nameIdx < len(m.profiles) {
		inst := m.profiles[m.nameIdx].install
		iw := m.innerWidth()
		sb.WriteString(fmt.Sprintf("%s  %s\n",
			styleLabel.Render("Install:"),
			styleValue.Render(truncatePath(inst.InstallRoot, iw-10))))
		sb.WriteString(fmt.Sprintf("%s     %s\n",
			styleLabel.Render("Log:"),
			styleDim.Render(truncatePath(inst.LogPath, iw-10))))
		sb.WriteString(fmt.Sprintf("%s    %s\n",
			styleLabel.Render("Kind:"),
			styleDim.Render(installKind(inst))))
		sb.WriteString("\n")
		sb.WriteString(styleWizNote.Render("Profile name identifies this install (e.g. main, wine, steam-proton)."))
		sb.WriteString("\n")
		sb.WriteString(styleWizNote.Render("Used with the --profile flag."))
		sb.WriteString("\n\n")
		sb.WriteString(styleLabel.Render("Profile name: "))
		sb.WriteString(m.nameInputs[m.nameIdx].View())
		sb.WriteString("\n\n")
	}

	help := "ENTER confirm · ESC back to scan"
	sb.WriteString(styleWizHelp.Render(help))

	return styleWizBorder.Width(m.width - 4).Render(sb.String())
}

// ----- Step 3: active profile view -----

func (m *DiscoverModel) viewActive() string {
	var sb strings.Builder

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render("Step 3 — Select active profile"))
	sb.WriteString("\n\n")

	sb.WriteString(styleDim.Render("Which profile should be active by default?"))
	sb.WriteString("\n\n")

	iw := m.innerWidth()
	for i, p := range m.profiles {
		cursor := "  "
		if i == m.activeCursor {
			cursor = styleWizCursor.Render("▶ ")
		}
		radio := styleWizRadioOff.Render("( )")
		if i == m.activeCursor {
			radio = styleWizRadioOn.Render("(•)")
		}
		root := truncatePath(p.install.InstallRoot, iw-len(p.name)-16)
		sb.WriteString(fmt.Sprintf("%s%s  %-20s  %s\n",
			cursor, radio,
			styleValue.Render(p.name),
			styleDim.Render(root)))
	}
	sb.WriteString("\n")

	help := "j/k navigate · ENTER confirm · ESC back"
	sb.WriteString(styleWizHelp.Render(help))

	return styleWizBorder.Width(m.width - 4).Render(sb.String())
}

// ----- Step 4: summary view -----

func (m *DiscoverModel) viewSummary() string {
	var sb strings.Builder

	stepNum := 2
	if len(m.profiles) > 1 {
		stepNum = 4
	}
	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render(fmt.Sprintf("Step %d — Confirm and save", stepNum)))
	sb.WriteString("\n\n")

	if m.clearExisting {
		sb.WriteString(styleWizWarning.Render("⚠  Existing profiles will be replaced"))
		sb.WriteString("\n\n")
	}

	sb.WriteString(styleLabel.Render("Profiles to save:"))
	sb.WriteString("\n\n")

	iw := m.innerWidth()
	for i, p := range m.profiles {
		active := ""
		if i == m.activeCursor {
			active = styleWizCheck.Render(" ← active")
		}
		v := verifyInstall(p.install)
		exeBadge := styleWizVerifyMiss.Render("· exe")
		if v.exeFound {
			exeBadge = styleWizVerifyOK.Render("✓ exe")
		}
		logNote := styleWizVerifyMiss.Render("· logs (will be created on first run)")
		if v.logFound {
			logNote = styleWizVerifyOK.Render("✓ logs")
		}
		root := truncatePath(p.install.InstallRoot, iw-len(p.name)-16)
		sb.WriteString(fmt.Sprintf("  %-20s → %s%s\n",
			styleValue.Render(p.name),
			styleDim.Render(root),
			active))
		sb.WriteString(fmt.Sprintf("  %s  %s\n\n", exeBadge, logNote))
	}

	savePath := m.cfgSavePath
	if savePath == "" {
		home, _ := os.UserHomeDir()
		savePath = filepath.Join(home, ".battlestream", "config.yaml")
	}
	sb.WriteString(fmt.Sprintf("%s  %s\n",
		styleLabel.Render("Active profile:"),
		styleValue.Render(m.profiles[m.activeCursor].name)))
	sb.WriteString(fmt.Sprintf("%s      %s\n",
		styleLabel.Render("Save path:"),
		styleDim.Render(truncatePath(savePath, iw-12))))
	sb.WriteString("\n")

	help := "ENTER save · Q cancel · ESC back"
	sb.WriteString(styleWizHelp.Render(help))

	return styleWizBorder.Width(m.width - 4).Render(sb.String())
}

// truncatePath shortens a path to maxLen with a leading ellipsis if needed.
func truncatePath(p string, maxLen int) string {
	if maxLen < 8 {
		maxLen = 8
	}
	if len(p) <= maxLen {
		return p
	}
	return "…" + p[len(p)-maxLen+1:]
}

// ============================================================
// RunDiscover — entry point
// ============================================================

// RunDiscover starts the interactive discovery wizard.
func RunDiscover(cfg *config.Config, cfgSavePath string) error {
	m := NewDiscoverModel(cfg, cfgSavePath)
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return err
	}
	final, ok := result.(*DiscoverModel)
	if !ok {
		return fmt.Errorf("unexpected model type")
	}
	if final.Error() != nil {
		return final.Error()
	}
	printDiscoverSummary(final)
	return nil
}

func printDiscoverSummary(m *DiscoverModel) {
	savePath := m.cfgSavePath
	if savePath == "" {
		home, _ := os.UserHomeDir()
		savePath = filepath.Join(home, ".battlestream", "config.yaml")
	}
	fmt.Printf("\nConfig saved to: %s\n", savePath)
	fmt.Printf("Active profile:  %s\n", m.profiles[m.activeCursor].name)
	if len(m.profiles) > 1 {
		names := make([]string, len(m.profiles))
		for i, p := range m.profiles {
			names[i] = p.name
		}
		fmt.Printf("All profiles:    %s\n", strings.Join(names, ", "))
		fmt.Println("\nUse --profile <name> to select a specific profile.")
	}
	fmt.Println("\nRun 'battlestream daemon' to start the service.")
}

// DumpDiscover renders the initial wizard screen to a string (for snapshot testing).
func DumpDiscover(cfg *config.Config, width int) (string, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	m := NewDiscoverModel(cfg, "")
	m.width = width
	m.height = 24
	return m.View(), nil
}
