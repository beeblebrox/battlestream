// Package stats computes aggregate statistics from game history.
package stats

import (
	"time"
)

// AggregateStats holds computed aggregate data across all games.
type AggregateStats struct {
	GamesPlayed   int     `json:"games_played"`
	Wins          int     `json:"wins"`
	Losses        int     `json:"losses"`
	AvgPlacement  float64 `json:"avg_placement"`
	BestPlacement int     `json:"best_placement"`
	WorstPlacement int    `json:"worst_placement"`
}

// GameResult is the minimal info needed to update aggregates.
type GameResult struct {
	Placement int
	EndTime   time.Time
	IsDuos    bool
}

// Compute calculates AggregateStats from a slice of game results.
func Compute(results []GameResult) AggregateStats {
	if len(results) == 0 {
		return AggregateStats{}
	}

	stats := AggregateStats{
		GamesPlayed:    len(results),
		BestPlacement:  8,
		WorstPlacement: 1,
	}

	totalPlacement := 0
	for _, r := range results {
		totalPlacement += r.Placement
		// Duos: 1-4 placements, top 2 is a win.
		// Solo: 1-8 placements, top 4 is a win.
		winThreshold := 4
		if r.IsDuos {
			winThreshold = 2
		}
		if r.Placement <= winThreshold {
			stats.Wins++
		} else {
			stats.Losses++
		}
		if r.Placement < stats.BestPlacement {
			stats.BestPlacement = r.Placement
		}
		if r.Placement > stats.WorstPlacement {
			stats.WorstPlacement = r.Placement
		}
	}

	if len(results) > 0 {
		stats.AvgPlacement = float64(totalPlacement) / float64(len(results))
	}

	return stats
}
