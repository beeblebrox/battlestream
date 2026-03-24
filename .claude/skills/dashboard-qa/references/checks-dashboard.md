# Dashboard QA Checks

All checks for validating the Battlestream dashboard. Each check has an ID, group, and
step-by-step validation instructions using Playwright browser tools.

## Table of Contents

| Check | Name                   | Group        |
|-------|------------------------|--------------|
| 1     | Page Load & Data       | core         |
| 2     | No Duplicate Games     | data         |
| 3     | Chart Rendering        | charts       |
| 4     | Help Tooltips          | charts       |
| 5     | Axis Label Rules       | charts       |
| 6     | Tribe Data Integrity   | data         |
| 7     | Hero Name Resolution   | data         |
| 8     | Filter: Last N Games   | filters      |
| 9     | Filter: Last N Days    | filters      |
| 10    | Filter: Reset          | filters      |
| 11    | Timeline Scrubber      | filters      |
| 12    | Navigation & Drill-down| navigation   |
| 13    | Mode Toggle            | filters      |
| 14    | Partner Filter         | filters      |
| 15    | Loading States         | visual       |
| 16    | Tavern Tier 7          | data         |
| 17    | Visual Consistency     | visual       |
| 18    | Duos Win Threshold Colors | duos      |
| 19    | Scrubber Inverted Axis | scrubber     |
| 20    | Scrubber Handle Stability | scrubber  |
| 21    | Scrubber Zoom Proportional | scrubber |
| 22    | Duos-Aware Grouping    | duos         |
| 23    | Partner Decal Styling  | duos         |

---

### Check 1: Page Load & Data
**Group:** core
**Steps:**
1. Navigate to `http://127.0.0.1:8080/dashboard/`
2. Verify page title is `"Battlestream Dashboard"`
3. Evaluate: `State.fullGames.size` — wait until > 0 (timeout 10s)
4. Snapshot the page and verify summary cards show non-zero "Games Played"
5. Evaluate console messages: verify no JS errors (ignore favicon 404s)
   Use `browser_console_messages` and filter for `error` level, excluding
   messages containing `favicon`

---

### Check 2: No Duplicate Games
**Group:** data
**Steps:**
1. Evaluate:
   ```javascript
   (() => {
     const games = [...State.fullGames.values()];
     const timestamps = games.map(g => g.start_time_unix);
     const dupes = [];
     for (let i = 0; i < timestamps.length; i++) {
       for (let j = i + 1; j < timestamps.length; j++) {
         const diff = Math.abs(timestamps[i] - timestamps[j]);
         if (Math.abs(diff - 86400) < 100) {
           dupes.push([timestamps[i], timestamps[j], diff]);
         }
       }
     }
     return { count: games.length, duplicates: dupes };
   })()
   ```
2. Verify: `duplicates` array is empty (no pairs exactly 24h apart within 100s tolerance)
3. Verify: `count` matches `allUnfilteredGames.length` (no inflation from reparse bugs)

---

### Check 3: Chart Rendering
**Group:** charts
**Steps:**
1. Evaluate:
   ```javascript
   (() => {
     const expectedL1 = [
       'chart-placement-trend', 'chart-placement-dist', 'chart-winrate-trend',
       'chart-hero-perf', 'chart-tavern-tier', 'chart-buff-breakdown',
       'chart-duration', 'chart-anomaly-perf', 'chart-tribe-winrate',
       'chart-buff-efficiency', 'chart-heatmap-hero', 'chart-heatmap-tier-turn',
       'chart-heatmap-tribe', 'chart-heatmap-buff'
     ];
     const results = {};
     for (const id of expectedL1) {
       const chart = Charts[id];
       if (!chart) { results[id] = 'MISSING'; continue; }
       const opts = chart.getOption();
       const hasSeries = opts.series && opts.series.length > 0;
       const hasData = hasSeries && opts.series.some(s => s.data && s.data.length > 0);
       results[id] = hasData ? 'OK' : 'EMPTY_SERIES';
     }
     return results;
   })()
   ```
