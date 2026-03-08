package debugtui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/parser"
)

// Style constants (copied from internal/tui/tui.go to avoid coupling).
var (
	colorPurple = lipgloss.Color("63")
	colorGold   = lipgloss.Color("220")
	colorGreen  = lipgloss.Color("46")
	colorRed    = lipgloss.Color("196")
	colorOrange = lipgloss.Color("214")
	colorDim    = lipgloss.Color("240")
	colorHelp   = lipgloss.Color("241")
	colorMuted  = lipgloss.Color("244")

	colorHealthFg = lipgloss.Color("46")
	colorHealthBg = lipgloss.Color("22")
	colorBarBg    = lipgloss.Color("235")

	// Buff category colors (mirrored from main TUI).
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
	colorMod       = lipgloss.Color("213")

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPurple).
			Padding(0, 1)

	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(colorGold)
	styleLabel = lipgloss.NewStyle().Foreground(colorMuted)
	styleValue = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	styleDim   = lipgloss.NewStyle().Foreground(colorDim)
	styleHelp  = lipgloss.NewStyle().Foreground(colorHelp)
	stylePhase = lipgloss.NewStyle().Foreground(colorOrange).Bold(true)
	styleWin   = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	styleLoss  = lipgloss.NewStyle().Foreground(colorRed)

	styleHealthBar = lipgloss.NewStyle().
			Foreground(colorHealthFg).
			Background(colorHealthBg)
	styleHealthBarEmpty = lipgloss.NewStyle().
				Background(colorBarBg)

	styleEvent    = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	styleRawLine  = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	styleInputBox = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
)

func renderMinion(mn gamestate.MinionState) string {
	name := mn.Name
	if name == "" {
		name = gamestate.CardName(mn.CardID)
	}
	if len(name) > 22 {
		name = name[:21] + "…"
	}
	stats := fmt.Sprintf("%d/%d", mn.Attack, mn.Health)
	return fmt.Sprintf("  %s %s",
		styleValue.Render(fmt.Sprintf("%-22s", name)),
		lipgloss.NewStyle().Foreground(colorGold).Render(stats),
	)
}

