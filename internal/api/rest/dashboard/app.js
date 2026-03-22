// Battlestream Dashboard — Single-page ECharts application
// Depends on: echarts (global, loaded via script tag)

'use strict';

// ============================================================================
// 1. Data Layer
// ============================================================================

const API = {
  async fetchJSON(url) {
    const resp = await fetch(url);
    if (!resp.ok) throw new Error(`API error: ${resp.status} ${resp.statusText}`);
    return resp.json();
  },

  getAggregate(mode) {
    return this.fetchJSON(`/v1/stats/aggregate?mode=${encodeURIComponent(mode)}`);
  },

  async getAllGames(mode) {
    const pageSize = 200;
    let offset = 0;
    let all = [];
    let total = Infinity;

    while (offset < total) {
      const data = await this.fetchJSON(
        `/v1/stats/games?mode=${encodeURIComponent(mode)}&limit=${pageSize}&offset=${offset}`
      );
      total = data.total || 0;
      if (!data.games || data.games.length === 0) break;
      all = all.concat(data.games);
      offset += data.games.length;
    }
    return all;
  },

  getGame(id) {
    return this.fetchJSON(`/v1/game/${encodeURIComponent(id)}`);
  },

  getTurns(id) {
    return this.fetchJSON(`/v1/game/${encodeURIComponent(id)}/turns`);
  },

  getCardNames() {
    return this.fetchJSON('/v1/cardnames');
  },
};

// ============================================================================
// 2. State
// ============================================================================

const State = {
  mode: 'all',
  level: 1,
  games: [],
  fullGames: new Map(),
  turnData: new Map(),
  cardNames: {},            // cardID -> display name (loaded once from /v1/cardnames)
  selectedGameID: null,
  selectedTurn: null,
  partners: new Set(),
  excludedPartners: new Set(),
  lastN: 0,                 // 0 = all games
  lastDays: 0,               // 0 = no day limit
  dateFrom: null,            // Date or null
  dateTo: null,              // Date or null
};

// ============================================================================
// 3. Chart Manager
// ============================================================================

const Charts = {};

const CHART_DIV_CLASS = 'echart-root';

function getChart(containerId) {
  if (Charts[containerId]) return Charts[containerId];

  const container = document.getElementById(containerId);
  if (!container) return null;

  let div = container.querySelector(`.${CHART_DIV_CLASS}`);
  if (!div) {
    div = document.createElement('div');
    div.className = CHART_DIV_CLASS;
    div.style.width = '100%';
    div.style.height = 'calc(100% - 28px)';
    container.appendChild(div);
  }

  const chart = echarts.init(div, 'dark');
  Charts[containerId] = chart;
  return chart;
}

function disposeAll() {
  for (const id of Object.keys(Charts)) {
    Charts[id].dispose();
    delete Charts[id];
  }
}

function resizeAll() {
  for (const chart of Object.values(Charts)) {
    chart.resize();
  }
}

window.addEventListener('resize', () => resizeAll());

// ============================================================================
// 4. Navigation
// ============================================================================

function showLevel(n) {
  State.level = n;
  document.getElementById('level-1').classList.toggle('hidden', n !== 1);
  document.getElementById('level-2').classList.toggle('hidden', n !== 2);
  document.getElementById('level-3').classList.toggle('hidden', n !== 3);
  updateBreadcrumb();
}

function updateBreadcrumb() {
  const bc = document.getElementById('breadcrumb');
  const parts = [{ label: 'Overview', level: 1 }];

  if (State.level >= 2 && State.selectedGameID) {
    parts.push({ label: `Game ${State.selectedGameID.slice(0, 8)}`, level: 2 });
  }
  if (State.level >= 3 && State.selectedTurn != null) {
    parts.push({ label: `Turn ${State.selectedTurn}`, level: 3 });
  }

  bc.innerHTML = parts
    .map((p, i) => {
      if (i === parts.length - 1) return `<span>${p.label}</span>`;
      return `<a href="#" data-level="${p.level}">${p.label}</a><span class="sep">/</span>`;
    })
    .join('');

  bc.querySelectorAll('a[data-level]').forEach((a) => {
    a.addEventListener('click', (e) => {
      e.preventDefault();
      navigateTo(parseInt(a.dataset.level, 10));
    });
  });
}

function navigateTo(level) {
  if (level <= 1) {
    State.selectedGameID = null;
    State.selectedTurn = null;
    showLevel(1);
    renderLevel1();
  } else if (level === 2 && State.selectedGameID) {
    State.selectedTurn = null;
    showLevel(2);
    renderLevel2();
  }
}

// ============================================================================
// 5. Filters (Last N games, Last N days, Timeline scrubber)
// ============================================================================

let timelineChart = null;
let scrubberDebounce = null;

function initFilters() {
  const lastNInput = document.getElementById('filter-last-n');
  lastNInput.addEventListener('input', () => {
    State.lastN = parseInt(lastNInput.value, 10) || 0;
    refreshDashboard();
  });

  const lastDaysInput = document.getElementById('filter-last-days');
  lastDaysInput.addEventListener('input', () => {
    State.lastDays = parseInt(lastDaysInput.value, 10) || 0;
    if (State.lastDays > 0) {
      // Set date range from lastDays and clear the scrubber selection
      const now = new Date();
      State.dateTo = null;
      State.dateFrom = new Date(now.getTime() - State.lastDays * 86400000);
      // Reset scrubber to match
      if (timelineChart) {
        timelineChart.dispatchAction({ type: 'dataZoom', start: 0, end: 100 });
      }
    } else {
      State.dateFrom = null;
      State.dateTo = null;
    }
    refreshDashboard();
  });

  document.getElementById('filter-clear-all').addEventListener('click', () => {
    lastNInput.value = '';
    lastDaysInput.value = '';
    State.lastN = 0;
    State.lastDays = 0;
    State.dateFrom = null;
    State.dateTo = null;
    if (timelineChart) {
      timelineChart.dispatchAction({ type: 'dataZoom', start: 0, end: 100 });
    }
    refreshDashboard();
  });
}

