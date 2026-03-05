package store

import (
	"testing"
	"time"

	"battlestream.fixates.io/internal/gamestate"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestGetAggregateEmpty(t *testing.T) {
	st := openTestStore(t)
	agg, err := st.GetAggregate()
	if err != nil {
		t.Fatal(err)
	}
	if agg.GamesPlayed != 0 || agg.Wins != 0 || agg.Losses != 0 {
		t.Errorf("expected zero aggregate for empty store, got %+v", agg)
	}
}

func TestListGamesEmpty(t *testing.T) {
	st := openTestStore(t)
	games, err := st.ListGames(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 0 {
		t.Errorf("expected 0 games, got %d", len(games))
	}
}

func TestSaveAndListGames(t *testing.T) {
	st := openTestStore(t)

	metas := []GameMeta{
		{GameID: "game-1", StartTime: time.Now().Unix(), Placement: 1},
		{GameID: "game-2", StartTime: time.Now().Unix(), Placement: 5},
		{GameID: "game-3", StartTime: time.Now().Unix(), Placement: 3},
	}
	for _, m := range metas {
		if err := st.SaveGame(m, m.Placement); err != nil {
			t.Fatalf("SaveGame(%s): %v", m.GameID, err)
		}
	}

	games, err := st.ListGames(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 3 {
		t.Fatalf("expected 3 games, got %d", len(games))
	}
}

func TestListGamesPagination(t *testing.T) {
	st := openTestStore(t)

	for i := 1; i <= 5; i++ {
		m := GameMeta{GameID: string(rune('A' + i - 1)), StartTime: int64(i), Placement: i}
		if err := st.SaveGame(m, i); err != nil {
			t.Fatal(err)
		}
	}

	page, err := st.ListGames(2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 {
		t.Errorf("expected 2 results with limit=2 offset=1, got %d", len(page))
	}
}

func TestGetAggregateWinsLosses(t *testing.T) {
	st := openTestStore(t)

	placements := []int{1, 2, 4, 5, 8} // 3 wins, 2 losses
	for i, p := range placements {
		m := GameMeta{GameID: string(rune('A' + i)), StartTime: int64(i), Placement: p}
		if err := st.SaveGame(m, p); err != nil {
			t.Fatal(err)
		}
	}

	agg, err := st.GetAggregate()
	if err != nil {
		t.Fatal(err)
	}
	if agg.GamesPlayed != 5 {
		t.Errorf("GamesPlayed: expected 5, got %d", agg.GamesPlayed)
	}
	if agg.Wins != 3 {
		t.Errorf("Wins: expected 3, got %d", agg.Wins)
	}
	if agg.Losses != 2 {
		t.Errorf("Losses: expected 2, got %d", agg.Losses)
	}
	expected := float64(1+2+4+5+8) / 5.0
	if agg.AvgPlacement != expected {
		t.Errorf("AvgPlacement: expected %.2f, got %.2f", expected, agg.AvgPlacement)
	}
}

func TestSaveFullGameAndRetrieve(t *testing.T) {
	st := openTestStore(t)

	now := time.Now()
	end := now.Add(10 * time.Minute)
	gs := gamestate.BGGameState{
		GameID:    "game-99",
		Phase:     gamestate.PhaseGameOver,
		Turn:      12,
		Placement: 2,
		StartTime: now,
		EndTime:   &end,
		Player: gamestate.PlayerState{
			Name:   "Fixates",
			Health: 15,
			Armor:  0,
		},
		Board: []gamestate.MinionState{
			{EntityID: 1, Name: "Murloc", Attack: 3, Health: 4},
		},
	}

	if err := st.SaveFullGame(gs); err != nil {
		t.Fatalf("SaveFullGame: %v", err)
	}

	retrieved, err := st.GetGame("game-99")
	if err != nil {
		t.Fatalf("GetGame: %v", err)
	}

	if retrieved.GameID != "game-99" {
		t.Errorf("GameID: expected game-99, got %q", retrieved.GameID)
	}
	if retrieved.Placement != 2 {
		t.Errorf("Placement: expected 2, got %d", retrieved.Placement)
	}
	if retrieved.Player.Name != "Fixates" {
		t.Errorf("Player.Name: expected Fixates, got %q", retrieved.Player.Name)
	}
	if len(retrieved.Board) != 1 || retrieved.Board[0].Attack != 3 {
		t.Errorf("Board not preserved correctly: %+v", retrieved.Board)
	}
}

func TestGetGameNotFound(t *testing.T) {
	st := openTestStore(t)
	_, err := st.GetGame("nonexistent")
	if err == nil {
		t.Error("expected error for missing game, got nil")
	}
}
