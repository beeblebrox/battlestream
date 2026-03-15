// Package watcher tails Hearthstone log files and emits raw lines.
package watcher

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/nxadm/tail"
)

// Watcher tails one or more HS log files and sends lines to Lines channel.
type Watcher struct {
	Lines  <-chan Line
	lines  chan Line
	errors chan error
	done   chan struct{}

	mu    sync.Mutex
	tails []*tail.Tail
}

// Line is a raw log line with its source file.
type Line struct {
	File string
	Text string
}

// Config configures which log files to watch.
type Config struct {
	LogDir        string   // directory containing the log files (or session subdirs)
	Files         []string // file names to tail (e.g. "Power.log", "Zone.log")
	Reopen        bool     // reopen file when truncated (log rotation)
	MustExist     bool     // fail if file does not exist at startup
	ReadFromStart bool     // read existing content on initial startup (catch up with current game)
	PlayerLogPath string   // macOS: path to Unity Player.log (console output fallback)
}

// New creates and starts a Watcher. It will continue until ctx is cancelled
// or Stop is called. Lines are sent on Watcher.Lines.
func New(ctx context.Context, cfg Config) (*Watcher, error) {
	if len(cfg.Files) == 0 {
		cfg.Files = []string{"Power.log", "Zone.log"}
	}

	lines := make(chan Line, 512)
	w := &Watcher{
		Lines:  lines,
		lines:  lines,
		errors: make(chan error, 1),
		done:   make(chan struct{}),
	}

	// On macOS, use Player.log exclusively. Hearthstone's per-file logging
	// (FilePrinting) hits a ~5MB Unity limit and stops mid-game. Console
	// output (ConsolePrinting) goes to Player.log without that limit.
	if cfg.PlayerLogPath != "" {
		if err := w.startPlayerLogTail(ctx, cfg.PlayerLogPath, cfg.ReadFromStart); err != nil {
			return nil, err
		}
		go func() {
			<-ctx.Done()
			w.Stop()
		}()
		return w, nil
	}

	// Non-macOS: tail per-session log files in the Logs directory.
	logDir := resolveLogDir(cfg.LogDir, cfg.Files)
	slog.Info("resolved log directory", "configured", cfg.LogDir, "resolved", logDir)

	if err := w.startTails(ctx, logDir, cfg, cfg.ReadFromStart); err != nil {
		return nil, err
	}

	// Watch for new session directories so we switch when HS restarts.
	go w.watchForNewSessions(ctx, cfg)

	go func() {
		<-ctx.Done()
		w.Stop()
	}()

	return w, nil
}

// startTails begins tailing all configured files in the given directory.
// If fromStart is true, reads from the beginning of the file to catch up.
func (w *Watcher) startTails(ctx context.Context, logDir string, cfg Config, fromStart bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, name := range cfg.Files {
		path := filepath.Join(logDir, name)

		if cfg.MustExist {
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("log file %s does not exist: %w", path, err)
			}
		}

		var loc *tail.SeekInfo
		if !fromStart {
			loc = &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd}
		}
		// loc=nil means start from the beginning of the file.

		usePoll := runtime.GOOS == "darwin" // kqueue unreliable for appended writes on macOS
		slog.Info("started tailing file", "file", name, "path", path, "poll", usePoll, "fromStart", fromStart)

		t, err := tail.TailFile(path, tail.Config{
			Follow:    true,
			ReOpen:    cfg.Reopen,
			MustExist: cfg.MustExist,
			Location:  loc,
			Poll:      usePoll,
			Logger:    tail.DiscardingLogger,
		})
		if err != nil {
			return fmt.Errorf("tailing %s: %w", path, err)
		}
		w.tails = append(w.tails, t)

		go func(t *tail.Tail, fname string) {
			firstLine := true
			for {
				select {
				case line, ok := <-t.Lines:
					if !ok {
						return
					}
					if line.Err != nil {
						slog.Error("tail error", "file", fname, "err", line.Err)
						continue
					}
					if firstLine {
						slog.Info("first line received from tail", "file", fname)
						firstLine = false
					}
					select {
					case w.lines <- Line{File: fname, Text: line.Text}:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}(t, name)
	}

	return nil
}

// stopTails stops all current tails.
func (w *Watcher) stopTails() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, t := range w.tails {
		_ = t.Stop()
	}
	w.tails = nil
}

