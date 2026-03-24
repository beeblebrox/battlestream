---
name: dashboard-qa
description: >
  Validate the Battlestream web dashboard at http://127.0.0.1:8080/dashboard/ using Playwright.
  Checks chart rendering, axis labels, help tooltips, filters, navigation, data integrity, and
  visual consistency. Use this skill whenever dashboard HTML/JS/CSS is modified, after builds that
  touch the dashboard, when the user asks to verify or test the dashboard, or when investigating
  a visual or data bug on the dashboard. Also use when the user says things like "check the
  dashboard", "run dashboard tests", "QA the charts", "verify the filters work", or "does the
  dashboard look right". Even partial mentions like "check the charts" or "test the heatmaps"
  should trigger this skill.
---

# Dashboard QA

Automated Playwright-based validation of the Battlestream web dashboard. The dashboard is an
ECharts-based single-page app served at `/dashboard/` by the REST server. It has three navigation
levels (Overview, Game Detail, Turn Detail), filtering (Last N games, Last N days, date range
scrubber, mode toggle, partner filter), and 14 Level-1 charts plus 6 Level-2 charts.

## Prerequisites

Before running any checks:

1. Navigate to `http://127.0.0.1:8080/dashboard/` using Playwright.
   If it fails to load, stop immediately and tell the user:
   > "Dashboard not accessible at http://127.0.0.1:8080/dashboard/. Start the daemon with
   > `./battlestream daemon` and try again."

2. Wait for data to load before running checks — evaluate `State.fullGames.size > 0` with a
   reasonable timeout (up to 10s). The dashboard fetches games asynchronously on load.

## Mindset: Think Like a Senior QA Engineer

You are not a checklist robot. You are a senior QA engineer who happens to have a checklist.
The numbered checks in `references/checks-dashboard.md` are your baseline — the minimum bar.
Beyond those, you should actively question what you see and flag anything that looks wrong,
incomplete, or confusing to a user.

### What "thinking like QA" means in practice

**Question empty or flat data.** If you drill into a game detail and see charts with flat
lines at zero (Board Stats, Health & Armor, Tier Progression, Gold Economy, Board Size),
don't just note "series exists" and move on. Ask: why is there no data? Is the API returning
empty turn snapshots? Is the chart rendering but with all-zero values? Report this as a
finding even if no check explicitly covers it.

**Question missing context.** If a chart exists but shows no useful information (e.g., a
scatter plot with a single data point, a heatmap with one cell), flag it. A chart that
technically renders but provides no analytical value is a UX problem.

**Question duos vs solo differences.** See the Duos-Specific Expectations section below.

**Question visual clarity.** If axis labels overlap, if colors are hard to distinguish, if
text is truncated, if a chart is too small to read — these are all findings worth reporting
even when the underlying data is correct.

**Question consistency.** If one chart uses green for wins and another uses a different shade,
if one tooltip says "Avg Placement" and another says "Average", if date formats differ
between charts — flag it.

**Question analytical usefulness.** A chart can render correctly but show the wrong
*kind* of data. For every aggregate chart, ask: is this metric actionable? Does the
aggregation method (total vs average vs median) actually help the user make decisions?

Common traps to watch for:
- **Totals that should be averages.** A bar chart showing "total buffs across all games"
  penalizes categories that appear in fewer games. Per-game averages are almost always
  more useful — they answer "when this happens, how impactful is it?" rather than "how
  many games had this?"
- **Counts without normalization.** A placement distribution with raw counts is fine, but
  a "hero performance" chart showing total wins (not win rate) would be misleading for
  heroes with different game counts.
- **Averages that hide variance.** An avg placement of 3.0 could mean "always 3rd" or
  "half 1st, half 5th" — if the chart doesn't show spread or game count, flag it.
- **Misleading axis labels.** If the y-axis says "Total" but the data would be more useful
  as "Avg per Game", the label is technically correct but the chart is unhelpful.
- **Cross-game aggregates vs per-game snapshots.** Level 1 charts aggregate across games;
  Level 2 charts show a single game. Make sure the aggregation method on Level 1 produces
  insights you can't get from just scrolling through Level 2 views.

When you find a metric that's technically correct but analytically weak, report it as a
**DATA** severity finding (distinct from BUG/UX/SUGGESTION):
- **DATA**: The chart renders correctly but the underlying metric or aggregation method
  reduces its analytical value. Include what the current metric is, why it's weak, and
  what would be more useful.

### Exploratory Analysis Phase

After running the numbered checks, do an exploratory pass:

1. **Drill into at least one game** and examine every Level-2 chart. For each chart, check:
   - Does it have actual data, or is it flat/empty?
   - If empty, what data would you expect to see here?
   - Does the chart type make sense for the data being shown?

2. **Switch between modes** (All, Solo, Duos, Compare) and look for:
   - Charts that don't adapt to the mode
   - Missing partner data in Duos mode
   - Compare mode showing useful side-by-side comparisons

