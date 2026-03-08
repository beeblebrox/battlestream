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

	grpcserver "battlestream.fixates.io/internal/api/grpc"
	"battlestream.fixates.io/internal/api/rest"
	"battlestream.fixates.io/internal/config"
	"battlestream.fixates.io/internal/debugtui"
	"battlestream.fixates.io/internal/discovery"
	"battlestream.fixates.io/internal/fileout"
	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/logconfig"
	"battlestream.fixates.io/internal/parser"
	"battlestream.fixates.io/internal/store"
	"battlestream.fixates.io/internal/tui"
	"battlestream.fixates.io/internal/watcher"
)

var version = "dev"

var (
	cfgFile     string
	profileFlag string
)

func main() {
	root := &cobra.Command{
		Use:   "battlestream",
		Short: "Hearthstone Battlegrounds stat tracker and overlay backend",
		Long: `battlestream monitors Hearthstone Battlegrounds games via log parsing,
persists aggregate stats, and exposes them via gRPC, REST, WebSocket, and file output.`,
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.battlestream/config.yaml)")
	root.PersistentFlags().StringVar(&profileFlag, "profile", "", "profile name to use (default: active_profile or sole profile)")

	root.AddCommand(
		cmdDaemon(),
		cmdTUI(),
		cmdDebug(),
		cmdReplay(),
		cmdDiscover(),
		cmdConfig(),
		cmdReparse(),
		cmdDBReset(),
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

			profile, err := cfg.GetProfile(profileFlag)
			if err != nil {
				return err
			}

			// --- Resolve log path ---
			logPath := profile.Hearthstone.LogPath
			if logPath == "" {
				info, err := discovery.Discover()
				if err != nil {
					return fmt.Errorf("auto-discovery failed: %w\nRun 'battlestream discover' to set paths manually", err)
				}
				logPath = info.LogPath
				slog.Info("auto-discovered HS logs", "path", logPath)

				if profile.Hearthstone.AutoPatchLogConfig {
					if err := logconfig.EnsureVerboseLogging(info.LogConfig); err != nil {
						slog.Warn("could not patch log.config", "err", err)
					} else {
						slog.Info("log.config patched", "path", info.LogConfig)
					}
				}
			}

			// --- Open store ---
			st, err := store.Open(profile.Storage.DBPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer st.Close()

			// --- File output ---
			var fw *fileout.Writer
			if profile.Output.Enabled {
				fw, err = fileout.New(profile.Output.Path)
				if err != nil {
					return fmt.Errorf("creating file writer: %w", err)
				}
				slog.Info("file output enabled", "path", profile.Output.Path)
			}

			// --- Game state machine ---
			machine := gamestate.New()
			proc := gamestate.NewProcessor(machine)

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			// --- Event bus ---
			parsedCh := make(chan parser.GameEvent, 512)
			broadcastCh := make(chan parser.GameEvent, 512)
			p := parser.New(parsedCh)

			// --- Log watcher ---
			w, err := watcher.New(ctx, watcher.Config{
				LogDir:        logPath,
				Files:         []string{"Power.log"},
				Reopen:        true,
				MustExist:     false,
				ReadFromStart: true,
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
				interval := time.Duration(profile.Output.WriteIntervalMs) * time.Millisecond
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
							if st.HasGame(s.GameID) {
								slog.Info("game already persisted, skipping", "gameID", s.GameID)
							} else if err := st.SaveFullGame(s); err != nil {
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
	var dumpFlag bool
	var widthFlag int

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Open the live TUI dashboard (connects to running daemon)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}
			if dumpFlag {
				out, err := tui.Dump(cfg.API.GRPCAddr, widthFlag)
				if err != nil {
					return err
				}
				fmt.Println(out)
				return nil
			}
			return tui.New(cfg.API.GRPCAddr).Run()
		},
	}

	cmd.Flags().BoolVar(&dumpFlag, "dump", false, "dump rendered TUI to stdout (no TTY needed)")
	cmd.Flags().IntVar(&widthFlag, "width", 120, "terminal width for --dump rendering")
	return cmd
}

// --- debug ---

func cmdDebug() *cobra.Command {
	return &cobra.Command{
		Use:   "debug [power.log...]",
		Short: "Step through Power.log files interactively",
		Long: `Opens a debug TUI to step through Power.log events one by one.

If no file arguments are given, discovers log files from config (same locations
as 'battlestream reparse'). All games found are listed for selection.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var logFiles []string

			if len(args) > 0 {
				logFiles = args
			} else {
				// Discover log files from config, same as reparse.
				cfg, err := config.Load(cfgFile)
				if err != nil {
					return fmt.Errorf("loading config: %w", err)
				}
				profile, err := cfg.GetProfile(profileFlag)
				if err != nil {
					return err
				}
				logPath := profile.Hearthstone.LogPath
				if logPath == "" {
					info, dErr := discovery.Discover()
					if dErr != nil {
						return fmt.Errorf("auto-discovery failed: %w\nSpecify log files as arguments or run 'battlestream discover'", dErr)
					}
					logPath = info.LogPath
				}
				logFiles = findPowerLogs(logPath)
				if len(logFiles) == 0 {
					return fmt.Errorf("no Power.log files found in %s", logPath)
				}
			}

			return debugtui.New(logFiles).Run()
		},
	}
}

// --- replay ---

func cmdReplay() *cobra.Command {
	var dumpFlag bool
	var turnFlag int
	var widthFlag int

	cmd := &cobra.Command{
		Use:   "replay [flags] [log-file...]",
		Short: "Step through a past game log by turn (no daemon required)",
		Long: `Opens a debug TUI to step through Power.log events one by one.

If --dump is set, renders the TUI state at the given turn to stdout and exits.
If no file arguments are given, discovers log files from config.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var logFiles []string

			if len(args) > 0 {
				logFiles = args
			} else {
				cfg, err := config.Load(cfgFile)
				if err != nil {
					return fmt.Errorf("loading config: %w", err)
				}
				profile, err := cfg.GetProfile(profileFlag)
				if err != nil {
					return err
				}
				logPath := profile.Hearthstone.LogPath
				if logPath == "" {
					info, dErr := discovery.Discover()
					if dErr != nil {
						return fmt.Errorf("auto-discovery failed: %w\nSpecify log files as arguments or run 'battlestream discover'", dErr)
					}
					logPath = info.LogPath
				}
				logFiles = findPowerLogs(logPath)
				if len(logFiles) == 0 {
					return fmt.Errorf("no Power.log files found in %s", logPath)
				}
			}

			if dumpFlag {
				if len(logFiles) != 1 {
					return fmt.Errorf("specify a single log file for --dump mode")
				}
				out, err := debugtui.Dump(logFiles[0], turnFlag, widthFlag)
				if err != nil {
					return err
				}
				fmt.Println(out)
				return nil
			}

			return debugtui.New(logFiles).Run()
		},
	}

	cmd.Flags().BoolVar(&dumpFlag, "dump", false, "render to stdout and exit instead of launching interactive TUI")
	cmd.Flags().IntVar(&turnFlag, "turn", 1, "which BG turn to render (only used with --dump)")
	cmd.Flags().IntVar(&widthFlag, "width", 120, "terminal width for --dump rendering")
	return cmd
}

// findPowerLogs returns all Power.log files in the given log directory.
// Checks both session subdirs (Hearthstone_*/) and the dir itself.
func findPowerLogs(logPath string) []string {
	var files []string
	entries, _ := os.ReadDir(logPath)
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "Hearthstone_") {
			candidate := filepath.Join(logPath, e.Name(), "Power.log")
			if fileExists(candidate) {
				files = append(files, candidate)
			}
		}
	}
	if direct := filepath.Join(logPath, "Power.log"); fileExists(direct) {
		files = append(files, direct)
	}
	return files
}