2. Verify: all 14 charts return `'OK'` (exist and have non-empty series data)
3. If any chart is `MISSING` or `EMPTY_SERIES`, report which ones failed

---

### Check 4: Help Tooltips
**Group:** charts
**Steps:**
1. Evaluate:
   ```javascript
   (() => {
     const ids = [
       'chart-placement-trend', 'chart-placement-dist', 'chart-winrate-trend',
       'chart-hero-perf', 'chart-tavern-tier', 'chart-buff-breakdown',
       'chart-duration', 'chart-anomaly-perf', 'chart-tribe-winrate',
       'chart-buff-efficiency', 'chart-heatmap-hero', 'chart-heatmap-tier-turn',
       'chart-heatmap-tribe', 'chart-heatmap-buff'
     ];
     const results = {};
     for (const id of ids) {
       const help = document.querySelector('#' + id + ' h3 .chart-help');
       if (!help) { results[id] = 'MISSING'; continue; }
       results[id] = help.title ? 'OK' : 'EMPTY_TITLE';
     }
     return results;
   })()
   ```
2. Verify: all 14 chart containers have a `.chart-help` element with a non-empty `title`

---

### Check 5: Axis Label Rules
**Group:** charts

This check validates axis naming, positioning, and styling for all charts.

**Axis requirements table:**

| Chart ID                | X-axis name         | Y-axis name    | Y inverted? | Notes                          |
|-------------------------|---------------------|----------------|-------------|--------------------------------|
| chart-placement-trend   | Date                | Placement      | yes         |                                |
| chart-placement-dist    | Placement           | Games          | no          |                                |
| chart-winrate-trend     | Game #              | %              | no          |                                |
| chart-hero-perf         | Avg Placement       | (none)         | yes         | Horizontal bar, y sorts heroes |
| chart-tavern-tier       | Tier                | Games          | no          |                                |
| chart-buff-breakdown    | Game                | Category       | no          | Per-game heatmap, nameGap=35   |
| chart-duration          | Duration            | Games          | no          | Histogram by time bucket       |
| chart-anomaly-perf      | Avg Placement       | (none)         | yes         | Horizontal bar, y sorts anomalies |
| chart-tribe-winrate     | Avg Placement       | (none)         | yes         | Horizontal bar, y sorts tribes |
| chart-buff-efficiency   | Total Buff (ATK+HP) | Avg Placement  | yes         |                                |
| chart-heatmap-hero      | Placement           | Hero           | no          |                                |
| chart-heatmap-tier-turn | Turn                | Tier           | no          |                                |
| chart-heatmap-tribe     | Placement           | Tribe          | no          |                                |
| chart-heatmap-buff      | Placement           | Buff Total     | no          |                                |

**Steps:**
1. For each chart in the table above, evaluate:
   ```javascript
   (() => {
     const chart = Charts['<chart-id>'];
     if (!chart) return { error: 'chart not found' };
     const opts = chart.getOption();
     const x = opts.xAxis && opts.xAxis[0];
     const y = opts.yAxis && opts.yAxis[0];
     return {
       xName: x && x.name,
       xNameLocation: x && x.nameLocation,
       xNameGap: x && x.nameGap,
       xNameTextStyle: x && x.nameTextStyle,
       yName: y && y.name,
       yNameLocation: y && y.nameLocation,
       yNameTextStyle: y && y.nameTextStyle,
       yInverse: y && y.inverse
     };
   })()
   ```
2. Verify per chart:
   - **X-axis name** matches table (if listed)
   - **X-axis nameLocation** is `'center'` for all charts with a named x-axis
   - **X-axis nameGap**: `35` for heatmap charts (IDs containing "heatmap") and `chart-buff-breakdown`, `25` for others
   - **Y-axis nameLocation**: `'start'` if `inverse: true`, `'end'` if not inverted
   - **nameTextStyle** on all named axes: `{ color: '#888', fontSize: 11 }`
   - Charts with `(none)` for Y-axis may omit the y-axis name entirely

---

