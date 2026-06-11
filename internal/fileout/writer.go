// Package fileout writes JSON stat files for overlay consumption.
package fileout

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/stats"
)

// Writer writes JSON stat files atomically.
type Writer struct {
	baseDir string
}

// New creates a Writer that writes to baseDir.
func New(baseDir string) (*Writer, error) {
	for _, sub := range []string{"current", "aggregate", "history"} {
		if err := os.MkdirAll(filepath.Join(baseDir, sub), 0755); err != nil {
			return nil, fmt.Errorf("creating output dir %s: %w", sub, err)
		}
	}
	return &Writer{baseDir: baseDir}, nil
}

// GameStateFile is the JSON schema for current/game_state.json.
type GameStateFile struct {
	GameID     string `json:"game_id"`
	Phase      string `json:"phase"`
	Turn       int    `json:"turn"`
	TavernTier int    `json:"tavern_tier"`
	UpdatedAt  string `json:"updated_at"`
}

// PlayerStatsFile is the JSON schema for current/player_stats.json.
type PlayerStatsFile struct {
	Name        string `json:"name"`
	HeroCardID  string `json:"hero_card_id"`
	Health      int    `json:"health"`
	Armor       int    `json:"armor"`
	SpellPower  int    `json:"spell_power"`
	TripleCount int    `json:"triple_count"`
	WinStreak   int    `json:"win_streak"`
	Placement   int    `json:"placement,omitempty"`
	UpdatedAt   string `json:"updated_at"`
}

// BoardStateFile is the JSON schema for current/board_state.json.
type BoardStateFile struct {
	Board    []gamestate.MinionState `json:"board"`
	UpdatedAt string                 `json:"updated_at"`
}

// ModificationsFile is the JSON schema for current/modifications.json.
type ModificationsFile struct {
	Modifications []gamestate.StatMod `json:"modifications"`
	UpdatedAt     string              `json:"updated_at"`
}

// BuffSourcesFile is the JSON schema for current/buff_sources.json.
type BuffSourcesFile struct {
	BuffSources []gamestate.BuffSource `json:"buff_sources"`
	UpdatedAt   string                 `json:"updated_at"`
}

// SummaryFile is the JSON schema for aggregate/summary.json.
type SummaryFile struct {
	GamesPlayed    int     `json:"games_played"`
	Wins           int     `json:"wins"`
	Losses         int     `json:"losses"`
	AvgPlacement   float64 `json:"avg_placement"`
	UpdatedAt      string  `json:"updated_at"`
}

// WriteCurrentState writes all current/ JSON files from a game state snapshot.
func (w *Writer) WriteCurrentState(s gamestate.BGGameState) error {
	now := time.Now().Format(time.RFC3339)

	if err := w.writeJSON(filepath.Join("current", "game_state.json"), GameStateFile{
		GameID:     s.GameID,
		Phase:      string(s.Phase),
		Turn:       s.Turn,
		TavernTier: s.TavernTier,
		UpdatedAt:  now,
	}); err != nil {
		return err
	}

	if err := w.writeJSON(filepath.Join("current", "player_stats.json"), PlayerStatsFile{
		Name:        s.Player.Name,
		HeroCardID:  s.Player.HeroCardID,
		Health:      s.Player.Health,
		Armor:       s.Player.Armor,
		SpellPower:  s.Player.SpellPower,
		TripleCount: s.Player.TripleCount,
		WinStreak:   s.Player.WinStreak,
		Placement:   s.Placement,
		UpdatedAt:   now,
	}); err != nil {
		return err
	}

	if err := w.writeJSON(filepath.Join("current", "board_state.json"), BoardStateFile{
		Board:     s.Board,
		UpdatedAt: now,
	}); err != nil {
		return err
	}

	if err := w.writeJSON(filepath.Join("current", "modifications.json"), ModificationsFile{
		Modifications: s.Modifications,
		UpdatedAt:     now,
	}); err != nil {
		return err
	}

	if err := w.writeJSON(filepath.Join("current", "buff_sources.json"), BuffSourcesFile{
		BuffSources: s.BuffSources,
		UpdatedAt:   now,
	}); err != nil {
		return err
	}

	// Write partner files for Duos games. For non-duos states (or duos states
	// without a resolved partner yet) remove any stale partner_stats.json left
	// over from a previous duos game, so overlays don't keep showing it.
	if s.IsDuos && s.Partner != nil {
		if err := w.writeJSON(filepath.Join("current", "partner_stats.json"), PlayerStatsFile{
			Name:        s.Partner.Name,
			HeroCardID:  s.Partner.HeroCardID,
			Health:      s.Partner.Health,
			Armor:       s.Partner.Armor,
			SpellPower:  s.Partner.SpellPower,
			TripleCount: s.Partner.TripleCount,
			WinStreak:   s.Partner.WinStreak,
			UpdatedAt:   now,
		}); err != nil {
			return err
		}
	} else {
		if err := w.removeFile(filepath.Join("current", "partner_stats.json")); err != nil {
			return err
		}
	}

	return nil
}

// removeFile deletes baseDir/relPath if it exists.
func (w *Writer) removeFile(relPath string) error {
	err := os.Remove(filepath.Join(w.baseDir, relPath))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", relPath, err)
	}
	return nil
}

// WriteAggregate writes the aggregate/summary.json file.
func (w *Writer) WriteAggregate(agg stats.AggregateStats) error {
	return w.writeJSON(filepath.Join("aggregate", "summary.json"), SummaryFile{
		GamesPlayed:  agg.GamesPlayed,
		Wins:         agg.Wins,
		Losses:       agg.Losses,
		AvgPlacement: agg.AvgPlacement,
		UpdatedAt:    time.Now().Format(time.RFC3339),
	})
}

// WriteHistory writes a full game state snapshot to history/{date}_{gameID}.json.
func (w *Writer) WriteHistory(s gamestate.BGGameState) error {
	date := s.StartTime.Format("2006-01-02")
	name := fmt.Sprintf("%s_%s.json", date, s.GameID)
	return w.writeJSON(filepath.Join("history", name), s)
}

// writeJSON atomically and durably writes v as JSON to baseDir/relPath.
//
// The temp file is created with a unique name (so a stray second daemon
// writing the same file cannot interleave with our temp file), fsync'd before
// the rename (so a crash/power loss cannot leave a truncated target behind),
// and the containing directory is fsync'd after the rename (so the rename
// itself is durable).
func (w *Writer) writeJSON(relPath string, v interface{}) error {
	full := filepath.Join(w.baseDir, relPath)
	dir := filepath.Dir(full)

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", relPath, err)
	}

	f, err := os.CreateTemp(dir, filepath.Base(full)+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp for %s: %w", relPath, err)
	}
	tmp := f.Name()
	cleanup := func(err error) error {
		f.Close()
		os.Remove(tmp)
		return err
	}

	if _, err := f.Write(data); err != nil {
		return cleanup(fmt.Errorf("writing temp %s: %w", relPath, err))
	}
	// CreateTemp uses 0600; these files are consumed by external overlay
	// tools, keep them world-readable like the previous implementation.
	if err := f.Chmod(0644); err != nil {
		return cleanup(fmt.Errorf("chmod temp %s: %w", relPath, err))
	}
	if err := f.Sync(); err != nil {
		return cleanup(fmt.Errorf("syncing temp %s: %w", relPath, err))
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("closing temp %s: %w", relPath, err)
	}

	if err := os.Rename(tmp, full); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming %s: %w", relPath, err)
	}

	// Best-effort directory sync to make the rename durable. Some platforms
	// (notably Windows) do not support fsync on directories; ignore errors.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
