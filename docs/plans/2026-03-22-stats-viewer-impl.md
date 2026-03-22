# Stats Viewer Web Dashboard — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build an ECharts-based web dashboard served at `/dashboard` from the daemon's REST server, providing session trends, solo/duos comparison, partner breakdowns, and per-game drill-down.

**Architecture:** Static HTML/JS/CSS embedded into the Go binary via `embed.FS`. The REST server serves the dashboard alongside existing API endpoints. The dashboard fetches data from REST endpoints and renders with ECharts (dark theme). Three-level drill-down: session overview -> single game -> turn detail.

**Tech Stack:** Go `embed`, Apache ECharts (vendored JS), vanilla HTML/JS/CSS, existing REST server on `:8080`

---

## Task 1: Fix REST `gameStateJSON` DTO — Add Missing Fields

The REST DTO at `internal/api/rest/server.go:290-305` is missing many fields that the gRPC serializer already handles (EndTime, AvailableTribes, anomaly, PartnerBoard, OpponentBoard). The dashboard needs these.

**Files:**
- Modify: `internal/api/rest/server.go:290-324`

**Step 1: Write a test for the REST game endpoint response shape**

Create `internal/api/rest/server_test.go`:

```go
package rest

import (
	"encoding/json"
	"testing"
	"time"

	"battlestream.fixates.io/internal/gamestate"
)

func TestGameStateToJSON_AllFields(t *testing.T) {
	endTime := time.Now()
	gs := gamestate.BGGameState{
		GameID:    "test-1",
		Phase:     gamestate.PhaseGameOver,
		Turn:      10,
		TavernTier: 5,
		Player:    gamestate.PlayerState{Name: "TestPlayer", HeroCardID: "TB_Hero1"},
		Board:     []gamestate.MinionState{{EntityID: 1, Name: "Cat", Attack: 3, Health: 3}},
		EndTime:   &endTime,
		Placement: 2,
		IsDuos:    true,
		Partner:   &gamestate.PlayerState{Name: "PartnerPlayer", HeroCardID: "TB_Hero2"},
		PartnerBoard: &gamestate.PartnerBoard{
			Minions: []gamestate.MinionState{{EntityID: 2, Name: "Dog", Attack: 5, Health: 5}},
			Turn:    9,
			Stale:   true,
		},
		AnomalyCardID:      "BG27_Anomaly_303",
		AnomalyName:        "Free Refresh",
		AnomalyDescription: "The first minion you buy each turn is free.",
		AvailableTribes:    []string{"BEAST", "MECH", "DRAGON"},
		BuffSources:        []gamestate.BuffSource{{Category: "BLOODGEM", Attack: 10, Health: 10}},
		AbilityCounters:    []gamestate.AbilityCounter{{Category: "SPELLCRAFT", Value: 3, Display: "3"}},
	}

	j := gameStateToJSON(gs)

	// Check fields exist by marshaling to map
	data, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}

	required := []string{
		"game_id", "phase", "turn", "tavern_tier", "player", "board",
		"end_time_unix", "placement", "is_duos", "partner",
		"partner_board", "anomaly_card_id", "anomaly_name",
		"anomaly_description", "available_tribes", "buff_sources",
		"ability_counters",
	}
	for _, key := range required {
		if _, ok := m[key]; !ok {
			t.Errorf("missing key %q in JSON output", key)
		}
	}

	if j.EndTimeUnix == 0 {
		t.Error("end_time_unix should not be zero")
	}
	if len(j.AvailableTribes) != 3 {
		t.Errorf("expected 3 tribes, got %d", len(j.AvailableTribes))
	}
	if j.PartnerBoard == nil || len(j.PartnerBoard.Minions) != 1 {
		t.Error("partner_board should have 1 minion")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -count=1 -run TestGameStateToJSON_AllFields ./internal/api/rest/`
Expected: FAIL — missing fields in gameStateJSON struct

**Step 3: Update the gameStateJSON struct and converter**

In `internal/api/rest/server.go`, replace the `gameStateJSON` struct and `gameStateToJSON` function with a complete version that includes all fields matching what the gRPC converter already serializes:

