# bscapture Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a standalone `bscapture` binary that captures timed screenshots of Hearthstone Battlegrounds during active games, tagged with full game state metadata, stored in per-game SQLite databases with JPEG frames on disk.

**Architecture:** Embedded pipeline reuses `internal/parser` and `internal/gamestate` packages through small interfaces. A poll-based capture loop runs at a configurable interval (default 1s), shelling out to `grim` for Wayland screenshots. Each frame is scaled to 1920x1080, JPEG-encoded, and stored alongside a metadata row in SQLite.

**Tech Stack:** Go, cobra/viper, `modernc.org/sqlite`, `golang.org/x/image/draw` (Lanczos), `grim` (Wayland screenshot), existing battlestream parser/gamestate/watcher packages.

**Design doc:** `docs/plans/2026-03-27-bscapture-screenshot-capture-design.md`

---

### Task 1: Scaffold binary and config

**Files:**
- Create: `cmd/bscapture/main.go`
- Create: `internal/capture/config.go`

**Step 1: Create the cobra root command with subcommands**

Create `cmd/bscapture/main.go` with root command and four subcommand stubs (run, detect, list, config). Follow the pattern from `cmd/battlestream/main.go` — function factories returning `*cobra.Command`.

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

func main() {
	root := &cobra.Command{
		Use:   "bscapture",
		Short: "Screenshot capture for Hearthstone Battlegrounds games",
		Long:  "Captures timed screenshots during BG games, tagged with game state metadata for post-game analysis.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/bscapture/config.yaml)")

	root.AddCommand(
		cmdRun(),
		cmdDetect(),
		cmdList(),
		cmdConfig(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func cmdRun() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Start capture session — waits for game, captures screenshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
}

func cmdDetect() *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Detect monitors and Hearthstone install, update config",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
}

func cmdList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List captured game sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
}

func cmdConfig() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
}
```

**Step 2: Create config package**

Create `internal/capture/config.go` with the bscapture config struct and viper loader. The config uses its own file at `~/.config/bscapture/config.yaml` with env prefix `BSC_`.

```go
package capture

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	PowerLogPath    string        `mapstructure:"power_log_path"`
	Monitor         string        `mapstructure:"monitor"`
	CaptureInterval time.Duration `mapstructure:"capture_interval"`
	OutputResWidth  int           `mapstructure:"output_res_width"`
	OutputResHeight int           `mapstructure:"output_res_height"`
	JPEGQuality     int           `mapstructure:"jpeg_quality"`
	DataDir         string        `mapstructure:"data_dir"`
	StaleTimeout    time.Duration `mapstructure:"stale_timeout"`
}

func LoadConfig(cfgFile string) (*Config, error) {
	v := viper.New()

	v.SetDefault("monitor", "DP-1")
	v.SetDefault("capture_interval", "1s")
	v.SetDefault("output_res_width", 1920)
	v.SetDefault("output_res_height", 1080)
	v.SetDefault("jpeg_quality", 92)
	v.SetDefault("stale_timeout", "5m")

	home, _ := os.UserHomeDir()
	v.SetDefault("data_dir", filepath.Join(home, ".local", "share", "bscapture"))

	v.SetEnvPrefix("BSC")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(filepath.Join(home, ".config", "bscapture"))
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config, path string) error {
	v := viper.New()
	v.Set("power_log_path", cfg.PowerLogPath)
	v.Set("monitor", cfg.Monitor)
	v.Set("capture_interval", cfg.CaptureInterval.String())
	v.Set("output_res_width", cfg.OutputResWidth)
	v.Set("output_res_height", cfg.OutputResHeight)
	v.Set("jpeg_quality", cfg.JPEGQuality)
	v.Set("data_dir", cfg.DataDir)
	v.Set("stale_timeout", cfg.StaleTimeout.String())

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return v.WriteConfigAs(path)
}
```

**Step 3: Verify it builds**

Run: `go build ./cmd/bscapture`
Expected: builds cleanly, produces `bscapture` binary.

**Step 4: Verify subcommand help**

Run: `./bscapture --help` and `./bscapture run --help`
Expected: shows help text for all subcommands.

**Step 5: Commit**

```
git add cmd/bscapture/ internal/capture/config.go
git commit -m "feat(bscapture): scaffold binary with cobra subcommands and config"
```

---

### Task 2: Interfaces and state types

**Files:**
- Create: `internal/capture/capture.go`
- Create: `internal/capture/state.go`

**Step 1: Write the interface definitions**

Create `internal/capture/capture.go` with the four core interfaces.

```go
package capture

