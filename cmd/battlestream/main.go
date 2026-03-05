package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"battlestream.fixates.io/internal/api/rest"
	grpcserver "battlestream.fixates.io/internal/api/grpc"
	"battlestream.fixates.io/internal/config"
	"battlestream.fixates.io/internal/discovery"
	"battlestream.fixates.io/internal/fileout"
	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/logconfig"
	"battlestream.fixates.io/internal/parser"
	"battlestream.fixates.io/internal/store"
	"battlestream.fixates.io/internal/tui"
	"battlestream.fixates.io/internal/watcher"
)

const version = "0.1.0-dev"

var cfgFile string

func main() {
	root := &cobra.Command{
		Use:   "battlestream",
		Short: "Hearthstone Battlegrounds stat tracker and overlay backend",
		Long: `battlestream monitors Hearthstone Battlegrounds games via log parsing,
persists aggregate stats, and exposes them via gRPC, REST, WebSocket, and file output.`,
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.battlestream/config.yaml)")

	root.AddCommand(
		cmdDaemon(),
		cmdTUI(),
		cmdDiscover(),
		cmdConfig(),
		cmdVersion(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// --- daemon ---

func cmdDaemon() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Start the battlestream background service",
		Long:  "Starts gRPC + REST + WebSocket servers, tails HS logs, and writes stat files.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			setupLogging(cfg.Logging)

			// --- Resolve log path ---
			logPath := cfg.Hearthstone.LogPath
			if logPath == "" {
				info, err := discovery.Discover()
				if err != nil {
					return fmt.Errorf("auto-discovery failed: %w\nRun 'battlestream discover' to set paths manually", err)
				}
				logPath = info.LogPath
				slog.Info("auto-discovered HS logs", "path", logPath)

				if cfg.Hearthstone.AutoPatchLogConfig {
					if err := logconfig.EnsureVerboseLogging(info.LogConfig); err != nil {
						slog.Warn("could not patch log.config", "err", err)
					} else {
						slog.Info("log.config patched", "path", info.LogConfig)
					}
				}
			}

			// --- Open store ---
			st, err := store.Open(cfg.Storage.DBPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer st.Close()

			// --- File output ---
			var fw *fileout.Writer
			if cfg.Output.Enabled {
				fw, err = fileout.New(cfg.Output.Path)
				if err != nil {
					return fmt.Errorf("creating file writer: %w", err)
				}
				slog.Info("file output enabled", "path", cfg.Output.Path)
			}

			// --- Game state machine ---
			machine := gamestate.New()
			proc := gamestate.NewProcessor(machine)

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			// --- Event bus ---
			// parsedCh receives events from the parser.
			// broadcastCh receives copies for the gRPC fan-out.
			parsedCh := make(chan parser.GameEvent, 512)
			broadcastCh := make(chan parser.GameEvent, 512)
			p := parser.New(parsedCh)

			// --- Log watcher ---
			w, err := watcher.New(ctx, watcher.Config{
				LogDir:    logPath,
				Files:     []string{"Power.log"},
				Reopen:    true,
				MustExist: false,
			})
			if err != nil {
				return fmt.Errorf("starting watcher: %w", err)
			}
			defer w.Stop()

			// --- Line ingestion: watcher → parser ---
			go func() {
				for {
					select {
					case line, ok := <-w.Lines:
						if !ok {
							return
						}
						p.Feed(line.Text)
					case <-ctx.Done():
						return
					}
				}
			}()

			// --- Event processing: parsedCh → state machine + broadcast ---
			go func() {
				interval := time.Duration(cfg.Output.WriteIntervalMs) * time.Millisecond
				ticker := time.NewTicker(interval)
				defer ticker.Stop()

				for {
					select {
					case e, ok := <-parsedCh:
						if !ok {
							return
						}
						proc.Handle(e)

						// Persist game end
						if e.Type == parser.EventGameEnd {
							s := machine.State()
							if err := st.SaveFullGame(s); err != nil {
								slog.Error("persisting game", "err", err)
							}
							if fw != nil {
								if err := fw.WriteHistory(s); err != nil {
									slog.Error("writing history", "err", err)
								}
								agg, err := st.GetAggregate()
								if err == nil {
									if err := fw.WriteAggregate(agg); err != nil {
										slog.Error("writing aggregate", "err", err)
									}
								}
							}
						}

						// Fan to broadcast channel (non-blocking)
						select {
						case broadcastCh <- e:
						default:
						}

					case <-ticker.C:
						if fw != nil {
							s := machine.State()
							if err := fw.WriteCurrentState(s); err != nil {
								slog.Error("writing current state", "err", err)
							}
						}

					case <-ctx.Done():
						return
					}
				}
			}()

			// --- gRPC server ---
			grpcSrv := grpcserver.New(machine, st, broadcastCh)

			go func() {
				if err := grpcSrv.Serve(ctx, cfg.API.GRPCAddr); err != nil {
					slog.Error("gRPC server error", "err", err)
				}
			}()

			// --- REST server ---
			restSrv := rest.New(grpcSrv, cfg.API.APIKey)
			go func() {
				if err := restSrv.Serve(ctx, cfg.API.RESTAddr); err != nil {
					slog.Error("REST server error", "err", err)
				}
			}()

			slog.Info("battlestream daemon started",
				"grpc", cfg.API.GRPCAddr,
				"rest", cfg.API.RESTAddr,
			)

			<-ctx.Done()
			slog.Info("shutting down")
			return nil
		},
	}
}

// --- tui ---

func cmdTUI() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the live TUI dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}
			_ = cfg

			// In the real implementation, this connects to the daemon via gRPC.
			// For now, use a local machine for standalone mode.
			machine := gamestate.New()

			model := tui.New(func() gamestate.BGGameState {
				return machine.State()
			})
			return model.Run()
		},
	}
}

