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

func TestPlayerLogTailFiltersPowerLines(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping file-watcher integration test in short mode")
	}

	dir := t.TempDir()
	playerLogPath := filepath.Join(dir, "Player.log")
	createLogFile(t, dir, "Player.log")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use PlayerLogPath — this makes the watcher use Player.log exclusively.
	w, err := New(ctx, Config{
		LogDir:        dir,
		Files:         []string{"Power.log"},
		PlayerLogPath: playerLogPath,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)

	// Write mixed content to Player.log: Power lines, Unity noise, other categories.
	f, err := os.OpenFile(playerLogPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{
		"Initialize engine version: 2022.3.62f2",
		"[Power] D 10:00:00.0000000 GameState.DebugPrintPower() - CREATE_GAME",
		"[Hearthstone] some UI event",
		"[Power] D 10:00:01.0000000 GameState.DebugPrintPower() -     TAG_CHANGE Entity=GameEntity tag=TURN value=1",
		"[GameNetLogger] network stuff",
	}
	for _, l := range lines {
		fmt.Fprintln(f, l)
	}
	f.Close()

	// Should receive exactly the 2 [Power] lines, with the prefix stripped.
	var received []string
	timeout := time.After(5 * time.Second)
	for len(received) < 2 {
		select {
		case line := <-w.Lines:
			received = append(received, line.Text)
			// File should be reported as "Power.log" even though source is Player.log.
			if line.File != "Power.log" {
				t.Errorf("expected file Power.log, got %q", line.File)
			}
		case <-timeout:
			t.Fatalf("timed out; received %d lines: %v", len(received), received)
		}
	}

	if !strings.Contains(received[0], "CREATE_GAME") {
		t.Errorf("first line should contain CREATE_GAME, got %q", received[0])
	}
	if !strings.Contains(received[1], "TAG_CHANGE") {
		t.Errorf("second line should contain TAG_CHANGE, got %q", received[1])
	}
	// Prefix should be stripped.
	if strings.HasPrefix(received[0], "[Power]") {
		t.Errorf("prefix was not stripped: %q", received[0])
	}
}

func TestPlayerLogTailReadFromStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping file-watcher integration test in short mode")
	}

	dir := t.TempDir()
	playerLogPath := filepath.Join(dir, "Player.log")

	// Pre-populate Player.log with content BEFORE starting the watcher.
	f, err := os.Create(playerLogPath)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, "Unity init stuff")
	fmt.Fprintln(f, "[Power] D 10:00:00.0000000 GameState.DebugPrintPower() - CREATE_GAME")
	fmt.Fprintln(f, "[Power] D 10:00:01.0000000 GameState.DebugPrintPower() -     TAG_CHANGE Entity=GameEntity tag=STATE value=RUNNING")
	f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w, err := New(ctx, Config{
		LogDir:        dir,
		Files:         []string{"Power.log"},
		PlayerLogPath: playerLogPath,
		ReadFromStart: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Stop()

	// Should receive the 2 pre-existing [Power] lines.
	var received []string
	timeout := time.After(5 * time.Second)
	for len(received) < 2 {
		select {
		case line := <-w.Lines:
			received = append(received, line.Text)
		case <-timeout:
			t.Fatalf("timed out; received %d lines: %v", len(received), received)
		}
	}

	if !strings.Contains(received[0], "CREATE_GAME") {
		t.Errorf("expected CREATE_GAME in first line, got %q", received[0])
	}
}

func TestPlayerLogSkipsNonPowerLines(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping file-watcher integration test in short mode")
	}

	dir := t.TempDir()
	playerLogPath := filepath.Join(dir, "Player.log")
	createLogFile(t, dir, "Player.log")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	w, err := New(ctx, Config{
		LogDir:        dir,
		Files:         []string{"Power.log"},
		PlayerLogPath: playerLogPath,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)

	// Write only non-Power lines.
	f, err := os.OpenFile(playerLogPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, "Unity init")
	fmt.Fprintln(f, "[Hearthstone] some event")
	fmt.Fprintln(f, "[GameNetLogger] network")
	f.Close()

	// Wait briefly — no Power lines should come through.
	select {
	case line := <-w.Lines:
		t.Errorf("should not receive non-Power lines, got: %q", line.Text)
	case <-time.After(500 * time.Millisecond):
		// Expected — no lines received.
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