### Check 6: Tribe Data Integrity
**Group:** data
**Steps:**
1. Evaluate:
   ```javascript
   (() => {
     const games = [...State.fullGames.values()];
     let gamesWithTribes = 0;
     for (const g of games) {
       if (g.board && g.board.some(m => m.minion_type && m.minion_type !== '')) {
         gamesWithTribes++;
       }
     }
     return { total: games.length, withTribes: gamesWithTribes };
   })()
   ```
2. Verify: `withTribes` > 0 (at least some games have tribe data on board minions)
3. Check the Tribe Win Rate chart labels:
   ```javascript
   (() => {
     const opts = Charts['chart-tribe-winrate'].getOption();
     const labels = opts.yAxis[0].data || opts.xAxis[0].data || [];
     const hasConcentration = labels.some(l => typeof l === 'string' && l.includes('%'));
     return { labels, hasConcentration };
   })()
   ```
4. Verify: `hasConcentration` is `false` — labels should be base tribe names
   (DRAGON, DEMON, etc.), NOT concentration labels like "DRAGON (50%+)"

---

### Check 7: Hero Name Resolution
**Group:** data
**Steps:**
1. Evaluate:
   ```javascript
   (() => {
     const opts = Charts['chart-hero-perf'].getOption();
     const yData = opts.yAxis[0].data || [];
     const rawIDs = yData.filter(n =>
       typeof n === 'string' && (n.startsWith('TB_BaconShop_HERO_') || n.startsWith('BG'))
     );
     return { heroCount: yData.length, rawIDs, sample: yData.slice(0, 5) };
   })()
   ```
2. Verify: `rawIDs` is empty — no unresolved card IDs in the hero performance chart
3. Verify: `sample` contains human-readable names (e.g., "Deathwing", "Smuggler Eudora")
4. Skin variants should appear as "HeroName (Skin X)" format

---

### Check 8: Filter: Last N Games
**Group:** filters
**Steps:**
1. Use `browser_fill_form` to set the "Last N games" input (`#filter-last-n`) to `"2"`
2. Wait 500ms for the dashboard to re-render
3. Evaluate:
   ```javascript
   (() => {
     const sorted = [...allUnfilteredGames]
       .sort((a, b) => b.start_time_unix - a.start_time_unix);
     const expected = sorted.slice(0, 2).map(g => g.game_id);
     const actual = State.games.map(g => g.game_id);
     return {
       filteredCount: State.games.length,
       expectedIDs: expected,
       actualIDs: actual,
       match: JSON.stringify(expected) === JSON.stringify(actual)
     };
   })()
   ```
4. Verify: `filteredCount === 2`
5. Verify: `match === true` (the 2 games are the most recent by timestamp)
6. Snapshot the page: verify summary cards show "2" for Games Played
7. Evaluate scrubber zoom:
   ```javascript
   (() => {
     if (!timelineChart) return { error: 'no scrubber' };
     const opts = timelineChart.getOption();
     const dz = opts.dataZoom[0];
     return { start: dz.start, end: dz.end };
   })()
   ```
8. Verify: `start > 90` (scrubber snapped to right end showing only recent games)
9. Snapshot: verify filter status text contains "Showing 2 of"

---

### Check 9: Filter: Last N Days
**Group:** filters
**Steps:**
1. Click "Reset Filters" button first (use `browser_click` on the reset button)
2. Wait 500ms
3. Use `browser_fill_form` to set "Last N days" input (`#filter-last-days`) to `"1"`
4. Wait 500ms
5. Evaluate:
   ```javascript
   (() => {
     const cutoff = Date.now() - 86400000;
     const expected = allUnfilteredGames.filter(g =>
       g.start_time_unix * 1000 >= cutoff
     ).length;
     return {
       filteredCount: State.games.length,
       expectedCount: expected,
       match: State.games.length === expected
     };
   })()
   ```
6. Verify: `match === true`
7. If all games are older than 1 day, verify `filteredCount === 0`

---