// Build the timeline scrubber from all game metas (unfiltered).
// This is an ECharts scatter chart with dataZoom slider showing game dots.
function renderTimelineScrubber(allMetas) {
  const el = document.getElementById('timeline-scrubber');
  if (!el) return;

  if (!timelineChart) {
    timelineChart = echarts.init(el, 'dark');
    window.addEventListener('resize', () => timelineChart.resize());
  }

  if (!allMetas || allMetas.length === 0) {
    timelineChart.clear();
    return;
  }

  const sorted = [...allMetas].sort((a, b) => a.start_time_unix - b.start_time_unix);
  const data = sorted.map((g) => {
    const ts = g.start_time_unix * 1000;
    return {
      value: [ts, g.placement],
      itemStyle: { color: (g.is_duos ? (g.placement <= 2) : (g.placement <= 4)) ? '#00c853' : '#ff5252' },
    };
  });

  const minTs = sorted[0].start_time_unix * 1000;
  const maxTs = sorted[sorted.length - 1].start_time_unix * 1000;

  // Compute initial zoom percent from current date filters
  let startPct = 0;
  let endPct = 100;
  if (State.dateFrom || State.dateTo) {
    const range = maxTs - minTs || 1;
    if (State.dateFrom) startPct = Math.max(0, ((State.dateFrom.getTime() - minTs) / range) * 100);
    if (State.dateTo) endPct = Math.min(100, ((State.dateTo.getTime() - minTs) / range) * 100);
  }

  timelineChart.setOption({
    grid: { left: 50, right: 20, top: 8, bottom: 30 },
    xAxis: {
      type: 'time',
      axisLabel: { fontSize: 10, color: '#888' },
      splitLine: { show: false },
    },
    yAxis: {
      type: 'value', min: 1, max: 8, inverse: true, show: false,
    },
    series: [{
      type: 'scatter',
      symbolSize: 8,
      data,
    }],
    tooltip: {
      trigger: 'item',
      formatter: (p) => {
        const d = new Date(p.data.value[0]);
        return `${d.toLocaleDateString()} ${d.toLocaleTimeString()}<br/>Placement: #${p.data.value[1]}`;
      },
    },
    dataZoom: [{
      type: 'slider',
      xAxisIndex: 0,
      start: startPct,
      end: endPct,
      height: 20,
      bottom: 2,
      borderColor: '#444',
      backgroundColor: '#1a1a2e',
      fillerColor: 'rgba(233, 69, 96, 0.25)',
      handleStyle: { color: '#e94560', borderColor: '#e94560' },
      textStyle: { color: '#888', fontSize: 10 },
      dataBackground: {
        lineStyle: { opacity: 0 },
        areaStyle: { opacity: 0 },
      },
      selectedDataBackground: {
        lineStyle: { opacity: 0 },
        areaStyle: { opacity: 0 },
      },
    }],
  }, true);

  // Debounced handler: update date filters when scrubber moves
  timelineChart.off('datazoom');
  timelineChart.on('datazoom', (params) => {
    clearTimeout(scrubberDebounce);
    scrubberDebounce = setTimeout(() => {
      const option = timelineChart.getOption();
      const zoom = option.dataZoom[0];
      const startVal = zoom.startValue;
      const endVal = zoom.endValue;

      if (startVal != null && endVal != null) {
        State.dateFrom = new Date(startVal);
        State.dateTo = new Date(endVal);
      } else {
        // Percent-based — compute from data range
        const range = maxTs - minTs || 1;
        State.dateFrom = (zoom.start > 0) ? new Date(minTs + (zoom.start / 100) * range) : null;
        State.dateTo = (zoom.end < 100) ? new Date(minTs + (zoom.end / 100) * range) : null;
      }

      // Clear last-days input since user is manually scrubbing
      document.getElementById('filter-last-days').value = '';
      State.lastDays = 0;

      refreshDashboardFromScrubber();
    }, 300);
  });
}

// Refresh without re-rendering the scrubber (to avoid loop).
async function refreshDashboardFromScrubber() {
  showLoading();
  try {
    const mode = State.mode === 'compare' ? 'all' : State.mode;
    const allGames = await API.getAllGames(mode);
    State.games = applyGameFilters(allGames);
    if (State.level === 1) await renderLevel1();
  } catch (err) {
    console.error('Dashboard refresh error:', err);
  } finally {
    hideLoading();
  }
}

// Apply lastN, lastDays, and date range filters to game metas.
// Games are assumed to be sorted newest-first from the API.
function applyGameFilters(metas) {
  let filtered = metas;

  // Last N days filter (sets dateFrom)
  if (State.lastDays > 0 && !State.dateFrom) {
    State.dateFrom = new Date(Date.now() - State.lastDays * 86400000);
  }

  // Date range filter
  if (State.dateFrom || State.dateTo) {
    filtered = filtered.filter((g) => {
      const ts = new Date(g.start_time_unix * 1000);
      if (State.dateFrom && ts < State.dateFrom) return false;
      if (State.dateTo && ts > State.dateTo) return false;
      return true;
    });
  }

  // Last N games filter (applied after date filter)
  if (State.lastN > 0 && filtered.length > State.lastN) {
    filtered = filtered.slice(0, State.lastN);
  }

  return filtered;
}

// ============================================================================
// 6. Mode Toggle
// ============================================================================

function initModeToggle() {
  document.querySelectorAll('.mode-btn').forEach((btn) => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.mode-btn').forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      State.mode = btn.dataset.mode;

      const pf = document.getElementById('partner-filter');
      if (State.mode === 'duos' || State.mode === 'compare') {
        pf.classList.remove('hidden');
      } else {
        pf.classList.add('hidden');
      }

      // Always return to overview when changing mode
      State.selectedGameID = null;
      State.selectedTurn = null;
      showLevel(1);
      refreshDashboard();
    });
  });
}

// ============================================================================
// 6. Partner Filter
// ============================================================================

function buildPartnerFilter(fullGames) {
  const pf = document.getElementById('partner-filter');
  // Keep the label, remove everything else
  const label = pf.querySelector('label');
  pf.innerHTML = '';
  pf.appendChild(label);

  State.partners.clear();
  for (const g of fullGames) {
    if (g.partner) {
      const partnerLabel = g.partner.name || heroName(g.partner.hero_card_id) || null;
      if (partnerLabel) State.partners.add(partnerLabel);
    }
  }

  if (State.partners.size === 0) return;

  // Quick actions
  const allBtn = document.createElement('span');
  allBtn.className = 'partner-tag selected';
  allBtn.textContent = 'All';
  allBtn.addEventListener('click', () => {
    State.excludedPartners.clear();
    refreshPartnerTags();
    reRenderFiltered(fullGames);
  });
  pf.appendChild(allBtn);

  const invertBtn = document.createElement('span');
  invertBtn.className = 'partner-tag';
  invertBtn.textContent = 'Invert';
  invertBtn.addEventListener('click', () => {
    const newExcluded = new Set();
    for (const name of State.partners) {
      if (!State.excludedPartners.has(name)) newExcluded.add(name);
    }
    State.excludedPartners = newExcluded;
    refreshPartnerTags();
    reRenderFiltered(fullGames);
  });
  pf.appendChild(invertBtn);

  for (const name of [...State.partners].sort()) {
    const tag = document.createElement('span');
    tag.className = 'partner-tag selected';
    tag.textContent = name;
    tag.dataset.partner = name;

    tag.addEventListener('click', (e) => {
      if (e.shiftKey) {
        // Toggle this partner
        if (State.excludedPartners.has(name)) {
          State.excludedPartners.delete(name);
        } else {
          State.excludedPartners.add(name);
        }
      } else {
        // Solo-select: exclude all except this
        State.excludedPartners = new Set(State.partners);
        State.excludedPartners.delete(name);
      }
      refreshPartnerTags();
      reRenderFiltered(fullGames);
    });

    tag.addEventListener('contextmenu', (e) => {
      e.preventDefault();
      State.excludedPartners.add(name);
      refreshPartnerTags();
      reRenderFiltered(fullGames);
    });

    pf.appendChild(tag);
  }
}

function refreshPartnerTags() {
  document.querySelectorAll('#partner-filter .partner-tag[data-partner]').forEach((tag) => {
    const name = tag.dataset.partner;
    tag.classList.remove('selected', 'excluded');
    if (State.excludedPartners.has(name)) {
      tag.classList.add('excluded');
    } else {
      tag.classList.add('selected');
    }
  });
}

function reRenderFiltered(fullGames) {
  const filtered = filterGamesByPartner(fullGames);
  renderRichCharts(filtered);
}

function getPartnerLabel(g) {
  if (!g.partner) return null;
  return g.partner.name || null;
}

function filterGamesByPartner(games) {
  if (State.excludedPartners.size === 0) return games;
  return games.filter((g) => {
    const label = getPartnerLabel(g);
    if (!label) return true;
    return !State.excludedPartners.has(label);
  });
}

// ============================================================================
// 7. Summary Cards
// ============================================================================

