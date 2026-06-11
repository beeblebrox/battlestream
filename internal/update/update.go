// Package update checks for new battlestream releases on GitHub.
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

	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

const (
	checkInterval = 24 * time.Hour
	repoSlug      = "beeblebrox/battlestream"
	stateFile     = "update-state.yaml"
	httpTimeout   = 10 * time.Second
)

// httpClient bounds the GitHub API request; the default client has no timeout.
var httpClient = &http.Client{Timeout: httpTimeout}

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
	resp, err := httpClient.Get(url)
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

// isNewer reports whether latest is a strictly newer semantic version than
// current. Inputs may or may not carry a leading "v". Equal versions are not
// newer, and per semver a pre-release (e.g. 0.6.1-beta) is lower than its
// release (0.6.1).
func isNewer(latest, current string) bool {
	if current == "dev" || current == "" {
		return false
	}
	// semver.Compare requires a leading "v"; normalize both inputs.
	l := "v" + strings.TrimPrefix(latest, "v")
	c := "v" + strings.TrimPrefix(current, "v")
	if !semver.IsValid(l) || !semver.IsValid(c) {
		return false
	}
	return semver.Compare(l, c) > 0
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
