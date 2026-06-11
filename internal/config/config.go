// Package config manages battlestream configuration via Viper.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Config holds all battlestream configuration.
// API and logging settings are global; per-install settings live in Profiles.
type Config struct {
	ActiveProfile string                   `yaml:"active_profile,omitempty" mapstructure:"active_profile"`
	Profiles      map[string]*ProfileConfig `yaml:"profiles,omitempty" mapstructure:"profiles"`
	API           APIConfig                 `yaml:"api" mapstructure:"api"`
	Logging       LoggingConfig             `yaml:"logging" mapstructure:"logging"`
	TUI           TUIConfig                 `yaml:"tui,omitempty" mapstructure:"tui"`

	// loadedPath is the config file this Config was loaded from (empty when
	// no config file was found). SaveTUI writes back to this file so that a
	// process started with --config /custom/path.yaml updates the right file.
	loadedPath string
}

// TUIConfig holds TUI layout preferences.
type TUIConfig struct {
	VerticalSplit   float64 `yaml:"vertical_split,omitempty" mapstructure:"vertical_split"`
	HorizontalSplit float64 `yaml:"horizontal_split,omitempty" mapstructure:"horizontal_split"`
	LeftHSplit      float64 `yaml:"left_hsplit,omitempty" mapstructure:"left_hsplit"`
	RightHSplit     float64 `yaml:"right_hsplit,omitempty" mapstructure:"right_hsplit"`
}

// ProfileConfig groups the settings that differ between Hearthstone installs.
type ProfileConfig struct {
	Hearthstone HearthstoneConfig `yaml:"hearthstone" mapstructure:"hearthstone"`
	Storage     StorageConfig     `yaml:"storage" mapstructure:"storage"`
	Output      OutputConfig      `yaml:"output" mapstructure:"output"`
}

type HearthstoneConfig struct {
	InstallPath        string `yaml:"install_path,omitempty" mapstructure:"install_path"`
	LogPath            string `yaml:"log_path,omitempty" mapstructure:"log_path"`
	AutoPatchLogConfig bool   `yaml:"auto_patch_logconfig" mapstructure:"auto_patch_logconfig"`
}

type StorageConfig struct {
	DBPath string `yaml:"db_path" mapstructure:"db_path"`
}

type OutputConfig struct {
	Enabled         bool   `yaml:"enabled" mapstructure:"enabled"`
	Path            string `yaml:"path" mapstructure:"path"`
	WriteIntervalMs int    `yaml:"write_interval_ms" mapstructure:"write_interval_ms"`
}

type APIConfig struct {
	GRPCAddr string `yaml:"grpc_addr" mapstructure:"grpc_addr"`
	RESTAddr string `yaml:"rest_addr" mapstructure:"rest_addr"`
	APIKey   string `yaml:"api_key,omitempty" mapstructure:"api_key"`
}

type LoggingConfig struct {
	Level string `yaml:"level" mapstructure:"level"`
	File  string `yaml:"file,omitempty" mapstructure:"file"`
}

// GetProfile resolves which profile to use.
//   - name="" + 1 profile  → returns that profile
//   - name="" + N profiles → uses active_profile; error if unset
//   - name="" + 0 profiles → returns a temporary default (no-config case)
//   - name set             → returns named profile or error
func (c *Config) GetProfile(name string) (*ProfileConfig, error) {
	if len(c.Profiles) == 0 {
		return NewProfileConfig("default"), nil
	}
	if name == "" {
		if len(c.Profiles) == 1 {
			for _, p := range c.Profiles {
				return p, nil
			}
		}
		name = c.ActiveProfile
		if name == "" {
			return nil, fmt.Errorf("multiple profiles configured, specify one with --profile\nAvailable: %s", c.ProfileList())
		}
	}
	p, ok := c.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found\nAvailable: %s", name, c.ProfileList())
	}
	return p, nil
}

// AddProfile adds or replaces a named profile.
// If setActive is true, or no active profile is set yet, this profile becomes active.
func (c *Config) AddProfile(name string, p *ProfileConfig, setActive bool) {
	if c.Profiles == nil {
		c.Profiles = make(map[string]*ProfileConfig)
	}
	c.Profiles[name] = p
	if setActive || c.ActiveProfile == "" {
		c.ActiveProfile = name
	}
}

