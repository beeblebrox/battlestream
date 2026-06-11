package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestBaseDirOverride verifies that BS_CONFIG_DIR takes precedence over
// the default ~/.battlestream base directory.
func TestBaseDirOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BS_CONFIG_DIR", dir)

	if got := BaseDir(); got != dir {
		t.Fatalf("BaseDir() = %q, want override %q", got, dir)
	}
}

// TestBaseDirDefault verifies the fallback to ~/.battlestream when the
// override is unset or empty.
func TestBaseDirDefault(t *testing.T) {
	t.Setenv("BS_CONFIG_DIR", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	want := filepath.Join(home, ".battlestream")
	if got := BaseDir(); got != want {
		t.Fatalf("BaseDir() = %q, want default %q", got, want)
	}
}

// TestLoadHonorsConfigDirOverride verifies that Load() discovers
// config.yaml inside $BS_CONFIG_DIR rather than ~/.battlestream.
func TestLoadHonorsConfigDirOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BS_CONFIG_DIR", dir)

	yaml := `api:
  grpc_addr: "127.0.0.1:59999"
profiles:
  isolated:
    storage:
      db_path: "` + filepath.Join(dir, "data") + `"
active_profile: isolated
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.API.GRPCAddr != "127.0.0.1:59999" {
		t.Errorf("Load() did not read config from BS_CONFIG_DIR: grpc_addr = %q, want %q",
			cfg.API.GRPCAddr, "127.0.0.1:59999")
	}
	if cfg.ActiveProfile != "isolated" {
		t.Errorf("active_profile = %q, want %q", cfg.ActiveProfile, "isolated")
	}
}

// TestLoadDefaultProfilePathsHonorOverride verifies that profiles with
// unset paths get defaults rooted under $BS_CONFIG_DIR, not ~/.battlestream.
func TestLoadDefaultProfilePathsHonorOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BS_CONFIG_DIR", dir)

	// Profile with no storage/output paths: defaults must be filled in
	// under the override dir.
	yaml := `profiles:
  bare:
    hearthstone:
      auto_patch_logconfig: true
active_profile: bare
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	p, err := cfg.GetProfile("")
	if err != nil {
		t.Fatalf("GetProfile() error: %v", err)
	}

	wantBase := filepath.Join(dir, "profiles", "bare")
	if !strings.HasPrefix(p.Storage.DBPath, wantBase) {
		t.Errorf("default DBPath = %q, want under %q", p.Storage.DBPath, wantBase)
	}
	if !strings.HasPrefix(p.Output.Path, wantBase) {
		t.Errorf("default Output.Path = %q, want under %q", p.Output.Path, wantBase)
	}
}

