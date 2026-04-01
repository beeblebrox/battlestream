package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
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
	discoverStepList    discoverStep = iota // multi-select list of all found installs
	discoverStepPicker                      // manual directory browser
	discoverStepNaming                      // profile name per selected install
	discoverStepActive                      // active profile radio select (multi only)
	discoverStepSummary                     // confirm + save
)

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

	// Step 1 — install list
	installs []*discoveredInstall
	cursor   int
	listNote string // transient feedback (e.g. "nothing selected")

	// Step 2 — file picker
	fp        filepicker.Model
	pickerErr string

	// Step 3 — profile naming
	profiles   []namedProfile
	nameInputs []textinput.Model
	nameIdx    int

	// Step 4 — active profile radio
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

// NewDiscoverModel constructs a DiscoverModel, auto-populating installs.
func NewDiscoverModel(cfg *config.Config, cfgSavePath string) *DiscoverModel {
	if cfg == nil {
		cfg = &config.Config{}
	}
	m := &DiscoverModel{
		cfg:         cfg,
		cfgSavePath: cfgSavePath,
		width:       80,
		height:      24,
	}
	m.loadAutoInstalls()
	return m
}

func (m *DiscoverModel) loadAutoInstalls() {
	found, err := discovery.DiscoverAll()
	if err != nil {
		m.listNote = fmt.Sprintf("Auto-discovery: %v", err)
		return
	}
	for _, info := range found {
		m.installs = append(m.installs, &discoveredInstall{info: info, selected: true})
	}
}

func (m *DiscoverModel) initFilePicker() {
	fp := filepicker.New()
	home, _ := os.UserHomeDir()
	fp.CurrentDirectory = home
	fp.DirAllowed = true
	fp.FileAllowed = false
	fp.ShowHidden = false
	fp.AutoHeight = false
	fp.Height = max(m.height-8, 5)
	m.fp = fp
	m.pickerErr = ""
}

// suggestProfileName derives a profile name from the install root path.
func suggestProfileName(installRoot string) string {
	lower := strings.ToLower(installRoot)
	switch {
	case strings.Contains(lower, "compatdata") || (strings.Contains(lower, "steam") && strings.Contains(lower, "drive_c")):
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
	return nil
}

// ============================================================
// Update
// ============================================================

func (m *DiscoverModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.step == discoverStepPicker {
			m.fp.Height = max(m.height-8, 5)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// File picker needs non-key messages (async dir reads).
	if m.step == discoverStepPicker {
		var cmd tea.Cmd
		m.fp, cmd = m.fp.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *DiscoverModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case discoverStepList:
		return m.handleListKey(msg)
	case discoverStepPicker:
		return m.handlePickerKey(msg)
	case discoverStepNaming:
		return m.handleNamingKey(msg)
	case discoverStepActive:
		return m.handleActiveKey(msg)
	case discoverStepSummary:
		return m.handleSummaryKey(msg)
	}
	return m, nil
}

// ----- Step 1: install list -----

func (m *DiscoverModel) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	case "a":
		// Toggle all.
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
		m.initFilePicker()
		m.step = discoverStepPicker
	case "enter", "right":
		var sel []*discovery.InstallInfo
		for _, inst := range m.installs {
			if inst.selected {
				sel = append(sel, inst.info)
			}
		}
		if len(sel) == 0 {
			m.listNote = "Select at least one install (SPACE to toggle)"
			return m, nil
		}
		m.buildNamingInputs(sel)
		m.step = discoverStepNaming
	}
	return m, nil
}

func (m *DiscoverModel) buildNamingInputs(selected []*discovery.InstallInfo) {
	m.profiles = make([]namedProfile, len(selected))
	m.nameInputs = make([]textinput.Model, len(selected))
	for i, info := range selected {
		m.profiles[i] = namedProfile{install: info}
		ti := textinput.New()
		ti.Placeholder = "profile name"
		ti.SetValue(suggestProfileName(info.InstallRoot))
		ti.Width = 30
		m.nameInputs[i] = ti
	}
	m.nameIdx = 0
	if len(m.nameInputs) > 0 {
		m.nameInputs[0].Focus()
	}
}