### Check 10: Filter: Reset
**Group:** filters
**Steps:**
1. Click "Reset Filters" button
2. Wait 500ms
3. Evaluate:
   ```javascript
   (() => {
     const scrubberOpts = timelineChart ? timelineChart.getOption().dataZoom[0] : null;
     return {
       gameCount: State.games.length,
       totalCount: allUnfilteredGames.length,
       match: State.games.length === allUnfilteredGames.length,
       scrubberStart: scrubberOpts && scrubberOpts.start,
       scrubberEnd: scrubberOpts && scrubberOpts.end
     };
   })()
   ```
4. Verify: `match === true` (filtered count equals total)
5. Verify: `scrubberStart === 0` and `scrubberEnd === 100` (full range)
6. Snapshot: verify filter status text is empty or not shown

---

### Check 11: Timeline Scrubber
**Group:** filters
**Steps:**
1. Evaluate:
   ```javascript
   (() => {
     if (!timelineChart) return { exists: false };
     const opts = timelineChart.getOption();
     const series = opts.series[0];
     const dz = opts.dataZoom[0];
     const bgStyle = dz.dataBackground && dz.dataBackground.lineStyle;
     return {
       exists: true,
       hasData: series && series.data && series.data.length > 0,
       dataLength: series && series.data ? series.data.length : 0,
       bgLineOpacity: bgStyle ? bgStyle.opacity : 'not set'
     };
   })()
   ```
2. Verify: `exists === true`
3. Verify: `hasData === true` with `dataLength > 0` (scatter dots visible)
4. Verify: `bgLineOpacity === 0` (no line in slider background, only dots)
5. Check scrubberManual flag behavior:
   - After setting "Last N games" to "2": evaluate `scrubberManual` — should be `false`
   - This confirms that typing in a filter input resets the manual flag

---

### Check 12: Navigation & Drill-down
**Group:** navigation
**Steps:**
1. First, ensure we're on Level 1. Evaluate `State.level` — should be `1`
2. Get a game ID to drill into:
   ```javascript
   State.games[0] ? State.games[0].game_id : null
   ```
3. If a game ID exists, drill into it:
   ```javascript
   drillToGame('<game_id>')
   ```
4. Wait 1s for Level 2 to render
5. Snapshot the page and verify:
   - Level 2 container (`#level-2`) is visible
   - Level 1 container (`#level-1`) is hidden
   - A "Back" button or breadcrumb link exists
   - Breadcrumb shows "Overview / Game ..."
6. Evaluate: `State.selectedGameID` — should equal the drilled game ID
7. Click the Back button (or navigate via breadcrumb to Level 1)
8. Wait 500ms
9. Evaluate: `State.selectedGameID` — should be `null`
10. Evaluate: `State.level` — should be `1`

---

### Check 13: Mode Toggle
**Group:** filters
**Steps:**
1. Click the "Duos" mode button (`.mode-btn[data-mode="duos"]`)
2. Wait 500ms
3. Evaluate: `State.mode` — should be `'duos'`
4. Snapshot: verify partner filter bar is visible (not hidden)
5. Click "Solo" mode button
6. Wait 500ms
7. Evaluate: `State.mode` — should be `'solo'`
8. Snapshot: verify partner filter bar is hidden
9. Click "All" mode button
10. Wait 500ms
11. Evaluate: `State.mode` — should be `'all'`
12. Verify all games are shown: `State.games.length === allUnfilteredGames.length`
    (only if no other filters are active)
13. If currently on Level 2, switching mode should return to Level 1:
    - Drill into a game, then switch mode
    - Evaluate: `State.level` — should be `1`

---

### Check 14: Partner Filter (Duos mode)
**Group:** filters
**Steps:**
1. Switch to Duos mode (click `.mode-btn[data-mode="duos"]`)
2. Wait 500ms
3. Evaluate:
   ```javascript
   (() => {
     const tags = document.querySelectorAll('#partner-filter .partner-tag');
     const names = [...tags].map(t => t.textContent.trim());
     const hasEmpty = names.some(n => n === '' || n === 'undefined');
     return { count: names.length, names, hasEmpty };
   })()
   ```
4. Verify: partner tags show names (not hero card IDs, not empty strings)
5. If `count === 0`, this is acceptable if no duos games exist in the dataset
6. If `hasEmpty === true`, flag as warning (known limitation if battletags missing from Power.log)