// Compute aggregate stats from filtered game metas (client-side).
function computeAgg(metas) {
  if (!metas || metas.length === 0) return { games_played: 0, wins: 0, losses: 0, avg_placement: 0, best_placement: 0, worst_placement: 0 };
  let wins = 0, losses = 0, total = 0, best = 8, worst = 1;
  for (const g of metas) {
    const p = g.placement || 0;
    total += p;
    const threshold = g.is_duos ? 2 : 4;
    if (p <= threshold) wins++; else losses++;
    if (p < best) best = p;
    if (p > worst) worst = p;
  }
  return {
    games_played: metas.length, wins, losses,
    avg_placement: total / metas.length,
    best_placement: best, worst_placement: worst,
  };
}

function renderSummaryCards(agg, compareAgg) {
  const el = document.getElementById('summary-cards');
  if (!agg) {
    el.innerHTML = '<div class="summary-card"><div class="value">No data</div></div>';
    return;
  }

  const winRate = agg.games_played > 0 ? ((agg.wins / agg.games_played) * 100).toFixed(1) : '0.0';

  const cards = [
    { label: 'Games Played', value: agg.games_played, sub: '' },
    { label: 'Win Rate', value: `${winRate}%`, sub: `${agg.wins}W / ${agg.losses}L` },
    { label: 'Avg Placement', value: agg.avg_placement ? agg.avg_placement.toFixed(2) : '-', sub: '' },
    { label: 'Best', value: agg.best_placement || '-', sub: '' },
    { label: 'Worst', value: agg.worst_placement || '-', sub: '' },
  ];

  if (compareAgg) {
    const cWR = compareAgg.games_played > 0 ? ((compareAgg.wins / compareAgg.games_played) * 100).toFixed(1) : '0.0';
    cards[0].sub = `Duos: ${compareAgg.games_played}`;
    cards[1].sub += ` | Duos: ${cWR}%`;
    const avgDelta = agg.avg_placement && compareAgg.avg_placement
      ? (compareAgg.avg_placement - agg.avg_placement).toFixed(2)
      : '';
    if (avgDelta) cards[2].sub = `Duos diff: ${avgDelta > 0 ? '+' : ''}${avgDelta}`;
  }

  el.innerHTML = cards
    .map(
      (c) => `
    <div class="summary-card">
      <div class="label">${c.label}</div>
      <div class="value">${c.value}</div>
      ${c.sub ? `<div class="sub">${c.sub}</div>` : ''}
    </div>`
    )
    .join('');
}

// ============================================================================
// 8. Level 1 Charts — Meta-based (fast)
// ============================================================================

const ACCENT = '#e94560';
const WIN_COLOR = '#00c853';
const LOSS_COLOR = '#ff5252';
const BASE_ANIM = { animationDuration: 800, animationEasing: 'cubicOut' };

function isWin(placement, isDuos) {
  return isDuos ? placement <= 2 : placement <= 4;
}

function showNoData(containerId) {
  const chart = getChart(containerId);
  if (!chart) return;
  chart.clear();
  chart.setOption({
    title: { text: 'No data', left: 'center', top: 'center', textStyle: { color: '#888', fontSize: 14 } },
  });
}

function renderPlacementTrend(metas) {
  if (!metas || metas.length === 0) return showNoData('chart-placement-trend');
  const chart = getChart('chart-placement-trend');

  const sorted = [...metas].sort((a, b) => a.start_time_unix - b.start_time_unix);

  if (State.mode === 'compare') {
    const soloGames = sorted.filter((g) => !g.is_duos);
    const duosGames = sorted.filter((g) => g.is_duos);

    const soloData = soloGames.map((g) => [g.start_time_unix * 1000, g.placement]);
    const duosData = duosGames.map((g) => [g.start_time_unix * 1000, g.placement]);

    chart.setOption({
      ...BASE_ANIM,
      tooltip: { trigger: 'item' },
      legend: { data: ['Solo', 'Duos'], textStyle: { color: '#ccc' } },
      xAxis: { type: 'time' },
      yAxis: { type: 'value', inverse: true, min: 1, max: 8 },
      series: [
        { name: 'Solo', type: 'line', data: soloData, smooth: true, symbol: 'circle', symbolSize: 6, lineStyle: { color: ACCENT }, itemStyle: { color: ACCENT } },
        { name: 'Duos', type: 'line', data: duosData, smooth: true, symbol: 'circle', symbolSize: 6, lineStyle: { color: '#4fc3f7' }, itemStyle: { color: '#4fc3f7' } },
      ],
    }, true);
  } else {
    const data = sorted.map((g) => [g.start_time_unix * 1000, g.placement]);
    const points = sorted.map((g, i) => [i, g.placement]);
    const reg = linearRegression(points);

    const trendData = sorted.map((g, i) => [g.start_time_unix * 1000, parseFloat((reg.slope * i + reg.intercept).toFixed(2))]);

    chart.setOption({
      ...BASE_ANIM,
      tooltip: { trigger: 'item' },
      xAxis: { type: 'time' },
      yAxis: { type: 'value', inverse: true, min: 1, max: 8 },
      series: [
        {
          name: 'Placement', type: 'line', data, smooth: true, symbol: 'circle', symbolSize: 6,
          lineStyle: { color: ACCENT }, itemStyle: { color: ACCENT },
        },
        {
          name: 'Trend', type: 'line', data: trendData, smooth: false, symbol: 'none',
          lineStyle: { color: '#888', type: 'dashed', width: 1 },
        },
      ],
    }, true);
  }

  chart.off('click');
  chart.on('click', (params) => {
    if (params.seriesName === 'Trend') return;
    const sorted2 = [...metas].sort((a, b) => a.start_time_unix - b.start_time_unix);
    const mode = State.mode === 'compare' ? (params.seriesName === 'Duos' ? 'duos' : 'solo') : null;
    const pool = mode === 'duos' ? sorted2.filter((g) => g.is_duos) : mode === 'solo' ? sorted2.filter((g) => !g.is_duos) : sorted2;
    if (params.dataIndex >= 0 && params.dataIndex < pool.length) {
      drillToGame(pool[params.dataIndex].game_id);
    }
  });
}

function renderPlacementDist(metas) {
  if (!metas || metas.length === 0) return showNoData('chart-placement-dist');
  const chart = getChart('chart-placement-dist');

  if (State.mode === 'compare') {
    const soloCounts = new Array(8).fill(0);
    const duosCounts = new Array(8).fill(0);
    metas.forEach((g) => {
      if (g.placement >= 1 && g.placement <= 8) {
        (g.is_duos ? duosCounts : soloCounts)[g.placement - 1]++;
      }
    });

    chart.setOption({
      ...BASE_ANIM,
      tooltip: { trigger: 'axis' },
      legend: { data: ['Solo', 'Duos'], textStyle: { color: '#ccc' } },
      xAxis: { type: 'category', data: ['1st', '2nd', '3rd', '4th', '5th', '6th', '7th', '8th'] },
      yAxis: { type: 'value' },
      series: [
        { name: 'Solo', type: 'bar', data: soloCounts, itemStyle: { color: ACCENT } },
        { name: 'Duos', type: 'bar', data: duosCounts, itemStyle: { color: '#4fc3f7' } },
      ],
    }, true);
  } else {
    const counts = new Array(8).fill(0);
    metas.forEach((g) => {
      if (g.placement >= 1 && g.placement <= 8) counts[g.placement - 1]++;
    });

    const colors = counts.map((_, i) => {
      const p = i + 1;
      // Use first meta to check duos threshold, or default to solo
      const anyDuos = metas.some((g) => g.is_duos);
      return (anyDuos && State.mode === 'duos') ? (p <= 2 ? WIN_COLOR : LOSS_COLOR) : (p <= 4 ? WIN_COLOR : LOSS_COLOR);
    });

    chart.setOption({
      ...BASE_ANIM,
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: ['1st', '2nd', '3rd', '4th', '5th', '6th', '7th', '8th'] },
      yAxis: { type: 'value' },
      series: [{
        type: 'bar',
        data: counts.map((v, i) => ({ value: v, itemStyle: { color: colors[i] } })),
      }],
    }, true);
  }
}