// ----- Step 2: file picker -----

func (m *DiscoverModel) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.err = fmt.Errorf("setup cancelled")
		m.done = true
		return m, tea.Quit
	case "esc":
		m.step = discoverStepList
		return m, nil
	case "tab", " ":
		// Select the current directory for discovery.
		selected := m.fp.CurrentDirectory
		m.addFromPath(selected)
		m.step = discoverStepList
		return m, nil
	}
	// Delegate all other keys to the filepicker.
	var cmd tea.Cmd
	m.fp, cmd = m.fp.Update(msg)
	return m, cmd
}

func (m *DiscoverModel) addFromPath(path string) {
	m.pickerErr = ""
	info, err := discovery.DiscoverFromRoot(path)
	if err == nil {
		m.addInstall(info)
		return
	}
	// Walk with depth cap to avoid slow scans.
	infos, walkErr := walkInstallsDepthLimited(path, 5)
	if walkErr != nil || len(infos) == 0 {
		m.pickerErr = fmt.Sprintf("No Hearthstone install found in %s", path)
		return
	}
	for _, inst := range infos {
		m.addInstall(inst)
	}
}

func (m *DiscoverModel) addInstall(info *discovery.InstallInfo) {
	for _, existing := range m.installs {
		if existing.info.InstallRoot == info.InstallRoot {
			return
		}
	}
	m.installs = append(m.installs, &discoveredInstall{info: info, selected: true})
	m.cursor = len(m.installs) - 1
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

// ----- Step 3: profile naming -----

func (m *DiscoverModel) handleNamingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.err = fmt.Errorf("setup cancelled")
		m.done = true
		return m, tea.Quit
	case "esc":
		m.step = discoverStepList
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.nameInputs[m.nameIdx].Value())
		if name == "" {
			name = suggestProfileName(m.profiles[m.nameIdx].install.InstallRoot)
		}
		m.profiles[m.nameIdx].name = name
		m.nameIdx++
		if m.nameIdx >= len(m.nameInputs) {
			// All profiles named — advance.
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
	// Delegate to the active text input.
	var cmd tea.Cmd
	m.nameInputs[m.nameIdx], cmd = m.nameInputs[m.nameIdx].Update(msg)
	return m, cmd
}

// ----- Step 4: active profile -----

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

// ----- Step 5: summary -----

func (m *DiscoverModel) handleSummaryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.err = fmt.Errorf("setup cancelled")
		m.done = true
		return m, tea.Quit
	case "esc":
		if len(m.profiles) > 1 {
			m.step = discoverStepActive
		} else {
			m.nameIdx = 0
			if len(m.nameInputs) > 0 {
				m.nameInputs[0].Focus()
			}
			m.step = discoverStepNaming
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
)

func (m *DiscoverModel) View() string {
	switch m.step {
	case discoverStepList:
		return m.viewList()
	case discoverStepPicker:
		return m.viewPicker()
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
	w := m.width - 8 // border + padding
	if w < 40 {
		return 40
	}
	return w
}

// ----- Step 1: list view -----

func (m *DiscoverModel) viewList() string {
	iw := m.innerWidth()
	var sb strings.Builder

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render("Step 1/4 — Select installations"))
	sb.WriteString("\n\n")

	if len(m.installs) == 0 {
		sb.WriteString(styleWizNote.Render("No installs found automatically."))
		sb.WriteString("\n")
		sb.WriteString(styleWizNote.Render("Press M to browse for one manually."))
		sb.WriteString("\n\n")
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
			sb.WriteString(fmt.Sprintf("%s%s %s  %s\n",
				cursor, check,
				styleValue.Render(root),
				styleDim.Render("("+kind+")")))
			if i == m.cursor {
				logShort := truncatePath(inst.info.LogPath, iw-6)
				sb.WriteString(fmt.Sprintf("       %s\n", styleDim.Render("Log: "+logShort)))
			}
		}
		sb.WriteString("\n")
	}

	if m.listNote != "" {
		sb.WriteString(styleWizError.Render(m.listNote))
		sb.WriteString("\n\n")
	}

	if m.pickerErr != "" {
		sb.WriteString(styleWizError.Render(m.pickerErr))
		sb.WriteString("\n\n")
	}

	help := "SPACE toggle · A toggle all · M add manually · ENTER continue · Q quit"
	sb.WriteString(styleWizHelp.Render(help))

	return styleWizBorder.Width(m.width - 4).Render(sb.String())
}

