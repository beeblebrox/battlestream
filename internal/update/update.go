package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	checkInterval = 24 * time.Hour
	repoSlug      = "beeblebrox/battlestream"
	stateFile     = "update-state.yaml"
)

type ReleaseInfo struct {
	Version string `json:"tag_name" yaml:"version"`
	URL     string `json:"html_url" yaml:"url"`
}

type state struct {
	CheckedAt time.Time `yaml:"checked_at"`
	Latest    string    `yaml:"latest_version"`
	URL       string    `yaml:"release_url"`
}

// CheckResult is returned from CheckForUpdate.
type CheckResult struct {
	NewVersion string
	URL        string
}

// ShouldCheck returns true if an update check should be performed.
func ShouldCheck(stateDir string) bool {
	if os.Getenv("BS_NO_UPDATE_CHECK") != "" {
		return false
	}
	if os.Getenv("CI") != "" {
		return false
	}
	s, err := readState(stateDir)
	if err != nil {
		return true // no state file = never checked
	}
	return time.Since(s.CheckedAt) > checkInterval
}

// CheckForUpdate queries GitHub for the latest release and returns
// a result if a newer version is available.
func CheckForUpdate(stateDir, currentVersion string) (*CheckResult, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repoSlug)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api: %s", resp.Status)
	}

	var rel ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}

	// Save state regardless of version comparison.
	_ = writeState(stateDir, state{
		CheckedAt: time.Now(),
		Latest:    rel.Version,
		URL:       rel.URL,
	})

	if !isNewer(rel.Version, currentVersion) {
		return nil, nil
	}

	return &CheckResult{
		NewVersion: rel.Version,
		URL:        rel.URL,
	}, nil
}

// AssetName returns the expected release asset name for the current platform.
func AssetName(version string) string {
	goos := runtime.GOOS
	arch := runtime.GOARCH
	if goos == "darwin" {
		arch = "all" // universal binary
	}
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	v := strings.TrimPrefix(version, "v")
	return fmt.Sprintf("battlestream_%s_%s_%s.%s", v, goos, arch, ext)
}

func isNewer(latest, current string) bool {
	latest = strings.TrimPrefix(latest, "v")
	current = strings.TrimPrefix(current, "v")
	if current == "dev" || current == "" {
		return false
	}
	return latest != current && latest > current
}

func statePath(dir string) string {
	return filepath.Join(dir, stateFile)
}

func readState(dir string) (*state, error) {
	data, err := os.ReadFile(statePath(dir))
	if err != nil {
		return nil, err
	}
	var s state
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func writeState(dir string, s state) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(dir), data, 0o644)
}