function renderWinRateTrend(metas) {
  if (!metas || metas.length < 2) return showNoData('chart-winrate-trend');
  const chart = getChart('chart-winrate-trend');

  const sorted = [...metas].sort((a, b) => a.start_time_unix - b.start_time_unix);
  const windowSize = Math.min(20, sorted.length);

  const data = [];
  for (let i = 0; i < sorted.length; i++) {
    const start = Math.max(0, i - windowSize + 1);
    const window = sorted.slice(start, i + 1);
    const wins = window.filter((g) => isWin(g.placement, g.is_duos)).length;
    data.push([i + 1, parseFloat(((wins / window.length) * 100).toFixed(1))]);
  }

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'axis', formatter: (p) => `Game #${p[0].data[0]}<br/>Win Rate: ${p[0].data[1]}%` },
    xAxis: { type: 'value', name: 'Game #' },
    yAxis: { type: 'value', name: '%', min: 0, max: 100 },
    series: [{
      type: 'line', data, smooth: true, symbol: 'none',
      areaStyle: { opacity: 0.15, color: ACCENT },
      lineStyle: { color: ACCENT },
    }],
  }, true);
}

// ============================================================================
// 9. Level 1 Charts — Full game data (rich)
// ============================================================================

function heroName(heroCardId) {
  if (!heroCardId) return 'Unknown';
  const names = State.cardNames;

  // Direct lookup
  if (names[heroCardId]) return names[heroCardId];

  // Strip _SKIN_* suffix and look up base card
  const skinMatch = heroCardId.match(/^(.+?)(_SKIN_\w+)$/);
  if (skinMatch) {
    const baseName = names[skinMatch[1]];
    if (baseName) return `${baseName} (${skinMatch[2].replace(/_SKIN_/, 'Skin ')})`;
  }

  // Strip _G (golden) suffix
  const goldenMatch = heroCardId.match(/^(.+?)_G$/);
  if (goldenMatch && names[goldenMatch[1]]) {
    return names[goldenMatch[1]];
  }

  return heroCardId;
}

function renderHeroPerf(games) {
  if (!games || games.length === 0) return showNoData('chart-hero-perf');
  const chart = getChart('chart-hero-perf');

  const heroMap = new Map();
  for (const g of games) {
    const hid = g.player?.hero_card_id || 'Unknown';
    if (!heroMap.has(hid)) heroMap.set(hid, { total: 0, count: 0 });
    const entry = heroMap.get(hid);
    entry.total += g.placement || 0;
    entry.count++;
  }

  const entries = [...heroMap.entries()]
    .map(([id, v]) => ({ id, name: heroName(id), avg: v.total / v.count, count: v.count }))
    .sort((a, b) => a.avg - b.avg);

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    grid: { left: 140, right: 60 },
    yAxis: { type: 'category', data: entries.map((e) => e.name), inverse: true },
    xAxis: { type: 'value', name: 'Avg Placement', min: 1 },
    series: [{
      type: 'bar',
      data: entries.map((e) => ({ value: parseFloat(e.avg.toFixed(2)), count: e.count })),
      itemStyle: { color: ACCENT },
      label: { show: true, position: 'right', formatter: (p) => `${p.data.value} (${entries[p.dataIndex].count}g)`, color: '#ccc', fontSize: 11 },
    }],
  }, true);
}

function renderTavernTier(games) {
  if (!games || games.length === 0) return showNoData('chart-tavern-tier');
  const chart = getChart('chart-tavern-tier');

  const tiers = new Array(8).fill(0); // index 0 unused, 1-7
  for (const g of games) {
    const t = g.tavern_tier || g.player?.tavern_tier || 0;
    if (t >= 1 && t <= 7) tiers[t]++;
  }

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: ['T1', 'T2', 'T3', 'T4', 'T5', 'T6', 'T7'] },
    yAxis: { type: 'value' },
    series: [{
      type: 'bar',
      data: tiers.slice(1, 8),
      itemStyle: { color: '#7c4dff' },
    }],
  }, true);
}

function renderBuffBreakdown(games) {
  if (!games || games.length === 0) return showNoData('chart-buff-breakdown');
  const chart = getChart('chart-buff-breakdown');

  const catMap = new Map();
  for (const g of games) {
    if (!g.buff_sources) continue;
    for (const bs of g.buff_sources) {
      if (!catMap.has(bs.category)) catMap.set(bs.category, { atk: 0, hp: 0 });
      const entry = catMap.get(bs.category);
      entry.atk += bs.attack || 0;
      entry.hp += bs.health || 0;
    }
  }

  if (catMap.size === 0) return showNoData('chart-buff-breakdown');

  const categories = [...catMap.keys()].sort();

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'axis' },
    legend: { data: ['Attack', 'Health'], textStyle: { color: '#ccc' } },
    xAxis: { type: 'category', data: categories, axisLabel: { rotate: 30, fontSize: 10 } },
    yAxis: { type: 'value' },
    series: [
      { name: 'Attack', type: 'bar', stack: 'total', data: categories.map((c) => catMap.get(c).atk), itemStyle: { color: '#ffc107' } },
      { name: 'Health', type: 'bar', stack: 'total', data: categories.map((c) => catMap.get(c).hp), itemStyle: { color: WIN_COLOR } },
    ],
  }, true);
}

function renderDuration(metas) {
  if (!metas || metas.length === 0) return showNoData('chart-duration');
  const chart = getChart('chart-duration');

  const data = metas
    .filter((g) => g.end_time_unix && g.start_time_unix && g.end_time_unix > g.start_time_unix)
    .map((g) => {
      const dur = (g.end_time_unix - g.start_time_unix) / 60;
      return { value: [parseFloat(dur.toFixed(1)), g.placement], gameId: g.game_id };
    });

  if (data.length === 0) return showNoData('chart-duration');

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'item', formatter: (p) => `Duration: ${p.data.value[0]} min<br/>Placement: ${p.data.value[1]}` },
    xAxis: { type: 'value', name: 'Minutes' },
    yAxis: { type: 'value', name: 'Placement', inverse: true, min: 1, max: 8 },
    series: [{
      type: 'scatter', data,
      symbolSize: 8,
      itemStyle: { color: ACCENT, opacity: 0.7 },
    }],
  }, true);

  chart.off('click');
  chart.on('click', (params) => {
    if (params.data && params.data.gameId) drillToGame(params.data.gameId);
  });
}