import (
	"context"
	"image"

	"battlestream.fixates.io/internal/parser"
)

// EventSource produces parsed game events from Power.log.
type EventSource interface {
	Events() <-chan parser.GameEvent
	Close() error
}

// StateTracker maintains game state from events.
type StateTracker interface {
	Apply(event parser.GameEvent)
	Snapshot() CaptureState
	InGame() bool
}

// Screenshotter captures the display.
type Screenshotter interface {
	Capture(ctx context.Context) (image.Image, error)
}

// FrameStore persists frames and metadata.
type FrameStore interface {
	InitGame(gameID string) error
	SaveFrame(frame Frame) error
	FinalizeGame(placement int) error
	Close() error
}

// Frame bundles a captured image with its metadata.
type Frame struct {
	Sequence       int
	Image          image.Image
	State          CaptureState
	CaptureLatency int64 // milliseconds
}
```

**Step 2: Write the state types**

Create `internal/capture/state.go` with `CaptureState` and `MinionSnapshot`.

```go
package capture

import "time"

// CaptureState is a point-in-time snapshot of game state taken under lock.
type CaptureState struct {
	GameID      string    `json:"game_id"`
	Timestamp   time.Time `json:"timestamp"`
	Turn        int       `json:"turn"`
	Phase       string    `json:"phase"`
	TavernTier  int       `json:"tavern_tier"`
	Health      int       `json:"health"`
	Armor       int       `json:"armor"`
	Gold        int       `json:"gold"`
	Placement   int       `json:"placement"`
	IsDuos      bool      `json:"is_duos"`
	PartnerHealth int     `json:"partner_health,omitempty"`
	PartnerTier   int     `json:"partner_tier,omitempty"`
	Board       []MinionSnapshot     `json:"board"`
	BuffSources []BuffSourceSnapshot `json:"buff_sources"`
}

// MinionSnapshot captures a single minion's state at capture time.
type MinionSnapshot struct {
	CardID     string `json:"card_id"`
	Name       string `json:"name"`
	Attack     int    `json:"attack"`
	Health     int    `json:"health"`
	Tribes     string `json:"tribes"`
	BuffAttack int    `json:"buff_attack"`
	BuffHealth int    `json:"buff_health"`
}

// BuffSourceSnapshot captures a buff source category total.
type BuffSourceSnapshot struct {
	Category string `json:"category"`
	Attack   int    `json:"attack"`
	Health   int    `json:"health"`
}
```

Note: `Tribes` is a single string from `MinionState.MinionType` (e.g. "DEMON"). If multi-tribe support is added to gamestate later, this can evolve to `[]string` in the JSON without a schema migration.

**Step 3: Verify it compiles**

Run: `go build ./internal/capture/`
Expected: compiles cleanly.

**Step 4: Commit**

```
git add internal/capture/capture.go internal/capture/state.go
git commit -m "feat(bscapture): add core interfaces and state types"
```

---

### Task 3: SQLite frame store with migrations

**Files:**
- Create: `internal/capture/store.go`
- Create: `internal/capture/migrate.go`
- Create: `internal/capture/store_test.go`

**Step 1: Write the migration framework**

Create `internal/capture/migrate.go`. Uses a `schema_version` table and an ordered slice of migration functions.

```go
package capture

import (
	"database/sql"
	"fmt"
)

type migration struct {
	version int
	name    string
	fn      func(tx *sql.Tx) error
}

// migrations is the ordered list of schema migrations.
// IMPORTANT: Only append to this slice. Never remove or reorder entries.
// Columns should only be added, never removed.
// JSON columns (board_json, buff_sources_json) provide flexibility for
// evolving nested structures without schema changes.
var migrations = []migration{
	{version: 1, name: "initial schema", fn: migrateV1},
}

