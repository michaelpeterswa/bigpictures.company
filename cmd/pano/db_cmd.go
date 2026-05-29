package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/config"
	"github.com/michaelpeterswa/bigpictures.company/internal/db"
)

// newDBCmd implements `pano db ping`, a hidden diagnostic used to verify the
// PANO_DATABASE_URL connection works against the live database.
func newDBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "db",
		Short:  "Database diagnostics",
		Hidden: true,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "ping",
		Short: "Verify connectivity to PANO_DATABASE_URL",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireDatabase(); err != nil {
				return err
			}
			conn, err := db.Open(cmd.Context(), cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer conn.Close()
			if err := conn.Ping(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "ok")
			return nil
		},
	})
	return cmd
}
