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
	roots := searchRoots()
	for _, root := range roots {
		info, err := probeRoot(root)
		if err == nil {
			return info, nil
		}
	}
	return nil, fmt.Errorf("hearthstone installation not found; run 'battlestream discover' to set it manually")
}

// DiscoverFromRoot probes a specific path as the HS install root.
func DiscoverFromRoot(root string) (*InstallInfo, error) {
	return probeRoot(root)
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
		// Linux: Wine, Proton, Flatpak
		var roots []string
		roots = append(roots, wineRoots(home)...)
		roots = append(roots, protonRoots(home)...)
		roots = append(roots, filepath.Join(home, ".var", "app"))
		return roots
	}
}

func wineRoots(home string) []string {
	return []string{
		filepath.Join(home, ".wine", "drive_c", "Program Files (x86)", "Hearthstone"),
		filepath.Join(home, ".wine", "drive_c", "Program Files", "Hearthstone"),
	}
}

func protonRoots(home string) []string {
	const hsAppID = "1463140"
	return []string{
		filepath.Join(home, ".local", "share", "Steam", "steamapps", "common", "Hearthstone"),
		filepath.Join(home, ".steam", "steam", "steamapps", "common", "Hearthstone"),
		filepath.Join(home, ".local", "share", "Steam", "steamapps", "compatdata", hsAppID),
		filepath.Join(home, ".steam", "steam", "steamapps", "compatdata", hsAppID),
	}
}

// probeRoot checks whether a directory looks like a Hearthstone install and
// returns a filled InstallInfo if it does.
func probeRoot(root string) (*InstallInfo, error) {
	// Accept if the root contains the exe, app bundle, or Logs directory.
	candidates := []string{
		filepath.Join(root, "Hearthstone.exe"),
		filepath.Join(root, "Hearthstone.app"),
		filepath.Join(root, "Logs"),
	}
	for _, c := range candidates {
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
		return &InstallInfo{
			InstallRoot: filepath.Dir(root),
			LogPath:     root,
			LogConfig:   logConfigPath(filepath.Dir(root)),
		}, nil
	}

	return nil, fmt.Errorf("not a hearthstone install: %s", root)
}

// logConfigPath returns the platform-appropriate log.config path given an install root.
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
		// Detect Wine prefix from install root path.
		if idx := strings.Index(installRoot, ".wine"); idx >= 0 {
			winePrefix := installRoot[:idx+len(".wine")]
			username := filepath.Base(home)
			return filepath.Join(winePrefix, "drive_c", "users", username,
				"AppData", "Local", "Blizzard", "Hearthstone", "log.config")
		}
		// Detect Proton prefix.
		if idx := strings.Index(installRoot, "compatdata"); idx >= 0 {
			rest := installRoot[idx+len("compatdata"):]
			parts := strings.SplitN(strings.TrimPrefix(rest, string(os.PathSeparator)), string(os.PathSeparator), 2)
			if len(parts) > 0 {
				pfxBase := filepath.Join(installRoot[:idx+len("compatdata")], parts[0], "pfx")
				return filepath.Join(pfxBase, "drive_c", "users", "steamuser",
					"AppData", "Local", "Blizzard", "Hearthstone", "log.config")
			}
		}
		// Fallback: default Wine prefix.
		username := filepath.Base(home)
		return filepath.Join(home, ".wine", "drive_c", "users", username,
			"AppData", "Local", "Blizzard", "Hearthstone", "log.config")
	}
}

// WalkForInstall walks startDir looking for a Hearthstone install.
// Used by the interactive discovery wizard as a last resort.
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