---

### Check 15: Loading States
**Group:** visual
**Steps:**
1. Evaluate:
   ```javascript
   (() => {
     const overlay = document.getElementById('loading-overlay');
     const overlayVisible = overlay && !overlay.classList.contains('hidden');
     const chartIds = Object.keys(Charts);
     const loadingCharts = chartIds.filter(id => {
       const c = Charts[id];
       return c && c._loadingFX && Object.keys(c._loadingFX).length > 0;
     });
     return {
       overlayVisible,
       loadingCharts,
       chartsDisposed: chartIds.filter(id => Charts[id] && Charts[id].isDisposed())
     };
   })()
   ```
2. Verify: `overlayVisible === false` (loading overlay should be hidden after data loads)
3. Verify: `loadingCharts` is empty (no chart should still have a loading spinner)
4. Verify: `chartsDisposed` is empty (no chart should be disposed)

---

### Check 16: Tavern Tier 7
**Group:** data
**Steps:**
1. Evaluate tavern tier chart:
   ```javascript
   (() => {
     const opts = Charts['chart-tavern-tier'].getOption();
     const xData = opts.xAxis[0].data || [];
     const hasT7 = xData.some(d => d === 'T7' || d === '7' || d === 7);
     return { xData, hasT7 };
   })()
   ```
2. Verify: `hasT7 === true` (x-axis includes Tier 7)
3. Evaluate tier-turn heatmap:
   ```javascript
   (() => {
     const opts = Charts['chart-heatmap-tier-turn'].getOption();
     const yData = opts.yAxis[0].data || [];
     const hasT7 = yData.some(d => d === 'T7' || d === '7' || d === 7);
     return { yData, hasT7 };
   })()
   ```
4. Verify: `hasT7 === true` (y-axis includes Tier 7)
5. If on Level 2 (game detail), check tier progression chart:
   ```javascript
   (() => {
     const chart = Charts['chart-tier-prog'];
     if (!chart) return { skipped: true };
     const opts = chart.getOption();
     return { yMax: opts.yAxis[0].max };
   })()
   ```
6. If available, verify: `yMax >= 7`

---

### Check 17: Visual Consistency
**Group:** visual
**Steps:**
1. Take a full-page screenshot using `browser_take_screenshot`
2. Evaluate dark theme colors:
   ```javascript
   (() => {
     const style = getComputedStyle(document.documentElement);
     return {
       bg: style.getPropertyValue('--bg').trim(),
       text: style.getPropertyValue('--text').trim(),
       accent: style.getPropertyValue('--accent').trim(),
       bgCard: style.getPropertyValue('--bg-card').trim()
     };
   })()
   ```
3. Verify:
   - `bg` is `#1a1a2e` (dark background)
   - `text` is `#eee` (light text)
   - `accent` is `#e94560` (pink accent)
   - `bgCard` is `#16213e` (card background)
4. Evaluate: check no charts show empty/"No data" state when games exist:
   ```javascript
   (() => {
     const chartIds = Object.keys(Charts);
     const empty = chartIds.filter(id => {
       const opts = Charts[id].getOption();
       return !opts.series || opts.series.length === 0 ||
              opts.series.every(s => !s.data || s.data.length === 0);
     });
     return { empty };
   })()
   ```
5. Verify: `empty` array is empty (all charts have data when games exist)

---

### Check 18: Duos Win Threshold Colors
**Group:** duos

Charts that color-code placements by win/loss must use the correct threshold for the current
mode. In solo mode, top 4 = win (green). In duos mode, top 2 = win (green). This must be
consistent across ALL charts that use win/loss coloring.

