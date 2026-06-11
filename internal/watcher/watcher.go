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
	Lines    <-chan Line
	lines    chan Line
	done     chan struct{}
	stopOnce sync.Once

	// wg tracks every goroutine that sends on w.lines. Stop waits for it
	// before closing the channel, so a send-after-close is impossible.
	wg sync.WaitGroup

	mu          sync.Mutex
	tails       []*tail.Tail
	stopped     bool   // set under mu by Stop; blocks new tails/senders
	resolvedDir string // the resolved session log directory being tailed
}

// ResolvedDir returns the session log directory currently being tailed.
// It is updated when the watcher switches to a new Hearthstone session
// directory (see watchForNewSessions).
func (w *Watcher) ResolvedDir() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.resolvedDir
}

// Line is a raw log line with its source file.
type Line struct {
	File string
	Text string
	// Backlog is true while the line comes from content that already existed
	// in the file when the tail started (ReadFromStart replay of history).
	// The first line with Backlog=false marks the live tail. Consumers that
	// must distinguish "replaying old games" from "live play" (e.g. the
	// bscapture catchup barrier) rely on this flag.
	Backlog bool
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
		Lines: lines,
		lines: lines,
		done:  make(chan struct{}),
	}

	// On macOS, use Player.log exclusively. Hearthstone's per-file logging
	// (FilePrinting) hits a ~5MB Unity limit and stops mid-game. Console
	// output (ConsolePrinting) goes to Player.log without that limit.
	if cfg.PlayerLogPath != "" {
		if err := w.startPlayerLogTail(ctx, cfg.PlayerLogPath, cfg.ReadFromStart); err != nil {
			w.Stop()
			return nil, err
		}
		go w.stopOnCancel(ctx)
		return w, nil
	}

	// Non-macOS: tail per-session log files in the Logs directory.
	logDir := resolveLogDir(cfg.LogDir, cfg.Files)
	slog.Info("resolved log directory", "configured", cfg.LogDir, "resolved", logDir)

	if err := w.startTails(ctx, logDir, cfg, cfg.ReadFromStart); err != nil {
		w.Stop()
		return nil, err
	}

	// Watch for new session directories so we switch when HS restarts.
	go w.watchForNewSessions(ctx, cfg)

	go w.stopOnCancel(ctx)

	return w, nil
}

// stopOnCancel stops the watcher when ctx is cancelled. It also exits when
// Stop is called directly, so it does not leak on non-context shutdown.
func (w *Watcher) stopOnCancel(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-w.done:
	}
	w.Stop()
}

// startTails begins tailing all configured files in the given directory.
// If fromStart is true, reads from the beginning of the file to catch up.
func (w *Watcher) startTails(ctx context.Context, logDir string, cfg Config, fromStart bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Stop has run (or is running): the Lines channel is closing, so no new
	// tails or sender goroutines may be created.
	if w.stopped {
		return nil
	}
	w.resolvedDir = logDir

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

		// Record the size of any pre-existing content. Each emitted tail.Line
		// carries the byte offset reached after reading it (SeekInfo.Offset),
		// so lines at or below this size are backlog (replayed history) and
		// lines beyond it are live writes. This is a purely mechanical
		// boundary — unlike event timestamps it cannot be fooled by date
		// rollover or a log written at the same wall-clock time yesterday.
		var backlogSize int64
		if fromStart {
			if fi, err := os.Stat(path); err == nil {
				backlogSize = fi.Size()
			}
		}

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

		// Register the sender while holding w.mu: Stop sets w.stopped under
		// the same lock before calling wg.Wait, so either this Add is visible
		// to Wait, or the stopped check above prevented the goroutine from
		// being created at all.
		w.wg.Add(1)
		go func(t *tail.Tail, fname string, backlogSize int64) {
			defer w.wg.Done()
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
					// SeekInfo.Offset is the position after the line was
					// consumed; the last backlog line lands exactly on
					// backlogSize, anything past it is live.
					backlog := line.SeekInfo.Offset <= backlogSize
					select {
					case w.lines <- Line{File: fname, Text: line.Text, Backlog: backlog}:
					case <-ctx.Done():
						return
					case <-w.done:
						return
					}
				case <-ctx.Done():
					return
				case <-w.done:
					return
				}
			}
		}(t, name, backlogSize)
	}

	return nil
}

// stopTails stops all current tails (used when switching session dirs).
func (w *Watcher) stopTails() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopTailsLocked()
}

// stopTailsLocked stops and cleans up all current tails. Caller holds w.mu.
// tail.Stop waits for the tail goroutine to exit (its sends select on the
// tomb's Dying channel, so it cannot block on an abandoned Lines channel),
// and Cleanup releases the inotify watch state — without it every
// Hearthstone restart would leak a watch.
func (w *Watcher) stopTailsLocked() {
	for _, t := range w.tails {
		_ = t.Stop()
		t.Cleanup()
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
// Safe to call concurrently and multiple times. Concurrent callers block
// until the first call completes; when Stop returns, the Lines channel is
// closed and no further sends can occur.
func (w *Watcher) Stop() {
	w.stopOnce.Do(func() {
		// Unblock any sender parked on a full w.lines channel.
		close(w.done)

		// Under the same lock the senders are registered with: after stopped
		// is set, startTails/startPlayerLogTail refuse to create new senders,
		// so wg below covers every goroutine that can send on w.lines.
		w.mu.Lock()
		w.stopped = true
		w.stopTailsLocked()
		w.mu.Unlock()

		// Wait for all senders to exit, then closing is safe.
		w.wg.Wait()
		close(w.lines)
	})
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

	// Pre-existing content boundary — see startTails for rationale.
	var backlogSize int64
	if fromStart {
		if fi, err := os.Stat(path); err == nil {
			backlogSize = fi.Size()
		}
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
	if w.stopped {
		// Stop already ran — do not register a sender on a closing channel.
		w.mu.Unlock()
		_ = t.Stop()
		t.Cleanup()
		return nil
	}
	w.tails = append(w.tails, t)
	w.wg.Add(1) // registered under w.mu — see startTails for the invariant
	w.mu.Unlock()

	go func() {
		defer w.wg.Done()
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
				backlog := line.SeekInfo.Offset <= backlogSize
				select {
				case w.lines <- Line{File: "Power.log", Text: text, Backlog: backlog}:
				case <-ctx.Done():
					return
				case <-w.done:
					return
				}
			case <-ctx.Done():
				return
			case <-w.done:
				return
			}
		}
	}()

	return nil
}
