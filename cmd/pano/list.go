package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/config"
	"github.com/michaelpeterswa/bigpictures.company/internal/db"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all panoramas",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireDatabase(); err != nil {
				return err
			}
			pool, err := db.Open(cmd.Context(), cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer pool.Close()

			rows, err := pool.ListPanoramas(cmd.Context())
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no panoramas")
				return nil
			}

			header := lipgloss.NewStyle().Bold(true)
			fmt.Fprintf(cmd.OutOrStdout(), "%-32s  %-25s  %s\n",
				header.Render("slug"), header.Render("title"), header.Render("size"))
			for _, p := range rows {
				captured := "—"
				if p.CapturedAt.Valid {
					captured = p.CapturedAt.Time.Format("2006-01-02")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-32s  %-25s  %dx%d  (%s)\n",
					trunc(p.Slug, 30), trunc(p.Title, 23), p.Width, p.Height, captured)
			}
			return nil
		},
	}
}

func trunc(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
