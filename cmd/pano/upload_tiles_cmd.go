package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/config"
	"github.com/michaelpeterswa/bigpictures.company/internal/upload"
)

// newUploadTilesCmd implements `pano upload-tiles <localDir> --prefix <key>`,
// a hidden dev helper for smoke-testing the R2 uploader against a real bucket
// without going through the full upload pipeline.
func newUploadTilesCmd() *cobra.Command {
	var prefix string
	cmd := &cobra.Command{
		Use:    "upload-tiles <localDir>",
		Short:  "Sync a local directory to R2 (dev tool)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireR2(); err != nil {
				return err
			}
			client, err := upload.NewClient(cmd.Context(), upload.Credentials{
				AccountID:       cfg.R2AccountID,
				AccessKeyID:     cfg.R2AccessKeyID,
				SecretAccessKey: cfg.R2SecretAccessKey,
				Bucket:          cfg.R2Bucket,
			})
			if err != nil {
				return err
			}
			err = client.UploadDir(cmd.Context(), args[0], prefix, upload.DirOptions{
				Progress: func(p upload.Progress) {
					fmt.Fprintf(cmd.ErrOrStderr(), "\rupload: %4d/%4d", p.UploadedFiles, p.TotalFiles)
				},
			})
			fmt.Fprintln(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "uploaded to %s/%s/\n", cfg.TileBaseURL, prefix)
			return nil
		},
	}
	cmd.Flags().StringVar(&prefix, "prefix", "", "Bucket key prefix (e.g. panos/mt-rainier)")
	_ = cmd.MarkFlagRequired("prefix")
	return cmd
}