// --- discover ---

func cmdDiscover() *cobra.Command {
	return &cobra.Command{
		Use:   "discover",
		Short: "Interactive Hearthstone install discovery wizard",
		RunE: func(cmd *cobra.Command, args []string) error {
			scanner := bufio.NewScanner(os.Stdin)

			// Load existing config (or start fresh).
			cfg, err := config.Load(cfgFile)
			if err != nil {
				cfg = &config.Config{}
			}

			fmt.Println("Searching for Hearthstone installations...")
			found, err := discovery.DiscoverAll()
			if err != nil {
				fmt.Printf("Auto-discovery: %v\n", err)
				found = nil
			} else {
				fmt.Printf("Found %d installation(s) automatically.\n", len(found))
			}

			// Show all auto-discovered installs and collect profile names.
			var namedInstalls []namedInstall
			for i, info := range found {
				fmt.Printf("\n[%d] Install root: %s\n", i+1, info.InstallRoot)
				fmt.Printf("    Log path:     %s\n", info.LogPath)
				fmt.Printf("    log.config:   %s\n", info.LogConfig)
				fmt.Printf("    Profile name (Enter to skip): ")
				if !scanner.Scan() {
					break
				}
				name := strings.TrimSpace(scanner.Text())
				if name != "" {
					namedInstalls = append(namedInstalls, namedInstall{name: name, info: info})
				}
			}

			// Offer to add paths manually.
			for {
				fmt.Print("\nAdd a path manually? (y/N): ")
				if !scanner.Scan() {
					break
				}
				answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
				if answer != "y" && answer != "yes" {
					break
				}

				fmt.Println("Enter one of:")
				fmt.Println("  • Hearthstone install dir  (contains Hearthstone.exe / Hearthstone.app)")
				fmt.Println("  • Logs directory           (contains Power.log)")
				fmt.Println("  • Wine/Proton prefix root  (contains drive_c/)  e.g. /chungus/battlenet")
				fmt.Print("Path: ")
				if !scanner.Scan() {
					break
				}
				userPath := strings.TrimSpace(scanner.Text())
				if userPath == "" {
					continue
				}

				info, err := discovery.DiscoverFromRoot(userPath)
				if err != nil {
					fmt.Printf("Probing %s (this may take a moment)...\n", userPath)
					info, err = discovery.WalkForInstall(userPath)
					if err != nil {
						fmt.Printf("Could not find Hearthstone install: %v\n", err)
						continue
					}
				}

				fmt.Printf("  Install root: %s\n", info.InstallRoot)
				fmt.Printf("  Log path:     %s\n", info.LogPath)
				fmt.Printf("  log.config:   %s\n", info.LogConfig)
				fmt.Print("Profile name: ")
				if !scanner.Scan() {
					break
				}
				name := strings.TrimSpace(scanner.Text())
				if name == "" {
					fmt.Println("Skipping (no name given).")
					continue
				}
				namedInstalls = append(namedInstalls, namedInstall{name: name, info: info})
			}

			if len(namedInstalls) == 0 {
				return fmt.Errorf("no installations configured; run 'battlestream discover' again")
			}

			// Build profiles and add to config.
			firstProfile := ""
			for _, ni := range namedInstalls {
				p := config.NewProfileConfig(ni.name)
				p.Hearthstone.InstallPath = ni.info.InstallRoot
				p.Hearthstone.LogPath = ni.info.LogPath
				setActive := firstProfile == ""
				cfg.AddProfile(ni.name, p, setActive)
				if setActive {
					firstProfile = ni.name
				}
				fmt.Printf("  Added profile %q → %s\n", ni.name, ni.info.InstallRoot)
			}

			// If multiple profiles, ask which should be active.
			if len(namedInstalls) > 1 {
				fmt.Printf("\nMultiple profiles added. Active profile [%s]: ", cfg.ActiveProfile)
				if scanner.Scan() {
					if name := strings.TrimSpace(scanner.Text()); name != "" {
						if _, ok := cfg.Profiles[name]; ok {
							cfg.ActiveProfile = name
						} else {
							fmt.Printf("Profile %q not found; keeping %q as active.\n", name, cfg.ActiveProfile)
						}
					}
				}
			}

			// Determine save path.
			home, _ := os.UserHomeDir()
			savePath := filepath.Join(home, ".battlestream", "config.yaml")
			if cfgFile != "" {
				savePath = cfgFile
			}

			if err := config.Save(cfg, savePath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("\nConfig saved to: %s\n", savePath)
			fmt.Printf("Active profile:  %s\n", cfg.ActiveProfile)
			if len(cfg.Profiles) > 1 {
				fmt.Printf("All profiles:    %s\n", cfg.ProfileList())
				fmt.Println("\nUse --profile <name> to select a specific profile.")
			}
			fmt.Println("\nRun 'battlestream daemon' to start the service.")
			return nil
		},
	}
}

// namedInstall pairs a profile name with a discovered install.
type namedInstall struct {
	name string
	info *discovery.InstallInfo
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

			fmt.Printf("Global settings:\n")
			fmt.Printf("  api.grpc_addr:   %q\n", cfg.API.GRPCAddr)
			fmt.Printf("  api.rest_addr:   %q\n", cfg.API.RESTAddr)
			fmt.Printf("  logging.level:   %q\n", cfg.Logging.Level)
			fmt.Printf("  logging.file:    %q\n", cfg.Logging.File)

			if len(cfg.Profiles) == 0 {
				fmt.Println("\nNo profiles configured. Run 'battlestream discover'.")
				return nil
			}

			fmt.Printf("\nActive profile: %s\n", cfg.ActiveProfile)
			fmt.Printf("Profiles (%d):\n", len(cfg.Profiles))

			names := cfg.ProfileList()
			for _, name := range strings.Split(names, ", ") {
				p := cfg.Profiles[name]
				marker := ""
				if name == cfg.ActiveProfile {
					marker = " *"
				}
				fmt.Printf("\n  [%s]%s\n", name, marker)
				fmt.Printf("    hearthstone.install_path:        %q\n", p.Hearthstone.InstallPath)
				fmt.Printf("    hearthstone.log_path:            %q\n", p.Hearthstone.LogPath)
				fmt.Printf("    hearthstone.auto_patch_logconfig: %v\n", p.Hearthstone.AutoPatchLogConfig)
				fmt.Printf("    storage.db_path:                 %q\n", p.Storage.DBPath)
				fmt.Printf("    output.enabled:                  %v\n", p.Output.Enabled)
				fmt.Printf("    output.path:                     %q\n", p.Output.Path)
				fmt.Printf("    output.write_interval_ms:        %d\n", p.Output.WriteIntervalMs)
			}
			return nil
		},
	}
}

