// Package config manages battlestream configuration via Viper.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

// NewProfileConfig returns a ProfileConfig with sensible path defaults for the given name.
func NewProfileConfig(name string) *ProfileConfig {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".battlestream", "profiles", name)
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
// Config file is searched in: $HOME/.battlestream/, current dir, /etc/battlestream/.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	setGlobalDefaults(v)

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			v.AddConfigPath(filepath.Join(home, ".battlestream"))
		}
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
	home, _ := os.UserHomeDir()
	for name, p := range cfg.Profiles {
		applyProfileDefaults(p, name, home)
		p.expandPaths()
	}

	return &cfg, nil
}

// Save writes the config to path using yaml.v3.
func Save(cfg *Config, path string) error {
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
	if err := enc.Encode(cfg); err != nil {
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
	v.SetDefault("api.grpc_addr", "127.0.0.1:50051")
	v.SetDefault("api.rest_addr", "127.0.0.1:8080")
	v.SetDefault("api.api_key", "")
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.file", "")
}

func applyProfileDefaults(p *ProfileConfig, name, home string) {
	base := filepath.Join(home, ".battlestream", "profiles", name)
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
	p.Storage.DBPath = expandHome(p.Storage.DBPath)
	p.Output.Path = expandHome(p.Output.Path)
}

func expandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// SaveTUI persists just the TUI section to the config file.
func (c *Config) SaveTUI() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("user home dir: %w", err)
	}
	path := filepath.Join(home, ".battlestream", "config.yaml")

	v := viper.New()
	v.SetConfigFile(path)
	_ = v.ReadInConfig()
	v.Set("tui.vertical_split", c.TUI.VerticalSplit)
	v.Set("tui.horizontal_split", c.TUI.HorizontalSplit)
	v.Set("tui.left_hsplit", c.TUI.LeftHSplit)
	v.Set("tui.right_hsplit", c.TUI.RightHSplit)
	return v.WriteConfig()
}