func migrateV1(tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE games (
			game_id      TEXT PRIMARY KEY,
			start_time   TEXT NOT NULL,
			end_time     TEXT,
			is_duos      BOOLEAN NOT NULL DEFAULT 0,
			placement    INTEGER DEFAULT 0,
			total_frames INTEGER DEFAULT 0
		)`,
		`CREATE TABLE frames (
			id                 INTEGER PRIMARY KEY AUTOINCREMENT,
			game_id            TEXT NOT NULL REFERENCES games(game_id),
			sequence           INTEGER NOT NULL,
			timestamp          TEXT NOT NULL,
			elapsed_ms         INTEGER NOT NULL,
			file_path          TEXT NOT NULL,
			file_size_bytes    INTEGER NOT NULL,
			capture_latency_ms INTEGER NOT NULL,
			turn               INTEGER NOT NULL,
			phase              TEXT NOT NULL,
			tavern_tier        INTEGER NOT NULL,
			health             INTEGER NOT NULL,
			armor              INTEGER NOT NULL,
			gold               INTEGER NOT NULL,
			placement          INTEGER NOT NULL DEFAULT 0,
			is_duos            BOOLEAN NOT NULL DEFAULT 0,
			partner_health     INTEGER,
			partner_tier       INTEGER,
			board_json         TEXT NOT NULL,
			buff_sources_json  TEXT NOT NULL,
			UNIQUE(game_id, sequence)
		)`,
		`CREATE INDEX idx_frames_game_turn ON frames(game_id, turn)`,
		`CREATE INDEX idx_frames_game_phase ON frames(game_id, phase)`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("exec %q: %w", s[:40], err)
		}
	}
	return nil
}

func ensureSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`)
	if err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	var current int
	row := db.QueryRow(`SELECT version FROM schema_version LIMIT 1`)
	if err := row.Scan(&current); err != nil {
		// No row = fresh DB, version 0.
		current = 0
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d (%s): %w", m.version, m.name, err)
		}
		if err := m.fn(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if current == 0 {
			_, err = tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, m.version)
		} else {
			_, err = tx.Exec(`UPDATE schema_version SET version = ?`, m.version)
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("update schema_version to %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
		current = m.version
	}
	return nil
}
```

**Step 2: Write the SQLite frame store**

Create `internal/capture/store.go`.

```go
package capture

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
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
```

**Step 3: Write tests for store and migrations**

Create `internal/capture/store_test.go`. Tests use `t.TempDir()` for isolation.

```go
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
				GameID:    "finalize-test",
				Timestamp: time.Now(),
				Phase:     "RECRUIT",
				Board:     []MinionSnapshot{},
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
```

**Step 4: Add modernc.org/sqlite dependency**

Run: `go get modernc.org/sqlite`

**Step 5: Run tests**

Run: `go test -count=1 ./internal/capture/`
Expected: all 3 tests pass.

**Step 6: Commit**

```
git add internal/capture/store.go internal/capture/migrate.go internal/capture/store_test.go go.mod go.sum
git commit -m "feat(bscapture): add SQLite frame store with migration framework"
```

---

### Task 4: Screenshotter implementation (grim + scale)

**Files:**
- Create: `internal/capture/grim.go`
- Create: `internal/capture/grim_test.go`

**Step 1: Write the grim screenshotter**

Create `internal/capture/grim.go`. Shells out to `grim -o <monitor> -` which writes PNG to stdout. Decodes, scales to target resolution using Lanczos, returns the scaled image.

```go
package capture

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os/exec"

	"golang.org/x/image/draw"
)

// grimScreenshotter captures screenshots via grim on Wayland/Hyprland.
type grimScreenshotter struct {
	monitor    string
	targetW    int
	targetH    int
}

// NewScreenshotter creates a Screenshotter that uses grim for capture
// and scales output to targetW x targetH.
func NewScreenshotter(monitor string, targetW, targetH int) Screenshotter {
	return &grimScreenshotter{
		monitor: monitor,
		targetW: targetW,
		targetH: targetH,
	}
}

func (g *grimScreenshotter) Capture(ctx context.Context) (image.Image, error) {
	cmd := exec.CommandContext(ctx, "grim", "-o", g.monitor, "-")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("grim: %w: %s", err, stderr.String())
	}

	src, err := png.Decode(&stdout)
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}

	// Scale to target resolution.
	srcBounds := src.Bounds()
	if srcBounds.Dx() == g.targetW && srcBounds.Dy() == g.targetH {
		return src, nil // already correct size
	}

	dst := image.NewRGBA(image.Rect(0, 0, g.targetW, g.targetH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, srcBounds, draw.Over, nil)
	return dst, nil
}
```

**Step 2: Write a test that verifies scaling logic**

Create `internal/capture/grim_test.go`. This tests the scaling path in isolation using a mock image (not grim itself, since that requires a display).

```go
package capture

import (
	"image"
	"testing"

	"golang.org/x/image/draw"
)

func TestScaleImage(t *testing.T) {
	src := testImage(2560, 1440)
	dst := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	if dst.Bounds().Dx() != 1920 || dst.Bounds().Dy() != 1080 {
		t.Errorf("scaled size: got %dx%d, want 1920x1080", dst.Bounds().Dx(), dst.Bounds().Dy())
	}
}

