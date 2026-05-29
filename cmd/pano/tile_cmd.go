package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/tile"
)

// newTileCmd implements `pano tile <src> --out <dir>`, a hidden dev helper for
// smoke-testing the tiling pipeline without going through the full upload
// orchestrator. The flag-driven entry point is replaced by `pano upload` once
// Phase 5 lands.
func newTileCmd() *cobra.Command {
	var (
		out       string
		idURL     string
		quality   int
		tileSize  int
		keepTmp   bool
		concurrency int
	)
	cmd := &cobra.Command{
		Use:    "tile <src.tiff>",
		Short:  "Tile a single TIFF into a local IIIF pyramid (dev tool)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			outAbs, err := filepath.Abs(out)
			if err != nil {
				return err
			}
			res, err := tile.Tile(cmd.Context(), src, outAbs, tile.Options{
				ID:          idURL,
				Quality:     quality,
				TileSize:    tileSize,
				Concurrency: concurrency,
				Progress: func(p tile.Progress) {
					fmt.Fprintf(cmd.ErrOrStderr(), "\rtile: %3d%%", p.Percent)
					if p.Done {
						fmt.Fprintln(cmd.ErrOrStderr())
					}
				},
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"ok: %dx%d, %d tiles\n  info: %s\n",
				res.Width, res.Height, res.TileCount, res.InfoJSONPath,
			)
			_ = keepTmp
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "./tmp/out", "Output directory")
	cmd.Flags().StringVar(&idURL, "id", "", "IIIF id URL to write into info.json")
	cmd.Flags().IntVarP(&quality, "quality", "Q", 82, "WebP quality")
	cmd.Flags().IntVar(&tileSize, "tile-size", 256, "Tile size in pixels")
	cmd.Flags().IntVar(&concurrency, "concurrency", 0, "VIPS_CONCURRENCY (0 = vips default)")
	cmd.Flags().BoolVar(&keepTmp, "keep-tmp", false, "Keep the output directory after the command (reserved)")
	return cmd
}
