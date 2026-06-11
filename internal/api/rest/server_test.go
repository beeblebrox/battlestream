package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
