package stats

import (
	"testing"
	"time"
)

func TestComputeEmpty(t *testing.T) {
	s := Compute(nil)
	if s.GamesPlayed != 0 || s.Wins != 0 || s.Losses != 0 || s.AvgPlacement != 0 {
		t.Errorf("expected zero stats for empty input, got %+v", s)
	}
}

func TestComputeWinLoss(t *testing.T) {
	results := []GameResult{
		{Placement: 1},
		{Placement: 2},
		{Placement: 4},
		{Placement: 5},
		{Placement: 8},
	}
	s := Compute(results)

	if s.GamesPlayed != 5 {
		t.Errorf("GamesPlayed: expected 5, got %d", s.GamesPlayed)
	}
	// placements 1,2,4 are wins (<=4); 5,8 are losses
	if s.Wins != 3 {
		t.Errorf("Wins: expected 3, got %d", s.Wins)
	}
	if s.Losses != 2 {
		t.Errorf("Losses: expected 2, got %d", s.Losses)
	}
}

func TestComputeAvgPlacement(t *testing.T) {
	results := []GameResult{
		{Placement: 1},
		{Placement: 3},
		{Placement: 5},
	}
	s := Compute(results)
	expected := (1 + 3 + 5) / 3.0
	if s.AvgPlacement != expected {
		t.Errorf("AvgPlacement: expected %.4f, got %.4f", expected, s.AvgPlacement)
	}
}

func TestComputeBestWorstPlacement(t *testing.T) {
	results := []GameResult{
		{Placement: 3},
		{Placement: 1},
		{Placement: 6},
	}
	s := Compute(results)
	if s.BestPlacement != 1 {
		t.Errorf("BestPlacement: expected 1, got %d", s.BestPlacement)
	}
	if s.WorstPlacement != 6 {
		t.Errorf("WorstPlacement: expected 6, got %d", s.WorstPlacement)
	}
}

func TestComputeSingleGame(t *testing.T) {
	results := []GameResult{{Placement: 1, EndTime: time.Now()}}
	s := Compute(results)
	if s.GamesPlayed != 1 {
		t.Errorf("GamesPlayed: expected 1, got %d", s.GamesPlayed)
	}
	if s.Wins != 1 || s.Losses != 0 {
		t.Errorf("expected 1 win 0 losses, got W=%d L=%d", s.Wins, s.Losses)
	}
	if s.AvgPlacement != 1.0 {
		t.Errorf("AvgPlacement: expected 1.0, got %f", s.AvgPlacement)
	}
}

func TestComputeAllLosses(t *testing.T) {
	results := []GameResult{
		{Placement: 5},
		{Placement: 6},
		{Placement: 7},
		{Placement: 8},
	}
	s := Compute(results)
	if s.Wins != 0 {
		t.Errorf("expected 0 wins, got %d", s.Wins)
	}
	if s.Losses != 4 {
		t.Errorf("expected 4 losses, got %d", s.Losses)
	}
}
