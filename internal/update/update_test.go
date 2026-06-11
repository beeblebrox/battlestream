package update

import (
	"strings"
	"testing"
	"time"
)

func TestShouldCheck_NoState(t *testing.T) {
	t.Setenv("CI", "")
	dir := t.TempDir()
	if !ShouldCheck(dir) {
		t.Error("expected true when no state file exists")
	}
}

func TestShouldCheck_RecentCheck(t *testing.T) {
	t.Setenv("CI", "")
	dir := t.TempDir()
	_ = writeState(dir, state{CheckedAt: time.Now()})
	if ShouldCheck(dir) {
		t.Error("expected false when checked recently")
	}
}

func TestShouldCheck_StaleCheck(t *testing.T) {
	t.Setenv("CI", "")
	dir := t.TempDir()
	_ = writeState(dir, state{CheckedAt: time.Now().Add(-25 * time.Hour)})
	if !ShouldCheck(dir) {
		t.Error("expected true when last check >24h ago")
	}
}

func TestShouldCheck_EnvDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BS_NO_UPDATE_CHECK", "1")
	if ShouldCheck(dir) {
		t.Error("expected false when BS_NO_UPDATE_CHECK set")
	}
}

func TestShouldCheck_CI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CI", "true")
	if ShouldCheck(dir) {
		t.Error("expected false in CI")
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"v0.14.0", "v0.13.0", true},
		{"v0.13.0", "v0.13.0", false},
		{"v0.12.0", "v0.13.0", false},
		{"v0.14.0-beta", "v0.13.0-beta", true},
		{"v0.14.0", "dev", false},
		{"v0.14.0", "", false},
		// Multi-digit components must compare numerically, not lexically.
		{"0.10.0", "0.9.0", true},
		{"0.9.0", "0.10.0", false},
		{"1.2.0", "1.10.0", false},
		{"1.10.0", "1.2.0", true},
		// Pre-release versions are LOWER than the corresponding release.
		{"0.6.1-beta", "0.6.1", false},
		{"0.6.1", "0.6.1-beta", true},
		// Equal versions are not newer.
		{"0.6.1", "0.6.1", false},
		{"0.6.1-beta", "0.6.1-beta", false},
		// v-prefixed inputs (mixed and matched).
		{"v0.10.0", "0.9.0", true},
		{"0.9.0", "v0.10.0", false},
		{"v0.6.1-beta", "v0.6.1", false},
		{"v0.6.1", "v0.6.1-beta", true},
		{"v0.6.1", "v0.6.1", false},
	}
	for _, tt := range tests {
		if got := isNewer(tt.latest, tt.current); got != tt.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	name := AssetName("v0.14.0-beta")
	if name == "" {
		t.Fatal("empty asset name")
	}
	if !strings.Contains(name, "0.14.0-beta") {
		t.Errorf("asset name %q missing version", name)
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := state{
		CheckedAt: time.Now().Truncate(time.Second),
		Latest:    "v0.14.0",
		URL:       "https://example.com",
	}
	if err := writeState(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := readState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Latest != want.Latest || got.URL != want.URL {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