```go
type gameStateJSON struct {
	GameID             string                       `json:"game_id"`
	Phase              string                       `json:"phase"`
	Turn               int                          `json:"turn"`
	TavernTier         int                          `json:"tavern_tier"`
	Player             gamestate.PlayerState        `json:"player"`
	Opponent           *gamestate.PlayerState       `json:"opponent,omitempty"`
	Board              []gamestate.MinionState      `json:"board"`
	OpponentBoard      []gamestate.MinionState      `json:"opponent_board,omitempty"`
	Modifications      []gamestate.StatMod          `json:"modifications"`
	BuffSources        []gamestate.BuffSource       `json:"buff_sources,omitempty"`
	AbilityCounters    []gamestate.AbilityCounter   `json:"ability_counters,omitempty"`
	Enchantments       []gamestate.Enchantment      `json:"enchantments,omitempty"`
	AvailableTribes    []string                     `json:"available_tribes,omitempty"`
	AnomalyCardID      string                       `json:"anomaly_card_id,omitempty"`
	AnomalyName        string                       `json:"anomaly_name,omitempty"`
	AnomalyDescription string                       `json:"anomaly_description,omitempty"`
	StartTimeUnix      int64                        `json:"start_time_unix"`
	EndTimeUnix        int64                        `json:"end_time_unix,omitempty"`
	Placement          int                          `json:"placement"`
	IsDuos             bool                         `json:"is_duos,omitempty"`
	Partner            *gamestate.PlayerState       `json:"partner,omitempty"`
	PartnerBoard       *gamestate.PartnerBoard      `json:"partner_board,omitempty"`
	PartnerBoardTurn   int                          `json:"partner_board_turn,omitempty"`
	PartnerBoardStale  bool                         `json:"partner_board_stale,omitempty"`
	PartnerBuffSources     []gamestate.BuffSource     `json:"partner_buff_sources,omitempty"`
	PartnerAbilityCounters []gamestate.AbilityCounter `json:"partner_ability_counters,omitempty"`
}

func gameStateToJSON(s gamestate.BGGameState) gameStateJSON {
	j := gameStateJSON{
		GameID:             s.GameID,
		Phase:              string(s.Phase),
		Turn:               s.Turn,
		TavernTier:         s.TavernTier,
		Player:             s.Player,
		Opponent:           s.Opponent,
		Board:              s.Board,
		OpponentBoard:      s.OpponentBoard,
		Modifications:      s.Modifications,
		BuffSources:        s.BuffSources,
		AbilityCounters:    s.AbilityCounters,
		Enchantments:       s.Enchantments,
		AvailableTribes:    s.AvailableTribes,
		AnomalyCardID:      s.AnomalyCardID,
		AnomalyName:        s.AnomalyName,
		AnomalyDescription: s.AnomalyDescription,
		StartTimeUnix:      s.StartTime.Unix(),
		Placement:          s.Placement,
		IsDuos:             s.IsDuos,
		Partner:            s.Partner,
		PartnerBoard:       s.PartnerBoard,
		PartnerBuffSources:     s.PartnerBuffSources,
		PartnerAbilityCounters: s.PartnerAbilityCounters,
	}
	if s.EndTime != nil {
		j.EndTimeUnix = s.EndTime.Unix()
	}
	if s.PartnerBoard != nil {
		j.PartnerBoardTurn = s.PartnerBoard.Turn
		j.PartnerBoardStale = s.PartnerBoard.Stale
	}
	return j
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race -count=1 -run TestGameStateToJSON_AllFields ./internal/api/rest/`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/rest/server.go internal/api/rest/server_test.go
git commit -m "fix: complete REST gameStateJSON DTO with all game state fields"
```

---

## Task 2: Add Mode Filter to ListGames and GetAggregate

Add `?mode=solo|duos|all` query parameter to filter games by mode.

**Files:**
- Modify: `internal/api/rest/server.go:114-130` (handleGetAggregate, handleListGames)
- Modify: `internal/store/store.go` (add ListGamesByMode or filter in REST layer)
- Test: `internal/api/rest/server_test.go`

**Step 1: Write test for mode filtering**

Add to `internal/api/rest/server_test.go`:

```go
func TestFilterMetasByMode(t *testing.T) {
	metas := []store.GameMeta{
		{GameID: "solo-1", Placement: 1, IsDuos: false},
		{GameID: "duos-1", Placement: 2, IsDuos: true},
		{GameID: "solo-2", Placement: 3, IsDuos: false},
		{GameID: "duos-2", Placement: 4, IsDuos: true},
	}

	solo := filterMetasByMode(metas, "solo")
	if len(solo) != 2 {
		t.Errorf("expected 2 solo games, got %d", len(solo))
	}

	duos := filterMetasByMode(metas, "duos")
	if len(duos) != 2 {
		t.Errorf("expected 2 duos games, got %d", len(duos))
	}

	all := filterMetasByMode(metas, "all")
	if len(all) != 4 {
		t.Errorf("expected 4 total games, got %d", len(all))
	}

	empty := filterMetasByMode(metas, "")
	if len(empty) != 4 {
		t.Errorf("expected 4 for empty mode, got %d", len(empty))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -count=1 -run TestFilterMetasByMode ./internal/api/rest/`
Expected: FAIL — `filterMetasByMode` not defined

**Step 3: Implement filterMetasByMode and update handlers**

Add to `internal/api/rest/server.go`:

```go
func filterMetasByMode(metas []store.GameMeta, mode string) []store.GameMeta {
	if mode == "" || mode == "all" {
		return metas
	}
	filtered := make([]store.GameMeta, 0, len(metas))
	for _, m := range metas {
		switch mode {
		case "solo":
			if !m.IsDuos {
				filtered = append(filtered, m)
			}
		case "duos":
			if m.IsDuos {
				filtered = append(filtered, m)
			}
		}
	}
	return filtered
}
```

Update `handleListGames` to read `r.URL.Query().Get("mode")` and apply the filter. Add `import "battlestream.fixates.io/internal/store"` to the import block. Also update `handleGetAggregate` to filter before computing:

```go
func (s *Server) handleGetAggregate(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	metas, err := s.grpc.GetStore().ListGames(0, 0)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	metas = filterMetasByMode(metas, mode)
	results := make([]stats.GameResult, len(metas))
	for i, m := range metas {
		results[i] = stats.GameResult{
			Placement: m.Placement,
			EndTime:   time.Unix(m.EndTime, 0),
			IsDuos:    m.IsDuos,
		}
	}
	s.writeJSON(w, stats.Compute(results))
}

func (s *Server) handleListGames(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	metas, err := s.grpc.GetStore().ListGames(0, 0)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	metas = filterMetasByMode(metas, mode)
	// Apply limit/offset from query params
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	limit := 50
	offset := 0
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}
	if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
		offset = v
	}
	total := len(metas)
	if offset >= len(metas) {
		s.writeJSON(w, map[string]interface{}{"games": []store.GameMeta{}, "total": total})
		return
	}
	metas = metas[offset:]
	if limit > 0 && limit < len(metas) {
		metas = metas[:limit]
	}
	s.writeJSON(w, map[string]interface{}{"games": metas, "total": total})
}
```

Note: Add `"strconv"` and `"battlestream.fixates.io/internal/store"` and `"battlestream.fixates.io/internal/stats"` to the import block.

**Step 4: Run test to verify it passes**

Run: `go test -race -count=1 -run TestFilterMetasByMode ./internal/api/rest/`
Expected: PASS

**Step 5: Run all tests**

Run: `go test -race -count=1 ./internal/api/rest/`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/api/rest/server.go internal/api/rest/server_test.go
git commit -m "feat: add mode filter to ListGames and GetAggregate REST endpoints"
```

