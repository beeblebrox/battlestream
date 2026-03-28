package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"battlestream.fixates.io/internal/capture"
	"github.com/spf13/cobra"
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

func cmdDetect() *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Detect monitors and Hearthstone install, update config",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
}

func cmdList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List captured game sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
}

func cmdConfig() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
}
