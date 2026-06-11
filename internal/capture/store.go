package capture

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"log/slog"
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

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return fmt.Errorf("set busy_timeout: %w", err)
	}

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
	filename := fmt.Sprintf("%06d.jpg", f.Sequence)
	relPath := filepath.Join("frames", filename)
	absPath := filepath.Join(s.framesDir, filename)

	// Encode in memory first: a failed encode must never disturb an existing
	// frame file, and the DB row needs the final byte size up front.
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, f.Image, &jpeg.Options{Quality: s.jpegQuality}); err != nil {
		return fmt.Errorf("encode jpeg: %w", err)
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

	// Stage the JPEG in a temp file, insert the DB row, then rename into
	// place. The frames table has UNIQUE(game_id, sequence): inserting
	// BEFORE the file lands at its final path guarantees a duplicate
	// sequence can never clobber a previously captured frame, and the
	// rename keeps the final path atomic (no partial JPEG ever visible).
	tmp, err := os.CreateTemp(s.framesDir, filename+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp frame file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp frame file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp frame file: %w", err)
	}

	_, err = s.db.Exec(`INSERT INTO frames (
		game_id, sequence, timestamp, elapsed_ms, file_path, file_size_bytes,
		capture_latency_ms, turn, phase, tavern_tier, health, armor, gold,
		placement, is_duos, partner_health, partner_tier,
		board_json, buff_sources_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.gameID, f.Sequence, f.State.Timestamp.Format(time.RFC3339Nano),
		elapsed, relPath, int64(buf.Len()), f.CaptureLatency,
		f.State.Turn, f.State.Phase, f.State.TavernTier,
		f.State.Health, f.State.Armor, f.State.Gold,
		f.State.Placement, f.State.IsDuos,
		f.State.PartnerHealth, f.State.PartnerTier,
		string(boardJSON), string(buffsJSON),
	)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("insert frame: %w", err)
	}

	if err := os.Rename(tmpPath, absPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename frame file into place: %w", err)
	}

	// Update is_duos on first frame when we have actual game state.
	if s.frameCount == 0 && f.State.IsDuos {
		if _, err := s.db.Exec(`UPDATE games SET is_duos = 1 WHERE game_id = ?`, s.gameID); err != nil {
			slog.Error("failed to update is_duos flag", "err", err, "game_id", s.gameID)
		}
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
		err := s.db.Close()
		s.db = nil
		return err
	}
	return nil
}
