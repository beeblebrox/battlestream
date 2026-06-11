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
// Results with Placement <= 0 (stale/incomplete games) are skipped; when no
// result has a valid placement, the zero AggregateStats is returned (in
// particular BestPlacement/WorstPlacement are 0, meaning "no data").
//
// Known limitation: solo (placements 1-8) and duos (placements 1-4) results
// are averaged together when both appear in the input. Wins/Losses use the
// correct per-mode threshold, but AvgPlacement/BestPlacement/WorstPlacement
// mix the two scales. The REST layer (`/v1/stats/aggregate?mode=`) filters
// results by mode before calling Compute; the store's GetAggregate (used by
// gRPC GetAggregate and the fileout summary) does not, so those consumers see
// the mixed average. Per-mode computation here would require an API change
// and is intentionally out of scope.
func Compute(results []GameResult) AggregateStats {
	var stats AggregateStats

	totalPlacement := 0
	counted := 0
	for _, r := range results {
		if r.Placement <= 0 {
			continue // skip stale/incomplete games
		}
		counted++
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
		if stats.BestPlacement == 0 || r.Placement < stats.BestPlacement {
			stats.BestPlacement = r.Placement
		}
		if r.Placement > stats.WorstPlacement {
			stats.WorstPlacement = r.Placement
		}
	}

	stats.GamesPlayed = counted
	if counted > 0 {
		stats.AvgPlacement = float64(totalPlacement) / float64(counted)
	}

	return stats
}
