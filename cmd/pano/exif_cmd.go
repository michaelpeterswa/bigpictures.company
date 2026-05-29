package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/exif"
)

// newExifCmd implements `pano exif <file.tiff>`, a hidden dev helper that
// prints the normalized EXIF JSON that would be written to the database.
func newExifCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "exif <src>",
		Short:  "Print normalized EXIF for a TIFF (dev tool)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			md, err := exif.Extract(path)
			if err != nil {
				return err
			}
			body, err := json.MarshalIndent(md, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(body))
			return nil
		},
	}
}