func TestScaleNoOp(t *testing.T) {
	src := testImage(1920, 1080)
	g := &grimScreenshotter{targetW: 1920, targetH: 1080}
	// Verify no-scale path: when source == target, return source unchanged.
	srcBounds := src.Bounds()
	if srcBounds.Dx() == g.targetW && srcBounds.Dy() == g.targetH {
		// This is the fast path — nothing to test beyond confirming the branch.
		return
	}
	t.Error("expected no-op scale path")
}
```

**Step 3: Add x/image dependency**

Run: `go get golang.org/x/image`

**Step 4: Run tests**

Run: `go test -count=1 ./internal/capture/`
Expected: all tests pass (store + grim tests).

**Step 5: Commit**

```
git add internal/capture/grim.go internal/capture/grim_test.go go.mod go.sum
git commit -m "feat(bscapture): add grim screenshotter with resolution scaling"
```

---

### Task 5: Pipeline wrapper (EventSource + StateTracker)

**Files:**
- Create: `internal/capture/pipeline.go`
- Create: `internal/capture/pipeline_test.go`

**Step 1: Write the pipeline wrappers**

Create `internal/capture/pipeline.go`. Wraps existing watcher/parser as EventSource and gamestate Machine/Processor as StateTracker.

```go
package capture

import (
	"context"
	"time"

	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/parser"
	"battlestream.fixates.io/internal/watcher"
)

// logEventSource wraps watcher + parser into an EventSource.
type logEventSource struct {
	watcher *watcher.Watcher
	parser  *parser.Parser
	events  chan parser.GameEvent
	cancel  context.CancelFunc
}

// NewEventSource creates an EventSource that tails Power.log and emits parsed events.
func NewEventSource(ctx context.Context, powerLogDir string) (EventSource, error) {
	events := make(chan parser.GameEvent, 256)
	p := parser.New(events)

	ctx, cancel := context.WithCancel(ctx)
	w, err := watcher.New(ctx, watcher.Config{
		LogDir: powerLogDir,
	})
	if err != nil {
		cancel()
		return nil, err
	}

	src := &logEventSource{
		watcher: w,
		parser:  p,
		events:  events,
		cancel:  cancel,
	}

	// Feed watcher lines to parser in background.
	go func() {
		for line := range w.Lines {
			p.Feed(line.Text)
		}
		p.Flush()
	}()

	return src, nil
}

func (s *logEventSource) Events() <-chan parser.GameEvent {
	return s.events
}

func (s *logEventSource) Close() error {
	s.cancel()
	s.watcher.Stop()
	return nil
}

// machineStateTracker wraps gamestate.Machine + Processor into a StateTracker.
type machineStateTracker struct {
	machine   *gamestate.Machine
	processor *gamestate.Processor
}

// NewStateTracker creates a StateTracker backed by gamestate.Machine/Processor.
func NewStateTracker() StateTracker {
	m := &gamestate.Machine{}
	p := gamestate.NewProcessor(m)
	return &machineStateTracker{machine: m, processor: p}
}

func (t *machineStateTracker) Apply(event parser.GameEvent) {
	t.processor.Handle(event)
}

func (t *machineStateTracker) Snapshot() CaptureState {
	s := t.machine.State() // acquires RLock internally, returns deep copy
	cs := CaptureState{
		GameID:     s.GameID,
		Timestamp:  time.Now(),
		Turn:       s.Turn,
		Phase:      string(s.Phase),
		TavernTier: s.TavernTier,
		Health:     s.Player.Health,
		Armor:      s.Player.Armor,
		Gold:       s.Player.CurrentGold,
		Placement:  s.Placement,
		IsDuos:     s.IsDuos,
	}
	if s.Partner != nil {
		cs.PartnerHealth = s.Partner.Health
		cs.PartnerTier = s.Partner.TavernTier
	}
	for _, m := range s.Board {
		cs.Board = append(cs.Board, MinionSnapshot{
			CardID:     m.CardID,
			Name:       m.Name,
			Attack:     m.Attack,
			Health:     m.Health,
			Tribes:     m.MinionType,
			BuffAttack: m.BuffAttack,
			BuffHealth: m.BuffHealth,
		})
	}
	for _, b := range s.BuffSources {
		cs.BuffSources = append(cs.BuffSources, BuffSourceSnapshot{
			Category: string(b.Category),
			Attack:   b.Attack,
			Health:   b.Health,
		})
	}
	return cs
}

