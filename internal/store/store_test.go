package store

import (
	"bytes"
	"errors"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"

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
		if err := st.SaveGame(m); err != nil {
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
		if err := st.SaveGame(m); err != nil {
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
		if err := st.SaveGame(m); err != nil {
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
	// Audit H8: the missing-game error must be the ErrGameNotFound sentinel so
	// API layers can distinguish 404/NotFound from real store failures.
	if !errors.Is(err, ErrGameNotFound) {
		t.Errorf("GetGame missing game: err = %v, want errors.Is(err, ErrGameNotFound)", err)
	}
}

func TestGetGameStoreFailureIsNotNotFound(t *testing.T) {
	st := openTestStore(t)
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := st.GetGame("any")
	if err == nil {
		t.Fatal("expected error on closed store, got nil")
	}
	if errors.Is(err, ErrGameNotFound) {
		t.Errorf("closed-store error must not match ErrGameNotFound: %v", err)
	}
}

func TestFilterCompleteGames(t *testing.T) {
	metas := []GameMeta{
		{GameID: "done-1", Placement: 1},
		{GameID: "stale", Placement: 0},
		{GameID: "done-2", Placement: 8},
	}
	got := FilterCompleteGames(metas)
	if len(got) != 2 || got[0].GameID != "done-1" || got[1].GameID != "done-2" {
		t.Errorf("FilterCompleteGames = %+v, want [done-1 done-2]", got)
	}
}

// --- Regression tests for audit finding M6 (store game-list integrity) ---

// Duplicate SaveFullGame calls for the same GameID must not double-count the
// game in ListGames or GetAggregate.
func TestSaveFullGameDuplicateDoesNotDoubleCount(t *testing.T) {
	st := openTestStore(t)

	gs := gamestate.BGGameState{
		GameID:    "dup-game",
		Placement: 3,
		StartTime: time.Now(),
	}
	if err := st.SaveFullGame(gs); err != nil {
		t.Fatalf("first SaveFullGame: %v", err)
	}
	// Second save of the same game (e.g. reparse or HasGame race).
	gs.Placement = 4
	if err := st.SaveFullGame(gs); err != nil {
		t.Fatalf("second SaveFullGame: %v", err)
	}

	games, err := st.ListGames(0, 0)
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 game after duplicate save, got %d", len(games))
	}
	// Meta overwrite is idempotent: latest placement wins.
	if games[0].Placement != 4 {
		t.Errorf("expected meta overwritten with placement 4, got %d", games[0].Placement)
	}

	agg, err := st.GetAggregate()
	if err != nil {
		t.Fatalf("GetAggregate: %v", err)
	}
	if agg.GamesPlayed != 1 {
		t.Errorf("GamesPlayed: expected 1 after duplicate save, got %d", agg.GamesPlayed)
	}
}

// Empty GameIDs must be rejected, never written under bare prefix keys.
func TestEmptyGameIDRejected(t *testing.T) {
	st := openTestStore(t)

	if err := st.SaveFullGame(gamestate.BGGameState{StartTime: time.Now()}); err == nil {
		t.Error("SaveFullGame with empty GameID: expected error, got nil")
	}
	if err := st.SaveGame(GameMeta{StartTime: time.Now().Unix()}); err == nil {
		t.Error("SaveGame with empty GameID: expected error, got nil")
	}
	if has, err := st.HasGame(""); has || err != nil {
		t.Errorf("HasGame(\"\"): expected (false, nil), got (%v, %v)", has, err)
	}

	games, err := st.ListGames(0, 0)
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if len(games) != 0 {
		t.Errorf("expected 0 games after rejected save, got %d", len(games))
	}
}

// After SaveFullGame, both the meta and the full state must exist: every ID
// returned by ListGames must be retrievable via GetGame.
func TestSaveFullGameAtomicMetaAndState(t *testing.T) {
	st := openTestStore(t)

	for _, id := range []string{"atomic-1", "atomic-2"} {
		gs := gamestate.BGGameState{
			GameID:    id,
			Placement: 1,
			StartTime: time.Now(),
		}
		if err := st.SaveFullGame(gs); err != nil {
			t.Fatalf("SaveFullGame(%s): %v", id, err)
		}
	}

	games, err := st.ListGames(0, 0)
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("expected 2 games, got %d", len(games))
	}
	for _, g := range games {
		if _, err := st.GetGame(g.GameID); err != nil {
			t.Errorf("listed game %q has no retrievable state: %v", g.GameID, err)
		}
	}
}

func TestHasGame(t *testing.T) {
	st := openTestStore(t)

	if has, err := st.HasGame("nope"); err != nil || has {
		t.Errorf("HasGame on missing game: expected (false, nil), got (%v, %v)", has, err)
	}

	gs := gamestate.BGGameState{GameID: "exists", Placement: 1, StartTime: time.Now()}
	if err := st.SaveFullGame(gs); err != nil {
		t.Fatalf("SaveFullGame: %v", err)
	}
	if has, err := st.HasGame("exists"); err != nil || !has {
		t.Errorf("HasGame on saved game: expected (true, nil), got (%v, %v)", has, err)
	}
}

// A corrupt (non-JSON) game list value must cause SaveGame to fail the
// transaction, NOT silently replace the list with a single new element.
func TestCorruptGameListNotWiped(t *testing.T) {
	st := openTestStore(t)

	gs := gamestate.BGGameState{GameID: "pre-corrupt", Placement: 1, StartTime: time.Now()}
	if err := st.SaveFullGame(gs); err != nil {
		t.Fatalf("SaveFullGame: %v", err)
	}

	// Corrupt the list key directly.
	garbage := []byte("\x00not json at all")
	if err := st.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(prefixGameList), garbage)
	}); err != nil {
		t.Fatalf("corrupting list key: %v", err)
	}

	gs2 := gamestate.BGGameState{GameID: "post-corrupt", Placement: 2, StartTime: time.Now()}
	if err := st.SaveFullGame(gs2); err == nil {
		t.Error("SaveFullGame with corrupt list: expected error, got nil")
	}

	// The list key must be untouched (still the garbage bytes), not replaced
	// with a fresh one-element list.
	var got []byte
	if err := st.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixGameList))
		if err != nil {
			return err
		}
		got, err = item.ValueCopy(nil)
		return err
	}); err != nil {
		t.Fatalf("reading list key: %v", err)
	}
	if !bytes.Equal(got, garbage) {
		t.Errorf("list key was overwritten on corrupt read: got %q", got)
	}
}