**Steps:**
1. Switch to Duos mode (click `.mode-btn[data-mode="duos"]`)
2. Wait 500ms for re-render
3. Evaluate the Placement Distribution chart colors:
   ```javascript
   (() => {
     const opts = Charts['chart-placement-dist'].getOption();
     const series = opts.series[0];
     if (!series || !series.data) return { error: 'no series data' };
     // Check color assignments per placement bar
     const colors = series.data.map((d, i) => {
       const color = typeof d === 'object' ? d.itemStyle?.color : null;
       return { placement: i + 1, color };
     });
     // In duos: placements 1-2 should be green (win), 3+ should be red (loss)
     const winColor = getComputedStyle(document.documentElement).getPropertyValue('--win').trim();
     const lossColor = getComputedStyle(document.documentElement).getPropertyValue('--loss').trim();
     return { colors, winColor, lossColor, mode: State.mode };
   })()
   ```
4. Verify: In duos mode, only placements 1-2 are colored with `--win` (green), 3+ with `--loss` (red)
5. Also check the timeline scrubber dot colors use the same threshold:
   ```javascript
   (() => {
     if (!timelineChart) return { error: 'no scrubber' };
     const opts = timelineChart.getOption();
     const data = opts.series[0].data;
     // Sample a few points and check their colors
     const samples = data.slice(0, 5).map(d => ({
       placement: d[1],
       color: d[2] || (typeof d.itemStyle !== 'undefined' ? d.itemStyle.color : null)
     }));
     return { samples, mode: State.mode };
   })()
   ```
6. Verify: scrubber dot colors match the duos win threshold (top 2 = win)
7. Switch to Solo mode and repeat — verify top 4 = win threshold is used
8. Switch back to All mode when done

---

### Check 19: Scrubber Inverted Axis
**Group:** scrubber

The timeline scrubber y-axis should be inverted so that lower placements (wins, e.g. 1st)
appear higher on the chart, matching the visual convention used in the main placement trend
chart. This makes wins visually prominent at the top.

**Steps:**
1. Evaluate:
   ```javascript
   (() => {
     if (!timelineChart) return { exists: false };
     const opts = timelineChart.getOption();
     const yAxis = opts.yAxis[0];
     return {
       exists: true,
       inverse: yAxis.inverse,
       min: yAxis.min,
       max: yAxis.max
     };
   })()
   ```
2. Verify: `inverse === true` (y-axis is inverted so placement 1 is at top)
3. Verify: `min` and `max` bound the placement range appropriately (e.g., min=1, max=8)

---

### Check 20: Scrubber Handle Stability
**Group:** scrubber

Moving either the left or right dataZoom handle on the timeline scrubber should only change
the visible date range — it must NOT cause the timeline's x-axis scale to change or the
scatter data to re-layout. The x-axis should always show the full date range of all games
regardless of handle position.

**Steps:**
1. Record the initial x-axis range:
   ```javascript
   (() => {
     if (!timelineChart) return { error: 'no scrubber' };
     const opts = timelineChart.getOption();
     const xAxis = opts.xAxis[0];
     return {
       xMin: xAxis.min,
       xMax: xAxis.max,
       dataLength: opts.series[0].data.length
     };
   })()
   ```
2. Apply a filter (set Last N games to 5) and wait 500ms
3. Evaluate the x-axis range again:
   ```javascript
   (() => {
     if (!timelineChart) return { error: 'no scrubber' };
     const opts = timelineChart.getOption();
     const xAxis = opts.xAxis[0];
     const dz = opts.dataZoom[0];
     return {
       xMin: xAxis.min,
       xMax: xAxis.max,
       dataLength: opts.series[0].data.length,
       dzStart: dz.start,
       dzEnd: dz.end
     };
   })()
   ```
4. Verify: `xMin` and `xMax` are unchanged (the axis scale did not change)
5. Verify: `dataLength` is unchanged (all data points still present in the scatter)
6. Verify: only `dzStart`/`dzEnd` changed (the handles moved, not the scale)
7. Reset filters when done

---

### Check 21: Scrubber Zoom Proportional Resizing
**Group:** scrubber

When zooming in on the timeline scrubber (e.g., via mouse wheel), the selection area between
the handles should proportionally grow larger to fill more of the slider width, since it now
represents a smaller time window within the same total range. The selection should grow until
it fills the full width. Conversely, zooming out should shrink the selection proportionally
while covering the same time period.