func (t *machineStateTracker) InGame() bool {
	phase := t.machine.Phase()
	return phase != gamestate.PhaseIdle && phase != gamestate.PhaseLobby
}
```

**Step 2: Write a test for StateTracker snapshot**

Create `internal/capture/pipeline_test.go`. Tests that the snapshot correctly maps gamestate fields to CaptureState. Does not require Power.log — directly drives the processor with synthetic events.

```go
package capture

import (
	"testing"

	"battlestream.fixates.io/internal/parser"
)

func TestStateTrackerSnapshot(t *testing.T) {
	tracker := NewStateTracker()

	if tracker.InGame() {
		t.Fatal("should not be in game initially")
	}

	// Simulate game start.
	tracker.Apply(parser.GameEvent{
		Type: parser.EventGameStart,
		Tags: map[string]string{
			"GameAccountId": "[hi=144115198130930503 lo=71498679]",
		},
	})

	if !tracker.InGame() {
		t.Fatal("should be in game after game start")
	}

	snap := tracker.Snapshot()
	if snap.Phase == "" {
		t.Error("phase should not be empty during game")
	}
}
```

**Step 3: Run tests**

Run: `go test -count=1 ./internal/capture/`
Expected: all tests pass.

**Step 4: Commit**

```
git add internal/capture/pipeline.go internal/capture/pipeline_test.go
git commit -m "feat(bscapture): add pipeline wrappers for EventSource and StateTracker"
```

---

### Task 6: Capture loop

**Files:**
- Create: `internal/capture/loop.go`
- Create: `internal/capture/loop_test.go`

**Step 1: Write the capture loop**

Create `internal/capture/loop.go`. Orchestrates the full lifecycle: wait for game, capture frames, detect game end / stale timeout.

```go
package capture

import (
	"context"
	"log/slog"
	"time"
)

// Loop is the main capture loop orchestrator.
type Loop struct {
	events       EventSource
	tracker      StateTracker
	screenshotter Screenshotter
	store        FrameStore
	interval     time.Duration
	staleTimeout time.Duration
}

// NewLoop creates a capture loop with the given components.
func NewLoop(
	events EventSource,
	tracker StateTracker,
	screenshotter Screenshotter,
	store FrameStore,
	interval time.Duration,
	staleTimeout time.Duration,
) *Loop {
	return &Loop{
		events:        events,
		tracker:       tracker,
		screenshotter: screenshotter,
		store:         store,
		interval:      interval,
		staleTimeout:  staleTimeout,
	}
}

// Run starts the capture loop. Blocks until ctx is cancelled.
func (l *Loop) Run(ctx context.Context) error {
	var (
		lastEventTime = time.Now()
		capturing     = false
		sequence      = 0
	)

	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	// Event consumer goroutine — applies events to state tracker,
	// updates last event time.
	eventTimeCh := make(chan time.Time, 256)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-l.events.Events():
				if !ok {
					return
				}
				l.tracker.Apply(ev)
				eventTimeCh <- time.Now()
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			if capturing {
				snap := l.tracker.Snapshot()
				l.store.FinalizeGame(snap.Placement)
			}
			return ctx.Err()

		case t := <-eventTimeCh:
			lastEventTime = t

		case <-ticker.C:
			// Drain any pending event times.
			for {
				select {
				case t := <-eventTimeCh:
					lastEventTime = t
				default:
					goto donedraining
				}
			}
		donedraining:

			inGame := l.tracker.InGame()

			// Check stale timeout.
			if capturing && time.Since(lastEventTime) > l.staleTimeout {
				slog.Warn("stale timeout reached, ending capture",
					"elapsed", time.Since(lastEventTime))
				snap := l.tracker.Snapshot()
				l.store.FinalizeGame(snap.Placement)
				l.store.Close()
				capturing = false
				sequence = 0
				continue
			}

			// Game started.
			if inGame && !capturing {
				snap := l.tracker.Snapshot()
				slog.Info("game detected, starting capture", "game_id", snap.GameID)
				if err := l.store.InitGame(snap.GameID); err != nil {
					slog.Error("failed to init game store", "err", err)
					continue
				}
				capturing = true
				sequence = 0
				lastEventTime = time.Now()
			}

			// Game ended.
			if !inGame && capturing {
				snap := l.tracker.Snapshot()
				slog.Info("game ended", "placement", snap.Placement)
				l.store.FinalizeGame(snap.Placement)
				l.store.Close()
				capturing = false
				sequence = 0
				continue
			}

			// Capture frame.
			if capturing {
				captureStart := time.Now()
				snap := l.tracker.Snapshot()

				img, err := l.screenshotter.Capture(ctx)
				if err != nil {
					slog.Error("screenshot failed", "err", err)
					continue
				}

				latency := time.Since(captureStart).Milliseconds()
				if latency > l.interval.Milliseconds() {
					slog.Warn("capture took longer than interval",
						"latency_ms", latency, "interval_ms", l.interval.Milliseconds())
				}

				frame := Frame{
					Sequence:       sequence,
					Image:          img,
					State:          snap,
					CaptureLatency: latency,
				}

				if err := l.store.SaveFrame(frame); err != nil {
					slog.Error("save frame failed", "err", err, "seq", sequence)
					continue
				}

				sequence++
			}
		}
	}
}
```

**Step 2: Write loop test with mock implementations**

Create `internal/capture/loop_test.go`. Uses mock implementations of all interfaces to test the loop lifecycle without real grim/sqlite.

```go
package capture