---

## Task 3: Add Turn Snapshots REST Endpoint

**Files:**
- Modify: `internal/api/rest/server.go` (add handler + route)
- Test: `internal/api/rest/server_test.go`

**Step 1: Add the handler and route**

Add route in `Serve()` after the existing game routes:

```go
mux.HandleFunc("GET /v1/game/{game_id}/turns", s.withAuth(s.handleGetTurnSnapshots))
```

Add handler:

```go
func (s *Server) handleGetTurnSnapshots(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("game_id")
	if id == "" {
		http.Error(w, "game_id required", http.StatusBadRequest)
		return
	}
	snapshots, err := s.grpc.GetStore().GetTurnSnapshots(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if snapshots == nil {
		snapshots = []gamestate.TurnSnapshot{}
	}
	s.writeJSON(w, snapshots)
}
```

**Step 2: Run go vet**

Run: `go vet ./internal/api/rest/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/api/rest/server.go
git commit -m "feat: add GET /v1/game/{id}/turns endpoint for turn snapshots"
```

---

## Task 4: Vendor ECharts and Create Embedded Static Assets

**Files:**
- Create: `internal/api/rest/dashboard/` directory
- Create: `internal/api/rest/dashboard/echarts.min.js` (vendored from CDN)
- Create: `internal/api/rest/dashboard/index.html`
- Create: `internal/api/rest/dashboard/app.js`
- Create: `internal/api/rest/dashboard/style.css`
- Create: `internal/api/rest/embed.go`

**Step 1: Download ECharts**

```bash
curl -L -o internal/api/rest/dashboard/echarts.min.js \
  "https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js"
```

Verify it downloaded (should be ~800KB+):
```bash
ls -la internal/api/rest/dashboard/echarts.min.js
```

**Step 2: Create the embed.go file**

Create `internal/api/rest/embed.go`:

```go
package rest

import "embed"

//go:embed dashboard/*
var dashboardFS embed.FS
```

**Step 3: Create the HTML shell**

Create `internal/api/rest/dashboard/index.html` — a single-page app with:
- `<div id="app">` root container
- Script tags for `echarts.min.js` and `app.js`
- Link to `style.css`
- Dark background, no-scrollbar body
- Navigation breadcrumb area
- Mode toggle (Solo / Duos / Compare)
- Chart grid containers

The HTML should be a skeleton — all chart rendering happens in `app.js`.

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Battlestream Dashboard</title>
  <link rel="stylesheet" href="style.css">