function renderAnomalyPerf(games) {
  if (!games || games.length === 0) return showNoData('chart-anomaly-perf');
  const chart = getChart('chart-anomaly-perf');

  const anomMap = new Map();
  for (const g of games) {
    const name = g.anomaly_name || g.anomaly_card_id;
    if (!name) continue;
    if (!anomMap.has(name)) anomMap.set(name, { total: 0, count: 0 });
    const entry = anomMap.get(name);
    entry.total += g.placement || 0;
    entry.count++;
  }

  const entries = [...anomMap.entries()]
    .filter(([, v]) => v.count >= 2)
    .map(([name, v]) => ({ name, avg: v.total / v.count, count: v.count }))
    .sort((a, b) => a.avg - b.avg);

  if (entries.length === 0) return showNoData('chart-anomaly-perf');

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    grid: { left: 140, right: 60 },
    yAxis: { type: 'category', data: entries.map((e) => e.name), inverse: true },
    xAxis: { type: 'value', name: 'Avg Placement', min: 1 },
    series: [{
      type: 'bar',
      data: entries.map((e) => parseFloat(e.avg.toFixed(2))),
      itemStyle: { color: '#ff9800' },
      label: { show: true, position: 'right', formatter: (p) => `${p.value} (${entries[p.dataIndex].count}g)`, color: '#ccc', fontSize: 11 },
    }],
  }, true);
}

function renderTribeWinrate(games) {
  if (!games || games.length === 0) return showNoData('chart-tribe-winrate');
  const chart = getChart('chart-tribe-winrate');

  const tribeMap = new Map();
  for (const g of games) {
    const tribe = getDominantTribe(g.board);
    if (!tribeMap.has(tribe)) tribeMap.set(tribe, { total: 0, count: 0 });
    const entry = tribeMap.get(tribe);
    entry.total += g.placement || 0;
    entry.count++;
  }

  const entries = [...tribeMap.entries()]
    .map(([name, v]) => ({ name, avg: v.total / v.count, count: v.count }))
    .sort((a, b) => a.avg - b.avg);

  if (entries.length === 0) return showNoData('chart-tribe-winrate');

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    grid: { left: 120, right: 60 },
    yAxis: { type: 'category', data: entries.map((e) => e.name), inverse: true },
    xAxis: { type: 'value', name: 'Avg Placement', min: 1 },
    series: [{
      type: 'bar',
      data: entries.map((e) => parseFloat(e.avg.toFixed(2))),
      itemStyle: { color: '#26c6da' },
      label: { show: true, position: 'right', formatter: (p) => `${p.value} (${entries[p.dataIndex].count}g)`, color: '#ccc', fontSize: 11 },
    }],
  }, true);
}

function renderBuffEfficiency(games) {
  if (!games || games.length === 0) return showNoData('chart-buff-efficiency');
  const chart = getChart('chart-buff-efficiency');

  const data = games
    .filter((g) => g.buff_sources && g.buff_sources.length > 0)
    .map((g) => {
      const totalBuff = g.buff_sources.reduce((s, bs) => s + (bs.attack || 0) + (bs.health || 0), 0);
      return { value: [totalBuff, g.placement], gameId: g.game_id };
    });

  if (data.length === 0) return showNoData('chart-buff-efficiency');

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'item', formatter: (p) => `Total Buffs: ${p.data.value[0]}<br/>Placement: ${p.data.value[1]}` },
    xAxis: { type: 'value', name: 'Total Buff (ATK+HP)' },
    yAxis: { type: 'value', name: 'Placement', inverse: true, min: 1, max: 8 },
    series: [{
      type: 'scatter', data,
      symbolSize: 8,
      itemStyle: { color: '#ab47bc', opacity: 0.7 },
    }],
  }, true);

  chart.off('click');
  chart.on('click', (params) => {
    if (params.data && params.data.gameId) drillToGame(params.data.gameId);
  });
}

// ============================================================================
// 10. Level 1 Heatmaps
// ============================================================================

function renderHeatmapHero(games) {
  if (!games || games.length === 0) return showNoData('chart-heatmap-hero');
  const chart = getChart('chart-heatmap-hero');

  const heroSet = new Set();
  const countMap = new Map();
  for (const g of games) {
    const hid = heroName(g.player?.hero_card_id || 'Unknown');
    heroSet.add(hid);
    const key = `${g.placement}|${hid}`;
    countMap.set(key, (countMap.get(key) || 0) + 1);
  }

  const heroes = [...heroSet].sort();
  const placements = [1, 2, 3, 4, 5, 6, 7, 8];
  const data = [];
  let maxVal = 0;
  for (let xi = 0; xi < placements.length; xi++) {
    for (let yi = 0; yi < heroes.length; yi++) {
      const val = countMap.get(`${placements[xi]}|${heroes[yi]}`) || 0;
      if (val > maxVal) maxVal = val;
      data.push([xi, yi, val]);
    }
  }

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { formatter: (p) => `${heroes[p.data[1]]}<br/>Placement: ${placements[p.data[0]]}<br/>Games: ${p.data[2]}` },
    grid: { left: 140, right: 60, bottom: 40 },
    xAxis: { type: 'category', data: placements.map(String), splitArea: { show: true } },
    yAxis: { type: 'category', data: heroes, splitArea: { show: true } },
    visualMap: { min: 0, max: maxVal || 1, calculable: true, orient: 'horizontal', left: 'center', bottom: 0, inRange: { color: ['#1a1a2e', '#304ffe', '#e94560', '#ff5252'] } },
    series: [{
      type: 'heatmap', data,
      label: { show: true, color: '#fff', fontSize: 10 },
      emphasis: { itemStyle: { shadowBlur: 10, shadowColor: 'rgba(0,0,0,0.5)' } },
    }],
  }, true);
}

function renderHeatmapTierTurn(games) {
  if (!games || games.length === 0) return showNoData('chart-heatmap-tier-turn');
  const chart = getChart('chart-heatmap-tier-turn');

  const countMap = new Map();
  let maxTurn = 0;
  for (const g of games) {
    const turn = g.turn || 0;
    const tier = g.tavern_tier || g.player?.tavern_tier || 0;
    if (turn > maxTurn) maxTurn = turn;
    const key = `${turn}|${tier}`;
    countMap.set(key, (countMap.get(key) || 0) + 1);
  }

  const turns = [];
  for (let i = 1; i <= Math.min(maxTurn, 20); i++) turns.push(i);
  const tiers = [1, 2, 3, 4, 5, 6, 7];
  const data = [];
  let maxVal = 0;
  for (let xi = 0; xi < turns.length; xi++) {
    for (let yi = 0; yi < tiers.length; yi++) {
      const val = countMap.get(`${turns[xi]}|${tiers[yi]}`) || 0;
      if (val > maxVal) maxVal = val;
      data.push([xi, yi, val]);
    }
  }

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { formatter: (p) => `Turn ${turns[p.data[0]]}<br/>Tier ${tiers[p.data[1]]}<br/>Games: ${p.data[2]}` },
    grid: { left: 60, right: 60, bottom: 40 },
    xAxis: { type: 'category', data: turns.map(String), name: 'Turn' },
    yAxis: { type: 'category', data: tiers.map((t) => `T${t}`) },
    visualMap: { min: 0, max: maxVal || 1, calculable: true, orient: 'horizontal', left: 'center', bottom: 0, inRange: { color: ['#1a1a2e', '#1565c0', '#4fc3f7'] } },
    series: [{
      type: 'heatmap', data,
      label: { show: data.length <= 120, color: '#fff', fontSize: 10 },
    }],
  }, true);
}