**Steps:**
1. Record initial scrubber state:
   ```javascript
   (() => {
     if (!timelineChart) return { error: 'no scrubber' };
     const opts = timelineChart.getOption();
     const dz = opts.dataZoom[0];
     return {
       start: dz.start,
       end: dz.end,
       span: dz.end - dz.start
     };
   })()
   ```
2. Apply "Last N games = 3" filter, wait 500ms
3. Record the filtered scrubber state:
   ```javascript
   (() => {
     if (!timelineChart) return { error: 'no scrubber' };
     const opts = timelineChart.getOption();
     const dz = opts.dataZoom[0];
     return {
       start: dz.start,
       end: dz.end,
       span: dz.end - dz.start,
       gameCount: State.games.length,
       totalCount: allUnfilteredGames.length
     };
   })()
   ```
4. Verify: the selection `span` is smaller than 100 (not showing full range)
5. Verify: the selection covers exactly the time range of the filtered games
6. Apply "Last N games = 1" filter, wait 500ms
7. Record the state again
8. Verify: the `span` is smaller than the 3-game span (tighter zoom for fewer games)
9. Reset filters when done

---

### Check 22: Duos-Aware Grouping
**Group:** duos

Validates that charts use duos-appropriate grouping when games are duos.

**Steps:**
1. Switch to All or Duos mode (ensure duos games exist in dataset)
2. Evaluate Hero Performance chart y-axis labels:
   ```javascript
   (() => {
     const opts = Charts['chart-hero-perf'].getOption();
     const labels = opts.yAxis[0].data || [];
     const hasPairings = labels.some(l => typeof l === 'string' && l.includes(' + '));
     const hasSkinSuffix = labels.some(l => typeof l === 'string' && l.includes('(Skin'));
     return { labels, hasPairings, hasSkinSuffix };
   })()
   ```
3. Verify: `hasPairings === true` — duos games show hero pairings like "HeroA + HeroB"
4. Verify: `hasSkinSuffix === false` — no skin variant suffixes in hero names
5. Evaluate Hero Placement Heatmap y-axis labels — same check as above for pairings
6. Drill into a duos game. Evaluate Level 2 charts for partner series:
   ```javascript
   (() => {
     const boardOpts = Charts['chart-board-stats']?.getOption();
     const tierOpts = Charts['chart-tier-prog']?.getOption();
     const sizeOpts = Charts['chart-board-size']?.getOption();
     const buffOpts = Charts['chart-buff-accum']?.getOption();
     return {
       boardSeriesCount: boardOpts?.series?.length,
       boardHasPartner: boardOpts?.series?.some(s => s.name?.includes('Partner')),
       tierSeriesCount: tierOpts?.series?.length,
       tierHasPartner: tierOpts?.series?.some(s => s.name?.includes('Partner')),
       sizeSeriesCount: sizeOpts?.series?.length,
       sizeHasPartner: sizeOpts?.series?.some(s => s.name?.includes('Partner')),
       buffHasPartner: buffOpts?.series?.some(s => s.name?.includes('Partner')),
       healthLabel: Charts['chart-health-armor']?.getOption()?.yAxis?.[0]?.name,
     };
   })()
   ```
7. Verify: `boardHasPartner === true` (Board Stats has partner ATK/HP dashed series)
8. Verify: `tierHasPartner === true` (Tier Progression has partner tier dashed series)
9. Verify: `sizeHasPartner === true` (Board Size has partner board dashed series)
10. Verify: `healthLabel === 'Team HP'` (Health chart labeled for shared team pool)
11. Drill into a turn. Verify partner board section visible:
    ```javascript
    (() => {
      const board = document.getElementById('turn-board');
      const hasPartnerSection = board && board.innerHTML.includes('Partner Board');
      return { hasPartnerSection };
    })()
    ```
12. Verify: `hasPartnerSection === true`
13. Navigate back to Level 1. Verify summary cards include "Best Partner":
    ```javascript
    (() => {
      const cards = document.getElementById('summary-cards');
      const hasPartnerCard = cards && cards.innerHTML.includes('Best Partner');
      return { hasPartnerCard };
    })()
    ```
