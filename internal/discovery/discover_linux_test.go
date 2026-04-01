package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleProcMounts is a representative /proc/mounts snippet.
const sampleProcMounts = `sysfs /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0
tmpfs /tmp tmpfs rw,nosuid,nodev 0 0
/dev/sda1 / ext4 rw,relatime 0 0
/dev/sda2 /home ext4 rw,relatime 0 0
/dev/sdb1 /mnt/games ext4 rw,relatime 0 0
/dev/nvme0n1p1 /data btrfs rw,relatime 0 0
/dev/sdc1 /run/media/alice/usb ntfs rw,relatime 0 0
overlay /var/lib/docker/overlay2 overlay rw,relatime 0 0
`

func TestDiscoverMountsFiltersRealFS(t *testing.T) {
	r := strings.NewReader(sampleProcMounts)
	got := discoverMounts(r)

	// Should include the real filesystems, excluding /, sysfs, proc, tmpfs, overlay.
	want := []string{"/home", "/mnt/games", "/data", "/run/media/alice/usb"}
	if len(got) != len(want) {
		t.Fatalf("discoverMounts: got %v, want %v", got, want)
	}
	for i, p := range want {
		if got[i] != p {
			t.Errorf("discoverMounts[%d]: got %q, want %q", i, got[i], p)
		}
	}
}

func TestDiscoverMountsEmptyInput(t *testing.T) {
	r := strings.NewReader("")
	got := discoverMounts(r)
	if len(got) != 0 {
		t.Errorf("expected empty slice for empty input, got %v", got)
	}
}

func TestDiscoverMountsMalformedLines(t *testing.T) {
	input := "only_one_field\n\n  \n/dev/sda1 /mnt ext4 rw 0 0\n"
	r := strings.NewReader(input)
	got := discoverMounts(r)
	if len(got) != 1 || got[0] != "/mnt" {
		t.Errorf("discoverMounts with malformed lines: got %v, want [/mnt]", got)
	}
}

// sampleVDF is a minimal libraryfolders.vdf covering both legacy and modern formats.
const sampleVDF = `"LibraryFolders"
{
	"contentstatsid"		"-1234567890"
	"1"
	{
		"path"		"/mnt/games/steam"
		"label"		""
		"contentid"		"1234"
		"totalsize"		"0"
		"update_clean_bytes_tally"		"0"
		"time_last_update_corruption"		"0"
		"apps"
		{
			"1463140"		"5234567890"
		}
	}
	"2"
	{
		"path"		"/run/media/alice/extern/steam"
		"label"		"extern"
		"contentid"		"5678"
		"totalsize"		"0"
		"update_clean_bytes_tally"		"0"
		"time_last_update_corruption"		"0"
		"apps"
		{
		}
	}
}
`

func TestSteamLibraryFolders(t *testing.T) {
	r := strings.NewReader(sampleVDF)
	got := steamLibraryFolders(r)

	want := []string{"/mnt/games/steam", "/run/media/alice/extern/steam"}
	if len(got) != len(want) {
		t.Fatalf("steamLibraryFolders: got %v, want %v", got, want)
	}
	for i, p := range want {
		if got[i] != p {
			t.Errorf("steamLibraryFolders[%d]: got %q, want %q", i, got[i], p)
		}
	}
}

func TestSteamLibraryFoldersEmpty(t *testing.T) {
	r := strings.NewReader(`"LibraryFolders" {}`)
	got := steamLibraryFolders(r)
	if len(got) != 0 {
		t.Errorf("expected empty slice for VDF with no paths, got %v", got)
	}
}

func TestSteamLibraryFoldersFromDisk(t *testing.T) {
	home := t.TempDir()

	// Write a VDF at the first candidate location.
	cfgDir := filepath.Join(home, ".local", "share", "Steam", "config")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	vdfPath := filepath.Join(cfgDir, "libraryfolders.vdf")
	if err := os.WriteFile(vdfPath, []byte(sampleVDF), 0644); err != nil {
		t.Fatal(err)
	}

	got := steamLibraryFoldersFromDisk(home)
	if len(got) == 0 {
		t.Error("expected at least one library path from disk, got none")
	}
	found := false
	for _, p := range got {
		if p == "/mnt/games/steam" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected /mnt/games/steam in results, got %v", got)
	}
}

func TestSteamLibraryFoldersFromDiskMissing(t *testing.T) {
	// No VDF files written — should return empty without error.
	home := t.TempDir()
	got := steamLibraryFoldersFromDisk(home)
	if len(got) != 0 {
		t.Errorf("expected empty when no VDF present, got %v", got)
	}
}

func TestSteamLibraryFoldersFromDiskDeduplicates(t *testing.T) {
	// Write the same VDF to both candidate locations; paths should be deduplicated.
	home := t.TempDir()
	for _, sub := range []string{
		filepath.Join(home, ".local", "share", "Steam", "config"),
		filepath.Join(home, ".steam", "steam", "config"),
	} {
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "libraryfolders.vdf"), []byte(sampleVDF), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got := steamLibraryFoldersFromDisk(home)
	// sampleVDF has 2 paths; even with 2 files, we want exactly 2 unique paths.
	if len(got) != 2 {
		t.Errorf("expected 2 deduplicated paths, got %d: %v", len(got), got)
	}
}

func TestLinuxExtraRootsIncludesStaticPaths(t *testing.T) {
	home := t.TempDir()
	user := filepath.Base(home)

	roots := linuxExtraRoots(home)

	// Should always include /run/media/<user> and /mnt.
	wantContains := []string{
		filepath.Join("/run/media", user),
		"/mnt",
	}
	for _, want := range wantContains {
		found := false
		for _, r := range roots {
			if r == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("linuxExtraRoots: expected %q in roots, got %v", want, roots)
		}
	}
}

func TestLinuxExtraRootsIncludesSteamLibrary(t *testing.T) {
	home := t.TempDir()

	// Provide a VDF with one library path.
	cfgDir := filepath.Join(home, ".local", "share", "Steam", "config")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	vdf := "\"LibraryFolders\"\n{\n\t\"1\"\n\t{\n\t\t\"path\"\t\"/extra/steam\"\n\t}\n}\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "libraryfolders.vdf"), []byte(vdf), 0644); err != nil {
		t.Fatal(err)
	}

	roots := linuxExtraRoots(home)

	wantContains := filepath.Join("/extra/steam", "steamapps", "common", "Hearthstone")
	found := false
	for _, r := range roots {
		if r == wantContains {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("linuxExtraRoots: expected %q in roots, got %v", wantContains, roots)
	}
}