function renderHeatmapTribe(games) {
  if (!games || games.length === 0) return showNoData('chart-heatmap-tribe');
  const chart = getChart('chart-heatmap-tribe');

  const tribeSet = new Set();
  const countMap = new Map();
  for (const g of games) {
    const tribe = getDominantTribe(g.board);
    tribeSet.add(tribe);
    const key = `${g.placement}|${tribe}`;
    countMap.set(key, (countMap.get(key) || 0) + 1);
  }

  const tribes = [...tribeSet].sort();
  const placements = [1, 2, 3, 4, 5, 6, 7, 8];
  const data = [];
  let maxVal = 0;
  for (let xi = 0; xi < placements.length; xi++) {
    for (let yi = 0; yi < tribes.length; yi++) {
      const val = countMap.get(`${placements[xi]}|${tribes[yi]}`) || 0;
      if (val > maxVal) maxVal = val;
      data.push([xi, yi, val]);
    }
  }

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { formatter: (p) => `${tribes[p.data[1]]}<br/>Placement: ${placements[p.data[0]]}<br/>Games: ${p.data[2]}` },
    grid: { left: 100, right: 60, bottom: 40 },
    xAxis: { type: 'category', data: placements.map(String) },
    yAxis: { type: 'category', data: tribes },
    visualMap: { min: 0, max: maxVal || 1, calculable: true, orient: 'horizontal', left: 'center', bottom: 0, inRange: { color: ['#1a1a2e', '#2e7d32', '#00c853'] } },
    series: [{
      type: 'heatmap', data,
      label: { show: true, color: '#fff', fontSize: 10 },
    }],
  }, true);
}

function renderHeatmapBuff(games) {
  if (!games || games.length === 0) return showNoData('chart-heatmap-buff');
  const chart = getChart('chart-heatmap-buff');

  const buckets = ['0-10', '10-25', '25-50', '50-100', '100+'];
  function getBucket(total) {
    if (total < 10) return 0;
    if (total < 25) return 1;
    if (total < 50) return 2;
    if (total < 100) return 3;
    return 4;
  }

  const countMap = new Map();
  for (const g of games) {
    const total = (g.buff_sources || []).reduce((s, bs) => s + (bs.attack || 0) + (bs.health || 0), 0);
    const bi = getBucket(total);
    const key = `${g.placement}|${bi}`;
    countMap.set(key, (countMap.get(key) || 0) + 1);
  }

  const placements = [1, 2, 3, 4, 5, 6, 7, 8];
  const data = [];
  let maxVal = 0;
  for (let xi = 0; xi < placements.length; xi++) {
    for (let yi = 0; yi < buckets.length; yi++) {
      const val = countMap.get(`${placements[xi]}|${yi}`) || 0;
      if (val > maxVal) maxVal = val;
      data.push([xi, yi, val]);
    }
  }

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { formatter: (p) => `Buff: ${buckets[p.data[1]]}<br/>Placement: ${placements[p.data[0]]}<br/>Games: ${p.data[2]}` },
    grid: { left: 80, right: 60, bottom: 40 },
    xAxis: { type: 'category', data: placements.map(String) },
    yAxis: { type: 'category', data: buckets },
    visualMap: { min: 0, max: maxVal || 1, calculable: true, orient: 'horizontal', left: 'center', bottom: 0, inRange: { color: ['#1a1a2e', '#6a1b9a', '#e94560'] } },
    series: [{
      type: 'heatmap', data,
      label: { show: true, color: '#fff', fontSize: 10 },
    }],
  }, true);
}

// ============================================================================
// 11. Level 2 — Single Game
// ============================================================================

function renderGameHeader(game) {
  const el = document.getElementById('game-header');
  if (!game) { el.innerHTML = '<h2>Game not found</h2>'; return; }

  const p = game.placement || 0;
  const pClass = isWin(p, game.is_duos) ? 'win' : 'loss';
  const start = game.start_time_unix ? new Date(game.start_time_unix * 1000).toLocaleString() : '';
  const dur = (game.end_time_unix && game.start_time_unix)
    ? `${((game.end_time_unix - game.start_time_unix) / 60).toFixed(0)} min`
    : '';
  const heroLabel = heroName(game.player?.hero_card_id);
  const modeBadge = game.is_duos ? '<span style="color:#4fc3f7;margin-left:0.5rem;">[Duos]</span>' : '<span style="color:#aaa;margin-left:0.5rem;">[Solo]</span>';
  const anomaly = game.anomaly_name ? `<span style="color:#ff9800;margin-left:0.5rem;">${game.anomaly_name}</span>` : '';
  const partnerLabel = getPartnerLabel(game);
  const partner = partnerLabel ? `<span style="color:#4fc3f7;margin-left:0.5rem;">w/ ${partnerLabel}</span>` : '';

  el.innerHTML = `
    <button onclick="navigateTo(1)" style="background:var(--accent);color:#fff;border:none;border-radius:4px;padding:0.4rem 0.8rem;cursor:pointer;font-size:0.85rem;">&#8592; Back</button>
    <h2>${heroLabel}</h2>
    <span class="placement ${pClass}">#${p}</span>
    <span style="color:var(--text-muted);font-size:0.85rem;">${start} &middot; ${dur}</span>
    ${modeBadge}${anomaly}${partner}
  `;
}

function renderBoardStats(turns) {
  if (!turns || turns.length === 0) return showNoData('chart-board-stats');
  const chart = getChart('chart-board-stats');

  const turnNums = turns.map((t) => t.turn);
  const totalAtk = turns.map((t) => (t.state.board || []).reduce((s, m) => s + (m.attack || 0), 0));
  const totalHp = turns.map((t) => (t.state.board || []).reduce((s, m) => s + (m.health || 0), 0));

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'axis' },
    legend: { data: ['Total ATK', 'Total HP'], textStyle: { color: '#ccc' } },
    xAxis: { type: 'category', data: turnNums, name: 'Turn' },
    yAxis: { type: 'value' },
    series: [
      { name: 'Total ATK', type: 'line', data: totalAtk, smooth: true, lineStyle: { color: '#ffc107' }, itemStyle: { color: '#ffc107' } },
      { name: 'Total HP', type: 'line', data: totalHp, smooth: true, lineStyle: { color: WIN_COLOR }, itemStyle: { color: WIN_COLOR } },
    ],
  }, true);

  setupTurnDrill(chart, turns);
}

function renderHealthArmor(turns) {
  if (!turns || turns.length === 0) return showNoData('chart-health-armor');
  const chart = getChart('chart-health-armor');

  const turnNums = turns.map((t) => t.turn);
  const effHP = turns.map((t) => {
    const p = t.state.player || {};
    return (p.health || 0) - (p.damage || 0) + (p.armor || 0);
  });

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: turnNums, name: 'Turn' },
    yAxis: { type: 'value', name: 'Effective HP' },
    series: [{
      name: 'Effective HP', type: 'line', data: effHP, smooth: true,
      areaStyle: { opacity: 0.15, color: WIN_COLOR },
      lineStyle: { color: WIN_COLOR }, itemStyle: { color: WIN_COLOR },
    }],
  }, true);

  setupTurnDrill(chart, turns);
}

function renderTierProg(turns) {
  if (!turns || turns.length === 0) return showNoData('chart-tier-prog');
  const chart = getChart('chart-tier-prog');

  const turnNums = turns.map((t) => t.turn);
  const tiers = turns.map((t) => t.state.tavern_tier || t.state.player?.tavern_tier || 0);

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: turnNums, name: 'Turn' },
    yAxis: { type: 'value', min: 1, max: 7, name: 'Tier' },
    series: [{
      name: 'Tavern Tier', type: 'line', data: tiers, step: 'end',
      lineStyle: { color: '#7c4dff' }, itemStyle: { color: '#7c4dff' },
      areaStyle: { opacity: 0.1, color: '#7c4dff' },
    }],
  }, true);

  setupTurnDrill(chart, turns);
}