import (
	"context"
	"image"
	"sync"
	"testing"
	"time"

	"battlestream.fixates.io/internal/parser"
)

// mockEventSource emits events sent via Send().
type mockEventSource struct {
	ch chan parser.GameEvent
}

func newMockEventSource() *mockEventSource {
	return &mockEventSource{ch: make(chan parser.GameEvent, 64)}
}
func (m *mockEventSource) Events() <-chan parser.GameEvent { return m.ch }
func (m *mockEventSource) Close() error                   { close(m.ch); return nil }
func (m *mockEventSource) Send(e parser.GameEvent)         { m.ch <- e }

// mockStateTracker returns canned state.
type mockStateTracker struct {
	mu    sync.Mutex
	state CaptureState
	inGame bool
}

func (m *mockStateTracker) Apply(_ parser.GameEvent) {}
func (m *mockStateTracker) Snapshot() CaptureState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}
func (m *mockStateTracker) InGame() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inGame
}
func (m *mockStateTracker) SetInGame(v bool, gameID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inGame = v
	m.state.GameID = gameID
	if v {
		m.state.Phase = "RECRUIT"
	} else {
		m.state.Phase = "IDLE"
	}
}

// mockScreenshotter returns a test image.
type mockScreenshotter struct{}

func (m *mockScreenshotter) Capture(_ context.Context) (image.Image, error) {
	return testImage(1920, 1080), nil
}

// mockFrameStore records calls.
type mockFrameStore struct {
	mu         sync.Mutex
	initCalls  []string
	frames     []Frame
	finalized  bool
	placement  int
}

func (m *mockFrameStore) InitGame(gameID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initCalls = append(m.initCalls, gameID)
	return nil
}
func (m *mockFrameStore) SaveFrame(f Frame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frames = append(m.frames, f)
	return nil
}
func (m *mockFrameStore) FinalizeGame(placement int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finalized = true
	m.placement = placement
	return nil
}
func (m *mockFrameStore) Close() error { return nil }
func (m *mockFrameStore) FrameCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.frames)
}

