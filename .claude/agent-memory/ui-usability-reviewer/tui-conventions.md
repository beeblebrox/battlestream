---
name: tui-conventions
description: Layout structure, color palette, and component patterns in the Battlestream TUI (tui.go)
metadata:
  type: project
---

## Layout (tui.go)
- 2-column layout with draggable vertical split (default 0.5)
- Row 1: game panel (left) + hero panel (right) — fixed height, rendered first to measure
- Row 2: scrollable viewports — board (left) + buff sources (right)
- Row 3 (Duos only): partner board (left) + partner buffs (right) with horizontal split
- Bottom: session stats bar (full width, rounded border) + help bar (plain text)
- `--dump --width 120` renders a usable static snapshot for testing

## Color Palette (lipgloss terminal 256-color)
- Purple 63 — border accent, spinner
- Gold 220 — titles (BATTLESTREAM, section headers), minion stats, tavern tier
- Green 46 — wins, health bar fill, buff markers
- Red 196 — losses, low health, bloodgem color, error state
- Orange 214 — phase label, tavern tier 4
- Dim 240 — secondary labels, tribe list, enchantment counts, scroll track
- Muted 244 — field labels ("Game", "Phase", "Turn", etc.)
- Mod 213 — modifications (pink/magenta)
- Help 241 — help bar text
- Bright cyan 51 — TAVERN-WIDE section header
- Tavern purple 141 — TARGETED section header, tavern-spell color

## Known Usability Issues (reviewed and updated 2026-05-24, third review)

### Fixed in second review (2026-05-24)
- Tribes line wrapped mid-word at narrow widths — FIXED: now truncates at last comma before limit with "…"
- Health bar fixed 16-char width overflowed at 60-col panels — FIXED: responsive barWidth (capped 6–16)
- Anomaly "[d]" hint was part of label field, shifting value column — FIXED: hint moved to value suffix
- Triples showed "0" when zero — FIXED: now shows "—" consistent with Armor
- Last result row silently absent when no streak — FIXED: shows "Last    —" (styleDim) when showLastResult=true but no streak
- Buff display showed "(+0/+3)" for HP-only buffs in renderMinion() — FIXED: now shows "(+3 hp)" / "(+3 atk)"

### Still present after fifth review (2026-05-24, post-buff-budget fix)
- renderMinion() budget fix confirmed LANDED: buff annotation and tribe are now gated on `budget` (lines 1377, 1385). Buff block DOES appear after used/budget computation. (CONFIRMED FIXED)
- At 80 cols (vpContentW=35): "large stats" minion like 804/744 leaves budget=5 — buff (+6/+3) is 9 chars and is suppressed; tribe also suppressed. Only small-stats minions (1/1, 3/4) will show buff at 80 cols. This is correct gating behavior, not a bug.
- At 60 cols (vpContentW=25): ALL buffs and tribes suppressed in practice (max budget after name+stats = 6, buff min is 8 chars " (+1 hp)"). Clean, no clipping — correct.
- At 120 cols (vpContentW=55): buff and tribe both fit for most minions; enchant count (8 chars) may be suppressed when tribe eats the remaining budget (e.g. budget=7 after tribe, need 8 for enchants). Minor: last character of enchant count always clipped.
- Buff panel TARGETED/TYPE BUFFS rows still use +%d/+%d format — "(+1/+0)" not fixed in modsItems/partnerModsItems (lines 913, 936)
- "Anomaly " label is 8 chars vs all other game panel labels at 7 — value column misaligned when anomaly present (line 681)
- Health bar label shows raw negative HP (e.g. "-3/30") at GAME_OVER — pct clamped but label uses raw current (line 1277)
- Help bar is 118 chars (standard) / 104 chars (Duos); truncation at line 607 is byte-indexed not rune-indexed — unsafe for multibyte chars; also truncates mid-keybind at narrow widths (lines 606-608)
- Session bar: renderSessionBar(m.width-2) at line 599 passes width minus 2 but styleBorder adds 2 chars overhead — at 60 cols this creates a 2-char right gap in the session bar border
- Buff panel dead space: when only TAVERN-WIDE (3 lines) or ABILITIES (2-3 lines) present, viewport (~20 rows) is mostly blank — no dim placeholder fills the remainder; intentional but visually unpolished
- partnerModsItems (line 827) still uses old %-14s format (not %-12s as in modsItems) — minor inconsistency
- Partner section in hero panel uses `styleDim.Render("─ Partner ─")` at line 774 — divider is dim (color 240), blends into background; not visually distinct enough from preceding content
- Partner section has no leading blank line before "─ Partner ─" divider — content runs continuously from local hero fields into partner fields without whitespace breathing room (line 774)
- Partner board placeholder is "awaiting first combat" (line 805) — lowercase, whereas board uses uppercase "(empty)"; inconsistent capitalization convention
- Tribes label in game panel (line 686) shows same tribes as buff panel TYPE BUFFS section — minor redundancy but different purpose (tribes in game = which tribe buffs are available; TYPE BUFFS shows what was earned). Not wrong, but users may conflate them.
- Tribes truncation at 80 cols: with 6-tribe game "Murloc, Beast, Naga, Undead, Dragon, Quilboar"=47 chars, maxTribesW = leftInner(38)-7-4-1=26. "Murloc, Beast, Naga…" fits; this is meaningful. At 60 cols: leftInner=28, maxTribesW=16. "Murloc, Beast…" shown — still informative. Truncation logic is clean.