3. **Look at edge cases**: What happens with 0 games? 1 game? Many games?

Report exploratory findings in a separate "Observations" section of the report, distinct
from the pass/fail checks. Use severity levels:
- **BUG**: Something is clearly broken (empty data that should exist, wrong colors, JS errors)
- **UX**: Works but is confusing or unhelpful to users (overlapping labels, misleading charts)
- **DATA**: Chart renders correctly but the metric or aggregation method is analytically weak
- **SUGGESTION**: Ideas for improvement (missing features, better layouts)

### Duos-Specific Expectations

Battlegrounds Duos is a fundamentally different game mode from Solo. A well-designed
dashboard should reflect these differences. When in Duos mode or viewing Duos games, look
for and report on:

**Player vs Partner separation:**
- Charts that show per-turn data (board stats, health/armor, tier progression, buff
  accumulation) should ideally have separate lines/series for the player and their partner.
  If only the player's data is shown, flag as a UX suggestion.
- Heatmaps that exist for the player (hero placement, tier-turn, tribe, buff) should
  ideally have partner variants or a way to toggle between player/partner views.

**Duos-specific metrics:**
- Win threshold is top 2 (not top 4) — all color coding must reflect this
- Partner name should be visible in game headers and filters
- Health/Armor is shared in Duos (team pool) — charts should label it accordingly
- Board composition matters differently: the combined board across both players determines
  tribe synergies, so the "Tribe Win Rate" chart should reflect the combination of minion
  types across both boards, not just the player's board

**Compare mode:**
- Should show Solo vs Duos side-by-side with dual series on trend charts
- Stats should be segmented (solo avg placement vs duos avg placement)

If any of these are missing, report them as UX suggestions, not failures — they represent
the difference between a good dashboard and a great one.

## How to Run Checks

Read the check definitions from `references/checks-dashboard.md`. That file contains all
numbered checks with their groups, steps, and expected values.

### Full QA Run (default)

Run all numbered checks in order, then do the exploratory analysis phase.

### Subset Run

When the user asks for specific areas (e.g., "just check axis labels", "test the filters",
"verify chart-hero-perf"), match by:
- **Check name or ID** — "check 5" or "axis labels" runs Check 5
- **Group name** — "filters" runs Checks 8, 9, 10
- **Chart ID** — "chart-hero-perf" runs relevant checks for that chart

Even on subset runs, apply the QA mindset to whatever you're looking at — if you notice
something wrong while checking axis labels, report it.

### Check Execution Pattern

Checks are divided into two phases to minimize round-trips:

**Phase 1 — Read-only checks (single batched evaluate):**
Checks 1-7, 15-17 only read DOM/Charts/State without clicking or typing. Combine them into
one or two `browser_evaluate` calls that return all results at once. This is much faster than
17 separate evaluations.

Example batched pattern:
```javascript
(() => {
  const r = {};
  // Check 2: duplicates
  const games = [...State.fullGames.values()];
  // ... duplicate logic ...
  r.duplicates = dupes;
  // Check 3: chart rendering
  r.charts = {};
  for (const id of expectedL1) { /* ... */ }
  // Check 4: tooltips
  r.tooltips = {};
  // Check 5: axis labels
  r.axes = {};
  // ... etc ...
  return r;
})()
```

**Phase 2 — Interactive checks (sequential):**
Checks 8-14 involve clicking, typing, and waiting. Run these sequentially since each
modifies page state. Use `browser_fill_form`, `browser_click`, then `browser_evaluate`
to verify results.

For each check:
1. Read the check steps from `references/checks-dashboard.md`
2. Execute using Playwright MCP tools
3. Record result: PASS or FAIL with details
4. On failure: take a screenshot of the current state and note expected vs actual values

### Dashboard Globals

The dashboard exposes these JavaScript globals accessible via `browser_evaluate`:

- **`State`** — application state object:
  - `State.mode` — `'all'`, `'solo'`, `'duos'`, or `'compare'`
  - `State.level` — navigation level (1, 2, or 3)
  - `State.games` — filtered game metadata array
  - `State.fullGames` — `Map<gameID, fullGame>` (all loaded games)
  - `State.selectedGameID` — current drill-down game (null on Level 1)
  - `State.lastN` — Last N games filter value (0 = disabled)
  - `State.lastDays` — Last N days filter value (0 = disabled)
  - `State.partners` — `Set` of partner names
  - `State.excludedPartners` — `Set` of partners filtered out

- **`Charts`** — object mapping container IDs to ECharts instances. Access options via:
  ```javascript
  Charts['chart-placement-trend'].getOption()
  ```

- **`timelineChart`** — the scrubber ECharts instance (scatter + dataZoom)

- **`allUnfilteredGames`** — full game array before any filters

