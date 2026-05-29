package main

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/config"
	"github.com/michaelpeterswa/bigpictures.company/internal/db"
	"github.com/michaelpeterswa/bigpictures.company/internal/upload"
)

func newDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <slug>",
		Short: "Delete a panorama (R2 + database row)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireDatabase(); err != nil {
				return err
			}
			if err := cfg.RequireR2(); err != nil {
				return err
			}
			pool, err := db.Open(cmd.Context(), cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer pool.Close()

			row, err := pool.GetPanoramaBySlug(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			if !yes {
				confirmed := false
				prompt := huh.NewConfirm().
					Title(fmt.Sprintf("Delete panorama %q (%s)?", row.Slug, row.Title)).
					Description("This removes the R2 tile prefix, the archived original, and the database row. This is permanent.").
					Affirmative("Delete").
					Negative("Cancel").
					Value(&confirmed)
				if err := prompt.Run(); err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintln(cmd.ErrOrStderr(), "cancelled")
					return nil
				}
			}

			r2, err := upload.NewClient(cmd.Context(), upload.Credentials{
				AccountID:       cfg.R2AccountID,
				AccessKeyID:     cfg.R2AccessKeyID,
				SecretAccessKey: cfg.R2SecretAccessKey,
				Bucket:          cfg.R2Bucket,
			})
			if err != nil {
				return err
			}

			// Order matters: delete R2 objects first so a DB delete failure
			// leaves us with an obviously broken row we can manually clean
			// up, rather than orphaned R2 objects no one will ever notice.
			if err := r2.DeletePrefix(cmd.Context(), row.TilePath); err != nil {
				return err
			}
			if err := r2.DeleteKey(cmd.Context(), row.OriginalPath); err != nil {
				return err
			}
			if err := pool.DeletePanoramaBySlug(cmd.Context(), row.Slug); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "deleted")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}
