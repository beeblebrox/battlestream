// Package logconfig manages the Hearthstone log.config file.
package logconfig

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Section represents one [Name] block in log.config.
type Section struct {
	Name   string
	Fields map[string]string
}

// Config is the parsed representation of log.config.
type Config struct {
	Sections []*Section
}

// required sections and their values for verbose BG logging.
var requiredSections = []*Section{
	{
		Name: "Power",
		Fields: map[string]string{
			"LogLevel":        "1",
			"FilePrinting":    "true",
			"ConsolePrinting": "false",
			"ScreenPrinting":  "false",
		},
	},
	{
		Name: "Zone",
		Fields: map[string]string{
			"LogLevel":        "1",
			"FilePrinting":    "true",
			"ConsolePrinting": "false",
			"ScreenPrinting":  "false",
		},
	},
	{
		Name: "Decks",
		Fields: map[string]string{
			"LogLevel":        "1",
			"FilePrinting":    "true",
			"ConsolePrinting": "false",
			"ScreenPrinting":  "false",
		},
	},
	{
		Name: "LoadingScreen",
		Fields: map[string]string{
			"LogLevel":        "1",
			"FilePrinting":    "true",
			"ConsolePrinting": "false",
			"ScreenPrinting":  "false",
		},
	},
}

// Parse reads and parses a log.config file.
func Parse(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("opening log.config: %w", err)
	}
	defer f.Close()

	cfg := &Config{}
	var current *Section

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := line[1 : len(line)-1]
			current = &Section{Name: name, Fields: make(map[string]string)}
			cfg.Sections = append(cfg.Sections, current)
			continue
		}
		if current == nil {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			current.Fields[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return cfg, scanner.Err()
}

// Merge ensures required sections exist with correct values.
// Existing user sections not in requiredSections are preserved.
func (c *Config) Merge() {
	for _, req := range requiredSections {
		existing := c.findSection(req.Name)
		if existing == nil {
			// Add the section
			s := &Section{Name: req.Name, Fields: make(map[string]string)}
			for k, v := range req.Fields {
				s.Fields[k] = v
			}
			c.Sections = append(c.Sections, s)
		} else {
			// Patch only the required keys
			for k, v := range req.Fields {
				existing.Fields[k] = v
			}
		}
	}
}

// Write serializes the config and atomically writes it to path.
func (c *Config) Write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating log.config directory: %w", err)
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating temp log.config: %w", err)
	}

	w := bufio.NewWriter(f)
	for i, s := range c.Sections {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "[%s]\n", s.Name)
		// Write required fields in deterministic order
		for _, k := range sectionKeyOrder(s.Name) {
			if v, ok := s.Fields[k]; ok {
				fmt.Fprintf(w, "%s=%s\n", k, v)
			}
		}
		// Write any extra fields the user might have added
		for k, v := range s.Fields {
			if !isRequiredKey(k) {
				fmt.Fprintf(w, "%s=%s\n", k, v)
			}
		}
	}

	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// IsComplete reports whether the config already contains all required sections
// with the correct field values. If true, EnsureVerboseLogging skips the write.
func (c *Config) IsComplete() bool {
	for _, req := range requiredSections {
		s := c.findSection(req.Name)
		if s == nil {
			return false
		}
		for k, v := range req.Fields {
			if s.Fields[k] != v {
				return false
			}
		}
	}
	return true
}

// EnsureVerboseLogging parses, merges required sections, and writes back.
// If the config is already complete the write is skipped.
func EnsureVerboseLogging(path string) error {
	cfg, err := Parse(path)
	if err != nil {
		return err
	}
	if cfg.IsComplete() {
		return nil
	}
	cfg.Merge()
	return cfg.Write(path)
}

func (c *Config) findSection(name string) *Section {
	for _, s := range c.Sections {
		if strings.EqualFold(s.Name, name) {
			return s
		}
	}
	return nil
}

func sectionKeyOrder(name string) []string {
	return []string{"LogLevel", "FilePrinting", "ConsolePrinting", "ScreenPrinting"}
}

func isRequiredKey(k string) bool {
	for _, req := range []string{"LogLevel", "FilePrinting", "ConsolePrinting", "ScreenPrinting"} {
		if strings.EqualFold(k, req) {
			return true
		}
	}
	return false
}
