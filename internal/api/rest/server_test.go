package rest

import (
	"encoding/json"
	"testing"
	"time"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/store"
)

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