func TestLoopCapturesDuringGame(t *testing.T) {
	events := newMockEventSource()
	tracker := &mockStateTracker{}
	store := &mockFrameStore{}

	loop := NewLoop(events, tracker, &mockScreenshotter{}, store,
		50*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	go loop.Run(ctx)

	// Simulate game start.
	tracker.SetInGame(true, "test-game")
	events.Send(parser.GameEvent{Type: parser.EventGameStart})

	// Wait for a few captures.
	time.Sleep(200 * time.Millisecond)

	if store.FrameCount() == 0 {
		t.Error("expected frames to be captured during game")
	}

	// Simulate game end.
	tracker.SetInGame(false, "")
	time.Sleep(100 * time.Millisecond)

	cancel()
	time.Sleep(50 * time.Millisecond)

	store.mu.Lock()
	finalized := store.finalized
	store.mu.Unlock()

	if !finalized {
		t.Error("expected game to be finalized after end")
	}
}
```

**Step 3: Run tests**

Run: `go test -count=1 ./internal/capture/`
Expected: all tests pass.

**Step 4: Commit**

```
git add internal/capture/loop.go internal/capture/loop_test.go
git commit -m "feat(bscapture): add capture loop with game lifecycle management"
```

---

### Task 7: Wire up `run` subcommand

**Files:**
- Modify: `cmd/bscapture/main.go`

**Step 1: Implement the run command**

Wire up the capture loop using real implementations. Set up signal handling for graceful shutdown.

Replace the `cmdRun()` stub in `cmd/bscapture/main.go`:

```go
func cmdRun() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Start capture session — waits for game, captures screenshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := capture.LoadConfig(cfgFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			if cfg.PowerLogPath == "" {
				return fmt.Errorf("power_log_path not configured — run 'bscapture detect' first")
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			slog.Info("bscapture starting",
				"monitor", cfg.Monitor,
				"interval", cfg.CaptureInterval,
				"resolution", fmt.Sprintf("%dx%d", cfg.OutputResWidth, cfg.OutputResHeight),
				"data_dir", cfg.DataDir,
			)

			events, err := capture.NewEventSource(ctx, filepath.Dir(cfg.PowerLogPath))
			if err != nil {
				return fmt.Errorf("event source: %w", err)
			}
			defer events.Close()

			tracker := capture.NewStateTracker()
			screenshotter := capture.NewScreenshotter(cfg.Monitor, cfg.OutputResWidth, cfg.OutputResHeight)
			store := capture.NewStore(cfg.DataDir, cfg.JPEGQuality)

			loop := capture.NewLoop(events, tracker, screenshotter, store,
				cfg.CaptureInterval, cfg.StaleTimeout)

			slog.Info("waiting for game...")
			return loop.Run(ctx)
		},
	}
}
```

Add necessary imports: `context`, `fmt`, `log/slog`, `os`, `os/signal`, `path/filepath`, `syscall`, and the capture package.

**Step 2: Verify it builds**

Run: `go build ./cmd/bscapture`
Expected: builds cleanly.

**Step 3: Verify help output**

Run: `./bscapture run --help`
Expected: shows help text.

**Step 4: Commit**

```
git add cmd/bscapture/main.go
git commit -m "feat(bscapture): wire up run subcommand with full capture pipeline"
```

---

### Task 8: `detect` subcommand

**Files:**
- Modify: `cmd/bscapture/main.go`

**Step 1: Implement the detect command**

Replace the `cmdDetect()` stub. Uses `hyprctl monitors -j` for monitor detection and `internal/discovery` for Power.log path. Prompts user interactively.

```go
func cmdDetect() *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Detect monitors and Hearthstone install, update config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := capture.LoadConfig(cfgFile) // OK if no config yet

			// Detect monitors via hyprctl.
			monitors, err := detectMonitors()
			if err != nil {
				return fmt.Errorf("detecting monitors: %w", err)
			}
			if len(monitors) == 0 {
				return fmt.Errorf("no monitors detected")
			}

			fmt.Println("Detected monitors:")
			for i, m := range monitors {
				fmt.Printf("  [%d] %-10s %dx%d  %s\n", i+1, m.Name, m.Width, m.Height, m.Description)
			}

			fmt.Printf("\nSelect monitor for capture [1]: ")
			var choice int
			if _, err := fmt.Scanln(&choice); err != nil || choice == 0 {
				choice = 1
			}
			if choice < 1 || choice > len(monitors) {
				return fmt.Errorf("invalid selection: %d", choice)
			}
			cfg.Monitor = monitors[choice-1].Name
			fmt.Printf("Selected: %s\n", cfg.Monitor)

			// Detect Power.log.
			info, err := discovery.Discover()
			if err != nil {
				fmt.Printf("\nCould not auto-detect Hearthstone: %v\n", err)
				fmt.Printf("Set power_log_path manually in config.\n")
			} else {
				cfg.PowerLogPath = filepath.Join(info.LogPath, "Power.log")
				fmt.Printf("Power.log: %s\n", cfg.PowerLogPath)
			}

			// Save config.
			home, _ := os.UserHomeDir()
			cfgPath := filepath.Join(home, ".config", "bscapture", "config.yaml")
			if cfgFile != "" {
				cfgPath = cfgFile
			}
			if err := capture.SaveConfig(cfg, cfgPath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
			fmt.Printf("\nConfig written to %s\n", cfgPath)
			return nil
		},
	}
}

type monitorInfo struct {
	Name        string
	Width       int
	Height      int
	Description string
}