function renderBuffAccum(turns) {
  if (!turns || turns.length === 0) return showNoData('chart-buff-accum');
  const chart = getChart('chart-buff-accum');

  // Collect all categories across turns
  const catSet = new Set();
  for (const t of turns) {
    if (t.state.buff_sources) {
      for (const bs of t.state.buff_sources) catSet.add(bs.category);
    }
  }

  if (catSet.size === 0) return showNoData('chart-buff-accum');

  const categories = [...catSet].sort();
  const turnNums = turns.map((t) => t.turn);
  const palette = ['#e94560', '#4fc3f7', '#ffc107', '#00c853', '#7c4dff', '#ff9800', '#26c6da', '#ab47bc', '#8d6e63', '#78909c', '#d4e157', '#ef5350', '#42a5f5'];

  const series = categories.map((cat, ci) => {
    const data = turns.map((t) => {
      const bs = (t.state.buff_sources || []).find((b) => b.category === cat);
      return bs ? (bs.attack || 0) + (bs.health || 0) : 0;
    });
    return {
      name: cat, type: 'line', stack: 'buffs', data,
      areaStyle: { opacity: 0.6 },
      lineStyle: { width: 1, color: palette[ci % palette.length] },
      itemStyle: { color: palette[ci % palette.length] },
      symbol: 'none',
    };
  });

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'axis' },
    legend: { data: categories, textStyle: { color: '#ccc', fontSize: 10 }, type: 'scroll', bottom: 0 },
    grid: { bottom: 40 },
    xAxis: { type: 'category', data: turnNums, name: 'Turn' },
    yAxis: { type: 'value', name: 'Buff Total' },
    series,
  }, true);

  setupTurnDrill(chart, turns);
}

function renderGoldEcon(turns) {
  if (!turns || turns.length === 0) return showNoData('chart-gold-econ');
  const chart = getChart('chart-gold-econ');

  const turnNums = turns.map((t) => t.turn);
  const maxGold = turns.map((t) => t.state.player?.max_gold || 0);
  const curGold = turns.map((t) => t.state.player?.current_gold || 0);

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'axis' },
    legend: { data: ['Max Gold', 'Current Gold'], textStyle: { color: '#ccc' } },
    xAxis: { type: 'category', data: turnNums, name: 'Turn' },
    yAxis: { type: 'value', name: 'Gold' },
    series: [
      { name: 'Max Gold', type: 'line', data: maxGold, lineStyle: { color: '#ffc107' }, itemStyle: { color: '#ffc107' } },
      { name: 'Current Gold', type: 'line', data: curGold, lineStyle: { color: '#ff9800', type: 'dashed' }, itemStyle: { color: '#ff9800' } },
    ],
  }, true);

  setupTurnDrill(chart, turns);
}

function renderBoardSize(turns) {
  if (!turns || turns.length === 0) return showNoData('chart-board-size');
  const chart = getChart('chart-board-size');

  const turnNums = turns.map((t) => t.turn);
  const sizes = turns.map((t) => (t.state.board || []).length);

  chart.setOption({
    ...BASE_ANIM,
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: turnNums, name: 'Turn' },
    yAxis: { type: 'value', name: 'Minions', min: 0, max: 7 },
    series: [{
      name: 'Board Size', type: 'line', data: sizes, step: 'end',
      areaStyle: { opacity: 0.15, color: ACCENT },
      lineStyle: { color: ACCENT }, itemStyle: { color: ACCENT },
    }],
  }, true);

  setupTurnDrill(chart, turns);
}

function setupTurnDrill(chart, turns) {
  chart.off('click');
  chart.on('click', (params) => {
    const idx = params.dataIndex;
    if (idx >= 0 && idx < turns.length) {
      drillToTurn(turns[idx].turn);
    }
  });
}

// ============================================================================
// 12. Level 3 — Turn Detail
// ============================================================================

function renderTurnDetail(snapshot) {
  const header = document.getElementById('turn-header');
  const board = document.getElementById('turn-board');
  const deltas = document.getElementById('turn-deltas');

  if (!snapshot) {
    header.innerHTML = '<h2>Turn not found</h2>';
    board.innerHTML = '';
    deltas.innerHTML = '';
    return;
  }

  const s = snapshot.state || {};
  const p = s.player || {};
  const tier = s.tavern_tier || p.tavern_tier || '?';
  const gold = p.current_gold != null ? `${p.current_gold}/${p.max_gold || '?'}` : '';

  header.innerHTML = `
    <button onclick="navigateTo(2)" style="background:var(--accent);color:#fff;border:none;border-radius:4px;padding:0.4rem 0.8rem;cursor:pointer;font-size:0.85rem;">&#8592; Back</button>
    <h2>Turn ${snapshot.turn}</h2>
    <span style="color:#7c4dff;">Tier ${tier}</span>
    ${gold ? `<span style="color:#ffc107;">Gold: ${gold}</span>` : ''}
  `;

  // Board minions
  const minions = s.board || [];
  if (minions.length === 0) {
    board.innerHTML = '<div style="color:var(--text-muted);text-align:center;width:100%;">No minions on board</div>';
  } else {
    board.innerHTML = minions
      .map((m) => {
        const typeBadge = m.minion_type && m.minion_type !== 'INVALID'
          ? `<div style="font-size:0.65rem;color:var(--text-muted);margin-top:0.25rem;">${m.minion_type}</div>`
          : '';
        return `
        <div class="minion-card">
          <div class="name" title="${m.name || m.card_id || ''}">${m.name || m.card_id || '???'}</div>
          <div class="stats">
            <span class="atk">${m.attack || 0}</span>
            <span style="color:var(--text-muted);"> / </span>
            <span class="hp">${m.health || 0}</span>
          </div>
          ${typeBadge}
        </div>`;
      })
      .join('');
  }

  // Deltas
  let deltasHTML = '';

  const buffDeltas = snapshot.buff_deltas || [];
  if (buffDeltas.length > 0) {
    deltasHTML += `
      <div style="flex:1;min-width:250px;">
        <h3 style="color:var(--text-muted);font-size:0.85rem;margin-bottom:0.5rem;">Buff Deltas</h3>
        <table style="width:100%;font-size:0.8rem;border-collapse:collapse;">
          <tr style="color:var(--text-muted);"><th style="text-align:left;padding:0.25rem;">Category</th><th>ATK</th><th>HP</th></tr>
          ${buffDeltas.map((d) => `
            <tr>
              <td style="padding:0.25rem;">${d.category}</td>
              <td style="text-align:center;color:#ffc107;">${d.attack_delta > 0 ? '+' : ''}${d.attack_delta || 0}</td>
              <td style="text-align:center;color:${WIN_COLOR};">${d.health_delta > 0 ? '+' : ''}${d.health_delta || 0}</td>
            </tr>
          `).join('')}
        </table>
      </div>`;
  }

  const abilityDeltas = snapshot.ability_deltas || [];
  if (abilityDeltas.length > 0) {
    deltasHTML += `
      <div style="flex:1;min-width:250px;">
        <h3 style="color:var(--text-muted);font-size:0.85rem;margin-bottom:0.5rem;">Ability Deltas</h3>
        <table style="width:100%;font-size:0.8rem;border-collapse:collapse;">
          <tr style="color:var(--text-muted);"><th style="text-align:left;padding:0.25rem;">Category</th><th>Delta</th></tr>
          ${abilityDeltas.map((d) => `
            <tr>
              <td style="padding:0.25rem;">${d.category}</td>
              <td style="text-align:center;color:${ACCENT};">${d.value_delta > 0 ? '+' : ''}${d.value_delta || 0}</td>
            </tr>
          `).join('')}
        </table>
      </div>`;
  }

  if (!deltasHTML) {
    deltasHTML = '<div style="color:var(--text-muted);">No deltas this turn</div>';
  }

  deltas.innerHTML = deltasHTML;
}

