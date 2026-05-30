package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/config"
	"github.com/michaelpeterswa/bigpictures.company/internal/db"
	"github.com/michaelpeterswa/bigpictures.company/internal/pipeline"
	"github.com/michaelpeterswa/bigpictures.company/internal/tile"
	"github.com/michaelpeterswa/bigpictures.company/internal/upload"
)

// newReprocessCmd downloads the archived original from R2, re-tiles it, and
// re-uploads the new pyramid. The DB row is left in place — slug, EXIF, title,
// etc. are immutable here. Only width/height move (if the source was somehow
// different, which it shouldn't be).
func newReprocessCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reprocess <slug>",
		Short: "Re-tile a panorama from the archived original",
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

			r2, err := upload.NewClient(cmd.Context(), upload.Credentials{
				AccountID:       cfg.R2AccountID,
				AccessKeyID:     cfg.R2AccessKeyID,
				SecretAccessKey: cfg.R2SecretAccessKey,
				Bucket:          cfg.R2Bucket,
			})
			if err != nil {
				return err
			}

			// Pull original to a temp file so the rest of the pipeline can
			// treat it like a normal local source.
			tmp, err := os.CreateTemp(cfg.TmpDir, "pano-orig-*"+filepath.Ext(row.OriginalPath))
			if err != nil {
				return fmt.Errorf("create temp: %w", err)
			}
			defer func() { _ = os.Remove(tmp.Name()) }()
			obj, err := r2.S3().GetObject(cmd.Context(), &s3.GetObjectInput{
				Bucket: aws.String(r2.Bucket()),
				Key:    aws.String(row.OriginalPath),
			})
			if err != nil {
				return err
			}
			defer func() { _ = obj.Body.Close() }()
			if _, err := tmp.ReadFrom(obj.Body); err != nil {
				return err
			}
			if err := tmp.Close(); err != nil {
				return err
			}

			runner := &pipeline.Runner{
				Tiler:    pipeline.TileAdapter{},
				Uploader: pipeline.UploadAdapter{Client: r2},
				Repo:     pool, // unused on Reprocess: true, but interface requires non-nil
			}
			res, err := runner.Run(cmd.Context(), pipeline.Request{
				Source:    tmp.Name(),
				Slug:      row.Slug,
				Title:     row.Title,
				Reprocess: true,
			}, pipeline.Options{
				TileBaseURL: cfg.TileBaseURL,
				TmpDir:      cfg.TmpDir,
				OnPhase: func(p pipeline.Phase) {
					fmt.Fprintf(cmd.ErrOrStderr(), "[%s]\n", p)
				},
				OnTileProgress: func(p tile.Progress) {
					fmt.Fprintf(cmd.ErrOrStderr(), "\r  tile: %3d%%", p.Percent)
				},
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "reprocessed %s: %dx%d (%d tiles)\n",
				res.Slug, res.Width, res.Height, res.TileCount)
			return nil
		},
	}
}
