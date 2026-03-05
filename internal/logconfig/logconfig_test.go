package logconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEmptyFile(t *testing.T) {
	path := writeTmp(t, "")
	cfg, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(cfg.Sections))
	}
}

func TestParseNonExistent(t *testing.T) {
	cfg, err := Parse(filepath.Join(t.TempDir(), "missing.config"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sections) != 0 {
		t.Error("expected empty config for missing file")
	}
}

func TestParseExistingSections(t *testing.T) {
	content := `[Power]
LogLevel=1
FilePrinting=true

[Zone]
LogLevel=1
FilePrinting=false
`
	path := writeTmp(t, content)
	cfg, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(cfg.Sections))
	}
	power := cfg.findSection("Power")
	if power == nil {
		t.Fatal("Power section not found")
	}
	if power.Fields["LogLevel"] != "1" {
		t.Errorf("expected LogLevel=1, got %q", power.Fields["LogLevel"])
	}
	if power.Fields["FilePrinting"] != "true" {
		t.Errorf("expected FilePrinting=true, got %q", power.Fields["FilePrinting"])
	}
}

func TestParseIgnoresComments(t *testing.T) {
	content := `# this is a comment
; also a comment
[Power]
LogLevel=1
`
	cfg, err := Parse(writeTmp(t, content))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(cfg.Sections))
	}
}

func TestMergeAddsRequiredSections(t *testing.T) {
	cfg := &Config{}
	cfg.Merge()

	for _, name := range []string{"Power", "Zone", "Decks", "LoadingScreen"} {
		s := cfg.findSection(name)
		if s == nil {
			t.Errorf("required section %q not added by Merge", name)
			continue
		}
		if s.Fields["LogLevel"] != "1" {
			t.Errorf("[%s] LogLevel: expected 1, got %q", name, s.Fields["LogLevel"])
		}
		if s.Fields["FilePrinting"] != "true" {
			t.Errorf("[%s] FilePrinting: expected true, got %q", name, s.Fields["FilePrinting"])
		}
	}
}

func TestMergePatchesExistingSection(t *testing.T) {
	content := `[Power]
LogLevel=0
FilePrinting=false
`
	cfg, err := Parse(writeTmp(t, content))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Merge()

	power := cfg.findSection("Power")
	if power.Fields["LogLevel"] != "1" {
		t.Errorf("expected LogLevel patched to 1, got %q", power.Fields["LogLevel"])
	}
	if power.Fields["FilePrinting"] != "true" {
		t.Errorf("expected FilePrinting patched to true, got %q", power.Fields["FilePrinting"])
	}
}

func TestMergePreservesUserSections(t *testing.T) {
	content := `[MyCustomSection]
Foo=bar
`
	cfg, err := Parse(writeTmp(t, content))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Merge()

	custom := cfg.findSection("MyCustomSection")
	if custom == nil {
		t.Fatal("user section was removed by Merge")
	}
	if custom.Fields["Foo"] != "bar" {
		t.Errorf("user field corrupted: got %q", custom.Fields["Foo"])
	}
}

func TestWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.config")

	cfg := &Config{}
	cfg.Merge()

	if err := cfg.Write(path); err != nil {
		t.Fatal(err)
	}

	// Re-parse the written file
	cfg2, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"Power", "Zone", "Decks", "LoadingScreen"} {
		s := cfg2.findSection(name)
		if s == nil {
			t.Errorf("section %q missing after round-trip", name)
			continue
		}
		if s.Fields["LogLevel"] != "1" {
			t.Errorf("[%s] LogLevel after round-trip: %q", name, s.Fields["LogLevel"])
		}
		if s.Fields["FilePrinting"] != "true" {
			t.Errorf("[%s] FilePrinting after round-trip: %q", name, s.Fields["FilePrinting"])
		}
	}
}

func TestWriteAtomic(t *testing.T) {
	// The .tmp file must not be left behind after a successful write.
	dir := t.TempDir()
	path := filepath.Join(dir, "log.config")

	cfg := &Config{}
	cfg.Merge()
	if err := cfg.Write(path); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestEnsureVerboseLogging(t *testing.T) {
	path := writeTmp(t, "")
	if err := EnsureVerboseLogging(path); err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Power", "Zone", "Decks", "LoadingScreen"} {
		if s := cfg.findSection(name); s == nil {
			t.Errorf("section %q missing after EnsureVerboseLogging", name)
		}
	}
}

// writeTmp writes content to a temp file and returns its path.
func writeTmp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "logconfig*.config")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}
