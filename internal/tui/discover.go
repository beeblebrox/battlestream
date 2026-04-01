package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
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
	discoverStepScanning  discoverStep = iota // async scan in progress
	discoverStepList                          // multi-select list of found installs
	discoverStepPathInput                     // manual path text entry
	discoverStepNaming                        // profile name per selected install
	discoverStepActive                        // active profile radio (multi only)
	discoverStepSummary                       // confirm + save
	discoverStepDone                          // success screen
)

// ============================================================
// Messages
// ============================================================

type scanResultMsg struct {
	installs []*discovery.InstallInfo
	err      error
}

type pathSearchResultMsg struct {
	infos []*discovery.InstallInfo
	path  string
	err   error
}

// ============================================================
// Internal types
// ============================================================

type discoveredInstall struct {
	info     *discovery.InstallInfo
	selected bool
}

type namedProfile struct {
	install *discovery.InstallInfo
	name    string
}

// ============================================================
// DiscoverModel
// ============================================================

// DiscoverModel is the Bubbletea model for the interactive setup wizard.
type DiscoverModel struct {
	step discoverStep

	// Step 0 — scanning
	spin spinner.Model

	// Step 1 — install list
	installs []*discoveredInstall
	cursor   int
	listNote string // transient feedback (e.g. "nothing selected")
	scanErr  string // non-fatal scan error message

	// Step 1b — path input
	pathInput     textinput.Model
	pathInputErr  string
	pathSearching bool

	// Step 2 — profile naming
	profiles   []namedProfile
	nameInputs []textinput.Model
	nameIdx    int
	nameNote   string // per-field warning

	// Step 3 — active profile radio
	activeCursor int

	// Config context
	cfg              *config.Config
	cfgSavePath      string
	existingProfiles map[string]bool // names already in cfg before this wizard run

	// Layout
	width  int
	height int

	// Terminal state
	err  error
	done bool
}

// NewDiscoverModel constructs a DiscoverModel. Discovery runs async via Init().
func NewDiscoverModel(cfg *config.Config, cfgSavePath string) *DiscoverModel {
	if cfg == nil {
		cfg = &config.Config{}
	}

	spin := spinner.New()
	spin.Spinner = spinner.Points
	spin.Style = lipgloss.NewStyle().Foreground(colorGold)

	existing := make(map[string]bool)
	for name := range cfg.Profiles {
		existing[name] = true
	}

	pi := textinput.New()
	pi.Placeholder = "e.g. ~/.wine  or  /path/to/Hearthstone"
	pi.Width = 55
	pi.CharLimit = 256

	return &DiscoverModel{
		step:             discoverStepScanning,
		spin:             spin,
		cfg:              cfg,
		cfgSavePath:      cfgSavePath,
		existingProfiles: existing,
		pathInput:        pi,
		width:            80,
		height:           24,
	}
}

// scanCmd runs DiscoverAll asynchronously.
func scanCmd() tea.Cmd {
	return func() tea.Msg {
		found, err := discovery.DiscoverAll()
		return scanResultMsg{installs: found, err: err}
	}
}

// pathSearchCmd runs path-based discovery asynchronously.
func pathSearchCmd(path string) tea.Cmd {
	return func() tea.Msg {
		expanded := expandPath(path)
		info, err := discovery.DiscoverFromRoot(expanded)
		if err == nil {
			return pathSearchResultMsg{infos: []*discovery.InstallInfo{info}, path: expanded}
		}
		infos, walkErr := walkInstallsDepthLimited(expanded, 5)
		if walkErr != nil || len(infos) == 0 {
			return pathSearchResultMsg{path: expanded, err: fmt.Errorf("no Hearthstone install found in %s", expanded)}
		}
		return pathSearchResultMsg{infos: infos, path: expanded}
	}
}

// expandPath resolves ~ and environment variables in a path.
func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	return os.ExpandEnv(p)
}