// watchForNewSessions uses fsnotify to detect new session subdirectories
// in the configured LogDir and restarts tails when one appears.
func (w *Watcher) watchForNewSessions(ctx context.Context, cfg Config) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("failed to create fsnotify watcher for session dirs", "err", err)
		return
	}
	defer fsw.Close()

	if err := fsw.Add(cfg.LogDir); err != nil {
		slog.Error("failed to watch log dir for new sessions", "dir", cfg.LogDir, "err", err)
		return
	}

	for {
		select {
		case ev, ok := <-fsw.Events:
			if !ok {
				return
			}
			if ev.Op&fsnotify.Create == 0 {
				continue
			}
			// Only care about new Hearthstone session directories.
			base := filepath.Base(ev.Name)
			if !strings.HasPrefix(base, "Hearthstone_") {
				continue
			}
			info, err := os.Stat(ev.Name)
			if err != nil || !info.IsDir() {
				continue
			}
			slog.Info("new HS session directory detected, switching tails", "dir", ev.Name)
			w.stopTails()
			// New session = new game, read from start to get CREATE_GAME.
			if err := w.startTails(ctx, ev.Name, cfg, true); err != nil {
				slog.Error("failed to start tails in new session dir", "dir", ev.Name, "err", err)
			}
		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			slog.Error("fsnotify error watching log dir", "err", err)
		case <-ctx.Done():
			return
		case <-w.done:
			return
		}
	}
}

// resolveLogDir checks if the log files exist directly in dir. If not, it
// looks for Hearthstone session subdirectories (Hearthstone_YYYY_MM_DD_...)
// and returns the most recent one.
func resolveLogDir(dir string, files []string) string {
	// Check if any target file exists directly in dir.
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return dir
		}
	}

	// Look for session subdirectories.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dir
	}

	var sessions []os.DirEntry
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "Hearthstone_") {
			sessions = append(sessions, e)
		}
	}
	if len(sessions) == 0 {
		return dir
	}

	// Sort by name descending — the timestamp format sorts lexicographically.
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Name() > sessions[j].Name()
	})

	resolved := filepath.Join(dir, sessions[0].Name())
	return resolved
}

// Stop gracefully stops all tails and closes the Lines channel.
func (w *Watcher) Stop() {
	select {
	case <-w.done:
		return
	default:
		close(w.done)
	}
	w.stopTails()
}

// powerLogPrefix is the category prefix prepended to Power log lines in Player.log.
const powerLogPrefix = "[Power] "

// startPlayerLogTail is the macOS primary log source. It tails Player.log,
// filters for [Power] lines, strips the prefix, and sends them to the lines
// channel. Player.log does not have the ~5MB Unity file size limit that
// kills per-section FilePrinting output mid-game.
func (w *Watcher) startPlayerLogTail(ctx context.Context, path string, fromStart bool) error {
	var loc *tail.SeekInfo
	if !fromStart {
		loc = &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd}
	}

	slog.Info("using Player.log as primary source (macOS)", "path", path, "fromStart", fromStart)

	t, err := tail.TailFile(path, tail.Config{
		Follow:    true,
		ReOpen:    true,
		MustExist: false,
		Location:  loc,
		Poll:      true, // macOS always
		Logger:    tail.DiscardingLogger,
	})
	if err != nil {
		return fmt.Errorf("tailing Player.log: %w", err)
	}

	w.mu.Lock()
	w.tails = append(w.tails, t)
	w.mu.Unlock()

	go func() {
		firstLine := true
		for {
			select {
			case line, ok := <-t.Lines:
				if !ok {
					return
				}
				if line.Err != nil {
					slog.Error("Player.log tail error", "err", line.Err)
					continue
				}
				// Only pass through [Power] lines, stripping the prefix.
				if !strings.HasPrefix(line.Text, powerLogPrefix) {
					continue
				}
				text := strings.TrimPrefix(line.Text, powerLogPrefix)
				if firstLine {
					slog.Info("first Power line received from Player.log")
					firstLine = false
				}
				select {
				case w.lines <- Line{File: "Power.log", Text: text}:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}