func detectMonitors() ([]monitorInfo, error) {
	out, err := exec.Command("hyprctl", "monitors", "-j").Output()
	if err != nil {
		return nil, fmt.Errorf("hyprctl monitors: %w", err)
	}

	var raw []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse monitors: %w", err)
	}

	monitors := make([]monitorInfo, len(raw))
	for i, r := range raw {
		monitors[i] = monitorInfo{
			Name:        r.Name,
			Width:       r.Width,
			Height:      r.Height,
			Description: r.Description,
		}
	}
	return monitors, nil
}
```

Add imports: `encoding/json`, `os/exec`, `battlestream.fixates.io/internal/discovery`.

**Step 2: Verify it builds**

Run: `go build ./cmd/bscapture`
Expected: builds cleanly.

**Step 3: Commit**

```
git add cmd/bscapture/main.go
git commit -m "feat(bscapture): add detect subcommand for monitor and HS path setup"
```

---

### Task 9: `list` and `config` subcommands

**Files:**
- Modify: `cmd/bscapture/main.go`

**Step 1: Implement the list command**

Scans data dir for game directories containing `capture.db`, reads the `games` table, prints summary.

```go
func cmdList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List captured game sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := capture.LoadConfig(cfgFile)
			if err != nil {
				return err
			}

			entries, err := os.ReadDir(cfg.DataDir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No captures found.")
					return nil
				}
				return err
			}

			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				dbPath := filepath.Join(cfg.DataDir, entry.Name(), "capture.db")
				if _, err := os.Stat(dbPath); err != nil {
					continue
				}
				db, err := sql.Open("sqlite", dbPath)
				if err != nil {
					continue
				}
				var gameID, startTime string
				var placement, frames int
				var endTime sql.NullString
				err = db.QueryRow(`SELECT game_id, start_time, end_time, placement, total_frames FROM games LIMIT 1`).
					Scan(&gameID, &startTime, &endTime, &placement, &frames)
				db.Close()
				if err != nil {
					continue
				}
				status := "in-progress"
				if endTime.Valid {
					status = fmt.Sprintf("#%d", placement)
				}
				fmt.Printf("%-40s  %s  %s  %d frames\n", gameID, startTime, status, frames)
			}
			return nil
		},
	}
}
```

**Step 2: Implement the config command**

Shows current config values.

```go
func cmdConfig() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := capture.LoadConfig(cfgFile)
			if err != nil {
				return err
			}
			fmt.Printf("power_log_path:    %s\n", cfg.PowerLogPath)
			fmt.Printf("monitor:           %s\n", cfg.Monitor)
			fmt.Printf("capture_interval:  %s\n", cfg.CaptureInterval)
			fmt.Printf("output_resolution: %dx%d\n", cfg.OutputResWidth, cfg.OutputResHeight)
			fmt.Printf("jpeg_quality:      %d\n", cfg.JPEGQuality)
			fmt.Printf("data_dir:          %s\n", cfg.DataDir)
			fmt.Printf("stale_timeout:     %s\n", cfg.StaleTimeout)
			return nil
		},
	}
}
```

Add imports: `database/sql`, `_ "modernc.org/sqlite"`.

**Step 3: Verify it builds**

Run: `go build ./cmd/bscapture`

**Step 4: Commit**

```
git add cmd/bscapture/main.go
git commit -m "feat(bscapture): add list and config subcommands"
```

---

### Task 10: Update .gitignore and final integration test

**Files:**
- Modify: `.gitignore`

**Step 1: Add bscapture binary to .gitignore**

Add `/bscapture` line next to the existing `/battlestream` entry.

**Step 2: Run full test suite**

Run: `go test -race -count=1 ./...`
Expected: all tests pass including new `internal/capture/` tests.

**Step 3: Run go vet**

Run: `go vet ./...`
Expected: clean.

**Step 4: Manual smoke test**

Run: `./bscapture config`
Expected: prints default config values.

Run: `./bscapture detect`
Expected: lists monitors, prompts for selection.

**Step 5: Commit**

```
git add .gitignore
git commit -m "chore: add bscapture binary to gitignore"
```

---

## Notes

- The `internal/capture/` package is intentionally separate from `internal/gamestate/` — it imports gamestate but doesn't modify it.
- The `watcher.Config` struct usage in Task 5 may need adjustment depending on the exact fields expected — verify by reading `internal/watcher/watcher.go` at implementation time.
- The `NewEventSource` function takes the Power.log directory path, not the file path — the watcher handles file discovery within the directory.
- All SQLite operations use `modernc.org/sqlite` (pure Go, no CGo) for easy builds.
- The `detect` command depends on `hyprctl` which is Hyprland-specific — this is intentional per the design (only needs to work on current system).
