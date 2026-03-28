package capture

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	PowerLogPath    string        `mapstructure:"power_log_path"`
	Monitor         string        `mapstructure:"monitor"`
	CaptureInterval time.Duration `mapstructure:"capture_interval"`
	OutputResWidth  int           `mapstructure:"output_res_width"`
	OutputResHeight int           `mapstructure:"output_res_height"`
	JPEGQuality     int           `mapstructure:"jpeg_quality"`
	DataDir         string        `mapstructure:"data_dir"`
	StaleTimeout    time.Duration `mapstructure:"stale_timeout"`
}

func LoadConfig(cfgFile string) (*Config, error) {
	v := viper.New()

	v.SetDefault("monitor", "DP-1")
	v.SetDefault("capture_interval", "1s")
	v.SetDefault("output_res_width", 1920)
	v.SetDefault("output_res_height", 1080)
	v.SetDefault("jpeg_quality", 92)
	v.SetDefault("stale_timeout", "5m")

	home, _ := os.UserHomeDir()
	v.SetDefault("data_dir", filepath.Join(home, ".local", "share", "bscapture"))

	v.SetEnvPrefix("BSC")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(filepath.Join(home, ".config", "bscapture"))
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config, path string) error {
	v := viper.New()
	v.Set("power_log_path", cfg.PowerLogPath)
	v.Set("monitor", cfg.Monitor)
	v.Set("capture_interval", cfg.CaptureInterval.String())
	v.Set("output_res_width", cfg.OutputResWidth)
	v.Set("output_res_height", cfg.OutputResHeight)
	v.Set("jpeg_quality", cfg.JPEGQuality)
	v.Set("data_dir", cfg.DataDir)
	v.Set("stale_timeout", cfg.StaleTimeout.String())

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return v.WriteConfigAs(path)
}
