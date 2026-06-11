package config

import (
	"os"
	"path/filepath"
	"strings"
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