- **`scrubberManual`** — boolean flag, true when user manually dragged scrubber handles

### ECharts Option Inspection

To check axis properties:
```javascript
const opts = Charts['chart-placement-trend'].getOption();
opts.yAxis[0].nameLocation;    // 'start' or 'end'
opts.yAxis[0].inverse;         // true or false
opts.xAxis[0].nameGap;         // 25 or 35
```

To check series data:
```javascript
const opts = Charts['chart-hero-perf'].getOption();
opts.series[0].data.length;    // should be > 0
```

### Axis Label Constants

The dashboard defines these helpers in app.js:
```javascript
const AXIS_NAME_STYLE = { color: '#888', fontSize: 11 };
// xName(label)        → nameLocation: 'center', nameGap: 25
// xNameHeatmap(label)  → nameLocation: 'center', nameGap: 35
// yName(label)        → nameLocation: 'end'
// yNameInverse(label) → nameLocation: 'start' (for inverted axes)
```

## Reporting

### Console Output

**On success:**
```
Dashboard QA: all N checks passed (M charts verified, K filters tested)
```

**On failure:** report each failing check with check ID, expected vs actual, and chart ID.

### HTML Report

After all checks complete, generate a self-contained HTML report and save it to
`qa-reports/` in the project root (this directory is gitignored). The filename should
include a timestamp: `qa-reports/dashboard-qa-YYYY-MM-DD-HHMMSS.html`.

Create the directory if it doesn't exist: `mkdir -p qa-reports/`

The HTML report must be **fully self-contained** (inline CSS, no external dependencies)
and include:

1. **Header**: timestamp, URL tested, total games, pass/fail summary with color coding
2. **Check results table**: each check as a row with ID, name, group, PASS/FAIL badge,
   and details column. Green for pass, red for fail. Failures should be expanded by
   default showing expected vs actual values.
3. **Screenshots**: embed any captured screenshots as base64 `<img>` tags inline in the
   HTML. Take a full-page screenshot at the end and embed it as "Final State".
4. **Dark theme**: use the same dark palette as the dashboard (`#1a1a2e` bg, `#eee` text,
   `#e94560` accent for failures, `#00c853` for passes)

To embed a screenshot as base64, after taking a screenshot with `browser_take_screenshot`,
read the PNG file with the Read tool and use the image data. Alternatively, use
`browser_evaluate` to capture a data URL if the screenshot tool doesn't return base64.

Since embedding large base64 images can be unwieldy, an acceptable alternative is to save
screenshots as separate PNGs in the same `qa-reports/` directory and reference them with
relative `<img src="...">` paths in the HTML.

**Template structure:**
```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Dashboard QA Report - {timestamp}</title>
  <style>/* Dark theme, table styles, badge styles inline */</style>
</head>
<body>
  <h1>Dashboard QA Report</h1>
  <div class="summary">...</div>

  <h2>Check Results</h2>
  <table class="checks">
    <tr class="pass"><td>1</td><td>Page Load</td><td>core</td><td>PASS</td><td>...</td></tr>
    <tr class="fail"><td>19</td><td>Scrubber Axis</td><td>scrubber</td><td>FAIL</td><td>...</td></tr>
  </table>

  <h2>Observations</h2>
  <!-- Exploratory findings from the QA mindset pass -->
  <div class="observation bug">
    <span class="severity">BUG</span>
    <p>Board Stats Over Time shows flat lines at zero for all turns...</p>
  </div>
  <div class="observation ux">
    <span class="severity">UX</span>
    <p>Duos game detail has no partner data series on charts...</p>
  </div>
  <div class="observation suggestion">
    <span class="severity">SUGGESTION</span>
    <p>Tribe Win Rate should show combined board composition...</p>
  </div>

  <h2>Screenshots</h2>
  <img src="screenshot-final.png" />
</body>
</html>
```

Use CSS classes for severity: `.bug` gets red-left-border, `.ux` gets orange, `.data` gets
yellow, `.suggestion` gets blue. The observations section is where the QA mindset findings
go — things you noticed that aren't covered by numbered checks.

Tell the user the report path when done so they can open it in a browser.

## Extending the Checks

This skill is designed to grow. When the user identifies a new issue or a chart is added:

1. Read `references/checks-dashboard.md`
2. Add a new check with the next available ID, following the existing format
3. If it involves axis labels for a new chart, update the axis table in Check 5
4. If a pattern applies to all charts, add it to the relevant existing check

The user may say "add a check for X" or "I found a bug where Y" — treat these as requests
to extend the check catalog in `references/checks-dashboard.md`, then run the new check.

### Adding a New Check

Follow this format in `references/checks-dashboard.md`:
```markdown
### Check N: Descriptive Name
**Group:** group-name
**Steps:**
1. Description of what to do
2. Evaluate: `javascript expression to run`
3. Verify: what the result should be
```