// ProfileList returns a comma-separated list of profile names, sorted.
func (c *Config) ProfileList() string {
	names := make([]string, 0, len(c.Profiles))
	for k := range c.Profiles {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// BaseDir returns the battlestream base directory. If the BS_CONFIG_DIR
// environment variable is set, it takes precedence (used by
// scripts/safe-test.sh and other isolation scenarios); otherwise the
// default is ~/.battlestream.
func BaseDir() string {
	if dir := os.Getenv("BS_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".battlestream"
	}
	return filepath.Join(home, ".battlestream")
}

// NewProfileConfig returns a ProfileConfig with sensible path defaults for the given name.
func NewProfileConfig(name string) *ProfileConfig {
	base := filepath.Join(BaseDir(), "profiles", name)
	return &ProfileConfig{
		Hearthstone: HearthstoneConfig{
			AutoPatchLogConfig: true,
		},
		Storage: StorageConfig{
			DBPath: filepath.Join(base, "data"),
		},
		Output: OutputConfig{
			Enabled:         true,
			Path:            filepath.Join(base, "stats"),
			WriteIntervalMs: 500,
		},
	}
}

// Load reads config from file and environment variables.
// Config file is searched in: BaseDir() ($BS_CONFIG_DIR if set, else
// $HOME/.battlestream/), current dir, /etc/battlestream/.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	setGlobalDefaults(v)

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.AddConfigPath(BaseDir())
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/battlestream")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	v.SetEnvPrefix("BS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}
	// Remember which file the config came from so SaveTUI can write back to
	// it (empty when no config file was found).
	cfg.loadedPath = v.ConfigFileUsed()

	// Migrate legacy flat config (hearthstone/storage/output at top level).
	if len(cfg.Profiles) == 0 && v.IsSet("hearthstone") {
		var legacy struct {
			Hearthstone HearthstoneConfig `mapstructure:"hearthstone"`
			Storage     StorageConfig     `mapstructure:"storage"`
			Output      OutputConfig      `mapstructure:"output"`
		}
		if err := v.Unmarshal(&legacy); err == nil {
			p := NewProfileConfig("default")
			p.Hearthstone = legacy.Hearthstone
			if legacy.Storage.DBPath != "" {
				p.Storage.DBPath = legacy.Storage.DBPath
			}
			if legacy.Output.Path != "" {
				p.Output.Path = legacy.Output.Path
			}
			if legacy.Output.WriteIntervalMs != 0 {
				p.Output.WriteIntervalMs = legacy.Output.WriteIntervalMs
			}
			p.Output.Enabled = legacy.Output.Enabled
			cfg.Profiles = map[string]*ProfileConfig{"default": p}
			cfg.ActiveProfile = "default"
		}
	}

	// Apply path expansion and fill zero-value defaults to all profiles.
	for name, p := range cfg.Profiles {
		applyProfileDefaults(p, name)
		p.expandPaths()
	}

	return &cfg, nil
}

// saveMu serializes all config file writes (Save and SaveTUI) so concurrent
// writers cannot interleave read-modify-write cycles or clobber each other's
// temp files.
var saveMu sync.Mutex

// Save writes the config to path using yaml.v3 (atomic temp+rename).
func Save(cfg *Config, path string) error {
	saveMu.Lock()
	defer saveMu.Unlock()
	return atomicWriteYAML(path, cfg)
}

// atomicWriteYAML encodes doc as YAML to a temp file in the target directory
// and renames it over path, so readers never observe a partial file.
// Callers must hold saveMu.
func atomicWriteYAML(path string, doc interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := enc.Close(); err != nil {
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

func setGlobalDefaults(v *viper.Viper) {
	// Viper limitation: AutomaticEnv only applies during Unmarshal to keys
	// viper already knows about (registered defaults or keys present in the
	// config file). Every documented BS_* env var (see docs/CONFIGURATION.md)
	// therefore needs a default registered here, otherwise the override is
	// silently ignored when the key is absent from the file. Profile-level
	// keys (profiles.<name>.*) cannot be bound this way because profile
	// names are dynamic; per-profile settings remain file-only.
	v.SetDefault("active_profile", "")
	v.SetDefault("api.grpc_addr", "127.0.0.1:50051")
	v.SetDefault("api.rest_addr", "127.0.0.1:8080")
	v.SetDefault("api.api_key", "")
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.file", "")
}

func applyProfileDefaults(p *ProfileConfig, name string) {
	base := filepath.Join(BaseDir(), "profiles", name)
	if p.Storage.DBPath == "" {
		p.Storage.DBPath = filepath.Join(base, "data")
	}
	if p.Output.Path == "" {
		p.Output.Path = filepath.Join(base, "stats")
	}
	if p.Output.WriteIntervalMs == 0 {
		p.Output.WriteIntervalMs = 500
	}
}

func (p *ProfileConfig) expandPaths() {
	p.Hearthstone.InstallPath = expandHome(p.Hearthstone.InstallPath)
	p.Hearthstone.LogPath = expandHome(p.Hearthstone.LogPath)
	p.Storage.DBPath = expandHome(p.Storage.DBPath)
	p.Output.Path = expandHome(p.Output.Path)
}

// expandHome expands "~" and "~/..." to the current user's home directory.
// "~user/..." paths are returned unchanged: resolving another user's home is
// not portable, and mangling them into $HOME/user/... is worse than passing
// them through.
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

// SetTUI updates the TUI layout preferences under the same lock SaveTUI
// uses, so callers may update prefs while a previous background SaveTUI is
// still reading them.
func (c *Config) SetTUI(t TUIConfig) {
	saveMu.Lock()
	c.TUI = t
	saveMu.Unlock()
}

// SaveTUI persists just the TUI section to the config file the Config was
// loaded from, falling back to BaseDir()/config.yaml when no config file
// existed at load time. It performs a read-modify-write that preserves all
// other keys, writes atomically (temp+rename), and is safe to call from
// multiple goroutines.
func (c *Config) SaveTUI() error {
	saveMu.Lock()
	defer saveMu.Unlock()

	path := c.loadedPath
	if path == "" {
		path = filepath.Join(BaseDir(), "config.yaml")
	}

	doc := map[string]interface{}{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parsing config %s: %w", path, err)
		}
	case os.IsNotExist(err):
		// No config file yet; write one containing only the tui section.
	default:
		return fmt.Errorf("reading config %s: %w", path, err)
	}

	doc["tui"] = map[string]interface{}{
		"vertical_split":   c.TUI.VerticalSplit,
		"horizontal_split": c.TUI.HorizontalSplit,
		"left_hsplit":      c.TUI.LeftHSplit,
		"right_hsplit":     c.TUI.RightHSplit,
	}

	return atomicWriteYAML(path, doc)
}