// ============================================================================
// 13. Main Orchestration
// ============================================================================

function showLoading() {
  let el = document.getElementById('loading-overlay');
  if (!el) {
    el = document.createElement('div');
    el.id = 'loading-overlay';
    el.className = 'loading';
    el.textContent = 'Loading';
    document.querySelector('main').prepend(el);
  }
  el.classList.remove('hidden');
}

function hideLoading() {
  const el = document.getElementById('loading-overlay');
  if (el) el.classList.add('hidden');
}

async function refreshDashboard() {
  showLoading();
  try {
    const mode = State.mode === 'compare' ? 'all' : State.mode;
    const allGames = await API.getAllGames(mode);

    // Render scrubber with ALL games (unfiltered) so user can see full timeline
    renderTimelineScrubber(allGames);

    State.games = applyGameFilters(allGames);

    if (State.level === 1) await renderLevel1();
    else if (State.level === 2) await renderLevel2();
    else if (State.level === 3) await renderLevel3();
  } catch (err) {
    console.error('Dashboard refresh error:', err);
  } finally {
    hideLoading();
  }
}

async function renderLevel1() {
  // Summary cards — computed client-side from filtered metas
  if (State.mode === 'compare') {
    renderSummaryCards(
      computeAgg(State.games.filter((g) => !g.is_duos)),
      computeAgg(State.games.filter((g) => g.is_duos))
    );
  } else {
    renderSummaryCards(computeAgg(State.games));
  }

  // Fast charts from metas
  renderPlacementTrend(State.games);
  renderPlacementDist(State.games);
  renderWinRateTrend(State.games);

  // Lazy: fetch full games for rich charts
  await fetchFullGames(State.games);
  const full = State.games.map((g) => State.fullGames.get(g.game_id)).filter(Boolean);

  // Build partner filter if relevant
  if (State.mode === 'duos' || State.mode === 'compare') {
    buildPartnerFilter(full);
  } else {
    const pf = document.getElementById('partner-filter');
    const label = pf.querySelector('label');
    pf.innerHTML = '';
    pf.appendChild(label);
  }

  const filtered = filterGamesByPartner(full);
  renderRichCharts(filtered);

  // Duration uses metas (has timestamps)
  renderDuration(State.games);

  // Resize after rendering
  setTimeout(() => resizeAll(), 100);
}

function renderRichCharts(filtered) {
  renderHeroPerf(filtered);
  renderTavernTier(filtered);
  renderBuffBreakdown(filtered);
  renderAnomalyPerf(filtered);
  renderTribeWinrate(filtered);
  renderBuffEfficiency(filtered);
  renderHeatmapHero(filtered);
  renderHeatmapTierTurn(filtered);
  renderHeatmapTribe(filtered);
  renderHeatmapBuff(filtered);
}

async function renderLevel2() {
  if (!State.selectedGameID) return;

  let game = State.fullGames.get(State.selectedGameID);
  if (!game) {
    try {
      game = await API.getGame(State.selectedGameID);
      State.fullGames.set(State.selectedGameID, game);
    } catch (err) {
      console.error('Failed to load game:', err);
      return;
    }
  }

  renderGameHeader(game);

  let turns = State.turnData.get(State.selectedGameID);
  if (!turns) {
    try {
      turns = await API.getTurns(State.selectedGameID);
      State.turnData.set(State.selectedGameID, turns);
    } catch (err) {
      console.error('Failed to load turns:', err);
      turns = [];
    }
  }

  renderBoardStats(turns);
  renderHealthArmor(turns);
  renderTierProg(turns);
  renderBuffAccum(turns);
  renderGoldEcon(turns);
  renderBoardSize(turns);

  setTimeout(() => resizeAll(), 100);
}

async function renderLevel3() {
  if (!State.selectedGameID || State.selectedTurn == null) return;

  let turns = State.turnData.get(State.selectedGameID);
  if (!turns) {
    try {
      turns = await API.getTurns(State.selectedGameID);
      State.turnData.set(State.selectedGameID, turns);
    } catch (err) {
      console.error('Failed to load turns:', err);
      return;
    }
  }

  const snapshot = turns.find((t) => t.turn === State.selectedTurn);
  renderTurnDetail(snapshot);
}

function drillToGame(gameID) {
  State.selectedGameID = gameID;
  State.selectedTurn = null;
  showLevel(2);
  renderLevel2();
}

function drillToTurn(turn) {
  State.selectedTurn = turn;
  showLevel(3);
  renderLevel3();
}

// ============================================================================
// 14. Helper: fetchFullGames
// ============================================================================

async function fetchFullGames(metas) {
  const toFetch = metas.filter((m) => !State.fullGames.has(m.game_id));
  const batchSize = 10;

  for (let i = 0; i < toFetch.length; i += batchSize) {
    const batch = toFetch.slice(i, i + batchSize);
    const results = await Promise.allSettled(batch.map((m) => API.getGame(m.game_id)));
    for (let j = 0; j < results.length; j++) {
      if (results[j].status === 'fulfilled' && results[j].value) {
        State.fullGames.set(batch[j].game_id, results[j].value);
      }
    }
  }
}

// ============================================================================
// 15. Helper: getDominantTribe
// ============================================================================

function getTribeComposition(board) {
  if (!board || board.length === 0) return { tribe: 'NONE', pct: 0 };

  const counts = new Map();
  let typed = 0;
  for (const m of board) {
    const t = m.minion_type || 'INVALID';
    if (t === 'INVALID') continue;
    if (t === 'ALL') { typed++; continue; } // ALL-type minions count toward typed but no single tribe
    counts.set(t, (counts.get(t) || 0) + 1);
    typed++;
  }

  if (typed === 0) return { tribe: 'MIXED', pct: 0 };

  let maxCount = 0;
  let maxTribe = 'MIXED';
  for (const [tribe, count] of counts) {
    if (count > maxCount) {
      maxCount = count;
      maxTribe = tribe;
    }
  }

  return { tribe: maxTribe, pct: typed > 0 ? maxCount / typed : 0 };
}

function getTribeLabel(board) {
  const { tribe, pct } = getTribeComposition(board);
  if (tribe === 'NONE' || tribe === 'MIXED') return 'Mixed';
  const p = Math.round(pct * 100);
  if (p >= 90) return `${tribe} (90%+)`;
  if (p >= 70) return `${tribe} (70%+)`;
  if (p >= 50) return `${tribe} (50%+)`;
  return 'Mixed';
}

// Legacy compat
function getDominantTribe(board) {
  return getTribeLabel(board);
}

// ============================================================================
// 16. Helper: linearRegression
// ============================================================================

function linearRegression(points) {
  const n = points.length;
  if (n === 0) return { slope: 0, intercept: 0 };

  let sumX = 0, sumY = 0, sumXY = 0, sumX2 = 0;
  for (const [x, y] of points) {
    sumX += x;
    sumY += y;
    sumXY += x * y;
    sumX2 += x * x;
  }

  const denom = n * sumX2 - sumX * sumX;
  if (denom === 0) return { slope: 0, intercept: sumY / n };

  const slope = (n * sumXY - sumX * sumY) / denom;
  const intercept = (sumY - slope * sumX) / n;

  return { slope, intercept };
}

// ============================================================================
// Init
// ============================================================================

document.addEventListener('DOMContentLoaded', async () => {
  initFilters();
  initModeToggle();
  showLevel(1);
  try {
    State.cardNames = await API.getCardNames();
  } catch (err) {
    console.error('Failed to load card names:', err);
  }
  refreshDashboard();
});
