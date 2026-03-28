package main

import (
	"fmt"
	"os"

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
			return fmt.Errorf("not implemented")
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
