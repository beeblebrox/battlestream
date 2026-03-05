package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

// makeInstallDir creates a temp directory that looks like a HS install root
// (contains Hearthstone.exe and a Logs/ subdirectory).
func makeInstallDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Hearthstone.exe"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "Logs"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// makeLogsDir creates a temp Logs directory containing Power.log.
func makeLogsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Power.log"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestProbeRootWithExe(t *testing.T) {
	dir := makeInstallDir(t)
	info, err := probeRoot(dir)
	if err != nil {
		t.Fatalf("probeRoot: %v", err)
	}
	if info.InstallRoot != dir {
		t.Errorf("InstallRoot: expected %q, got %q", dir, info.InstallRoot)
	}
	if info.LogPath != filepath.Join(dir, "Logs") {
		t.Errorf("LogPath: expected %q, got %q", filepath.Join(dir, "Logs"), info.LogPath)
	}
	if info.LogConfig == "" {
		t.Error("LogConfig should not be empty")
	}
}

func TestProbeRootWithLogsDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Logs"), 0755); err != nil {
		t.Fatal(err)
	}
	info, err := probeRoot(dir)
	if err != nil {
		t.Fatalf("probeRoot: %v", err)
	}
	if info.InstallRoot != dir {
		t.Errorf("InstallRoot: expected %q, got %q", dir, info.InstallRoot)
	}
}

func TestProbeRootWithAppBundle(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Hearthstone.app"), 0755); err != nil {
		t.Fatal(err)
	}
	info, err := probeRoot(dir)
	if err != nil {
		t.Fatalf("probeRoot: %v", err)
	}
	if info.InstallRoot != dir {
		t.Errorf("InstallRoot: expected %q, got %q", dir, info.InstallRoot)
	}
}

// TestProbeRootAsLogsDir verifies that if the root itself is the Logs
// directory (contains Power.log), the parent is used as InstallRoot.
func TestProbeRootAsLogsDir(t *testing.T) {
	logsDir := makeLogsDir(t)
	info, err := probeRoot(logsDir)
	if err != nil {
		t.Fatalf("probeRoot: %v", err)
	}
	if info.LogPath != logsDir {
		t.Errorf("LogPath: expected %q, got %q", logsDir, info.LogPath)
	}
	if info.InstallRoot != filepath.Dir(logsDir) {
		t.Errorf("InstallRoot: expected %q, got %q", filepath.Dir(logsDir), info.InstallRoot)
	}
}

func TestProbeRootEmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := probeRoot(dir)
	if err == nil {
		t.Error("expected error for empty directory, got nil")
	}
}

func TestProbeRootNonExistent(t *testing.T) {
	_, err := probeRoot("/absolutely/nonexistent/path/to/hearthstone")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestDiscoverFromRoot(t *testing.T) {
	dir := makeInstallDir(t)
	info, err := DiscoverFromRoot(dir)
	if err != nil {
		t.Fatalf("DiscoverFromRoot: %v", err)
	}
	if info.InstallRoot != dir {
		t.Errorf("InstallRoot: expected %q, got %q", dir, info.InstallRoot)
	}
}

func TestDiscoverFromRootFails(t *testing.T) {
	_, err := DiscoverFromRoot(t.TempDir())
	if err == nil {
		t.Error("expected error for non-install directory")
	}
}

func TestWalkForInstall(t *testing.T) {
	parent := t.TempDir()
	sub := filepath.Join(parent, "games", "hearthstone")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "Hearthstone.exe"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	info, err := WalkForInstall(parent)
	if err != nil {
		t.Fatalf("WalkForInstall: %v", err)
	}
	if info.InstallRoot != sub {
		t.Errorf("InstallRoot: expected %q, got %q", sub, info.InstallRoot)
	}
}

func TestWalkForInstallNotFound(t *testing.T) {
	_, err := WalkForInstall(t.TempDir())
	if err == nil {
		t.Error("expected error when no install found")
	}
}

// TestWalkForInstallDeep ensures the walk finds an install several levels deep.
func TestWalkForInstallDeep(t *testing.T) {
	parent := t.TempDir()
	deep := filepath.Join(parent, "a", "b", "c", "Hearthstone")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(deep, "Logs"), 0755); err != nil {
		t.Fatal(err)
	}

	info, err := WalkForInstall(parent)
	if err != nil {
		t.Fatalf("WalkForInstall deep: %v", err)
	}
	if info.InstallRoot != deep {
		t.Errorf("InstallRoot: expected %q, got %q", deep, info.InstallRoot)
	}
}

// TestWalkForInstallStopsAtFirst verifies only the first match is returned
// when multiple installs exist under the search root.
func TestWalkForInstallStopsAtFirst(t *testing.T) {
	parent := t.TempDir()
	for _, name := range []string{"hs1", "hs2"} {
		d := filepath.Join(parent, name)
		if err := os.Mkdir(d, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "Hearthstone.exe"), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	info, err := WalkForInstall(parent)
	if err != nil {
		t.Fatalf("WalkForInstall: %v", err)
	}
	// Should succeed and return exactly one result.
	if info == nil {
		t.Error("expected a result, got nil")
	}
}
