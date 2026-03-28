package capture

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// sqliteStore implements FrameStore using SQLite + filesystem.
type sqliteStore struct {
	dataDir     string
	jpegQuality int
	db          *sql.DB
	gameID      string
	framesDir   string
	startTime   time.Time
	frameCount  int
}

// NewStore creates a FrameStore backed by SQLite + filesystem.
// dataDir is the root data directory (e.g. ~/.local/share/bscapture).
func NewStore(dataDir string, jpegQuality int) FrameStore {
	return &sqliteStore{
		dataDir:     dataDir,
		jpegQuality: jpegQuality,
	}
}

func (s *sqliteStore) InitGame(gameID string) error {
	if s.db != nil {
		s.db.Close()
	}
	s.gameID = gameID
	s.frameCount = 0
	s.startTime = time.Now()

	gameDir := filepath.Join(s.dataDir, gameID)
	s.framesDir = filepath.Join(gameDir, "frames")
	if err := os.MkdirAll(s.framesDir, 0o755); err != nil {
		return fmt.Errorf("create frames dir: %w", err)
	}

	dbPath := filepath.Join(gameDir, "capture.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	s.db = db

	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=5000")

	if err := ensureSchema(db); err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}

	_, err = db.Exec(
		`INSERT OR IGNORE INTO games (game_id, start_time) VALUES (?, ?)`,
		gameID, s.startTime.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert game: %w", err)
	}
	return nil
}

func (s *sqliteStore) SaveFrame(f Frame) error {
	if s.db == nil {
		return fmt.Errorf("store not initialized: call InitGame first")
	}
	// Write JPEG to disk.
	filename := fmt.Sprintf("%06d.jpg", f.Sequence)
	relPath := filepath.Join("frames", filename)
	absPath := filepath.Join(s.dataDir, s.gameID, relPath)

	file, err := os.Create(absPath)
	if err != nil {
		return fmt.Errorf("create frame file: %w", err)
	}
	if err := jpeg.Encode(file, f.Image, &jpeg.Options{Quality: s.jpegQuality}); err != nil {
		file.Close()
		return fmt.Errorf("encode jpeg: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close frame file: %w", err)
	}

	fi, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("stat frame: %w", err)
	}

	boardJSON, err := json.Marshal(f.State.Board)
	if err != nil {
		return fmt.Errorf("marshal board: %w", err)
	}
	buffsJSON, err := json.Marshal(f.State.BuffSources)
	if err != nil {
		return fmt.Errorf("marshal buffs: %w", err)
	}

	elapsed := f.State.Timestamp.Sub(s.startTime).Milliseconds()

	_, err = s.db.Exec(`INSERT INTO frames (
		game_id, sequence, timestamp, elapsed_ms, file_path, file_size_bytes,
		capture_latency_ms, turn, phase, tavern_tier, health, armor, gold,
		placement, is_duos, partner_health, partner_tier,
		board_json, buff_sources_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.gameID, f.Sequence, f.State.Timestamp.Format(time.RFC3339Nano),
		elapsed, relPath, fi.Size(), f.CaptureLatency,
		f.State.Turn, f.State.Phase, f.State.TavernTier,
		f.State.Health, f.State.Armor, f.State.Gold,
		f.State.Placement, f.State.IsDuos,
		f.State.PartnerHealth, f.State.PartnerTier,
		string(boardJSON), string(buffsJSON),
	)
	if err != nil {
		return fmt.Errorf("insert frame: %w", err)
	}

	s.frameCount++
	return nil
}

func (s *sqliteStore) FinalizeGame(placement int) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(
		`UPDATE games SET end_time = ?, placement = ?, total_frames = ? WHERE game_id = ?`,
		time.Now().Format(time.RFC3339), placement, s.frameCount, s.gameID,
	)
	return err
}

func (s *sqliteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
