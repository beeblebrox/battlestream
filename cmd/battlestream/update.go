package main

import (
	"fmt"

	selfupdate "github.com/creativeprojects/go-selfupdate"
	"github.com/spf13/cobra"
)

func cmdUpdate() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update battlestream to the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
			if err != nil {
				return fmt.Errorf("create source: %w", err)
			}
			updater, err := selfupdate.NewUpdater(selfupdate.Config{
				Source:    source,
				Validator: &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"},
			})
			if err != nil {
				return fmt.Errorf("create updater: %w", err)
			}

			latest, found, err := updater.DetectLatest(cmd.Context(), selfupdate.ParseSlug("beeblebrox/battlestream"))
			if err != nil {
				return fmt.Errorf("detect latest: %w", err)
			}
			if !found {
				fmt.Println("No release found.")
				return nil
			}

			current := version
			if latest.LessOrEqual(current) {
				fmt.Printf("Already up to date: %s\n", current)
				return nil
			}

			fmt.Printf("Updating %s -> %s ...\n", current, latest.Version())
			exe, err := selfupdate.ExecutablePath()
			if err != nil {
				return fmt.Errorf("executable path: %w", err)
			}
			if err := updater.UpdateTo(cmd.Context(), latest, exe); err != nil {
				return fmt.Errorf("update failed: %w", err)
			}
			fmt.Printf("Updated to %s\n", latest.Version())
			return nil
		},
	}
}
