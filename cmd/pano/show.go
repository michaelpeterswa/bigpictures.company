package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/config"
	"github.com/michaelpeterswa/bigpictures.company/internal/db"
)

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <slug>",
		Short: "Print details for a single panorama",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			p, err := pool.GetPanoramaBySlug(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "id:           %s\n", p.ID)
			fmt.Fprintf(out, "slug:         %s\n", p.Slug)
			fmt.Fprintf(out, "title:        %s\n", p.Title)
			if p.Description != "" {
				fmt.Fprintf(out, "description:  %s\n", p.Description)
			}
			if p.CapturedAt.Valid {
				fmt.Fprintf(out, "captured_at:  %s\n", p.CapturedAt.Time.Format("2006-01-02 15:04:05 MST"))
			}
			if p.Location != nil {
				fmt.Fprintf(out, "location:     %f,%f\n", p.Location.Lat(), p.Location.Lon())
			}
			fmt.Fprintf(out, "size:         %dx%d\n", p.Width, p.Height)
			fmt.Fprintf(out, "tile_path:    %s\n", p.TilePath)
			fmt.Fprintf(out, "thumb_prefix: %s\n", p.ThumbPrefix)
			fmt.Fprintf(out, "original:     %s\n", p.OriginalPath)
			if len(p.Tags) > 0 {
				fmt.Fprintf(out, "tags:         %v\n", p.Tags)
			}
			fmt.Fprintf(out, "info_json:    %s/%s/info.json\n", cfg.TileBaseURL, p.TilePath)
			return nil
		},
	}
}