// --- discover ---

func cmdDiscover() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Interactive Hearthstone install discovery wizard",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Searching for Hearthstone installation...")

			info, err := discovery.Discover()
			if err != nil {
				fmt.Printf("Auto-discovery failed: %v\n\n", err)
				fmt.Print("Enter the path to your Hearthstone install or logs directory: ")

				scanner := bufio.NewScanner(os.Stdin)
				if !scanner.Scan() {
					return fmt.Errorf("no input provided")
				}
				userPath := strings.TrimSpace(scanner.Text())
				if userPath == "" {
					return fmt.Errorf("no path entered")
				}

				// Try direct probe first
				info, err = discovery.DiscoverFromRoot(userPath)
				if err != nil {
					// Try walking
					fmt.Printf("Probing %s (this may take a moment)...\n", userPath)
					info, err = discovery.WalkForInstall(userPath)
					if err != nil {
						return fmt.Errorf("could not find Hearthstone install: %w", err)
					}
				}
			}

			fmt.Printf("\nFound Hearthstone installation:\n")
			fmt.Printf("  Install root: %s\n", info.InstallRoot)
			fmt.Printf("  Log path:     %s\n", info.LogPath)
			fmt.Printf("  log.config:   %s\n", info.LogConfig)

			// Save to config
			cfg, err := config.Load(cfgFile)
			if err != nil {
				cfg = &config.Config{}
			}
			cfg.Hearthstone.InstallPath = info.InstallRoot
			cfg.Hearthstone.LogPath = info.LogPath

			home, _ := os.UserHomeDir()
			savePath := filepath.Join(home, ".battlestream", "config.yaml")
			if cfgFile != "" {
				savePath = cfgFile
			}

			if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
				return fmt.Errorf("creating config dir: %w", err)
			}
			if err := config.Save(cfg, savePath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("\nConfig saved to: %s\n", savePath)
			fmt.Println("\nRun 'battlestream daemon' to start the service.")
			return nil
		},
	}
	return cmd
}

// --- config ---

func cmdConfig() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show or validate current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			fmt.Printf("Configuration:\n")
			fmt.Printf("  hearthstone.install_path:        %q\n", cfg.Hearthstone.InstallPath)
			fmt.Printf("  hearthstone.log_path:            %q\n", cfg.Hearthstone.LogPath)
			fmt.Printf("  hearthstone.auto_patch_logconfig: %v\n", cfg.Hearthstone.AutoPatchLogConfig)
			fmt.Printf("  storage.db_path:                 %q\n", cfg.Storage.DBPath)
			fmt.Printf("  output.enabled:                  %v\n", cfg.Output.Enabled)
			fmt.Printf("  output.path:                     %q\n", cfg.Output.Path)
			fmt.Printf("  output.write_interval_ms:        %d\n", cfg.Output.WriteIntervalMs)
			fmt.Printf("  api.grpc_addr:                   %q\n", cfg.API.GRPCAddr)
			fmt.Printf("  api.rest_addr:                   %q\n", cfg.API.RESTAddr)
			fmt.Printf("  logging.level:                   %q\n", cfg.Logging.Level)
			fmt.Printf("  logging.file:                    %q\n", cfg.Logging.File)
			return nil
		},
	}
}

// --- version ---

func cmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("battlestream %s\n", version)
			fmt.Printf("module: battlestream.fixates.io\n")
		},
	}
}

// --- helpers ---

func setupLogging(cfg config.LoggingConfig) {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}

	if cfg.File != "" {
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			handler = slog.NewJSONHandler(f, opts)
		}
	}

	if handler == nil {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(handler))
}