// suggestProfileName derives a collision-free profile name.
// usedNames should contain existing config names plus any already-assigned session names.
func suggestProfileName(installRoot string, usedNames map[string]bool) string {
	base := suggestBaseName(installRoot)
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

func suggestBaseName(installRoot string) string {
	lower := strings.ToLower(installRoot)
	switch {
	case strings.Contains(lower, "compatdata") ||
		(strings.Contains(lower, "steam") && strings.Contains(lower, "drive_c")):
		return "steam-proton"
	case strings.Contains(lower, ".wine"):
		return "wine"
	default:
		return "main"
	}
}

// installKind returns a short label for display ("Wine", "Proton", "Native").
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

// ============================================================
// Init
// ============================================================

func (m *DiscoverModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, scanCmd())
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

	case spinner.TickMsg:
		if m.step == discoverStepScanning {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case scanResultMsg:
		for _, info := range msg.installs {
			m.installs = append(m.installs, &discoveredInstall{info: info, selected: true})
		}
		if msg.err != nil && len(msg.installs) == 0 {
			m.scanErr = msg.err.Error()
		}
		m.step = discoverStepList
		return m, nil

	case pathSearchResultMsg:
		m.pathSearching = false
		if msg.err != nil {
			m.pathInputErr = msg.err.Error()
			return m, nil
		}
		added := 0
		for _, info := range msg.infos {
			if m.addInstall(info) {
				added++
			}
		}
		if added == 0 {
			m.pathInputErr = "All found installs are already in the list."
			return m, nil
		}
		m.pathInput.SetValue("")
		m.pathInputErr = ""
		m.step = discoverStepList
		m.listNote = fmt.Sprintf("Added %d install(s) from %s", added, truncatePath(msg.path, 40))
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Path input needs non-key messages (cursor blink, etc.).
	if m.step == discoverStepPathInput && !m.pathSearching {
		var cmd tea.Cmd
		m.pathInput, cmd = m.pathInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *DiscoverModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quit.
	if msg.String() == "ctrl+c" {
		m.err = fmt.Errorf("setup cancelled")
		m.done = true
		return m, tea.Quit
	}
	// Ignore keys while scanning.
	if m.step == discoverStepScanning {
		return m, nil
	}
	switch m.step {
	case discoverStepList:
		return m.handleListKey(msg)
	case discoverStepPathInput:
		return m.handlePathInputKey(msg)
	case discoverStepNaming:
		return m.handleNamingKey(msg)
	case discoverStepActive:
		return m.handleActiveKey(msg)
	case discoverStepSummary:
		return m.handleSummaryKey(msg)
	case discoverStepDone:
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

// ----- Step 1: install list -----

func (m *DiscoverModel) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.listNote = ""
	switch msg.String() {
	case "q":
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
	case "a":
		allOn := true
		for _, inst := range m.installs {
			if !inst.selected {
				allOn = false
				break
			}
		}
		for _, inst := range m.installs {
			inst.selected = !allOn
		}
	case "m":
		m.pathInput.Focus()
		m.pathInputErr = ""
		m.step = discoverStepPathInput
	case "enter", "right":
		var sel []*discovery.InstallInfo
		for _, inst := range m.installs {
			if inst.selected {
				sel = append(sel, inst.info)
			}
		}
		if len(sel) == 0 {
			if len(m.installs) == 0 {
				m.listNote = "No installs found — press M to add one manually"
			} else {
				m.listNote = "Select at least one install (SPACE to toggle)"
			}
			return m, nil
		}
		m.buildNamingInputs(sel)
		m.step = discoverStepNaming
	}
	return m, nil
}

func (m *DiscoverModel) buildNamingInputs(selected []*discovery.InstallInfo) {
	// Collision-free suggestions: existing config names + session names assigned so far.
	used := make(map[string]bool)
	for name := range m.existingProfiles {
		used[name] = true
	}

	m.profiles = make([]namedProfile, len(selected))
	m.nameInputs = make([]textinput.Model, len(selected))
	for i, info := range selected {
		name := suggestProfileName(info.InstallRoot, used)
		used[name] = true // reserve for subsequent installs

		m.profiles[i] = namedProfile{install: info}
		ti := textinput.New()
		ti.Placeholder = "profile name"
		ti.SetValue(name)
		ti.Width = 30
		m.nameInputs[i] = ti
	}
	m.nameIdx = 0
	m.nameNote = ""
	if len(m.nameInputs) > 0 {
		m.nameInputs[0].Focus()
	}
}

// ----- Step 1b: path input -----

func (m *DiscoverModel) handlePathInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.pathInput.Blur()
		m.pathInputErr = ""
		m.step = discoverStepList
		return m, nil
	case "enter":
		path := strings.TrimSpace(m.pathInput.Value())
		if path == "" {
			m.pathInputErr = "Enter a path to search"
			return m, nil
		}
		m.pathSearching = true
		m.pathInputErr = ""
		return m, pathSearchCmd(path)
	}
	if !m.pathSearching {
		var cmd tea.Cmd
		m.pathInput, cmd = m.pathInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// addInstall adds an install if not already present. Returns true if added.
func (m *DiscoverModel) addInstall(info *discovery.InstallInfo) bool {
	for _, existing := range m.installs {
		if existing.info.InstallRoot == info.InstallRoot {
			return false
		}
	}
	m.installs = append(m.installs, &discoveredInstall{info: info, selected: true})
	m.cursor = len(m.installs) - 1
	return true
}

// walkInstallsDepthLimited finds all HS installs under startDir up to maxDepth levels deep.
func walkInstallsDepthLimited(startDir string, maxDepth int) ([]*discovery.InstallInfo, error) {
	var all []*discovery.InstallInfo
	err := walkDirDepth(startDir, 0, maxDepth, func(path string) error {
		if info, err := discovery.DiscoverFromRoot(path); err == nil {
			all = append(all, info)
			return filepath.SkipDir
		}
		return nil
	})
	return all, err
}

func walkDirDepth(dir string, depth, maxDepth int, fn func(string) error) error {
	if depth > maxDepth {
		return nil
	}
	if err := fn(dir); err == filepath.SkipDir {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil //nolint: nilerr — skip unreadable dirs silently
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		child := filepath.Join(dir, e.Name())
		if err := walkDirDepth(child, depth+1, maxDepth, fn); err != nil {
			return err
		}
	}
	return nil
}

// ----- Step 2: profile naming -----

func (m *DiscoverModel) handleNamingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.nameIdx = 0
		m.nameNote = ""
		for i := range m.nameInputs {
			m.nameInputs[i].Blur()
		}
		m.step = discoverStepList
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.nameInputs[m.nameIdx].Value())
		if name == "" {
			used := m.usedNamesExcluding(m.nameIdx)
			name = suggestProfileName(m.profiles[m.nameIdx].install.InstallRoot, used)
		}
		// Hard-block duplicates within the session.
		if m.isSessionDuplicate(name, m.nameIdx) {
			m.nameNote = fmt.Sprintf("'%s' is already used by another install — choose a different name", name)
			return m, nil
		}
		m.nameNote = ""
		m.profiles[m.nameIdx].name = name
		m.nameInputs[m.nameIdx].Blur()
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
		m.nameInputs[m.nameIdx].Focus()
		m.nameNote = m.warnNameConflict(m.nameInputs[m.nameIdx].Value(), m.nameIdx)
		return m, nil
	}
	// Delegate to active text input; update inline note on each keystroke.
	var cmd tea.Cmd
	m.nameInputs[m.nameIdx], cmd = m.nameInputs[m.nameIdx].Update(msg)
	m.nameNote = m.warnNameConflict(m.nameInputs[m.nameIdx].Value(), m.nameIdx)
	return m, cmd
}

