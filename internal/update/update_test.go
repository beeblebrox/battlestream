package update

import (
	"strings"
	"testing"
	"time"
)

func TestShouldCheck_NoState(t *testing.T) {
	dir := t.TempDir()
	if !ShouldCheck(dir) {
		t.Error("expected true when no state file exists")
	}
}

func TestShouldCheck_RecentCheck(t *testing.T) {
	dir := t.TempDir()
	_ = writeState(dir, state{CheckedAt: time.Now()})
	if ShouldCheck(dir) {
		t.Error("expected false when checked recently")
	}
}

func TestShouldCheck_StaleCheck(t *testing.T) {
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
