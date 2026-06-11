package rest

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	grpcserver "battlestream.fixates.io/internal/api/grpc"
	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/store"
)

// newStoreBackedServer returns a REST Server wired to a real store in a temp dir.
func newStoreBackedServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(grpcserver.New(nil, st, nil), ""), st
}

func TestGameStateToJSON_AllFields(t *testing.T) {
	now := time.Now()
	end := now.Add(20 * time.Minute)

	gs := gamestate.BGGameState{
		GameID:     "test-game-123",
		Phase:      gamestate.PhaseRecruit,
		Turn:       5,
		TavernTier: 3,
		Player: gamestate.PlayerState{
			Name:       "TestPlayer",
			HeroCardID: "BG_HERO_01",
			Health:     40,
			MaxHealth:  40,
			Damage:     5,
			Armor:      10,
		},
		Opponent: &gamestate.PlayerState{
			Name:       "OpponentPlayer",
			HeroCardID: "BG_HERO_02",
			Health:     30,
			MaxHealth:  40,
		},
		Board: []gamestate.MinionState{
			{EntityID: 1, CardID: "BG_MINION_01", Name: "TestMinion", Attack: 3, Health: 4},
		},
		OpponentBoard: []gamestate.MinionState{
			{EntityID: 100, CardID: "BG_MINION_02", Name: "OppMinion", Attack: 5, Health: 6},
		},
		Modifications: []gamestate.StatMod{
			{Turn: 3, Target: "ALL", Stat: "ATTACK", Delta: 1},
		},
		BuffSources: []gamestate.BuffSource{
			{Category: "BLOODGEM", Attack: 2, Health: 2},
		},
		AbilityCounters: []gamestate.AbilityCounter{
			{Category: "SPELLCRAFT", Value: 3, Display: "3"},
		},
		Enchantments: []gamestate.Enchantment{
			{EntityID: 50, CardID: "BG_ENC_01", TargetID: 1, AttackBuff: 1, HealthBuff: 1},
		},
		AvailableTribes: []string{"BEAST", "MECH", "DRAGON"},
		AnomalyCardID:      "BG_ANOMALY_01",
		AnomalyName:        "TestAnomaly",
		AnomalyDescription: "Test anomaly description",
		StartTime:          now,
		EndTime:            &end,
		Placement:          4,
		IsDuos:             true,
		Partner: &gamestate.PlayerState{
			Name:       "PartnerPlayer",
			HeroCardID: "BG_HERO_03",
			Health:     35,
			MaxHealth:  40,
		},
		PartnerBoard: &gamestate.PartnerBoard{
			Minions: []gamestate.MinionState{
				{EntityID: 200, CardID: "BG_MINION_03", Name: "PartnerMinion", Attack: 7, Health: 8},
			},
			Turn:  4,
			Stale: false,
		},
		PartnerBuffSources: []gamestate.BuffSource{
			{Category: "NOMI", Attack: 0, Health: 5},
		},
		PartnerAbilityCounters: []gamestate.AbilityCounter{
			{Category: "FREE_REFRESH", Value: 2, Display: "2"},
		},
	}

	result := gameStateToJSON(gs)

	// Marshal to JSON and check all keys are present.
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	requiredKeys := []string{
		"game_id", "phase", "turn", "tavern_tier", "player", "board",
		"modifications", "buff_sources", "ability_counters", "enchantments",
		"start_time_unix", "placement", "is_duos", "partner",
		"end_time_unix", "available_tribes",
		"anomaly_card_id", "anomaly_name", "anomaly_description",
		"opponent", "opponent_board",
		"partner_board", "partner_board_turn", "partner_board_stale",
		"partner_buff_sources", "partner_ability_counters",
	}

	for _, key := range requiredKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("missing key %q in JSON output", key)
		}
	}

	// Verify specific values.
	if v, ok := m["end_time_unix"].(float64); !ok || v == 0 {
		t.Errorf("end_time_unix should be non-zero, got %v", m["end_time_unix"])
	}

	tribes, ok := m["available_tribes"].([]interface{})
	if !ok {
		t.Fatalf("available_tribes is not an array")
	}
	if len(tribes) != 3 {
		t.Errorf("expected 3 available_tribes, got %d", len(tribes))
	}

	pb, ok := m["partner_board"].([]interface{})
	if !ok {
		t.Fatalf("partner_board is not an array")
	}
	if len(pb) != 1 {
		t.Errorf("expected 1 partner_board minion, got %d", len(pb))
	}

	ob, ok := m["opponent_board"].([]interface{})
	if !ok {
		t.Fatalf("opponent_board is not an array")
	}
	if len(ob) != 1 {
		t.Errorf("expected 1 opponent_board minion, got %d", len(ob))
	}
}