// usedNamesExcluding returns all names in use except the profile at excludeIdx.
func (m *DiscoverModel) usedNamesExcluding(excludeIdx int) map[string]bool {
	used := make(map[string]bool)
	for name := range m.existingProfiles {
		used[name] = true
	}
	for i, p := range m.profiles {
		if i != excludeIdx && p.name != "" {
			used[p.name] = true
		}
	}
	return used
}

// isSessionDuplicate returns true if name is already committed to another profile in this session.
func (m *DiscoverModel) isSessionDuplicate(name string, excludeIdx int) bool {
	for i, p := range m.profiles {
		if i != excludeIdx && p.name == name {
			return true
		}
	}
	return false
}

// warnNameConflict returns an advisory note for the name at nameIdx.
// Empty string means no conflict.
func (m *DiscoverModel) warnNameConflict(name string, idx int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// Session duplicate (already committed above).
	for i, p := range m.profiles {
		if i != idx && p.name == name {
			return fmt.Sprintf("'%s' is already used by another install in this session", name)
		}
	}
	// Existing config profile — allow but warn.
	if m.existingProfiles[name] {
		return fmt.Sprintf("'%s' exists — saving will update the existing profile", name)
	}
	return ""
}

// ----- Step 3: active profile -----

func (m *DiscoverModel) handleActiveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Return to the last naming field.
		m.nameIdx = len(m.nameInputs) - 1
		for i := range m.nameInputs {
			m.nameInputs[i].Blur()
		}
		if m.nameIdx >= 0 {
			m.nameInputs[m.nameIdx].Focus()
		}
		m.nameNote = ""
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
	case "q":
		m.err = fmt.Errorf("setup cancelled")
		m.done = true
		return m, tea.Quit
	case "esc":
		if len(m.profiles) > 1 {
			m.step = discoverStepActive
		} else {
			m.nameIdx = 0
			m.nameNote = ""
			for i := range m.nameInputs {
				m.nameInputs[i].Blur()
			}
			if len(m.nameInputs) > 0 {
				m.nameInputs[0].Focus()
			}
			m.step = discoverStepNaming
		}
	case "enter", "y":
		if err := m.saveConfig(); err != nil {
			m.err = err
			m.done = true
			return m, tea.Quit
		}
		m.step = discoverStepDone
	}
	return m, nil
}