// --- reparse ---

func cmdReparse() *cobra.Command {
	return &cobra.Command{
		Use:   "reparse",
		Short: "Parse all existing Power.log files and populate the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			profile, err := cfg.GetProfile(profileFlag)
			if err != nil {
				return err
			}

			logPath := profile.Hearthstone.LogPath
			if logPath == "" {
				info, dErr := discovery.Discover()
				if dErr != nil {
					return fmt.Errorf("auto-discovery failed: %w", dErr)
				}
				logPath = info.LogPath
			}

			st, err := store.Open(profile.Storage.DBPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer st.Close()

			machine := gamestate.New()
			proc := gamestate.NewProcessor(machine)

			// Find all Power.log files in the log directory.
			var logFiles []string
			entries, _ := os.ReadDir(logPath)
			for _, e := range entries {
				if e.IsDir() && strings.HasPrefix(e.Name(), "Hearthstone_") {
					candidate := filepath.Join(logPath, e.Name(), "Power.log")
					if _, err := os.Stat(candidate); err == nil {
						logFiles = append(logFiles, candidate)
					}
				}
			}
			// Also check for Power.log directly in the log dir.
			if direct := filepath.Join(logPath, "Power.log"); fileExists(direct) {
				logFiles = append(logFiles, direct)
			}

			if len(logFiles) == 0 {
				fmt.Println("No Power.log files found in", logPath)
				return nil
			}

			parsedCh := make(chan parser.GameEvent, 512)
			p := parser.New(parsedCh)

			// Process events in a goroutine.
			done := make(chan struct{})
			gamesFound := 0
			gamesSaved := 0
			go func() {
				defer close(done)
				for e := range parsedCh {
					proc.Handle(e)
					if e.Type == parser.EventGameEnd {
						gamesFound++
						s := machine.State()
						if st.HasGame(s.GameID) {
							continue
						}
						if err := st.SaveFullGame(s); err != nil {
							slog.Error("persisting game", "err", err)
						} else {
							gamesSaved++
						}
					}
				}
			}()

			// Parse each log file sequentially.
			for _, lf := range logFiles {
				fmt.Printf("Parsing %s...\n", lf)
				f, err := os.Open(lf)
				if err != nil {
					slog.Error("opening log file", "path", lf, "err", err)
					continue
				}
				scanner := bufio.NewScanner(f)
				scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
				for scanner.Scan() {
					p.Feed(scanner.Text())
				}
				f.Close()
			}
			p.Flush()
			close(parsedCh)
			<-done

			fmt.Printf("Done: found %d games, saved %d new\n", gamesFound, gamesSaved)
			return nil
		},
	}
}

// --- db reset ---

func cmdDBReset() *cobra.Command {
	return &cobra.Command{
		Use:   "db-reset",
		Short: "Clear all data in the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			profile, err := cfg.GetProfile(profileFlag)
			if err != nil {
				return err
			}

			fmt.Printf("This will delete ALL data in %s\n", profile.Storage.DBPath)
			fmt.Print("Are you sure? (yes/N): ")
			scanner := bufio.NewScanner(os.Stdin)
			if !scanner.Scan() {
				return nil
			}
			answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
			if answer != "yes" {
				fmt.Println("Aborted.")
				return nil
			}

			st, err := store.Open(profile.Storage.DBPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer st.Close()

			if err := st.DropAll(); err != nil {
				return fmt.Errorf("dropping data: %w", err)
			}

			fmt.Println("Database cleared.")
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

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