</head>
<body>
  <div id="app">
    <header id="header">
      <h1>Battlestream</h1>
      <nav id="breadcrumb"></nav>
      <div id="mode-toggle">
        <button class="mode-btn active" data-mode="all">All</button>
        <button class="mode-btn" data-mode="solo">Solo</button>
        <button class="mode-btn" data-mode="duos">Duos</button>
        <button class="mode-btn" data-mode="compare">Compare</button>
      </div>
    </header>

    <!-- Duos partner filter (hidden unless duos/compare mode) -->
    <div id="partner-filter" class="hidden">
      <div id="partner-filter-bar"></div>
    </div>

    <!-- Level 1: Session Overview -->
    <div id="level-1" class="level">
      <div id="summary-cards" class="card-row"></div>
      <div class="chart-grid">
        <div class="chart-container" id="chart-placement-trend"></div>
        <div class="chart-container" id="chart-placement-dist"></div>
        <div class="chart-container" id="chart-winrate-trend"></div>
        <div class="chart-container" id="chart-hero-perf"></div>
        <div class="chart-container" id="chart-tavern-tier"></div>
        <div class="chart-container" id="chart-buff-breakdown"></div>
        <div class="chart-container" id="chart-duration"></div>
        <div class="chart-container" id="chart-anomaly-perf"></div>
        <div class="chart-container" id="chart-tribe-winrate"></div>
        <div class="chart-container" id="chart-buff-efficiency"></div>
        <div class="chart-container wide" id="chart-heatmap-hero"></div>
        <div class="chart-container wide" id="chart-heatmap-tier-turn"></div>
        <div class="chart-container wide" id="chart-heatmap-tribe"></div>
        <div class="chart-container wide" id="chart-heatmap-buff"></div>
      </div>
    </div>

    <!-- Level 2: Single Game -->
    <div id="level-2" class="level hidden">
      <div id="game-header"></div>
      <div class="chart-grid">
        <div class="chart-container" id="chart-board-stats"></div>
        <div class="chart-container" id="chart-health-armor"></div>
        <div class="chart-container" id="chart-tier-prog"></div>
        <div class="chart-container" id="chart-buff-accum"></div>
        <div class="chart-container" id="chart-gold-econ"></div>
        <div class="chart-container" id="chart-board-size"></div>
      </div>
    </div>

    <!-- Level 3: Turn Detail -->
    <div id="level-3" class="level hidden">
      <div id="turn-header"></div>
      <div id="turn-board"></div>
      <div id="turn-deltas"></div>
    </div>
  </div>

  <script src="echarts.min.js"></script>
  <script src="app.js"></script>
</body>
</html>
```

**Step 4: Create style.css**

Create `internal/api/rest/dashboard/style.css` — dark gaming theme:

```css
* { margin: 0; padding: 0; box-sizing: border-box; }

:root {
  --bg: #1a1a2e;
  --bg-card: #16213e;
  --bg-chart: #0f3460;
  --accent: #e94560;
  --text: #eee;
  --text-muted: #888;
  --win: #00c853;
  --loss: #ff5252;
  --border: #333;
}

body {
  font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
  background: var(--bg);
  color: var(--text);
  min-height: 100vh;
}

#header {
  display: flex;
  align-items: center;
  gap: 1.5rem;
  padding: 1rem 2rem;
  background: var(--bg-card);
  border-bottom: 1px solid var(--border);
}

#header h1 { font-size: 1.4rem; font-weight: 600; }
#breadcrumb { flex: 1; color: var(--text-muted); font-size: 0.9rem; }

#mode-toggle { display: flex; gap: 0.25rem; }
.mode-btn {
  padding: 0.4rem 1rem;
  border: 1px solid var(--border);
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  border-radius: 4px;
  font-size: 0.85rem;
  transition: all 0.2s;
}
.mode-btn:hover { border-color: var(--accent); color: var(--text); }
.mode-btn.active { background: var(--accent); color: #fff; border-color: var(--accent); }

#partner-filter {
  padding: 0.5rem 2rem;
  background: var(--bg-card);
  border-bottom: 1px solid var(--border);
}
#partner-filter-bar { display: flex; gap: 0.5rem; flex-wrap: wrap; align-items: center; }
.partner-tag {
  padding: 0.3rem 0.8rem;
  border: 1px solid var(--border);
  border-radius: 12px;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.2s;
}
.partner-tag.selected { background: var(--accent); border-color: var(--accent); color: #fff; }
.partner-tag.excluded { opacity: 0.3; text-decoration: line-through; }

.card-row {
  display: flex;
  gap: 1rem;
  padding: 1.5rem 2rem;
  flex-wrap: wrap;
}
.summary-card {
  flex: 1;
  min-width: 140px;
  padding: 1rem;
  background: var(--bg-card);
  border-radius: 8px;
  border: 1px solid var(--border);
}
.summary-card .label { font-size: 0.75rem; color: var(--text-muted); text-transform: uppercase; }
.summary-card .value { font-size: 1.6rem; font-weight: 700; margin-top: 0.3rem; }
.summary-card .delta { font-size: 0.8rem; margin-top: 0.2rem; }
.summary-card .delta.positive { color: var(--win); }
.summary-card .delta.negative { color: var(--loss); }

.chart-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 1rem;
  padding: 0 2rem 2rem;
}
.chart-container {
  background: var(--bg-card);
  border-radius: 8px;
  border: 1px solid var(--border);
  height: 350px;
  padding: 0.5rem;
}
.chart-container.wide { grid-column: span 2; height: 300px; }

#game-header {
  padding: 1.5rem 2rem;
  background: var(--bg-card);
  border-bottom: 1px solid var(--border);
  display: flex;
  gap: 2rem;
  align-items: center;
}

#turn-header { padding: 1.5rem 2rem; }
#turn-board {
  display: flex;
  gap: 0.5rem;
  padding: 0 2rem;
  flex-wrap: wrap;
}
.minion-card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 0.8rem;
  width: 120px;
  text-align: center;
}
.minion-card .name { font-size: 0.75rem; font-weight: 600; }
.minion-card .stats { font-size: 1.1rem; margin-top: 0.3rem; }
#turn-deltas { padding: 1.5rem 2rem; }

.level { transition: opacity 0.3s ease; }
.hidden { display: none; }