func renderHealthBar(current, max, barWidth int) string {
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
	return lipgloss.NewStyle().Foreground(tavernTierColor(tier)).Render(fmt.Sprintf("%d %s", tier, stars))
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

func renderEvent(e parser.GameEvent) string {
	var parts []string
	parts = append(parts, string(e.Type))

	if e.EntityName != "" {
		name := e.EntityName
		if len(name) > 40 {
			name = name[:39] + "…"
		}
		parts = append(parts, name)
	} else if e.EntityID > 0 {
		parts = append(parts, fmt.Sprintf("id=%d", e.EntityID))
	}
	if e.CardID != "" {
		parts = append(parts, e.CardID)
	}

	// Show key tags inline.
	for k, v := range e.Tags {
		switch k {
		case "TURN", "ZONE", "ATK", "HEALTH", "CARDTYPE", "STATE", "CONTROLLER":
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return styleEvent.Render(strings.Join(parts, "  "))
}

func renderBoard(board []gamestate.MinionState) string {
	if len(board) == 0 {
		return styleDim.Render("  (empty)")
	}
	var lines []string
	for _, mn := range board {
		lines = append(lines, renderMinion(mn))
	}
	return strings.Join(lines, "\n")
}

var eventTypeNames = []string{
	"ALL",
	string(parser.EventGameStart),
	string(parser.EventGameEnd),
	string(parser.EventTurnStart),
	string(parser.EventEntityUpdate),
	string(parser.EventTagChange),
	string(parser.EventPlayerDef),
	string(parser.EventPlayerName),
}

// wrapLine splits a line into multiple lines that fit within maxWidth characters.
func wrapLine(line string, maxWidth int) []string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	if len(line) <= maxWidth {
		return []string{line}
	}
	var lines []string
	for len(line) > maxWidth {
		lines = append(lines, line[:maxWidth])
		line = line[maxWidth:]
	}
	if len(line) > 0 {
		lines = append(lines, line)
	}
	return lines
}

func filterName(idx int) string {
	if idx < 0 || idx >= len(eventTypeNames) {
		return "ALL"
	}
	return eventTypeNames[idx]
}

// ── Buff sources panel ──────────────────────────────────────────

func renderBuffSources(state gamestate.BGGameState) string {
	var b strings.Builder

	if len(state.BuffSources) == 0 {
		b.WriteString(styleDim.Render("(none)"))
		return b.String()
	}

	// Sort by total magnitude (largest first).
	sources := make([]gamestate.BuffSource, len(state.BuffSources))
	copy(sources, state.BuffSources)
	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			totalI := abs(sources[i].Attack) + abs(sources[i].Health)
			totalJ := abs(sources[j].Attack) + abs(sources[j].Health)
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

	if len(state.AbilityCounters) > 0 {
		b.WriteString("\n" + styleTitle.Render("ABILITIES") + "\n")
		for _, ac := range state.AbilityCounters {
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
		"BLOODGEM":         colorBloodgem,
		"BLOODGEM_BARRAGE": colorBloodgem,
		"NOMI":             colorNomi,
		"ELEMENTAL":        colorElemental,
		"TAVERN_SPELL":     colorTavern,
		"WHELP":            colorWhelp,
		"BEETLE":           colorBeetle,
		"RIGHTMOST":        colorRightmost,
		"UNDEAD":           colorUndead,
		"VOLUMIZER":        colorVolumizer,
		"LIGHTFANG":        colorLightfang,
		"NOMI_ALL":         colorNomi,
		"NAGA_SPELLS":      colorTavern,
		"FREE_REFRESH":     colorGold,
		"GOLD_NEXT_TURN":   colorGold,
		"CONSUMED":         colorDim,
		"GENERAL":          colorGeneral,
	}
	if c, ok := colors[cat]; ok {
		return c
	}
	return colorMod
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// renderScrollbar returns a 1-char-wide vertical scrollbar for a viewport.
// Outputs exactly height lines joined by "\n".
// Returns a blank column if all content fits without scrolling.
func renderScrollbar(vp viewport.Model, height int) string {
	if height <= 0 {
		return ""
	}
	if vp.TotalLineCount() <= height {
		// No scrollbar needed — blank column so JoinHorizontal stays aligned.
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

// ── Step delta / changes ────────────────────────────────────────

// Change describes a single state difference between two steps.
type Change struct {
	Kind        string // "add", "remove", "change", "phase", "player", "buff"
	Description string
}

// computeChanges compares two game states and returns a list of human-readable changes.
func computeChanges(prev, curr gamestate.BGGameState) []Change {
	var changes []Change

	// Phase change.
	if prev.Phase != curr.Phase {
		changes = append(changes, Change{"phase",
			fmt.Sprintf("Phase: %s -> %s", prev.Phase, curr.Phase)})
	}

	// Turn change.
	if prev.Turn != curr.Turn {
		changes = append(changes, Change{"change",
			fmt.Sprintf("Turn: %d -> %d", prev.Turn, curr.Turn)})
	}

	// Tavern tier change.
	if prev.TavernTier != curr.TavernTier {
		changes = append(changes, Change{"change",
			fmt.Sprintf("Tavern: %d -> %d", prev.TavernTier, curr.TavernTier)})
	}

	// Player stat changes.
	if prev.Player.Health != curr.Player.Health {
		changes = append(changes, Change{"player",
			fmt.Sprintf("Health: %d -> %d", prev.Player.Health, curr.Player.Health)})
	}
	if prev.Player.Damage != curr.Player.Damage {
		changes = append(changes, Change{"player",
			fmt.Sprintf("Damage: %d -> %d (HP: %d)", prev.Player.Damage, curr.Player.Damage, curr.Player.EffectiveHealth())})
	}
	if prev.Player.Armor != curr.Player.Armor {
		changes = append(changes, Change{"player",
			fmt.Sprintf("Armor: %d -> %d", prev.Player.Armor, curr.Player.Armor)})
	}
	if prev.Player.TripleCount != curr.Player.TripleCount {
		changes = append(changes, Change{"player",
			fmt.Sprintf("Triples: %d -> %d", prev.Player.TripleCount, curr.Player.TripleCount)})
	}

	// Board diff.
	prevBoard := make(map[int]gamestate.MinionState)
	for _, mn := range prev.Board {
		prevBoard[mn.EntityID] = mn
	}
	currBoard := make(map[int]gamestate.MinionState)
	for _, mn := range curr.Board {
		currBoard[mn.EntityID] = mn
	}

	// Added minions.
	for _, mn := range curr.Board {
		if _, ok := prevBoard[mn.EntityID]; !ok {
			name := mn.Name
			if name == "" {
				name = gamestate.CardName(mn.CardID)
			}
			changes = append(changes, Change{"add",
				fmt.Sprintf("+ %s (%d/%d)", name, mn.Attack, mn.Health)})
		}
	}

	// Removed minions.
	for _, mn := range prev.Board {
		if _, ok := currBoard[mn.EntityID]; !ok {
			name := mn.Name
			if name == "" {
				name = gamestate.CardName(mn.CardID)
			}
			changes = append(changes, Change{"remove",
				fmt.Sprintf("- %s", name)})
		}
	}

	// Changed minion stats.
	for _, mn := range curr.Board {
		if old, ok := prevBoard[mn.EntityID]; ok {
			var diffs []string
			if mn.Attack != old.Attack {
				diffs = append(diffs, fmt.Sprintf("ATK %d->%d", old.Attack, mn.Attack))
			}
			if mn.Health != old.Health {
				diffs = append(diffs, fmt.Sprintf("HP %d->%d", old.Health, mn.Health))
			}
			if len(diffs) > 0 {
				name := mn.Name
				if name == "" {
					name = gamestate.CardName(mn.CardID)
				}
				changes = append(changes, Change{"change",
					fmt.Sprintf("~ %s: %s", name, strings.Join(diffs, ", "))})
			}
		}
	}

	// Buff source changes.
	prevBuffs := make(map[string]gamestate.BuffSource)
	for _, bs := range prev.BuffSources {
		prevBuffs[bs.Category] = bs
	}
	for _, bs := range curr.BuffSources {
		old := prevBuffs[bs.Category]
		if bs.Attack != old.Attack || bs.Health != old.Health {
			name := buffCategoryDisplayName(bs.Category)
			changes = append(changes, Change{"buff",
				fmt.Sprintf("%s: +%d/+%d -> +%d/+%d", name, old.Attack, old.Health, bs.Attack, bs.Health)})
		}
	}

	return changes
}

func renderChanges(changes []Change) string {
	var b strings.Builder

	if len(changes) == 0 {
		b.WriteString(styleDim.Render("(no state change)"))
		return b.String()
	}

	styleAdd := lipgloss.NewStyle().Foreground(colorGreen)
	styleRemove := lipgloss.NewStyle().Foreground(colorRed)
	styleChg := lipgloss.NewStyle().Foreground(colorGold)
	styleBuff := lipgloss.NewStyle().Foreground(colorOrange)
	stylePhaseChg := lipgloss.NewStyle().Foreground(colorPurple).Bold(true)

	for _, c := range changes {
		var s lipgloss.Style
		switch c.Kind {
		case "add":
			s = styleAdd
		case "remove":
			s = styleRemove
		case "change", "player":
			s = styleChg
		case "buff":
			s = styleBuff
		case "phase":
			s = stylePhaseChg
		default:
			s = styleValue
		}
		b.WriteString(s.Render(c.Description) + "\n")
	}

	return b.String()
}
