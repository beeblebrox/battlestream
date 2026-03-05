// Package discovery finds Hearthstone installations across platforms.
package discovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// InstallInfo describes a found Hearthstone installation.
type InstallInfo struct {
	InstallRoot string // root of the HS install
	LogPath     string // path to the Logs directory (where Power.log lives)
	LogConfig   string // path to log.config
}

// errStopWalk is used as a sentinel to abort WalkDir early.
var errStopWalk = errors.New("stop walk")

// Discover attempts to find the Hearthstone installation automatically.
// Returns the first match found, or an error if nothing is found.
func Discover() (*InstallInfo, error) {
	for _, root := range searchRoots() {
		if info, err := DiscoverFromRoot(root); err == nil {
			return info, nil
		}
	}
	return nil, fmt.Errorf("hearthstone installation not found; run 'battlestream discover' to set it manually")
}

// DiscoverFromRoot probes a specific path. It accepts three forms:
//  1. A Hearthstone install directory (contains Hearthstone.exe, Hearthstone.app, or Logs/)
//  2. A Logs directory (contains Power.log)
//  3. A Wine/Proton prefix root (contains drive_c/) — walks drive_c/ for an install
func DiscoverFromRoot(root string) (*InstallInfo, error) {
	// Direct probe: Hearthstone install or Logs directory.
	if info, err := probeRoot(root); err == nil {
		return info, nil
	}
	// Wine/Proton prefix: if root contains drive_c/, scan inside it.
	driveC := filepath.Join(root, "drive_c")
	if _, err := os.Stat(driveC); err == nil {
		return WalkForInstall(driveC)
	}
	return nil, fmt.Errorf("not a hearthstone install or wine/proton prefix: %s", root)
}