14. Verify: `hasPartnerCard === true` (when duos games exist)

---

### Check 23: Partner Decal Styling
**Group:** duos

Partner series on Level-1 charts must use the same base colors as their player counterparts,
distinguished by a diagonal stripe decal pattern overlay. This ensures aesthetic consistency
and accessibility. Scatter series use a different symbol shape (diamond) instead of decals.

**Charts with partner series:**
- `chart-tavern-tier` — Partner bar uses same purple (#7c4dff) + decal
- `chart-buff-breakdown` — Partner ATK uses same yellow (#ffc107) + decal, Partner HP uses same green (#00c853) + decal
- `chart-buff-efficiency` (Bar mode) — Partner bars use same win/loss colors + decal
- `chart-buff-efficiency` (Scatter mode) — Partner uses same purple (#ab47bc) + diamond symbol

**Steps:**
1. Ensure mode is All or Duos (so partner series appear)
2. Evaluate tavern tier partner series:
   ```javascript
   (() => {
     const opts = Charts['chart-tavern-tier'].getOption();
     const series = opts.series || [];
     const partner = series.find(s => s.name === 'Partner');
     const player = series.find(s => s.name === 'Player' || s.name === 'Games');
     if (!partner) return { hasPartner: false };
     return {
       hasPartner: true,
       partnerColor: partner.itemStyle?.color,
       playerColor: player?.itemStyle?.color,
       colorsMatch: partner.itemStyle?.color === player?.itemStyle?.color,
       hasDecal: !!partner.itemStyle?.decal
     };
   })()
   ```
3. Verify: `colorsMatch === true` (partner uses same color as player)
4. Verify: `hasDecal === true` (partner has decal pattern)
5. Evaluate buff breakdown heatmap player/partner row styling:
   ```javascript
   (() => {
     const opts = Charts['chart-buff-breakdown'].getOption();
     const series = opts.series || [];
     // Buff breakdown is a heatmap — player rows use blue tones, partner rows use orange tones.
     // Check that the visualMap or series encoding distinguishes player from partner.
     const yData = opts.yAxis?.[0]?.data || [];
     const hasPlayerRows = yData.some(l => typeof l === 'string' && !l.includes('*'));
     const hasPartnerRows = yData.some(l => typeof l === 'string' && l.includes('*'));
     return { hasPlayerRows, hasPartnerRows, yLabels: yData };
   })()
   ```
6. Verify: `hasPlayerRows === true` (player buff category rows present)
7. Verify: `hasPartnerRows === true` when duos games exist (partner rows marked with `*` suffix)
8. Visual: player rows should use blue color scale, partner rows should use orange color scale
9. Evaluate buff efficiency bar mode partner series:
   ```javascript
   (() => {
     const opts = Charts['chart-buff-efficiency'].getOption();
     const series = opts.series || [];
     const partner = series.find(s => s.name === 'Partner');
     if (!partner) return { hasPartner: false };
     const partnerData = partner.data || [];
     const hasDecal = partnerData.some(d => d?.itemStyle?.decal);
     return { hasPartner: true, hasDecal };
   })()
   ```
10. Verify: `hasDecal === true` (partner bars have decal in bar mode)
11. Switch buff efficiency to Scatter mode (click Scatter toggle)
12. Evaluate scatter partner series:
    ```javascript
    (() => {
      const opts = Charts['chart-buff-efficiency'].getOption();
      const series = opts.series || [];
      const player = series.find(s => s.name === 'Player' || s.name === 'Games');
      const partner = series.find(s => s.name === 'Partner');
      if (!partner) return { hasPartner: false };
      return {
        hasPartner: true,
        playerSymbol: player?.symbol || 'circle',
        partnerSymbol: partner?.symbol || 'circle',
        symbolsDiffer: (player?.symbol || 'circle') !== (partner?.symbol || 'circle'),
        colorsMatch: player?.itemStyle?.color === partner?.itemStyle?.color
      };
    })()
    ```
13. Verify: `symbolsDiffer === true` (partner uses different symbol, e.g. diamond)
14. Verify: `colorsMatch === true` (same base color for both)
