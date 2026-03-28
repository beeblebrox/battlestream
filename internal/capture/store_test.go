package capture

import (
	"database/sql"
	"image"
	"image/color"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	return img
}

func TestEnsureSchema(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", dir+"/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := ensureSchema(db); err != nil {
		t.Fatalf("first ensureSchema: %v", err)
	}

	// Running again should be idempotent.
	if err := ensureSchema(db); err != nil {
		t.Fatalf("second ensureSchema: %v", err)
	}

	var version int
	if err := db.QueryRow(`SELECT version FROM schema_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Errorf("schema version: got %d, want 1", version)
	}
}

func TestStoreInitAndSaveFrame(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, 90)

	if err := store.InitGame("test-game-001"); err != nil {
		t.Fatalf("InitGame: %v", err)
	}

	frame := Frame{
		Sequence: 0,
		Image:    testImage(64, 64),
		State: CaptureState{
			GameID:    "test-game-001",
			Timestamp: time.Now(),
			Turn:      3,
			Phase:     "RECRUIT",
			TavernTier: 2,
			Health:    30,
			Armor:     5,
			Gold:      7,
			Board: []MinionSnapshot{
				{CardID: "BGS_004", Name: "Wrath Weaver", Attack: 25, Health: 65, Tribes: "DEMON"},
			},
			BuffSources: []BuffSourceSnapshot{
				{Category: "Demons", Attack: 10, Health: 20},
			},
		},
		CaptureLatency: 45,
	}

	if err := store.SaveFrame(frame); err != nil {
		t.Fatalf("SaveFrame: %v", err)
	}

	if err := store.FinalizeGame(1); err != nil {
		t.Fatalf("FinalizeGame: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify DB contents.
	db, err := sql.Open("sqlite", dir+"/test-game-001/capture.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM frames WHERE game_id = 'test-game-001'`).Scan(&count)
	if count != 1 {
		t.Errorf("frame count: got %d, want 1", count)
	}

	var turn int
	var phase, boardJSON string
	db.QueryRow(`SELECT turn, phase, board_json FROM frames WHERE sequence = 0`).Scan(&turn, &phase, &boardJSON)
	if turn != 3 {
		t.Errorf("turn: got %d, want 3", turn)
	}
	if phase != "RECRUIT" {
		t.Errorf("phase: got %q, want RECRUIT", phase)
	}
	if boardJSON == "" || boardJSON == "null" {
		t.Error("board_json is empty")
	}
}

func TestStoreFinalizeUpdatesGame(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, 90)

	if err := store.InitGame("finalize-test"); err != nil {
		t.Fatal(err)
	}

	// Save two frames.
	for i := range 2 {
		store.SaveFrame(Frame{
			Sequence: i,
			Image:    testImage(32, 32),
			State: CaptureState{
				GameID:      "finalize-test",
				Timestamp:   time.Now(),
				Phase:       "RECRUIT",
				Board:       []MinionSnapshot{},
				BuffSources: []BuffSourceSnapshot{},
			},
		})
	}

	store.FinalizeGame(3)
	store.Close()

	db, _ := sql.Open("sqlite", dir+"/finalize-test/capture.db")
	defer db.Close()

	var placement, totalFrames int
	var endTime sql.NullString
	db.QueryRow(`SELECT placement, total_frames, end_time FROM games WHERE game_id = 'finalize-test'`).
		Scan(&placement, &totalFrames, &endTime)

	if placement != 3 {
		t.Errorf("placement: got %d, want 3", placement)
	}
	if totalFrames != 2 {
		t.Errorf("total_frames: got %d, want 2", totalFrames)
	}
	if !endTime.Valid {
		t.Error("end_time should be set after finalize")
	}
}