func (m *DiscoverModel) saveConfig() error {
	for i, p := range m.profiles {
		pc := config.NewProfileConfig(p.name)
		pc.Hearthstone.InstallPath = p.install.InstallRoot
		pc.Hearthstone.LogPath = p.install.LogPath
		setActive := i == m.activeCursor
		m.cfg.AddProfile(p.name, pc, setActive)
	}
	return config.Save(m.cfg, m.cfgSavePath)
}

// Error returns any error that terminated the wizard (cancelled or save failure).
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

	styleWizError = lipgloss.NewStyle().
			Foreground(colorRed)

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

	styleWizWarn = lipgloss.NewStyle().
			Foreground(colorGold)

	styleWizSuccess = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)
)

func (m *DiscoverModel) View() string {
	switch m.step {
	case discoverStepScanning:
		return m.viewScanning()
	case discoverStepList:
		return m.viewList()
	case discoverStepPathInput:
		return m.viewPathInput()
	case discoverStepNaming:
		return m.viewNaming()
	case discoverStepActive:
		return m.viewActive()
	case discoverStepSummary:
		return m.viewSummary()
	case discoverStepDone:
		return m.viewDone()
	}
	return ""
}

func (m *DiscoverModel) innerWidth() int {
	w := m.width - 8 // border + padding
	if w < 40 {
		return 40
	}
	return w
}

// totalSteps returns the number of visible numbered steps.
func (m *DiscoverModel) totalSteps() int {
	if len(m.profiles) > 1 {
		return 4 // list, naming, active, summary
	}
	return 3 // list, naming, summary
}

// stepLabel returns "Step N/M — description" for the given step.
func (m *DiscoverModel) stepLabel(step discoverStep) string {
	t := m.totalSteps()
	switch step {
	case discoverStepList, discoverStepPathInput:
		return fmt.Sprintf("Step 1/%d — Find installations", t)
	case discoverStepNaming:
		return fmt.Sprintf("Step 2/%d — Name profiles", t)
	case discoverStepActive:
		return fmt.Sprintf("Step 3/%d — Select active profile", t)
	case discoverStepSummary:
		return fmt.Sprintf("Step %d/%d — Confirm and save", t, t)
	}
	return ""
}

// ----- Scanning view -----

func (m *DiscoverModel) viewScanning() string {
	var sb strings.Builder
	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("  %s  Scanning for Hearthstone installations...\n", m.spin.View()))
	sb.WriteString("\n")
	sb.WriteString(styleWizNote.Render("  Checking standard install paths and Steam libraries."))
	sb.WriteString("\n\n")
	sb.WriteString(styleWizHelp.Render("CTRL+C to cancel"))
	return styleWizBorder.Width(m.width - 4).Render(sb.String())
}

// ----- Step 1: list view -----