@media (max-width: 900px) {
  .chart-grid { grid-template-columns: 1fr; }
  .chart-container.wide { grid-column: span 1; }
}
```

**Step 5: Run go build to verify embed compiles**

Run: `go build ./internal/api/rest/`
Expected: PASS (no errors)

**Step 6: Commit**

```bash
git add internal/api/rest/dashboard/ internal/api/rest/embed.go
git commit -m "feat: add embedded dashboard assets with ECharts, HTML shell, and dark theme CSS"
```

---

## Task 5: Wire Dashboard Route into REST Server

**Files:**
- Modify: `internal/api/rest/server.go:42-54` (add dashboard file server route)

**Step 1: Add the dashboard file server**

In `Serve()`, add after the existing routes (before the `srv` creation):

```go
// Dashboard — serve embedded SPA
dashSub, _ := fs.Sub(dashboardFS, "dashboard")
mux.Handle("GET /dashboard/", http.StripPrefix("/dashboard/", http.FileServer(http.FS(dashSub))))
// Redirect bare /dashboard to /dashboard/
mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
})
```

Add `"io/fs"` to the import block.

**Step 2: Run go build**

Run: `go build ./internal/api/rest/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/api/rest/server.go
git commit -m "feat: wire /dashboard route serving embedded SPA from REST server"
```

---

## Task 6: Implement Dashboard JavaScript — Data Layer

The JS data layer fetches game data from REST endpoints, caches results, and provides filtered views.

**Files:**
- Create: `internal/api/rest/dashboard/app.js`

**Step 1: Write the data layer module**

Create `internal/api/rest/dashboard/app.js` with:

```javascript
// === Data Layer ===
const API = {
  async fetchJSON(url) {
    const resp = await fetch(url);
    if (!resp.ok) throw new Error(`${url}: ${resp.status}`);
    return resp.json();
  },

  async getAggregate(mode) {
    return this.fetchJSON(`/v1/stats/aggregate?mode=${mode || 'all'}`);
  },

  async getAllGames(mode) {
    // Fetch all game metas page by page (metas are small)
    const PAGE = 200;
    let all = [];
    let offset = 0;
    let total = Infinity;
    while (offset < total) {
      const params = new URLSearchParams({ limit: PAGE, offset, mode: mode || 'all' });
      const resp = await this.fetchJSON(`/v1/stats/games?${params}`);
      total = resp.total;
      all = all.concat(resp.games);
      offset += PAGE;
    }
    return all;
  },

  async getGame(id) {
    return this.fetchJSON(`/v1/game/${id}`);
  },

  async getTurns(id) {
    return this.fetchJSON(`/v1/game/${id}/turns`);
  },
};

// === State ===
const State = {
  mode: 'all',           // 'all' | 'solo' | 'duos' | 'compare'
  level: 1,              // 1 = overview, 2 = game, 3 = turn
  games: [],             // all GameMeta[]
  fullGames: new Map(),  // gameID -> full game state (cached)
  turnData: new Map(),   // gameID -> TurnSnapshot[] (cached)
  selectedGameID: null,
  selectedTurn: null,
  partners: new Set(),        // all partner battletags seen
  selectedPartners: new Set(), // currently selected partners (empty = all)
  excludedPartners: new Set(), // explicitly excluded partners
};
```

This is the foundation. The chart rendering code builds on top of this in subsequent tasks.

**Step 2: Verify it builds**

Run: `go build ./internal/api/rest/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/api/rest/dashboard/app.js
git commit -m "feat: add dashboard JS data layer with REST client and state management"
```

---

## Task 7: Implement Dashboard JavaScript — Navigation & Summary Cards

**Files:**
- Modify: `internal/api/rest/dashboard/app.js`

**Step 1: Add navigation, mode toggle, partner filter, and summary cards**

Append to `app.js`:

```javascript
// === Navigation ===
function showLevel(n) {
  State.level = n;
  document.querySelectorAll('.level').forEach(el => el.classList.add('hidden'));
  document.getElementById(`level-${n}`).classList.remove('hidden');
  updateBreadcrumb();
}

function updateBreadcrumb() {
  const bc = document.getElementById('breadcrumb');
  const parts = ['<a href="#" onclick="navigateTo(1);return false">Overview</a>'];
  if (State.level >= 2) {
    parts.push(`<a href="#" onclick="navigateTo(2);return false">Game</a>`);
  }
  if (State.level >= 3) {
    parts.push(`Turn ${State.selectedTurn}`);
  }
  bc.innerHTML = parts.join(' / ');
}

function navigateTo(level) {
  if (level === 1) {
    State.selectedGameID = null;
    State.selectedTurn = null;
    showLevel(1);
  } else if (level === 2 && State.selectedGameID) {
    State.selectedTurn = null;
    showLevel(2);
  }
}

// === Mode Toggle ===
function initModeToggle() {
  document.querySelectorAll('.mode-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.mode-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      State.mode = btn.dataset.mode;
      // Show/hide partner filter
      const pf = document.getElementById('partner-filter');
      pf.classList.toggle('hidden', State.mode !== 'duos' && State.mode !== 'compare');
      refreshDashboard();
    });
  });
}