// searchRoots returns platform-appropriate candidate directories to search.
func searchRoots() []string {
	home, _ := os.UserHomeDir()

	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\Program Files (x86)\Hearthstone`,
			`C:\Program Files\Hearthstone`,
			`C:\Program Files (x86)\Battle.net\Hearthstone`,
			`C:\Program Files\Battle.net\Hearthstone`,
		}
	case "darwin":
		return []string{
			"/Applications/Hearthstone",
			filepath.Join(home, "Applications", "Hearthstone"),
		}
	default:
		var roots []string
		roots = append(roots, wineRoots(home)...)
		roots = append(roots, protonRoots(home)...)
		roots = append(roots, filepath.Join(home, ".var", "app"))
		return roots
	}
}

func wineRoots(home string) []string {
	// Return the Wine prefix root so DiscoverFromRoot can find drive_c/ inside.
	return []string{
		filepath.Join(home, ".wine"),
	}
}

func protonRoots(home string) []string {
	const hsAppID = "1463140"
	// Return the Proton pfx root so DiscoverFromRoot can find drive_c/ inside.
	return []string{
		filepath.Join(home, ".local", "share", "Steam", "steamapps", "common", "Hearthstone"),
		filepath.Join(home, ".steam", "steam", "steamapps", "common", "Hearthstone"),
		filepath.Join(home, ".local", "share", "Steam", "steamapps", "compatdata", hsAppID, "pfx"),
		filepath.Join(home, ".steam", "steam", "steamapps", "compatdata", hsAppID, "pfx"),
	}
}

// probeRoot checks whether a directory looks like a Hearthstone install and
// returns a filled InstallInfo if it does.
func probeRoot(root string) (*InstallInfo, error) {
	// Accept if the root contains the exe, app bundle, or Logs directory.
	for _, c := range []string{
		filepath.Join(root, "Hearthstone.exe"),
		filepath.Join(root, "Hearthstone.app"),
		filepath.Join(root, "Logs"),
	} {
		if _, err := os.Stat(c); err == nil {
			return &InstallInfo{
				InstallRoot: root,
				LogPath:     filepath.Join(root, "Logs"),
				LogConfig:   logConfigPath(root),
			}, nil
		}
	}

	// Accept if root itself is the Logs directory (contains Power.log).
	if _, err := os.Stat(filepath.Join(root, "Power.log")); err == nil {
		parent := filepath.Dir(root)
		return &InstallInfo{
			InstallRoot: parent,
			LogPath:     root,
			LogConfig:   logConfigPath(parent),
		}, nil
	}

	return nil, fmt.Errorf("not a hearthstone install: %s", root)
}

// logConfigPath returns the platform-appropriate log.config path given an
// install root. On Linux it detects Wine/Proton prefixes by looking for
// drive_c in the path, so arbitrary prefix locations like /chungus/battlenet
// are handled correctly.
func logConfigPath(installRoot string) string {
	home, _ := os.UserHomeDir()

	switch runtime.GOOS {
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "Blizzard", "Hearthstone", "log.config")

	case "darwin":
		return filepath.Join(home, "Library", "Preferences", "Blizzard", "Hearthstone", "log.config")

	default:
		// Detect any Wine/Proton prefix by finding /drive_c/ in the path.
		if prefix, ok := extractWinePrefix(installRoot); ok {
			return logConfigInPrefix(prefix, home)
		}
		// Fallback: default Wine prefix.
		return logConfigInPrefix(filepath.Join(home, ".wine"), home)
	}
}

// extractWinePrefix returns the Wine/Proton prefix root if the given path
// is inside a drive_c hierarchy. Examples:
//
//	/chungus/battlenet/drive_c/Program Files/Hearthstone  →  /chungus/battlenet
//	~/.wine/drive_c/Program Files (x86)/Hearthstone       →  ~/.wine
//	/pfx/drive_c/users/steamuser/...                      →  /pfx
func extractWinePrefix(path string) (string, bool) {
	sep := string(filepath.Separator)
	marker := sep + "drive_c" + sep
	if idx := strings.Index(path, marker); idx >= 0 {
		return path[:idx], true
	}
	// Path ends with /drive_c (no trailing separator).
	if strings.HasSuffix(path, sep+"drive_c") {
		return strings.TrimSuffix(path, sep+"drive_c"), true
	}
	return "", false
}

// logConfigInPrefix returns the Blizzard log.config path inside a Wine/Proton
// prefix. It checks whether a users directory exists to pick the right
// username (host username for Wine, "steamuser" for Proton).
func logConfigInPrefix(prefixRoot, home string) string {
	usersDir := filepath.Join(prefixRoot, "drive_c", "users")
	tail := filepath.Join("AppData", "Local", "Blizzard", "Hearthstone", "log.config")

	for _, user := range []string{filepath.Base(home), "steamuser"} {
		if _, err := os.Stat(filepath.Join(usersDir, user)); err == nil {
			return filepath.Join(usersDir, user, tail)
		}
	}
	// Neither user directory exists yet; default to host username.
	return filepath.Join(usersDir, filepath.Base(home), tail)
}

// WalkForInstall walks startDir looking for a Hearthstone install.
// Returns the first match found, or an error if nothing is found.
func WalkForInstall(startDir string) (*InstallInfo, error) {
	var found *InstallInfo
	err := filepath.WalkDir(startDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		info, e := probeRoot(path)
		if e == nil {
			found = info
			return errStopWalk
		}
		return nil
	})
	if found != nil {
		return found, nil
	}
	if err != nil && !errors.Is(err, errStopWalk) {
		return nil, err
	}
	return nil, fmt.Errorf("no hearthstone install found under %s", startDir)
}

// WalkForAllInstalls walks startDir and returns every Hearthstone install found.
// Unlike WalkForInstall it does not stop at the first match.
func WalkForAllInstalls(startDir string) ([]*InstallInfo, error) {
	var all []*InstallInfo
	err := filepath.WalkDir(startDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		info, e := probeRoot(path)
		if e == nil {
			all = append(all, info)
			// Don't recurse inside a found install directory.
			return filepath.SkipDir
		}
		return nil
	})
	return all, err
}

// DiscoverAll scans all platform-default search roots and returns every
// Hearthstone install found. Unlike Discover it does not stop at the first match.
func DiscoverAll() ([]*InstallInfo, error) {
	seen := make(map[string]bool)
	var all []*InstallInfo
	for _, root := range searchRoots() {
		infos := discoverAllFromRoot(root)
		for _, info := range infos {
			if !seen[info.InstallRoot] {
				seen[info.InstallRoot] = true
				all = append(all, info)
			}
		}
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no hearthstone installations found in default search paths")
	}
	return all, nil
}

// discoverAllFromRoot returns all installs under a single root path.
func discoverAllFromRoot(root string) []*InstallInfo {
	if info, err := probeRoot(root); err == nil {
		return []*InstallInfo{info}
	}
	driveC := filepath.Join(root, "drive_c")
	if _, err := os.Stat(driveC); err == nil {
		all, _ := WalkForAllInstalls(driveC)
		return all
	}
	return nil
}