func TestWithAuth_EmptyKey_AllowsAllRequests(t *testing.T) {
	// Server with empty key already bypasses auth — this is the --no-auth path.
	s := New(nil, "")
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil) // no Authorization header
	rw := httptest.NewRecorder()
	handler(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rw.Code)
	}
}

func TestWithAuth_NonEmptyKey_RejectsUnauthenticated(t *testing.T) {
	s := New(nil, "secret")
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// No token — should be rejected.
	req := httptest.NewRequest("GET", "/", nil)
	rw := httptest.NewRecorder()
	handler(rw, req)
	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rw.Code)
	}

	// Correct token — should pass through.
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Authorization", "Bearer secret")
	rw2 := httptest.NewRecorder()
	handler(rw2, req2)
	if rw2.Code != http.StatusOK {
		t.Fatalf("want 200 with correct token, got %d", rw2.Code)
	}
}

// TestHandleListGames_FiltersIncompleteAndTotal pins the reference REST
// behavior (audit H8): placement-0 games are excluded and total reflects the
// full filtered count, not the page size.
func TestHandleListGames_FiltersIncompleteAndTotal(t *testing.T) {
	s, st := newStoreBackedServer(t)
	for i, placement := range []int{1, 4, 0, 7} { // one stale game
		meta := store.GameMeta{
			GameID:    "g-" + string(rune('a'+i)),
			StartTime: time.Now().Unix(),
			Placement: placement,
		}
		if err := st.SaveGame(meta); err != nil {
			t.Fatalf("SaveGame: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "/v1/stats/games?limit=2", nil)
	rw := httptest.NewRecorder()
	s.handleListGames(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rw.Code)
	}

	var resp struct {
		Games []store.GameMeta `json:"games"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(rw.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("total = %d, want 3 (stale game excluded, full count)", resp.Total)
	}
	if len(resp.Games) != 2 {
		t.Errorf("len(games) = %d, want 2 (page size)", len(resp.Games))
	}
	for _, g := range resp.Games {
		if g.Placement == 0 {
			t.Errorf("REST returned incomplete game %q", g.GameID)
		}
	}
}

// TestHandleGetGame_NotFound404: a missing game must return 404.
func TestHandleGetGame_NotFound404(t *testing.T) {
	s, _ := newStoreBackedServer(t)
	req := httptest.NewRequest("GET", "/v1/game/nope", nil)
	req.SetPathValue("game_id", "nope")
	rw := httptest.NewRecorder()
	s.handleGetGame(rw, req)
	if rw.Code != http.StatusNotFound {
		t.Fatalf("want 404 for missing game, got %d", rw.Code)
	}
}

// TestHandleGetGame_StoreError500 is a regression test for audit H8 bug 3:
// a store failure (closed DB) must return 500, not 404.
func TestHandleGetGame_StoreError500(t *testing.T) {
	s, st := newStoreBackedServer(t)
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	req := httptest.NewRequest("GET", "/v1/game/any", nil)
	req.SetPathValue("game_id", "any")
	rw := httptest.NewRecorder()
	s.handleGetGame(rw, req)
	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 for store failure, got %d", rw.Code)
	}
}

// TestHandleGetModifications_StoreError500: same not-found/internal split for
// the modifications endpoint.
func TestHandleGetModifications_StoreError500(t *testing.T) {
	s, st := newStoreBackedServer(t)
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	req := httptest.NewRequest("GET", "/v1/stats/games/any/modifications", nil)
	req.SetPathValue("game_id", "any")
	rw := httptest.NewRecorder()
	s.handleGetModifications(rw, req)
	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 for store failure, got %d", rw.Code)
	}
}

// TestHandleGetModifications_NotFound404: missing game stays a 404.
func TestHandleGetModifications_NotFound404(t *testing.T) {
	s, _ := newStoreBackedServer(t)
	req := httptest.NewRequest("GET", "/v1/stats/games/nope/modifications", nil)
	req.SetPathValue("game_id", "nope")
	rw := httptest.NewRecorder()
	s.handleGetModifications(rw, req)
	if rw.Code != http.StatusNotFound {
		t.Fatalf("want 404 for missing game, got %d", rw.Code)
	}
}

// newWSTestServer starts an httptest server that serves only the WebSocket
// handler, with the hub running so successful upgrades can register.
func newWSTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := New(nil, "")
	ctx, cancel := context.WithCancel(context.Background())
	go s.hub.run(ctx)
	ts := httptest.NewServer(http.HandlerFunc(s.handleWebSocket))
	t.Cleanup(func() {
		ts.Close()
		cancel()
	})
	return ts
}

func wsURL(ts *httptest.Server) string {
	return "ws" + strings.TrimPrefix(ts.URL, "http")
}

func TestWebSocketOrigin_EvilRejected(t *testing.T) {
	ts := newWSTestServer(t)
	hdr := http.Header{"Origin": []string{"https://evil.example"}}
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL(ts), hdr)
	if err == nil {
		conn.Close()
		t.Fatalf("want upgrade rejected for Origin https://evil.example, got success")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		t.Fatalf("want 403 for evil origin, got %d (err=%v)", code, err)
	}
}

func TestWebSocketOrigin_NoOriginAllowed(t *testing.T) {
	ts := newWSTestServer(t)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(ts), nil)
	if err != nil {
		t.Fatalf("want upgrade success with no Origin header, got err: %v", err)
	}
	conn.Close()
}

func TestWebSocketOrigin_LocalhostAllowed(t *testing.T) {
	ts := newWSTestServer(t)
	hdr := http.Header{"Origin": []string{"http://localhost:8080"}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(ts), hdr)
	if err != nil {
		t.Fatalf("want upgrade success with Origin http://localhost:8080, got err: %v", err)
	}
	conn.Close()
}

// sseGet performs a GET against the SSE handler and returns the response once
// headers arrive. The request is cancelled on test cleanup.
func sseGet(t *testing.T, ts *httptest.Server, origin string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE headers did not arrive immediately (handler must flush headers on connect): %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func newSSETestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := New(grpcserver.New(nil, nil, nil), "")
	ts := httptest.NewServer(http.HandlerFunc(s.handleSSE))
	t.Cleanup(ts.Close)
	return ts
}

func TestSSE_ImmediateHeaders_NoOrigin(t *testing.T) {
	ts := newSSETestServer(t)
	resp := sseGet(t, ts, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao != "" {
		t.Errorf("Access-Control-Allow-Origin should be absent without an Origin header, got %q", acao)
	}
}

func TestSSE_LocalhostOrigin_EchoedACAO(t *testing.T) {
	ts := newSSETestServer(t)
	resp := sseGet(t, ts, "http://127.0.0.1:8080")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao != "http://127.0.0.1:8080" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the localhost request origin (never *)", acao)
	}
}

func TestSSE_EvilOrigin_Rejected(t *testing.T) {
	ts := newSSETestServer(t)
	resp := sseGet(t, ts, "https://evil.example")
	if resp.StatusCode == http.StatusOK {
		if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao != "" {
			t.Fatalf("evil origin must not receive Access-Control-Allow-Origin, got %q", acao)
		}
	} else if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 (or 200 without ACAO) for evil origin, got %d", resp.StatusCode)
	}
}

func TestSSE_Keepalive(t *testing.T) {
	oldInterval := sseKeepaliveInterval
	sseKeepaliveInterval = 50 * time.Millisecond
	t.Cleanup(func() { sseKeepaliveInterval = oldInterval })

	ts := newSSETestServer(t)
	resp := sseGet(t, ts, "")
	reader := bufio.NewReader(resp.Body)

	// The handler writes an initial comment on connect...
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("reading first SSE line: %v", err)
	}
	if !strings.HasPrefix(line, ":") {
		t.Errorf("first SSE line = %q, want a comment line starting with ':'", line)
	}

	// ...and periodic keepalive comments thereafter.
	sawKeepalive := false
	for i := 0; i < 10; i++ {
		line, err = reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading SSE stream: %v", err)
		}
		if strings.HasPrefix(line, ": keepalive") {
			sawKeepalive = true
			break
		}
	}
	if !sawKeepalive {
		t.Errorf("no keepalive comment observed on SSE stream")
	}
}

func TestHostCheck(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name     string
		bindHost string
		reqHost  string
		want     int
	}{
		{"evil host rejected", "127.0.0.1", "evil.example:8080", http.StatusForbidden},
		{"127.0.0.1 allowed", "127.0.0.1", "127.0.0.1:8080", http.StatusOK},
		{"localhost allowed", "127.0.0.1", "localhost:8080", http.StatusOK},
		{"ipv6 loopback allowed", "127.0.0.1", "[::1]:8080", http.StatusOK},
		{"no port allowed", "127.0.0.1", "localhost", http.StatusOK},
		{"configured non-localhost bind host allowed", "192.168.1.5", "192.168.1.5:8080", http.StatusOK},
		{"other host rejected on non-localhost bind", "192.168.1.5", "evil.example:8080", http.StatusForbidden},
		{"wildcard bind allows any host", "", "anything.example:8080", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := hostCheckHandler(tt.bindHost, inner)
			req := httptest.NewRequest("GET", "/v1/health", nil)
			req.Host = tt.reqHost
			rw := httptest.NewRecorder()
			h.ServeHTTP(rw, req)
			if rw.Code != tt.want {
				t.Errorf("bind %q, Host %q: got %d, want %d", tt.bindHost, tt.reqHost, rw.Code, tt.want)
			}
		})
	}
}

func TestHandleGetPlayer_NotImplemented(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.SaveGame(store.GameMeta{GameID: "g1", Placement: 3}); err != nil {
		t.Fatalf("saving game: %v", err)
	}

	s := New(grpcserver.New(nil, st, nil), "")
	req := httptest.NewRequest("GET", "/v1/player/SomeName", nil)
	req.SetPathValue("name", "SomeName")
	rw := httptest.NewRecorder()
	s.handleGetPlayer(rw, req)

	// The store does not record player names per game, so the endpoint must
	// not pretend to filter — it returns 501 Not Implemented.
	if rw.Code != http.StatusNotImplemented {
		t.Fatalf("want 501 Not Implemented, got %d (body: %s)", rw.Code, rw.Body.String())
	}
}

func TestFilterMetasByMode(t *testing.T) {
	metas := []store.GameMeta{
		{GameID: "solo-1", Placement: 1, IsDuos: false},
		{GameID: "duos-1", Placement: 2, IsDuos: true},
		{GameID: "solo-2", Placement: 3, IsDuos: false},
		{GameID: "duos-2", Placement: 4, IsDuos: true},
	}

	tests := []struct {
		name     string
		mode     string
		wantIDs  []string
	}{
		{"empty returns all", "", []string{"solo-1", "duos-1", "solo-2", "duos-2"}},
		{"all returns all", "all", []string{"solo-1", "duos-1", "solo-2", "duos-2"}},
		{"solo filters duos", "solo", []string{"solo-1", "solo-2"}},
		{"duos filters solo", "duos", []string{"duos-1", "duos-2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterMetasByMode(metas, tt.mode)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("filterMetasByMode(%q) returned %d metas, want %d", tt.mode, len(got), len(tt.wantIDs))
			}
			for i, m := range got {
				if m.GameID != tt.wantIDs[i] {
					t.Errorf("filterMetasByMode(%q)[%d].GameID = %q, want %q", tt.mode, i, m.GameID, tt.wantIDs[i])
				}
			}
		})
	}
}