// === Partner Filter ===
function buildPartnerFilter(games) {
  State.partners.clear();
  // Extract partner names from full game data
  games.forEach(g => {
    if (g.is_duos && g.partner && g.partner.name) {
      State.partners.add(g.partner.name);
    }
  });

  const bar = document.getElementById('partner-filter-bar');
  bar.innerHTML = '';

  if (State.partners.size === 0) return;

  // Quick actions
  const allBtn = document.createElement('button');
  allBtn.textContent = 'All';
  allBtn.className = 'mode-btn';
  allBtn.onclick = () => { State.selectedPartners.clear(); State.excludedPartners.clear(); updatePartnerTags(); refreshCharts(); };
  bar.appendChild(allBtn);

  const invertBtn = document.createElement('button');
  invertBtn.textContent = 'Invert';
  invertBtn.className = 'mode-btn';
  invertBtn.onclick = () => {
    const newExcluded = new Set([...State.partners].filter(p => !State.excludedPartners.has(p)));
    State.excludedPartners = newExcluded;
    updatePartnerTags();
    refreshCharts();
  };
  bar.appendChild(invertBtn);

  State.partners.forEach(name => {
    const tag = document.createElement('span');
    tag.className = 'partner-tag selected';
    tag.textContent = name;
    tag.dataset.partner = name;
    tag.onclick = (e) => {
      if (e.shiftKey) {
        // Toggle individual
        if (State.excludedPartners.has(name)) {
          State.excludedPartners.delete(name);
        } else {
          State.excludedPartners.add(name);
        }
      } else {
        // Solo select
        State.excludedPartners = new Set([...State.partners].filter(p => p !== name));
      }
      updatePartnerTags();
      refreshCharts();
    };
    tag.oncontextmenu = (e) => {
      e.preventDefault();
      // Right-click to exclude
      State.excludedPartners.add(name);
      updatePartnerTags();
      refreshCharts();
    };
    bar.appendChild(tag);
  });
}

function updatePartnerTags() {
  document.querySelectorAll('.partner-tag').forEach(tag => {
    const name = tag.dataset.partner;
    if (!name) return;
    tag.classList.toggle('excluded', State.excludedPartners.has(name));
    tag.classList.toggle('selected', !State.excludedPartners.has(name));
  });
}

function filterGamesByPartner(games) {
  if (State.excludedPartners.size === 0) return games;
  return games.filter(g => {
    if (!g.is_duos || !g.partner) return true;
    return !State.excludedPartners.has(g.partner.name);
  });
}

