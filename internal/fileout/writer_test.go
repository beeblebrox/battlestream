package fileout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/stats"
)

func newWriter(t *testing.T) *Writer {
	t.Helper()
	w, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return w
}

func TestWriterCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	_, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"current", "aggregate", "history"} {
		if _, err := os.Stat(filepath.Join(dir, sub)); err != nil {
			t.Errorf("subdirectory %q not created: %v", sub, err)
		}
	}
}

func TestWriteCurrentState(t *testing.T) {
	w := newWriter(t)

	state := gamestate.BGGameState{
		GameID:     "game-1",
		Phase:      gamestate.PhaseRecruit,
		Turn:       5,
		TavernTier: 3,
		Player: gamestate.PlayerState{
			Name:        "Fixates",
			Health:      30,
			Armor:       2,
			SpellPower:  1,
			TripleCount: 2,
		},
		Board: []gamestate.MinionState{
			{EntityID: 1, Name: "Murloc", Attack: 3, Health: 4, BuffAttack: 1},
		},
		Modifications: []gamestate.StatMod{
			{Turn: 3, Target: "ALL", Stat: "ATTACK", Delta: 1, Source: "Blood Gem"},
		},
		StartTime: time.Now(),
	}

	if err := w.WriteCurrentState(state); err != nil {
		t.Fatal(err)
	}

	// Verify game_state.json
	var gs GameStateFile
	readJSON(t, filepath.Join(w.baseDir, "current", "game_state.json"), &gs)
	if gs.GameID != "game-1" {
		t.Errorf("GameID: expected game-1, got %q", gs.GameID)
	}
	if gs.Phase != "RECRUIT" {
		t.Errorf("Phase: expected RECRUIT, got %q", gs.Phase)
	}
	if gs.Turn != 5 {
		t.Errorf("Turn: expected 5, got %d", gs.Turn)
	}

	// Verify player_stats.json
	var ps PlayerStatsFile
	readJSON(t, filepath.Join(w.baseDir, "current", "player_stats.json"), &ps)
	if ps.Name != "Fixates" {
		t.Errorf("Name: expected Fixates, got %q", ps.Name)
	}
	if ps.Health != 30 {
		t.Errorf("Health: expected 30, got %d", ps.Health)
	}

	// Verify board_state.json
	var bs BoardStateFile
	readJSON(t, filepath.Join(w.baseDir, "current", "board_state.json"), &bs)
	if len(bs.Board) != 1 {
		t.Fatalf("expected 1 minion in board, got %d", len(bs.Board))
	}
	if bs.Board[0].BuffAttack != 1 {
		t.Errorf("BuffAttack: expected 1, got %d", bs.Board[0].BuffAttack)
	}

	// Verify modifications.json
	var mf ModificationsFile
	readJSON(t, filepath.Join(w.baseDir, "current", "modifications.json"), &mf)
	if len(mf.Modifications) != 1 {
		t.Fatalf("expected 1 mod, got %d", len(mf.Modifications))
	}
	if mf.Modifications[0].Source != "Blood Gem" {
		t.Errorf("Source: expected Blood Gem, got %q", mf.Modifications[0].Source)
	}
}

func TestWriteAggregate(t *testing.T) {
	w := newWriter(t)

	agg := stats.AggregateStats{
		GamesPlayed:  10,
		Wins:         7,
		Losses:       3,
		AvgPlacement: 2.5,
	}
	if err := w.WriteAggregate(agg); err != nil {
		t.Fatal(err)
	}

	var sf SummaryFile
	readJSON(t, filepath.Join(w.baseDir, "aggregate", "summary.json"), &sf)
	if sf.GamesPlayed != 10 {
		t.Errorf("GamesPlayed: expected 10, got %d", sf.GamesPlayed)
	}
	if sf.Wins != 7 {
		t.Errorf("Wins: expected 7, got %d", sf.Wins)
	}
	if sf.AvgPlacement != 2.5 {
		t.Errorf("AvgPlacement: expected 2.5, got %f", sf.AvgPlacement)
	}
}

func TestWriteHistory(t *testing.T) {
	w := newWriter(t)

	now := time.Now()
	state := gamestate.BGGameState{
		GameID:    "game-42",
		Phase:     gamestate.PhaseGameOver,
		Placement: 1,
		StartTime: now,
	}
	if err := w.WriteHistory(state); err != nil {
		t.Fatal(err)
	}

	pattern := filepath.Join(w.baseDir, "history", "*.json")
	matches, _ := filepath.Glob(pattern)
	if len(matches) != 1 {
		t.Fatalf("expected 1 history file, got %d", len(matches))
	}

	var retrieved gamestate.BGGameState
	readJSON(t, matches[0], &retrieved)
	if retrieved.GameID != "game-42" {
		t.Errorf("GameID: expected game-42, got %q", retrieved.GameID)
	}
	if retrieved.Placement != 1 {
		t.Errorf("Placement: expected 1, got %d", retrieved.Placement)
	}
}

func TestAtomicWriteNoTmpLeftBehind(t *testing.T) {
	w := newWriter(t)
	state := gamestate.BGGameState{GameID: "g", StartTime: time.Now()}
	if err := w.WriteCurrentState(state); err != nil {
		t.Fatal(err)
	}

	// Walk and ensure no .tmp files exist
	err := filepath.WalkDir(w.baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".tmp" {
			t.Errorf("temp file left behind: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshaling %s: %v", path, err)
	}
}
