package discovery

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeWinePrefix builds a fake Wine prefix rooted at dir/prefix containing
// a Hearthstone install and the expected users directory structure.
func makeWinePrefix(t *testing.T, prefixRoot string) string {
	t.Helper()
	installDir := filepath.Join(prefixRoot, "drive_c", "Program Files (x86)", "Hearthstone")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "Hearthstone.exe"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(installDir, "Logs"), 0755); err != nil {
		t.Fatal(err)
	}
	return installDir
}

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

// TestDiscoverFromRootWinePrefix verifies that providing a Wine/Proton prefix
// root (a directory that has drive_c/ inside it) finds the install within it.
func TestDiscoverFromRootWinePrefix(t *testing.T) {
	prefix := t.TempDir()
	installDir := makeWinePrefix(t, prefix)

	info, err := DiscoverFromRoot(prefix)
	if err != nil {
		t.Fatalf("DiscoverFromRoot(winePrefix): %v", err)
	}
	if info.InstallRoot != installDir {
		t.Errorf("InstallRoot: expected %q, got %q", installDir, info.InstallRoot)
	}
	if info.LogPath != filepath.Join(installDir, "Logs") {
		t.Errorf("LogPath: expected %q, got %q", filepath.Join(installDir, "Logs"), info.LogPath)
	}
	// LogConfig should be inside the prefix, not in ~/.wine.
	if !filepath.IsAbs(info.LogConfig) {
		t.Errorf("LogConfig is not absolute: %q", info.LogConfig)
	}
	// Must contain the prefix root path.
	if !isSubpath(info.LogConfig, prefix) {
		t.Errorf("LogConfig %q is not under prefix %q", info.LogConfig, prefix)
	}
}

// TestExtractWinePrefix covers the path-parsing helper directly.
func TestExtractWinePrefix(t *testing.T) {
	cases := []struct {
		input  string
		prefix string
		ok     bool
	}{
		{"/chungus/battlenet/drive_c/Program Files/Hearthstone", "/chungus/battlenet", true},
		{"/home/user/.wine/drive_c/Program Files (x86)/Hearthstone", "/home/user/.wine", true},
		{"/pfx/drive_c/users/steamuser/AppData/Local", "/pfx", true},
		{"/just/a/plain/path/Hearthstone", "", false},
		{"/no/drive_c_here/Hearthstone", "", false},
	}
	for _, tc := range cases {
		got, ok := extractWinePrefix(tc.input)
		if ok != tc.ok {
			t.Errorf("extractWinePrefix(%q): ok=%v, want %v", tc.input, ok, tc.ok)
			continue
		}
		if ok && got != tc.prefix {
			t.Errorf("extractWinePrefix(%q): prefix=%q, want %q", tc.input, got, tc.prefix)
		}
	}
}

// TestLogConfigInPrefixPrefersExistingUser verifies that logConfigInPrefix
// returns a path under the first user directory that actually exists.
func TestLogConfigInPrefixPrefersExistingUser(t *testing.T) {
	prefix := t.TempDir()
	home := t.TempDir() // fake home; base = last segment
	username := filepath.Base(home)

	// Create the steamuser directory but NOT the host-username directory.
	steamUserDir := filepath.Join(prefix, "drive_c", "users", "steamuser")
	if err := os.MkdirAll(steamUserDir, 0755); err != nil {
		t.Fatal(err)
	}

	path := logConfigInPrefix(prefix, home)
	if filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(path)))))) != "steamuser" {
		// Check that "steamuser" appears in the path since that dir exists.
		if !containsSegment(path, "steamuser") {
			t.Errorf("expected steamuser in path %q (host user %q dir absent)", path, username)
		}
	}
}

// TestLogConfigInPrefixFallsBackToUsername verifies fallback when no user dir exists.
func TestLogConfigInPrefixFallsBackToUsername(t *testing.T) {
	prefix := t.TempDir()
	home := t.TempDir()
	username := filepath.Base(home)

	path := logConfigInPrefix(prefix, home)
	if !containsSegment(path, username) {
		t.Errorf("expected username %q in fallback path %q", username, path)
	}
}

// isSubpath reports whether child is lexically under parent.
func isSubpath(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && !filepath.IsAbs(rel) && rel != ".."
}

// containsSegment reports whether any path segment of p equals seg.
func containsSegment(p, seg string) bool {
	for _, part := range filepath.SplitList(p) {
		_ = part
	}
	// Walk each component.
	cur := p
	for {
		base := filepath.Base(cur)
		if base == seg {
			return true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return false
}

func TestPlayerLogPathOnDarwin(t *testing.T) {
	path := PlayerLogPath()
	if runtime.GOOS == "darwin" {
		if path == "" {
			t.Error("on macOS, PlayerLogPath should return a non-empty path")
		}
		if !strings.Contains(path, "Player.log") {
			t.Errorf("expected Player.log in path, got %q", path)
		}
		if !strings.Contains(path, "Blizzard Entertainment") {
			t.Errorf("expected 'Blizzard Entertainment' in path, got %q", path)
		}
	} else {
		if path != "" {
			t.Errorf("on non-macOS, expected empty PlayerLogPath, got %q", path)
		}
	}
}

func TestProbeRootSetsPlayerLogPath(t *testing.T) {
	dir := makeInstallDir(t)
	info, err := probeRoot(dir)
	if err != nil {
		t.Fatalf("probeRoot: %v", err)
	}
	if runtime.GOOS == "darwin" {
		if info.PlayerLogPath == "" {
			t.Error("on macOS, PlayerLogPath should be set")
		}
	} else {
		if info.PlayerLogPath != "" {
			t.Errorf("on non-macOS, expected empty PlayerLogPath, got %q", info.PlayerLogPath)
		}
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
