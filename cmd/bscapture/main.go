package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"battlestream.fixates.io/internal/capture"
	"battlestream.fixates.io/internal/discovery"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

var cfgFile string

func main() {
	root := &cobra.Command{
		Use:   "bscapture",
		Short: "Screenshot capture for Hearthstone Battlegrounds games",
		Long:  "Captures timed screenshots during BG games, tagged with game state metadata for post-game analysis.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/bscapture/config.yaml)")

	root.AddCommand(
		cmdRun(),
		cmdDetect(),
		cmdList(),
		cmdConfig(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func cmdRun() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Start capture session — waits for game, captures screenshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := capture.LoadConfig(cfgFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			if cfg.PowerLogPath == "" {
				return fmt.Errorf("power_log_path is not set; run 'bscapture detect' or set it in config")
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			slog.Info("bscapture starting",
				"power_log", cfg.PowerLogPath,
				"monitor", cfg.Monitor,
				"interval", cfg.CaptureInterval,
				"data_dir", cfg.DataDir,
			)

			events, err := capture.NewEventSource(ctx, filepath.Dir(cfg.PowerLogPath))
			if err != nil {
				return fmt.Errorf("creating event source: %w", err)
			}
			defer events.Close()

			tracker := capture.NewStateTracker()
			screenshotter := capture.NewScreenshotter(cfg.Monitor, cfg.OutputResWidth, cfg.OutputResHeight)
			store := capture.NewStore(cfg.DataDir, cfg.JPEGQuality)

			loop := capture.NewLoop(events, tracker, screenshotter, store, cfg.CaptureInterval, cfg.StaleTimeout)

			slog.Info("capture loop running, waiting for games...")
			return loop.Run(ctx)
		},
	}
}

type monitorInfo struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	X           int     `json:"x"`
	Y           int     `json:"y"`
	Scale       float64 `json:"scale"`
}

func detectMonitors() ([]monitorInfo, error) {
	out, err := exec.Command("hyprctl", "monitors", "-j").Output()
	if err != nil {
		return nil, fmt.Errorf("running hyprctl monitors -j: %w", err)
	}
	var monitors []monitorInfo
	if err := json.Unmarshal(out, &monitors); err != nil {
		return nil, fmt.Errorf("parsing hyprctl output: %w", err)
	}
	return monitors, nil
}

func cmdDetect() *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Detect monitors and Hearthstone install, update config",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load existing config (OK if none exists yet).
			cfg, err := capture.LoadConfig(cfgFile)
			if err != nil {
				cfg = &capture.Config{}
			}

			// Detect monitors.
			monitors, err := detectMonitors()
			if err != nil {
				return fmt.Errorf("detecting monitors: %w", err)
			}
			if len(monitors) == 0 {
				return fmt.Errorf("no monitors detected")
			}

			// Display numbered list.
			fmt.Println("Detected monitors:")
			for i, m := range monitors {
				fmt.Printf("  %d) %s  %dx%d  %s\n", i+1, m.Name, m.Width, m.Height, m.Description)
			}

			// Prompt user to pick one.
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("Select monitor [1-%d]: ", len(monitors))
			line, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading input: %w", err)
			}
			choice, err := strconv.Atoi(strings.TrimSpace(line))
			if err != nil || choice < 1 || choice > len(monitors) {
				return fmt.Errorf("invalid selection: %s", strings.TrimSpace(line))
			}
			selected := monitors[choice-1]
			cfg.Monitor = selected.Name
			fmt.Printf("Selected: %s (%dx%d)\n", selected.Name, selected.Width, selected.Height)

			// Discover Power.log path.
			info, err := discovery.Discover()
			if err != nil {
				return fmt.Errorf("detecting Hearthstone install: %w", err)
			}
			cfg.PowerLogPath = filepath.Join(info.LogPath, "Power.log")
			fmt.Printf("Power.log: %s\n", cfg.PowerLogPath)

			// Determine config path.
			savePath := cfgFile
			if savePath == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("finding home directory: %w", err)
				}
				savePath = filepath.Join(home, ".config", "bscapture", "config.yaml")
			}

			if err := capture.SaveConfig(cfg, savePath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
			fmt.Printf("Config saved to %s\n", savePath)
			return nil
		},
	}
}

func cmdList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List captured game sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := capture.LoadConfig(cfgFile)
			if err != nil {
				return err
			}

			entries, err := os.ReadDir(cfg.DataDir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No captures found.")
					return nil
				}
				return err
			}

			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				dbPath := filepath.Join(cfg.DataDir, entry.Name(), "capture.db")
				if _, err := os.Stat(dbPath); err != nil {
					continue
				}
				db, err := sql.Open("sqlite", dbPath)
				if err != nil {
					continue
				}
				var gameID, startTime string
				var placement, frames int
				var endTime sql.NullString
				err = db.QueryRow(`SELECT game_id, start_time, end_time, placement, total_frames FROM games LIMIT 1`).
					Scan(&gameID, &startTime, &endTime, &placement, &frames)
				db.Close()
				if err != nil {
					continue
				}
				status := "in-progress"
				if endTime.Valid {
					status = fmt.Sprintf("#%d", placement)
				}
				fmt.Printf("%-40s  %s  %s  %d frames\n", gameID, startTime, status, frames)
			}
			return nil
		},
	}
}

func cmdConfig() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := capture.LoadConfig(cfgFile)
			if err != nil {
				return err
			}
			fmt.Printf("power_log_path:    %s\n", cfg.PowerLogPath)
			fmt.Printf("monitor:           %s\n", cfg.Monitor)
			fmt.Printf("capture_interval:  %s\n", cfg.CaptureInterval)
			fmt.Printf("output_resolution: %dx%d\n", cfg.OutputResWidth, cfg.OutputResHeight)
			fmt.Printf("jpeg_quality:      %d\n", cfg.JPEGQuality)
			fmt.Printf("data_dir:          %s\n", cfg.DataDir)
			fmt.Printf("stale_timeout:     %s\n", cfg.StaleTimeout)
			return nil
		},
	}
}
