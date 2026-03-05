package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func createLogFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	f.Close()
	return path
}

func TestWatcherMustExistFails(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := New(ctx, Config{
		LogDir:    dir,
		Files:     []string{"nonexistent.log"},
		MustExist: true,
	})
	if err == nil {
		t.Error("expected error for non-existent file with MustExist=true")
	}
}

func TestWatcherSucceedsWithExistingFile(t *testing.T) {
	dir := t.TempDir()
	createLogFile(t, dir, "Power.log")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := New(ctx, Config{
		LogDir:    dir,
		Files:     []string{"Power.log"},
		MustExist: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	w.Stop()
}

func TestWatcherSucceedsWithMissingFileWhenNotRequired(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := New(ctx, Config{
		LogDir:    dir,
		Files:     []string{"missing.log"},
		MustExist: false,
	})
	if err != nil {
		t.Fatalf("expected no error when MustExist=false, got: %v", err)
	}
	w.Stop()
}

func TestWatcherStopIdempotent(t *testing.T) {
	dir := t.TempDir()
	createLogFile(t, dir, "Power.log")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := New(ctx, Config{LogDir: dir, Files: []string{"Power.log"}})
	if err != nil {
		t.Fatal(err)
	}

	// Multiple Stop calls must not panic or deadlock.
	w.Stop()
	w.Stop()
	w.Stop()
}

func TestWatcherPicksUpNewLines(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping file-watcher integration test in short mode")
	}

	dir := t.TempDir()
	logPath := createLogFile(t, dir, "Power.log")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w, err := New(ctx, Config{
		LogDir:    dir,
		Files:     []string{"Power.log"},
		MustExist: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Stop()

	// Give the tail goroutine a moment to seek to end and start watching.
	time.Sleep(50 * time.Millisecond)

	// Append a line to the file.
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(f, "D 10:00:00.0000000 GameState.DebugPrintPower() - CREATE_GAME"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	select {
	case line := <-w.Lines:
		if !strings.Contains(line.Text, "CREATE_GAME") {
			t.Errorf("unexpected line content: %q", line.Text)
		}
		if line.File != "Power.log" {
			t.Errorf("expected file Power.log, got %q", line.File)
		}
	case <-ctx.Done():
		t.Error("timed out waiting for line from watcher")
	}
}

func TestWatcherMultipleFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping file-watcher integration test in short mode")
	}

	dir := t.TempDir()
	powerPath := createLogFile(t, dir, "Power.log")
	zonePath := createLogFile(t, dir, "Zone.log")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w, err := New(ctx, Config{
		LogDir:    dir,
		Files:     []string{"Power.log", "Zone.log"},
		MustExist: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)

	// Write to both files.
	for _, p := range []string{powerPath, zonePath} {
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(f, "test line")
		f.Close()
	}

	received := make(map[string]bool)
	timeout := time.After(5 * time.Second)
	for len(received) < 2 {
		select {
		case line := <-w.Lines:
			received[line.File] = true
		case <-timeout:
			t.Errorf("timed out; received from files: %v", received)
			return
		}
	}
	if !received["Power.log"] || !received["Zone.log"] {
		t.Errorf("did not receive lines from both files: %v", received)
	}
}

func TestWatcherContextCancellation(t *testing.T) {
	dir := t.TempDir()
	createLogFile(t, dir, "Power.log")

	ctx, cancel := context.WithCancel(context.Background())
	w, err := New(ctx, Config{LogDir: dir, Files: []string{"Power.log"}})
	if err != nil {
		t.Fatal(err)
	}

	cancel() // should trigger graceful shutdown
	// Give goroutines a moment to exit; Stop should not deadlock after cancel.
	time.Sleep(20 * time.Millisecond)
	w.Stop()
}
