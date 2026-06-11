package grpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	bspb "battlestream.fixates.io/internal/api/grpc/gen/battlestream/v1"
	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st := openTestStore(t)
	return New(nil, st, nil), st
}

// saveCompleteGames saves n games with non-zero placements.
func saveCompleteGames(t *testing.T, st *store.Store, n int) {
	t.Helper()
	base := time.Now().Unix()
	for i := 0; i < n; i++ {
		meta := store.GameMeta{
			GameID:    fmt.Sprintf("game-%03d", i),
			StartTime: base + int64(i*100),
			EndTime:   base + int64(i*100+60),
			Placement: i%8 + 1,
		}
		if err := st.SaveGame(meta); err != nil {
			t.Fatalf("SaveGame(%s): %v", meta.GameID, err)
		}
	}
}

// TestListGames_TotalIsFullCount is a regression test for audit H8 bug 1:
// Total must be the count of all matching games, not the page size.
func TestListGames_TotalIsFullCount(t *testing.T) {
	srv, st := newTestServer(t)
	const total = 10
	saveCompleteGames(t, st, total)

	resp, err := srv.ListGames(context.Background(), &bspb.ListGamesRequest{Limit: 3})
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if len(resp.Games) != 3 {
		t.Errorf("len(Games) = %d, want 3 (page size)", len(resp.Games))
	}
	if resp.Total != total {
		t.Errorf("Total = %d, want %d (full count, not page size)", resp.Total, total)
	}

	// Offset into the middle: page shrinks, total must not change.
	resp2, err := srv.ListGames(context.Background(), &bspb.ListGamesRequest{Limit: 4, Offset: 8})
	if err != nil {
		t.Fatalf("ListGames offset: %v", err)
	}
	if len(resp2.Games) != 2 {
		t.Errorf("len(Games) at offset 8 = %d, want 2", len(resp2.Games))
	}
	if resp2.Total != total {
		t.Errorf("Total at offset 8 = %d, want %d", resp2.Total, total)
	}
}

// TestListGames_ExcludesIncompleteGames is a regression test for audit H8 bug 2:
// placement==0 (stale/incomplete) games must be filtered out like the REST API does.
func TestListGames_ExcludesIncompleteGames(t *testing.T) {
	srv, st := newTestServer(t)
	saveCompleteGames(t, st, 3)
	stale := store.GameMeta{
		GameID:    "stale-game",
		StartTime: time.Now().Unix(),
		Placement: 0,
	}
	if err := st.SaveGame(stale); err != nil {
		t.Fatalf("SaveGame(stale): %v", err)
	}

	resp, err := srv.ListGames(context.Background(), &bspb.ListGamesRequest{})
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	for _, g := range resp.Games {
		if g.GameId == "stale-game" || g.Placement == 0 {
			t.Errorf("ListGames returned incomplete game %q (placement=%d)", g.GameId, g.Placement)
		}
	}
	if len(resp.Games) != 3 {
		t.Errorf("len(Games) = %d, want 3 (stale game excluded)", len(resp.Games))
	}
	if resp.Total != 3 {
		t.Errorf("Total = %d, want 3 (stale game excluded from total)", resp.Total)
	}
}

// TestGetGame_NotFound: a missing game must map to codes.NotFound.
func TestGetGame_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	_, err := srv.GetGame(context.Background(), &bspb.GetGameRequest{GameId: "does-not-exist"})
	if err == nil {
		t.Fatal("GetGame: expected error for missing game, got nil")
	}
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("GetGame missing game: code = %v, want %v", got, codes.NotFound)
	}
}

// TestGetGame_StoreErrorIsInternal is a regression test for audit H8 bug 3:
// a store failure (here: closed DB) must surface as Internal, not NotFound.
func TestGetGame_StoreErrorIsInternal(t *testing.T) {
	srv, st := newTestServer(t)
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err := srv.GetGame(context.Background(), &bspb.GetGameRequest{GameId: "any-game"})
	if err == nil {
		t.Fatal("GetGame: expected error on closed store, got nil")
	}
	if got := status.Code(err); got != codes.Internal {
		t.Errorf("GetGame store failure: code = %v, want %v", got, codes.Internal)
	}
}

// TestGetGame_FoundRoundTrip: a saved game is retrievable (sanity check that
// the not-found/internal split does not break the happy path).
func TestGetGame_FoundRoundTrip(t *testing.T) {
	srv, st := newTestServer(t)
	gs := gamestate.BGGameState{
		GameID:    "full-game-1",
		Placement: 2,
		StartTime: time.Now(),
	}
	if err := st.SaveFullGame(gs); err != nil {
		t.Fatalf("SaveFullGame: %v", err)
	}

	resp, err := srv.GetGame(context.Background(), &bspb.GetGameRequest{GameId: "full-game-1"})
	if err != nil {
		t.Fatalf("GetGame: %v", err)
	}
	if resp.GameId != "full-game-1" || resp.Placement != 2 {
		t.Errorf("GetGame = (id=%q, placement=%d), want (full-game-1, 2)", resp.GameId, resp.Placement)
	}
}
