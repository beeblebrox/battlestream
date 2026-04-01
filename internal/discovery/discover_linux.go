package discovery

import (
	"bufio"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// discoverMounts reads r (expected to contain /proc/mounts content) and
// returns the mount-point paths for "real" filesystems, filtering out
// pseudo-filesystems and anything that won't contain a game install.
func discoverMounts(r io.Reader) []string {
	// Filesystems we consider "real" (could hold user data).
	realFS := map[string]bool{
		"ext4":  true,
		"ext3":  true,
		"ext2":  true,
		"btrfs": true,
		"xfs":   true,
		"ntfs":  true,
		"ntfs3": true,
		"exfat": true,
		"vfat":  true,
		"f2fs":  true,
	}

	var mounts []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		// /proc/mounts format: <device> <mountpoint> <fstype> <options> <dump> <pass>
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		mountPoint := fields[1]
		fsType := fields[2]

		if mountPoint == "/" {
			// The root filesystem is already covered by the default wine/proton roots.
			continue
		}
		if !realFS[fsType] {
			continue
		}
		mounts = append(mounts, mountPoint)
	}
	return mounts
}

// discoverMountsFromFile reads /proc/mounts and returns mount-point paths for
// real filesystems. Errors are logged and an empty slice is returned.
func discoverMountsFromFile() []string {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		log.Printf("discovery: cannot read /proc/mounts: %v", err)
		return nil
	}
	defer f.Close()
	return discoverMounts(f)
}

// steamLibraryFolders parses a Steam libraryfolders.vdf reader and returns
// all Steam library root paths listed in it.
//
// libraryfolders.vdf has a simple text format:
//
//	"LibraryFolders"
//	{
//	    "contentstatsid" "..."
//	    "1"
//	    {
//	        "path"  "/mnt/games/steam"
//	        ...
//	    }
//	    ...
//	}
//
// We extract any line whose key is "path".
func steamLibraryFolders(r io.Reader) []string {
	var paths []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Match lines like: "path"	"/some/directory"
		if !strings.HasPrefix(line, `"path"`) {
			continue
		}
		// Remove the key token and extract the value token.
		rest := strings.TrimPrefix(line, `"path"`)
		rest = strings.TrimSpace(rest)
		if len(rest) < 2 || rest[0] != '"' {
			continue
		}
		// Strip surrounding quotes.
		rest = rest[1:]
		end := strings.Index(rest, `"`)
		if end < 0 {
			continue
		}
		p := rest[:end]
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// steamLibraryFoldersFromDisk reads Steam's libraryfolders.vdf from the two
// known locations and returns all library paths found. Missing files are
// silently skipped.
func steamLibraryFoldersFromDisk(home string) []string {
	candidates := []string{
		filepath.Join(home, ".local", "share", "Steam", "config", "libraryfolders.vdf"),
		filepath.Join(home, ".steam", "steam", "config", "libraryfolders.vdf"),
	}

	seen := make(map[string]bool)
	var all []string
	for _, vdf := range candidates {
		f, err := os.Open(vdf)
		if err != nil {
			continue
		}
		paths := steamLibraryFolders(f)
		f.Close()
		for _, p := range paths {
			if !seen[p] {
				seen[p] = true
				all = append(all, p)
			}
		}
	}
	return all
}

// linuxExtraRoots returns additional Linux-specific search roots beyond the
// default wine/proton paths.
func linuxExtraRoots(home string) []string {
	var roots []string

	// 1. Mount-point-based roots: append common HS sub-paths under each mount.
	const hsAppID = "1463140"
	for _, mp := range discoverMountsFromFile() {
		roots = append(roots,
			filepath.Join(mp, "Hearthstone"),
			filepath.Join(mp, "games", "Hearthstone"),
			filepath.Join(mp, "steamapps", "common", "Hearthstone"),
			filepath.Join(mp, "steamapps", "compatdata", hsAppID, "pfx"),
		)
	}

	// 2. Steam library folders (non-default Steam libraries).
	for _, lib := range steamLibraryFoldersFromDisk(home) {
		roots = append(roots,
			filepath.Join(lib, "steamapps", "common", "Hearthstone"),
			filepath.Join(lib, "steamapps", "compatdata", hsAppID, "pfx"),
		)
	}

	// 3. /run/media/$USER/* — common automount point for removable drives.
	user := filepath.Base(home)
	roots = append(roots, filepath.Join("/run/media", user))

	// 4. /mnt/* — common manual mount point.
	roots = append(roots, "/mnt")

	return roots
}