func (m *DiscoverModel) viewList() string {
	iw := m.innerWidth()
	var sb strings.Builder

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render(m.stepLabel(discoverStepList)))
	sb.WriteString("\n\n")

	if len(m.installs) == 0 {
		sb.WriteString(styleWizNote.Render("  No Hearthstone installations found automatically."))
		sb.WriteString("\n")
		sb.WriteString(styleWizNote.Render("  Press M to add a path manually."))
		sb.WriteString("\n")
		if m.scanErr != "" {
			sb.WriteString("\n")
			sb.WriteString(styleWizNote.Render("  Scan note: " + m.scanErr))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	} else {
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
			root := truncatePath(inst.info.InstallRoot, iw-20)
			sb.WriteString(fmt.Sprintf("%s%s  %s  %s\n",
				cursor, check,
				styleValue.Render(root),
				styleDim.Render("("+kind+")")))
			if i == m.cursor {
				logShort := truncatePath(inst.info.LogPath, iw-6)
				sb.WriteString(fmt.Sprintf("       %s\n", styleDim.Render("logs: "+logShort)))
			}
		}
		sb.WriteString("\n")
	}

	if m.listNote != "" {
		sb.WriteString(styleWizWarn.Render("  " + m.listNote))
		sb.WriteString("\n\n")
	}

	help := "SPACE toggle  ·  A toggle all  ·  M add manually  ·  ENTER continue  ·  Q quit"
	sb.WriteString(styleWizHelp.Render(help))

	return styleWizBorder.Width(m.width - 4).Render(sb.String())
}

// ----- Step 1b: path input view -----

func (m *DiscoverModel) viewPathInput() string {
	var sb strings.Builder

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render(m.stepLabel(discoverStepPathInput)))
	sb.WriteString("\n\n")

	sb.WriteString(styleWizNote.Render("  Enter a path to a Hearthstone install or a directory to search."))
	sb.WriteString("\n")
	sb.WriteString(styleWizNote.Render("  Tilde (~) and $ENV_VARS are expanded automatically."))
	sb.WriteString("\n\n")

	sb.WriteString("  ")
	sb.WriteString(styleLabel.Render("Path: "))
	if m.pathSearching {
		sb.WriteString(styleDim.Render("searching..."))
	} else {
		sb.WriteString(m.pathInput.View())
	}
	sb.WriteString("\n\n")

	if m.pathInputErr != "" {
		sb.WriteString(styleWizError.Render("  " + m.pathInputErr))
		sb.WriteString("\n\n")
	}

	help := "ENTER search  ·  ESC back to list"
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
	label := m.stepLabel(discoverStepNaming)
	if total > 1 {
		label += fmt.Sprintf(" (%d/%d)", current, total)
	}
	sb.WriteString(styleWizStep.Render(label))
	sb.WriteString("\n\n")

	if m.nameIdx < len(m.profiles) {
		inst := m.profiles[m.nameIdx].install
		iw := m.innerWidth()
		sb.WriteString(fmt.Sprintf("  %s  %s\n",
			styleLabel.Render("Install:"),
			styleValue.Render(truncatePath(inst.InstallRoot, iw-12))))
		sb.WriteString(fmt.Sprintf("  %s     %s\n",
			styleLabel.Render("Log:"),
			styleDim.Render(truncatePath(inst.LogPath, iw-12))))
		sb.WriteString(fmt.Sprintf("  %s    %s\n",
			styleLabel.Render("Kind:"),
			styleDim.Render(installKind(inst))))
		sb.WriteString("\n")
		sb.WriteString(styleWizNote.Render("  Profile name is used with the --profile flag (e.g. main, ptr, steam-proton)."))
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("  %s %s", styleLabel.Render("Name:"), m.nameInputs[m.nameIdx].View()))
		sb.WriteString("\n\n")
	}

	if m.nameNote != "" {
		isError := strings.Contains(m.nameNote, "already used by another install") ||
			strings.Contains(m.nameNote, "already used above")
		if isError {
			sb.WriteString(styleWizError.Render("  ⚠ " + m.nameNote))
		} else {
			sb.WriteString(styleWizWarn.Render("  ℹ " + m.nameNote))
		}
		sb.WriteString("\n\n")
	}

	help := "ENTER confirm  ·  ESC back to list"
	sb.WriteString(styleWizHelp.Render(help))

	return styleWizBorder.Width(m.width - 4).Render(sb.String())
}

// ----- Step 3: active profile view -----

