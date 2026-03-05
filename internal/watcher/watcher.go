// Package watcher tails Hearthstone log files and emits raw lines.
package watcher

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/nxadm/tail"
)

// Watcher tails one or more HS log files and sends lines to Lines channel.
type Watcher struct {
	Lines  <-chan Line
	errors chan error
	done   chan struct{}
	tails  []*tail.Tail
}

// Line is a raw log line with its source file.
type Line struct {
	File string
	Text string
}

// Config configures which log files to watch.
type Config struct {
	LogDir  string   // directory containing the log files
	Files   []string // file names to tail (e.g. "Power.log", "Zone.log")
	Reopen  bool     // reopen file when truncated (log rotation)
	MustExist bool   // fail if file does not exist at startup
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
		errors: make(chan error, 1),
		done:   make(chan struct{}),
	}

	for _, name := range cfg.Files {
		path := filepath.Join(cfg.LogDir, name)

		if cfg.MustExist {
			if _, err := os.Stat(path); err != nil {
				return nil, fmt.Errorf("log file %s does not exist: %w", path, err)
			}
		}

		t, err := tail.TailFile(path, tail.Config{
			Follow:    true,
			ReOpen:    cfg.Reopen,
			MustExist: cfg.MustExist,
			Location:  &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd}, // only new lines
			Logger:    tail.DiscardingLogger,
		})
		if err != nil {
			w.Stop()
			return nil, fmt.Errorf("tailing %s: %w", path, err)
		}
		w.tails = append(w.tails, t)

		go func(t *tail.Tail, fname string) {
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
					select {
					case lines <- Line{File: fname, Text: line.Text}:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}(t, name)
	}

	go func() {
		<-ctx.Done()
		w.Stop()
	}()

	return w, nil
}

// Stop gracefully stops all tails and closes the Lines channel.
func (w *Watcher) Stop() {
	select {
	case <-w.done:
		return
	default:
		close(w.done)
	}
	for _, t := range w.tails {
		_ = t.Stop()
	}
}