// ----- Step 2: file picker view -----

func (m *DiscoverModel) viewPicker() string {
	iw := m.innerWidth()
	var sb strings.Builder

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render("Step 1b — Browse to Hearthstone directory"))
	sb.WriteString("\n\n")

	sb.WriteString(styleDim.Render("Current: "))
	sb.WriteString(styleValue.Render(truncatePath(m.fp.CurrentDirectory, iw-10)))
	sb.WriteString("\n\n")

	sb.WriteString(m.fp.View())
	sb.WriteString("\n\n")

	if m.pickerErr != "" {
		sb.WriteString(styleWizError.Render(m.pickerErr))
		sb.WriteString("\n")
	}

	help := "j/k navigate · ENTER open dir · TAB/SPACE select current dir · ESC back"
	sb.WriteString(styleWizHelp.Render(help))

	return styleWizBorder.Width(m.width - 4).Render(sb.String())
}

// ----- Step 3: profile naming view -----

func (m *DiscoverModel) viewNaming() string {
	var sb strings.Builder

	total := len(m.nameInputs)
	current := m.nameIdx + 1
	if current > total {
		current = total
	}

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render(fmt.Sprintf("Step 2/4 — Name profile (%d/%d)", current, total)))
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
		sb.WriteString(styleWizNote.Render("Profile name identifies this install (e.g. main, ptr, steam-proton)."))
		sb.WriteString("\n")
		sb.WriteString(styleWizNote.Render("Used with the --profile flag."))
		sb.WriteString("\n\n")
		sb.WriteString(styleLabel.Render("Profile name: "))
		sb.WriteString(m.nameInputs[m.nameIdx].View())
		sb.WriteString("\n\n")
	}

	help := "ENTER confirm · ESC back"
	sb.WriteString(styleWizHelp.Render(help))

	return styleWizBorder.Width(m.width - 4).Render(sb.String())
}

// ----- Step 4: active profile view -----

func (m *DiscoverModel) viewActive() string {
	var sb strings.Builder

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render("Step 3/4 — Select active profile"))
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

// ----- Step 5: summary view -----

func (m *DiscoverModel) viewSummary() string {
	var sb strings.Builder

	sb.WriteString(styleWizTitle.Render("BATTLESTREAM SETUP"))
	sb.WriteString("\n")
	sb.WriteString(styleWizStep.Render("Step 4/4 — Confirm and save"))
	sb.WriteString("\n\n")

	sb.WriteString(styleLabel.Render("Profiles to add:"))
	sb.WriteString("\n\n")

	iw := m.innerWidth()
	for i, p := range m.profiles {
		active := ""
		if i == m.activeCursor {
			active = styleWizCheck.Render(" ← active")
		}
		root := truncatePath(p.install.InstallRoot, iw-len(p.name)-16)
		sb.WriteString(fmt.Sprintf("  %-20s → %s%s\n",
			styleValue.Render(p.name),
			styleDim.Render(root),
			active))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("%s  %s\n",
		styleLabel.Render("Active profile:"),
		styleValue.Render(m.profiles[m.activeCursor].name)))

	savePath := m.cfgSavePath
	if savePath == "" {
		home, _ := os.UserHomeDir()
		savePath = filepath.Join(home, ".battlestream", "config.yaml")
	}
	sb.WriteString(fmt.Sprintf("%s      %s\n",
		styleLabel.Render("Save path:"),
		styleDim.Render(truncatePath(savePath, iw-12))))
	sb.WriteString("\n")

	help := "ENTER save and continue · Q cancel · ESC back"
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
// On success the config is written to cfgSavePath and a summary is printed.
// cfgSavePath may be empty; the model will derive the default path.
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
	// Print post-save summary to stdout.
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