func (m *DiscoverModel) viewActive() string {
	var sb strings.Builder

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render(m.stepLabel(discoverStepActive)))
	sb.WriteString("\n\n")

	sb.WriteString(styleDim.Render("  Which profile should be active by default?"))
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

	help := "j/k navigate  ·  ENTER confirm  ·  ESC back"
	sb.WriteString(styleWizHelp.Render(help))

	return styleWizBorder.Width(m.width - 4).Render(sb.String())
}

// ----- Step 4: summary view -----

func (m *DiscoverModel) viewSummary() string {
	var sb strings.Builder

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render(m.stepLabel(discoverStepSummary)))
	sb.WriteString("\n\n")

	sb.WriteString(styleLabel.Render("  Profiles to save:"))
	sb.WriteString("\n\n")

	iw := m.innerWidth()
	for i, p := range m.profiles {
		active := ""
		if i == m.activeCursor {
			active = styleWizCheck.Render(" ← active")
		}
		update := ""
		if m.existingProfiles[p.name] {
			update = styleWizWarn.Render(" (update)")
		}
		root := truncatePath(p.install.InstallRoot, iw-len(p.name)-22)
		sb.WriteString(fmt.Sprintf("  %-20s → %s%s%s\n",
			styleValue.Render(p.name),
			styleDim.Render(root),
			update,
			active))
	}
	sb.WriteString("\n")

	savePath := m.cfgSavePath
	if savePath == "" {
		home, _ := os.UserHomeDir()
		savePath = filepath.Join(home, ".battlestream", "config.yaml")
	}
	sb.WriteString(fmt.Sprintf("  %s  %s\n",
		styleLabel.Render("Save to:"),
		styleDim.Render(truncatePath(savePath, iw-12))))
	sb.WriteString("\n")

	help := "ENTER save  ·  Q cancel  ·  ESC back"
	sb.WriteString(styleWizHelp.Render(help))

	return styleWizBorder.Width(m.width - 4).Render(sb.String())
}

// ----- Done view -----

func (m *DiscoverModel) viewDone() string {
	var sb strings.Builder

	savePath := m.cfgSavePath
	if savePath == "" {
		home, _ := os.UserHomeDir()
		savePath = filepath.Join(home, ".battlestream", "config.yaml")
	}

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n\n")

	sb.WriteString(styleWizSuccess.Render("  ✓ Config saved successfully!"))
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("  %s  %s\n",
		styleLabel.Render("Config:"),
		styleDim.Render(savePath)))
	sb.WriteString(fmt.Sprintf("  %s  %s\n",
		styleLabel.Render("Active:"),
		styleValue.Render(m.profiles[m.activeCursor].name)))

	if len(m.profiles) > 1 {
		names := make([]string, len(m.profiles))
		for i, p := range m.profiles {
			names[i] = p.name
		}
		sb.WriteString(fmt.Sprintf("  %s  %s\n",
			styleLabel.Render("Profiles:"),
			styleDim.Render(strings.Join(names, ", "))))
	}

	sb.WriteString("\n")
	sb.WriteString(styleDim.Render("  Next steps:"))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %s  %s %s\n",
		styleValue.Render("1."),
		styleDim.Render("Run"),
		styleValue.Render("battlestream daemon")))
	sb.WriteString(fmt.Sprintf("  %s  %s %s\n",
		styleValue.Render("2."),
		styleDim.Render("Run"),
		styleValue.Render("battlestream tui")))
	if len(m.profiles) > 1 {
		sb.WriteString(fmt.Sprintf("  %s  %s %s %s\n",
			styleValue.Render("3."),
			styleDim.Render("Use"),
			styleValue.Render("--profile <name>"),
			styleDim.Render("to select a profile")))
	}
	sb.WriteString("\n")

	sb.WriteString(styleWizHelp.Render("  Press any key to exit"))

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
// On success the config is written to cfgSavePath.
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
	return final.Error()
}

// DumpDiscover renders the initial wizard screen to a string (for snapshot testing).
func DumpDiscover(cfg *config.Config, width int) (string, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	m := NewDiscoverModel(cfg, "")
	m.width = width
	m.height = 24
	// For dump mode, skip async scanning: run discovery synchronously and show the list.
	found, _ := discovery.DiscoverAll()
	for _, info := range found {
		m.installs = append(m.installs, &discoveredInstall{info: info, selected: true})
	}
	m.step = discoverStepList
	return m.View(), nil
}
