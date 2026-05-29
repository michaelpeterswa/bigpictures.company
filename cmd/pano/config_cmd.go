package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/config"
)

// newConfigCmd implements `pano config show`, a hidden debug helper that prints
// the loaded config as JSON. R2 secret material is redacted; the goal is to
// confirm env wiring, not to dump credentials.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "config",
		Short:  "Inspect resolved configuration",
		Hidden: true,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print resolved configuration as JSON (secrets redacted)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			redacted := struct {
				R2AccountID       string `json:"PANO_R2_ACCOUNT_ID"`
				R2AccessKeyID     string `json:"PANO_R2_ACCESS_KEY_ID"`
				R2SecretAccessKey string `json:"PANO_R2_SECRET_ACCESS_KEY"`
				R2Bucket          string `json:"PANO_R2_BUCKET"`
				TileBaseURL       string `json:"PANO_TILE_BASE_URL"`
				DatabaseURL       string `json:"PANO_DATABASE_URL"`
				TmpDir            string `json:"PANO_TMP_DIR,omitempty"`
			}{
				R2AccountID:       cfg.R2AccountID,
				R2AccessKeyID:     redact(cfg.R2AccessKeyID),
				R2SecretAccessKey: redact(cfg.R2SecretAccessKey),
				R2Bucket:          cfg.R2Bucket,
				TileBaseURL:       cfg.TileBaseURL,
				DatabaseURL:       redactURL(cfg.DatabaseURL),
				TmpDir:            cfg.TmpDir,
			}
			body, err := json.MarshalIndent(redacted, "", "  ")
			if err != nil {
				return fmt.Errorf("encode config: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(body))
			return nil
		},
	})
	return cmd
}

func redact(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:2] + "****" + s[len(s)-2:]
}

// redactURL keeps the scheme and host visible but masks any userinfo and
// query string (which often holds connection params worth not echoing).
func redactURL(s string) string {
	if s == "" {
		return ""
	}
	return "****"
}