// === Summary Cards ===
function renderSummaryCards(agg, compareAgg) {
  const container = document.getElementById('summary-cards');
  const cards = [
    { label: 'Games Played', value: agg.games_played, compare: compareAgg?.games_played },
    { label: 'Win Rate', value: agg.games_played ? ((agg.wins / agg.games_played) * 100).toFixed(1) + '%' : '—',
      compare: compareAgg?.games_played ? ((compareAgg.wins / compareAgg.games_played) * 100).toFixed(1) + '%' : null },
    { label: 'Avg Placement', value: agg.avg_placement?.toFixed(2) || '—', compare: compareAgg?.avg_placement?.toFixed(2), invert: true },
    { label: 'Best', value: agg.best_placement || '—', compare: compareAgg?.best_placement },
    { label: 'Worst', value: agg.worst_placement || '—', compare: compareAgg?.worst_placement, invert: true },
  ];

  container.innerHTML = cards.map(c => {
    let deltaHTML = '';
    if (State.mode === 'compare' && c.compare != null) {
      deltaHTML = `<div class="delta">Duos: ${c.compare}</div>`;
    }
    return `<div class="summary-card">
      <div class="label">${c.label}</div>
      <div class="value">${c.value}</div>
      ${deltaHTML}
    </div>`;
  }).join('');
}
```

**Step 2: Verify it builds**

Run: `go build ./internal/api/rest/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/api/rest/dashboard/app.js
git commit -m "feat: add dashboard navigation, mode toggle, partner filter, and summary cards"
```

---

## Task 8: Implement Level 1 Charts — Placement Trend, Distribution, Win Rate

**Files:**
- Modify: `internal/api/rest/dashboard/app.js`

**Step 1: Add the core Level 1 chart renderers**

Append chart initialization and rendering functions for:
- `renderPlacementTrend(games)` — line chart, x=game index or date, y=placement (inverted: 1 at top). In Compare mode, two series (solo + duos). Clicking a point triggers drill-down.
- `renderPlacementDist(games)` — bar chart, x=placement (1-8), y=count. Side-by-side bars in Compare.
- `renderWinRateTrend(games)` — rolling average (last 20 games default) line chart.

Each function: `echarts.init(dom, 'dark')`, build option object, call `chart.setOption(option)`, register click handler for drill-down.

Store chart instances in a `Charts` map for resize/dispose management. Add `window.addEventListener('resize', ...)` to handle responsive resize.

**Step 2: Verify it builds**

Run: `go build ./internal/api/rest/`

**Step 3: Commit**

```bash
git add internal/api/rest/dashboard/app.js
git commit -m "feat: add Level 1 charts — placement trend, distribution, and win rate"
```

---

## Task 9: Implement Level 1 Charts — Hero, Tier, Buffs, Duration, Anomaly, Tribes

**Files:**
- Modify: `internal/api/rest/dashboard/app.js`

**Step 1: Add remaining Level 1 chart renderers**

These charts require full game state data (not just metas), so they need to fetch each game. Implement lazy loading: on first dashboard load, fetch metas first (fast), render what we can (placement trend, distribution, win rate from metas alone). Then fetch full game states in the background and render the richer charts.

Chart functions:
- `renderHeroPerf(games)` — horizontal bar, y=hero name, x=avg placement, sorted best-first. Needs full game state for hero CardID.
- `renderTavernTier(games)` — bar, x=max tier reached, y=count.
- `renderBuffBreakdown(games)` — stacked bar, x=buff category, y=total ATK+HP across games.
- `renderDuration(games)` — scatter, x=duration (minutes), y=placement. Duration = end_time - start_time.
- `renderAnomalyPerf(games)` — bar, x=anomaly name, y=avg placement.
- `renderTribeWinrate(games)` — grouped bar, x=dominant tribe, y=avg placement. Dominant tribe = most common minion_type on final board.
- `renderMinionImpact(games)` — table (HTML, not chart), sorted by frequency on final board.

**Step 2: Verify it builds and commit**

```bash
git add internal/api/rest/dashboard/app.js
git commit -m "feat: add Level 1 charts — hero, tier, buffs, duration, anomaly, tribes, minions"
```

---

## Task 10: Implement Level 1 Heatmaps

**Files:**
- Modify: `internal/api/rest/dashboard/app.js`

**Step 1: Add 4 heatmap renderers**

ECharts heatmap requires `series.type = 'heatmap'` with `visualMap` component.

- `renderHeatmapHero(games)` — x=placement (1-8), y=hero name, value=game count. Color scale: white(0) -> accent(max).
- `renderHeatmapTierTurn(games)` — x=turn number, y=tavern tier (1-6), value=game count. Uses turn snapshots to find when each tier was reached.
- `renderHeatmapTribe(games)` — x=placement (1-8), y=tribe, value=game count.
- `renderHeatmapBuff(games)` — x=placement (1-8), y=total buff bucket (0-10, 10-25, 25-50, 50-100, 100+), value=game count.

All heatmaps respect mode toggle and partner filter.

**Step 2: Verify and commit**

```bash
git add internal/api/rest/dashboard/app.js
git commit -m "feat: add Level 1 heatmaps — hero, tier-turn, tribe, buff efficiency"
```

---

## Task 11: Implement Level 2 — Single Game Turn-by-Turn Charts

**Files:**
- Modify: `internal/api/rest/dashboard/app.js`

**Step 1: Add game header renderer**

`renderGameHeader(game)` — displays hero name (resolved from CardID via cardnames if available), placement badge, date, duration, anomaly name, solo/duos badge, partner info.

**Step 2: Add turn-by-turn chart renderers**

All use turn snapshot data from `GET /v1/game/{id}/turns`. Shared x-axis: turn number.

- `renderBoardStats(turns)` — dual-axis line: total board ATK (left axis) and total board HP (right axis) per turn.
- `renderHealthArmor(turns)` — line: effective health (health - damage) and armor per turn.
- `renderTierProg(turns)` — step chart: tavern tier at each turn.
- `renderBuffAccum(turns)` — stacked area: each buff category's total value per turn.
- `renderGoldEcon(turns)` — line: max gold and current gold per turn.
- `renderBoardSize(turns)` — line: number of minions on board per turn.

Clicking a data point on any chart navigates to Level 3 for that turn.

**Step 3: Verify and commit**

```bash
git add internal/api/rest/dashboard/app.js
git commit -m "feat: add Level 2 single game turn-by-turn charts"
```

---

## Task 12: Implement Level 3 — Turn Detail

**Files:**
- Modify: `internal/api/rest/dashboard/app.js`

**Step 1: Add turn detail renderers**

- `renderTurnDetail(snapshot)` — populates:
  - `#turn-header`: "Turn N" with phase/tier/gold info
  - `#turn-board`: minion cards rendered as HTML divs (`.minion-card` with name, ATK/HP, enchantment count)
  - `#turn-deltas`: buff deltas (category, +ATK/+HP) and ability deltas as a simple table

**Step 2: Verify and commit**

```bash
git add internal/api/rest/dashboard/app.js
git commit -m "feat: add Level 3 turn detail view with board snapshot and deltas"
```

---

## Task 13: Implement Main Orchestration — Init, Refresh, Compare Mode

**Files:**
- Modify: `internal/api/rest/dashboard/app.js`

**Step 1: Add the main init and refresh orchestration**

