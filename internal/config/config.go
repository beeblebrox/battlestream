package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all configuration for battlestream.
type Config struct {
	Hearthstone HearthstoneConfig `mapstructure:"hearthstone"`
	Storage     StorageConfig     `mapstructure:"storage"`
	Output      OutputConfig      `mapstructure:"output"`
	API         APIConfig         `mapstructure:"api"`
	Logging     LoggingConfig     `mapstructure:"logging"`
}

type HearthstoneConfig struct {
	InstallPath       string `mapstructure:"install_path"`
	LogPath           string `mapstructure:"log_path"`
	AutoPatchLogConfig bool   `mapstructure:"auto_patch_logconfig"`
}

type StorageConfig struct {
	DBPath string `mapstructure:"db_path"`
}

type OutputConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	Path            string `mapstructure:"path"`
	WriteIntervalMs int    `mapstructure:"write_interval_ms"`
}

type APIConfig struct {
	GRPCAddr string `mapstructure:"grpc_addr"`
	RESTAddr string `mapstructure:"rest_addr"`
	APIKey   string `mapstructure:"api_key"`
}

type LoggingConfig struct {
	Level string `mapstructure:"level"`
	File  string `mapstructure:"file"`
}

// Load reads config from file and environment variables.
// Config file is searched in: $HOME/.battlestream/, current dir, /etc/battlestream/.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	setDefaults(v)

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

	cfg.expandPaths()
	return &cfg, nil
}

// Save writes the config to a file.
func Save(cfg *Config, path string) error {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.MergeConfigMap(map[string]interface{}{
		"hearthstone": cfg.Hearthstone,
		"storage":     cfg.Storage,
		"output":      cfg.Output,
		"api":         cfg.API,
		"logging":     cfg.Logging,
	}); err != nil {
		return fmt.Errorf("merging config: %w", err)
	}

	return v.WriteConfigAs(path)
}

func setDefaults(v *viper.Viper) {
	home, _ := os.UserHomeDir()

	v.SetDefault("hearthstone.install_path", "")
	v.SetDefault("hearthstone.log_path", "")
	v.SetDefault("hearthstone.auto_patch_logconfig", true)

	v.SetDefault("storage.db_path", filepath.Join(home, ".battlestream", "data"))
	v.SetDefault("output.enabled", true)
	v.SetDefault("output.path", filepath.Join(home, ".battlestream", "stats"))
	v.SetDefault("output.write_interval_ms", 500)

	v.SetDefault("api.grpc_addr", "127.0.0.1:50051")
	v.SetDefault("api.rest_addr", "127.0.0.1:8080")
	v.SetDefault("api.api_key", "")

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.file", "")
}

// expandPaths expands ~ in path fields.
func (c *Config) expandPaths() {
	c.Storage.DBPath = expandHome(c.Storage.DBPath)
	c.Output.Path = expandHome(c.Output.Path)
	c.Logging.File = expandHome(c.Logging.File)
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