### Fixed in fourth review (2026-05-24)
- renderMinion() name field was fixed-22-char regardless of viewport width — FIXED
- Dump() showLastResult was false — FIXED

### Fixed in fifth review (2026-05-24, buff budget fix)
- renderMinion() buff annotation and tribe were rendered before used/budget check — FIXED: buff block now appears after budget computation; both are gated on budget

### Structural / not fixable in 5 lines
- scrollbar glyphs (▲/▼/█/│) in dim color — not immediately obvious as interactive
- Buff panel dead space is a data/content issue, not a rendering bug — needs more buff categories or a filler section

## ABILITIES Panel — Category Registration Checklist (as of 2026-05-24)
When a new AbilityCounter category is added to the backend:
1. Add entry to `CategoryDisplayName` in `internal/gamestate/categories.go` — fallback is raw SCREAMING_SNAKE_CASE, which is always wrong
2. Pass `fmt.Sprintf("%d", count)` (or a richer string) as the `Display` arg to `SetAbilityCounter` — the TUI renders `ac.Display` verbatim; `ac.Value` is never shown
3. Optionally add a color entry in `buffCategoryColor()` in `tui.go` — fallback is `colorMod` (pink, 213), semantically wrong for non-buff counters; use `colorDim` for historical/count stats
- MINIONS_SOLD fix: label="Sold", display=count integer string, color=colorDim (dim gray)

## Dump Output (width=120, GAME_OVER state, WIN #2, turn 13) — as of third review
Board: 7 minions. Buff panel: TAVERN-WIDE +6/+3, TYPE BUFFS Tavern Spells +1/+0, ABILITIES Bonus Gold 2 / Refreshes 1.
Tribes: 6 tribes fit at 120, truncate to "Naga, Murloc, Undead…" at 80, "Naga, Murloc…" at 60. Health: -3/30 shown.
Session: W:71 L:63 Avg 3.3 Games 134 Best #1. Help bar fits without truncation at 120 cols.

## Width math for renderMinion (post-fifth-review, buff budget fix confirmed)
- 120 cols: vpContentW=55, nameW=22, budget~25+ → buff+tribe both usually fit; enchant (8 chars) may not
- 80 cols: vpContentW=35, nameW=22, budget~5-9 → small-stats minions show buff/tribe; large-stats do not
- 60 cols: vpContentW=25, nameW=15, budget~2-6 → no buffs/tribes ever shown (minimum buff is 8 chars)
- Budget thresholds: smallest buff " (+1 hp)"=8, smallest tribe " [beast]"=8, enchant " 1 buffs"=8