```javascript
// === Main ===
async function refreshDashboard() {
  const games = await API.getAllGames(State.mode === 'compare' ? 'all' : State.mode);
  State.games = games;
  if (State.mode === 'duos' || State.mode === 'compare') {
    // Need full game data for partner names — fetch first page
    await fetchFullGames(games.slice(0, 50));
    buildPartnerFilter(
      games.map(g => State.fullGames.get(g.game_id)).filter(Boolean)
    );
  }
  await refreshCharts();
}

async function refreshCharts() {
  if (State.level === 1) {
    await renderLevel1();
  } else if (State.level === 2) {
    await renderLevel2();
  }
}

async function renderLevel1() {
  let games = State.games;
  // Fetch aggregates
  if (State.mode === 'compare') {
    const [soloAgg, duosAgg] = await Promise.all([
      API.getAggregate('solo'),
      API.getAggregate('duos'),
    ]);
    renderSummaryCards(soloAgg, duosAgg);
  } else {
    const agg = await API.getAggregate(State.mode);
    renderSummaryCards(agg);
  }
  // Charts from metas (fast)
  renderPlacementTrend(games);
  renderPlacementDist(games);
  renderWinRateTrend(games);
  renderDuration(games);

  // Fetch full game data for richer charts (lazy)
  await fetchFullGames(games);
  const fullGames = games.map(g => State.fullGames.get(g.game_id)).filter(Boolean);
  const filtered = filterGamesByPartner(fullGames);

  renderHeroPerf(filtered);
  renderTavernTier(filtered);
  renderBuffBreakdown(filtered);
  renderAnomalyPerf(filtered);
  renderTribeWinrate(filtered);
  renderHeatmapHero(filtered);
  renderHeatmapTribe(filtered);
  renderHeatmapBuff(filtered);
}

async function renderLevel2() {
  const id = State.selectedGameID;
  if (!State.fullGames.has(id)) {
    State.fullGames.set(id, await API.getGame(id));
  }
  if (!State.turnData.has(id)) {
    State.turnData.set(id, await API.getTurns(id));
  }
  const game = State.fullGames.get(id);
  const turns = State.turnData.get(id);
  renderGameHeader(game);
  renderBoardStats(turns);
  renderHealthArmor(turns);
  renderTierProg(turns);
  renderBuffAccum(turns);
  renderGoldEcon(turns);
  renderBoardSize(turns);
}

async function fetchFullGames(metas) {
  const missing = metas.filter(m => !State.fullGames.has(m.game_id));
  // Fetch in batches of 10 to avoid overwhelming
  for (let i = 0; i < missing.length; i += 10) {
    const batch = missing.slice(i, i + 10);
    const results = await Promise.all(batch.map(m => API.getGame(m.game_id)));
    results.forEach((g, idx) => State.fullGames.set(batch[idx].game_id, g));
  }
}

async function drillToGame(gameID) {
  State.selectedGameID = gameID;
  showLevel(2);
  await renderLevel2();
}

function drillToTurn(turn, snapshot) {
  State.selectedTurn = turn;
  showLevel(3);
  renderTurnDetail(snapshot);
}

// === Init ===
document.addEventListener('DOMContentLoaded', () => {
  initModeToggle();
  showLevel(1);
  refreshDashboard();
});
```

**Step 2: Verify full build**

Run: `go build ./cmd/battlestream`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/api/rest/dashboard/app.js
git commit -m "feat: add dashboard main orchestration with init, refresh, compare mode"
```

---

## Task 14: Integration Test — Build + Serve

**Files:**
- None new — manual verification

**Step 1: Build the binary**

Run: `go build -o ./battlestream ./cmd/battlestream`
Expected: binary produced

**Step 2: Run go vet**

Run: `go vet ./...`
Expected: PASS

**Step 3: Run all tests**

Run: `go test -race -count=1 ./...`
Expected: PASS

**Step 4: Verify dashboard loads**

Start daemon in background, then curl the dashboard:

```bash
./battlestream daemon &
sleep 2
curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8080/dashboard/
# Expected: 200
curl -s http://127.0.0.1:8080/dashboard/ | head -5
# Expected: HTML starting with <!DOCTYPE html>
kill %1
```

**Step 5: Commit if any fixes were needed**

---

## Task 15: Polish — Chart Animations, Responsive Resize, Error States

**Files:**
- Modify: `internal/api/rest/dashboard/app.js`
- Modify: `internal/api/rest/dashboard/style.css`

**Step 1: Add chart resize handling**

```javascript
const Charts = {};
window.addEventListener('resize', () => {
  Object.values(Charts).forEach(c => c.resize());
});
```

**Step 2: Add ECharts animation config to all charts**

Ensure every `setOption` call includes:
```javascript
animationDuration: 800,
animationEasing: 'cubicOut',
```

**Step 3: Add empty/error state rendering**

When no games exist, show a centered message: "No games recorded yet. Play some Battlegrounds!"

**Step 4: Add loading spinner**

Show a spinner while fetching data, hide when charts render.

**Step 5: Commit**

```bash
git add internal/api/rest/dashboard/
git commit -m "feat: polish dashboard with animations, responsive resize, and empty states"
```

---

## Task 16: Final Verification

**Step 1: Run full test suite**

Run: `go test -race -count=1 ./...`
Expected: ALL PASS

**Step 2: Run go vet**

Run: `go vet ./...`
Expected: PASS

**Step 3: Build final binary**

Run: `go build ./cmd/battlestream`
Expected: PASS

---

## Follow-up Items (not in scope)

- **Buy/sell tracking** — new parser/processor work to capture card transactions with tribe/gold-type breakdowns
- **Session grouping** — detect play sessions by time gaps between games
- **Streak tracking** — consecutive top-4/bottom-4 indicators
- **Tier×Turn heatmap** — requires loading turn snapshots for all games (expensive); defer or make lazy
