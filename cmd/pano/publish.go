package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/config"
	"github.com/michaelpeterswa/bigpictures.company/internal/db"
	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
	slugpkg "github.com/michaelpeterswa/bigpictures.company/internal/slug"
)

// publishFlags configures the `pano publish` recovery subcommand.
type publishFlags struct {
	title       string
	description string
	captured    string // YYYY-MM-DD
	location    string // "lat,lon"
	tags        []string
	originalExt string
}

// newPublishCmd builds the `pano publish` recovery subcommand.
//
// `pano upload` does its work in this order: tile → upload tiles → upload
// original → INSERT row. If the INSERT step fails (the historical case is the
// Neon pooler wedging the initial dial after a multi-minute upload idle
// window), the R2 work is already done — only the database row is missing.
// This subcommand fetches the live info.json from the tile base URL to read
// width and height, then writes the row using user-supplied metadata. No
// tiling, no re-upload.
//
// Idempotency: the panoramas table has a UNIQUE constraint on slug, so an
// accidental second publish for the same slug is rejected with a typed
// db/slug-conflict Problem.
func newPublishCmd() *cobra.Command {
	var f publishFlags
	cmd := &cobra.Command{
		Use:   "publish <slug>",
		Short: "Register an already-uploaded panorama in the database",
		Long: `Recovery for an upload whose R2 work succeeded but whose database
insert failed. Reads width/height from the live info.json at
<PANO_TILE_BASE_URL>/panos/<slug>/info.json, then writes the row.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPublish(cmd, args[0], f)
		},
	}
	cmd.Flags().StringVar(&f.title, "title", "", "Display title (required)")
	cmd.Flags().StringVar(&f.description, "description", "", "Description")
	cmd.Flags().StringVar(&f.captured, "captured", "", "Capture date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&f.location, "location", "", `Location "lat,lon"`)
	cmd.Flags().StringSliceVar(&f.tags, "tags", nil, "Tags (comma-separated)")
	cmd.Flags().StringVar(&f.originalExt, "original-ext", ".tif", "Extension of the archived original in R2 (used to compose originals/<slug><ext>)")
	return cmd
}

func runPublish(cmd *cobra.Command, slug string, f publishFlags) error {
	if err := slugpkg.Validate(slug); err != nil {
		return err
	}
	if strings.TrimSpace(f.title) == "" {
		return errors.New("--title is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.RequireDatabase(); err != nil {
		return err
	}

	infoURL := strings.TrimRight(cfg.TileBaseURL, "/") + "/panos/" + slug + "/info.json"
	width, height, err := fetchTileDimensions(cmd.Context(), infoURL)
	if err != nil {
		return err
	}

	tilePrefix := "panos/" + slug
	row := &db.Panorama{
		Slug:         slug,
		Title:        f.title,
		Description:  f.description,
		Width:        width,
		Height:       height,
		TilePath:     tilePrefix,
		ThumbPrefix:  tilePrefix,
		OriginalPath: "originals/" + slug + f.originalExt,
		Tags:         f.tags,
	}
	if f.captured != "" {
		t, err := time.Parse("2006-01-02", f.captured)
		if err != nil {
			return fmt.Errorf("invalid --captured %q: %w", f.captured, err)
		}
		row.CapturedAt = pgtype.Timestamptz{Time: t, Valid: true}
	}
	if f.location != "" {
		pt, err := parseLatLon(f.location)
		if err != nil {
			return err
		}
		row.Location = pt
	}

	pool, err := db.Open(cmd.Context(), cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Bound the insert. Same 30s budget as the upload-path safety net; if the
	// DB is unreachable, fail loudly rather than hang.
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	id, err := pool.InsertPanorama(ctx, row)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "published %s\n  id:    %s\n  size:  %dx%d\n  info:  %s\n",
		slug, id, width, height, infoURL)
	return nil
}

// fetchTileDimensions GETs the IIIF info.json at url and returns its width
// and height. Surfaces non-2xx responses and missing fields as typed Problems
// so the CLI renders something the operator can act on.
func fetchTileDimensions(ctx context.Context, url string) (int, int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, problems.New(
			"publish/info-fetch-failed",
			"Could not build request for info.json",
			err.Error(),
			problems.WithExt(problems.Ext("url", url)),
		)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, problems.New(
			"publish/info-fetch-failed",
			"Could not fetch info.json",
			err.Error(),
			problems.WithExt(problems.Ext("url", url)),
		)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, problems.New(
			"publish/info-fetch-failed",
			"Unexpected status from info.json",
			fmt.Sprintf("got %d %s", resp.StatusCode, resp.Status),
			problems.WithExt(
				problems.Ext("url", url),
				problems.Ext("status", resp.StatusCode),
			),
		)
	}
	var info struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return 0, 0, problems.New(
			"publish/info-fetch-failed",
			"Malformed info.json",
			err.Error(),
			problems.WithExt(problems.Ext("url", url)),
		)
	}
	if info.Width <= 0 || info.Height <= 0 {
		return 0, 0, problems.New(
			"publish/info-fetch-failed",
			"info.json missing width or height",
			fmt.Sprintf("width=%d height=%d", info.Width, info.Height),
			problems.WithExt(problems.Ext("url", url)),
		)
	}
	return info.Width, info.Height, nil
}