// TestExpandHomeProfilePaths verifies that ~/... expands for ALL profile
// paths (including hearthstone.log_path and hearthstone.install_path, which
// main.go uses verbatim as the watcher LogDir), and that ~user/... paths are
// left untouched rather than being mangled into $HOME/user/....
func TestExpandHomeProfilePaths(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("BS_CONFIG_DIR", dir)
	t.Setenv("HOME", home)

	yaml := `profiles:
  main:
    hearthstone:
      install_path: "~/Games/Hearthstone"
      log_path: "~/Games/Hearthstone/Logs"
    storage:
      db_path: "~/.battlestream/data"
    output:
      path: "~bob/stats"
active_profile: main
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	p, err := cfg.GetProfile("")
	if err != nil {
		t.Fatalf("GetProfile() error: %v", err)
	}

	if want := filepath.Join(home, "Games", "Hearthstone"); p.Hearthstone.InstallPath != want {
		t.Errorf("InstallPath = %q, want %q", p.Hearthstone.InstallPath, want)
	}
	if want := filepath.Join(home, "Games", "Hearthstone", "Logs"); p.Hearthstone.LogPath != want {
		t.Errorf("LogPath = %q, want %q", p.Hearthstone.LogPath, want)
	}
	if want := filepath.Join(home, ".battlestream", "data"); p.Storage.DBPath != want {
		t.Errorf("DBPath = %q, want %q", p.Storage.DBPath, want)
	}
	// ~user paths cannot be resolved portably; they must pass through as-is.
	if want := "~bob/stats"; p.Output.Path != want {
		t.Errorf("Output.Path = %q, want untouched %q", p.Output.Path, want)
	}
}

// TestExpandHome exercises the expandHome helper directly.
func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cases := []struct{ in, want string }{
		{"", ""},
		{"~", home},
		{"~/x/y", filepath.Join(home, "x", "y")},
		{"~bob/data", "~bob/data"},
		{"/abs/path", "/abs/path"},
		{"rel/path", "rel/path"},
	}
	for _, c := range cases {
		if got := expandHome(c.in); got != c.want {
			t.Errorf("expandHome(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSaveTUIWritesLoadedConfigPath verifies that SaveTUI writes back to the
// file Load() actually loaded (e.g. one given via --config), not to the
// default BaseDir()/config.yaml, and that it preserves existing non-TUI keys.
func TestSaveTUIWritesLoadedConfigPath(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("BS_CONFIG_DIR", baseDir) // default location: must NOT be written

	custom := filepath.Join(t.TempDir(), "custom.yaml")
	yaml := `api:
  grpc_addr: "127.0.0.1:51234"
profiles:
  main:
    storage:
      db_path: "/tmp/bs-test-data"
active_profile: main
`
	if err := os.WriteFile(custom, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(custom)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	cfg.TUI = TUIConfig{VerticalSplit: 0.42, LeftHSplit: 0.3, RightHSplit: 0.7}
	if err := cfg.SaveTUI(); err != nil {
		t.Fatalf("SaveTUI() error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(baseDir, "config.yaml")); !os.IsNotExist(err) {
		t.Errorf("SaveTUI wrote to default path %s instead of loaded path", filepath.Join(baseDir, "config.yaml"))
	}

	re, err := Load(custom)
	if err != nil {
		t.Fatalf("re-Load() error: %v", err)
	}
	if re.TUI.VerticalSplit != 0.42 || re.TUI.LeftHSplit != 0.3 || re.TUI.RightHSplit != 0.7 {
		t.Errorf("TUI prefs not persisted to loaded path: got %+v", re.TUI)
	}
	if re.API.GRPCAddr != "127.0.0.1:51234" {
		t.Errorf("SaveTUI dropped api.grpc_addr: got %q", re.API.GRPCAddr)
	}
	if re.ActiveProfile != "main" {
		t.Errorf("SaveTUI dropped active_profile: got %q", re.ActiveProfile)
	}
	p, err := re.GetProfile("")
	if err != nil {
		t.Fatalf("GetProfile() after SaveTUI: %v", err)
	}
	if p.Storage.DBPath != "/tmp/bs-test-data" {
		t.Errorf("SaveTUI dropped profile db_path: got %q", p.Storage.DBPath)
	}
}

// TestEnvOverrideActiveProfile verifies that the documented BS_ACTIVE_PROFILE
// environment variable takes effect even when active_profile is absent from
// the config file (viper's AutomaticEnv only applies during Unmarshal to keys
// it already knows about).
func TestEnvOverrideActiveProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BS_CONFIG_DIR", dir)
	t.Setenv("BS_ACTIVE_PROFILE", "second")

	yaml := `profiles:
  first:
    storage:
      db_path: "/tmp/first"
  second:
    storage:
      db_path: "/tmp/second"
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.ActiveProfile != "second" {
		t.Fatalf("ActiveProfile = %q, want env override %q", cfg.ActiveProfile, "second")
	}
	p, err := cfg.GetProfile("")
	if err != nil {
		t.Fatalf("GetProfile() error: %v", err)
	}
	if p.Storage.DBPath != "/tmp/second" {
		t.Errorf("resolved profile DBPath = %q, want %q", p.Storage.DBPath, "/tmp/second")
	}
}

// TestSaveTUIConcurrent verifies that concurrent SaveTUI calls neither
// corrupt the config file nor drop non-TUI keys.
func TestSaveTUIConcurrent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BS_CONFIG_DIR", dir)

	yaml := `api:
  grpc_addr: "127.0.0.1:51235"
profiles:
  main:
    storage:
      db_path: "/tmp/bs-conc-data"
active_profile: main
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	cfg.TUI = TUIConfig{VerticalSplit: 0.5, LeftHSplit: 0.4, RightHSplit: 0.6}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := cfg.SaveTUI(); err != nil {
				t.Errorf("SaveTUI() error: %v", err)
			}
		}()
	}
	wg.Wait()

	re, err := Load("")
	if err != nil {
		t.Fatalf("re-Load() after concurrent SaveTUI: %v", err)
	}
	if re.API.GRPCAddr != "127.0.0.1:51235" {
		t.Errorf("concurrent SaveTUI dropped api.grpc_addr: got %q", re.API.GRPCAddr)
	}
	if re.ActiveProfile != "main" {
		t.Errorf("concurrent SaveTUI dropped active_profile: got %q", re.ActiveProfile)
	}
	if re.TUI.VerticalSplit != 0.5 || re.TUI.LeftHSplit != 0.4 || re.TUI.RightHSplit != 0.6 {
		t.Errorf("TUI prefs wrong after concurrent SaveTUI: got %+v", re.TUI)
	}
}

// TestNewProfileConfigHonorsOverride verifies that NewProfileConfig roots
// its default paths under $BS_CONFIG_DIR when set.
func TestNewProfileConfigHonorsOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BS_CONFIG_DIR", dir)

	p := NewProfileConfig("default")
	wantBase := filepath.Join(dir, "profiles", "default")
	if !strings.HasPrefix(p.Storage.DBPath, wantBase) {
		t.Errorf("NewProfileConfig DBPath = %q, want under %q", p.Storage.DBPath, wantBase)
	}
	if !strings.HasPrefix(p.Output.Path, wantBase) {
		t.Errorf("NewProfileConfig Output.Path = %q, want under %q", p.Output.Path, wantBase)
	}
}
